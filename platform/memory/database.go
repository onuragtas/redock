package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
)

// Database is a generic in-memory database
type Database struct {
	baseDir   string
	tables    map[string]*Table
	mutex     sync.RWMutex
	ctx       chan struct{}
	wg        sync.WaitGroup
	closed    bool
	closeMux  sync.Mutex
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

// Register registers a new entity type
func Register[T Entity](db *Database, tableName string) error {
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
	}

	// Load existing data
	if err := table.load(db.baseDir); err != nil {
		return fmt.Errorf("failed to load table %s: %w", tableName, err)
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
	table.dirty = true

	return nil
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

// load loads table data from JSON file
func (t *Table) load(baseDir string) error {
	path := filepath.Join(baseDir, t.filename)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist, not an error
		}
		return err
	}

	var wrapper struct {
		Meta struct {
			Version        int       `json:"version"`
			LastMigration  string    `json:"last_migration"`
			MigratedAt     time.Time `json:"migrated_at"`
		} `json:"_meta"`
		Data []json.RawMessage `json:"data"`
	}

	if err := json.Unmarshal(data, &wrapper); err != nil {
		return err
	}

	// Deserialize each entity
	for _, raw := range wrapper.Data {
		// First unmarshal to map to fix SQLite integer booleans
		var tempMap map[string]interface{}
		if err := json.Unmarshal(raw, &tempMap); err != nil {
			continue
		}

		// Fix SQLite integer booleans (0/1 → false/true)
		normalizeSQLiteBooleans(tempMap)

		// Marshal back to JSON with fixed booleans
		fixedRaw, err := json.Marshal(tempMap)
		if err != nil {
			continue
		}

		// Create new instance of the entity type
		entityPtr := reflect.New(t.indexType).Interface()

		if err := json.Unmarshal(fixedRaw, entityPtr); err != nil {
			continue
		}

		entity := entityPtr.(Entity)
		id := entity.GetID()
		
		t.data[id] = entity

		// Update nextID
		if id >= uint(t.nextID) {
			t.nextID = uint32(id + 1)
		}
	}

	return nil
}

// save writes table data to JSON file
func (t *Table) save(baseDir string) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if !t.dirty {
		return nil
	}

	path := filepath.Join(baseDir, t.filename)
	tmpPath := path + ".tmp"

	// Convert map to slice
	entities := make([]interface{}, 0, len(t.data))
	for _, entity := range t.data {
		entities = append(entities, entity)
	}

	// Wrap with metadata
	wrapper := struct {
		Meta struct {
			Version       int       `json:"version"`
			LastMigration string    `json:"last_migration"`
			UpdatedAt     time.Time `json:"updated_at"`
		} `json:"_meta"`
		Data []interface{} `json:"data"`
	}{
		Data: entities,
	}

	wrapper.Meta.Version = 5
	wrapper.Meta.LastMigration = "005_vpn_server"
	wrapper.Meta.UpdatedAt = time.Now()

	// Marshal with indentation
	jsonData, err := json.MarshalIndent(wrapper, "", "  ")
	if err != nil {
		return err
	}

	// Write to temp file
	if err := os.WriteFile(tmpPath, jsonData, 0644); err != nil {
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
