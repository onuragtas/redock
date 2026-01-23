package database

import (
	"time"

	"gorm.io/gorm"
)

// MigrationHistory tracks database migrations
type MigrationHistory struct {
	ID        uint      `gorm:"primarykey"`
	Name      string    `gorm:"uniqueIndex;not null"`
	AppliedAt time.Time `gorm:"not null"`
}

// SavedCommand represents a saved command in the database
type SavedCommand struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	Command   string         `gorm:"uniqueIndex;not null" json:"command"`
	Path      string         `gorm:"not null" json:"path"`
}
