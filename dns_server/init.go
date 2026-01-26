package dns_server

import (
	"log"
	dockermanager "redock/docker-manager"
	"redock/platform/database"
)

var dnsServer *DNSServer

// Init initializes DNS server module
func Init(dockerManager *dockermanager.DockerEnvironmentManager) {

	// Get memory database connection
	db := database.GetMemoryDB()

	// Get DNS server instance
	dnsServer = GetDNSServer()

	// Initialize with database and docker manager
	if err := dnsServer.Init(db, dockerManager); err != nil {
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
