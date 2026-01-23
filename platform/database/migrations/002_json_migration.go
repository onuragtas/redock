package migrations

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	docker_manager "redock/docker-manager"
	"redock/platform/database"
	"time"

	"gorm.io/gorm"
)

func init() {
	database.RegisterMigration(database.Migration{
		Version:     "002_json_migration",
		Description: "Migrate saved commands from JSON to SQLite",
		Up: func(db *gorm.DB) error {
			// This will be set during runtime
			workDir := docker_manager.GetDockerManager().GetWorkDir()
			jsonPath := filepath.Join(workDir, "data", "saved_commands.json")

			// Check if JSON file exists
			if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
				log.Println("ℹ️  No saved_commands.json found, skipping migration", jsonPath)
				return nil
			}

			// Read JSON file
			data, err := os.ReadFile(jsonPath)
			if err != nil {
				return fmt.Errorf("failed to read saved_commands.json: %w", err)
			}

			// Parse JSON
			type OldSavedCommand struct {
				Command string `json:"command"`
				Path    string `json:"path"`
			}
			var oldCommands []OldSavedCommand
			if err := json.Unmarshal(data, &oldCommands); err != nil {
				return fmt.Errorf("failed to parse saved_commands.json: %w", err)
			}

			// Migrate to SQLite
			for _, old := range oldCommands {
				newCmd := database.SavedCommand{
					Command: old.Command,
					Path:    old.Path,
				}
				if err := db.Create(&newCmd).Error; err != nil {
					log.Printf("⚠️  Failed to migrate command '%s': %v", old.Command, err)
				}
			}

			// Backup old JSON file
			backupDir := filepath.Join(workDir, "data", "json_backup")
			if err := os.MkdirAll(backupDir, 0755); err != nil {
				return fmt.Errorf("failed to create backup directory: %w", err)
			}

			backupPath := filepath.Join(backupDir, fmt.Sprintf("saved_commands_%s.json", time.Now().Format("20060102_150405")))
			if err := os.Rename(jsonPath, backupPath); err != nil {
				log.Printf("⚠️  Failed to backup saved_commands.json: %v", err)
			} else {
				log.Printf("✅ Backed up saved_commands.json to: %s", backupPath)
			}

			log.Printf("✅ Migrated %d saved commands from JSON to SQLite", len(oldCommands))
			return nil
		},
		Down: func(db *gorm.DB) error {
			// Clear saved commands (cannot restore from backup automatically)
			log.Println("⚠️  Rolling back: Clearing saved_commands table")
			return db.Exec("DELETE FROM saved_commands").Error
		},
	})
}
