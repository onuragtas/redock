package main

import (
	"fmt"
	"github.com/AlecAivazis/survey/v2"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"io/fs"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"

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
	var typeConf string
	var proxyPass string

	service = pickService()

	inputBox := &survey.Input{Message: "Domain:"}
	err := survey.AskOne(inputBox, &domain)
	if err != nil {
		log.Println(err)
	}

	typeConf = selectTypeConf()

	if typeConf == "Default" {

		inputBox = &survey.Input{Message: "Folder:"}
		err = survey.AskOne(inputBox, &folder)
		if err != nil {
			log.Println(err)
		}

		phpService = selectPhpServices()

	} else {
		proxyPass = ask("Proxy Pass Port:")
	}

	fmt.Println(domain)
	dockerEnvironmentManager.AddVirtualHost(service, domain, folder, phpService, typeConf, proxyPass, true)
}

func editVirtualHost() {
	var service string
	var domain string
	var domains []string

	service = pickService()

	domains = dockerEnvironmentManager.GetDomains(dockerEnvironmentManager.Virtualhost.GetConfigPath(service))
	domains = append(domains, "Quit")

	selectBox := &survey.Select{Message: "Pick your domain", Options: domains, PageSize: 10}
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

	dockerEnvironmentManager.RestartAll()
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

	dockerEnvironmentManager.RestartAll()
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

func dockerUpdate() {
	_, err := git.PlainClone(getHomeDir()+"/.docker-environment", false, &git.CloneOptions{
		URL:      dockerRepo,
		Progress: os.Stdout,
	})
	if err != nil && err.Error() != git.ErrRepositoryAlreadyExists.Error() {
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
}
func dockerImageUpdate() {
	command := command.Command{}
	for _, service := range dockerEnvironmentManager.ActiveServicesList {
		if service.Image != "" {
			log.Println("docker pull", service.Image)
			command.RunWithPipe("docker", "pull", service.Image)
		}
	}
}

func selfUpdate() {

	log.Println("https://github.com/onuragtas/redock/releases/latest/download/redock_"+runtime.GOOS+"_"+runtime.GOARCH, "downloading...")

	var updater = &selfupdate.Updater{
		CurrentVersion: "v1.0.0",
		BinURL:         "https://github.com/onuragtas/redock/releases/latest/download/redock_" + runtime.GOOS + "_" + runtime.GOARCH,
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
