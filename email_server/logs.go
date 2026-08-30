package email_server

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"

	"redock/platform/memory"
)

const (
	// defaultLogTail / maxLogTail bound how many container log lines are read.
	// The mail log of a busy server is unbounded on disk; the dashboard only
	// ever needs the recent tail, and reading more would just burn memory.
	defaultLogTail = 1000
	maxLogTail     = 10000
	// maxLogBytes caps the raw log read regardless of the line count.
	maxLogBytes = 8 << 20 // 8 MB
	// maxRawLinesPerEntry keeps the per-message raw excerpt small.
	maxRawLinesPerEntry = 12
	// defaultLogLimit / maxLogLimit bound how many entries are returned.
	defaultLogLimit = 200
	maxLogLimit     = 2000
)

// MailLogEntry is one mail transaction assembled from the mail log: a message
// that came in, went out, or was rejected, plus the lines it was built from.
type MailLogEntry struct {
	ID         string    `json:"id"`
	Timestamp  time.Time `json:"timestamp"`
	Direction  string    `json:"direction"` // in | out | system
	Service    string    `json:"service"`   // postfix/smtp, postfix/smtpd, dovecot, redock
	QueueID    string    `json:"queue_id,omitempty"`
	MessageID  string    `json:"message_id,omitempty"`
	From       string    `json:"from,omitempty"`
	To         []string  `json:"to,omitempty"`
	Subject    string    `json:"subject,omitempty"`
	Status     string    `json:"status,omitempty"` // sent, bounced, deferred, rejected, received, login, error
	Detail     string    `json:"detail,omitempty"`
	RemoteIP   string    `json:"remote_ip,omitempty"`
	RemoteHost string    `json:"remote_host,omitempty"`
	Size       int64     `json:"size,omitempty"`
	Raw        []string  `json:"raw,omitempty"`
}

// MailLogQuery filters what the logs endpoint returns.
type MailLogQuery struct {
	Tail      int    // container log lines to scan
	Limit     int    // entries returned
	Direction string // in | out | system (empty = all)
	Status    string // exact status match (empty = all)
	Search    string // case-insensitive match on address/subject/detail
}

// MailLogResult is the payload handed to the dashboard.
type MailLogResult struct {
	Entries   []MailLogEntry `json:"entries"`
	Source    string         `json:"source"`
	LinesRead int            `json:"lines_read"`
	Stats     MailLogStats   `json:"stats"`
}

// MailLogStats summarises the scanned window.
type MailLogStats struct {
	Incoming int `json:"incoming"`
	Outgoing int `json:"outgoing"`
	Rejected int `json:"rejected"`
	Deferred int `json:"deferred"`
	Bounced  int `json:"bounced"`
}

// Syslog line: "Feb 12 10:23:45 mail postfix/smtp[140]: A1B2C3: to=<...>",
// optionally prefixed by the RFC3339 timestamp the Docker API adds.
var logLineRe = regexp.MustCompile(`^(?:(\d{4}-\d{2}-\d{2}T[\d:.]+Z?\S*)\s+)?(\w{3}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2})\s+\S+\s+([\w./-]+?)(?:\[\d+\])?:\s+(.*)$`)

var (
	clientRe      = regexp.MustCompile(`client=([^\[\s]+)\[([0-9a-fA-F.:]+)\]`)
	messageIDRe   = regexp.MustCompile(`message-id=<?([^>\s,]+)>?`)
	fromSizeRe    = regexp.MustCompile(`from=<([^>]*)>,\s*size=(\d+)`)
	toStatusRe    = regexp.MustCompile(`to=<([^>]*)>.*?status=(\w+)\s*\((.*)\)\s*$`)
	relayRe       = regexp.MustCompile(`relay=([^,\s]+)`)
	rejectRe      = regexp.MustCompile(`reject(?:_warning)?:\s+\w+\s+from\s+([^\[\s]+)\[([0-9a-fA-F.:]+)\]:\s*(.*?)(?:;|$)`)
	rejectAddrRe  = regexp.MustCompile(`from=<([^>]*)>\s+to=<([^>]*)>`)
	dovecotLogin  = regexp.MustCompile(`(imap|pop3|managesieve)-login: Login: user=<([^>]*)>.*?rip=([0-9a-fA-F.:]+)`)
	dovecotSaved  = regexp.MustCompile(`msgid=<?([^>\s]+)>?:\s*saved mail to (\S+)`)
	localhostAddr = regexp.MustCompile(`^(127\.|::1$|localhost$)`)
)

