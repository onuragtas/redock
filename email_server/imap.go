package email_server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"redock/platform/memory"
	"sort"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/mail"
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

	cl, err := client.Dial(addr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to IMAP: %w", err)
	}
	defer cl.Logout()
	defer cl.Close()

	if err := cl.Login(mailbox.Email, password); err != nil {
		return nil, fmt.Errorf("IMAP login failed: %w", err)
	}

	if folderPath == "" {
		folderPath = "INBOX"
	}

	mbox, err := cl.Select(folderPath, false)
	if err != nil {
		return nil, fmt.Errorf("failed to select folder %s: %w", folderPath, err)
	}

	if mbox.Messages == 0 {
		return []*Email{}, nil
	}

	start := uint32(1)
	end := mbox.Messages
	if limit > 0 && int(end) > limit {
		start = end - uint32(limit) + 1
	}

	bodySection := &imap.BodySectionName{}
	items := []imap.FetchItem{imap.FetchEnvelope, imap.FetchFlags, imap.FetchUid, bodySection.FetchItem()}

	seqSet := new(imap.SeqSet)
	seqSet.AddRange(start, end)

	messages := make(chan *imap.Message, 50)
	done := make(chan error, 1)
	go func() {
		done <- cl.Fetch(seqSet, items, messages)
	}()

	emails := make([]*Email, 0, int(end-start+1))
	for msg := range messages {
		email := &Email{Seen: false, Flagged: false}

		if msg.Uid > 0 {
			email.UID = msg.Uid
		}
		if msg.Envelope != nil {
			email.MessageID = msg.Envelope.MessageId
			email.Subject = msg.Envelope.Subject
			email.Date = time.Time(msg.Envelope.Date)
			email.InReplyTo = msg.Envelope.InReplyTo
			if len(msg.Envelope.From) > 0 {
				email.From = formatAddressV1(msg.Envelope.From[0])
			}
			if len(msg.Envelope.To) > 0 {
				toAddrs := make([]string, len(msg.Envelope.To))
				for i := range msg.Envelope.To {
					toAddrs[i] = formatAddressV1(msg.Envelope.To[i])
				}
				email.To = strings.Join(toAddrs, ", ")
			}
		}
		for _, flag := range msg.Flags {
			switch imap.CanonicalFlag(flag) {
			case imap.SeenFlag:
				email.Seen = true
			case imap.FlaggedFlag:
				email.Flagged = true
			}
		}
		literal := msg.GetBody(bodySection)
		if literal != nil {
			raw, err := io.ReadAll(literal)
			if err != nil {
				log.Printf("⚠️  Failed to read message body: %v", err)
			} else if len(raw) > 0 {
				plain, html, references, attCount := extractBodyFromRawMessage(raw)
				email.BodyPlain = plain
				email.BodyHTML = html
				email.References = references
				email.ThreadID = computeThreadID(email.MessageID, references, email.InReplyTo)
				email.AttachmentCount = attCount
				email.HasAttachments = attCount > 0
			}
		}
		emails = append(emails, email)
	}

	if err := <-done; err != nil {
		log.Printf("⚠️  IMAP fetch error: %v", err)
	}

	return emails, nil
}

// GetThread returns all emails in the same thread as the message with the given UID, sorted by date (oldest first).
func (c *IMAPClient) GetThread(mailboxID uint, folderPath string, threadUID uint32, _ int) ([]*Email, error) {
	const threadScanLimit = 5000
	all, err := c.GetMessages(mailboxID, folderPath, threadScanLimit)
	if err != nil {
		return nil, err
	}
	var seed *Email
	for i := range all {
		if all[i].UID == threadUID {
			seed = all[i]
			break
		}
	}
	if seed == nil {
		return []*Email{}, nil
	}
	wantedIDs := make(map[string]struct{})
	addIDs := func(refs, inReplyTo string) {
		for _, s := range parseMessageIDList(refs) {
			wantedIDs[s] = struct{}{}
		}
		for _, s := range parseMessageIDList(inReplyTo) {
			wantedIDs[s] = struct{}{}
		}
	}
	wantedIDs[normalizeID(seed.MessageID)] = struct{}{}
	addIDs(seed.References, seed.InReplyTo)
	for {
		prev := len(wantedIDs)
		for i := range all {
			mid := normalizeID(all[i].MessageID)
			if _, ok := wantedIDs[mid]; !ok {
				continue
			}
			addIDs(all[i].References, all[i].InReplyTo)
		}
		if len(wantedIDs) == prev {
			break
		}
	}
	var thread []*Email
	for i := range all {
		if _, ok := wantedIDs[normalizeID(all[i].MessageID)]; ok {
			thread = append(thread, all[i])
		}
	}
	sort.Slice(thread, func(i, j int) bool {
		return thread[i].Date.Before(thread[j].Date)
	})
	return thread, nil
}

