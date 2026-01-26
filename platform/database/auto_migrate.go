package database

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"redock/platform/memory"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// MigrationStatus tracks migration state
type MigrationStatus struct {
	Version           int       `json:"version"`
	LastMigration     string    `json:"last_migration"`
	MigratedFrom      string    `json:"migrated_from"`
	MigratedAt        time.Time `json:"migrated_at"`
	SQLiteBackupPath  string    `json:"sqlite_backup_path,omitempty"`
	AutoMigrated      bool      `json:"auto_migrated"`
}

// AutoMigrate automatically migrates from SQLite to JSON if needed
func AutoMigrate(dataDir string) error {
	// Check migration status
	statusPath := filepath.Join(dataDir, ".migration_status")
	status, err := loadMigrationStatus(statusPath)
	if err == nil && status.AutoMigrated {
		// Already migrated, skip
		return nil
	}

	// Check if SQLite database exists
	sqlitePath := filepath.Join(dataDir, "redock.db")
	if _, err := os.Stat(sqlitePath); os.IsNotExist(err) {
		// No SQLite, fresh install
		log.Println("📝 Fresh installation, no migration needed")
		return saveMigrationStatus(statusPath, MigrationStatus{
			Version:      5,
			MigratedFrom: "none",
			MigratedAt:   time.Now(),
			AutoMigrated: true,
		})
	}

	// SQLite exists, perform migration
	log.Println("🔄 SQLite database detected, starting automatic migration...")

	// Backup SQLite first
	backupDir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	timestamp := time.Now().Format("20060102_150405")
	backupPath := filepath.Join(backupDir, fmt.Sprintf("redock_backup_%s.db", timestamp))

	log.Printf("💾 Creating backup: %s", backupPath)
	if err := copyFile(sqlitePath, backupPath); err != nil {
		return fmt.Errorf("failed to backup SQLite: %w", err)
	}

	// Open SQLite connection
	db, err := gorm.Open(sqlite.Dialector{
		DriverName: "sqlite",
		DSN:        sqlitePath + "?_journal_mode=WAL",
	}, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("failed to open SQLite: %w", err)
	}

	// Perform migration
	log.Println("📦 Migrating data from SQLite to JSON...")
	if err := memory.MigrateFromSQLite(db, dataDir); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	// Save migration status
	status = MigrationStatus{
		Version:          5,
		LastMigration:    "005_vpn_server",
		MigratedFrom:     "sqlite",
		MigratedAt:       time.Now(),
		SQLiteBackupPath: backupPath,
		AutoMigrated:     true,
	}

	if err := saveMigrationStatus(statusPath, status); err != nil {
		return fmt.Errorf("failed to save migration status: %w", err)
	}

	log.Println("✅ Migration completed successfully!")
	log.Printf("💾 SQLite backup saved to: %s", backupPath)
	log.Println("ℹ️  You can now safely delete redock.db and redock.db-* files")

	return nil
}

// loadMigrationStatus loads migration status from file
func loadMigrationStatus(path string) (MigrationStatus, error) {
	var status MigrationStatus

	data, err := os.ReadFile(path)
	if err != nil {
		return status, err
	}

	if err := json.Unmarshal(data, &status); err != nil {
		return status, err
	}

	return status, nil
}

// saveMigrationStatus saves migration status to file
func saveMigrationStatus(path string, status MigrationStatus) error {
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	return os.WriteFile(dst, data, 0644)
}

// GetMigrationStatus returns current migration status
func GetMigrationStatus(dataDir string) (*MigrationStatus, error) {
	statusPath := filepath.Join(dataDir, ".migration_status")
	status, err := loadMigrationStatus(statusPath)
	if err != nil {
		return nil, err
	}
	return &status, nil
}
