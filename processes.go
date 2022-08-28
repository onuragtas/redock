package main

import (
	"fmt"
	"io/fs"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"syscall"

	"github.com/AlecAivazis/survey/v2"
	"github.com/kardianos/osext"
	"github.com/onuragtas/docker-env/command"
	"github.com/onuragtas/docker-env/selfupdate"
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

	phpService = selectPhpServices()

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

func regenerateXDebugConf() {
	dockerEnvironmentManager.RegenerateXDebugConf()
}

func addXDebug() {
	filepath.Walk(dockerEnvironmentManager.HttpdConfPath, func(path string, info fs.FileInfo, err error) error {
		file, err := ioutil.ReadFile(path)
		if err != nil {
			return nil
		}

		var re = regexp.MustCompile(`(?m)fcgi://php([a-z0-9-_]+):9000`)

		for _, match := range re.FindAllString(string(file), -1) {
			if !strings.Contains(match, "xdebug") {
				n := strings.ReplaceAll(string(file), match, re.ReplaceAllString(match, "fcgi://php${1}_xdebug:9000"))
				ioutil.WriteFile(path, []byte(n), 0777)
				log.Println(path, "xdebug added")
			}
		}
		return nil
	})

	filepath.Walk(dockerEnvironmentManager.NginxConfPath, func(path string, info fs.FileInfo, err error) error {
		file, err := ioutil.ReadFile(path)
		if err != nil {
			return nil
		}

		var re = regexp.MustCompile(`(?m)fastcgi_pass php([a-z0-9-_]+):9000;`)

		for _, match := range re.FindAllString(string(file), -1) {
			if !strings.Contains(match, "xdebug") {
				n := strings.ReplaceAll(string(file), match, re.ReplaceAllString(match, "fastcgi_pass php${1}_xdebug:9000;"))
				ioutil.WriteFile(path, []byte(n), 0777)
				log.Println(path, "xdebug added")
			}
		}
		return nil
	})

	restartAll()
}

func removeXDebug() {
	filepath.Walk(dockerEnvironmentManager.HttpdConfPath, func(path string, info fs.FileInfo, err error) error {
		file, err := ioutil.ReadFile(path)
		if err != nil {
			return nil
		}

		var re = regexp.MustCompile(`(?m)fcgi://php([a-z0-9-_]+)([-_]+)([a-z]+):9000`)

		for _, match := range re.FindAllString(string(file), -1) {
			if strings.Contains(match, "xdebug") {
				n := strings.ReplaceAll(string(file), match, re.ReplaceAllString(match, "fcgi://php${1}:9000"))
				ioutil.WriteFile(path, []byte(n), 0777)
				log.Println(path, "xdebug removed")
			}
		}
		return nil
	})

	filepath.Walk(dockerEnvironmentManager.NginxConfPath, func(path string, info fs.FileInfo, err error) error {
		file, err := ioutil.ReadFile(path)
		if err != nil {
			return nil
		}

		var re = regexp.MustCompile(`(?m)fastcgi_pass php([a-z0-9-_]+)([-_]+)([a-z]+):9000;`)

		for _, match := range re.FindAllString(string(file), -1) {
			if strings.Contains(match, "xdebug") {
				n := strings.ReplaceAll(string(file), match, re.ReplaceAllString(match, "fastcgi_pass php${1}:9000;"))
				ioutil.WriteFile(path, []byte(n), 0777)
				log.Println(path, "xdebug removed")
			}
		}
		return nil
	})

	restartAll()
}

func restartAll() {
	var wg sync.WaitGroup
	wg.Add(6)
	c := command.Command{}

	go func(wg *sync.WaitGroup) {
		c.RunWithPipe("/usr/local/bin/docker", "restart", "php56_xdebug")
		wg.Done()
	}(&wg)

	go func(wg *sync.WaitGroup) {
		c.RunWithPipe("/usr/local/bin/docker", "restart", "php72_xdebug")
		wg.Done()
	}(&wg)

	go func(wg *sync.WaitGroup) {
		c.RunWithPipe("/usr/local/bin/docker", "restart", "php72_xdebug_kurumsal")
		wg.Done()
	}(&wg)

	go func(wg *sync.WaitGroup) {
		c.RunWithPipe("/usr/local/bin/docker", "restart", "php74_xdebug")
		wg.Done()
	}(&wg)

	go func(wg *sync.WaitGroup) {
		c.RunWithPipe("/usr/local/bin/docker", "restart", "httpd")
		wg.Done()
	}(&wg)

	go func(wg *sync.WaitGroup) {
		c.RunWithPipe("/usr/local/bin/docker", "restart", "nginx")
		wg.Done()
	}(&wg)

	wg.Wait()
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
	arch["arm64"] = "arm64"

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

func restartServices() {
	dockerEnvironmentManager.Restart("httpd")
}

func execBashService() {
	var domain string

	service := allServices()

	domains := dockerEnvironmentManager.GetDomains(dockerEnvironmentManager.Virtualhost.GetConfigPath("nginx"))

	selectBox := &survey.Select{Message: "Pick your domain", Options: domains, PageSize: 50}
	err := survey.AskOne(selectBox, &domain)
	if err != nil {
		log.Println(err)
	}

	dockerEnvironmentManager.ExecBash(service, domain)
}
