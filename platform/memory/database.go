package memory

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Database is a generic in-memory database
type Database struct {
	baseDir  string
	tables   map[string]*Table
	mutex    sync.RWMutex
	ctx      chan struct{}
	wg       sync.WaitGroup
	closed   bool
	closeMux sync.Mutex
}

// Table holds a collection of entities
type Table struct {
	name      string
	filename  string
	data      map[uint]interface{} // map[id]entity
	nextID    uint32
	mutex     sync.RWMutex
	dirty     bool
	indexType reflect.Type

	// maxRows caps how many rows the table may hold in RAM. 0 means unbounded.
	// Append-only log tables (DNS query logs, connection logs, …) must set this
	// or they grow until the process is killed.
	maxRows int
	// order keeps insertion order for eviction; only maintained when maxRows > 0.
	order []uint
}

// Entity is the interface that all storable entities must implement
type Entity interface {
	GetID() uint
	SetID(id uint)
	SetTimestamps(created, updated time.Time)
}

// NewDatabase creates a new in-memory database
func NewDatabase(baseDir string) (*Database, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	db := &Database{
		baseDir: baseDir,
		tables:  make(map[string]*Table),
		ctx:     make(chan struct{}),
	}

	// Start periodic writer
	db.wg.Add(1)
	go db.periodicWriter()

	return db, nil
}

// Register registers a new entity type with no row cap.
func Register[T Entity](db *Database, tableName string) error {
	return RegisterWithLimit[T](db, tableName, 0)
}

// RegisterWithLimit registers an entity type whose table is capped at maxRows,
// with the cap enforced while the file is being read. Use it for append-only
// log tables: registering them uncapped and trimming afterwards would still
// spike the heap with the entire file's worth of rows first.
func RegisterWithLimit[T Entity](db *Database, tableName string, maxRows int) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	if _, exists := db.tables[tableName]; exists {
		return fmt.Errorf("table %s already registered", tableName)
	}

	var zero T
	entityType := reflect.TypeOf(zero)
	// If T is a pointer, get the underlying type
	if entityType.Kind() == reflect.Ptr {
		entityType = entityType.Elem()
	}

	table := &Table{
		name:      tableName,
		filename:  tableName + ".json",
		data:      make(map[uint]interface{}),
		indexType: entityType,
		maxRows:   maxRows,
	}

	// Load existing data; if format is invalid (e.g. legacy), use empty table
	if err := table.load(db.baseDir); err != nil {
		log.Printf("memory: failed to load table %s (using empty table): %v", tableName, err)
	}

	db.tables[tableName] = table
	return nil
}

// Create creates a new entity
func Create[T Entity](db *Database, tableName string, entity T) error {
	db.mutex.RLock()
	table, exists := db.tables[tableName]
	db.mutex.RUnlock()

	if !exists {
		return fmt.Errorf("table %s not found", tableName)
	}

	table.mutex.Lock()
	defer table.mutex.Unlock()

	// Auto-increment ID
	id := atomic.AddUint32(&table.nextID, 1)
	entity.SetID(uint(id))

	now := time.Now()
	entity.SetTimestamps(now, now)

	table.data[uint(id)] = entity
	table.trackAndEvictLocked(uint(id))
	table.dirty = true

	return nil
}

// CreatePreserveTimestamps inserts a new entity with auto-increment ID but does not overwrite CreatedAt/UpdatedAt.
// Used by migrations that import existing data (e.g. JSONL) where timestamps must be preserved.
func CreatePreserveTimestamps[T Entity](db *Database, tableName string, entity T) error {
	db.mutex.RLock()
	table, exists := db.tables[tableName]
	db.mutex.RUnlock()

	if !exists {
		return fmt.Errorf("table %s not found", tableName)
	}

	table.mutex.Lock()
	defer table.mutex.Unlock()

	id := atomic.AddUint32(&table.nextID, 1)
	entity.SetID(uint(id))
	// Do not call SetTimestamps — entity keeps existing CreatedAt/UpdatedAt

	table.data[uint(id)] = entity
	table.trackAndEvictLocked(uint(id))
	table.dirty = true

	return nil
}

