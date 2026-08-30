package email_server

import (
	"bufio"
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"

	"redock/platform/memory"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/backend/backendutil"
	"github.com/emersion/go-message/textproto"
)

// The webmail reads mail straight from the Maildir. It used to log in to the
// mail server over IMAP — a round-trip through our own listener that also
// required a recoverable plaintext password. Reading the store directly is
// faster and means the dashboard needs no mailbox credentials at all.

// accountFor resolves a mailbox ID to its Maildir root.
func (m *EmailManager) accountForID(mailboxID uint) (*Account, error) {
	mailbox, err := memory.FindByID[*EmailMailbox](m.db, "email_mailboxes", mailboxID)
	if err != nil || mailbox == nil {
		return nil, fmt.Errorf("mailbox not found")
	}

	account := m.accountFor(mailbox)
	if account == nil {
		return nil, fmt.Errorf("domain not found for %s", mailbox.Email)
	}
	if err := m.store().EnsureMailbox(account.Domain.Domain, account.Mailbox.Username); err != nil {
		return nil, err
	}
	return account, nil
}

// WebmailMessages returns the newest messages of a folder, newest last (the
// order the dashboard's thread grouping expects).
func (m *EmailManager) WebmailMessages(mailboxID uint, folder string, limit int) ([]*Email, error) {
	account, err := m.accountForID(mailboxID)
	if err != nil {
		return nil, err
	}
	if folder == "" {
		folder = inboxName
	}

	messages, err := m.store().List(account.Base, folder)
	if err != nil {
		return nil, fmt.Errorf("failed to open folder %s: %w", folder, err)
	}
	if limit > 0 && len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}

	emails := make([]*Email, 0, len(messages))
	for _, msg := range messages {
		email, err := m.readEmail(account, folder, msg)
		if err != nil {
			continue // one unreadable message must not empty the mailbox
		}
		emails = append(emails, email)
	}
	return emails, nil
}

// WebmailThread returns every message sharing a thread with the given UID,
// oldest first.
func (m *EmailManager) WebmailThread(mailboxID uint, folder string, uid uint32, limit int) ([]*Email, error) {
	if limit <= 0 {
		limit = 200
	}

	emails, err := m.WebmailMessages(mailboxID, folder, limit)
	if err != nil {
		return nil, err
	}

	var target *Email
	for _, email := range emails {
		if email.UID == uid {
			target = email
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("message not found")
	}

	thread := make([]*Email, 0, 8)
	for _, email := range emails {
		if email.ThreadID == target.ThreadID {
			thread = append(thread, email)
		}
	}
	sort.Slice(thread, func(i, j int) bool { return thread[i].Date.Before(thread[j].Date) })
	return thread, nil
}

// WebmailFolders lists a mailbox's folders with their counts.
func (m *EmailManager) WebmailFolders(mailboxID uint) ([]*IMAPFolder, error) {
	account, err := m.accountForID(mailboxID)
	if err != nil {
		return nil, err
	}

	names, err := m.store().ListFolders(account.Base)
	if err != nil {
		return nil, err
	}

	folders := make([]*IMAPFolder, 0, len(names))
	for _, name := range names {
		stats, err := m.store().Stats(account.Base, name)
		if err != nil {
			continue
		}
		folders = append(folders, &IMAPFolder{
			Name:         name,
			Path:         name,
			Delimiter:    imapDelimiter,
			MessageCount: stats.Messages,
			UnseenCount:  stats.Unseen,
		})
	}
	return folders, nil
}

// MarkMessageSeen sets or clears the \Seen flag, which is what opening a
// message in the webmail should do.
func (m *EmailManager) MarkMessageSeen(mailboxID uint, folder string, uid uint32, seen bool) error {
	account, err := m.accountForID(mailboxID)
	if err != nil {
		return err
	}
	if folder == "" {
		folder = inboxName
	}

	messages, err := m.store().List(account.Base, folder)
	if err != nil {
		return err
	}

	for _, msg := range messages {
		if msg.UID != uid {
			continue
		}
		flags := stripFlag(msg.Flags, imapFlagRecent)
		if seen {
			if !hasFlag(flags, imapFlagSeen) {
				flags = append(flags, imapFlagSeen)
			}
		} else {
			flags = stripFlag(flags, imapFlagSeen)
		}
		_, err := m.store().SetFlags(account.Base, folder, msg, flags)
		return err
	}
	return fmt.Errorf("message not found")
}

// DeleteMessage moves a message to Trash, or removes it for good when it is
// already there.
func (m *EmailManager) DeleteMessage(mailboxID uint, folder string, uid uint32) error {
	account, err := m.accountForID(mailboxID)
	if err != nil {
		return err
	}
	if folder == "" {
		folder = inboxName
	}

	messages, err := m.store().List(account.Base, folder)
	if err != nil {
		return err
	}

	for _, msg := range messages {
		if msg.UID != uid {
			continue
		}
		if strings.EqualFold(folder, "Trash") {
			return m.store().Remove(account.Base, folder, msg)
		}
		_, err := m.store().Move(account.Base, folder, "Trash", msg)
		return err
	}
	return fmt.Errorf("message not found")
}

// readEmail turns one stored message into the shape the dashboard renders.
func (m *EmailManager) readEmail(account *Account, folder string, msg MaildirMessage) (*Email, error) {
	raw, err := m.store().Read(account.Base, folder, msg)
	if err != nil {
		return nil, err
	}

	email := &Email{
		MailboxID: account.Mailbox.ID,
		UID:       msg.UID,
		Size:      msg.Size,
		Date:      msg.Date,
		Seen:      hasFlag(msg.Flags, imapFlagSeen),
		Flagged:   hasFlag(msg.Flags, imapFlagFlagged),
		Answered:  hasFlag(msg.Flags, imapFlagAnswered),
		Draft:     hasFlag(msg.Flags, imapFlagDraft),
		Recent:    hasFlag(msg.Flags, imapFlagRecent),
	}

	reader := bufio.NewReader(bytes.NewReader(raw))
	header, err := textproto.ReadHeader(reader)
	if err == nil {
		if envelope, err := backendutil.FetchEnvelope(header); err == nil && envelope != nil {
			email.MessageID = strings.Trim(envelope.MessageId, "<>")
			email.Subject = envelope.Subject
			email.InReplyTo = strings.Trim(envelope.InReplyTo, "<>")
			if !time.Time(envelope.Date).IsZero() {
				email.Date = time.Time(envelope.Date)
			}
			if len(envelope.From) > 0 {
				email.From = formatAddressV1(envelope.From[0])
			}
			email.To = joinAddresses(envelope.To)
			email.CC = joinAddresses(envelope.Cc)
			email.ReplyTo = joinAddresses(envelope.ReplyTo)
		}
	}

	plain, html, references, attachments := extractBodyFromRawMessage(raw)
	email.BodyPlain = plain
	email.BodyHTML = html
	email.References = references
	email.AttachmentCount = attachments
	email.HasAttachments = attachments > 0
	email.ThreadID = computeThreadID(email.MessageID, references, email.InReplyTo)

	return email, nil
}

func joinAddresses(addresses []*imap.Address) string {
	if len(addresses) == 0 {
		return ""
	}
	formatted := make([]string, 0, len(addresses))
	for _, address := range addresses {
		if address == nil {
			continue
		}
		formatted = append(formatted, formatAddressV1(address))
	}
	return strings.Join(formatted, ", ")
}
