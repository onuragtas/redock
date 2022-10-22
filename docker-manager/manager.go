package docker_manager

import (
	"fmt"
	"github.com/onuragtas/docker-env/command"
	"gopkg.in/yaml.v2"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"runtime"
	"sort"
	"strings"
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
	t.Services = Services{}
	t.activeServices = make(map[int]bool)
	t.ActiveServices = []string{}

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
	yamlFile, err := ioutil.ReadFile(strings.ReplaceAll(t.File, "{.arch}", runtime.GOARCH))
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
	osName := runtime.GOOS
	switch osName {
	case "linux":
		t.command.RunCommand(t.GetWorkDir(), t.InstallPath)
		break
	default:
		t.command.RunCommand(t.GetWorkDir(), "sh", t.InstallPath)
	}

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

func (t *DockerEnvironmentManager) Restart(service string) {
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

func (t *DockerEnvironmentManager) ExecBash(service string, domain string) {
	c := command.Command{}
	c.AddStdIn(1, func() {
		_, _ = io.WriteString(os.Stdin, `export PHP_IDE_CONFIG="serverName=`+strings.ReplaceAll(domain, ".conf", "")+"\"")
	})
	c.RunWithPipe("docker", "exec", "-it", service, "bash")
}

func (t *DockerEnvironmentManager) getLocalIP() string {

	netInterfaceAddresses, err := net.InterfaceAddrs()

	if err != nil {
		return ""
	}

	for _, netInterfaceAddress := range netInterfaceAddresses {

		networkIp, ok := netInterfaceAddress.(*net.IPNet)

		if ok && !networkIp.IP.IsLoopback() && networkIp.IP.To4() != nil {

			ip := networkIp.IP.String()

			fmt.Println("Resolved Host IP: " + ip)

			return ip
		}
	}
	return ""
}
func (t *DockerEnvironmentManager) RegenerateXDebugConf() {
	c := command.Command{}
	conf := fmt.Sprintf(xdebugConf, t.getLocalIP(), 10000) // todo hardcoded read .env

	var phpServices []string

	for _, service := range t.ActiveServices {
		if strings.Contains(service, "_xdebug") {
			phpServices = append(phpServices, service)
		}
	}

	for _, service := range phpServices {
		c.RunWithPipe("/usr/local/bin/docker", "exec", "-it", service, "bash", "-c", `echo "`+conf+`" > /usr/local/etc/php/conf.d/xdebug.ini`)
	}

	t.RestartAll()
}

func (t *DockerEnvironmentManager) RestartAll() {
	//var wg sync.WaitGroup
	c := command.Command{}

	var phpServices []string

	for _, service := range t.ActiveServices {
		if strings.Contains(service, "php") {
			phpServices = append(phpServices, service)
		}
	}
	//wg.Add(len(phpServices) + 2)

	for _, service := range phpServices {
		//go func(wg *sync.WaitGroup, serviceName string) {
		c.RunWithPipe("/usr/local/bin/docker", "restart", service)
		//	wg.Done()
		//}(&wg, service)
	}

	//go func(wg *sync.WaitGroup) {
	c.RunWithPipe("/usr/local/bin/docker", "restart", "httpd")
	//wg.Done()
	//}(&wg)

	//go func(wg *sync.WaitGroup) {
	c.RunWithPipe("/usr/local/bin/docker", "restart", "nginx")
	//	wg.Done()
	//}(&wg)

	//wg.Wait()
}
