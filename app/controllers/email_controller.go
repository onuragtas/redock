package controllers

import (
	"fmt"
	"log"
	"redock/cloudflare"
	"redock/email_server"
	"redock/platform/memory"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

func GetEmailDomains(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Email server not initialized",
		})
	}

	db := manager.GetDB()
	domains := memory.FindAll[*email_server.EmailDomain](db, "email_domains")

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"data":  domains,
	})
}

func AddEmailDomain(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Email server not initialized",
		})
	}

	var req struct {
		Domain      string `json:"domain" validate:"required"`
		Description string `json:"description"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid request body: " + err.Error(),
		})
	}

	domain, err := manager.AddDomain(req.Domain, req.Description)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to add domain: " + err.Error(),
		})
	}

	go func() {
		cfManager := cloudflare.GetManager()
		if cfManager == nil {
			return
		}
		
		db := manager.GetDB()
		zones := memory.Filter[*cloudflare.CloudflareZone](db, "cloudflare_zones", func(z *cloudflare.CloudflareZone) bool {
			return z.Name == req.Domain
		})
		
		if len(zones) == 0 {
			return
		}
		
		zone := zones[0]
		serverConfig := manager.GetConfig()
		
		params := cloudflare.EmailDNSParams{
			MXRecord:       "mail." + req.Domain,
			SPFRecord:      domain.SPFRecord,
			DKIMSelector:   domain.DKIMSelector,
			DKIMRecord:     domain.DKIMPublicKey,
			DMARCRecord:    domain.DMARCRecord,
			MailServerIP:   serverConfig.IPAddress,
		}
		
		if err := cfManager.CreateEmailDNSRecords(zone.ZoneID, params); err != nil {
			log.Printf("⚠️  Failed to create Cloudflare DNS records: %v", err)
		}
	}()

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Domain added successfully",
		"data":  domain,
	})
}

func UpdateEmailDomain(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Email server not initialized",
		})
	}

	domainID, err := strconv.ParseUint(c.Params("domain_id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid domain ID",
		})
	}

	var req struct {
		Description string `json:"description"`
		Enabled     bool   `json:"enabled"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid request body: " + err.Error(),
		})
	}

	db := manager.GetDB()
	domain, err := memory.FindByID[*email_server.EmailDomain](db, "email_domains", uint(domainID))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "Domain not found",
		})
	}

	domain.Description = req.Description
	domain.Enabled = req.Enabled

	serverIP := manager.GetConfig().IPAddress
	if serverIP == "" {
		serverIP = "127.0.0.1"
	}
	domain.SPFRecord = fmt.Sprintf("v=spf1 ip4:%s ~all", serverIP)

	if err := memory.Update[*email_server.EmailDomain](db, "email_domains", domain); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to update domain: " + err.Error(),
		})
	}

	go func() {
		cfManager := cloudflare.GetManager()
		if cfManager == nil {
			return
		}

		zones := memory.Filter[*cloudflare.CloudflareZone](db, "cloudflare_zones", func(z *cloudflare.CloudflareZone) bool {
			return z.Name == domain.Domain
		})

		if len(zones) == 0 {
			return
		}

		zone := zones[0]

		params := cloudflare.EmailDNSParams{
			MXRecord:       "mail." + domain.Domain,
			SPFRecord:      domain.SPFRecord,
			DKIMSelector:   domain.DKIMSelector,
			DKIMRecord:     domain.DKIMPublicKey,
			DMARCRecord:    domain.DMARCRecord,
			MailServerIP:   serverIP,
		}

		if err := cfManager.UpdateEmailDNSRecords(zone.ZoneID, params); err != nil {
			log.Printf("⚠️  Failed to update Cloudflare DNS records: %v", err)
		}
	}()

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Domain updated successfully and DNS records queued for update",
		"data":  domain,
	})
}

func DeleteEmailDomain(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Email server not initialized",
		})
	}

	domainID, err := strconv.ParseUint(c.Params("domain_id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid domain ID",
		})
	}

	if err := manager.DeleteDomain(uint(domainID)); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to delete domain: " + err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Domain deleted successfully",
	})
}

