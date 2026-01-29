package email_server

import (
	"context"
	"fmt"
	"log"
	"redock/platform/memory"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

type IMAPClient struct {
	manager *EmailManager
}

func NewIMAPClient(manager *EmailManager) *IMAPClient {
	return &IMAPClient{manager: manager}
}

func (c *IMAPClient) GetMessages(mailboxID uint, folderPath string, limit int) ([]*Email, error) {
	db := c.manager.GetDB()
	mailbox, _ := memory.FindByID[*EmailMailbox](db, "email_mailboxes", mailboxID)
	if mailbox == nil {
		return nil, fmt.Errorf("mailbox not found")
	}

	password, err := c.manager.GetMailboxPassword(mailbox.Email)
	if err != nil {
		return nil, fmt.Errorf("failed to get password: %w", err)
	}

	config := c.manager.config
	addr := fmt.Sprintf("localhost:%d", config.IMAPPort)

	client, err := imapclient.DialInsecure(addr, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to IMAP: %w", err)
	}
	defer client.Close()

	if err := client.Login(mailbox.Email, password).Wait(); err != nil {
		return nil, fmt.Errorf("IMAP login failed: %w", err)
	}

	if folderPath == "" {
		folderPath = "INBOX"
	}

	selectCmd := client.Select(folderPath, nil)
	mailboxData, err := selectCmd.Wait()
	if err != nil {
		return nil, fmt.Errorf("failed to select folder %s: %w", folderPath, err)
	}

	if mailboxData.NumMessages == 0 {
		return []*Email{}, nil
	}

	start := uint32(1)
	end := mailboxData.NumMessages
	if limit > 0 && int(end) > limit {
		start = end - uint32(limit) + 1
	}

	seqSet := imap.SeqSetNum(start, end)

	fetchOptions := &imap.FetchOptions{
		Envelope: true,
		BodySection: []*imap.FetchItemBodySection{
			{Specifier: imap.PartSpecifierHeader},
			{Specifier: imap.PartSpecifierText},
		},
		Flags: true,
		UID:   true,
	}

	fetchCmd := client.Fetch(seqSet, fetchOptions)

	emails := []*Email{}

	for {
		msg := fetchCmd.Next()
		if msg == nil {
			break
		}

		email := &Email{
			Seen:    false,
			Flagged: false,
		}

		// Collect message data
		buf, err := msg.Collect()
		if err != nil {
			log.Printf("⚠️  Failed to collect message: %v", err)
			continue
		}

		// Parse collected data
		if buf.UID > 0 {
			email.UID = uint32(buf.UID)
		}

		if buf.Envelope != nil {
			email.MessageID = buf.Envelope.MessageID
			email.Subject = buf.Envelope.Subject
			email.Date = buf.Envelope.Date

			if len(buf.Envelope.From) > 0 {
				email.From = formatAddress(&buf.Envelope.From[0])
			}
			if len(buf.Envelope.To) > 0 {
				toAddrs := make([]string, len(buf.Envelope.To))
				for i := range buf.Envelope.To {
					toAddrs[i] = formatAddress(&buf.Envelope.To[i])
				}
				email.To = strings.Join(toAddrs, ", ")
			}
		}

		if len(buf.Flags) > 0 {
			for _, flag := range buf.Flags {
				switch flag {
				case imap.FlagSeen:
					email.Seen = true
				case imap.FlagFlagged:
					email.Flagged = true
				}
			}
		}

		// Get body from sections
		for _, bodyBuf := range buf.BodySection {
			if len(bodyBuf.Bytes) > 0 {
				email.BodyPlain = string(bodyBuf.Bytes)
				break
			}
		}

		emails = append(emails, email)
	}

	if err := fetchCmd.Close(); err != nil {
		log.Printf("⚠️  IMAP fetch error: %v", err)
	}

	return emails, nil
}

func formatAddress(addr *imap.Address) string {
	if addr == nil {
		return ""
	}
	email := addr.Mailbox + "@" + addr.Host
	if addr.Name != "" {
		return fmt.Sprintf("%s <%s>", addr.Name, email)
	}
	return email
}

type IMAPFolder struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Attributes []string `json:"attributes"`
	Delimiter  string `json:"delimiter"`
	HasChildren bool `json:"has_children"`
	NoSelect    bool `json:"no_select"`
	MessageCount uint32 `json:"message_count"`
}

