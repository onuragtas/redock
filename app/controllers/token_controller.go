package controllers

import (
	"redock/app/models"
	"redock/pkg/utils"
	"redock/platform/database"
	"redock/platform/memory"

	"github.com/gofiber/fiber/v2"
)

// RenewTokens method for renew access and refresh tokens.
// Sadece refresh_token gerekir; access token expire olsa da renew çalışır. Önemli olan refresh token'ın expire olmaması ve kullanıcıya bağlı olması.
// @Description Renew access and refresh tokens.
// @Summary renew access and refresh tokens
// @Tags Token
// @Accept json
// @Produce json
// @Param refresh_token body string true "Refresh token"
// @Success 200 {string} status "ok"
// @Router /v1/token/renew [post]
func RenewTokens(c *fiber.Ctx) error {
	renew := &models.Renew{}
	if err := c.BodyParser(renew); err != nil {
		// Return, if JSON data is not correct.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Refresh token'ı doğrula (imza + süre); user_id döner. Expire veya hatalıysa 401.
	userID, err := utils.ParseRefreshToken(renew.RefreshToken)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "unauthorized, your session was ended earlier",
		})
	}

	db := database.GetMemoryDB()
	if db == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "database not initialized",
		})
	}

	userPtr, err := memory.FindByID[*models.User](db, "users", uint(userID))
	if err != nil || userPtr == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "user with the given ID is not found",
		})
	}
	foundedUser := *userPtr

	credentials, err := utils.GetCredentialsByRole(foundedUser.UserRole)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	tokens, err := utils.GenerateNewTokens(userID, credentials)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"tokens": fiber.Map{
			"access":  tokens.Access,
			"refresh": tokens.Refresh,
		},
	})
}
