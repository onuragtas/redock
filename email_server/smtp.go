package email_server

import (
	"bytes"
	"fmt"
	"log"
	"mime/multipart"
	"net/mail"
	"net/textproto"
	"redock/platform/memory"
	"strings"
	"time"
)

type SMTPClient struct {
	manager *EmailManager
}

type EmailMessage struct {
	From string
	// FromName is the display name shown beside the address. A From header with
	// only a bare address is one of the things spam filters count against a
	// message — SpamAssassin's NO_FM_NAME_IP_HOSTN, worth about 1.5 points,
	// needs a missing display name to fire.
	FromName    string
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

	// Fill the sender in from the mailbox this is being sent through. A caller
	// that forgets used to produce a message with no From header at all, which
	// every large receiver refuses — Gmail answers "550 5.7.1 Messages missing
	// a valid address in From: header". The bounce arrives from a remote server
	// long after the send looked successful, so the mistake is worth making
	// impossible rather than merely documenting.
	if strings.TrimSpace(msg.From) == "" {
		msg.From = mailbox.Email
	}
	if strings.TrimSpace(msg.FromName) == "" {
		msg.FromName = mailbox.Name
	}
	if strings.TrimSpace(msg.From) == "" {
		return fmt.Errorf("the mailbox has no address to send from")
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

	go func(mbxID uint, mime []byte) {
		account, err := c.manager.accountForID(mbxID)
		if err != nil {
			log.Printf("mail: could not file the Sent copy: %v", err)
			return
		}
		c.manager.saveToSent(account, mime)
	}(mailboxID, mimeMsg)

	return nil
}

func (c *SMTPClient) buildMIMEMessage(msg *EmailMessage) ([]byte, error) {
	var buf bytes.Buffer

	headers := make(textproto.MIMEHeader)
	headers.Set("From", formatFrom(msg.FromName, msg.From))
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

// formatFrom builds the From header, quoting and encoding the display name the
// way RFC 5322 and RFC 2047 require — a name with a comma, a quote or a
// non-ASCII letter breaks the header if it is simply concatenated.
func formatFrom(name, address string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return address
	}
	return (&mail.Address{Name: name, Address: address}).String()
}