// GetMailLogs reads the mail server's log, turns it into per-message entries and
// merges in the deliveries this dashboard itself performed.
func (m *EmailManager) GetMailLogs(query MailLogQuery) (*MailLogResult, error) {
	tail := query.Tail
	if tail <= 0 {
		tail = defaultLogTail
	}
	if tail > maxLogTail {
		tail = maxLogTail
	}

	limit := query.Limit
	if limit <= 0 {
		limit = defaultLogLimit
	}
	if limit > maxLogLimit {
		limit = maxLogLimit
	}

	lines, source, err := m.readMailLogLines(tail)
	if err != nil {
		return nil, err
	}

	entries := parseMailLog(lines)
	entries = append(entries, m.dashboardSentEntries()...)

	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Timestamp.After(entries[j].Timestamp) })

	stats := MailLogStats{}
	filtered := make([]MailLogEntry, 0, limit)
	for i := range entries {
		e := entries[i]
		switch {
		case e.Status == "rejected":
			stats.Rejected++
		case e.Status == "deferred":
			stats.Deferred++
		case e.Status == "bounced":
			stats.Bounced++
		}
		switch e.Direction {
		case "in":
			stats.Incoming++
		case "out":
			stats.Outgoing++
		}

		if !matchesQuery(e, query) {
			continue
		}
		if len(filtered) < limit {
			filtered = append(filtered, e)
		}
	}

	return &MailLogResult{
		Entries:   filtered,
		Source:    source,
		LinesRead: len(lines),
		Stats:     stats,
	}, nil
}

// GetRawMailLog returns the unparsed tail of the mail log.
func (m *EmailManager) GetRawMailLog(tail int) ([]string, string, error) {
	if tail <= 0 {
		tail = defaultLogTail
	}
	if tail > maxLogTail {
		tail = maxLogTail
	}
	return m.readMailLogLines(tail)
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
	haystack := []string{e.From, e.Subject, e.Detail, e.RemoteIP, e.RemoteHost, e.QueueID, e.MessageID}
	haystack = append(haystack, e.To...)
	for _, h := range haystack {
		if h != "" && strings.Contains(strings.ToLower(h), needle) {
			return true
		}
	}
	return false
}

// readMailLogLines pulls the tail of the mail log, preferring the container's
// stdout (no exec needed) and falling back to /var/log/mail.log inside it.
func (m *EmailManager) readMailLogLines(tail int) ([]string, string, error) {
	containerID := m.resolveLogContainerID()
	if containerID == "" {
		return nil, "", fmt.Errorf("mail server container is not running")
	}

	lines, err := m.readContainerStdout(containerID, tail)
	if err == nil && countParsableLines(lines) > 0 {
		return lines, "container", nil
	}

	fallback, fallbackErr := m.readContainerMailLogFile(tail)
	if fallbackErr == nil && len(fallback) > 0 {
		return fallback, "mail.log", nil
	}

	if err != nil && fallbackErr != nil {
		return nil, "", fmt.Errorf("could not read mail logs: %v (fallback: %v)", err, fallbackErr)
	}
	// stdout was readable but held nothing recognisable; report it as-is.
	return lines, "container", nil
}

// resolveLogContainerID returns the mail container's ID, looking it up by name
// when the stored ID is empty or stale (e.g. the container was recreated while
// the dashboard was running).
func (m *EmailManager) resolveLogContainerID() string {
	if id := m.getContainerID(); id != "" {
		return id
	}
	if m.config == nil || m.dockerClient == nil {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	containers, err := m.dockerClient.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return ""
	}
	for _, c := range containers {
		for _, name := range c.Names {
			if name == "/"+m.config.ContainerName {
				return c.ID
			}
		}
	}
	return ""
}

func (m *EmailManager) readContainerStdout(containerID string, tail int) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	reader, err := m.dockerClient.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Timestamps: true,
		Tail:       strconv.Itoa(tail),
	})
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	var out bytes.Buffer
	// The container runs without a TTY, so the stream is multiplexed.
	if _, err := stdcopy.StdCopy(&out, &out, io.LimitReader(reader, maxLogBytes)); err != nil && out.Len() == 0 {
		return nil, err
	}

	return splitLines(out.Bytes()), nil
}

func (m *EmailManager) readContainerMailLogFile(tail int) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "exec", m.config.ContainerName,
		"tail", "-n", strconv.Itoa(tail), "/var/log/mail.log")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	if len(out) > maxLogBytes {
		out = out[len(out)-maxLogBytes:]
	}
	return splitLines(out), nil
}

