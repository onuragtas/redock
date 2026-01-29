package controllers

import (
	"fmt"
	"log"
	"redock/cloudflare"
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

		// Update DNS in background
		go func(d *email_server.EmailDomain) {
			cfManager := cloudflare.GetManager()
			if cfManager == nil {
				return
			}

			zones := memory.Filter[*cloudflare.CloudflareZone](db, "cloudflare_zones", func(z *cloudflare.CloudflareZone) bool {
				return z.Name == d.Domain
			})

			if len(zones) == 0 {
				return
			}

			zone := zones[0]
			params := cloudflare.EmailDNSParams{
				MXRecord:       "mail." + d.Domain,
				SPFRecord:      d.SPFRecord,
				DKIMSelector:   d.DKIMSelector,
				DKIMRecord:     d.DKIMPublicKey,
				DMARCRecord:    d.DMARCRecord,
				MailServerIP:   req.IPAddress,
			}

			if err := cfManager.UpdateEmailDNSRecords(zone.ZoneID, params); err != nil {
				log.Printf("⚠️  Failed to update Cloudflare DNS for %s: %v", d.Domain, err)
			}
		}(domain)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   fmt.Sprintf("IP address updated to %s and DNS records queued for update", req.IPAddress),
		"data":  config,
	})
}

// GetServerConfig returns the current server configuration
func GetServerConfig(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Email server not initialized",
		})
	}

	config := manager.GetConfig()

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"data":  config,
	})
}