// SetTableLimit caps a table at maxRows entries, evicting the oldest rows
// (lowest IDs) first. Call it right after Register for append-only log tables.
// maxRows <= 0 removes the cap. Returns the number of rows evicted immediately.
func SetTableLimit(db *Database, tableName string, maxRows int) int {
	db.mutex.RLock()
	table, exists := db.tables[tableName]
	db.mutex.RUnlock()

	if !exists {
		return 0
	}

	table.mutex.Lock()
	defer table.mutex.Unlock()

	table.maxRows = maxRows
	if maxRows <= 0 {
		table.order = nil
		return 0
	}

	// Rebuild the eviction order from the current contents (ID is monotonic, so
	// ascending ID is insertion order).
	table.order = make([]uint, 0, len(table.data))
	for id := range table.data {
		table.order = append(table.order, id)
	}
	sort.Slice(table.order, func(i, j int) bool { return table.order[i] < table.order[j] })

	return table.evictOverflowLocked()
}

// trackAndEvictLocked records a newly inserted ID and drops the oldest rows when
// the table is over its cap. Caller holds t.mutex.
func (t *Table) trackAndEvictLocked(id uint) {
	if t.maxRows <= 0 {
		return
	}
	t.order = append(t.order, id)
	t.evictOverflowLocked()
}

// evictOverflowLocked removes oldest rows until the table fits its cap.
// Caller holds t.mutex. Returns how many rows were dropped.
func (t *Table) evictOverflowLocked() int {
	if t.maxRows <= 0 {
		return 0
	}

	dropped := 0
	idx := 0
	for len(t.data) > t.maxRows && idx < len(t.order) {
		oldest := t.order[idx]
		idx++
		if _, ok := t.data[oldest]; !ok {
			continue // already deleted through Delete()
		}
		delete(t.data, oldest)
		dropped++
	}

	if idx > 0 {
		// Compact the order slice; copying into a fresh slice keeps the backing
		// array from growing without bound on a hot append-only table.
		remaining := make([]uint, 0, len(t.order)-idx)
		remaining = append(remaining, t.order[idx:]...)
		t.order = remaining
	}
	if dropped > 0 {
		t.dirty = true
	}
	return dropped
}

// TableInfo reports a table's in-memory footprint for the memory dashboard.
type TableInfo struct {
	Name    string `json:"name"`
	Rows    int    `json:"rows"`
	MaxRows int    `json:"max_rows"`
	Dirty   bool   `json:"dirty"`
}

// Tables lists every registered table with its current row count, so the
// dashboard can point at whatever is actually eating memory.
func (db *Database) Tables() []TableInfo {
	db.mutex.RLock()
	tables := make([]*Table, 0, len(db.tables))
	for _, t := range db.tables {
		tables = append(tables, t)
	}
	db.mutex.RUnlock()

	out := make([]TableInfo, 0, len(tables))
	for _, t := range tables {
		t.mutex.RLock()
		out = append(out, TableInfo{Name: t.name, Rows: len(t.data), MaxRows: t.maxRows, Dirty: t.dirty})
		t.mutex.RUnlock()
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rows > out[j].Rows })
	return out
}