func splitLines(data []byte) []string {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)

	lines := make([]string, 0, 256)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func countParsableLines(lines []string) int {
	n := 0
	for _, l := range lines {
		if logLineRe.MatchString(l) {
			n++
		}
	}
	return n
}

// parseMailLog aggregates log lines into one entry per message (keyed by the
// Postfix queue ID), plus standalone entries for rejections and logins.
func parseMailLog(lines []string) []MailLogEntry {
	byQueue := make(map[string]*MailLogEntry)
	order := make([]*MailLogEntry, 0, 64)

	for _, line := range lines {
		match := logLineRe.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		dockerTS, syslogTS, service, message := match[1], match[2], match[3], match[4]
		ts := parseLogTime(dockerTS, syslogTS)

		// Rejections never get a queue ID: they are their own entry.
		if strings.Contains(message, "reject") && strings.HasPrefix(message, "NOQUEUE") {
			if entry := parseRejection(ts, service, message, line); entry != nil {
				order = append(order, entry)
			}
			continue
		}

		if strings.HasPrefix(service, "dovecot") {
			if entry := parseDovecot(ts, service, message, line); entry != nil {
				order = append(order, entry)
			}
			continue
		}

		queueID, rest, ok := splitQueueLine(message)
		if !ok {
			continue
		}

		entry, ok := byQueue[queueID]
		if !ok {
			entry = &MailLogEntry{
				ID:        "q-" + queueID,
				QueueID:   queueID,
				Timestamp: ts,
				Service:   service,
				Direction: "out", // refined below once we see how it was handled
			}
			byQueue[queueID] = entry
			order = append(order, entry)
		}
		if ts.After(entry.Timestamp) {
			entry.Timestamp = ts
		}
		applyQueueLine(entry, service, rest, line)
	}

	out := make([]MailLogEntry, 0, len(order))
	for _, e := range order {
		finalizeEntry(e)
		out = append(out, *e)
	}
	return out
}

// splitQueueLine separates a Postfix queue ID from the rest of the message.
// Queue IDs are short alphanumeric tokens containing at least one digit (both
// the classic hex form and the long base-36 form), which is what keeps plain
// keyword lines such as "warning: ..." or "statistics: ..." from being taken
// for a message.
func splitQueueLine(message string) (queueID, rest string, ok bool) {
	idx := strings.Index(message, ": ")
	if idx <= 0 {
		return "", "", false
	}

	token := message[:idx]
	if len(token) < 6 || len(token) > 20 {
		return "", "", false
	}

	hasDigit := false
	for _, r := range token {
		switch {
		case r >= '0' && r <= '9':
			hasDigit = true
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
		default:
			return "", "", false
		}
	}
	if !hasDigit {
		return "", "", false
	}

	return token, message[idx+2:], true
}

// applyQueueLine folds one "QUEUEID: ..." line into the message's entry.
func applyQueueLine(entry *MailLogEntry, service, rest, raw string) {
	if len(entry.Raw) < maxRawLinesPerEntry {
		entry.Raw = append(entry.Raw, raw)
	}

	if c := clientRe.FindStringSubmatch(rest); c != nil {
		entry.RemoteHost = c[1]
		entry.RemoteIP = c[2]
		// A message the server accepted over SMTP from a remote client is
		// incoming; one submitted from localhost is our own outgoing mail.
		if localhostAddr.MatchString(entry.RemoteIP) || localhostAddr.MatchString(entry.RemoteHost) {
			entry.Direction = "out"
		} else {
			entry.Direction = "in"
		}
	}
	if mid := messageIDRe.FindStringSubmatch(rest); mid != nil && entry.MessageID == "" {
		entry.MessageID = strings.Trim(mid[1], "<>")
	}
	if fs := fromSizeRe.FindStringSubmatch(rest); fs != nil {
		entry.From = fs[1]
		if size, err := strconv.ParseInt(fs[2], 10, 64); err == nil {
			entry.Size = size
		}
	}
	if ts := toStatusRe.FindStringSubmatch(rest); ts != nil {
		recipient := ts[1]
		if recipient != "" && !contains(entry.To, recipient) {
			entry.To = append(entry.To, recipient)
		}
		entry.Status = normalizeStatus(ts[2])
		entry.Detail = strings.TrimSpace(ts[3])
		entry.Service = service

		// Delivery through Dovecot/LMTP means it landed in a local mailbox.
		if relay := relayRe.FindStringSubmatch(rest); relay != nil {
			target := strings.ToLower(relay[1])
			if strings.Contains(target, "dovecot") || strings.Contains(target, "lmtp") || target == "local" {
				entry.Direction = "in"
			}
		}
		if service == "postfix/local" || service == "postfix/lmtp" {
			entry.Direction = "in"
		}
	}
}

