package traffic_inspector

import (
	"fmt"
	"log"
	"sync"
	"time"

	"redock/platform/memory"
	"redock/vpn_server"
)

var manager *Manager

// Manager ties together the CA, capture hub, and per-server interception
// listeners.
type Manager struct {
	CA  *CAManager
	Hub *Hub
	db  *memory.Database

	mu      sync.Mutex
	servers map[uint]*serverInterceptor
}

// Init loads/creates the root CA, starts the capture hub, and registers
// vpn_server's NAT hooks so per-server MITM listeners and per-user redirect
// rules are wired automatically as VPN servers start/stop. Called from
// init.go after vpn_server.Init().
func Init(db *memory.Database) {
	ca, err := LoadOrCreateCAManager(db)
	if err != nil {
		log.Printf("⚠️  Failed to initialize traffic inspector CA: %v", err)
		return
	}

	manager = &Manager{
		CA:      ca,
		Hub:     NewHub(),
		db:      db,
		servers: make(map[uint]*serverInterceptor),
	}

	vpn_server.OnNATConfigured = manager.onServerNATConfigured
	vpn_server.OnNATCleanup = manager.onServerNATCleanup

	log.Println("Traffic Inspector initialized")
}

// GetManager returns the traffic inspector manager instance (nil if Init
// failed or hasn't run).
func GetManager() *Manager {
	return manager
}

// logWarn logs a warning to the console (as before) and, if the traffic
// inspector is initialized, also publishes it as a LogEvent over
// /ws/traffic — so the dashboard's Live Traffic tab can surface handshake
// failures, natlookup misses, etc. in its own panel instead of requiring a
// server console to be visible.
func logWarn(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	log.Printf("⚠️  %s", msg)

	if manager != nil && manager.Hub != nil {
		manager.Hub.PublishLog(LogEvent{
			Timestamp: time.Now().Unix(),
			Level:     "warning",
			Message:   msg,
		})
	}
}
