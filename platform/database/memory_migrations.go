package database

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	"redock/platform/memory"
)

const memoryMigrationsFile = "memory_migrations.json"

// MemoryMigration defines a one-time migration for the memory database.
// Migrations run in Version order; each runs at most once.
// Up receives dataDir so migrations can read legacy JSON files (e.g. data/local_proxy.json).
type MemoryMigration struct {
	Version int
	Name    string
	Up      func(db *memory.Database, dataDir string) error
}

// AppliedMigration records a migration that has been run (for state file).
type AppliedMigration struct {
	Version   int       `json:"version"`
	Name      string    `json:"name"`
	AppliedAt time.Time `json:"applied_at"`
}

// memoryMigrationState persists which migrations have been applied.
type memoryMigrationState struct {
	Applied []AppliedMigration `json:"applied"`
}

// RunMemoryMigrations runs all pending migrations in version order.
// State is stored in dataDir/.memory_migrations.json.
func RunMemoryMigrations(db *memory.Database, dataDir string, migrations []MemoryMigration) error {
	if len(migrations) == 0 {
		return nil
	}

	statePath := filepath.Join(dataDir, memoryMigrationsFile)
	state, err := loadMemoryMigrationState(statePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("load migration state: %w", err)
	}

	appliedSet := make(map[int]bool)
	for _, a := range state.Applied {
		appliedSet[a.Version] = true
	}

	sorted := make([]MemoryMigration, len(migrations))
	copy(sorted, migrations)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Version < sorted[j].Version })

	for _, m := range sorted {
		if appliedSet[m.Version] {
			continue
		}
		log.Printf("📦 Memory migration %d: %s", m.Version, m.Name)
		if err := m.Up(db, dataDir); err != nil {
			return fmt.Errorf("migration %d (%s): %w", m.Version, m.Name, err)
		}
		state.Applied = append(state.Applied, AppliedMigration{
			Version:   m.Version,
			Name:      m.Name,
			AppliedAt: time.Now(),
		})
		if err := saveMemoryMigrationState(statePath, state); err != nil {
			return fmt.Errorf("save migration state after %d: %w", m.Version, err)
		}
	}

	return nil
}

func loadMemoryMigrationState(path string) (memoryMigrationState, error) {
	var state memoryMigrationState
	data, err := os.ReadFile(path)
	if err != nil {
		return state, err
	}
	// Support legacy format: {"applied": [1, 2]}
	var raw struct {
		Applied json.RawMessage `json:"applied"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return state, err
	}
	var versions []int
	if err := json.Unmarshal(raw.Applied, &versions); err == nil && len(versions) > 0 {
		for _, v := range versions {
			state.Applied = append(state.Applied, AppliedMigration{Version: v})
		}
		return state, nil
	}
	if err := json.Unmarshal(raw.Applied, &state.Applied); err != nil {
		return state, err
	}
	return state, nil
}

func saveMemoryMigrationState(path string, state memoryMigrationState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