// TrimTable drops the oldest rows of a table until at most keep remain. Used by
// the memory guard to reclaim RAM from log-like tables under pressure. Returns
// the number of rows dropped.
func TrimTable(db *Database, tableName string, keep int) int {
	if keep < 0 {
		keep = 0
	}

	db.mutex.RLock()
	table, exists := db.tables[tableName]
	db.mutex.RUnlock()

	if !exists {
		return 0
	}

	table.mutex.Lock()
	defer table.mutex.Unlock()

	if len(table.data) <= keep {
		return 0
	}

	ids := make([]uint, 0, len(table.data))
	for id := range table.data {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	dropCount := len(ids) - keep
	for _, id := range ids[:dropCount] {
		delete(table.data, id)
	}
	table.dirty = true

	if table.maxRows > 0 {
		remaining := make([]uint, 0, keep)
		for _, id := range table.order {
			if _, ok := table.data[id]; ok {
				remaining = append(remaining, id)
			}
		}
		table.order = remaining
	}

	return dropCount
}

// Update updates an existing entity
func Update[T Entity](db *Database, tableName string, entity T) error {
	db.mutex.RLock()
	table, exists := db.tables[tableName]
	db.mutex.RUnlock()

	if !exists {
		return fmt.Errorf("table %s not found", tableName)
	}

	table.mutex.Lock()
	defer table.mutex.Unlock()

	id := entity.GetID()
	if _, exists := table.data[id]; !exists {
		return fmt.Errorf("entity with ID %d not found", id)
	}

	// Update timestamp
	v := reflect.ValueOf(entity)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	updatedAtField := v.FieldByName("UpdatedAt")
	if updatedAtField.IsValid() && updatedAtField.CanSet() {
		updatedAtField.Set(reflect.ValueOf(time.Now()))
	}

	table.data[id] = entity
	table.dirty = true

	return nil
}

// Delete deletes an entity by ID
func Delete[T Entity](db *Database, tableName string, id uint) error {
	db.mutex.RLock()
	table, exists := db.tables[tableName]
	db.mutex.RUnlock()

	if !exists {
		return fmt.Errorf("table %s not found", tableName)
	}

	table.mutex.Lock()
	defer table.mutex.Unlock()

	delete(table.data, id)
	table.dirty = true

	return nil
}

// FindByID finds an entity by ID
func FindByID[T Entity](db *Database, tableName string, id uint) (T, error) {
	var zero T

	db.mutex.RLock()
	table, exists := db.tables[tableName]
	db.mutex.RUnlock()

	if !exists {
		return zero, fmt.Errorf("table %s not found", tableName)
	}

	table.mutex.RLock()
	defer table.mutex.RUnlock()

	entity, exists := table.data[id]
	if !exists {
		return zero, fmt.Errorf("entity with ID %d not found", id)
	}

	return entity.(T), nil
}

// FindAll returns all entities
func FindAll[T Entity](db *Database, tableName string) []T {
	db.mutex.RLock()
	table, exists := db.tables[tableName]
	db.mutex.RUnlock()

	if !exists {
		return []T{}
	}

	table.mutex.RLock()
	defer table.mutex.RUnlock()

	result := make([]T, 0, len(table.data))
	for _, entity := range table.data {
		result = append(result, entity.(T))
	}

	// table.data is a map (random iteration order); sort by ID for a stable,
	// insertion-ordered result so list pages don't shuffle on every refresh.
	sort.Slice(result, func(i, j int) bool { return result[i].GetID() < result[j].GetID() })

	return result
}

// Where filters entities by field value
func Where[T Entity](db *Database, tableName string, fieldName string, value interface{}) []T {
	all := FindAll[T](db, tableName)
	result := make([]T, 0)

	for _, entity := range all {
		v := reflect.ValueOf(entity)
		if v.Kind() == reflect.Ptr {
			v = v.Elem()
		}

		field := v.FieldByName(fieldName)
		if !field.IsValid() {
			continue
		}

		if reflect.DeepEqual(field.Interface(), value) {
			result = append(result, entity)
		}
	}

	return result
}

// Filter filters entities using a custom function
func Filter[T Entity](db *Database, tableName string, fn func(T) bool) []T {
	all := FindAll[T](db, tableName)
	result := make([]T, 0)

	for _, entity := range all {
		if fn(entity) {
			result = append(result, entity)
		}
	}

	return result
}

// Count returns the total count of entities
func Count[T Entity](db *Database, tableName string) int {
	db.mutex.RLock()
	table, exists := db.tables[tableName]
	db.mutex.RUnlock()

	if !exists {
		return 0
	}

	table.mutex.RLock()
	defer table.mutex.RUnlock()

	return len(table.data)
}

// periodicWriter writes dirty tables to disk every 10 seconds
func (db *Database) periodicWriter() {
	defer db.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-db.ctx:
			// Final flush on shutdown
			db.flushAll()
			return
		case <-ticker.C:
			db.flushAll()
		}
	}
}

// flushAll writes all dirty tables to disk
func (db *Database) flushAll() {
	db.mutex.RLock()
	tables := make([]*Table, 0, len(db.tables))
	for _, table := range db.tables {
		tables = append(tables, table)
	}
	db.mutex.RUnlock()

	for _, table := range tables {
		table.mutex.RLock()
		if table.dirty {
			table.mutex.RUnlock()
			table.save(db.baseDir)
		} else {
			table.mutex.RUnlock()
		}
	}
}

// load streams table data from its JSON file. The whole file is deliberately
// never held in memory at once: a multi-hundred-MB log table would otherwise
// cost the file size several times over during boot (raw bytes + RawMessage
// slice + per-record map round-trip) and spike the process into an OOM kill.
func (t *Table) load(baseDir string) error {
	path := filepath.Join(baseDir, t.filename)

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist, not an error
		}
		return err
	}
	defer f.Close()

	dec := json.NewDecoder(bufio.NewReaderSize(f, 1<<20))

	// Walk to the "data" array without materialising the rest of the document.
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return fmt.Errorf("table %s: unexpected JSON root", t.name)
	}

	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return err
		}
		key, _ := keyTok.(string)
		if key != "data" {
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				return err
			}
			continue
		}

		// Entering the data array: decode one record at a time.
		arrTok, err := dec.Token()
		if err != nil {
			return err
		}
		if delim, ok := arrTok.(json.Delim); !ok || delim != '[' {
			return fmt.Errorf("table %s: \"data\" is not an array", t.name)
		}

		for dec.More() {
			var raw json.RawMessage
			if err := dec.Decode(&raw); err != nil {
				return err
			}
			entity, ok := t.decodeEntity(raw)
			if !ok {
				continue
			}

			id := entity.GetID()
			t.data[id] = entity
			// Enforce the cap as we go: a capped table never holds more than
			// maxRows rows, no matter how large the file is.
			t.trackAndEvictLocked(id)

			// Update nextID
			if id >= uint(t.nextID) {
				t.nextID = uint32(id + 1)
			}
		}

		if _, err := dec.Token(); err != nil { // closing ']'
			return err
		}
	}

	return nil
}

