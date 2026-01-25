package vpn_server

import (
	"log"
	"redock/platform/database"
)

var vpnManager *WireGuardManager

// Init initializes VPN server module
func Init() {
	// Get database connection
	db := database.GetDB()

	// Get VPN manager instance
	vpnManager = GetWireGuardManager()

	// Initialize with database
	if err := vpnManager.Init(db); err != nil {
		log.Printf("⚠️  Failed to initialize VPN server: %v", err)
		return
	}
}

// GetManager returns the VPN manager instance
func GetManager() *WireGuardManager {
	return vpnManager
}
