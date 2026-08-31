package controllers

import (
	"fmt"
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

	// Publish MX/SPF/DKIM/DMARC to Cloudflare when the domain is a zone on a
	// connected account. Runs in the background: DNS must never block the response.
	manager.SyncDomainDNSAsync(domain)

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

	manager.SyncDomainDNSAsync(domain)

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

// sanitizedMailbox returns a copy of a mailbox with its secrets removed, for
// sending to the dashboard. It must never mutate the argument: the in-memory
// database hands out pointers to the live records, so editing one in place
// changes — and eventually persists — the stored account.
func sanitizedMailbox(mb *email_server.EmailMailbox) *email_server.EmailMailbox {
	if mb == nil {
		return nil
	}
	copied := *mb
	copied.Password = ""
	copied.PlainPassword = ""
	return &copied
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

	// Never blank the stored entity: memory.Filter hands back pointers into the
	// in-memory database, so clearing a field here would wipe the real password
	// hash — and the periodic flush would persist that loss.
	response := make([]*email_server.EmailMailbox, 0, len(mailboxes))
	for _, mb := range mailboxes {
		response = append(response, sanitizedMailbox(mb))
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"data":  response,
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

	// Respond with a copy; blanking the stored mailbox would destroy the
	// password that was just set.
	response := sanitizedMailbox(mailbox)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Mailbox created successfully",
		"data":  response,
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

	// Keep the domain's DNS in step with the change.
	if domain, err := memory.FindByID[*email_server.EmailDomain](db, "email_domains", mailbox.DomainID); err == nil && domain != nil {
		manager.SyncDomainDNSAsync(domain)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Mailbox updated successfully and DNS records queued for update",
		"data":  sanitizedMailbox(mailbox),
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

	emails, err := manager.WebmailMessages(uint(mailboxID), folder, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to get emails: " + err.Error(),
		})
	}

	db := manager.GetDB()
	var folderID uint
	if fs := memory.Filter[*email_server.EmailFolder](db, "email_folders", func(f *email_server.EmailFolder) bool {
		return f.MailboxID == uint(mailboxID) && f.Path == folder
	}); len(fs) > 0 {
		folderID = fs[0].ID
	}
	for _, e := range emails {
		e.MailboxID = uint(mailboxID)
		e.FolderID = folderID
		e.CreatedAt = e.Date
		e.UpdatedAt = e.Date
	}

	threads := email_server.GroupEmailsIntoThreads(emails)
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"data":  threads,
	})
}

func GetThread(c *fiber.Ctx) error {
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
	uidStr := c.Query("uid", "0")
	uid, _ := strconv.ParseUint(uidStr, 10, 32)
	if uid == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "uid query parameter required",
		})
	}

	thread, err := manager.WebmailThread(uint(mailboxID), folder, uint32(uid), 200)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to get thread: " + err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"data":  thread,
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

	folders, err := manager.WebmailFolders(uint(mailboxID))
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
		To       []string `json:"to" validate:"required"`
		CC       []string `json:"cc"`
		BCC      []string `json:"bcc"`
		Subject  string   `json:"subject" validate:"required"`
		Body     string   `json:"body"`
		BodyHTML string   `json:"body_html"`
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
		From: mailbox.Email,
		// The mailbox already carries the person's name; putting it in the From
		// header costs nothing and removes a spam-filter penalty for sending
		// with a bare address.
		FromName:  mailbox.Name,
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

	// Two different failures hide behind "no password", and they need different
	// answers. A mailbox with no hash but a stored copy repairs itself on the
	// next start; one with neither cannot be recovered and has to be set again
	// by hand, which means telling the person who uses it.
	broken := make([]fiber.Map, 0)
	repairable := 0
	for _, mb := range mailboxes {
		if mb == nil || mb.IsDeleted() {
			continue
		}

		hasHash := mb.Password != ""
		hasCopy := mb.PlainPassword != ""
		if hasHash && hasCopy {
			continue
		}

		state := "unusable"
		switch {
		case hasHash:
			// Logins work; only the copy used to show the password is gone.
			state = "no_stored_copy"
		case hasCopy:
			state = "repairable"
			repairable++
		}

		broken = append(broken, fiber.Map{
			"id":    mb.ID,
			"email": mb.Email,
			"name":  mb.Name,
			"state": state,
			"fix":   fmt.Sprintf("PUT /api/email/mailboxes/%d/password", mb.ID),
		})
	}

	unusable := 0
	for _, entry := range broken {
		if entry["state"] == "unusable" {
			unusable++
		}
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error":      false,
		"total":      len(mailboxes),
		"unusable":   unusable,
		"repairable": repairable,
		"mailboxes":  broken,
	})
}
