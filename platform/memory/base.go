package memory

import "time"

// BaseEntity provides common fields and methods for all entities
type BaseEntity struct {
	ID        uint      `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GetID returns the entity ID
func (b *BaseEntity) GetID() uint {
	return b.ID
}

// SetID sets the entity ID
func (b *BaseEntity) SetID(id uint) {
	b.ID = id
}

// SetTimestamps sets created and updated timestamps
func (b *BaseEntity) SetTimestamps(created, updated time.Time) {
	b.CreatedAt = created
	b.UpdatedAt = updated
}

// SoftDeleteEntity provides soft delete functionality
type SoftDeleteEntity struct {
	BaseEntity
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

// IsDeleted checks if entity is soft deleted
func (s *SoftDeleteEntity) IsDeleted() bool {
	return s.DeletedAt != nil
}

// SoftDelete marks entity as deleted
func (s *SoftDeleteEntity) SoftDelete() {
	now := time.Now()
	s.DeletedAt = &now
}
