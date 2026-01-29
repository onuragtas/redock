package cloudflare

import (
	"log"
	"redock/platform/database"
)

var manager *CloudflareManager

// Init initializes Cloudflare module
func Init() {
	db := database.GetMemoryDB()
	
	manager = GetManager()
	
	if err := manager.Init(db); err != nil {
		log.Printf("⚠️  Failed to initialize Cloudflare manager: %v", err)
		return
	}
}

// GetCloudflareManager returns the Cloudflare manager instance
func GetCloudflareManager() *CloudflareManager {
	return manager
}
