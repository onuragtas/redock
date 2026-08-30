package email_server

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"redock/platform/memory"
)

const (
	// defaultLogLimit / maxLogLimit bound how many entries are returned.
	defaultLogLimit = 200
	maxLogLimit     = 2000
	// defaultLogTail / maxLogTail bound the raw view.
	defaultLogTail = 1000
	maxLogTail     = 10000
)

// MailLogEntry is one mail transaction: a message that came in, went out, or
// was rejected. The engine records these as they happen, so nothing has to be
// parsed out of a log file.
type MailLogEntry struct {
	ID         string    `json:"id"`
	Timestamp  time.Time `json:"timestamp"`
	Direction  string    `json:"direction"` // in | out | system
	Service    string    `json:"service"`   // smtp, submission, imap, pop3, dashboard
	QueueID    string    `json:"queue_id,omitempty"`
	MessageID  string    `json:"message_id,omitempty"`
	From       string    `json:"from,omitempty"`
	To         []string  `json:"to,omitempty"`
	Subject    string    `json:"subject,omitempty"`
	Status     string    `json:"status,omitempty"` // delivered, sent, queued, deferred, bounced, rejected, login, auth-failed, error
	Detail     string    `json:"detail,omitempty"`
	RemoteIP   string    `json:"remote_ip,omitempty"`
	RemoteHost string    `json:"remote_host,omitempty"`
	Size       int64     `json:"size,omitempty"`
	MailboxID  uint      `json:"mailbox_id,omitempty"`
}

// MailLogQuery filters what the logs endpoint returns.
type MailLogQuery struct {
	Limit     int    // entries returned
	Direction string // in | out | system (empty = all)
	Status    string // exact status match (empty = all)
	Search    string // case-insensitive match on address/subject/detail
	// Tail bounds the raw view only; the parsed view is limited by Limit.
	Tail int
}

// MailLogResult is the payload handed to the dashboard.
type MailLogResult struct {
	Entries []MailLogEntry `json:"entries"`
	Source  string         `json:"source"`
	Total   int            `json:"total"`
	Stats   MailLogStats   `json:"stats"`
}

// MailLogStats summarises the whole retained window, not just the page.
type MailLogStats struct {
	Incoming  int `json:"incoming"`
	Outgoing  int `json:"outgoing"`
	Delivered int `json:"delivered"`
	Queued    int `json:"queued"`
	Rejected  int `json:"rejected"`
	Deferred  int `json:"deferred"`
	Bounced   int `json:"bounced"`
	AuthFail  int `json:"auth_failed"`
}

// GetMailLogs returns recent mail activity, newest first.
func (m *EmailManager) GetMailLogs(query MailLogQuery) (*MailLogResult, error) {
	limit := query.Limit
	if limit <= 0 {
		limit = defaultLogLimit
	}
	if limit > maxLogLimit {
		limit = maxLogLimit
	}

	entries := m.storedLogEntries()
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Timestamp.After(entries[j].Timestamp) })

	stats := MailLogStats{}
	filtered := make([]MailLogEntry, 0, limit)
	for _, entry := range entries {
		switch entry.Direction {
		case "in":
			stats.Incoming++
		case "out":
			stats.Outgoing++
		}
		switch entry.Status {
		case "delivered", "sent":
			stats.Delivered++
		case "queued":
			stats.Queued++
		case "rejected":
			stats.Rejected++
		case "deferred":
			stats.Deferred++
		case "bounced":
			stats.Bounced++
		case "auth-failed":
			stats.AuthFail++
		}

		if !matchesQuery(entry, query) {
			continue
		}
		if len(filtered) < limit {
			filtered = append(filtered, entry)
		}
	}

	return &MailLogResult{
		Entries: filtered,
		Source:  "native",
		Total:   len(entries),
		Stats:   stats,
	}, nil
}

// GetRawMailLog renders the retained events one line each, for operators who
// want to scan or copy them like a traditional mail log.
func (m *EmailManager) GetRawMailLog(tail int) ([]string, string, error) {
	if tail <= 0 {
		tail = defaultLogTail
	}
	if tail > maxLogTail {
		tail = maxLogTail
	}

	entries := m.storedLogEntries()
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Timestamp.Before(entries[j].Timestamp) })
	if len(entries) > tail {
		entries = entries[len(entries)-tail:]
	}

	lines := make([]string, 0, len(entries))
	for _, e := range entries {
		lines = append(lines, fmt.Sprintf("%s %s[%s] %s -> %s %s",
			e.Timestamp.Format("Jan _2 15:04:05"),
			e.Service,
			e.Direction,
			orEmpty(e.From, "-"),
			orEmpty(strings.Join(e.To, ","), "-"),
			strings.TrimSpace(e.Status+" "+e.Detail)))
	}
	return lines, "native", nil
}

func matchesQuery(e MailLogEntry, q MailLogQuery) bool {
	if q.Direction != "" && e.Direction != q.Direction {
		return false
	}
	if q.Status != "" && e.Status != q.Status {
		return false
	}
	if q.Search == "" {
		return true
	}

	needle := strings.ToLower(q.Search)
	haystack := []string{e.From, e.Subject, e.Detail, e.RemoteIP, e.RemoteHost, e.QueueID, e.MessageID, e.Service}
	haystack = append(haystack, e.To...)
	for _, h := range haystack {
		if h != "" && strings.Contains(strings.ToLower(h), needle) {
			return true
		}
	}
	return false
}

// storedLogEntries turns the email_logs table into log entries. The table is
// capped by the memory guard, so this is inherently bounded.
func (m *EmailManager) storedLogEntries() []MailLogEntry {
	if m.db == nil {
		return nil
	}

	rows := memory.FindAll[*EmailLog](m.db, "email_logs")
	out := make([]MailLogEntry, 0, len(rows))
	for _, row := range rows {
		if row == nil || row.IsDeleted() {
			continue
		}

		ts := row.Timestamp
		if ts.IsZero() {
			ts = row.CreatedAt
		}

		direction := "out"
		switch row.Type {
		case "received", "in":
			direction = "in"
		case "system":
			direction = "system"
		}

		entry := MailLogEntry{
			ID:         fmt.Sprintf("log-%d", row.GetID()),
			Timestamp:  ts,
			Direction:  direction,
			Service:    orEmpty(row.Service, "mail"),
			Status:     row.Status,
			Detail:     row.StatusMessage,
			From:       row.From,
			Subject:    row.Subject,
			Size:       row.Size,
			RemoteIP:   row.RemoteIP,
			RemoteHost: row.RemoteHost,
			QueueID:    row.QueueID,
			MailboxID:  row.MailboxID,
		}
		if row.To != "" {
			for _, addr := range strings.Split(row.To, ",") {
				if addr = strings.TrimSpace(addr); addr != "" {
					entry.To = append(entry.To, addr)
				}
			}
		}
		out = append(out, entry)
	}
	return out
}

func orEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
