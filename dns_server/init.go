package dns_server

import (
	"log"
	dockermanager "redock/docker-manager"
	"redock/platform/database"
)

var dnsServer *DNSServer

// Init initializes DNS server module
func Init(dockerManager *dockermanager.DockerEnvironmentManager) {

	// Get database connection
	db := database.GetDB()

	// Get DNS server instance
	dnsServer = GetDNSServer()

	// Initialize with database
	if err := dnsServer.Init(db); err != nil {
		log.Printf("⚠️  Failed to initialize DNS server: %v", err)
		return
	}

	// Auto-start if enabled in config
	config := dnsServer.GetConfig()
	if config != nil && config.Enabled {
		if err := dnsServer.Start(); err != nil {
			log.Printf("⚠️  Failed to auto-start DNS server: %v", err)
		}
	}

}

// GetServer returns the DNS server instance
func GetServer() *DNSServer {
	return dnsServer
}
