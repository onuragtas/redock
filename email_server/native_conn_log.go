package email_server

import (
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
)

// Connection-level logging. Without this, anything that fails before a message
// is transferred — a refused TLS handshake, a client that connects and gives
// up, a protocol error — leaves no trace at all: the SMTP library answers the
// client and returns, and the dashboard shows nothing. These hooks record the
// whole conversation, not just the messages that made it through.

// mailLogger adapts the SMTP/IMAP servers' error logger onto the mail log, so
// library-level failures are visible in the dashboard instead of only on
// stdout.
type mailLogger struct {
	manager *EmailManager
	service string
}

func (l *mailLogger) Printf(format string, v ...interface{}) {
	l.record(strings.TrimSpace(fmt.Sprintf(format, v...)))
}

func (l *mailLogger) Println(v ...interface{}) {
	l.record(strings.TrimSpace(fmt.Sprintln(v...)))
}

func (l *mailLogger) record(message string) {
	if message == "" {
		return
	}
	l.manager.logMailEvent(mailEvent{
		Direction: "system",
		Status:    "error",
		Service:   l.service,
		RemoteIP:  remoteIPFromMessage(message),
		Detail:    message,
	})
}

// remoteIPFromMessage digs the peer address out of the library's error text
// ("error handling 203.0.113.5:54321: ...") so the entry can be filtered by IP
// like every other event.
func remoteIPFromMessage(message string) string {
	const marker = "error handling "
	idx := strings.Index(message, marker)
	if idx < 0 {
		return ""
	}
	rest := message[idx+len(marker):]
	if end := strings.Index(rest, ":"); end > 0 {
		if host, _, err := net.SplitHostPort(strings.SplitN(rest, " ", 2)[0]); err == nil {
			return host
		}
		return rest[:end]
	}
	return ""
}

// connectionCounter numbers connections so every event of one conversation can
// be tied together in the dashboard.
var connectionCounter atomic.Uint64

// logTLSHandshake records that a client began a TLS handshake. It is wired
// through tls.Config.GetConfigForClient, which fires at the ClientHello —
// before verification can fail — so a client that rejects our certificate
// still leaves a trace of the attempt.
func (m *EmailManager) logTLSHandshake(service string, hello *tls.ClientHelloInfo) {
	if !m.nativeConfig().LogConnections || hello == nil {
		return
	}

	remote := ""
	if hello.Conn != nil {
		remote = hostOf(hello.Conn.RemoteAddr())
	}

	detail := "TLS handshake started"
	if hello.ServerName != "" {
		detail += " for " + hello.ServerName
	}

	m.logMailEvent(mailEvent{
		Direction: "system",
		Status:    "tls-handshake",
		Service:   service,
		RemoteIP:  remote,
		Detail:    detail,
	})
}

// tlsConfigForService wraps the shared TLS config so each listener reports its
// own handshakes.
func (m *EmailManager) tlsConfigForService(base *tls.Config, service string) *tls.Config {
	cfg := base.Clone()
	cfg.GetConfigForClient = func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
		m.logTLSHandshake(service, hello)
		return nil, nil // keep the base configuration
	}
	return cfg
}

func hostOf(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return host
}
