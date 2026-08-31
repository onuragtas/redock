package controllers

import (
	"strconv"
	"strings"

	"redock/email_server"
	"redock/platform/memory"

	"github.com/gofiber/fiber/v2"
)

// Aliases route mail for one address into another mailbox. The delivery path
// has always resolved them; these endpoints are what let anyone create one
// without editing the database by hand.

// GetEmailAliases lists the aliases, optionally filtered by domain.
// @Summary list aliases
// @Tags Email
// @Security ApiKeyAuth
// @Produce json
// @Router /email/aliases [get]
func GetEmailAliases(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true, "msg": "Email server not initialized",
		})
	}

	db := manager.GetDB()
	aliases := memory.Filter[*email_server.EmailAlias](db, "email_aliases", func(a *email_server.EmailAlias) bool {
		if a.IsDeleted() {
			return false
		}
		if domainID := c.Query("domain_id"); domainID != "" {
			if id, err := strconv.ParseUint(domainID, 10, 32); err == nil {
				return a.DomainID == uint(id)
			}
		}
		return true
	})

	// Report the destination address alongside the id, so the dashboard does not
	// have to resolve every mailbox itself.
	out := make([]fiber.Map, 0, len(aliases))
	for _, alias := range aliases {
		destination := alias.Destination
		if alias.DestinationID != 0 {
			if mb, err := memory.FindByID[*email_server.EmailMailbox](db, "email_mailboxes", alias.DestinationID); err == nil && mb != nil {
				destination = mb.Email
			}
		}
		out = append(out, fiber.Map{
			"id":             alias.ID,
			"domain_id":      alias.DomainID,
			"alias":          alias.Alias,
			"destination":    destination,
			"destination_id": alias.DestinationID,
			"enabled":        alias.Enabled,
			"created_at":     alias.CreatedAt,
		})
	}
	return c.JSON(fiber.Map{"error": false, "data": out})
}

// AddEmailAlias creates an alias.
// @Summary create an alias
// @Tags Email
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Router /email/aliases [post]
func AddEmailAlias(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true, "msg": "Email server not initialized",
		})
	}

	var req struct {
		Alias       string `json:"alias"`
		Destination string `json:"destination"`
		Enabled     *bool  `json:"enabled"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}

	alias, err := manager.AddAlias(req.Alias, req.Destination, req.Enabled == nil || *req.Enabled)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	return c.JSON(fiber.Map{"error": false, "data": alias})
}

// UpdateEmailAlias changes an alias's destination or enabled state.
// @Summary update an alias
// @Tags Email
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Router /email/aliases/{id} [put]
func UpdateEmailAlias(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true, "msg": "Email server not initialized",
		})
	}

	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "invalid alias id"})
	}

	var req struct {
		Destination string `json:"destination"`
		Enabled     *bool  `json:"enabled"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}

	alias, err := manager.UpdateAlias(uint(id), strings.TrimSpace(req.Destination), req.Enabled)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	return c.JSON(fiber.Map{"error": false, "data": alias})
}

// DeleteEmailAlias removes an alias.
// @Summary delete an alias
// @Tags Email
// @Security ApiKeyAuth
// @Produce json
// @Router /email/aliases/{id} [delete]
func DeleteEmailAlias(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true, "msg": "Email server not initialized",
		})
	}

	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "invalid alias id"})
	}

	if err := manager.DeleteAlias(uint(id)); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	return c.JSON(fiber.Map{"error": false, "msg": "alias deleted"})
}

// GetEmailBlockedClients lists the addresses the guard is refusing.
// @Summary list blocked clients
// @Tags Email
// @Security ApiKeyAuth
// @Produce json
// @Router /email/blocked [get]
func GetEmailBlockedClients(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true, "msg": "Email server not initialized",
		})
	}
	return c.JSON(fiber.Map{"error": false, "data": manager.BlockedClients()})
}

// BlockEmailClient blocks an address by hand.
// @Summary block a client
// @Tags Email
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Router /email/blocked [post]
func BlockEmailClient(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true, "msg": "Email server not initialized",
		})
	}

	var req struct {
		IP      string `json:"ip"`
		Reason  string `json:"reason"`
		Minutes int    `json:"minutes"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}

	entry, err := manager.BlockClient(req.IP, req.Reason, req.Minutes)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	return c.JSON(fiber.Map{"error": false, "data": entry})
}

// UnblockEmailClient lifts a block.
// @Summary unblock a client
// @Tags Email
// @Security ApiKeyAuth
// @Produce json
// @Router /email/blocked/{ip} [delete]
func UnblockEmailClient(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true, "msg": "Email server not initialized",
		})
	}

	if err := manager.UnblockClient(c.Params("ip")); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	return c.JSON(fiber.Map{"error": false, "msg": "unblocked"})
}
