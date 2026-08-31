package email_server

import (
	"crypto/tls"
	"net"
	"strings"
	"sync"
	"time"
)

// Connection tracing. The mail log records what happened to *messages*; this
// records what happened on the *wire*, so an attempt that never became a
// message — a client that refused our certificate, a probe that hung up after
// EHLO, a wrong password — is still fully visible.
//
// Traces live in a bounded in-memory ring rather than the database: they are
// verbose by nature and only useful while recent.

const (
	// maxTracedConnections is how many connections are kept.
	maxTracedConnections = 300
	// maxTraceLines / maxTraceBytes bound a single connection's trace.
	maxTraceLines = 400
	maxTraceBytes = 64 * 1024
)

// TraceLine is one line of protocol conversation.
type TraceLine struct {
	Timestamp time.Time `json:"timestamp"`
	// Direction is "in" for what the client sent and "out" for our answer.
	Direction string `json:"direction"`
	Text      string `json:"text"`
}

// ConnectionTrace is everything known about one connection.
type ConnectionTrace struct {
	ID         uint64      `json:"id"`
	Service    string      `json:"service"`
	RemoteIP   string      `json:"remote_ip"`
	RemotePort string      `json:"remote_port,omitempty"`
	StartedAt  time.Time   `json:"started_at"`
	EndedAt    *time.Time  `json:"ended_at,omitempty"`
	TLS        string      `json:"tls"` // none, starttls, implicit
	Encrypted  bool        `json:"encrypted"`
	Error      string      `json:"error,omitempty"`
	Lines      []TraceLine `json:"lines"`
	Truncated  bool        `json:"truncated"`
}

// traceStore is the bounded ring of connection traces.
type traceStore struct {
	mu      sync.RWMutex
	entries []*ConnectionTrace
	pos     int
	length  int
	// live maps connection id → trace for connections still open.
	live map[uint64]*ConnectionTrace
}

func newTraceStore() *traceStore {
	return &traceStore{
		entries: make([]*ConnectionTrace, maxTracedConnections),
		live:    make(map[uint64]*ConnectionTrace),
	}
}

func (s *traceStore) begin(trace *ConnectionTrace) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries[s.pos] = trace
	s.pos = (s.pos + 1) % maxTracedConnections
	if s.length < maxTracedConnections {
		s.length++
	}
	s.live[trace.ID] = trace
}

func (s *traceStore) end(id uint64, err string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	trace, ok := s.live[id]
	if !ok {
		return
	}
	now := time.Now()
	trace.EndedAt = &now
	if err != "" {
		trace.Error = err
	}
	delete(s.live, id)
}

func (s *traceStore) append(id uint64, direction, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	trace, ok := s.live[id]
	if !ok {
		return
	}
	if len(trace.Lines) >= maxTraceLines {
		trace.Truncated = true
		return
	}
	trace.Lines = append(trace.Lines, TraceLine{Timestamp: time.Now(), Direction: direction, Text: text})
}

func (s *traceStore) note(id uint64, text string) { s.append(id, "note", text) }

func (s *traceStore) markEncrypted(id uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if trace, ok := s.live[id]; ok {
		trace.Encrypted = true
		if trace.TLS == "none" {
			trace.TLS = "starttls"
		}
	}
}

// List returns the traces, newest first.
func (s *traceStore) List(limit int) []ConnectionTrace {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > s.length {
		limit = s.length
	}

	out := make([]ConnectionTrace, 0, limit)
	for i := 0; i < limit; i++ {
		idx := (s.pos - 1 - i + maxTracedConnections*2) % maxTracedConnections
		trace := s.entries[idx]
		if trace == nil {
			continue
		}
		// Copy so a live connection cannot mutate what the caller reads.
		snapshot := *trace
		snapshot.Lines = append([]TraceLine(nil), trace.Lines...)
		out = append(out, snapshot)
	}
	return out
}

// trim reduces what the store holds to the newest `keep` connections, and
// reports how many traces were dropped and how many were emptied.
//
// A connection that is still open cannot be dropped — its trace is still being
// written to, and losing it would lose the conversation happening right now.
// IMAP clients hold a connection open for hours, so on a busy server most of
// what is stored can be live; those traces keep their place in the ring but
// give up the lines they have accumulated, which is where the memory actually
// sits.
func (s *traceStore) trim(keep int) (dropped, emptied int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if keep < 0 {
		keep = 0
	}
	if s.length == 0 {
		return 0, 0
	}

	// Walk newest first, the order List reports, and keep that many.
	kept := make([]*ConnectionTrace, 0, s.length)
	for i := 0; i < s.length; i++ {
		idx := (s.pos - 1 - i + maxTracedConnections*2) % maxTracedConnections
		trace := s.entries[idx]
		s.entries[idx] = nil
		if trace == nil {
			continue
		}

		if len(kept) < keep {
			kept = append(kept, trace)
			continue
		}

		if _, live := s.live[trace.ID]; live {
			if len(trace.Lines) > 0 {
				trace.Lines = nil
				trace.Truncated = true
				emptied++
			}
			kept = append(kept, trace)
			continue
		}
		dropped++
	}

	// Rebuild the ring oldest first so pos/length stay consistent with begin().
	s.pos = 0
	s.length = 0
	for i := len(kept) - 1; i >= 0; i-- {
		s.entries[s.pos] = kept[i]
		s.pos = (s.pos + 1) % maxTracedConnections
		s.length++
	}
	return dropped, emptied
}

