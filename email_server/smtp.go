package email_server

import (
	"bytes"
	"fmt"
	"log"
	"mime/multipart"
	"net/textproto"
	"os/exec"
	"redock/platform/memory"
	"strings"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
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

func (c *SMTPClient) SendEmail(mailboxID uint, msg *EmailMessage) error {
	mailbox, err := memory.FindByID[*EmailMailbox](c.manager.db, "email_mailboxes", mailboxID)
	if err != nil {
		return fmt.Errorf("mailbox not found: %w", err)
	}
	
	password, err := c.manager.GetMailboxPassword(mailbox.Email)
	if err != nil {
		return fmt.Errorf("password not available: %w", err)
	}
	
	mimeMsg, err := c.buildMIMEMessage(msg)
	if err != nil {
		return fmt.Errorf("failed to build MIME message: %w", err)
	}
	
	client, err := c.connect(mailbox.Email, password)
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP: %w", err)
	}
	defer client.Quit()
	
	if err := client.Mail(msg.From, nil); err != nil {
		return fmt.Errorf("failed to set sender: %w", err)
	}
	
	allRecipients := append(msg.To, msg.CC...)
	allRecipients = append(allRecipients, msg.BCC...)
	
	for _, rcpt := range allRecipients {
		if err := client.Rcpt(rcpt, nil); err != nil {
			return fmt.Errorf("failed to set recipient %s: %w", rcpt, err)
		}
	}
	
	wc, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to initiate data: %w", err)
	}
	
	if _, err := wc.Write(mimeMsg); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}
	
	if err := wc.Close(); err != nil {
		return fmt.Errorf("failed to close data writer: %w", err)
	}
	
	// Save to Sent folder asynchronously
	// With named volumes, we can write directly to maildir without permission issues!
	go func(mbxID uint, message *EmailMessage, mime []byte) {
		if err := c.saveToSent(mbxID, message, mime); err != nil {
			log.Printf("⚠️  Failed to save to Sent folder: %v", err)
		}
	}(mailboxID, msg, mimeMsg)
	
	c.logSentEmail(mailboxID, msg)
	
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

func (c *SMTPClient) connect(email, password string) (*smtp.Client, error) {
	config := c.manager.config
	addr := fmt.Sprintf("localhost:%d", config.SubmissionPort)
	
	client, err := smtp.Dial(addr)
	if err != nil {
		return nil, fmt.Errorf("dial failed: %w", err)
	}
	
	auth := sasl.NewPlainClient("", email, password)
	if err := client.Auth(auth); err != nil {
		client.Close()
		return nil, fmt.Errorf("authentication failed: %w", err)
	}
	
	return client, nil
}

func (c *SMTPClient) saveToSent(mailboxID uint, msg *EmailMessage, mimeMsg []byte) error {
	mailbox, err := memory.FindByID[*EmailMailbox](c.manager.db, "email_mailboxes", mailboxID)
	if err != nil {
		return fmt.Errorf("mailbox not found: %w", err)
	}
	
	domain, err := memory.FindByID[*EmailDomain](c.manager.db, "email_domains", mailbox.DomainID)
	if err != nil {
		return fmt.Errorf("domain not found: %w", err)
	}
	
	// With named volumes, we can write directly via container!
	// Create unique Maildir filename with Seen flag
	timestamp := time.Now().UnixNano()
	filename := fmt.Sprintf("%d.%s:2,S", timestamp, "localhost")
	
	// Use docker exec to write inside container as docker user (not root!)
	// Note: docker-mailserver uses mail_home=/var/mail/%d/%n/home/, so Maildir is at home/.Sent/
	containerPath := fmt.Sprintf("/var/mail/%s/%s/home/.Sent/cur/%s", domain.Domain, mailbox.Username, filename)
	
	// Run as docker user to ensure correct ownership
	cmd := exec.Command("docker", "exec", "-i", "-u", "docker", c.manager.config.ContainerName, 
		"tee", containerPath)
	cmd.Stdin = bytes.NewReader(mimeMsg)
	
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to save via docker exec: %w (output: %s)", err, string(output))
	}
	
	return nil
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
