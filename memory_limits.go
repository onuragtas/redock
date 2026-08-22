package main

import (
	"fmt"
	"log"

	"redock/platform/memguard"
	"redock/platform/memory"
)

// logTableLimit caps one append-only table. Every row of these tables lives in
// RAM as a Go struct for the whole process lifetime, so without a cap a busy
// resolver or gateway grows the heap until the OS kills the process.
type logTableLimit struct {
	table string
	// maxRows is the steady-state cap.
	maxRows int
	// keepUnderPressure is how many rows survive a memory-guard relief sweep.
	keepUnderPressure int
	// avgRowBytes is a rough per-row cost, used only to report reclaimed bytes.
	avgRowBytes int64
}

// logTables lists the tables that behave like logs: append-only, unbounded, and
// safe to truncate from the oldest end. Everything else (config, users, routes)
// is real state and is never trimmed automatically.
var logTables = []logTableLimit{
	{table: "dns_query_logs", maxRows: 50000, keepUnderPressure: 10000, avgRowBytes: 320},
	{table: "vpn_connection_logs", maxRows: 20000, keepUnderPressure: 5000, avgRowBytes: 256},
	{table: "vpn_bandwidth_stats", maxRows: 20000, keepUnderPressure: 5000, avgRowBytes: 128},
	{table: "email_logs", maxRows: 20000, keepUnderPressure: 5000, avgRowBytes: 256},
	{table: "cloudflare_events", maxRows: 10000, keepUnderPressure: 2000, avgRowBytes: 384},
}

// logTableCap returns the row cap for a table, or 0 when it is not a log table.
// Used at registration time so the cap is enforced while the file is read.
func logTableCap(table string) int {
	for _, limit := range logTables {
		if limit.table == table {
			return limit.maxRows
		}
	}
	return 0
}

// applyLogTableLimits is the safety net for log tables that were registered
// without a cap; tables registered through RegisterWithLimit are already capped.
func applyLogTableLimits(db *memory.Database) {
	for _, limit := range logTables {
		if dropped := memory.SetTableLimit(db, limit.table, limit.maxRows); dropped > 0 {
			log.Printf("memory: %s capped at %d rows, dropped %d old rows", limit.table, limit.maxRows, dropped)
		}
	}
}

// registerMemoryDBReliever lets the memory guard reclaim RAM from the in-memory
// database: log tables are truncated, then the dirty tables are flushed so the
// on-disk copy matches what is left in memory.
func registerMemoryDBReliever(db *memory.Database) {
	memguard.RegisterReliever(memguard.Reliever{
		Name:        "memory-db-log-tables",
		Description: "Truncates append-only tables (DNS/VPN/email logs) held in the in-memory DB",
		MinLevel:    memguard.LevelWarning,
		Priority:    5, // cheapest big win: run before anything else
		Release: func(level memguard.Level) (int64, string) {
			var freed int64
			trimmed := 0
			for _, limit := range logTables {
				keep := limit.keepUnderPressure
				if level >= memguard.LevelEmergency {
					keep = keep / 10
				}
				dropped := memory.TrimTable(db, limit.table, keep)
				if dropped > 0 {
					freed += int64(dropped) * limit.avgRowBytes
					trimmed += dropped
				}
			}
			if trimmed == 0 {
				return 0, ""
			}
			// Persist immediately so the trimmed state survives a restart.
			if err := db.Flush(); err != nil {
				log.Printf("memory: flush after trim failed: %v", err)
			}
			return freed, fmt.Sprintf("%d log rows dropped from the in-memory DB", trimmed)
		},
	})
}
