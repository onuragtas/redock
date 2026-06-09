package docker_manager

import (
	"context"
	"io/fs"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path/filepath"
	"redock/docker-manager/stacks"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/onuragtas/command"
	"gopkg.in/yaml.v2"
)

type DockerEnvironmentManager struct {
	ComposeFilePath    string
	File               string
	Struct             map[string]interface{}
	CopyStruct         map[string]interface{}
	copyStruct         map[string]interface{}
	Services           Services
	ActiveServicesList Services
	ActiveServices     []string
	EnvDistPath        string
	EnvDist            string
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
	DevEnv             bool
	Username           string
	ServiceSettings    *ServiceSettings
}

type DevEnv struct {
	Username   string `yaml:"username" json:"username"`
	Password   string `yaml:"password" json:"password"`
	Port       int    `yaml:"port" json:"port"`
	RedockPort int    `yaml:"redockPort" json:"redockPort"`
}

type Process struct {
	Name string
	Func func()
}

var dockerEnvironmentManager DockerEnvironmentManager

func (t *DockerEnvironmentManager) GetWorkDir() string {
	return t.getHomeDir() + "/.docker-environment"
}

func GetDockerManager() *DockerEnvironmentManager {
	return &dockerEnvironmentManager
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

func (t *DockerEnvironmentManager) initialize() {
	t.File = dockerEnvironmentManager.GetWorkDir() + "/docker-compose.yml.{.arch}.dist"
	t.ComposeFilePath = dockerEnvironmentManager.GetWorkDir() + "/docker-compose.yml"
	t.EnvDistPath = dockerEnvironmentManager.GetWorkDir() + "/.env.example"
	t.EnvPath = dockerEnvironmentManager.GetWorkDir() + "/.env"
	t.InstallPath = dockerEnvironmentManager.GetWorkDir() + "/install.sh"
	t.AddVirtualHostPath = dockerEnvironmentManager.GetWorkDir() + "/add_virtualhost.sh"
	t.HttpdConfPath = dockerEnvironmentManager.GetWorkDir() + "/httpd/sites-enabled"
	t.NginxConfPath = dockerEnvironmentManager.GetWorkDir() + "/etc/nginx"
}

func (t *DockerEnvironmentManager) Init() {
	t.initialize()
	t.loadServiceSettings()

	t.Services = Services{}
	t.activeServices = make(map[int]bool)
	t.ActiveServices = []string{}

	t.Virtualhost = NewVirtualHost(t)
	t.command = command.Command{}
	t.activeServices = make(map[int]bool)
	envDist, err := ioutil.ReadFile(t.EnvDistPath)
	t.EnvDist = string(envDist)
	envFile, envFileErr := ioutil.ReadFile(t.EnvPath)
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
				Image:         t.findImage(value),
			})

			t.activeServices[i] = t.isActive(key.(string))
			i++
		}
	}

	if obj, ok := Find(t.copyStruct, "services"); ok {
		i := 0
		for key, value := range obj.(map[interface{}]interface{}) {
			t.ActiveServices = append(t.ActiveServices, key.(string))
			t.ActiveServicesList = append(t.ActiveServicesList, Service{
				ContainerName: key,
				Links:         t.findLinks(value),
				DependsOn:     t.findDependsOn(value),
				Original:      value,
				Image:         t.findImage(value),
			})
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
func (t *DockerEnvironmentManager) findImage(value interface{}) string {
	var image string
	if obj, ok := value.(map[interface{}]interface{})["image"]; ok {
		image = obj.(string)
	}
	return image
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
	if m, err := stacks.GetManager(t.GetWorkDir()); err == nil {
		_ = m.Up(context.Background(), services...)
	}
}

func (t *DockerEnvironmentManager) AddService(item string) {
	if m, err := stacks.GetManager(t.GetWorkDir()); err == nil {
		_ = m.Up(context.Background(), item)
	}
}

func (t *DockerEnvironmentManager) RemoveService(item string) {
	if m, err := stacks.GetManager(t.GetWorkDir()); err == nil {
		_ = m.Down(context.Background(), item)
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

func (t *DockerEnvironmentManager) AddVirtualHost(service, domain, folder, phpVersion, typeConf, proxyPassPort string, addHosts bool) {
	t.Virtualhost.AddVirtualHost(service, domain, folder, phpVersion, typeConf, proxyPassPort, addHosts)
}

func (t *DockerEnvironmentManager) getHomeDir() string {
	dirname, _ := os.UserHomeDir()
	return dirname
}

func (t *DockerEnvironmentManager) Restart(service string) {
	m, err := stacks.GetManager(t.GetWorkDir())
	if err != nil {
		return
	}
	if service == "nginx" {
		_ = m.Restart(context.Background(), "nginx")
		return
	}
	_ = m.Restart(context.Background(), "nginx")
	_ = m.Restart(context.Background(), "httpd")
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
	cmd := `PHP_IDE_CONFIG=serverName=` + strings.ReplaceAll(domain, ".conf", "")
	//c.AddStdIn(1, func() {
	//	_, _ = io.WriteString(os.Stdin, `export PHP_IDE_CONFIG="serverName=`+strings.ReplaceAll(domain, ".conf", "")+"\"")
	//})
	c.RunWithPipe("docker", "exec", "-it", service, "env", cmd, "bash", "-l")
}

func (t *DockerEnvironmentManager) GetLocalIP() string {

	netInterfaceAddresses, err := net.InterfaceAddrs()

	if err != nil {
		return ""
	}

	for _, netInterfaceAddress := range netInterfaceAddresses {

		networkIp, ok := netInterfaceAddress.(*net.IPNet)

		if t.DevEnv && !strings.Contains(networkIp.IP.String(), "172.28") {
			continue
		}

		if ok && !networkIp.IP.IsLoopback() && networkIp.IP.To4() != nil {

			ip := networkIp.IP.String()

			return ip
		}
	}
	return ""
}
func (t *DockerEnvironmentManager) RegenerateXDebugConf() {
	// Update XDEBUG_HOST in .env to the current local IP.
	if ip, err := t.Virtualhost.getXDebugIp(); err == nil {
		t.Env = strings.ReplaceAll(t.Env, "XDEBUG_HOST="+ip, "XDEBUG_HOST="+t.GetLocalIP())
		os.WriteFile(t.EnvPath, []byte(t.Env), 0644)
	}
	if m, err := stacks.GetManager(t.GetWorkDir()); err == nil {
		m.ReloadEnv()
		_ = m.RegenerateXDebugINI(context.Background())
	}
}

func (t *DockerEnvironmentManager) RestartAll() {
	m, err := stacks.GetManager(t.GetWorkDir())
	if err != nil {
		return
	}
	for _, name := range m.Active() {
		if strings.Contains(name, "php") {
			_ = m.Restart(context.Background(), name)
		}
	}
	_ = m.Restart(context.Background(), "httpd")
	_ = m.Restart(context.Background(), "nginx")
}

func (t *DockerEnvironmentManager) CheckLocalIpAndRegenerate() {
	for true {
		localIp := t.GetLocalIP()
		if ip, err := t.Virtualhost.getXDebugIp(); err == nil && ip != localIp {
			t.RegenerateXDebugConf()
		}
		time.Sleep(5 * time.Second)
	}

}

func (t *DockerEnvironmentManager) AddXDebug() {
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

func (t *DockerEnvironmentManager) RemoveXDebug() {
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
