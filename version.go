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

const (
	UPDATE_CONFIG_URL = "https://onuragtas.github.io/redock/update.json"
	GITHUB_REPO       = "onuragtas/redock"
)

func checkSelfUpdate() {
	if os.Getenv("SKIP_UPDATE_CHECK") == "1" {
		return
	}

	currentVer, err := selfupdate.ParseVersion(version)
	if err != nil {
		log.Printf("⚠️  Failed to parse current version: %v", err)
		return
	}

	if currentVer.IsBeta() {
		checkBetaUpdate(currentVer)
		return
	}

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

	isBelowMinimum := currentVer.Compare(minimumVer) < 0

	if isBelowMinimum {
		log.Printf("⚠️  CRITICAL: Version below minimum required! Current: %s | Required: %s", version, minimumRequiredVersion)
		performForceUpdate("https://github.com/onuragtas/redock/releases/latest/download/redock_" + runtime.GOOS + "_" + runtime.GOARCH)
	}
}

func checkBetaUpdate(currentVer *selfupdate.Version) {
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

	if currentVer.Compare(latestBetaVer) < 0 {
		downloadURL := fmt.Sprintf("https://github.com/onuragtas/redock/releases/download/v%s/redock_%s_%s", 
			latestBeta, runtime.GOOS, runtime.GOARCH)
		
		performForceUpdate(downloadURL)
	}
}

func performForceUpdate(downloadURL string) {
	if getProcessOwner() != "root" {
		log.Fatalln("❌ Please run this command as root user for force update.")
	}

	updater := &selfupdate.Updater{
		CurrentVersion: version,
		BinURL:         downloadURL,
		Dir:            "update/",
		CmdName:        "redock",
	}

	if err := updater.UpdateWithRestart(); err != nil {
		log.Fatalf("❌ Force update failed: %v", err)
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
