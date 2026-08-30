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

	// The mail servers live in this process, so bring them up here — but only
	// for an installation that actually serves mail. Binding 25/143/587 on a
	// machine with no mail domain would just take ports from something else.
	if !manager.ShouldAutoStart() {
		log.Println("📧 Mail server idle: add a domain to start it")
		return
	}

	if err := manager.StartServer(); err != nil {
		log.Printf("⚠️  Failed to start the mail server: %v", err)
		return
	}
	log.Printf("📧 %s", manager.NativeSummaryLine())
}

// GetEmailManager returns the email server manager instance
func GetEmailManager() *EmailManager {
	return manager
}
