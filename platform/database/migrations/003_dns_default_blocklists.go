package migrations

import (
	"log"
	"redock/dns_server"
	"redock/platform/database"

	"gorm.io/gorm"
)

func init() {
	database.RegisterMigration(database.Migration{
		Version:     "003_dns_default_blocklists",
		Description: "Add default AdGuard DNS blocklists",
		Up: func(db *gorm.DB) error {
			// Define default blocklists
			defaultBlocklists := []dns_server.DNSBlocklist{
				{
					Name:           "AdGuard DNS filter",
					URL:            "https://adguardteam.github.io/HostlistsRegistry/assets/filter_1.txt",
					Enabled:        true,
					Format:         "adblock",
					UpdateInterval: 86400, // 24 hours
					DomainCount:    0,
				},
				{
					Name:           "AdAway Default Blocklist",
					URL:            "https://adguardteam.github.io/HostlistsRegistry/assets/filter_2.txt",
					Enabled:        true,
					Format:         "hosts",
					UpdateInterval: 86400, // 24 hours
					DomainCount:    0,
				},
			}

			// Insert blocklists if they don't exist
			for _, blocklist := range defaultBlocklists {
				var existing dns_server.DNSBlocklist
				result := db.Where("name = ?", blocklist.Name).First(&existing)

				if result.Error == gorm.ErrRecordNotFound {
					if err := db.Create(&blocklist).Error; err != nil {
						log.Printf("⚠️  Failed to create blocklist '%s': %v", blocklist.Name, err)
						return err
					}
					log.Printf("✅ Created default blocklist: %s", blocklist.Name)
				} else {
					log.Printf("ℹ️  Blocklist '%s' already exists, skipping", blocklist.Name)
				}
			}

			return nil
		},
		Down: func(db *gorm.DB) error {
			// Remove default blocklists
			blocklistNames := []string{
				"AdGuard DNS filter",
				"AdAway Default Blocklist",
			}

			for _, name := range blocklistNames {
				if err := db.Where("name = ?", name).Delete(&dns_server.DNSBlocklist{}).Error; err != nil {
					log.Printf("⚠️  Failed to delete blocklist '%s': %v", name, err)
				} else {
					log.Printf("✅ Deleted default blocklist: %s", name)
				}
			}

			return nil
		},
	})
}
