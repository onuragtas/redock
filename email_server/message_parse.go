package email_server

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
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

// subjectReplyPrefixes are reply/forward markers stripped before subjects are
// compared ("Re:", "Fwd:", Outlook's "FW:" and Turkish "YNT:").
var subjectReplyPrefixes = []string{"re:", "fwd:", "fw:", "ynt:"}

// normalizeSubjectForThread konu satırından "Re:", "Fwd:" vb. kaldırıp thread gruplamada kullanılacak anahtarı döner.
func normalizeSubjectForThread(subject string) string {
	s := strings.TrimSpace(subject)
	for {
		lower := strings.ToLower(s)
		trimmed := false
		for _, prefix := range subjectReplyPrefixes {
			if strings.HasPrefix(lower, prefix) {
				s = strings.TrimSpace(s[len(prefix):])
				trimmed = true
				break
			}
		}
		if !trimmed {
			break
		}
	}
	return strings.TrimSpace(s)
}

var emailAddressPattern = regexp.MustCompile(`[^\s<>,;"']+@[^\s<>,;"']+`)

// threadParticipants returns the sorted, de-duplicated From+To addresses. The
// set is the same for a message and its answer (B→A and A→B), so it pairs an
// Inbox message with the Sent reply while keeping mails to different people apart.
func threadParticipants(e *Email) string {
	seen := make(map[string]bool)
	var list []string
	for _, addr := range emailAddressPattern.FindAllString(e.From+" "+e.To, -1) {
		addr = strings.ToLower(addr)
		if !seen[addr] {
			seen[addr] = true
			list = append(list, addr)
		}
	}
	sort.Strings(list)
	return strings.Join(list, ",")
}

// assignThreadIDs sets ThreadID on every email so that one conversation shares
// one ID. Messages are linked through Message-ID / In-Reply-To / References;
// a message without reply headers falls back to "normalized subject + the
// same participants", so unrelated mails that merely share a subject (or have
// none) to different people stay separate. The thread ID is the Message-ID of
// the oldest message in the conversation.
func assignThreadIDs(emails []*Email) {
	parent := make(map[string]string)
	var find func(string) string
	find = func(x string) string {
		p, ok := parent[x]
		if !ok || p == x {
			parent[x] = x
			return x
		}
		root := find(p)
		parent[x] = root
		return root
	}
	union := func(a, b string) {
		if ra, rb := find(a), find(b); ra != rb {
			parent[ra] = rb
		}
	}

	nodes := make([]string, len(emails))
	for i, e := range emails {
		node := fmt.Sprintf("e:%d", i)
		nodes[i] = node
		find(node)

		if id := normalizeID(e.MessageID); id != "" {
			union(node, "id:"+id)
		}
		refs := parseMessageIDList(e.References)
		if id := normalizeID(e.InReplyTo); id != "" {
			refs = append(refs, id)
		}
		for _, ref := range refs {
			union(node, "id:"+ref)
		}
		if len(refs) == 0 {
			key := strings.ToLower(normalizeSubjectForThread(e.Subject))
			union(node, "subj:"+key+"|"+threadParticipants(e))
		}
	}

	groups := make(map[string][]*Email)
	for i, e := range emails {
		root := find(nodes[i])
		groups[root] = append(groups[root], e)
	}
	for _, group := range groups {
		sort.SliceStable(group, func(i, j int) bool { return group[i].Date.Before(group[j].Date) })
		tid := ""
		for _, e := range group {
			if tid = normalizeID(e.MessageID); tid != "" {
				break
			}
		}
		if tid == "" {
			tid = fmt.Sprintf("uid:%d", group[0].UID)
		}
		for _, e := range group {
			e.ThreadID = tid
		}
	}
}

// GroupEmailsIntoThreads mailleri konuşmalara gruplar (bkz. assignThreadIDs);
// en yeni konuşma başta olacak şekilde döner.
func GroupEmailsIntoThreads(emails []*Email) []*EmailThread {
	if len(emails) == 0 {
		return nil
	}
	assignThreadIDs(emails)

	byThread := make(map[string][]*Email)
	for _, e := range emails {
		byThread[e.ThreadID] = append(byThread[e.ThreadID], e)
	}
	threads := make([]*EmailThread, 0, len(byThread))
	for tid, group := range byThread {
		sort.SliceStable(group, func(i, j int) bool { return group[i].Date.Before(group[j].Date) })
		subject := group[0].Subject
		if subject == "" {
			subject = "(konu yok)"
		}
		threads = append(threads, &EmailThread{
			ThreadID: tid,
			Subject:  subject,
			Date:     group[len(group)-1].Date,
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
