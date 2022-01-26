package main

import (
	dockermanager "github.com/onuragtas/docker-env/docker-manager"
	git "gopkg.in/src-d/go-git.v4"
	"log"
	"os"
)

var processesMap map[string]func()
var processes []string

var answers []string

var dockerRepo = "https://github.com/onuragtas/docker"

var dockerEnvironmentManager dockermanager.DockerEnvironmentManager

func init() {
	setupProcesses()

	_, err := git.PlainClone(getHomeDir()+"/.docker-environment", false, &git.CloneOptions{
		URL:      dockerRepo,
		Progress: os.Stdout,
	})
	if err.Error() != git.ErrRepositoryAlreadyExists.Error() {
		panic(err)
	}

	r, err := git.PlainOpen(getHomeDir() + "/.docker-environment")
	if err != nil {
		log.Print(err)
	}

	w, err := r.Worktree()
	if err != nil {
		log.Print(err)
	}

	err = w.Pull(&git.PullOptions{RemoteName: "origin", Progress: os.Stdout})
	if err != nil {
		log.Print(err)
	}

	dockerEnvironmentManager = dockermanager.DockerEnvironmentManager{
		File:               getHomeDir() + "/.docker-environment/docker-compose.yml.dist",
		ComposeFilePath:    getHomeDir() + "/.docker-environment/docker-compose.yml",
		EnvDistPath:        getHomeDir() + "/.docker-environment/.env.example",
		EnvPath:            getHomeDir() + "/.docker-environment/.env",
		InstallPath:        getHomeDir() + "/.docker-environment/install.sh",
		AddVirtualHostPath: getHomeDir() + "/.docker-environment/add_virtualhost.sh",
		HttpdConfPath:      getHomeDir() + "/.docker-environment/httpd/sites-enabled",
		NginxConfPath:      getHomeDir() + "/.docker-environment/etc/nginx",
	}
	dockerEnvironmentManager.Init()
}

func setupProcesses() {
	processesMap = make(map[string]func())
	processesMap["Setup Environment"] = setupEnv
	processesMap["Install Development Environment"] = installDevelopmentEnvironment
	processesMap["Edit Compose Yaml"] = editComposeYaml
	processesMap["Add Virtual Host"] = addVirtualHost
	processesMap["Edit Virtual Hosts"] = editVirtualHost
	processesMap["Quit"] = func() {
		os.Exit(1)
	}

	for process := range processesMap {
		processes = append(processes, process)
	}
}

func getHomeDir() string {
	dirname, _ := os.UserHomeDir()
	return dirname
}
