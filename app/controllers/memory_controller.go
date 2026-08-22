package controllers

import (
	"redock/platform/database"
	"redock/platform/memguard"

	"github.com/gofiber/fiber/v2"
)

// MemoryStatus returns the current memory picture: budget, pressure level,
// runtime counters, registered relievers and the in-memory DB table sizes.
// @Summary memory guard status
// @Tags System
// @Security ApiKeyAuth
// @Produce json
// @Router /v1/system/memory [get]
func MemoryStatus(c *fiber.Ctx) error {
	status := memguard.Get().Snapshot()

	var tables interface{}
	if db := database.GetMemoryDB(); db != nil {
		tables = db.Tables()
	}

	return c.JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data": fiber.Map{
			"status": status,
			"tables": tables,
		},
	})
}

// MemoryHistory returns the sampled memory usage series for the chart.
// @Summary memory usage history
// @Tags System
// @Security ApiKeyAuth
// @Produce json
// @Router /v1/system/memory/history [get]
func MemoryHistory(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  memguard.Get().History(),
	})
}

// MemoryEvents returns what the guard has done recently (level changes,
// relief sweeps, per-reliever results).
// @Summary memory guard events
// @Tags System
// @Security ApiKeyAuth
// @Produce json
// @Router /v1/system/memory/events [get]
func MemoryEvents(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  memguard.Get().Events(),
	})
}

// MemoryUpdateConfig persists and applies a new memory guard configuration.
// @Summary update memory guard config
// @Tags System
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Router /v1/system/memory/config [put]
func MemoryUpdateConfig(c *fiber.Ctx) error {
	// Start from the live config so a partial payload only changes what it sets.
	cfg := memguard.Get().Config()
	if err := c.BodyParser(&cfg); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	applied, err := memguard.Get().UpdateConfig(cfg)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  applied,
	})
}

// MemoryRelease runs a relief sweep on demand. The optional "level" body field
// ("warning" | "critical" | "emergency") decides how aggressive it is;
// "critical" is the default.
// @Summary release memory now
// @Tags System
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Router /v1/system/memory/release [post]
func MemoryRelease(c *fiber.Ctx) error {
	body := struct {
		Level string `json:"level"`
	}{}
	_ = c.BodyParser(&body)

	level := memguard.LevelCritical
	if body.Level != "" {
		level = memguard.ParseLevel(body.Level)
		if level == memguard.LevelNormal {
			level = memguard.LevelCritical
		}
	}

	results := memguard.Get().ReleaseNow(level)

	return c.JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data": fiber.Map{
			"level":   level,
			"results": results,
			"status":  memguard.Get().Snapshot(),
		},
	})
}
