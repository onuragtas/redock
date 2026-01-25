package database

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

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

	// DSN with SQLite optimizations for concurrent access
	// WAL mode: Allows concurrent reads and writes
	// Busy timeout: Wait up to 10 seconds before returning SQLITE_BUSY (increased from 5s)
	// Cache size: Larger cache for better performance
	// Synchronous NORMAL: Balance between safety and speed
	dsn := dbPath + "?_journal_mode=WAL&_busy_timeout=10000&_synchronous=NORMAL&_cache_size=1000000000&_txlock=immediate"

	// Open database connection using modernc.org/sqlite (pure Go, no CGO)
	db, err := gorm.Open(sqlite.Dialector{
		DriverName: "sqlite",
		DSN:        dsn,
	}, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
		// Prepare statements for better performance
		PrepareStmt: true,
	})
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}

	// SQLite with WAL mode supports concurrent reads + single writer
	// For high-traffic applications (DNS server, API Gateway), we need more connections
	sqlDB.SetMaxOpenConns(25)                  // Max 25 concurrent connections (good for WAL mode)
	sqlDB.SetMaxIdleConns(10)                  // Keep 10 idle connections ready
	sqlDB.SetConnMaxLifetime(time.Hour * 1)    // Recycle connections every 1 hour (prevents stale connections)
	sqlDB.SetConnMaxIdleTime(time.Minute * 10) // Close idle connections after 10 minutes

	globalDB = db

	// Run schema migrations
	if err := RunMigrations(db); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

// GetDB returns the global database instance
func GetDB() *gorm.DB {
	return globalDB
}
