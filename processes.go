package main

import (
	"fmt"
	"github.com/AlecAivazis/survey/v2"
	"github.com/kardianos/osext"
	"github.com/onuragtas/docker-env/command"
	"github.com/onuragtas/docker-env/selfupdate"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
)

func setupEnv() {
	c := command.Command{}
	c.RunWithPipe("nano", dockerEnvironmentManager.EnvDistPath)
}

func editComposeYaml() {
	c := command.Command{}
	c.RunWithPipe("nano", dockerEnvironmentManager.ComposeFilePath)
}

func addVirtualHost() {
	var service string
	var domain string
	var folder string
	var phpService string

	service = pickService()

	inputBox := &survey.Input{Message: "Domain:"}
	err := survey.AskOne(inputBox, &domain)
	if err != nil {
		log.Println(err)
	}

	inputBox = &survey.Input{Message: "Folder:"}
	err = survey.AskOne(inputBox, &folder)
	if err != nil {
		log.Println(err)
	}

	selectBox := &survey.Select{Message: "Pick your service", Options: []string{"php56", "php70", "php71", "php72", "php74", "php56_xdebug", "php72_xdebug", "php74_xdebug"}}
	err = survey.AskOne(selectBox, &phpService)
	if err != nil {
		log.Println(err)
	}

	fmt.Println(domain)
	dockerEnvironmentManager.AddVirtualHost(service, domain, folder, phpService)
}

func editVirtualHost() {
	var service string
	var domain string
	var domains []string

	service = pickService()

	domains = dockerEnvironmentManager.GetDomains(dockerEnvironmentManager.Virtualhost.GetConfigPath(service))
	domains = append(domains, "Quit")

	selectBox := &survey.Select{Message: "Pick your domain", Options: domains, PageSize: 50}
	err := survey.AskOne(selectBox, &domain)
	if err != nil {
		log.Println(err)
	}

	if domain == "Quit" {
		return
	}

	c := command.Command{}
	if service == "nginx" {
		c.RunWithPipe("nano", dockerEnvironmentManager.NginxConfPath+"/"+domain)
	} else {
		c.RunWithPipe("nano", dockerEnvironmentManager.HttpdConfPath+"/"+domain)
	}
}

func installDevelopmentEnvironment() {
	var services []string
	for _, value := range dockerEnvironmentManager.Services {
		services = append(services, value.ContainerName.(string))
	}

	answers = Checkboxes("Which are your favourite services?", services)
	for _, answer := range answers {
		check(answer)
	}

	continueAnswer := pickContinue()

	if continueAnswer == "y" {
		install()
		go dockerEnvironmentManager.Init()
	}
}

func importVirtualHosts() {
	var service string
	var path string

	service = pickService()

	prompt := &survey.Input{
		Message: "Select Docker Path:",
		Suggest: func(toComplete string) []string {
			files, _ := filepath.Glob(toComplete + "*")
			return files
		},
	}

	err := survey.AskOne(prompt, &path)
	if err != nil {
		fmt.Println()
	}

	continueAnswer := pickContinue()

	if continueAnswer == "y" {
		importVirtualHost(service, path)
	}
}

func selfUpdate() {
	arch := make(map[string]string)
	arch["386"] = "i386"
	arch["amd64"] = "x86_64"

	goos := make(map[string]string)
	goos["darwin"] = "Darwin"
	goos["linux"] = "Linux"
	goos["windows"] = "Windows"

	var updater = &selfupdate.Updater{
		CurrentVersion: "v1.0.0",
		BinURL:         "https://github.com/onuragtas/redock/releases/latest/download/redock_" + goos[runtime.GOOS] + "_" + arch[runtime.GOARCH],
		Dir:            "update/",
		CmdName:        "/docker-env",
	}

	if updater != nil {
		log.Println("update: started, please wait...")
		err := updater.Update()
		if err != nil {
			log.Println("update error:", err)
		}
		path, _ := osext.Executable()
		log.Println("update: finished")
		if err := syscall.Exec(path, os.Args, os.Environ()); err != nil {
			panic(err)
		}
	}
}
