package middleware

import (
	"strings"

	"redock/app/models"
	"redock/pkg/repository"
	"redock/pkg/utils"
	"redock/platform/database"
	"redock/platform/memory"

	"github.com/gofiber/fiber/v2"

	jwtMiddleware "github.com/gofiber/contrib/jwt"
)

// JWTProtected func for specify routes group with JWT authentication.
// See: https://github.com/gofiber/contrib/jwt
func JWTProtected() func(*fiber.Ctx) error {
	// Create config for JWT authentication middleware.
	config := jwtMiddleware.Config{
		SigningKey:   jwtMiddleware.SigningKey{Key: utils.GetJWTSecretKey()},
		ContextKey:   "jwt", // used in private routes
		ErrorHandler: jwtError,
	}

	return jwtMiddleware.New(config)
}

func jwtError(c *fiber.Ctx, err error) error {
	// Return status 401 and failed authentication error.
	if err.Error() == "Missing or malformed JWT" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Return status 401 and failed authentication error.
	return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
		"error": true,
		"msg":   err.Error(),
	})
}

// AdminOnly requires JWT + admin role. JWTProtected() ile birlikte kullanılmalı (önce JWT, sonra AdminOnly).
func AdminOnly() func(*fiber.Ctx) error {
	return func(c *fiber.Ctx) error {
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
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": true,
				"msg":   "user not found",
			})
		}
		if userPtr.UserRole != repository.AdminRoleName {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": true,
				"msg":   "sadece admin bu işlemi yapabilir",
			})
		}
		return c.Next()
	}
}

// WebSocketAccessToken requires a valid access token for WebSocket upgrade.
// Token can be sent as query param "token" or "access_token", or as "Authorization: Bearer <token>".
func WebSocketAccessToken() func(*fiber.Ctx) error {
	return func(c *fiber.Ctx) error {
		tokenStr := c.Query("token")
		if tokenStr == "" {
			tokenStr = c.Query("access_token")
		}
		if tokenStr == "" {
			tokenStr = strings.TrimPrefix(c.Get("Authorization"), "Bearer ")
		}
		if _, err := utils.VerifyAccessTokenString(tokenStr); err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": true,
				"msg":   "invalid or missing access token",
			})
		}
		return c.Next()
	}
}