func (c *IMAPClient) GetFolders(mailboxID uint) ([]*IMAPFolder, error) {
	db := c.manager.GetDB()
	mailbox, _ := memory.FindByID[*EmailMailbox](db, "email_mailboxes", mailboxID)
	if mailbox == nil {
		return nil, fmt.Errorf("mailbox not found")
	}

	password, err := c.manager.GetMailboxPassword(mailbox.Email)
	if err != nil {
		return nil, fmt.Errorf("failed to get password: %w", err)
	}

	config := c.manager.config
	addr := fmt.Sprintf("localhost:%d", config.IMAPPort)

	client, err := imapclient.DialInsecure(addr, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to IMAP: %w", err)
	}
	defer client.Close()

	if err := client.Login(mailbox.Email, password).Wait(); err != nil {
		return nil, fmt.Errorf("IMAP login failed: %w", err)
	}

	// List all folders
	listCmd := client.List("", "*", nil)
	mailboxes, err := listCmd.Collect()
	if err != nil {
		return nil, fmt.Errorf("failed to list folders: %w", err)
	}

	folders := make([]*IMAPFolder, 0)
	for _, mbox := range mailboxes {
		folder := &IMAPFolder{
			Name:       mbox.Mailbox,
			Path:       mbox.Mailbox,
			Attributes: make([]string, 0),
			Delimiter:  string(mbox.Delim),
		}

		// Parse attributes
		for _, attr := range mbox.Attrs {
			attrStr := strings.ToLower(string(attr))
			folder.Attributes = append(folder.Attributes, attrStr)
			
			if attrStr == "\\haschildren" {
				folder.HasChildren = true
			}
			if attrStr == "\\noselect" {
				folder.NoSelect = true
			}
		}

		// Get message count for each folder (if selectable)
		if !folder.NoSelect {
			if selectData, err := client.Select(mbox.Mailbox, nil).Wait(); err == nil {
				folder.MessageCount = selectData.NumMessages
			}
		}

		folders = append(folders, folder)
	}

	return folders, nil
}

func (c *IMAPClient) appendToFolder(email, password, folderName string, message []byte) error {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	config := c.manager.config
	addr := fmt.Sprintf("localhost:%d", config.IMAPPort)

	// Use a channel to handle timeout
	errChan := make(chan error, 1)
	
	go func() {
		client, err := imapclient.DialInsecure(addr, nil)
		if err != nil {
			errChan <- fmt.Errorf("failed to connect to IMAP: %w", err)
			return
		}
		defer client.Close()

		if err := client.Login(email, password).Wait(); err != nil {
			errChan <- fmt.Errorf("IMAP login failed: %w", err)
			return
		}

		// Append message to folder using IMAP APPEND command
		appendOpts := &imap.AppendOptions{
			Flags: []imap.Flag{imap.FlagSeen}, // Mark as read
			Time:  time.Now(),
		}

		appendCmd := client.Append(folderName, int64(len(message)), appendOpts)
		
		// Write message
		if _, err := appendCmd.Write(message); err != nil {
			errChan <- fmt.Errorf("failed to write message: %w", err)
			return
		}
		
		// Wait for completion
		if _, err := appendCmd.Wait(); err != nil {
			errChan <- fmt.Errorf("failed to append to %s: %w", folderName, err)
			return
		}

		errChan <- nil
	}()
	
	// Wait for either completion or timeout
	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return fmt.Errorf("IMAP append timeout after 10 seconds")
	}
}
