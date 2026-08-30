package email_server

import (
	"bytes"
	"fmt"
	"log"
	"mime/multipart"
	"net/textproto"
	"redock/platform/memory"
	"strings"
	"time"
)

type SMTPClient struct {
	manager *EmailManager
}

type EmailMessage struct {
	From        string
	To          []string
	CC          []string
	BCC         []string
	Subject     string
	BodyPlain   string
	BodyHTML    string
	Attachments []EmailAttachmentData
	InReplyTo   string
	References  string
	Priority    int
}

type EmailAttachmentData struct {
	Filename    string
	ContentType string
	Data        []byte
}

func NewSMTPClient(manager *EmailManager) *SMTPClient {
	return &SMTPClient{manager: manager}
}

// SendEmail composes a dashboard message and hands it to the engine: local
// recipients are written straight to their Maildir, remote ones go on the
// outbound queue (which signs them with DKIM and retries).
func (c *SMTPClient) SendEmail(mailboxID uint, msg *EmailMessage) error {
	mailbox, err := memory.FindByID[*EmailMailbox](c.manager.db, "email_mailboxes", mailboxID)
	if err != nil {
		return fmt.Errorf("mailbox not found: %w", err)
	}

	mimeMsg, err := c.buildMIMEMessage(msg)
	if err != nil {
		return fmt.Errorf("failed to build MIME message: %w", err)
	}

	return c.sendNative(mailboxID, mailbox, msg, mimeMsg)
}

// sendNative delivers a dashboard-composed message through the built-in
// engine: local recipients land in their Maildir immediately, remote ones go
// on the outbound queue (which signs them with DKIM and retries).
func (c *SMTPClient) sendNative(mailboxID uint, mailbox *EmailMailbox, msg *EmailMessage, mimeMsg []byte) error {
	recipients := append(append(append([]string{}, msg.To...), msg.CC...), msg.BCC...)
	if len(recipients) == 0 {
		return fmt.Errorf("no recipients")
	}

	var remote []string
	for _, rcpt := range recipients {
		address := normalizeAddress(rcpt)
		if address == "" {
			continue
		}
		if account := c.manager.LookupAccount(address); account != nil {
			if err := c.manager.deliverLocal(account, inboxName, mimeMsg, nil); err != nil {
				return fmt.Errorf("failed to deliver to %s: %w", address, err)
			}
			c.manager.logMailEvent(mailEvent{
				Direction: "out",
				Status:    "delivered",
				From:      msg.From,
				To:        address,
				Subject:   msg.Subject,
				Size:      int64(len(mimeMsg)),
				Service:   "dashboard",
				MailboxID: account.Mailbox.ID,
				Detail:    "delivered locally",
			})
			continue
		}
		remote = append(remote, address)
	}

	if len(remote) > 0 {
		_, senderDomain := splitAddress(normalizeAddress(msg.From))
		item := &QueueItem{
			From:       normalizeAddress(msg.From),
			Recipients: remote,
			Domain:     senderDomain,
			Subject:    msg.Subject,
			MailboxID:  mailboxID,
		}
		if err := c.manager.queue().Enqueue(item, mimeMsg); err != nil {
			return fmt.Errorf("failed to queue message: %w", err)
		}
		for _, rcpt := range remote {
			c.manager.logMailEvent(mailEvent{
				Direction: "out",
				Status:    "queued",
				From:      msg.From,
				To:        rcpt,
				Subject:   msg.Subject,
				Size:      int64(len(mimeMsg)),
				Service:   "dashboard",
				QueueID:   item.ID,
				MailboxID: mailboxID,
				Detail:    "accepted for delivery",
			})
		}
	}

	go func(mbxID uint, message *EmailMessage, mime []byte) {
		if err := c.saveToSent(mbxID, message, mime); err != nil {
			log.Printf("⚠️  Failed to save to Sent folder: %v", err)
		}
	}(mailboxID, msg, mimeMsg)

	return nil
}

