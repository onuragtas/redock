package saved_commands

import (
	"log"
	docker_manager "redock/docker-manager"
	"redock/platform/database"
)

type Model struct {
	ID      uint   `json:"id"`
	Command string `json:"command"`
	Path    string `json:"path"`
}

type Manager struct {
	dockerEnvironmentManager *docker_manager.DockerEnvironmentManager
	repo                     *database.SavedCommandRepository
}

var manager Manager

func Init(dockerEnvironmentManager *docker_manager.DockerEnvironmentManager) {
	manager = Manager{
		dockerEnvironmentManager: dockerEnvironmentManager,
		repo:                     database.NewSavedCommandRepository(),
	}
}

func GetManager() *Manager {
	return &manager
}

func (t *Manager) Delete(model Model) {
	if err := t.repo.Delete(model.Command); err != nil {
		log.Printf("Failed to delete saved command: %v", err)
	}
}

func (t *Manager) Add(model *Model) bool {
	// Check if command already exists
	if existing, _ := t.repo.FindByCommand(model.Command); existing != nil {
		return false
	}

	// Use current working directory if path is empty
	if model.Path == "" {
		model.Path = t.dockerEnvironmentManager.GetWorkDir()
	}

	if err := t.repo.Add(model.Command, model.Path); err != nil {
		log.Printf("Failed to add saved command: %v", err)
		return false
	}

	return true
}

func (t *Manager) GetList() []Model {
	commands, err := t.repo.GetAll()
	if err != nil {
		log.Printf("Failed to get saved commands: %v", err)
		return []Model{}
	}

	// Convert database models to API models
	list := make([]Model, len(commands))
	for i, cmd := range commands {
		list[i] = Model{
			ID:      cmd.ID,
			Command: cmd.Command,
			Path:    cmd.Path,
		}
	}

	return list
}

// DeleteByID deletes a saved command by ID
func (t *Manager) DeleteByID(id uint) error {
	return t.repo.DeleteByID(id)
}

// UpdateByID updates a saved command by ID
func (t *Manager) UpdateByID(id uint, command, path string) error {
	return t.repo.Update(id, command, path)
}

// GetByID gets a saved command by ID
func (t *Manager) GetByID(id uint) (*Model, error) {
	cmd, err := t.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	return &Model{
		ID:      cmd.ID,
		Command: cmd.Command,
		Path:    cmd.Path,
	}, nil
}