// tracedListener wraps every accepted connection so the whole conversation is
// recorded, whichever protocol the listener serves.
type tracedListener struct {
	net.Listener
	manager *EmailManager
	service string
	// implicitTLS marks listeners that are encrypted from the first byte, where
	// no plaintext protocol can be recorded.
	implicitTLS bool
}

func (l *tracedListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}

	host, port := splitHostPort(conn.RemoteAddr())

	// A blocked or too-eager address never gets a protocol conversation.
	if !l.manager.allowConnection(l.service, host) {
		conn.Close()
		return l.Accept()
	}

	id := connectionCounter.Add(1)

	tlsMode := "none"
	if l.implicitTLS {
		tlsMode = "implicit"
	}

	trace := &ConnectionTrace{
		ID:         id,
		Service:    l.service,
		RemoteIP:   host,
		RemotePort: port,
		StartedAt:  time.Now(),
		TLS:        tlsMode,
		Encrypted:  l.implicitTLS,
	}
	l.manager.traces().begin(trace)

	l.manager.logMailEvent(mailEvent{
		Direction: "system",
		Status:    "connect",
		Service:   l.service,
		RemoteIP:  host,
		QueueID:   connKey(id),
		Detail:    "connection opened",
	})

	return &tracedConn{
		Conn:    conn,
		id:      id,
		manager: l.manager,
		service: l.service,
		remote:  host,
		// An implicit-TLS listener hands us ciphertext, so raw recording is off
		// from the start; the session-level events carry the content instead.
		recording: !l.implicitTLS,
	}, nil
}

// tracedConn records the plaintext phase of a connection and reports how it
// ended. Once TLS takes over, the bytes are ciphertext, so recording stops and
// the trace says so — the decrypted commands still show up as session events.
type tracedConn struct {
	net.Conn
	id      uint64
	manager *EmailManager
	service string
	remote  string

	mu        sync.Mutex
	recording bool
	encrypted bool
	bytesIn   int
	bytesOut  int
	closed    bool
	lastErr   string
	// expectSecret marks that the next line from the client carries SASL
	// credentials: they arrive as bare base64 on their own line after an AUTH
	// command or a 334 challenge, and must never be written to a trace.
	expectSecret bool
}

func (c *tracedConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	if n > 0 {
		c.capture("in", b[:n])
	}
	if err != nil && !isNormalClose(err) {
		c.setError(err.Error())
	}
	return n, err
}

func (c *tracedConn) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	if n > 0 {
		c.capture("out", b[:n])
	}
	if err != nil && !isNormalClose(err) {
		c.setError(err.Error())
	}
	return n, err
}

// capture turns raw bytes into protocol lines, and notices when the stream
// turns into TLS records.
func (c *tracedConn) capture(direction string, data []byte) {
	c.mu.Lock()
	if direction == "in" {
		c.bytesIn += len(data)
	} else {
		c.bytesOut += len(data)
	}

	if !c.recording {
		c.mu.Unlock()
		return
	}

	// A TLS record starts with 0x16 (handshake) and a version byte: that is the
	// moment the conversation stops being readable.
	if direction == "in" && len(data) > 2 && data[0] == 0x16 && data[1] == 0x03 {
		c.recording = false
		c.encrypted = true
		c.mu.Unlock()
		c.manager.traces().markEncrypted(c.id)
		c.manager.traces().note(c.id, "— TLS handshake begins; the rest of this connection is encrypted —")
		return
	}

	if c.bytesIn+c.bytesOut > maxTraceBytes {
		c.recording = false
		c.mu.Unlock()
		c.manager.traces().note(c.id, "— trace truncated —")
		return
	}
	c.mu.Unlock()

	for _, line := range splitProtocolLines(data) {
		c.manager.traces().append(c.id, direction, c.redact(direction, line))
	}
}

// redact hides SASL credentials, which travel on their own line rather than as
// part of the command. Everything else is recorded verbatim.
func (c *tracedConn) redact(direction, line string) string {
	c.mu.Lock()
	defer c.mu.Unlock()

	upper := strings.ToUpper(strings.TrimSpace(line))

	if direction == "out" {
		// A 334 is the server asking for the credential line.
		if strings.HasPrefix(upper, "334") {
			c.expectSecret = true
		}
		return line
	}

	if c.expectSecret {
		c.expectSecret = false
		return "[credentials hidden]"
	}
	if strings.HasPrefix(upper, "AUTH") {
		// Either the credentials are inline (handled by sanitizeLine) or they
		// follow on the next line.
		c.expectSecret = true
	}
	return line
}