func (c *SMTPClient) buildMIMEMessage(msg *EmailMessage) ([]byte, error) {
	var buf bytes.Buffer

	headers := make(textproto.MIMEHeader)
	headers.Set("From", msg.From)
	headers.Set("To", strings.Join(msg.To, ", "))
	if len(msg.CC) > 0 {
		headers.Set("Cc", strings.Join(msg.CC, ", "))
	}
	headers.Set("Subject", msg.Subject)
	headers.Set("Date", time.Now().Format(time.RFC1123Z))
	headers.Set("Message-ID", generateMessageID(msg.From))

	if msg.InReplyTo != "" {
		headers.Set("In-Reply-To", msg.InReplyTo)
	}
	if msg.References != "" {
		headers.Set("References", msg.References)
	}

	headers.Set("MIME-Version", "1.0")

	hasHTML := msg.BodyHTML != ""
	hasPlain := msg.BodyPlain != ""

	if hasHTML && hasPlain {
		boundary := generateBoundary()
		headers.Set("Content-Type", fmt.Sprintf("multipart/alternative; boundary=%s", boundary))

		for key, values := range headers {
			for _, value := range values {
				fmt.Fprintf(&buf, "%s: %s\r\n", key, value)
			}
		}
		buf.WriteString("\r\n")

		writer := multipart.NewWriter(&buf)
		writer.SetBoundary(boundary)

		plainHeader := textproto.MIMEHeader{}
		plainHeader.Set("Content-Type", "text/plain; charset=utf-8")
		plainPart, _ := writer.CreatePart(plainHeader)
		plainPart.Write([]byte(msg.BodyPlain))

		htmlHeader := textproto.MIMEHeader{}
		htmlHeader.Set("Content-Type", "text/html; charset=utf-8")
		htmlPart, _ := writer.CreatePart(htmlHeader)
		htmlPart.Write([]byte(msg.BodyHTML))

		writer.Close()
	} else {
		if hasHTML {
			headers.Set("Content-Type", "text/html; charset=utf-8")
		} else {
			headers.Set("Content-Type", "text/plain; charset=utf-8")
		}

		for key, values := range headers {
			for _, value := range values {
				fmt.Fprintf(&buf, "%s: %s\r\n", key, value)
			}
		}
		buf.WriteString("\r\n")

		if hasHTML {
			buf.WriteString(msg.BodyHTML)
		} else {
			buf.WriteString(msg.BodyPlain)
		}
	}

	return buf.Bytes(), nil
}

// saveToSent files a copy of an outgoing message in the sender's Sent folder.
func (c *SMTPClient) saveToSent(mailboxID uint, msg *EmailMessage, mimeMsg []byte) error {
	mailbox, err := memory.FindByID[*EmailMailbox](c.manager.db, "email_mailboxes", mailboxID)
	if err != nil {
		return fmt.Errorf("mailbox not found: %w", err)
	}

	domain, err := memory.FindByID[*EmailDomain](c.manager.db, "email_domains", mailbox.DomainID)
	if err != nil {
		return fmt.Errorf("domain not found: %w", err)
	}

	if err := c.manager.store().EnsureMailbox(domain.Domain, mailbox.Username); err != nil {
		return err
	}

	base := c.manager.store().MailboxPath(domain.Domain, mailbox.Username)
	_, err = c.manager.store().Deliver(base, "Sent", mimeMsg, []string{imapFlagSeen}, time.Now())
	return err
}

func (c *SMTPClient) logSentEmail(mailboxID uint, msg *EmailMessage) {
	logEntry := &EmailLog{
		MailboxID:     mailboxID,
		Type:          "sent",
		From:          msg.From,
		To:            strings.Join(msg.To, ", "),
		Subject:       msg.Subject,
		Status:        "delivered",
		StatusMessage: "Sent successfully",
		Timestamp:     time.Now(),
	}

	memory.Create[*EmailLog](c.manager.db, "email_logs", logEntry)
}

func generateMessageID(from string) string {
	parts := strings.Split(from, "@")
	domain := "localhost"
	if len(parts) > 1 {
		domain = parts[1]
	}
	return fmt.Sprintf("<%d.%d@%s>", time.Now().UnixNano(), time.Now().Unix(), domain)
}

func generateBoundary() string {
	return fmt.Sprintf("boundary_%d", time.Now().UnixNano())
}
