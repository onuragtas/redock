package database

import (
	"redock/platform/memory"
	"time"
)

// MigrationHistory tracks database migrations (stored in JSON meta)
type MigrationHistory struct {
	ID        uint      `json:"id"`
	Name      string    `json:"name"`
	AppliedAt time.Time `json:"applied_at"`
}

// SavedCommand represents a saved command in the database
type SavedCommand struct {
	memory.SoftDeleteEntity
	Command string `json:"command"`
	Path    string `json:"path"`
}
