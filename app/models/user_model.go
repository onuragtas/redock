package models

import (
	"time"
)

// User struct to describe User object.
type User struct {
	ID           int       `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"password_hash"`
	UserStatus   int       `json:"user_status"`
	UserRole     string    `json:"user_role"`
}
