package email_server

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-message/mail"
)

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
	Name         string   `json:"name"`
	Path         string   `json:"path"`
	Attributes   []string `json:"attributes"`
	Delimiter    string   `json:"delimiter"`
	HasChildren  bool     `json:"has_children"`
	NoSelect     bool     `json:"no_select"`
	MessageCount uint32   `json:"message_count"`
	UnseenCount  uint32   `json:"unseen_count"`
}
