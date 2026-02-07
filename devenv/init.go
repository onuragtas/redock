package devenv

import (
	"log"
	"strconv"

	docker_manager "redock/docker-manager"
	"redock/platform/database"
	"redock/platform/memory"

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

func (t *DevEnvManager) db() *memory.Database {
	return database.GetMemoryDB()
}

// toDTO DevEnvEntity -> docker_manager.DevEnv (API/DTO)
func toDTO(e *DevEnvEntity) docker_manager.DevEnv {
	return docker_manager.DevEnv{
		Username:   e.Username,
		Password:   e.Password,
		Port:       e.Port,
		RedockPort: e.RedockPort,
	}
}

// GetList memory DB'den tüm dev env listesini döner (API için).
func (t *DevEnvManager) GetList() []docker_manager.DevEnv {
	db := t.db()
	if db == nil {
		return nil
	}
	entities := memory.FindAll[*DevEnvEntity](db, "dev_envs")
	out := make([]docker_manager.DevEnv, 0, len(entities))
	for _, e := range entities {
		out = append(out, toDTO(e))
	}
	return out
}

func (t *DevEnvManager) DeleteDevEnv(username string) {
	db := t.db()
	if db == nil {
		return
	}
	list := memory.Where[*DevEnvEntity](db, "dev_envs", "Username", username)
	for _, e := range list {
		if err := memory.Delete[*DevEnvEntity](db, "dev_envs", e.GetID()); err != nil {
			log.Println("DeleteDevEnv:", err)
		}
	}
	go func() {
		c := command.Command{}
		c.RunCommand(t.dockerEnvironmentManager.GetWorkDir(), "docker", "rm", username, "-f")
	}()
}

func (t *DevEnvManager) AddDevEnv(model *DevEnvModel) bool {
	port, _ := strconv.Atoi(model.Port)
	redockPort, _ := strconv.Atoi(model.RedockPort)
	db := t.db()
	if db == nil {
		return false
	}
	manager := t.dockerEnvironmentManager

	all := memory.FindAll[*DevEnvEntity](db, "dev_envs")

	for _, e := range all {
		if e.Username == model.Username {
			log.Println("User already exists:", model.Username)
			return false
		}
	}
	for _, e := range all {
		if e.Port == port {
			log.Println("Port already in use:", port)
			return false
		}
	}
	if redockPort > 0 {
		for _, e := range all {
			if e.RedockPort == redockPort {
				log.Println("RedockPort already in use:", redockPort)
				return false
			}
		}
	}

	entity := &DevEnvEntity{
		Username:   model.Username,
		Password:   model.Password,
		Port:       port,
		RedockPort: redockPort,
	}
	if err := memory.Create(db, "dev_envs", entity); err != nil {
		log.Println("AddDevEnv:", err)
		return false
	}

	go func() {
		cmd := command.Command{}
		cmd.RunCommand(manager.GetWorkDir(), "bash", "serviceip.sh", model.Port, model.Username, model.Password, model.RedockPort)
	}()

	return true
}

func (t *DevEnvManager) EditDevEnv(model *DevEnvModel) bool {
	port, _ := strconv.Atoi(model.Port)
	redockPort, _ := strconv.Atoi(model.RedockPort)
	db := t.db()
	if db == nil {
		return false
	}
	manager := t.dockerEnvironmentManager

	list := memory.Where[*DevEnvEntity](db, "dev_envs", "Username", model.Username)
	if len(list) == 0 {
		log.Println("User not found:", model.Username)
		return false
	}
	entity := list[0]
	all := memory.FindAll[*DevEnvEntity](db, "dev_envs")

	for i, e := range all {
		if e.GetID() != entity.GetID() && e.Port == port {
			log.Println("Port already in use by another user:", port)
			return false
		}
		_ = i
	}
	if redockPort > 0 {
		for _, e := range all {
			if e.GetID() != entity.GetID() && e.RedockPort == redockPort {
				log.Println("RedockPort already in use by another user:", redockPort)
				return false
			}
		}
	}

	go func() {
		c := command.Command{}
		c.RunCommand(manager.GetWorkDir(), "docker", "rm", model.Username, "-f")
	}()

	entity.Password = model.Password
	entity.Port = port
	entity.RedockPort = redockPort
	if err := memory.Update(db, "dev_envs", entity); err != nil {
		return false
	}

	go func() {
		cmd := command.Command{}
		cmd.RunCommand(manager.GetWorkDir(), "bash", "serviceip.sh", model.Port, model.Username, model.Password, model.RedockPort)
	}()

	return true
}

func (t *DevEnvManager) Regenerate() {
	db := t.db()
	if db == nil {
		return
	}
	devEnvList := memory.FindAll[*DevEnvEntity](db, "dev_envs")

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
