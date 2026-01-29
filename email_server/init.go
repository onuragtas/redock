package email_server

import (
	"log"
	"path/filepath"
	dockermanager "redock/docker-manager"
	"redock/platform/database"
)

var manager *EmailManager

// Init initializes email server module
func Init(dockerManager *dockermanager.DockerEnvironmentManager) {
	db := database.GetMemoryDB()
	
	manager = GetManager()
	
	dataPath := filepath.Join(dockerManager.GetWorkDir(), "data")
	if err := manager.Init(db, dataPath); err != nil {
		log.Printf("⚠️  Failed to initialize email server manager: %v", err)
		return
	}
}

// GetEmailManager returns the email server manager instance
func GetEmailManager() *EmailManager {
	return manager
}
