package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/kardianos/service"
)

type program struct{}

func (p *program) Start(s service.Service) error {
	go p.run()
	return nil
}

func (p *program) Stop(s service.Service) error {
	log.Println("Service is stopping...")
	return nil
}

func (p *program) run() {
	log.Println("Service has been started.")
	initialize()
	app()
}

func main() {
	action := flag.String("action", "", "Use this flag to perform an action on the service. [install|start|stop|uninstall]")
	flag.Parse()

	svcConfig := &service.Config{
		Name:        "redock",
		DisplayName: "Redock",
		Description: "Redock Service",
		EnvVars:     map[string]string{"PATH": "/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"},
		Option: map[string]interface{}{
			"OnFailure": "restart",
		},
	}

	if getProcessOwner() == "root" {
		svcConfig.UserName = "root"
		svcConfig.WorkingDirectory = "/root"
	}

	prg := &program{}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatalf("Service couldn't create: %v", err)
	}

	logger, err := s.Logger(nil)
	if err != nil {
		log.Fatalf("Logger couldn't create: %v", err)
	}

	if *action != "" {
		handleAction(*action, s, logger)
		return
	}

	err = s.Run()
	if err != nil {
		log.Fatalf("Service couldn't start: %v", err)
	}
}

func handleAction(action string, s service.Service, logger service.Logger) {
	var err error

	switch action {
	case "install":
		err = s.Install()
		printResult("Service has been installed", "The service couldn't be installed.", err, logger)
	case "start":
		err = s.Start()
		printResult("Service has been started", "The service couldn't be started.", err, logger)
	case "stop":
		err = s.Stop()
		printResult("Service has been stopped", "The service couldn't be stopped", err, logger)
	case "uninstall":
		err = s.Uninstall()
		printResult("Service has been uninstalled", "The service couldn't be uninstalled", err, logger)
	default:
		fmt.Println("Geçersiz işlem. Kullanım: ./app --action [install|start|stop|uninstall]")
		os.Exit(1)
	}
}

func printResult(successMsg, errorMsg string, err error, logger service.Logger) {
	if err != nil {
		logger.Errorf("%s: %v", errorMsg, err)
		fmt.Printf("%s: %v\n", errorMsg, err)
	} else {
		logger.Info(successMsg)
		fmt.Println(successMsg)
	}
}
