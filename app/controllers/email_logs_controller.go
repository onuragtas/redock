package controllers

import (
	"strconv"

	"redock/email_server"

	"github.com/gofiber/fiber/v2"
)

// GetEmailLogs returns the recent mail traffic — what came in, what went out,
// what was rejected — parsed from the mail server's log.
// @Summary mail traffic log
// @Tags Email
// @Security ApiKeyAuth
// @Produce json
// @Router /email/logs [get]
func GetEmailLogs(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Email server not initialized",
		})
	}

	query := email_server.MailLogQuery{
		Tail:      atoiDefault(c.Query("tail"), 0),
		Limit:     atoiDefault(c.Query("limit"), 0),
		Direction: c.Query("direction"),
		Status:    c.Query("status"),
		Search:    c.Query("search"),
	}

	result, err := manager.GetMailLogs(query)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"data":  result,
	})
}

// GetEmailRawLogs returns the unparsed tail of the mail log, for when the
// parsed view is not enough to explain what happened.
// @Summary raw mail log
// @Tags Email
// @Security ApiKeyAuth
// @Produce json
// @Router /email/logs/raw [get]
func GetEmailRawLogs(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Email server not initialized",
		})
	}

	lines, source, err := manager.GetRawMailLog(atoiDefault(c.Query("tail"), 0))
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"data": fiber.Map{
			"lines":  lines,
			"source": source,
		},
	})
}

// atoiDefault parses a query parameter, falling back when it is absent or junk.
func atoiDefault(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return n
}
