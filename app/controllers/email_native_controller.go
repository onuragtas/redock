package controllers

import (
	"redock/email_server"
	"redock/platform/memory"

	"github.com/gofiber/fiber/v2"
)

// GetEmailEngine reports which engine is active (docker-mailserver or the
// built-in native servers) plus the native listener/queue status.
// @Summary mail engine status
// @Tags Email
// @Security ApiKeyAuth
// @Produce json
// @Router /email/engine [get]
func GetEmailEngine(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Email server not initialized",
		})
	}

	return c.JSON(fiber.Map{
		"error": false,
		"data": fiber.Map{
			"native":    manager.Native().Status(),
			"config":    manager.GetConfig(),
			"self_test": manager.NativeSelfTest(),
		},
	})
}

// ControlEmailServer starts, stops or restarts the mail listeners.
// @Summary control the mail server
// @Tags Email
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Router /email/control [post]
func ControlEmailServer(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Email server not initialized",
		})
	}

	body := struct {
		Action string `json:"action"`
	}{}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}

	var err error
	switch body.Action {
	case "start":
		err = manager.StartServer()
	case "stop":
		err = manager.StopServer()
	case "restart":
		err = manager.RestartServer()
	default:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "action must be start, stop or restart",
		})
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}

	return c.JSON(fiber.Map{
		"error": false,
		"data": fiber.Map{
			"native": manager.Native().Status(),
			"config": manager.GetConfig(),
		},
	})
}

// UpdateEmailNativeSettings persists the native engine's listener and policy
// toggles.
// @Summary update native mail settings
// @Tags Email
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Router /email/native/settings [put]
func UpdateEmailNativeSettings(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Email server not initialized",
		})
	}

	config := manager.GetConfig()
	if config == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "msg": "no configuration"})
	}

	// Start from the live config so a partial payload only changes what it sets.
	updated := *config
	if err := c.BodyParser(&updated); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}

	applied, err := manager.UpdateNativeSettings(updated)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}

	return c.JSON(fiber.Map{"error": false, "data": applied})
}

// GetEmailQueue lists the outbound queue.
// @Summary outbound mail queue
// @Tags Email
// @Security ApiKeyAuth
// @Produce json
// @Router /email/queue [get]
func GetEmailQueue(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Email server not initialized",
		})
	}

	return c.JSON(fiber.Map{"error": false, "data": manager.QueueItems()})
}

// FlushEmailQueue retries every queued message immediately.
// @Summary flush outbound queue
// @Tags Email
// @Security ApiKeyAuth
// @Produce json
// @Router /email/queue/flush [post]
func FlushEmailQueue(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Email server not initialized",
		})
	}

	return c.JSON(fiber.Map{"error": false, "data": fiber.Map{"flushed": manager.FlushQueue()}})
}

// DeleteEmailQueueItem drops one queued message.
// @Summary delete queued message
// @Tags Email
// @Security ApiKeyAuth
// @Produce json
// @Router /email/queue/{id} [delete]
func DeleteEmailQueueItem(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Email server not initialized",
		})
	}

	if err := manager.DeleteQueueItem(c.Params("id")); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	return c.JSON(fiber.Map{"error": false, "msg": "deleted"})
}

// GetEmailDNSRecords returns the DNS records each domain needs for mail to
// work: MX, SPF, DKIM and DMARC. With the native engine these are the operator's
// remaining manual step, so the dashboard spells them out.
// @Summary required DNS records
// @Tags Email
// @Security ApiKeyAuth
// @Produce json
// @Router /email/dns-records [get]
func GetEmailDNSRecords(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Email server not initialized",
		})
	}

	db := manager.GetDB()
	if db == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "msg": "database not initialized"})
	}

	domains := memory.FindAll[*email_server.EmailDomain](db, "email_domains")
	out := make([]fiber.Map, 0, len(domains))
	for _, domain := range domains {
		if domain == nil || domain.IsDeleted() {
			continue
		}
		out = append(out, fiber.Map{
			"domain":  domain.Domain,
			"records": manager.RequiredDNSRecords(domain),
		})
	}

	return c.JSON(fiber.Map{"error": false, "data": out})
}

