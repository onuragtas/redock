package controllers

import (
	"redock/notify"
	"redock/platform/database"

	"github.com/gofiber/fiber/v2"
)

// GetNotifySettings returns the alert settings and what has been raised so far.
// @Summary alert settings
// @Tags Notifications
// @Security ApiKeyAuth
// @Produce json
// @Router /system/notifications [get]
func GetNotifySettings(c *fiber.Ctx) error {
	db := database.GetMemoryDB()

	var recent []notify.Alert
	if notifier := notify.Get(); notifier != nil {
		recent = notifier.Recent()
	}

	return c.JSON(fiber.Map{
		"error": false,
		"data": fiber.Map{
			"settings": notify.CurrentSettings(db),
			"recent":   recent,
		},
	})
}

// UpdateNotifySettings saves the alert settings.
// @Summary update alert settings
// @Tags Notifications
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Router /system/notifications [put]
func UpdateNotifySettings(c *fiber.Ctx) error {
	db := database.GetMemoryDB()

	// Start from the live settings so a partial payload only changes what it
	// sets, the way the other settings endpoints behave.
	updated := *notify.CurrentSettings(db)
	if err := c.BodyParser(&updated); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}

	saved, err := notify.Save(db, &updated)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	return c.JSON(fiber.Map{"error": false, "data": saved})
}

// SendNotifyTest delivers a test alert so the address can be confirmed before
// anything depends on it.
// @Summary send a test alert
// @Tags Notifications
// @Security ApiKeyAuth
// @Produce json
// @Router /system/notifications/test [post]
func SendNotifyTest(c *fiber.Ctx) error {
	notifier := notify.Get()
	if notifier == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true, "msg": "notifications are not initialized",
		})
	}

	if err := notifier.SendTest(); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	return c.JSON(fiber.Map{"error": false, "msg": "test sent"})
}

// RunNotifyCheck reads every watched state now instead of waiting for the
// timer, so the page can show what the checks currently see.
// @Summary run the checks now
// @Tags Notifications
// @Security ApiKeyAuth
// @Produce json
// @Router /system/notifications/check [post]
func RunNotifyCheck(c *fiber.Ctx) error {
	notifier := notify.Get()
	if notifier == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true, "msg": "notifications are not initialized",
		})
	}

	return c.JSON(fiber.Map{"error": false, "data": notifier.Check()})
}