func GetMailboxes(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Email server not initialized",
		})
	}

	domainIDStr := c.Query("domain_id")
	db := manager.GetDB()
	
	var mailboxes []*email_server.EmailMailbox
	if domainIDStr == "" {
		mailboxes = memory.FindAll[*email_server.EmailMailbox](db, "email_mailboxes")
	} else {
		domainID, err := strconv.ParseUint(domainIDStr, 10, 32)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": true,
				"msg":   "Invalid domain ID",
			})
		}
		mailboxes = memory.Filter[*email_server.EmailMailbox](db, "email_mailboxes", func(mb *email_server.EmailMailbox) bool {
			return mb.DomainID == uint(domainID)
		})
	}

	for _, mb := range mailboxes {
		mb.PlainPassword = ""
		mb.Password = ""
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"data":  mailboxes,
	})
}

func AddMailbox(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Email server not initialized",
		})
	}

	var req struct {
		DomainID uint   `json:"domain_id" validate:"required"`
		Username string `json:"username" validate:"required"`
		Password string `json:"password" validate:"required"`
		Name     string `json:"name"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid request body: " + err.Error(),
		})
	}

	mailbox, err := manager.AddMailbox(req.DomainID, req.Username, req.Password, req.Name)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to add mailbox: " + err.Error(),
		})
	}

	mailbox.PlainPassword = ""
	mailbox.Password = ""

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Mailbox created successfully",
		"data":  mailbox,
	})
}

func UpdateMailbox(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Email server not initialized",
		})
	}

	mailboxID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid mailbox ID",
		})
	}

	var req struct {
		Name         string `json:"name"`
		Quota        int64  `json:"quota"`
		Enabled      bool   `json:"enabled"`
		ForwardTo    string `json:"forward_to"`
		KeepCopy     bool   `json:"keep_copy"`
		AutoReply    bool   `json:"auto_reply"`
		AutoReplyMsg string `json:"auto_reply_msg"`
		Password     string `json:"password,omitempty"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid request body: " + err.Error(),
		})
	}

	db := manager.GetDB()
	mailbox, err := memory.FindByID[*email_server.EmailMailbox](db, "email_mailboxes", uint(mailboxID))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "Mailbox not found",
		})
	}

	// Update fields
	mailbox.Name = req.Name
	mailbox.Quota = req.Quota
	mailbox.Enabled = req.Enabled
	mailbox.ForwardTo = req.ForwardTo
	mailbox.KeepCopy = req.KeepCopy
	mailbox.AutoReply = req.AutoReply
	mailbox.AutoReplyMsg = req.AutoReplyMsg

	// Update password if provided
	if req.Password != "" {
		if err := manager.UpdateMailboxPassword(uint(mailboxID), req.Password); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": true,
				"msg":   "Failed to update password: " + err.Error(),
			})
		}
	}

	if err := memory.Update[*email_server.EmailMailbox](db, "email_mailboxes", mailbox); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to update mailbox: " + err.Error(),
		})
	}

	// Update DNS records in background
	go func() {
		domain, err := memory.FindByID[*email_server.EmailDomain](db, "email_domains", mailbox.DomainID)
		if err != nil {
			return
		}

		cfManager := cloudflare.GetManager()
		if cfManager == nil {
			return
		}

		zones := memory.Filter[*cloudflare.CloudflareZone](db, "cloudflare_zones", func(z *cloudflare.CloudflareZone) bool {
			return z.Name == domain.Domain
		})

		if len(zones) == 0 {
			return
		}

		zone := zones[0]
		serverIP := manager.GetConfig().IPAddress
		if serverIP == "" {
			serverIP = "127.0.0.1"
		}

		params := cloudflare.EmailDNSParams{
			MXRecord:       "mail." + domain.Domain,
			SPFRecord:      domain.SPFRecord,
			DKIMSelector:   domain.DKIMSelector,
			DKIMRecord:     domain.DKIMPublicKey,
			DMARCRecord:    domain.DMARCRecord,
			MailServerIP:   serverIP,
		}

		if err := cfManager.UpdateEmailDNSRecords(zone.ZoneID, params); err != nil {
			log.Printf("⚠️  Failed to update Cloudflare DNS records: %v", err)
		}
	}()

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Mailbox updated successfully and DNS records queued for update",
		"data":  mailbox,
	})
}