// SyncEmailDNS publishes the mail DNS records of one domain (or every domain)
// to Cloudflare. This is the automation the container setup performed on domain
// creation, exposed so it can also be re-run on demand.
// @Summary publish mail DNS records to Cloudflare
// @Tags Email
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Router /email/dns-records/sync [post]
func SyncEmailDNS(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Email server not initialized",
		})
	}

	body := struct {
		DomainID uint `json:"domain_id"`
	}{}
	_ = c.BodyParser(&body)

	if body.DomainID == 0 {
		return c.JSON(fiber.Map{"error": false, "data": manager.SyncAllDomainsDNS()})
	}

	domain, err := memory.FindByID[*email_server.EmailDomain](manager.GetDB(), "email_domains", body.DomainID)
	if err != nil || domain == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": true, "msg": "domain not found"})
	}

	result := manager.SyncDomainDNS(domain)
	if !result.Synced {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": result.Message, "data": result})
	}
	return c.JSON(fiber.Map{"error": false, "data": result})
}

// GetEmailLegacyArtifacts reports what is left over from the retired
// docker-mailserver setup.
// @Summary legacy docker artifacts
// @Tags Email
// @Security ApiKeyAuth
// @Produce json
// @Router /email/legacy [get]
func GetEmailLegacyArtifacts(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Email server not initialized",
		})
	}
	return c.JSON(fiber.Map{"error": false, "data": manager.LegacyArtifacts()})
}

// CleanupEmailLegacyArtifacts deletes the leftover container config directories
// and the old docker volume. Mail itself is never touched.
// @Summary remove legacy docker artifacts
// @Tags Email
// @Security ApiKeyAuth
// @Produce json
// @Router /email/legacy [delete]
func CleanupEmailLegacyArtifacts(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Email server not initialized",
		})
	}
	return c.JSON(fiber.Map{"error": false, "data": manager.CleanupLegacyArtifacts()})
}

// GetEmailConnections returns the recent connection traces: every connection
// the mail server accepted, with its protocol conversation and how it ended.
// This is where an attempt that never became a message — a refused TLS
// handshake, a probe, a bad password — becomes visible.
// @Summary recent mail connections
// @Tags Email
// @Security ApiKeyAuth
// @Produce json
// @Router /email/logs/connections [get]
func GetEmailConnections(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Email server not initialized",
		})
	}

	return c.JSON(fiber.Map{
		"error": false,
		"data":  manager.ConnectionTraces(atoiDefault(c.Query("limit"), 100)),
	})
}

// GetEmailCertificate reports on the mail server's TLS certificate: what it
// covers, what it is missing, and whether a Let's Encrypt request can be made.
// @Summary mail TLS certificate status
// @Tags Email
// @Security ApiKeyAuth
// @Produce json
// @Router /email/certificate [get]
func GetEmailCertificate(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Email server not initialized",
		})
	}
	return c.JSON(fiber.Map{"error": false, "data": manager.CertificateStatus()})
}

// RequestEmailCertificate issues a Let's Encrypt certificate for the mail
// hostname through the API Gateway's ACME account.
// @Summary request a Let's Encrypt certificate for mail
// @Tags Email
// @Security ApiKeyAuth
// @Produce json
// @Router /email/certificate/request [post]
func RequestEmailCertificate(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Email server not initialized",
		})
	}

	status, err := manager.RequestLetsEncryptCertificate()
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
			"data":  status,
		})
	}
	return c.JSON(fiber.Map{"error": false, "data": status})
}

// CheckEmailDeliverability inspects the public DNS and reverse DNS this server
// depends on and reports what a receiving system would see — the things that
// actually decide inbox versus spam.
// @Summary deliverability check
// @Tags Email
// @Security ApiKeyAuth
// @Produce json
// @Router /email/deliverability [get]
func CheckEmailDeliverability(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Email server not initialized",
		})
	}
	return c.JSON(fiber.Map{"error": false, "data": manager.CheckDeliverability()})
}
