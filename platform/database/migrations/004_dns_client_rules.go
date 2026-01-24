package migrations

import (
	"log"
	"redock/dns_server"
	"redock/platform/database"

	"gorm.io/gorm"
)

func init() {
	database.RegisterMigration(database.Migration{
		Version:     "004_dns_client_rules",
		Description: "Add client-specific domain rules and IP ban support",
		Up: func(db *gorm.DB) error {
			// Create DNSClientDomainRule table
			if err := db.AutoMigrate(&dns_server.DNSClientDomainRule{}); err != nil {
				log.Printf("⚠️  Failed to create dns_client_domain_rules table: %v", err)
				return err
			}
			log.Println("✅ Created dns_client_domain_rules table")

			// Add Blocked, BlockReason, BlockedAt columns to DNSClientSettings
			// Note: AutoMigrate will add missing columns automatically
			if err := db.AutoMigrate(&dns_server.DNSClientSettings{}); err != nil {
				log.Printf("⚠️  Failed to update dns_client_settings table: %v", err)
				return err
			}
			log.Println("✅ Updated dns_client_settings table with Blocked fields")

			// Add composite index for client_ip + domain lookup performance
			if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_client_domain ON dns_client_domain_rules(client_ip, domain)").Error; err != nil {
				log.Printf("⚠️  Failed to create composite index: %v", err)
				// Don't return error, it's not critical
			}

			return nil
		},
		Down: func(db *gorm.DB) error {
			// Drop DNSClientDomainRule table
			if err := db.Migrator().DropTable(&dns_server.DNSClientDomainRule{}); err != nil {
				log.Printf("⚠️  Failed to drop dns_client_domain_rules table: %v", err)
				return err
			}
			log.Println("✅ Dropped dns_client_domain_rules table")

			// Remove columns from DNSClientSettings
			// Note: SQLite doesn't support dropping columns easily, so we skip this
			log.Println("ℹ️  Skipped removing Blocked fields from dns_client_settings (SQLite limitation)")

			return nil
		},
	})
}
