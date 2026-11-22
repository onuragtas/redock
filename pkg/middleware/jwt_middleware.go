package middleware

import (
	"strings"

	"redock/app/models"
	"redock/pkg/repository"
	"redock/pkg/utils"
	"redock/platform/database"
	"redock/platform/memory"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"

	jwtMiddleware "github.com/gofiber/contrib/jwt"
)

// JWTProtected func for specify routes group with JWT authentication.
// See: https://github.com/gofiber/contrib/jwt
//
// A valid signature is not enough: the token must have been issued by the panel ("iss": "redock") and
// its "id" must be a panel user that still exists. Tokens of other kinds (the tunnel server's,
// "iss": "tunnel") once shared the signing key, and their ids are tunnel-user ids, not panel ones;
// accepting them gave anyone who could register a tunnel account the whole panel (2026-11-21).
func JWTProtected() func(*fiber.Ctx) error {
	// Create config for JWT authentication middleware.
	config := jwtMiddleware.Config{
		SigningKey:     jwtMiddleware.SigningKey{JWTAlg: jwtMiddleware.HS256, Key: utils.GetJWTSecretKey()},
		ContextKey:     "jwt", // used in private routes
		ErrorHandler:   jwtError,
		SuccessHandler: panelUserOnly,
	}

	return jwtMiddleware.New(config)
}

// panelUserOnly runs after the signature and expiry checks of JWTProtected.
func panelUserOnly(c *fiber.Ctx) error {
	token, _ := c.Locals("jwt").(*jwt.Token)
	if token == nil {
		return unauthorized(c, "invalid token")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !utils.IsPanelToken(claims) {
		return unauthorized(c, "invalid token")
	}
	id, ok := utils.ClaimUserID(claims)
	if !ok {
		return unauthorized(c, "invalid token")
	}
	if user, status, msg := panelUser(id); user == nil {
		return c.Status(status).JSON(fiber.Map{"error": true, "msg": msg})
	}
	return c.Next()
}

// panelUser looks a token's user up; when it is not there, the answer to give (status, message).
func panelUser(id int) (*models.User, int, string) {
	db := database.GetMemoryDB()
	if db == nil {
		return nil, fiber.StatusInternalServerError, "database not initialized"
	}
	if id <= 0 {
		return nil, fiber.StatusUnauthorized, "invalid token"
	}
	user, err := memory.FindByID[*models.User](db, "users", uint(id))
	if err != nil || user == nil {
		return nil, fiber.StatusUnauthorized, "user not found"
	}
	return user, 0, ""
}

func unauthorized(c *fiber.Ctx, msg string) error {
	return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": true, "msg": msg})
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
			return unauthorized(c, err.Error())
		}
		return requireAdmin(c, claims.UserID)
	}
}

func requireAdmin(c *fiber.Ctx, id int) error {
	user, status, msg := panelUser(id)
	if user == nil {
		if status == fiber.StatusUnauthorized {
			status = fiber.StatusForbidden
		}
		return c.Status(status).JSON(fiber.Map{"error": true, "msg": msg})
	}
	if user.UserRole != repository.AdminRoleName {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": true,
			"msg":   "only admin can perform this action",
		})
	}
	return c.Next()
}

// WebSocketAccessToken requires a valid panel access token of an admin for WebSocket upgrade: /ws opens a
// shell (on the host, or in a container) and /ws/traffic streams every request — neither is for other users.
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
		token, err := utils.VerifyAccessTokenString(tokenStr)
		if err != nil {
			return unauthorized(c, "invalid or missing access token")
		}
		claims, _ := token.Claims.(jwt.MapClaims)
		id, ok := utils.ClaimUserID(claims)
		if !ok {
			return unauthorized(c, "invalid or missing access token")
		}
		return requireAdmin(c, id)
	}
}
