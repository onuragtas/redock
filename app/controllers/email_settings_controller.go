package controllers

import (
	"fmt"
	"log"
	"redock/email_server"
	"redock/platform/memory"

	"github.com/gofiber/fiber/v2"
)

// UpdateServerIP updates the server's public IP address
func UpdateServerIP(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Email server not initialized",
		})
	}

	var req struct {
		IPAddress string `json:"ip_address" validate:"required"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid request body: " + err.Error(),
		})
	}

	if req.IPAddress == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "IP address is required",
		})
	}

	// Update server config
	db := manager.GetDB()
	config := manager.GetConfig()
	if config == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "Server config not found",
		})
	}

	config.IPAddress = req.IPAddress
	if err := memory.Update[*email_server.EmailServerConfig](db, "email_server_configs", config); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to update IP address: " + err.Error(),
		})
	}

	// Update SPF records for all domains and trigger DNS update
	domains := memory.FindAll[*email_server.EmailDomain](db, "email_domains")
	for _, domain := range domains {
		domain.SPFRecord = fmt.Sprintf("v=spf1 ip4:%s ~all", req.IPAddress)
		if err := memory.Update[*email_server.EmailDomain](db, "email_domains", domain); err != nil {
			log.Printf("⚠️  Failed to update SPF for %s: %v", domain.Domain, err)
			continue
		}

		// Re-publish the domain's records with the new server address.
		manager.SyncDomainDNSAsync(domain)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   fmt.Sprintf("IP address updated to %s and DNS records queued for update", req.IPAddress),
		"data":  config,
	})
}
