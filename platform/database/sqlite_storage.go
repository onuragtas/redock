package database

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	// Use modernc.org/sqlite driver (pure Go, no CGO required)
	_ "modernc.org/sqlite"
)

var (
	globalDB *gorm.DB
)

// InitSQLiteStorage initializes the SQLite database with GORM
func InitSQLiteStorage(workDir string) error {
	dbPath := filepath.Join(workDir, "data", "redock.db")

	// Ensure data directory exists
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Open database connection using modernc.org/sqlite (pure Go, no CGO)
	db, err := gorm.Open(sqlite.Dialector{
		DriverName: "sqlite",
		DSN:        dbPath,
	}, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	globalDB = db

	// Run schema migrations
	if err := RunMigrations(db); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	log.Printf("✅ SQLite database initialized at: %s", dbPath)
	return nil
}

// GetDB returns the global database instance
func GetDB() *gorm.DB {
	return globalDB
}
