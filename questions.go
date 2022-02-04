package main

import (
	"github.com/AlecAivazis/survey/v2"
	"log"
)

func pickService() string {
	var service string
	selectBox := &survey.Select{Message: "Pick your service", Options: []string{"nginx", "httpd"}}
	err := survey.AskOne(selectBox, &service)
	if err != nil {
		log.Println(err)
	}
	return service
}

func pickContinue() string {
	var continueAnswer string
	selectBox := &survey.Select{Message: "Continue? :", Options: []string{"y", "n"}}
	err := survey.AskOne(selectBox, &continueAnswer)
	if err != nil {
		log.Println(err)
	}
	return continueAnswer
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

func selectProcess() {
	var process string
	selectBox := &survey.Select{Message: "Pick your process", Options: processes, PageSize: 20}
	err := survey.AskOne(selectBox, &process)
	if err != nil {
		log.Println(err)
	}

	for _, f := range processMapList {
		if f.Name == process {
			f.Func()
		}
	}
}

func selectPhpServices() string {
	var phpService string
	selectBox := &survey.Select{Message: "Pick your service", Options: []string{"php56", "php70", "php71", "php72", "php74", "php56_xdebug", "php72_xdebug", "php74_xdebug"}}
	err := survey.AskOne(selectBox, &phpService)
	if err != nil {
		log.Println(err)
	}
	return phpService
}

func allServices() string {
	var services []string
	var service string

	for _, value := range dockerEnvironmentManager.Services {
		services = append(services, value.ContainerName.(string))
	}

	selectBox := &survey.Select{Message: "Pick your service", Options: services}
	err := survey.AskOne(selectBox, &service)
	if err != nil {
		log.Println(err)
	}
	return service
}
