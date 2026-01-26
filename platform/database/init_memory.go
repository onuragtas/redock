package database

import (
	"fmt"
	"redock/platform/memory"
)

// InitMemoryDB initializes the generic in-memory database
// Entity registration should be done by caller to avoid circular imports
func InitMemoryDB(dataDir string) (*memory.Database, error) {
	db, err := memory.NewDatabase(dataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create database: %w", err)
	}

	// Set as global database
	SetGlobalDB(db)

	return db, nil
}
