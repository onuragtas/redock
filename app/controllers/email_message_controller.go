package controllers

import (
	"fmt"
	"strconv"
	"strings"

	"redock/email_server"

	"github.com/gofiber/fiber/v2"
)

// messageTarget pulls the mailbox, folder and UID every message action needs.
func messageTarget(c *fiber.Ctx) (*email_server.EmailManager, uint, string, uint32, error) {
	manager := email_server.GetEmailManager()
	if manager == nil {
		return nil, 0, "", 0, fmt.Errorf("email server not initialized")
	}

	mailboxID, err := strconv.ParseUint(c.Params("mailbox_id"), 10, 32)
	if err != nil {
		return nil, 0, "", 0, fmt.Errorf("invalid mailbox id")
	}

	uid, err := strconv.ParseUint(c.Params("uid"), 10, 32)
	if err != nil {
		// Some actions take the UID in the body instead of the path.
		uidQuery := c.Query("uid")
		parsed, qErr := strconv.ParseUint(uidQuery, 10, 32)
		if qErr != nil {
			return nil, 0, "", 0, fmt.Errorf("invalid message uid")
		}
		uid = parsed
	}

	folder := c.Query("folder", "INBOX")
	return manager, uint(mailboxID), folder, uint32(uid), nil
}

// SetMessageFlag marks a message read/unread or starred/unstarred.
// @Summary set a message flag
// @Tags Email
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Router /email/mailboxes/{mailbox_id}/messages/{uid}/flag [put]
func SetMessageFlag(c *fiber.Ctx) error {
	manager, mailboxID, folder, uid, err := messageTarget(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}

	body := struct {
		Flag  string `json:"flag"`
		Value bool   `json:"value"`
	}{}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}

	if err := manager.SetMessageFlag(mailboxID, folder, uid, body.Flag, body.Value); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	return c.JSON(fiber.Map{"error": false, "msg": "flag updated"})
}

// MoveMessage moves a message to another folder; deleting is a move to Trash.
// @Summary move a message
// @Tags Email
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Router /email/mailboxes/{mailbox_id}/messages/{uid}/move [post]
func MoveMessage(c *fiber.Ctx) error {
	manager, mailboxID, folder, uid, err := messageTarget(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}

	body := struct {
		To string `json:"to"`
	}{}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}

	if err := manager.MoveMessage(mailboxID, folder, body.To, uid); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	return c.JSON(fiber.Map{"error": false, "msg": "message moved"})
}

// DeleteMessage moves a message to Trash, or removes it when already there.
// @Summary delete a message
// @Tags Email
// @Security ApiKeyAuth
// @Produce json
// @Router /email/mailboxes/{mailbox_id}/messages/{uid} [delete]
func DeleteMessage(c *fiber.Ctx) error {
	// Deleting is the one action that must not fall back to a default folder.
	// UIDs are per-folder, so a request that forgets to say where it means
	// would delete whatever happens to hold that number in the inbox — a
	// message the caller never named. Check this before anything else: it is a
	// property of the request, true or false whether or not the server is up.
	folder := strings.TrimSpace(c.Query("folder"))
	if folder == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "msg": "folder is required when deleting a message",
		})
	}

	manager, mailboxID, _, uid, err := messageTarget(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}

	if err := manager.DeleteMessage(mailboxID, folder, uid); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	return c.JSON(fiber.Map{"error": false, "msg": "message deleted"})
}

// ListMessageAttachments describes what is attached to a message.
// @Summary list attachments
// @Tags Email
// @Security ApiKeyAuth
// @Produce json
// @Router /email/mailboxes/{mailbox_id}/messages/{uid}/attachments [get]
func ListMessageAttachments(c *fiber.Ctx) error {
	manager, mailboxID, folder, uid, err := messageTarget(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}

	attachments, err := manager.ListAttachments(mailboxID, folder, uid)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	return c.JSON(fiber.Map{"error": false, "data": attachments})
}

// DownloadMessageAttachment streams one attachment to the browser.
// @Summary download an attachment
// @Tags Email
// @Security ApiKeyAuth
// @Router /email/mailboxes/{mailbox_id}/messages/{uid}/attachments/{index} [get]
func DownloadMessageAttachment(c *fiber.Ctx) error {
	manager, mailboxID, folder, uid, err := messageTarget(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}

	index, err := strconv.Atoi(c.Params("index"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "invalid attachment index"})
	}

	attachment, err := manager.Attachment(mailboxID, folder, uid, index)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}

	contentType := attachment.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.Set("Content-Type", contentType)
	// Quote the filename so spaces and non-ASCII names survive the header.
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", attachment.Filename))
	return c.Send(attachment.Data)
}

// DownloadRawMessage returns the original message as an .eml file.
// @Summary download the original message
// @Tags Email
// @Security ApiKeyAuth
// @Router /email/mailboxes/{mailbox_id}/messages/{uid}/raw [get]
func DownloadRawMessage(c *fiber.Ctx) error {
	manager, mailboxID, folder, uid, err := messageTarget(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}

	raw, err := manager.RawMessage(mailboxID, folder, uid)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}

	c.Set("Content-Type", "message/rfc822")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fmt.Sprintf("message-%d.eml", uid)))
	return c.Send(raw)
}

// SaveDraft stores a composed message in the Drafts folder.
// @Summary save a draft
// @Tags Email
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Router /email/mailboxes/{mailbox_id}/drafts [post]
func SaveDraft(c *fiber.Ctx) error {
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

	var req struct {
		From     string   `json:"from"`
		To       []string `json:"to"`
		CC       []string `json:"cc"`
		BCC      []string `json:"bcc"`
		Subject  string   `json:"subject"`
		Body     string   `json:"body"`
		BodyHTML string   `json:"body_html"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}

	uid, err := manager.SaveDraft(uint(mailboxID), &email_server.EmailMessage{
		From:      req.From,
		To:        req.To,
		CC:        req.CC,
		BCC:       req.BCC,
		Subject:   req.Subject,
		BodyPlain: req.Body,
		BodyHTML:  req.BodyHTML,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}

	return c.JSON(fiber.Map{"error": false, "data": fiber.Map{"uid": uid}})
}