func UpdateMailboxPassword(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Email server not initialized",
		})
	}

	mailboxID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid mailbox ID",
		})
	}

	var req struct {
		Password string `json:"password" validate:"required"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid request body: " + err.Error(),
		})
	}

	if req.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Password is required",
		})
	}

	if err := manager.UpdateMailboxPassword(uint(mailboxID), req.Password); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to update password: " + err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Password updated successfully. You can now send emails!",
	})
}

func DeleteMailbox(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Email server not initialized",
		})
	}

	mailboxID, err := strconv.ParseUint(c.Params("mailbox_id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid mailbox ID",
		})
	}

	if err := manager.DeleteMailbox(uint(mailboxID)); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to delete mailbox: " + err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Mailbox deleted successfully",
	})
}

func GetEmails(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Email server not initialized",
		})
	}

	mailboxIDStr := c.Params("mailbox_id")
	mailboxID, err := strconv.ParseUint(mailboxIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid mailbox ID",
		})
	}

	folder := c.Query("folder", "INBOX")
	limitStr := c.Query("limit", "50")
	limit, _ := strconv.Atoi(limitStr)

	imapClient := email_server.NewIMAPClient(manager)
	emails, err := imapClient.GetMessages(uint(mailboxID), folder, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to get emails: " + err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"data":  emails,
	})
}

func GetFolders(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Email server not initialized",
		})
	}

	mailboxIDStr := c.Params("mailbox_id")
	mailboxID, err := strconv.ParseUint(mailboxIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid mailbox ID",
		})
	}

	imapClient := email_server.NewIMAPClient(manager)
	folders, err := imapClient.GetFolders(uint(mailboxID))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to get folders: " + err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"data":  folders,
	})
}

func SendEmail(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Email server not initialized",
		})
	}

	mailboxIDStr := c.Params("mailbox_id")
	mailboxID, err := strconv.ParseUint(mailboxIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid mailbox ID",
		})
	}

	var req struct {
		To      []string `json:"to" validate:"required"`
		CC      []string `json:"cc"`
		BCC     []string `json:"bcc"`
		Subject string   `json:"subject" validate:"required"`
		Body    string   `json:"body"`
		BodyHTML string  `json:"body_html"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid request body: " + err.Error(),
		})
	}

	mailbox, err := memory.FindByID[*email_server.EmailMailbox](manager.GetDB(), "email_mailboxes", uint(mailboxID))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "Mailbox not found",
		})
	}

	msg := &email_server.EmailMessage{
		From:      mailbox.Email,
		To:        req.To,
		CC:        req.CC,
		BCC:       req.BCC,
		Subject:   req.Subject,
		BodyPlain: req.Body,
		BodyHTML:  req.BodyHTML,
		Priority:  3,
	}

	smtpClient := email_server.NewSMTPClient(manager)
	if err := smtpClient.SendEmail(uint(mailboxID), msg); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to send email: " + err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Email sent successfully",
	})
}

func StartEmailServer(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Email server not initialized",
		})
	}

	if err := manager.StartServer(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to start server: " + err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Email server started successfully",
	})
}

func StopEmailServer(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Email server not initialized",
		})
	}

	if err := manager.StopServer(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to stop server: " + err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Email server stopped successfully",
	})
}

func GetEmailServerStatus(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Email server not initialized",
		})
	}

	status, err := manager.GetServerStatus()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to get status: " + err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"data":  status,
	})
}

func CheckMailboxPasswords(c *fiber.Ctx) error {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Email server not initialized",
		})
	}

	db := manager.GetDB()
	mailboxes := memory.FindAll[*email_server.EmailMailbox](db, "email_mailboxes")
	
	var missingPasswords []fiber.Map
	for _, mb := range mailboxes {
		if mb.PlainPassword == "" {
			missingPasswords = append(missingPasswords, fiber.Map{
				"id":    mb.ID,
				"email": mb.Email,
				"name":  mb.Name,
				"fix":   fmt.Sprintf("PUT /api/email/mailboxes/%d/password", mb.ID),
			})
		}
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"total": len(mailboxes),
		"missing_passwords": len(missingPasswords),
		"mailboxes": missingPasswords,
	})
}
