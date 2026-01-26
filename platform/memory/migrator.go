package memory

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"gorm.io/gorm"
)

// MigrationMetadata holds migration information
type MigrationMetadata struct {
	Version        int       `json:"version"`
	LastMigration  string    `json:"last_migration"`
	MigratedFrom   string    `json:"migrated_from"`
	MigratedAt     time.Time `json:"migrated_at"`
}

// MigrateFromSQLite migrates data from SQLite to JSON
func MigrateFromSQLite(sqliteDB *gorm.DB, jsonDir string) error {
	log.Println("🔄 Starting SQLite → JSON migration...")

	if err := os.MkdirAll(jsonDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Check if already migrated
	if checkAlreadyMigrated(jsonDir) {
		log.Println("⚠️  JSON files already exist. Skipping migration.")
		return nil
	}

	// Get migration version from SQLite
	var migrationHistory struct {
		Version string
	}
	sqliteDB.Table("migration_histories").
		Order("id DESC").
		Limit(1).
		Select("version").
		Scan(&migrationHistory)

	metadata := MigrationMetadata{
		Version:       5,
		LastMigration: migrationHistory.Version,
		MigratedFrom:  "sqlite",
		MigratedAt:    time.Now(),
	}

	// Migrate each table
	tables := []struct {
		name   string
		model  interface{}
		output string
	}{
		// DNS tables
		{"dns_config", &map[string]interface{}{}, "dns_config.json"},
		{"dns_blocklists", &[]interface{}{}, "dns_blocklists.json"},
		{"dns_custom_filters", &[]interface{}{}, "dns_custom_filters.json"},
		{"dns_client_settings", &[]interface{}{}, "dns_client_settings.json"},
		{"dns_client_domain_rules", &[]interface{}{}, "dns_client_rules.json"},
		{"dns_rewrites", &[]interface{}{}, "dns_rewrites.json"},

		// VPN tables
		{"vpn_servers", &[]interface{}{}, "vpn_servers.json"},
		{"vpn_users", &[]interface{}{}, "vpn_users.json"},
		{"vpn_user_groups", &[]interface{}{}, "vpn_groups.json"},
		{"vpn_security_rules", &[]interface{}{}, "vpn_security_rules.json"},

		// Other tables
		{"saved_commands", &[]interface{}{}, "saved_commands.json"},
	}

	for _, table := range tables {
		if err := migrateTable(sqliteDB, jsonDir, table.name, table.output, metadata); err != nil {
			log.Printf("⚠️  Failed to migrate table %s: %v", table.name, err)
			// Continue with other tables
		} else {
			log.Printf("✅ Migrated table: %s → %s", table.name, table.output)
		}
	}

	// Migrate DNS logs to JSONL
	if err := migrateDNSLogs(sqliteDB, jsonDir); err != nil {
		log.Printf("⚠️  Failed to migrate DNS logs: %v", err)
	} else {
		log.Println("✅ Migrated DNS logs to JSONL")
	}

	log.Println("✅ Migration completed!")
	return nil
}

// migrateTable migrates a single table
func migrateTable(db *gorm.DB, baseDir, tableName, outputFile string, metadata MigrationMetadata) error {
	// Check if table exists
	if !db.Migrator().HasTable(tableName) {
		return nil // Table doesn't exist, skip
	}

	// Get all rows as generic maps
	var rows []map[string]interface{}
	if err := db.Table(tableName).Find(&rows).Error; err != nil {
		return err
	}

	if len(rows) == 0 {
		// Empty table, create empty JSON with metadata
		rows = []map[string]interface{}{}
	}

	// Fix SQLite integer booleans in all rows
	for _, row := range rows {
		normalizeSQLiteBooleans(row)
	}

	// Wrap with metadata
	wrapper := struct {
		Meta MigrationMetadata      `json:"_meta"`
		Data []map[string]interface{} `json:"data"`
	}{
		Meta: metadata,
		Data: rows,
	}

	// Write to JSON file
	path := filepath.Join(baseDir, outputFile)
	data, err := json.MarshalIndent(wrapper, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// migrateDNSLogs migrates DNS query logs to JSONL format
func migrateDNSLogs(db *gorm.DB, baseDir string) error {
	if !db.Migrator().HasTable("dns_query_logs") {
		return nil
	}

	// Get logs in batches (to avoid memory issues)
	const batchSize = 1000
	offset := 0

	// Create JSONL file for today
	date := time.Now().Format("2006-01-02")
	filename := fmt.Sprintf("dns_logs_%s.jsonl", date)
	path := filepath.Join(baseDir, filename)

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	for {
		var logs []map[string]interface{}
		if err := db.Table("dns_query_logs").
			Limit(batchSize).
			Offset(offset).
			Find(&logs).Error; err != nil {
			return err
		}

		if len(logs) == 0 {
			break
		}

		// Write each log as JSON line
		for _, logEntry := range logs {
			data, err := json.Marshal(logEntry)
			if err != nil {
				continue
			}
			file.Write(append(data, '\n'))
		}

		offset += batchSize
	}

	return nil
}

// checkAlreadyMigrated checks if JSON files already exist
func checkAlreadyMigrated(baseDir string) bool {
	// Check for key files
	files := []string{
		"vpn_servers.json",
		"dns_blocklists.json",
	}

	for _, file := range files {
		path := filepath.Join(baseDir, file)
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}

	return false
}

// BackupSQLite creates a backup of SQLite database before migration
func BackupSQLite(sqlitePath, backupDir string) error {
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return err
	}

	timestamp := time.Now().Format("20060102_150405")
	backupPath := filepath.Join(backupDir, fmt.Sprintf("redock_backup_%s.db", timestamp))

	// Copy file
	input, err := os.ReadFile(sqlitePath)
	if err != nil {
		return err
	}

	if err := os.WriteFile(backupPath, input, 0644); err != nil {
		return err
	}

	log.Printf("✅ SQLite backup created: %s", backupPath)
	return nil
}
