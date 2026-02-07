package controllers

import (
	"redock/app/models"
	"redock/pkg/repository"
	"redock/pkg/utils"
	"redock/platform/database"
	"redock/platform/memory"

	"github.com/gofiber/fiber/v2"
)

// ListUsers returns all users (admin only). Şifreler dönmez.
// @Summary list users (admin)
// @Tags Users
// @Security ApiKeyAuth
// @Produce json
// @Router /v1/users [get]
func ListUsers(c *fiber.Ctx) error {
	db := database.GetMemoryDB()
	if db == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "database not initialized",
		})
	}
	users := memory.FindAll[*models.User](db, "users")
	list := make([]fiber.Map, 0, len(users))
	for _, u := range users {
		list = append(list, fiber.Map{
			"id":             u.ID,
			"email":          u.Email,
			"user_status":    u.UserStatus,
			"user_role":      u.UserRole,
			"allowed_menus":  u.AllowedMenus,
			"created_at":     u.CreatedAt,
		})
	}
	return c.JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  list,
	})
}

// CreateUser creates a new user (admin only).
// @Summary create user (admin)
// @Tags Users
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param body body models.CreateUserRequest true "body"
// @Router /v1/users [post]
func CreateUser(c *fiber.Ctx) error {
	var req models.CreateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	validate := utils.NewValidator()
	if err := validate.Struct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   utils.ValidatorErrors(err),
		})
	}
	if _, err := utils.VerifyRole(req.UserRole); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "user_role admin veya user olmalı",
		})
	}

	db := database.GetMemoryDB()
	if db == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "database not initialized",
		})
	}
	existing := memory.Where[*models.User](db, "users", "Email", req.Email)
	if len(existing) > 0 {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"error": true,
			"msg":   "bu e-posta zaten kayıtlı",
		})
	}

	user := &models.User{
		Email:        req.Email,
		PasswordHash: utils.GeneratePassword(req.Password),
		UserStatus:   1,
		UserRole:     req.UserRole,
		AllowedMenus: req.AllowedMenus,
	}
	if err := memory.Create(db, "users", user); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data": fiber.Map{
			"id":             user.ID,
			"email":          user.Email,
			"user_status":    user.UserStatus,
			"user_role":      user.UserRole,
			"allowed_menus":  user.AllowedMenus,
			"created_at":     user.CreatedAt,
		},
	})
}

// UpdateUser updates a user (admin only).
// @Summary update user (admin)
// @Tags Users
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param id path int true "User ID"
// @Param body body models.UpdateUserRequest true "body"
// @Router /v1/users/{id} [put]
func UpdateUser(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil || id < 1 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "geçersiz kullanıcı id",
		})
	}
	var req models.UpdateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	validate := utils.NewValidator()
	if err := validate.Struct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   utils.ValidatorErrors(err),
		})
	}

	db := database.GetMemoryDB()
	if db == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "database not initialized",
		})
	}
	userPtr, err := memory.FindByID[*models.User](db, "users", uint(id))
	if err != nil || userPtr == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "kullanıcı bulunamadı",
		})
	}
	user := *userPtr

	if req.UserRole != nil {
		if _, err := utils.VerifyRole(*req.UserRole); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": true,
				"msg":   "user_role admin veya user olmalı",
			})
		}
		user.UserRole = *req.UserRole
	}
	if req.UserStatus != nil {
		user.UserStatus = *req.UserStatus
	}
	if req.AllowedMenus != nil {
		user.AllowedMenus = req.AllowedMenus
	}

	if err := memory.Update(db, "users", &user); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	return c.JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data": fiber.Map{
			"id":             user.ID,
			"email":          user.Email,
			"user_status":    user.UserStatus,
			"user_role":      user.UserRole,
			"allowed_menus":  user.AllowedMenus,
			"updated_at":     user.UpdatedAt,
		},
	})
}

// DeleteUser deletes a user (admin only).
// @Summary delete user (admin)
// @Tags Users
// @Security ApiKeyAuth
// @Param id path int true "User ID"
// @Router /v1/users/{id} [delete]
func DeleteUser(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil || id < 1 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "geçersiz kullanıcı id",
		})
	}

	db := database.GetMemoryDB()
	if db == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "database not initialized",
		})
	}
	_, err = memory.FindByID[*models.User](db, "users", uint(id))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "kullanıcı bulunamadı",
		})
	}

	if err := memory.Delete[*models.User](db, "users", uint(id)); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// GetMenuOptions returns all menu items (path, name, icon) for admin user form (admin only).
// @Summary get menu options for user form (admin)
// @Tags Users
// @Security ApiKeyAuth
// @Produce json
// @Router /v1/users/menu-options [get]
func GetMenuOptions(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  repository.AllMenuItems,
	})
}
