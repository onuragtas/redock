package middleware

import (
	"strings"

	"redock/pkg/utils"

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
