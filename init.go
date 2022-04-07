package main

import (
	"log"
	"os"

	dockermanager "github.com/onuragtas/docker-env/docker-manager"
	git "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
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
	head, err := r.Head()
	if err != nil {
		log.Print(err)
	}

	commit := plumbing.NewHash(head.Hash().String())

	err = w.Reset(&git.ResetOptions{
		Mode:   git.HardReset,
		Commit: commit,
	})
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
	go dockerEnvironmentManager.Init()
}

func setupProcesses() {
	processMapList = append(processMapList, Process{Name: "Exec Bash Service", Func: execBashService})
	processMapList = append(processMapList, Process{Name: "Setup Environment", Func: setupEnv})
	processMapList = append(processMapList, Process{Name: "Regenerate XDebug Configuration", Func: regenerateXDebugConf})
	processMapList = append(processMapList, Process{Name: "Install Development Environment", Func: installDevelopmentEnvironment})
	processMapList = append(processMapList, Process{Name: "Restart Nginx/Httpd", Func: restartServices})
	processMapList = append(processMapList, Process{Name: "Add Virtual Host", Func: addVirtualHost})
	processMapList = append(processMapList, Process{Name: "Edit Virtual Hosts", Func: editVirtualHost})
	processMapList = append(processMapList, Process{Name: "Edit Compose Yaml", Func: editComposeYaml})
	processMapList = append(processMapList, Process{Name: "Import Nginx/Apache2 Sites From Other Docker Project", Func: importVirtualHosts})
	processMapList = append(processMapList, Process{Name: "Self-Update", Func: selfUpdate})
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
