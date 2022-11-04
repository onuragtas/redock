package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

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

func copyFolder(source, destination string) error {
	var err = filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		var relPath = strings.Replace(path, source, "", 1)
		if relPath == "" {
			return nil
		}
		if info.IsDir() {
			return os.Mkdir(filepath.Join(destination, relPath), 0755)
		} else {
			var data, err1 = ioutil.ReadFile(filepath.Join(source, relPath))
			if err1 != nil {
				return err1
			}
			return ioutil.WriteFile(filepath.Join(destination, relPath), data, 0777)
		}
	})
	return err
}

func importVirtualHost(service string, path string) {
	if service == "nginx" {
		err := copyFolder(path+"/etc/nginx", dockerEnvironmentManager.NginxConfPath)
		if err != nil {
			fmt.Println(err)
		}
	} else {
		err := copyFolder(path+"/httpd/sites-enabled", dockerEnvironmentManager.HttpdConfPath)
		if err != nil {
			fmt.Println(err)
		}
	}
}
