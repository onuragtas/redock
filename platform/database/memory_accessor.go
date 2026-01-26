package database

import "redock/platform/memory"

var globalMemoryDB *memory.Database

// SetGlobalDB sets the global memory database
func SetGlobalDB(db *memory.Database) {
	globalMemoryDB = db
}

// GetMemoryDB returns the global memory database
func GetMemoryDB() *memory.Database {
	return globalMemoryDB
}
