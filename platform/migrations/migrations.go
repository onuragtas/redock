package migrations

import (
	"encoding/json"
	"os"
	"path/filepath"

	"redock/app/models"
	localproxy "redock/local_proxy"
	"redock/platform/database"
	"redock/platform/memory"
)

// MemoryMigrations returns the list of memory DB migrations (run once in version order).
func MemoryMigrations() []database.MemoryMigration {
	return []database.MemoryMigration{
		{
			Version: 1,
			Name:    "cleanup_legacy_users",
			Up: func(db *memory.Database, _ string) error {
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
			Up: func(db *memory.Database, _ string) error {
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
			Version: 3,
			Name:    "import_local_proxy_json",
			Up: func(db *memory.Database, dataDir string) error {
				path := filepath.Join(dataDir, "local_proxy.json")
				data, err := os.ReadFile(path)
				if err != nil {
					if os.IsNotExist(err) {
						return nil
					}
					return err
				}
				var list []localproxy.Item
				if err := json.Unmarshal(data, &list); err != nil {
					return err
				}
				for _, item := range list {
					entity := &localproxy.LocalProxyItem{
						Name:       item.Name,
						LocalPort:  item.LocalPort,
						Host:       item.Host,
						RemotePort: item.RemotePort,
						Timeout:    item.Timeout,
						Started:    item.Started,
					}
					if err := memory.Create(db, "local_proxy_items", entity); err != nil {
						return err
					}
				}
				return nil
			},
		},
	}
}
