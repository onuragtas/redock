package database

import (
	"fmt"
	"redock/platform/memory"
)

// SavedCommandRepository handles saved commands CRUD operations
type SavedCommandRepository struct {
	db *memory.Database
}

// NewSavedCommandRepository creates a new saved command repository
func NewSavedCommandRepository() *SavedCommandRepository {
	return &SavedCommandRepository{db: GetMemoryDB()}
}

// GetAll returns all saved commands (excluding soft-deleted)
func (r *SavedCommandRepository) GetAll() ([]SavedCommand, error) {
	commands := memory.FindAll[*SavedCommand](r.db, "saved_commands")
	result := make([]SavedCommand, len(commands))
	for i, cmd := range commands {
		result[i] = *cmd
	}
	return result, nil
}

// Add creates a new saved command
func (r *SavedCommandRepository) Add(command, path string) error {
	cmd := &SavedCommand{
		Command: command,
		Path:    path,
	}
	if err := memory.Create[*SavedCommand](r.db, "saved_commands", cmd); err != nil {
		return fmt.Errorf("failed to add command: %w", err)
	}
	return nil
}

// Delete soft-deletes a saved command by command string (deprecated, use DeleteByID)
func (r *SavedCommandRepository) Delete(command string) error {
	commands := memory.Filter[*SavedCommand](r.db, "saved_commands", func(c *SavedCommand) bool {
		return c.Command == command
	})
	if len(commands) == 0 {
		return fmt.Errorf("command not found: %s", command)
	}
	
	for _, cmd := range commands {
		if err := memory.Delete[*SavedCommand](r.db, "saved_commands", cmd.ID); err != nil {
			return fmt.Errorf("failed to delete command: %w", err)
		}
	}
	return nil
}

// DeleteByID soft-deletes a saved command by ID
func (r *SavedCommandRepository) DeleteByID(id uint) error {
	if err := memory.Delete[*SavedCommand](r.db, "saved_commands", id); err != nil {
		return fmt.Errorf("failed to delete command: %w", err)
	}
	return nil
}

// FindByID finds a saved command by ID
func (r *SavedCommandRepository) FindByID(id uint) (*SavedCommand, error) {
	cmd, err := memory.FindByID[*SavedCommand](r.db, "saved_commands", id)
	if err != nil {
		return nil, err
	}
	return cmd, nil
}

// Update updates a saved command by ID
func (r *SavedCommandRepository) Update(id uint, command, path string) error {
	cmd, err := memory.FindByID[*SavedCommand](r.db, "saved_commands", id)
	if err != nil {
		return fmt.Errorf("command not found with ID: %d", id)
	}
	
	cmd.Command = command
	cmd.Path = path
	
	if err := memory.Update[*SavedCommand](r.db, "saved_commands", cmd); err != nil {
		return fmt.Errorf("failed to update command: %w", err)
	}
	return nil
}

// FindByCommand finds a saved command by command string
func (r *SavedCommandRepository) FindByCommand(command string) (*SavedCommand, error) {
	commands := memory.Filter[*SavedCommand](r.db, "saved_commands", func(c *SavedCommand) bool {
		return c.Command == command
	})
	if len(commands) == 0 {
		return nil, fmt.Errorf("command not found")
	}
	return commands[0], nil
}
