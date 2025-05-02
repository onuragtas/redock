package docker_manager

import (
	"fmt"
	"io/fs"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/onuragtas/command"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
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
}

type DevEnv struct {
	Username string `yaml:"username" json:"username"`
	Password string `yaml:"password" json:"password"`
	Port     int    `yaml:"port" json:"port"`
}

type Process struct {
	Name string
	Func func()
}

var answers []string

var dockerRepo = "https://github.com/onuragtas/docker"

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
	t.runInstall()

}

func (t *DockerEnvironmentManager) getDepends(answer string) []string {
	if depends, ok := dockerEnvironmentManager.CheckDepends(answer); ok {
		for _, dependsValue := range depends.Links {
			if !strings.Contains(dependsValue, answer) && !t.inService(dependsValue, answers) {
				answers = append(answers, dependsValue)
				t.getDepends(dependsValue)
			}
		}

		for _, dependsValue := range depends.DependsOn {
			if !strings.Contains(dependsValue, answer) && !t.inService(dependsValue, answers) {
				answers = append(answers, dependsValue)
				t.getDepends(dependsValue)
			}
		}
	}

	return answers
}

func (t *DockerEnvironmentManager) inService(service string, answers []string) bool {
	for _, answer := range answers {
		if service == answer {
			return true
		}
	}

	return false
}

func (t *DockerEnvironmentManager) AddService(item string) {

	t.CopyStruct = t.Struct
	t.CopyStruct["services"] = make(map[interface{}]interface{})
	services := t.ActiveServices
	services = append(services, item)

	depends := t.getDepends(item)
	for _, depend := range depends {
		if !t.inService(depend, services) {
			services = append(services, depend)
		}
	}

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

	t.ActiveServices = services

	t.command.RunCommand(t.GetWorkDir(), "sysctl", "-w", "vm.max_map_count=2048000")
	t.command.RunCommand(t.GetWorkDir(), "sysctl", "-w", "fs.file-max=65536")
	t.command.RunCommand(t.GetWorkDir(), "docker-compose", "up", "-d", item)
	t.Init()
}

func (t *DockerEnvironmentManager) RemoveService(item string) {
	t.CopyStruct = t.copyStruct
	if service, ok := t.GetService(item); ok {
		delete(t.CopyStruct["services"].(map[interface{}]interface{}), service.ContainerName)
	}

	yamlData, _ := yaml.Marshal(t.CopyStruct)
	err := ioutil.WriteFile(t.ComposeFilePath, yamlData, 0644)
	if err != nil {
		log.Println(err)
	}

	t.command.RunCommand(t.GetWorkDir(), "docker", "rm", item, "-f")
	newList := []string{}
	for _, v := range t.ActiveServices {
		if v != item {
			newList = append(newList, v)
		}
	}

	t.ActiveServices = newList
	t.Init()
}

func (t *DockerEnvironmentManager) runInstall() {
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

func (t *DockerEnvironmentManager) AddVirtualHost(service, domain, folder, phpVersion, typeConf, proxyPassPort string, addHosts bool) {
	t.Virtualhost.AddVirtualHost(service, domain, folder, phpVersion, typeConf, proxyPassPort, addHosts)
}

func (t *DockerEnvironmentManager) getHomeDir() string {
	dirname, _ := os.UserHomeDir()
	return dirname
}

func (t *DockerEnvironmentManager) Restart(service string) {
	if service == "nginx" {
		if t.DevEnv {
			t.command.RunCommand(t.GetWorkDir(), "docker", "-H", "192.168.36.240:4243", "exec", "-t", "nginx", "sh", "-c", "nginx -s reload")
		} else {
			t.command.RunCommand(t.GetWorkDir(), "docker-compose", "restart", "nginx")
		}
	} else {
		if t.DevEnv {
			t.command.RunCommand(t.GetWorkDir(), "docker", "-H", "192.168.36.240:4243", "exec", "-t", "nginx", "sh", "-c", "nginx -s reload")
			t.command.RunCommand(t.GetWorkDir(), "docker", "-H", "192.168.36.240:4243", "exec", "-t", "httpd", "sh", "-c", "apache2ctl restart")
		} else {
			t.command.RunCommand(t.GetWorkDir(), "docker-compose", "restart", "nginx")
			t.command.RunCommand(t.GetWorkDir(), "docker-compose", "restart", "httpd")
		}
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
	c := command.Command{}
	conf := fmt.Sprintf(xdebugConf, t.GetLocalIP(), 10000) // todo hardcoded read .env
	if ip, err := t.Virtualhost.getXDebugIp(); err == nil {
		t.Env = strings.ReplaceAll(t.Env, "XDEBUG_HOST="+ip, "XDEBUG_HOST="+t.GetLocalIP())
		os.WriteFile(t.EnvPath, []byte(t.Env), 0644)
	}

	var phpServices []string

	for _, service := range t.ActiveServices {
		if strings.Contains(service, "_xdebug") {
			phpServices = append(phpServices, service)
		}
	}

	for _, service := range phpServices {
		if strings.Contains(service, "81") || strings.Contains(service, "84") {
			conf = fmt.Sprintf(xdebugConf8, t.GetLocalIP(), 10000)
		} else {
			conf = fmt.Sprintf(xdebugConf, t.GetLocalIP(), 10000)
		}
		os.WriteFile(service+".ini", []byte(conf), 0644)
		c.RunWithPipe("docker", "cp", service+".ini", service+":/usr/local/etc/php/conf.d/xdebug.ini")
		os.RemoveAll(service + ".ini")
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
		//	go func(w *sync.WaitGroup, serviceName string) {
		c.RunWithPipe("/usr/local/bin/docker", "restart", service)
		//w.Done()
		//}(&wg, service)
	}

	//go func(w *sync.WaitGroup) {
	c.RunWithPipe("/usr/local/bin/docker", "restart", "httpd")
	//w.Done()
	//}(&wg)

	//go func(w *sync.WaitGroup) {
	c.RunWithPipe("/usr/local/bin/docker", "restart", "nginx")
	//w.Done()
	//}(&wg)

	//wg.Wait()
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

func (t *DockerEnvironmentManager) UpdateDocker() {
	_, err := git.PlainClone(t.GetWorkDir(), false, &git.CloneOptions{
		URL:      dockerRepo,
		Progress: os.Stdout,
	})
	if err != nil && err.Error() != git.ErrRepositoryAlreadyExists.Error() {
		panic(err)
	}

	r, err := git.PlainOpen(t.GetWorkDir())
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
