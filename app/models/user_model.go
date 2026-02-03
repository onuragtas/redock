package models

import "redock/platform/memory"

// User struct to describe User object (memory.Entity).
// JSON format: {"id":0,"created_at":"2025-05-16T13:53:56.185691+03:00","updated_at":"0001-01-01T00:00:00Z","email":"admin","password_hash":"...","user_status":1,"user_role":"admin"}
type User struct {
	memory.BaseEntity                                 // id, created_at, updated_at
	Email        string    `json:"email"`
	PasswordHash string    `json:"password_hash"`
	UserStatus   int       `json:"user_status"`
	UserRole     string    `json:"user_role"`
}
