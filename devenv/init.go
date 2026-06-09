package devenv

import (
	"context"
	"log"
	"strconv"

	docker_manager "redock/docker-manager"
	"redock/docker-manager/stacks"
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
		if m, err := stacks.GetManager(t.dockerEnvironmentManager.GetWorkDir()); err == nil {
			_ = m.RemoveDevEnv(context.Background(), username)
		}
	}()
}

func (t *DevEnvManager) AddDevEnv(model *DevEnvModel) bool {
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

	// SSH host port is auto-assigned from a configurable base (skipping ports
	// already taken or bound on the host); the redock port is user-provided
	// (default 6001) and not auto-incremented.
	usedSSH := map[int]bool{}
	for _, e := range all {
		usedSSH[e.Port] = true
	}
	sshBase := 100
	if m, err := stacks.GetManager(manager.GetWorkDir()); err == nil {
		sshBase = m.GetDevEnvSettings().SSHPortBase
	}
	port := stacks.NextFreePort(sshBase, usedSSH)

	redockPort := 6001
	if v, err := strconv.Atoi(model.RedockPort); err == nil && v > 0 {
		redockPort = v
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
		if m, err := stacks.GetManager(manager.GetWorkDir()); err == nil {
			if err := m.CreateDevEnv(context.Background(), model.Username, model.Password, port, redockPort); err != nil {
				log.Println("CreateDevEnv:", err)
			}
		}
	}()

	return true
}

func (t *DevEnvManager) EditDevEnv(model *DevEnvModel) bool {
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

	// SSH port stays auto-assigned/stable; password and redock port are editable.
	entity.Password = model.Password
	if v, err := strconv.Atoi(model.RedockPort); err == nil && v > 0 {
		entity.RedockPort = v
	}
	if err := memory.Update(db, "dev_envs", entity); err != nil {
		return false
	}

	go func() {
		if m, err := stacks.GetManager(manager.GetWorkDir()); err == nil {
			_ = m.RemoveDevEnv(context.Background(), model.Username)
			if err := m.CreateDevEnv(context.Background(), model.Username, model.Password, entity.Port, entity.RedockPort); err != nil {
				log.Println("CreateDevEnv:", err)
			}
		}
	}()

	return true
}

func (t *DevEnvManager) Regenerate() {
	db := t.db()
	if db == nil {
		return
	}
	devEnvList := memory.FindAll[*DevEnvEntity](db, "dev_envs")

	m, err := stacks.GetManager(t.dockerEnvironmentManager.GetWorkDir())
	if err != nil {
		return
	}
	for _, env := range devEnvList {
		_ = m.RemoveDevEnv(context.Background(), env.Username)
		if err := m.CreateDevEnv(context.Background(), env.Username, env.Password, env.Port, env.RedockPort); err != nil {
			log.Println("CreateDevEnv:", err)
		}
	}
}

func (t *DevEnvManager) Install() {
	c := command.Command{}
	c.RunCommand(t.dockerEnvironmentManager.GetWorkDir(), "bash", "install.sh")
}
