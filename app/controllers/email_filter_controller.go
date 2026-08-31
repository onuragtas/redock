package controllers

import (
	"strconv"

	"redock/email_server"

	"github.com/gofiber/fiber/v2"
)

// Filters are a mailbox owner's sorting rules, applied as mail arrives.

// GetEmailFilters lists a mailbox's rules in the order they run.
// @Summary list mailbox filters
// @Tags Email
// @Security ApiKeyAuth
// @Produce json
// @Router /email/mailboxes/{mailbox_id}/filters [get]
func GetEmailFilters(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true, "msg": "Email server not initialized",
		})
	}

	mailboxID, err := strconv.ParseUint(c.Params("mailbox_id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "invalid mailbox id"})
	}
	return c.JSON(fiber.Map{"error": false, "data": manager.ListFilters(uint(mailboxID))})
}

// AddEmailFilter creates a rule.
// @Summary create a mailbox filter
// @Tags Email
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Router /email/mailboxes/{mailbox_id}/filters [post]
func AddEmailFilter(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true, "msg": "Email server not initialized",
		})
	}

	mailboxID, err := strconv.ParseUint(c.Params("mailbox_id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "invalid mailbox id"})
	}

	filter := &email_server.EmailFilter{MailboxID: uint(mailboxID), Enabled: true}
	if err := c.BodyParser(filter); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	filter.MailboxID = uint(mailboxID)

	created, err := manager.AddFilter(filter)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	return c.JSON(fiber.Map{"error": false, "data": created})
}

// UpdateEmailFilter replaces a rule.
// @Summary update a mailbox filter
// @Tags Email
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Router /email/filters/{id} [put]
func UpdateEmailFilter(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true, "msg": "Email server not initialized",
		})
	}

	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "invalid filter id"})
	}

	updated := &email_server.EmailFilter{}
	if err := c.BodyParser(updated); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}

	filter, err := manager.UpdateFilter(uint(id), updated)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	return c.JSON(fiber.Map{"error": false, "data": filter})
}

// DeleteEmailFilter removes a rule.
// @Summary delete a mailbox filter
// @Tags Email
// @Security ApiKeyAuth
// @Produce json
// @Router /email/filters/{id} [delete]
func DeleteEmailFilter(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true, "msg": "Email server not initialized",
		})
	}

	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "invalid filter id"})
	}

	if err := manager.DeleteFilter(uint(id)); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	return c.JSON(fiber.Map{"error": false, "msg": "filter deleted"})
}