func normalizeID(id string) string {
	return strings.Trim(strings.TrimSpace(id), "<>")
}

func parseMessageIDList(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var ids []string
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == '\t' }) {
		part = strings.Trim(part, "<>")
		if part != "" {
			ids = append(ids, part)
		}
	}
	return ids
}

func computeThreadID(messageID, references, inReplyTo string) string {
	ref := strings.TrimSpace(references)
	if ref != "" {
		first := ref
		if idx := strings.IndexAny(ref, " \t"); idx > 0 {
			first = ref[:idx]
		}
		return strings.Trim(first, "<>")
	}
	if s := strings.TrimSpace(inReplyTo); s != "" {
		return strings.Trim(s, "<>")
	}
	return strings.Trim(messageID, "<>")
}

// extractBodyFromRawMessage parses full raw RFC 5322 message; returns plain, html, References and attachment count.
func extractBodyFromRawMessage(raw []byte) (plain, html, references string, attachmentCount int) {
	mr, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return "", "", "", 0
	}
	defer mr.Close()

	references = mr.Header.Get("References")

	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		switch h := p.Header.(type) {
		case *mail.InlineHeader:
			ct, _, _ := h.ContentType()
			body, _ := io.ReadAll(p.Body)
			content := strings.TrimSpace(string(body))
			if content == "" {
				continue
			}
			if strings.HasPrefix(ct, "text/html") {
				html = content
			} else {
				plain = content
			}
		case *mail.AttachmentHeader:
			attachmentCount++
			_, _ = io.Copy(io.Discard, p.Body)
		}
	}
	return plain, html, references, attachmentCount
}

func formatAddressV1(addr *imap.Address) string {
	if addr == nil {
		return ""
	}
	email := addr.MailboxName + "@" + addr.HostName
	if addr.PersonalName != "" {
		return fmt.Sprintf("%s <%s>", addr.PersonalName, email)
	}
	return email
}

// normalizeSubjectForThread konu satırından "Re:", "Fwd:" vb. kaldırıp thread gruplamada kullanılacak anahtarı döner.
func normalizeSubjectForThread(subject string) string {
	s := strings.TrimSpace(subject)
	for {
		lower := strings.ToLower(s)
		trimmed := false
		if strings.HasPrefix(lower, "re:") {
			s = strings.TrimSpace(s[3:])
			trimmed = true
		}
		if strings.HasPrefix(strings.ToLower(s), "fwd:") {
			s = strings.TrimSpace(s[4:])
			trimmed = true
		}
		if !trimmed {
			break
		}
	}
	return strings.TrimSpace(s)
}

