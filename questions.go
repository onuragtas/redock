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

	for s, f := range processesMap {
		if s == process {
			f()
		}
	}
}
