package database

import (
	"fmt"
	"log"
	"regexp"
	"sort"
	"strconv"
	"time"

	"gorm.io/gorm"
)

// Migration represents a database migration
type Migration struct {
	Version     string
	Description string
	Up          func(*gorm.DB) error
	Down        func(*gorm.DB) error
}

// migrations holds all registered migrations in order
var migrations = []Migration{}

// RegisterMigration registers a new migration
func RegisterMigration(m Migration) {
	migrations = append(migrations, m)
}

// RunMigrations runs all pending migrations
func RunMigrations(db *gorm.DB) error {
	// Ensure migration_histories table exists
	if err := db.AutoMigrate(&MigrationHistory{}); err != nil {
		return fmt.Errorf("failed to create migration_histories table: %w", err)
	}

	// Sort migrations by version (extract numeric prefix for proper ordering)
	sort.Slice(migrations, func(i, j int) bool {
		return compareVersions(migrations[i].Version, migrations[j].Version)
	})

	// Run each migration if not already applied
	for _, migration := range migrations {
		var count int64
		db.Model(&MigrationHistory{}).Where("name = ?", migration.Version).Count(&count)

		if count > 0 {
			continue
		}

		log.Printf("🔄 Running migration: %s - %s", migration.Version, migration.Description)

		// Run migration in transaction
		if err := db.Transaction(func(tx *gorm.DB) error {
			// Execute Up migration
			if err := migration.Up(tx); err != nil {
				return fmt.Errorf("migration %s failed: %w", migration.Version, err)
			}

			// Record migration
			return tx.Create(&MigrationHistory{
				Name:      migration.Version,
				AppliedAt: time.Now(),
			}).Error
		}); err != nil {
			return err
		}

		log.Printf("Migration %s completed successfully", migration.Version)
	}

	return nil
}

// RollbackMigration rolls back the last applied migration
func RollbackMigration(db *gorm.DB) error {
	// Get last applied migration
	var history MigrationHistory
	if err := db.Order("applied_at DESC").First(&history).Error; err != nil {
		return fmt.Errorf("no migrations to rollback: %w", err)
	}

	// Find migration
	var targetMigration *Migration
	for i := range migrations {
		if migrations[i].Version == history.Name {
			targetMigration = &migrations[i]
			break
		}
	}

	if targetMigration == nil {
		return fmt.Errorf("migration %s not found in registry", history.Name)
	}

	if targetMigration.Down == nil {
		return fmt.Errorf("migration %s does not have a Down function", history.Name)
	}

	log.Printf("🔄 Rolling back migration: %s", history.Name)

	// Rollback in transaction
	if err := db.Transaction(func(tx *gorm.DB) error {
		// Execute Down migration
		if err := targetMigration.Down(tx); err != nil {
			return fmt.Errorf("rollback failed: %w", err)
		}

		// Remove migration history
		return tx.Delete(&history).Error
	}); err != nil {
		return err
	}

	log.Printf("Rollback completed successfully")
	return nil
}

// GetAppliedMigrations returns list of applied migrations
func GetAppliedMigrations(db *gorm.DB) ([]MigrationHistory, error) {
	var histories []MigrationHistory
	if err := db.Order("applied_at ASC").Find(&histories).Error; err != nil {
		return nil, err
	}
	return histories, nil
}

// GetPendingMigrations returns list of pending migrations
func GetPendingMigrations(db *gorm.DB) ([]Migration, error) {
	applied, err := GetAppliedMigrations(db)
	if err != nil {
		return nil, err
	}

	appliedMap := make(map[string]bool)
	for _, h := range applied {
		appliedMap[h.Name] = true
	}

	var pending []Migration
	for _, m := range migrations {
		if !appliedMap[m.Version] {
			pending = append(pending, m)
		}
	}

	return pending, nil
}

// compareVersions compares two version strings
// Extracts numeric prefix (e.g., "001" from "001_initial_schema")
// Falls back to string comparison if no numeric prefix found
func compareVersions(v1, v2 string) bool {
	// Extract numeric prefix using regex
	re := regexp.MustCompile(`^(\d+)`)

	match1 := re.FindStringSubmatch(v1)
	match2 := re.FindStringSubmatch(v2)

	// If both have numeric prefix, compare as integers
	if len(match1) > 1 && len(match2) > 1 {
		num1, err1 := strconv.Atoi(match1[1])
		num2, err2 := strconv.Atoi(match2[1])

		if err1 == nil && err2 == nil {
			if num1 != num2 {
				return num1 < num2
			}
			// If numbers are equal, compare full string
			return v1 < v2
		}
	}

	// Fallback to string comparison
	return v1 < v2
}
