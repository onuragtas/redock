package models

// SignUp struct to describe register (sadece ilk kullanıcı; role sunucuda admin yapılır).
type SignUp struct {
	Email    string `json:"email" validate:"required,email,lte=255"`
	Password string `json:"password" validate:"required,lte=255"`
	UserRole string `json:"user_role" validate:"omitempty,lte=25"` // ilk kayıtta kullanılmaz
}

// SignIn struct to describe login user.
type SignIn struct {
	Email    string `json:"email" validate:"required,email,lte=255"`
	Password string `json:"password" validate:"required,lte=255"`
}

// CreateUserRequest admin tarafından yeni kullanıcı eklemek için.
type CreateUserRequest struct {
	Email        string   `json:"email" validate:"required,email,lte=255"`
	Password     string   `json:"password" validate:"required,lte=255"`
	UserRole     string   `json:"user_role" validate:"required,oneof=admin user"`
	AllowedMenus []string `json:"allowed_menus,omitempty"`
}

// UpdateUserRequest admin tarafından kullanıcı güncellemek için.
type UpdateUserRequest struct {
	UserRole     *string  `json:"user_role,omitempty" validate:"omitempty,oneof=admin user"`
	UserStatus   *int     `json:"user_status,omitempty" validate:"omitempty,oneof=0 1"`
	AllowedMenus []string `json:"allowed_menus,omitempty"`
}
