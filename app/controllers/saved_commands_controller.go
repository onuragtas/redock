package controllers

import (
	"redock/saved_commands"

	"github.com/gofiber/fiber/v2"
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
func GetSavedCommands(c *fiber.Ctx) error {
	list := saved_commands.GetManager().GetList()

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  list,
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
func AddCommand(c *fiber.Ctx) error {
	model := &saved_commands.Model{}
	// Checking received data from JSON body.
	if err := c.BodyParser(model); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	saved_commands.GetManager().Add(model)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  fiber.Map{},
	})
}

// RemoveCommandByID removes a saved command by ID (RESTful)
// @Description Remove a saved command by ID
// @Summary remove a saved command
// @Tags SavedCommands
// @Produce json
// @Param id path int true "Command ID"
// @Success 200 {object} map[string]interface{}
// @Router /v1/saved_commands/{id} [delete]
func RemoveCommandByID(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid ID parameter",
		})
	}

	if err := saved_commands.GetManager().DeleteByID(uint(id)); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Command deleted successfully",
		"data":  fiber.Map{},
	})
}

// UpdateCommandByID updates a saved command by ID (RESTful)
// @Description Update a saved command by ID
// @Summary update a saved command
// @Tags SavedCommands
// @Accept json
// @Produce json
// @Param id path int true "Command ID"
// @Success 200 {object} map[string]interface{}
// @Router /v1/saved_commands/{id} [put]
func UpdateCommandByID(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid ID parameter",
		})
	}

	model := &saved_commands.Model{}
	if err := c.BodyParser(model); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	if err := saved_commands.GetManager().UpdateByID(uint(id), model.Command, model.Path); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Command updated successfully",
		"data":  fiber.Map{},
	})
}

// GetCommandByID gets a saved command by ID (RESTful)
// @Description Get a saved command by ID
// @Summary get a saved command
// @Tags SavedCommands
// @Produce json
// @Param id path int true "Command ID"
// @Success 200 {object} map[string]interface{}
// @Router /v1/saved_commands/{id} [get]
func GetCommandByID(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid ID parameter",
		})
	}

	cmd, err := saved_commands.GetManager().GetByID(uint(id))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "Command not found",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  cmd,
	})
}
