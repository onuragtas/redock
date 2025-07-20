package devenv

import (
	"encoding/json"
	"log"
	"os"
	docker_manager "redock/docker-manager"
	"strconv"

	"github.com/onuragtas/command"
)

type DevEnvModel struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	Port       string `json:"port"`
	RedockPort string `json:"redockPort"`
}

type DevEnvManager struct {
	dockerEnvironmentManager *docker_manager.DockerEnvironmentManager
}

var manager DevEnvManager

func Init(dockerEnvironmentManager *docker_manager.DockerEnvironmentManager) {
	manager = DevEnvManager{dockerEnvironmentManager: dockerEnvironmentManager}
}

func GetDevEnvManager() *DevEnvManager {
	return &manager
}

func (t *DevEnvManager) DeleteDevEnv(username string) {
	file, err := os.ReadFile(t.dockerEnvironmentManager.GetWorkDir() + "/devenv.json")
	if err != nil {
		log.Println(err)
	}

	var devEnvList []docker_manager.DevEnv
	json.Unmarshal(file, &devEnvList)

	for i, listItem := range devEnvList {
		if listItem.Username == username {
			devEnvList = append(devEnvList[:i], devEnvList[i+1:]...)
		}
	}
	marshal, err := json.Marshal(devEnvList)

	if err != nil {
		log.Println(err)
	}

	os.WriteFile(t.dockerEnvironmentManager.GetWorkDir()+"/devenv.json", marshal, 0777)

	go func() {
		c := command.Command{}
		c.RunCommand(t.dockerEnvironmentManager.GetWorkDir(), "docker", "rm", username, "-f")
	}()
}

func (t *DevEnvManager) AddDevEnv(model *DevEnvModel) bool {
	port, _ := strconv.Atoi(model.Port)
	redockPort, _ := strconv.Atoi(model.RedockPort)

	manager := t.dockerEnvironmentManager

	file, err := os.ReadFile(manager.GetWorkDir() + "/devenv.json")
	if err != nil {
		log.Println(err)
	}

	var devEnvList []docker_manager.DevEnv

	json.Unmarshal(file, &devEnvList)

	// Kullanıcı zaten var mı kontrol et
	for _, listItem := range devEnvList {
		if listItem.Username == model.Username {
			log.Println("User already exists:", model.Username)
			return false
		}
	}

	// Port zaten kullanılıyor mu kontrol et
	for _, listItem := range devEnvList {
		if listItem.Port == port {
			log.Println("Port already in use:", port)
			return false
		}
	}

	// RedockPort zaten kullanılıyor mu kontrol et
	if redockPort > 0 {
		for _, listItem := range devEnvList {
			if listItem.RedockPort == redockPort {
				log.Println("RedockPort already in use:", redockPort)
				return false
			}
		}
	}

	devEnvList = append(devEnvList, docker_manager.DevEnv{
		Username:   model.Username,
		Password:   model.Password,
		Port:       port,
		RedockPort: redockPort,
	})

	marshal, err := json.Marshal(devEnvList)
	if err != nil {
		log.Println(err)
	}

	os.WriteFile(manager.GetWorkDir()+"/devenv.json", marshal, 0777)

	go func() {
		cmd := command.Command{}
		cmd.RunCommand(manager.GetWorkDir(), "bash", "serviceip.sh", model.Port, model.Username, model.Password, model.RedockPort)
	}()

	return true
}

func (t *DevEnvManager) EditDevEnv(model *DevEnvModel) bool {
	port, _ := strconv.Atoi(model.Port)
	redockPort, _ := strconv.Atoi(model.RedockPort)

	manager := t.dockerEnvironmentManager

	file, err := os.ReadFile(manager.GetWorkDir() + "/devenv.json")
	if err != nil {
		log.Println(err)
		return false
	}

	var devEnvList []docker_manager.DevEnv
	json.Unmarshal(file, &devEnvList)

	// Kullanıcıyı bul
	var userIndex = -1
	for i, listItem := range devEnvList {
		if listItem.Username == model.Username {
			userIndex = i
			break
		}
	}

	if userIndex == -1 {
		log.Println("User not found:", model.Username)
		return false
	}

	// Port çakışması kontrol et (kendi dışında)
	for i, listItem := range devEnvList {
		if i != userIndex && listItem.Port == port {
			log.Println("Port already in use by another user:", port)
			return false
		}
	}

	// RedockPort çakışması kontrol et (kendi dışında)
	if redockPort > 0 {
		for i, listItem := range devEnvList {
			if i != userIndex && listItem.RedockPort == redockPort {
				log.Println("RedockPort already in use by another user:", redockPort)
				return false
			}
		}
	}

	// Eski container'ı kaldır
	go func() {
		c := command.Command{}
		c.RunCommand(manager.GetWorkDir(), "docker", "rm", model.Username, "-f")
	}()

	// Güncelle
	devEnvList[userIndex].Password = model.Password
	devEnvList[userIndex].Port = port
	devEnvList[userIndex].RedockPort = redockPort

	marshal, err := json.Marshal(devEnvList)
	if err != nil {
		log.Println(err)
		return false
	}

	os.WriteFile(manager.GetWorkDir()+"/devenv.json", marshal, 0777)

	// Yeni container'ı başlat
	go func() {
		cmd := command.Command{}
		cmd.RunCommand(manager.GetWorkDir(), "bash", "serviceip.sh", model.Port, model.Username, model.Password, model.RedockPort)
	}()

	return true
}

func (t *DevEnvManager) Regenerate() {
	file, err := os.ReadFile(t.dockerEnvironmentManager.GetWorkDir() + "/devenv.json")
	if err != nil {
		log.Println(err)
	}

	var devEnvList []docker_manager.DevEnv
	json.Unmarshal(file, &devEnvList)

	c := command.Command{}
	c.RunCommand(t.dockerEnvironmentManager.GetWorkDir(), "docker", "pull", "hakanbaysal/devenv:latest")

	for _, env := range devEnvList {
		c.RunCommand(t.dockerEnvironmentManager.GetWorkDir(), "docker", "rm", env.Username, "-f")
		c.RunCommand(t.dockerEnvironmentManager.GetWorkDir(), "bash", "serviceip.sh", strconv.Itoa(env.Port), env.Username, env.Password, strconv.Itoa(env.RedockPort))
	}
}

func (t *DevEnvManager) Install() {
	c := command.Command{}
	c.RunCommand(t.dockerEnvironmentManager.GetWorkDir(), "bash", "install.sh")
}