// GroupEmailsIntoThreads mailleri thread_id'ye göre gruplar; thread_id eksik/yanlışsa konu (Re: temizlenmiş) ile gruplar.
// Her thread tek bir root message_id ile etiketlenir; cevaplar da aynı thread_id'yi alır.
func GroupEmailsIntoThreads(emails []*Email) []*EmailThread {
	if len(emails) == 0 {
		return nil
	}
	// Önce konuya göre grupla (References/InReplyTo boş olan cevaplar için)
	bySubject := make(map[string][]*Email)
	for _, e := range emails {
		key := normalizeSubjectForThread(e.Subject)
		if key == "" {
			key = "(konu yok)"
		}
		bySubject[key] = append(bySubject[key], e)
	}
	// Her konu grubunda en eski mesajı root kabul et, thread_id'yi root'un message_id yap
	for _, group := range bySubject {
		sort.Slice(group, func(i, j int) bool { return group[i].Date.Before(group[j].Date) })
		rootID := strings.Trim(group[0].MessageID, "<>")
		for _, e := range group {
			e.ThreadID = rootID
		}
	}
	// thread_id'ye göre grupla, EmailThread olarak dön
	byThread := make(map[string][]*Email)
	for _, e := range emails {
		tid := e.ThreadID
		if tid == "" {
			tid = normalizeID(e.MessageID)
		}
		byThread[tid] = append(byThread[tid], e)
	}
	var threads []*EmailThread
	for tid, group := range byThread {
		sort.Slice(group, func(i, j int) bool { return group[i].Date.Before(group[j].Date) })
		latest := group[len(group)-1].Date
		subject := group[0].Subject
		if subject == "" {
			subject = "(konu yok)"
		}
		threads = append(threads, &EmailThread{
			ThreadID: tid,
			Subject:  subject,
			Date:     latest,
			Count:    len(group),
			Messages: group,
		})
	}
	sort.Slice(threads, func(i, j int) bool { return threads[i].Date.After(threads[j].Date) })
	return threads
}

type IMAPFolder struct {
	Name          string   `json:"name"`
	Path          string   `json:"path"`
	Attributes    []string `json:"attributes"`
	Delimiter     string   `json:"delimiter"`
	HasChildren   bool     `json:"has_children"`
	NoSelect      bool     `json:"no_select"`
	MessageCount  uint32   `json:"message_count"`
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

	cl, err := client.Dial(addr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to IMAP: %w", err)
	}
	defer cl.Logout()
	defer cl.Close()

	if err := cl.Login(mailbox.Email, password); err != nil {
		return nil, fmt.Errorf("IMAP login failed: %w", err)
	}

	mailboxes := make(chan *imap.MailboxInfo, 50)
	done := make(chan error, 1)
	go func() {
		done <- cl.List("", "*", mailboxes)
	}()

	var folders []*IMAPFolder
	for m := range mailboxes {
		folder := &IMAPFolder{
			Name:       m.Name,
			Path:       m.Name,
			Attributes: m.Attributes,
			Delimiter:  m.Delimiter,
		}
		for _, attr := range m.Attributes {
			attrStr := strings.ToLower(attr)
			if attrStr == "\\haschildren" {
				folder.HasChildren = true
			}
			if attrStr == "\\noselect" {
				folder.NoSelect = true
			}
		}
		if !folder.NoSelect {
			if status, err := cl.Status(m.Name, []imap.StatusItem{imap.StatusMessages}); err == nil {
				if n, ok := status.Items[imap.StatusMessages].(uint32); ok {
					folder.MessageCount = n
				}
			}
		}
		folders = append(folders, folder)
	}

	if err := <-done; err != nil {
		return nil, fmt.Errorf("failed to list folders: %w", err)
	}

	return folders, nil
}

func (c *IMAPClient) appendToFolder(email, password, folderName string, message []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	config := c.manager.config
	addr := fmt.Sprintf("localhost:%d", config.IMAPPort)

	errChan := make(chan error, 1)
	go func() {
		cl, err := client.Dial(addr)
		if err != nil {
			errChan <- fmt.Errorf("failed to connect to IMAP: %w", err)
			return
		}
		defer cl.Logout()
		defer cl.Close()

		if err := cl.Login(email, password); err != nil {
			errChan <- fmt.Errorf("IMAP login failed: %w", err)
			return
		}

		literal := bytes.NewBuffer(message)
		if err := cl.Append(folderName, []string{imap.SeenFlag}, time.Now(), literal); err != nil {
			errChan <- fmt.Errorf("failed to append to %s: %w", folderName, err)
			return
		}
		errChan <- nil
	}()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return fmt.Errorf("IMAP append timeout after 10 seconds")
	}
}
