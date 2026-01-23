package migrations

import (
	"redock/platform/database"

	"gorm.io/gorm"
)

func init() {
	database.RegisterMigration(database.Migration{
		Version:     "001_initial_schema",
		Description: "Create initial tables (SavedCommand, MigrationHistory)",
		Up: func(db *gorm.DB) error {
			// Create SavedCommand table
			if err := db.AutoMigrate(&database.SavedCommand{}); err != nil {
				return err
			}

			// Create MigrationHistory table
			if err := db.AutoMigrate(&database.MigrationHistory{}); err != nil {
				return err
			}

			return nil
		},
		Down: func(db *gorm.DB) error {
			// Drop tables in reverse order
			if err := db.Migrator().DropTable(&database.MigrationHistory{}); err != nil {
				return err
			}

			if err := db.Migrator().DropTable(&database.SavedCommand{}); err != nil {
				return err
			}

			return nil
		},
	})
}
