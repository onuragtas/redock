package database

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
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

// AutoMigrate checks migration status (SQLite migration removed - use JSON only)
func AutoMigrate(dataDir string) error {
	// On a fresh install the data directory does not exist yet; the status file
	// is written here, before the memory DB gets a chance to create it.
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory %s: %w", dataDir, err)
	}

	// Check migration status
	statusPath := filepath.Join(dataDir, ".migration_status")
	status, err := loadMigrationStatus(statusPath)
	if err == nil && status.AutoMigrated {
		// Already migrated, skip
		return nil
	}

	// Check if SQLite database exists (legacy)
	sqlitePath := filepath.Join(dataDir, "redock.db")
	if _, err := os.Stat(sqlitePath); err == nil {
		// SQLite exists but we don't support automatic migration anymore
		log.Println("⚠️  Old SQLite database detected!")
		log.Println("⚠️  SQLite is no longer supported. The system will use JSON storage.")
		log.Println("⚠️  Your old SQLite data will NOT be migrated automatically.")
		log.Println("💡 To preserve old data, please backup redock.db manually before proceeding.")
		log.Println("💡 The system will start fresh with empty JSON files.")
		
		// Wait a bit so user can see the warning
		time.Sleep(3 * time.Second)
	}

	// Mark as migrated (fresh start with JSON)
	log.Println("📝 Initializing JSON storage...")
	status = MigrationStatus{
		Version:      5,
		MigratedFrom: "none",
		MigratedAt:   time.Now(),
		AutoMigrated: true,
	}

	if err := saveMigrationStatus(statusPath, status); err != nil {
		return fmt.Errorf("failed to save migration status: %w", err)
	}

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

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
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
