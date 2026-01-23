package database

import (
	"fmt"

	"gorm.io/gorm"
)

// SavedCommandRepository handles saved commands CRUD operations
type SavedCommandRepository struct {
	db *gorm.DB
}

// NewSavedCommandRepository creates a new saved command repository
func NewSavedCommandRepository() *SavedCommandRepository {
	return &SavedCommandRepository{db: GetDB()}
}

// GetAll returns all saved commands (excluding soft-deleted)
func (r *SavedCommandRepository) GetAll() ([]SavedCommand, error) {
	var commands []SavedCommand
	if err := r.db.Find(&commands).Error; err != nil {
		return nil, err
	}
	return commands, nil
}

// Add creates a new saved command
func (r *SavedCommandRepository) Add(command, path string) error {
	cmd := SavedCommand{
		Command: command,
		Path:    path,
	}
	if err := r.db.Create(&cmd).Error; err != nil {
		return fmt.Errorf("failed to add command: %w", err)
	}
	return nil
}

// Delete soft-deletes a saved command by command string (deprecated, use DeleteByID)
func (r *SavedCommandRepository) Delete(command string) error {
	result := r.db.Where("command = ?", command).Delete(&SavedCommand{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete command: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("command not found: %s", command)
	}
	return nil
}

// DeleteByID soft-deletes a saved command by ID
func (r *SavedCommandRepository) DeleteByID(id uint) error {
	result := r.db.Delete(&SavedCommand{}, id)
	if result.Error != nil {
		return fmt.Errorf("failed to delete command: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("command not found with ID: %d", id)
	}
	return nil
}

// FindByID finds a saved command by ID
func (r *SavedCommandRepository) FindByID(id uint) (*SavedCommand, error) {
	var cmd SavedCommand
	if err := r.db.First(&cmd, id).Error; err != nil {
		return nil, err
	}
	return &cmd, nil
}

// Update updates a saved command by ID
func (r *SavedCommandRepository) Update(id uint, command, path string) error {
	result := r.db.Model(&SavedCommand{}).Where("id = ?", id).Updates(map[string]interface{}{
		"command": command,
		"path":    path,
	})
	if result.Error != nil {
		return fmt.Errorf("failed to update command: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("command not found with ID: %d", id)
	}
	return nil
}

// FindByCommand finds a saved command by command string
func (r *SavedCommandRepository) FindByCommand(command string) (*SavedCommand, error) {
	var cmd SavedCommand
	if err := r.db.Where("command = ?", command).First(&cmd).Error; err != nil {
		return nil, err
	}
	return &cmd, nil
}
