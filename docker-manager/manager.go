package docker_manager

import (
	"github.com/onuragtas/docker-env/command"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"os"
	"sort"
)

type DockerEnvironmentManager struct {
	ComposeFilePath    string
	File               string
	Struct             map[string]interface{}
	CopyStruct         map[string]interface{}
	copyStruct         map[string]interface{}
	Services           Services
	ActiveServices     []string
	EnvDistPath        string
	EnvPath            string
	InstallPath        string
	limitLog           int
	Env                string
	activeServices     map[int]bool
	command            command.Command
	AddVirtualHostPath string
	Virtualhost        *VirtualHost
	HttpdConfPath      string
	NginxConfPath      string
}

func Find(obj interface{}, key string) (interface{}, bool) {

	//if the argument is not a map, ignore it
	mobj, ok := obj.(map[string]interface{})
	if !ok {
		return nil, false
	}

	for k, v := range mobj {
		// key match, return value
		if k == key {
			return v, true
		}

		// if the value is a map, search recursively
		if m, ok := v.(map[string]interface{}); ok {
			if res, ok := Find(m, key); ok {
				return res, true
			}
		}
		// if the value is an array, search recursively
		// from each element
		if va, ok := v.([]interface{}); ok {
			for _, a := range va {
				if res, ok := Find(a, key); ok {
					return res, true
				}
			}
		}
	}

	// element not found
	return nil, false
}

func (t *DockerEnvironmentManager) Init() {
	t.Virtualhost = NewVirtualHost(t)
	t.command = command.Command{}
	t.activeServices = make(map[int]bool)
	envFile, err := ioutil.ReadFile(t.EnvDistPath)
	_, envFileErr := ioutil.ReadFile(t.EnvPath)
	t.Env = string(envFile)
	if envFileErr == nil {
		t.EnvDistPath = t.EnvPath
	}
	composeYamlFile, err := ioutil.ReadFile(t.ComposeFilePath)
	yamlFile, err := ioutil.ReadFile(t.File)
	if err != nil {
		log.Printf("yamlFile.Get err   #%v ", err)
	}
	err = yaml.Unmarshal(yamlFile, &t.Struct)
	err = yaml.Unmarshal(composeYamlFile, &t.copyStruct)
	if err != nil {
		log.Fatalf("Unmarshal: %v", err)
	}

	if obj, ok := Find(t.Struct, "services"); ok {
		i := 0
		for key, value := range obj.(map[interface{}]interface{}) {
			t.Services = append(t.Services, Service{
				ContainerName: key,
				Links:         t.findLinks(value),
				DependsOn:     t.findDependsOn(value),
				Original:      value,
			})

			t.activeServices[i] = t.isActive(key.(string))
			i++
		}
	}

	if obj, ok := Find(t.copyStruct, "services"); ok {
		i := 0
		for key := range obj.(map[interface{}]interface{}) {
			t.ActiveServices = append(t.ActiveServices, key.(string))
			i++
		}
	}

	sort.Slice(t.Services, func(i, j int) bool {
		return t.Services[i].ContainerName.(string) < t.Services[j].ContainerName.(string)
	})

	t.limitLog = 500

}

func (t *DockerEnvironmentManager) findLinks(value interface{}) []string {
	var links []string
	if obj, ok := value.(map[interface{}]interface{})["links"]; ok {
		for _, value := range obj.([]interface{}) {
			links = append(links, value.(string))
		}
	}
	return links
}

func (t *DockerEnvironmentManager) findDependsOn(value interface{}) []string {
	var dependsOn []string
	if obj, ok := value.(map[interface{}]interface{})["depends_on"]; ok {
		for _, value := range obj.([]interface{}) {
			dependsOn = append(dependsOn, value.(string))
		}
	}
	return dependsOn
}

func (t *DockerEnvironmentManager) CheckDepends(label string) (*Service, bool) {
	return t.GetService(label)
}

func (t *DockerEnvironmentManager) GetService(name string) (*Service, bool) {
	for _, value := range t.Services {
		if value.ContainerName == name {
			return &value, true
		}
	}
	return nil, false
}

func (t *DockerEnvironmentManager) Up(services []string) {
	t.createComposeFile(services)
	//t.startCommand("cp", t.EnvDistPath, t.EnvPath)
	t.command.RunCommand(t.GetWorkDir(), "sh", t.InstallPath)

}

func (t *DockerEnvironmentManager) createComposeFile(services []string) {
	t.CopyStruct = t.Struct
	t.CopyStruct["services"] = make(map[interface{}]interface{})
	for _, item := range services {
		if service, ok := t.GetService(item); ok {
			t.CopyStruct["services"].(map[interface{}]interface{})[item] = service.Original
		}
	}

	yamlData, _ := yaml.Marshal(t.CopyStruct)
	err := ioutil.WriteFile(t.ComposeFilePath, yamlData, 0644)
	if err != nil {
		log.Println(err)
	}
}

func (t *DockerEnvironmentManager) SetEnv(text string) {
	err := ioutil.WriteFile(t.EnvPath, []byte(text), 0644)
	if err != nil {
		log.Fatal(err)
	}
}

func (t *DockerEnvironmentManager) isActive(service string) bool {
	if obj, ok := Find(t.copyStruct, "services"); ok {
		for key := range obj.(map[interface{}]interface{}) {
			if key == service {
				return true
			}
		}
	}
	return false
}

func (t *DockerEnvironmentManager) GetActiveServices() map[int]bool {
	return t.activeServices
}

func (t *DockerEnvironmentManager) AddVirtualHost(service, domain, folder, phpVersion string) {
	t.Virtualhost.AddVirtualHost(service, domain, folder, phpVersion)
}

func (t *DockerEnvironmentManager) GetWorkDir() string {
	return t.getHomeDir() + "/.docker-environment"
}

func (t *DockerEnvironmentManager) getHomeDir() string {
	dirname, _ := os.UserHomeDir()
	return dirname
}

func (t *DockerEnvironmentManager) restart(service string) {
	if service == "nginx" {
		t.command.RunCommand(t.GetWorkDir(), "docker-compose", "restart", "nginx")
	} else {
		t.command.RunCommand(t.GetWorkDir(), "docker-compose", "restart", "nginx")
		t.command.RunCommand(t.GetWorkDir(), "docker-compose", "restart", "httpd")
	}
}

func (t *DockerEnvironmentManager) GetDomains(path string) []string {
	var domains []string
	files, err := ioutil.ReadDir(path)
	if err != nil {
		log.Fatal(err)
	}

	for _, f := range files {
		domains = append(domains, f.Name())
	}

	return domains
}
