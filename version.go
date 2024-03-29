package main

import (
	"encoding/json"
	"github.com/onuragtas/docker-env/selfupdate"
	"github.com/onuragtas/go-requests"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

var version = "1.0.118"

func checkSelfUpdate() {
	var lastRelease selfupdate.LastRelease
	req := requests.Request{BaseUrl: "https://api.github.com/repos/onuragtas/redock/releases/latest"}
	req.Get()

	json.Unmarshal(req.GetBody(), &lastRelease)
	var lastReleaseName = strings.Replace(lastRelease.Name, "v", "", 1)
	log.Println("Current version:", version, "Latest version:", lastReleaseName)
	if version != lastReleaseName {
		if getProcessOwner() != "root" {
			log.Fatalln("Please run this command as root user.")
		}

		log.Println("https://github.com/onuragtas/redock/releases/latest/download/redock_"+runtime.GOOS+"_"+runtime.GOARCH, "downloading...")

		var updater = &selfupdate.Updater{
			CurrentVersion: version,
			BinURL:         "https://github.com/onuragtas/redock/releases/latest/download/redock_" + runtime.GOOS + "_" + runtime.GOARCH,
			Dir:            "update/",
			CmdName:        "/docker-env",
		}

		if updater != nil {
			log.Println("update: started, please wait...")
			updater.Update()
			log.Fatalln("Update complete please run again command.")
		}
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
