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
	Username string `json:"username"`
	Password string `json:"password"`
	Port     string `json:"port"`
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

	manager := t.dockerEnvironmentManager

	file, err := os.ReadFile(manager.GetWorkDir() + "/devenv.json")
	if err != nil {
		log.Println(err)
	}

	var devEnvList []docker_manager.DevEnv

	json.Unmarshal(file, &devEnvList)

	devEnvList = append(devEnvList, docker_manager.DevEnv{
		Username: model.Username,
		Password: model.Password,
		Port:     port,
	})

	marshal, err := json.Marshal(devEnvList)
	if err != nil {
		log.Println(err)
	}

	os.WriteFile(manager.GetWorkDir()+"/devenv.json", marshal, 0777)

	go func() {
		cmd := command.Command{}
		cmd.RunCommand(manager.GetWorkDir(), "bash", "serviceip.sh", model.Port, model.Username, model.Password)
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
		c.RunCommand(t.dockerEnvironmentManager.GetWorkDir(), "bash", "serviceip.sh", strconv.Itoa(env.Port), env.Username, env.Password)
	}
}
