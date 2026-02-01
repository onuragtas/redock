package migrations

import (
	"redock/app/models"
	"redock/platform/database"
	"redock/platform/memory"
)

// MemoryMigrations returns the list of memory DB migrations (run once in version order).
func MemoryMigrations() []database.MemoryMigration {
	return []database.MemoryMigration{
		{
			Version: 1,
			Name:    "cleanup_legacy_users",
			Up: func(db *memory.Database) error {
				// Bir kerelik: eski createAdmin ile oluşturulmuş user'ları siler.
				users := memory.FindAll[*models.User](db, "users")
				for _, u := range users {
					if err := memory.Delete[*models.User](db, "users", u.GetID()); err != nil {
						return err
					}
				}
				return nil
			},
		},
		{
			Version: 2,
			Name:    "cleanup_legacy_users2",
			Up: func(db *memory.Database) error {
				// Bir kerelik: eski createAdmin ile oluşturulmuş user'ları siler.
				users := memory.FindAll[*models.User](db, "users")
				for _, u := range users {
					if err := memory.Delete[*models.User](db, "users", u.GetID()); err != nil {
						return err
					}
				}
				return nil
			},
		},
	}
}
