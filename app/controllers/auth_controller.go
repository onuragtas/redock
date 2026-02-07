package controllers

import (
	"redock/app/models"
	"redock/pkg/repository"
	"redock/pkg/utils"
	"redock/platform/database"
	"redock/platform/memory"

	"github.com/gofiber/fiber/v2"
)

// UserSignUp method to create the first user only (always admin). Sonraki kullanıcılar sadece admin panelinden eklenir.
// @Description Create the first user (admin). Further users must be added by an admin.
// @Summary create first user (admin)
// @Tags User
// @Accept json
// @Produce json
// @Param email body string true "Email"
// @Param password body string true "Password"
// @Success 200 {object} models.User
// @Router /v1/user/sign/up [post]
func UserSignUp(c *fiber.Ctx) error {
	signUp := &models.SignUp{}

	if err := c.BodyParser(signUp); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	validate := utils.NewValidator()
	if err := validate.Struct(signUp); err != nil {
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

	existing := memory.FindAll[*models.User](db, "users")
	if len(existing) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "New user can only be added by an existing admin",
		})
	}

	user := &models.User{}
	user.Email = signUp.Email
	user.PasswordHash = utils.GeneratePassword(signUp.Password)
	user.UserStatus = 1
	user.UserRole = repository.AdminRoleName // İlk kullanıcı her zaman admin

	if err := validate.Struct(user); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   utils.ValidatorErrors(err),
		})
	}

	if err := memory.Create(db, "users", user); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	responseUser := *user
	responseUser.PasswordHash = ""

	return c.JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"user":  responseUser,
	})
}

// UserSignIn method to auth user and return access and refresh tokens.
// @Description Auth user and return access and refresh token.
// @Summary auth user and return access and refresh token
// @Tags User
// @Accept json
// @Produce json
// @Param email body string true "User Email"
// @Param password body string true "User Password"
// @Success 200 {string} status "ok"
// @Router /v1/user/sign/in [post]
func UserSignIn(c *fiber.Ctx) error {
	// Create a new user auth struct.
	signIn := &models.SignIn{}

	// Checking received data from JSON body.
	if err := c.BodyParser(signIn); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	db := database.GetMemoryDB()
	if db == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "database not initialized",
		})
	}

	// Get user by email.
	users := memory.Where[*models.User](db, "users", "Email", signIn.Email)
	if len(users) == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "user with the given email is not found",
		})
	}
	foundedUser := *users[0]

	// Compare given user password with stored in found user.
	compareUserPassword := utils.ComparePasswords(foundedUser.PasswordHash, signIn.Password)
	if !compareUserPassword {
		// Return, if password is not compare to stored in database.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "wrong user email address or password",
		})
	}

	// Get role credentials from founded user.
	credentials, err := utils.GetCredentialsByRole(foundedUser.UserRole)
	if err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Generate a new pair of access and refresh tokens.
	tokens, err := utils.GenerateNewTokens(int(foundedUser.ID), credentials)
	if err != nil {
		// Return status 500 and token generation error.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"tokens": fiber.Map{
			"access":  tokens.Access,
			"refresh": tokens.Refresh,
		},
	})
}

// UserSignOut method to de-authorize user and delete refresh token from Redis.
// @Description De-authorize user and delete refresh token from Redis.
// @Summary de-authorize user and delete refresh token from Redis
// @Tags User
// @Accept json
// @Produce json
// @Success 204 {string} status "ok"
// @Security ApiKeyAuth
// @Router /v1/user/sign/out [post]
func UserSignOut(c *fiber.Ctx) error {
	// Return status 204 no content.
	return c.SendStatus(fiber.StatusNoContent)
}

// AuthSetup returns whether any user exists (for register visibility on login page).
// @Description Returns has_any_user for login page register visibility.
// @Summary auth setup - has any user
// @Tags Auth
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /v1/auth/setup [get]
func AuthSetup(c *fiber.Ctx) error {
	db := database.GetMemoryDB()
	if db == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "database not initialized",
		})
	}
	hasAnyUser := len(memory.FindAll[*models.User](db, "users")) > 0
	return c.JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data": fiber.Map{
			"has_any_user": hasAnyUser,
		},
	})
}

// AuthMe returns the current user from JWT (for "girişli mi?" check).
// @Description Returns current user from JWT.
// @Summary auth me - current user
// @Tags Auth
// @Security ApiKeyAuth
// @Produce json
// @Success 200 {object} models.User
// @Router /v1/auth/me [get]
func AuthMe(c *fiber.Ctx) error {
	claims, err := utils.ExtractTokenMetadata(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	db := database.GetMemoryDB()
	if db == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "database not initialized",
		})
	}
	userPtr, err := memory.FindByID[*models.User](db, "users", uint(claims.UserID))
	if err != nil || userPtr == nil || userPtr.Email == "" {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "user not found",
		})
	}
	user := *userPtr
	user.PasswordHash = ""

	allowedMenus := user.AllowedMenus
	if user.UserRole == repository.AdminRoleName {
		allowedMenus = repository.AllMenuPaths
	} else if len(allowedMenus) == 0 {
		allowedMenus = repository.DefaultUserMenuPaths
	}

	return c.JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data": fiber.Map{
			"id":            user.ID,
			"email":         user.Email,
			"user_status":   user.UserStatus,
			"user_role":     user.UserRole,
			"allowed_menus": allowedMenus,
		},
	})
}

// Menus returns menu items for the current user (allowed paths + name + icon). Frontend sadece route tanımlar, menü verisi backend'den.
// @Summary get menus for current user
// @Tags Auth
// @Security ApiKeyAuth
// @Produce json
// @Router /v1/menus [get]
func Menus(c *fiber.Ctx) error {
	claims, err := utils.ExtractTokenMetadata(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	db := database.GetMemoryDB()
	if db == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "database not initialized",
		})
	}
	userPtr, err := memory.FindByID[*models.User](db, "users", uint(claims.UserID))
	if err != nil || userPtr == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "user not found",
		})
	}

	users := memory.FindAll[*models.User](db, "users")

	if len(users) == 1 {
		userPtr.UserRole = repository.AdminRoleName
	}

	items := repository.GetMenuItemsForUser(userPtr.UserRole, userPtr.AllowedMenus)
	return c.JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  items,
	})
}
