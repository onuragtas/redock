package models

import "redock/platform/memory"

// User struct to describe User object (memory.Entity).
// AllowedMenus: sadece user rolü için kullanılır; boşsa varsayılan menüler. admin için tüm menüler görünür.
type User struct {
	memory.BaseEntity                                 // id, created_at, updated_at
	Email        string   `json:"email"`
	PasswordHash string   `json:"password_hash"`
	UserStatus   int      `json:"user_status"`
	UserRole     string   `json:"user_role"`
	AllowedMenus []string `json:"allowed_menus,omitempty"` // görünecek menü path'leri (örn: "/", "/deployment")
}
