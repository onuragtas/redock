package migrations

import (
	"log"
	"redock/platform/database"
	"redock/vpn_server"

	"gorm.io/gorm"
)

func init() {
	database.RegisterMigration(database.Migration{
		Version:     "005_vpn_server",
		Description: "Add VPN server tables (WireGuard)",
		Up: func(db *gorm.DB) error {
			// Create VPN tables
			tables := []interface{}{
				&vpn_server.VPNServer{},
				&vpn_server.VPNUser{},
				&vpn_server.VPNConnection{},
				&vpn_server.VPNConnectionLog{},
				&vpn_server.VPNBandwidthStat{},
				&vpn_server.VPNUserGroup{},
				&vpn_server.VPNUserGroupMember{},
				&vpn_server.VPNSecurityRule{},
			}

			for _, table := range tables {
				if err := db.AutoMigrate(table); err != nil {
					log.Printf("⚠️  Failed to create VPN table: %v", err)
					return err
				}
			}

			log.Println("✅ Created VPN server tables")

			// Add composite indexes for performance
			indexes := []string{
				"CREATE INDEX IF NOT EXISTS idx_vpn_user_server ON vpn_users(server_id, enabled)",
				"CREATE INDEX IF NOT EXISTS idx_vpn_connection_user ON vpn_connections(user_id, status)",
				"CREATE INDEX IF NOT EXISTS idx_vpn_connection_server ON vpn_connections(server_id, status)",
				"CREATE INDEX IF NOT EXISTS idx_vpn_connection_log_user ON vpn_connection_logs(user_id, created_at)",
				"CREATE INDEX IF NOT EXISTS idx_vpn_bandwidth_stat ON vpn_bandwidth_stats(user_id, date, hour)",
				"CREATE INDEX IF NOT EXISTS idx_vpn_user_group_member ON vpn_user_group_members(user_id, group_id)",
			}

			for _, indexSQL := range indexes {
				if err := db.Exec(indexSQL).Error; err != nil {
					log.Printf("⚠️  Failed to create index: %v", err)
					// Don't return error, it's not critical
				}
			}

			log.Println("✅ Created VPN server indexes")

			return nil
		},
		Down: func(db *gorm.DB) error {
			// Drop VPN tables in reverse order
			tables := []interface{}{
				&vpn_server.VPNSecurityRule{},
				&vpn_server.VPNUserGroupMember{},
				&vpn_server.VPNUserGroup{},
				&vpn_server.VPNBandwidthStat{},
				&vpn_server.VPNConnectionLog{},
				&vpn_server.VPNConnection{},
				&vpn_server.VPNUser{},
				&vpn_server.VPNServer{},
			}

			for _, table := range tables {
				if err := db.Migrator().DropTable(table); err != nil {
					log.Printf("⚠️  Failed to drop VPN table: %v", err)
					return err
				}
			}

			log.Println("✅ Dropped VPN server tables")
			return nil
		},
	})
}
