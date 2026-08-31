package email_server

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"strings"
	"time"

	"github.com/emersion/go-message"
	_ "github.com/emersion/go-message/charset" // register the common charsets
)

// Reading mail is only half of a mailbox. These are the actions the webmail
// needs to be usable: mark as read, flag, move to trash, and get at what was
// attached.

// MessageAttachment is one attached part, extracted on demand rather than kept
// in memory with the message list.
type MessageAttachment struct {
	Index       int    `json:"index"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int    `json:"size"`
	Inline      bool   `json:"inline"`
	Data        []byte `json:"-"`
}

// ListAttachments returns what is attached to a message, without the contents.
func (m *EmailManager) ListAttachments(mailboxID uint, folder string, uid uint32) ([]MessageAttachment, error) {
	raw, err := m.rawMessage(mailboxID, folder, uid)
	if err != nil {
		return nil, err
	}

	parts, err := extractAttachments(raw)
	if err != nil {
		return nil, err
	}
	for i := range parts {
		parts[i].Data = nil // the listing must stay cheap
	}
	return parts, nil
}

// Attachment returns one attachment with its bytes, for download.
func (m *EmailManager) Attachment(mailboxID uint, folder string, uid uint32, index int) (*MessageAttachment, error) {
	raw, err := m.rawMessage(mailboxID, folder, uid)
	if err != nil {
		return nil, err
	}

	parts, err := extractAttachments(raw)
	if err != nil {
		return nil, err
	}
	if index < 0 || index >= len(parts) {
		return nil, fmt.Errorf("attachment %d not found", index)
	}
	return &parts[index], nil
}

// RawMessage returns the complete original message, which is what "show
// source" and "download .eml" need.
func (m *EmailManager) RawMessage(mailboxID uint, folder string, uid uint32) ([]byte, error) {
	return m.rawMessage(mailboxID, folder, uid)
}

func (m *EmailManager) rawMessage(mailboxID uint, folder string, uid uint32) ([]byte, error) {
	account, err := m.accountForID(mailboxID)
	if err != nil {
		return nil, err
	}
	if folder == "" {
		folder = inboxName
	}

	messages, err := m.store().List(account.Base, folder)
	if err != nil {
		return nil, err
	}
	for _, msg := range messages {
		if msg.UID == uid {
			return m.store().Read(account.Base, folder, msg)
		}
	}
	return nil, fmt.Errorf("message not found")
}

// extractAttachments walks a MIME tree and collects the parts a mail client
// would offer to save.
func extractAttachments(raw []byte) ([]MessageAttachment, error) {
	entity, err := message.Read(bytes.NewReader(raw))
	if err != nil && entity == nil {
		return nil, err
	}

	attachments := []MessageAttachment{}
	var walk func(e *message.Entity) error

	walk = func(e *message.Entity) error {
		if multipart := e.MultipartReader(); multipart != nil {
			for {
				part, err := multipart.NextPart()
				if err == io.EOF {
					return nil
				}
				if err != nil {
					return nil // a malformed tail must not hide what came before
				}
				if err := walk(part); err != nil {
					return err
				}
			}
		}

		disposition, params, _ := e.Header.ContentDisposition()
		contentType, ctParams, _ := e.Header.ContentType()

		filename := params["filename"]
		if filename == "" {
			filename = ctParams["name"]
		}
		inline := strings.EqualFold(disposition, "inline")

		// A part counts as an attachment when it is marked as one, or when it
		// carries a filename, or when it is a body type no client renders.
		isAttachment := strings.EqualFold(disposition, "attachment") || filename != "" ||
			(!strings.HasPrefix(contentType, "text/") && !strings.HasPrefix(contentType, "multipart/"))
		if !isAttachment {
			return nil
		}

		data, err := io.ReadAll(io.LimitReader(e.Body, maxAttachmentBytes))
		if err != nil {
			return nil
		}

		if decoded, err := new(mime.WordDecoder).DecodeHeader(filename); err == nil && decoded != "" {
			filename = decoded
		}
		if filename == "" {
			filename = fmt.Sprintf("part-%d", len(attachments)+1)
		}

		attachments = append(attachments, MessageAttachment{
			Index:       len(attachments),
			Filename:    filename,
			ContentType: contentType,
			Size:        len(data),
			Inline:      inline,
			Data:        data,
		})
		return nil
	}

	_ = walk(entity)
	return attachments, nil
}

// maxAttachmentBytes bounds a single extracted part, so a malformed message
// cannot pull an unbounded amount into memory.
const maxAttachmentBytes = 50 << 20 // 50 MB

// SetMessageFlag turns one IMAP flag on or off, which is what "mark as read"
// and "star" do.
func (m *EmailManager) SetMessageFlag(mailboxID uint, folder string, uid uint32, flag string, on bool) error {
	account, err := m.accountForID(mailboxID)
	if err != nil {
		return err
	}
	if folder == "" {
		folder = inboxName
	}

	canonical, err := canonicalFlag(flag)
	if err != nil {
		return err
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
		if on {
			if !hasFlag(flags, canonical) {
				flags = append(flags, canonical)
			}
		} else {
			flags = stripFlag(flags, canonical)
		}
		_, err := m.store().SetFlags(account.Base, folder, msg, flags)
		return err
	}
	return fmt.Errorf("message not found")
}

// canonicalFlag maps the names the dashboard uses onto IMAP flags, and refuses
// anything else so a caller cannot invent flags the store will not understand.
func canonicalFlag(flag string) (string, error) {
	switch strings.ToLower(strings.TrimPrefix(flag, "\\")) {
	case "seen", "read":
		return imapFlagSeen, nil
	case "flagged", "starred":
		return imapFlagFlagged, nil
	case "answered", "replied":
		return imapFlagAnswered, nil
	case "draft":
		return imapFlagDraft, nil
	case "deleted":
		return imapFlagDeleted, nil
	default:
		return "", fmt.Errorf("unknown flag %q", flag)
	}
}

// MoveMessage relocates a message between folders.
func (m *EmailManager) MoveMessage(mailboxID uint, from, to string, uid uint32) error {
	account, err := m.accountForID(mailboxID)
	if err != nil {
		return err
	}
	if from == "" {
		from = inboxName
	}
	if to == "" {
		return fmt.Errorf("no destination folder")
	}
	if strings.EqualFold(from, to) {
		return nil
	}

	messages, err := m.store().List(account.Base, from)
	if err != nil {
		return err
	}
	for _, msg := range messages {
		if msg.UID != uid {
			continue
		}
		_, err := m.store().Move(account.Base, from, to, msg)
		return err
	}
	return fmt.Errorf("message not found")
}

// SaveDraft stores a composed message in Drafts so it survives leaving the
// page. Returns the new UID.
func (m *EmailManager) SaveDraft(mailboxID uint, msg *EmailMessage) (uint32, error) {
	account, err := m.accountForID(mailboxID)
	if err != nil {
		return 0, err
	}

	client := NewSMTPClient(m)
	raw, err := client.buildMIMEMessage(msg)
	if err != nil {
		return 0, err
	}

	return m.store().Deliver(account.Base, "Drafts", raw, []string{imapFlagDraft, imapFlagSeen}, time.Now())
}
