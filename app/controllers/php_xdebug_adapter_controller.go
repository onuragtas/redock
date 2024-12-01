package controllers

import (
	"github.com/gofiber/fiber/v2"
	"redock/php_debug_adapter"
)

// GetXDebugAdapterSettings method to create a new user.
// @Description Create a new user.
// @Summary create a new user
// @Tags User
// @Accept json
// @Produce json
// @Param email body string true "Email"
// @Param password body string true "Password"
// @Param user_role body string true "User role"
// @Success 200 {object} models.User
// @Router /v1/docker/env [get]
func GetXDebugAdapterSettings(c *fiber.Ctx) error {
	settings := php_debug_adapter.GetPHPDebugAdapter().Settings()

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  settings,
	})
}

// AddXDebugAdapterListener method to create a new user.
// @Description Create a new user.
// @Summary create a new user
// @Tags User
// @Accept json
// @Produce json
// @Param email body string true "Email"
// @Param password body string true "Password"
// @Param user_role body string true "User role"
// @Success 200 {object} models.User
// @Router /v1/docker/env [get]
func AddXDebugAdapterListener(c *fiber.Ctx) error {
	model := &php_debug_adapter.Mapping{}
	// Checking received data from JSON body.
	if err := c.BodyParser(model); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	php_debug_adapter.GetPHPDebugAdapter().Add(*model)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  fiber.Map{},
	})
}

// RemoveXDebugAdapterListener method to create a new user.
// @Description Create a new user.
// @Summary create a new user
// @Tags User
// @Accept json
// @Produce json
// @Param email body string true "Email"
// @Param password body string true "Password"
// @Param user_role body string true "User role"
// @Success 200 {object} models.User
// @Router /v1/docker/env [get]
func RemoveXDebugAdapterListener(c *fiber.Ctx) error {
	model := &php_debug_adapter.Mapping{}
	// Checking received data from JSON body.
	if err := c.BodyParser(model); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	php_debug_adapter.GetPHPDebugAdapter().Del(model.Path)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  fiber.Map{},
	})
}

// XDebugAdapterStart method to create a new user.
// @Description Create a new user.
// @Summary create a new user
// @Tags User
// @Accept json
// @Produce json
// @Param email body string true "Email"
// @Param password body string true "Password"
// @Param user_role body string true "User role"
// @Success 200 {object} models.User
// @Router /v1/docker/env [get]
func XDebugAdapterStart(c *fiber.Ctx) error {
	go php_debug_adapter.GetPHPDebugAdapter().Start()

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  fiber.Map{},
	})
}

// XDebugAdapterStop method to create a new user.
// @Description Create a new user.
// @Summary create a new user
// @Tags User
// @Accept json
// @Produce json
// @Param email body string true "Email"
// @Param password body string true "Password"
// @Param user_role body string true "User role"
// @Success 200 {object} models.User
// @Router /v1/docker/env [get]
func XDebugAdapterStop(c *fiber.Ctx) error {
	php_debug_adapter.GetPHPDebugAdapter().Stop()

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  fiber.Map{},
	})
}

// UpdateXDebugAdapterConfiguration method to create a new user.
// @Description Create a new user.
// @Summary create a new user
// @Tags User
// @Accept json
// @Produce json
// @Param email body string true "Email"
// @Param password body string true "Password"
// @Param user_role body string true "User role"
// @Success 200 {object} models.User
// @Router /v1/docker/env [get]
func UpdateXDebugAdapterConfiguration(c *fiber.Ctx) error {
	model := &php_debug_adapter.Data{}
	// Checking received data from JSON body.
	if err := c.BodyParser(model); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	php_debug_adapter.GetPHPDebugAdapter().Update(model)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  fiber.Map{},
	})
}