func parseRejection(ts time.Time, service, message, raw string) *MailLogEntry {
	rm := rejectRe.FindStringSubmatch(message)
	if rm == nil {
		return nil
	}

	entry := &MailLogEntry{
		ID:         fmt.Sprintf("rej-%d-%s", ts.UnixNano(), rm[2]),
		Timestamp:  ts,
		Direction:  "in",
		Service:    service,
		Status:     "rejected",
		RemoteHost: rm[1],
		RemoteIP:   rm[2],
		Detail:     strings.TrimSpace(rm[3]),
		Raw:        []string{raw},
	}
	if addr := rejectAddrRe.FindStringSubmatch(message); addr != nil {
		entry.From = addr[1]
		if addr[2] != "" {
			entry.To = []string{addr[2]}
		}
	}
	return entry
}

func parseDovecot(ts time.Time, service, message, raw string) *MailLogEntry {
	if lm := dovecotLogin.FindStringSubmatch(message); lm != nil {
		return &MailLogEntry{
			ID:        fmt.Sprintf("login-%d-%s", ts.UnixNano(), lm[3]),
			Timestamp: ts,
			Direction: "system",
			Service:   service + "/" + lm[1],
			Status:    "login",
			From:      lm[2],
			RemoteIP:  lm[3],
			Detail:    fmt.Sprintf("%s login", strings.ToUpper(lm[1])),
			Raw:       []string{raw},
		}
	}
	if sm := dovecotSaved.FindStringSubmatch(message); sm != nil {
		return &MailLogEntry{
			ID:        fmt.Sprintf("saved-%d", ts.UnixNano()),
			Timestamp: ts,
			Direction: "in",
			Service:   service,
			Status:    "delivered",
			MessageID: strings.Trim(sm[1], "<>"),
			Detail:    "saved to " + sm[2],
			Raw:       []string{raw},
		}
	}
	return nil
}

// finalizeEntry fills in whatever the individual lines left undecided.
func finalizeEntry(e *MailLogEntry) {
	if e.Status == "" {
		e.Status = "received"
	}
	if e.Direction == "" {
		e.Direction = "out"
	}
}

// normalizeStatus maps Postfix status words onto the set the UI colours.
func normalizeStatus(status string) string {
	switch strings.ToLower(status) {
	case "sent":
		return "sent"
	case "bounced":
		return "bounced"
	case "deferred":
		return "deferred"
	case "expired":
		return "expired"
	default:
		return strings.ToLower(status)
	}
}

// dashboardSentEntries surfaces the mail this dashboard sent itself, which is
// recorded in the email_logs table rather than in the container's log.
func (m *EmailManager) dashboardSentEntries() []MailLogEntry {
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
		if row.Type == "received" || row.Type == "in" {
			direction = "in"
		}

		entry := MailLogEntry{
			ID:         fmt.Sprintf("redock-%d", row.GetID()),
			Timestamp:  ts,
			Direction:  direction,
			Service:    "redock",
			From:       row.From,
			Subject:    row.Subject,
			Status:     row.Status,
			Detail:     row.StatusMessage,
			RemoteIP:   row.RemoteIP,
			RemoteHost: row.RemoteHost,
			Size:       row.Size,
			QueueID:    row.QueueID,
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

// parseLogTime prefers the Docker timestamp (it carries the year) and falls
// back to the syslog stamp, which does not.
func parseLogTime(dockerTS, syslogTS string) time.Time {
	if dockerTS != "" {
		if ts, err := time.Parse(time.RFC3339Nano, dockerTS); err == nil {
			return ts.Local()
		}
	}
	if ts, err := time.Parse(time.Stamp, syslogTS); err == nil {
		now := time.Now()
		withYear := time.Date(now.Year(), ts.Month(), ts.Day(), ts.Hour(), ts.Minute(), ts.Second(), 0, time.Local)
		// A stamp in the future means the log rolled over from last year.
		if withYear.After(now.Add(24 * time.Hour)) {
			withYear = withYear.AddDate(-1, 0, 0)
		}
		return withYear
	}
	return time.Now()
}

func contains(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}
