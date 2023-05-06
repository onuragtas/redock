package main

import (
	"os"
	"strings"

	dockermanager "github.com/onuragtas/docker-env/docker-manager"
)

type Process struct {
	Name string
	Func func()
}

var processMapList []Process

var processes []string

var answers []string

var dockerRepo = "https://github.com/onuragtas/docker"

var dockerEnvironmentManager dockermanager.DockerEnvironmentManager

var devEnv bool

func init() {

	if len(os.Args) > 1 && os.Args[1] == "--devenv" {
		devEnv = true
	}

	setupProcesses()

	go dockerUpdate()

	dockerEnvironmentManager = dockermanager.DockerEnvironmentManager{
		File:               getHomeDir() + "/.docker-environment/docker-compose.yml.{.arch}.dist",
		ComposeFilePath:    getHomeDir() + "/.docker-environment/docker-compose.yml",
		EnvDistPath:        getHomeDir() + "/.docker-environment/.env.example",
		EnvPath:            getHomeDir() + "/.docker-environment/.env",
		InstallPath:        getHomeDir() + "/.docker-environment/install.sh",
		AddVirtualHostPath: getHomeDir() + "/.docker-environment/add_virtualhost.sh",
		HttpdConfPath:      getHomeDir() + "/.docker-environment/httpd/sites-enabled",
		NginxConfPath:      getHomeDir() + "/.docker-environment/etc/nginx",
		DevEnv:             devEnv,
	}

	if devEnv {
		byteArray, _ := os.ReadFile("/root/.username")
		dockerEnvironmentManager.Username = strings.TrimSpace(string(byteArray))
		dockerEnvironmentManager.HttpdConfPath = "/usr/local/httpd"
		dockerEnvironmentManager.NginxConfPath = "/usr/local/nginx"
	}

	go dockerEnvironmentManager.Init()
	if !devEnv {
		go dockerEnvironmentManager.CheckLocalIpAndRegenerate()
	}
}

func setupProcesses() {
	if !devEnv {
		processMapList = append(processMapList, Process{Name: "Exec Bash Service", Func: execBashService})
		processMapList = append(processMapList, Process{Name: "Setup Environment", Func: setupEnv})
		processMapList = append(processMapList, Process{Name: "Regenerate XDebug Configuration", Func: regenerateXDebugConf})
		processMapList = append(processMapList, Process{Name: "Add XDebug", Func: addXDebug})
		processMapList = append(processMapList, Process{Name: "Remove XDebug", Func: removeXDebug})
		processMapList = append(processMapList, Process{Name: "Install Development Environment", Func: installDevelopmentEnvironment})
	}
	processMapList = append(processMapList, Process{Name: "Restart Nginx/Httpd", Func: restartServices})
	processMapList = append(processMapList, Process{Name: "Add Virtual Host", Func: addVirtualHost})
	processMapList = append(processMapList, Process{Name: "Edit Virtual Hosts", Func: editVirtualHost})
	if !devEnv {
		processMapList = append(processMapList, Process{Name: "Edit Compose Yaml", Func: editComposeYaml})
		processMapList = append(processMapList, Process{Name: "Import Nginx/Apache2 Sites From Other Docker Project", Func: importVirtualHosts})
	}
	processMapList = append(processMapList, Process{Name: "Self-Update", Func: selfUpdate})
	processMapList = append(processMapList, Process{Name: "Update Docker", Func: dockerUpdate})
	processMapList = append(processMapList, Process{Name: "Update Docker Images", Func: dockerImageUpdate})
	// processMapList = append(processMapList, Process{Name: "TCP Forward", Func: TcpForward})
	processMapList = append(processMapList, Process{Name: "Quit", Func: func() {
		os.Exit(1)
	}})

	for _, process := range processMapList {
		processes = append(processes, process.Name)
	}
}

func getHomeDir() string {
	dirname, _ := os.UserHomeDir()
	return dirname
}
