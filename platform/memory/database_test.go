package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type logRow struct {
	BaseEntity
	Domain  string `json:"domain"`
	Blocked bool   `json:"blocked"`
}

func newTestDB(t *testing.T) (*Database, string) {
	t.Helper()

	dir := t.TempDir()
	db, err := NewDatabase(dir)
	if err != nil {
		t.Fatalf("NewDatabase: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, dir
}

func TestSetTableLimitEvictsOldestRows(t *testing.T) {
	db, _ := newTestDB(t)
	if err := Register[*logRow](db, "logs"); err != nil {
		t.Fatalf("Register: %v", err)
	}

	SetTableLimit(db, "logs", 10)

	for i := 0; i < 100; i++ {
		if err := Create(db, "logs", &logRow{Domain: "example.com"}); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	rows := FindAll[*logRow](db, "logs")
	if len(rows) != 10 {
		t.Fatalf("expected the table to stay capped at 10 rows, got %d", len(rows))
	}

	// The survivors must be the newest ones (highest IDs).
	for _, r := range rows {
		if r.GetID() <= 90 {
			t.Fatalf("expected only the 10 newest rows, found id %d", r.GetID())
		}
	}
}

func TestSetTableLimitTrimsExistingRows(t *testing.T) {
	db, _ := newTestDB(t)
	if err := Register[*logRow](db, "logs"); err != nil {
		t.Fatalf("Register: %v", err)
	}

	for i := 0; i < 50; i++ {
		if err := Create(db, "logs", &logRow{Domain: "example.com"}); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	dropped := SetTableLimit(db, "logs", 20)
	if dropped != 30 {
		t.Fatalf("expected 30 rows dropped when capping an existing table, got %d", dropped)
	}
	if got := len(FindAll[*logRow](db, "logs")); got != 20 {
		t.Fatalf("expected 20 rows after capping, got %d", got)
	}
}

func TestTrimTableKeepsNewest(t *testing.T) {
	db, _ := newTestDB(t)
	if err := Register[*logRow](db, "logs"); err != nil {
		t.Fatalf("Register: %v", err)
	}

	for i := 0; i < 30; i++ {
		if err := Create(db, "logs", &logRow{Domain: "example.com"}); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	if dropped := TrimTable(db, "logs", 5); dropped != 25 {
		t.Fatalf("expected 25 rows dropped, got %d", dropped)
	}

	rows := FindAll[*logRow](db, "logs")
	if len(rows) != 5 {
		t.Fatalf("expected 5 rows kept, got %d", len(rows))
	}
	for _, r := range rows {
		if r.GetID() <= 25 {
			t.Fatalf("TrimTable kept an old row (id %d)", r.GetID())
		}
	}

	// Trimming below the current size is a no-op.
	if dropped := TrimTable(db, "logs", 10); dropped != 0 {
		t.Fatalf("expected no rows dropped when keep > size, got %d", dropped)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	db, dir := newTestDB(t)
	if err := Register[*logRow](db, "logs"); err != nil {
		t.Fatalf("Register: %v", err)
	}

	for i := 0; i < 5; i++ {
		if err := Create(db, "logs", &logRow{Domain: "example.com", Blocked: i%2 == 0}); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}
	if err := db.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// The file must be valid JSON in the documented shape.
	raw, err := os.ReadFile(filepath.Join(dir, "logs.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var parsed struct {
		Meta struct {
			UpdatedAt time.Time `json:"updated_at"`
		} `json:"_meta"`
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("saved file is not valid JSON: %v", err)
	}
	if len(parsed.Data) != 5 {
		t.Fatalf("expected 5 records on disk, got %d", len(parsed.Data))
	}
	if parsed.Meta.UpdatedAt.IsZero() {
		t.Fatal("expected _meta.updated_at to be written")
	}

	// A fresh database over the same directory must read it back.
	db2, err := NewDatabase(dir)
	if err != nil {
		t.Fatalf("NewDatabase: %v", err)
	}
	defer db2.Close()
	if err := Register[*logRow](db2, "logs"); err != nil {
		t.Fatalf("Register on reload: %v", err)
	}

	rows := FindAll[*logRow](db2, "logs")
	if len(rows) != 5 {
		t.Fatalf("expected 5 rows after reload, got %d", len(rows))
	}
	blocked := 0
	for _, r := range rows {
		if r.Blocked {
			blocked++
		}
		if r.Domain != "example.com" {
			t.Fatalf("unexpected domain after reload: %q", r.Domain)
		}
	}
	if blocked != 3 {
		t.Fatalf("expected 3 blocked rows after reload, got %d", blocked)
	}
}

func TestLoadLegacyIndentedFile(t *testing.T) {
	dir := t.TempDir()

	// The previous writer used MarshalIndent and SQLite-style integer booleans;
	// both must still load.
	legacy := `{
  "_meta": {
    "updated_at": "2026-01-01T00:00:00Z"
  },
  "data": [
    {
      "id": 7,
      "created_at": "2026-01-01T00:00:00Z",
      "updated_at": "2026-01-01T00:00:00Z",
      "domain": "legacy.test",
      "blocked": 1
    }
  ]
}`
	if err := os.WriteFile(filepath.Join(dir, "logs.json"), []byte(legacy), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	db, err := NewDatabase(dir)
	if err != nil {
		t.Fatalf("NewDatabase: %v", err)
	}
	defer db.Close()
	if err := Register[*logRow](db, "logs"); err != nil {
		t.Fatalf("Register: %v", err)
	}

	row, err := FindByID[*logRow](db, "logs", 7)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if row.Domain != "legacy.test" {
		t.Fatalf("unexpected domain: %q", row.Domain)
	}
	if !row.Blocked {
		t.Fatal("expected the integer boolean 1 to load as true")
	}

	// New rows must not reuse the legacy ID.
	fresh := &logRow{Domain: "new.test"}
	if err := Create(db, "logs", fresh); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if fresh.GetID() <= 7 {
		t.Fatalf("new row reused a legacy ID: %d", fresh.GetID())
	}
}

// TestRegisterWithLimitCapsDuringLoad proves the cap is applied while the file
// is streamed, not after the whole thing is in memory.
func TestRegisterWithLimitCapsDuringLoad(t *testing.T) {
	dir := t.TempDir()

	// Build a file with 5000 records.
	f, err := os.Create(filepath.Join(dir, "logs.json"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := f.WriteString(`{"_meta":{"updated_at":"2026-01-01T00:00:00Z"},"data":[`); err != nil {
		t.Fatalf("write: %v", err)
	}
	enc := json.NewEncoder(f)
	for i := 1; i <= 5000; i++ {
		if i > 1 {
			f.WriteString(",")
		}
		if err := enc.Encode(&logRow{BaseEntity: BaseEntity{ID: uint(i)}, Domain: "x.test"}); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}
	f.WriteString("]}")
	f.Close()

	db, err := NewDatabase(dir)
	if err != nil {
		t.Fatalf("NewDatabase: %v", err)
	}
	defer db.Close()

	if err := RegisterWithLimit[*logRow](db, "logs", 100); err != nil {
		t.Fatalf("RegisterWithLimit: %v", err)
	}

	rows := FindAll[*logRow](db, "logs")
	if len(rows) != 100 {
		t.Fatalf("expected the cap to hold during load, got %d rows", len(rows))
	}
	for _, r := range rows {
		if r.GetID() <= 4900 {
			t.Fatalf("expected only the newest 100 rows, found id %d", r.GetID())
		}
	}
}
