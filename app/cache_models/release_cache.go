package cache_models

import (
	"redock/selfupdate"
	"time"
)

// ReleaseCache stores cached GitHub releases
type ReleaseCache struct {
	ID        uint                          `json:"id"`
	Owner     string                        `json:"owner"`
	Repo      string                        `json:"repo"`
	Releases  []selfupdate.ReleaseInfo      `json:"releases"`
	FetchedAt time.Time                     `json:"fetched_at"`
	CreatedAt time.Time                     `json:"created_at"`
	UpdatedAt time.Time                     `json:"updated_at"`
}

// GetID returns the ID
func (r *ReleaseCache) GetID() uint {
	return r.ID
}

// SetID sets the ID
func (r *ReleaseCache) SetID(id uint) {
	r.ID = id
}

// SetTimestamps sets created and updated timestamps
func (r *ReleaseCache) SetTimestamps(created, updated time.Time) {
	r.CreatedAt = created
	r.UpdatedAt = updated
}

// IsValid checks if cache is still valid (5 minutes)
func (r *ReleaseCache) IsValid() bool {
	return time.Since(r.FetchedAt) < 5*time.Minute
}