func (c *tracedConn) setError(message string) {
	c.mu.Lock()
	if c.lastErr == "" {
		c.lastErr = message
	}
	c.mu.Unlock()
}

func (c *tracedConn) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return c.Conn.Close()
	}
	c.closed = true
	encrypted := c.encrypted
	lastErr := c.lastErr
	bytesIn, bytesOut := c.bytesIn, c.bytesOut
	c.mu.Unlock()

	c.manager.traces().end(c.id, lastErr)

	detail := formatConnSummary(encrypted, bytesIn, bytesOut, lastErr)
	status := "disconnect"
	if lastErr != "" {
		status = "conn-error"
	}

	c.manager.logMailEvent(mailEvent{
		Direction: "system",
		Status:    status,
		Service:   c.service,
		RemoteIP:  c.remote,
		QueueID:   connKey(c.id),
		Detail:    detail,
	})

	return c.Conn.Close()
}

func formatConnSummary(encrypted bool, bytesIn, bytesOut int, lastErr string) string {
	parts := []string{}
	if encrypted {
		parts = append(parts, "TLS established")
	} else {
		parts = append(parts, "plaintext")
	}
	parts = append(parts, "in "+formatBytes(bytesIn), "out "+formatBytes(bytesOut))
	if lastErr != "" {
		parts = append(parts, "error: "+lastErr)
	}
	return "connection closed (" + strings.Join(parts, ", ") + ")"
}

func formatBytes(n int) string {
	switch {
	case n < 1024:
		return itoa(n) + " B"
	case n < 1024*1024:
		return itoa(n/1024) + " KB"
	default:
		return itoa(n/(1024*1024)) + " MB"
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

// splitProtocolLines turns a chunk of stream into printable lines.
func splitProtocolLines(data []byte) []string {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	raw := strings.Split(text, "\n")

	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, sanitizeLine(line))
	}
	return lines
}

// sanitizeLine keeps traces readable and never records a password in the clear.
func sanitizeLine(line string) string {
	trimmed := strings.TrimSpace(line)
	upper := strings.ToUpper(trimmed)

	switch {
	case strings.HasPrefix(upper, "AUTH "):
		fields := strings.Fields(trimmed)
		if len(fields) >= 2 {
			return "AUTH " + fields[1] + " [credentials hidden]"
		}
		return "AUTH [credentials hidden]"
	case strings.HasPrefix(upper, "PASS "):
		return "PASS [hidden]"
	case strings.HasPrefix(upper, "LOGIN "):
		return "LOGIN [credentials hidden]"
	}

	// Replace anything unprintable so a binary probe cannot corrupt the view.
	var b strings.Builder
	for _, r := range trimmed {
		if r < 32 || r == 127 {
			b.WriteRune('.')
			continue
		}
		b.WriteRune(r)
	}
	if b.Len() > 500 {
		return b.String()[:500] + "…"
	}
	return b.String()
}

// isNormalClose filters the errors that just mean "the peer went away".
func isNormalClose(err error) bool {
	if err == nil {
		return true
	}
	message := err.Error()
	return strings.Contains(message, "EOF") ||
		strings.Contains(message, "use of closed network connection") ||
		strings.Contains(message, "connection reset by peer")
}

func splitHostPort(addr net.Addr) (string, string) {
	if addr == nil {
		return "", ""
	}
	host, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String(), ""
	}
	return host, port
}

func connKey(id uint64) string { return "conn-" + itoa(int(id)) }

// traceIDOf digs our connection id out of whatever wrapping the server applied,
// so session-level events can be tied to the connection that produced them.
func traceIDOf(conn net.Conn) uint64 {
	for i := 0; i < 4 && conn != nil; i++ {
		switch c := conn.(type) {
		case *tracedConn:
			return c.id
		case *tls.Conn:
			conn = c.NetConn()
		default:
			return 0
		}
	}
	return 0
}

// traces returns the manager's connection trace store.
func (m *EmailManager) traces() *traceStore {
	n := m.Native()
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.traceStore == nil {
		n.traceStore = newTraceStore()
	}
	return n.traceStore
}

// ConnectionTraces exposes recent connections to the API.
func (m *EmailManager) ConnectionTraces(limit int) []ConnectionTrace {
	return m.traces().List(limit)
}

// TraceSessionEvent records a protocol step that happened after encryption, so
// the trace of a TLS connection is not empty.
func (m *EmailManager) TraceSessionEvent(conn net.Conn, text string) {
	if id := traceIDOf(conn); id != 0 {
		m.traces().note(id, text)
	}
}
