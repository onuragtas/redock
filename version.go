package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"redock/selfupdate"
	"runtime"
	"strings"
)

// Update configuration URLs
const (
	// GitHub Pages serves from /redock subpath (repository name)
	UPDATE_CONFIG_URL = "https://onuragtas.github.io/redock/update.json"
	GITHUB_REPO       = "onuragtas/redock"
)

func checkSelfUpdate() {
	// Skip update check if restarting after an update
	skipCheck := os.Getenv("SKIP_UPDATE_CHECK")
	log.Printf("🔍 SKIP_UPDATE_CHECK env: '%s'", skipCheck) // Debug log
	if skipCheck == "1" {
		log.Println("⏭️  Skipping update check (post-restart)")
		return
	}
	
	// Parse current version
	currentVer, err := selfupdate.ParseVersion(version)
	if err != nil {
		log.Printf("⚠️  Failed to parse current version: %v", err)
		return
	}

	// BETA USERS: Check GitHub for latest beta
	if currentVer.IsBeta() {
		checkBetaUpdate(currentVer)
		return
	}

	// STABLE USERS: Check update.json for force update
	config, err := selfupdate.FetchUpdateConfig(UPDATE_CONFIG_URL, GITHUB_REPO)
	if err != nil {
		log.Printf("⚠️  Failed to fetch update config: %v", err)
		return
	}

	minimumRequiredVersion := config.MinimumRequiredVersion
	minimumVer, err := selfupdate.ParseVersion(minimumRequiredVersion)
	if err != nil {
		log.Printf("⚠️  Failed to parse minimum required version: %v", err)
		return
	}
	
	// Check if current version is below minimum required version
	isBelowMinimum := currentVer.Compare(minimumVer) < 0
	
	if isBelowMinimum {
		// FORCE UPDATE - Current version is below minimum required
		log.Println("⚠️  CRITICAL: Your version is below minimum required!")
		log.Printf("⚠️  Current: %s | Minimum Required: %s", version, minimumRequiredVersion)
		
		if config.CriticalUpdate {
			log.Println("🔒 This is a CRITICAL security update!")
		}
		
		if config.ReleaseNotes != "" {
			log.Printf("📝 %s", config.ReleaseNotes)
		}
		
		performForceUpdate("https://github.com/onuragtas/redock/releases/latest/download/redock_" + runtime.GOOS + "_" + runtime.GOARCH)
	}
}

func checkBetaUpdate(currentVer *selfupdate.Version) {
	log.Println("🧪 Beta version detected, checking for updates from GitHub...")
	
	// Get latest beta version from GitHub
	latestBeta, err := selfupdate.GetLatestBetaVersion("onuragtas", "redock")
	if err != nil {
		log.Printf("⚠️  Failed to fetch latest beta: %v", err)
		return
	}

	latestBetaVer, err := selfupdate.ParseVersion(latestBeta)
	if err != nil {
		log.Printf("⚠️  Failed to parse latest beta version: %v", err)
		return
	}

	// Compare versions
	if currentVer.Compare(latestBetaVer) < 0 {
		// New beta available - force update
		log.Println("🆕 New beta version available!")
		log.Printf("⚠️  Current: %s | Latest Beta: %s", currentVer.String(), latestBeta)
		log.Println("🚀 Auto-updating to latest beta...")
		
		// Build download URL for latest beta
		downloadURL := fmt.Sprintf("https://github.com/onuragtas/redock/releases/download/v%s/redock_%s_%s", 
			latestBeta, runtime.GOOS, runtime.GOARCH)
		
		performForceUpdate(downloadURL)
	} else {
		log.Printf("✅ You are on the latest beta: %s", currentVer.String())
	}
}

func performForceUpdate(downloadURL string) {
	log.Println("🚀 FORCE UPDATE starting automatically...")
	
	if getProcessOwner() != "root" {
		log.Fatalln("❌ Please run this command as root user for force update.")
	}

	log.Println("📥 Downloading:", downloadURL)

	var updater = &selfupdate.Updater{
		CurrentVersion: version,
		BinURL:         downloadURL,
		Dir:            "update/",
		CmdName:        "/docker-env",
	}

	if updater != nil {
		log.Println("🔄 Update started, please wait...")
		updater.Update()
		log.Fatalln("✅ Force update complete. Please run the command again.")
	}
}

func getProcessOwner() string {
	stdout, err := exec.Command("whoami").Output()
	if err != nil {
		os.Exit(1)
	}
	owner := string(stdout)
	owner = strings.TrimSpace(owner)
	return owner
}
