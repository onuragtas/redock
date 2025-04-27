package saved_commands

import (
	"encoding/json"
	"log"
	"os"
	docker_manager "redock/docker-manager"
)

type Model struct {
	Command string `json:"command"`
}

type Manager struct {
	dockerEnvironmentManager *docker_manager.DockerEnvironmentManager
}

var manager Manager

func Init(dockerEnvironmentManager *docker_manager.DockerEnvironmentManager) {
	manager = Manager{dockerEnvironmentManager: dockerEnvironmentManager}
}

func GetManager() *Manager {
	return &manager
}

func (t *Manager) Delete(model Model) {
	file, err := os.ReadFile(t.dockerEnvironmentManager.GetWorkDir() + "/saved_commands.json")
	if err != nil {
		log.Println(err)
	}

	var list []Model
	json.Unmarshal(file, &list)

	for i, item := range list {
		if item.Command == model.Command {
			list = append(list[:i], list[i+1:]...)
		}
	}
	marshal, err := json.Marshal(list)

	if err != nil {
		log.Println(err)
	}

	os.WriteFile(t.dockerEnvironmentManager.GetWorkDir()+"/saved_commands.json", marshal, 0777)
}

func (t *Manager) Add(model *Model) bool {

	manager := t.dockerEnvironmentManager

	file, err := os.ReadFile(manager.GetWorkDir() + "/saved_commands.json")
	if err != nil {
		log.Println(err)
	}

	var list []Model

	json.Unmarshal(file, &list)

	for _, item := range list {
		if item.Command == model.Command {
			return false
		}
	}

	list = append(list, *model)

	marshal, err := json.Marshal(list)
	if err != nil {
		log.Println(err)
	}

	os.WriteFile(manager.GetWorkDir()+"/saved_commands.json", marshal, 0777)

	return true
}

func (t *Manager) GetList() []Model {
	file, err := os.ReadFile(t.dockerEnvironmentManager.GetWorkDir() + "/saved_commands.json")
	if err != nil {
		log.Println(err)
	}

	var list []Model
	json.Unmarshal(file, &list)
	return list
}