// decodeEntity turns one raw record into an entity. The fast path decodes
// straight into the struct; only records that fail (legacy SQLite rows with
// integer booleans) pay for the map normalisation round-trip.
func (t *Table) decodeEntity(raw json.RawMessage) (Entity, bool) {
	entityPtr := reflect.New(t.indexType).Interface()
	if err := json.Unmarshal(raw, entityPtr); err == nil {
		if entity, ok := entityPtr.(Entity); ok {
			return entity, true
		}
		return nil, false
	}

	var tempMap map[string]interface{}
	if err := json.Unmarshal(raw, &tempMap); err != nil {
		return nil, false
	}
	normalizeSQLiteBooleans(tempMap)

	fixedRaw, err := json.Marshal(tempMap)
	if err != nil {
		return nil, false
	}

	entityPtr = reflect.New(t.indexType).Interface()
	if err := json.Unmarshal(fixedRaw, entityPtr); err != nil {
		return nil, false
	}
	entity, ok := entityPtr.(Entity)
	return entity, ok
}

// save streams the table to disk through a buffered writer. Encoding record by
// record keeps peak allocation at one record instead of one contiguous buffer
// holding the entire serialised table — the old MarshalIndent path allocated a
// copy larger than the data itself on every flush.
func (t *Table) save(baseDir string) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if !t.dirty {
		return nil
	}

	path := filepath.Join(baseDir, t.filename)
	tmpPath := path + ".tmp"

	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	w := bufio.NewWriterSize(f, 1<<20)
	enc := json.NewEncoder(w)

	writeErr := func() error {
		meta, err := json.Marshal(time.Now())
		if err != nil {
			return err
		}
		if _, err := w.WriteString(`{"_meta":{"updated_at":` + string(meta) + `},"data":[`); err != nil {
			return err
		}
		first := true
		for _, entity := range t.data {
			if !first {
				if _, err := w.WriteString(","); err != nil {
					return err
				}
			}
			first = false
			// Encode appends a newline; that is valid JSON whitespace here.
			if err := enc.Encode(entity); err != nil {
				return err
			}
		}
		if _, err := w.WriteString("]}"); err != nil {
			return err
		}
		return w.Flush()
	}()

	if writeErr != nil {
		f.Close()
		os.Remove(tmpPath)
		return writeErr
	}

	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}

	t.dirty = false
	return nil
}

// normalizeSQLiteBooleans converts SQLite integer booleans (0/1) to Go booleans
func normalizeSQLiteBooleans(m map[string]interface{}) {
	for key, value := range m {
		// Check if value is a number that could be a boolean
		switch v := value.(type) {
		case float64:
			// JSON unmarshals numbers as float64
			if v == 0 || v == 1 {
				// Check if field name suggests it's a boolean
				if isBooleanField(key) {
					m[key] = v == 1
				}
			}
		case int:
			if v == 0 || v == 1 {
				if isBooleanField(key) {
					m[key] = v == 1
				}
			}
		case int64:
			if v == 0 || v == 1 {
				if isBooleanField(key) {
					m[key] = v == 1
				}
			}
		}
	}
}

// isBooleanField checks if a field name suggests it's a boolean
func isBooleanField(name string) bool {
	booleanFields := []string{
		"enabled", "blocked", "cached", "is_regex", "is_wildcard",
		"doh_enabled", "dot_enabled", "blocking_enabled", "query_logging",
		"cache_enabled", "running",
	}
	for _, field := range booleanFields {
		if name == field {
			return true
		}
	}
	return false
}

// Close gracefully shuts down the database
func (db *Database) Close() error {
	db.closeMux.Lock()
	defer db.closeMux.Unlock()

	// Prevent double close
	if db.closed {
		return nil
	}

	close(db.ctx)
	db.wg.Wait()

	// Flush all dirty tables to disk before closing
	db.flushAll()
	db.closed = true

	return nil
}

// Flush forces immediate write of all dirty tables
func (db *Database) Flush() error {
	db.flushAll()
	return nil
}
