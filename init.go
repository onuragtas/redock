package main

import (
	"log"
	"os"
	"redock/api_gateway"
	"redock/deployment"
	"redock/devenv"
	localproxy "redock/local_proxy"
	"redock/php_debug_adapter"
	"redock/saved_commands"
	"redock/tunnel_proxy"
	"time"

	dockermanager "redock/docker-manager"
)

type Process struct {
	Name string
	Func func()
}

var devEnv bool

func initialize() {
	checkSelfUpdate()

	go func() {
		for range time.Tick(time.Minute * 5) {
			checkSelfUpdate()
		}
	}()

	if len(os.Args) > 1 && os.Args[1] == "--devenv" {
		devEnv = true
	}

	log.Println("initialize....")
	dockerEnvironmentManager := dockermanager.GetDockerManager()

	go dockerEnvironmentManager.UpdateDocker()

	dockerEnvironmentManager.Init()
	if !devEnv {
		go dockerEnvironmentManager.CheckLocalIpAndRegenerate()
	}
	devenv.Init(dockerEnvironmentManager)
	tunnel_proxy.Init(dockerEnvironmentManager)
	localproxy.Init(dockerEnvironmentManager)
	php_debug_adapter.Init(dockerEnvironmentManager)
	saved_commands.Init(dockerEnvironmentManager)
	deployment.Init(dockerEnvironmentManager)
	api_gateway.Init(dockerEnvironmentManager)
	go deployment.GetDeployment().Run()
	localproxy.GetLocalProxyManager().StartAll()
	api_gateway.GetGateway().StartAll()
}
