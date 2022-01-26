package main

import (
	"fmt"
	"github.com/AlecAivazis/survey/v2"
	"github.com/onuragtas/docker-env/command"
	"log"
	"strings"
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

	selectBox := &survey.Select{Message: "Pick your service", Options: []string{"nginx", "httpd"}}
	err := survey.AskOne(selectBox, &service)
	if err != nil {
		log.Println(err)
	}

	inputBox := &survey.Input{Message: "Domain:"}
	err = survey.AskOne(inputBox, &domain)
	if err != nil {
		log.Println(err)
	}

	inputBox = &survey.Input{Message: "Folder:"}
	err = survey.AskOne(inputBox, &folder)
	if err != nil {
		log.Println(err)
	}

	selectBox = &survey.Select{Message: "Pick your service", Options: []string{"php56", "php70", "php71", "php72", "php74", "php56_xdebug", "php72_xdebug", "php74_xdebug"}}
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
	domains = dockerEnvironmentManager.GetDomains()
	domains = append(domains, "Quit")

	selectBox := &survey.Select{Message: "Pick your domain", Options: domains, PageSize: 50}
	err := survey.AskOne(selectBox, &domain)
	if err != nil {
		log.Println(err)
	}

	if domain == "Quit" {
		return
	}

	selectBox = &survey.Select{Message: "Pick your service", Options: []string{"nginx", "httpd"}}
	err = survey.AskOne(selectBox, &service)
	if err != nil {
		log.Println(err)
	}

	c := command.Command{}
	if service == "nginx" {
		c.RunWithPipe("nano", dockerEnvironmentManager.NginxConfPath + "/" + domain)
	} else {
		c.RunWithPipe("nano", dockerEnvironmentManager.HttpdConfPath + "/" + domain)
	}
}

func main() {
	for true {
		selectProcess()
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

	install()
}

func selectProcess() {
	var process string
	selectBox := &survey.Select{Message: "Pick your process", Options: processes}
	err := survey.AskOne(selectBox, &process)
	if err != nil {
		log.Println(err)
	}

	for s, f := range processesMap {
		if s == process {
			f()
		}
	}
}

func Checkboxes(label string, opts []string) []string {
	var res []string
	prompt := &survey.MultiSelect{
		Default:  dockerEnvironmentManager.ActiveServices,
		Message:  label,
		Options:  opts,
		PageSize: 50,
	}
	err := survey.AskOne(prompt, &res)
	if err != nil {
		log.Println(err)
	}
	return res
}

func install() {
	dockerEnvironmentManager.Up(answers)
}

func check(answer string) {
	if depends, ok := dockerEnvironmentManager.CheckDepends(answer); ok {
		for _, dependsValue := range depends.Links {
			if !strings.Contains(dependsValue, answer) && !inService(dependsValue) {
				answers = append(answers, dependsValue)
				check(dependsValue)
			}
		}

		for _, dependsValue := range depends.DependsOn {
			if !strings.Contains(dependsValue, answer) && !inService(dependsValue) {
				answers = append(answers, dependsValue)
				check(dependsValue)
			}
		}
	}
}

func inService(service string) bool {
	for _, answer := range answers {
		if service == answer {
			return true
		}
	}

	return false
}
