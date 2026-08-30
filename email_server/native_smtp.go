package email_server

import (
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
)

// smtpBackend serves one SMTP role. Port 25 runs in inbound mode (no auth,
// local recipients only — an open relay is the one mistake a mail server must
// never make); 587/465 run in submission mode (auth required, any recipient,
// outbound goes through the queue).
type smtpBackend struct {
	manager    *EmailManager
	submission bool
	requireTLS bool
}

func (b *smtpBackend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return &smtpSession{backend: b, conn: c}, nil
}

// smtpSession is one client connection's state.
type smtpSession struct {
	backend *smtpBackend
	conn    *smtp.Conn

	account    *Account // set once authenticated (submission only)
	from       string
	recipients []string
}

func (s *smtpSession) Reset() {
	s.from = ""
	s.recipients = nil
}

func (s *smtpSession) Logout() error { return nil }

// AuthMechanisms advertises the SASL mechanisms we accept.
func (s *smtpSession) AuthMechanisms() []string {
	if !s.backend.submission {
		return nil // port 25 never authenticates
	}
	return []string{sasl.Plain}
}

// Auth validates a mailbox login against the memory DB.
func (s *smtpSession) Auth(mech string) (sasl.Server, error) {
	if !s.backend.submission {
		return nil, smtp.ErrAuthUnsupported
	}
	if s.backend.requireTLS && !s.isEncrypted() {
		return nil, &smtp.SMTPError{
			Code:         538,
			EnhancedCode: smtp.EnhancedCode{5, 7, 11},
			Message:      "Encryption required for authentication",
		}
	}

	return sasl.NewPlainServer(func(identity, username, password string) error {
		account, err := s.backend.manager.Authenticate(username, password)
		if err != nil {
			s.backend.manager.logMailEvent(mailEvent{
				Direction: "system",
				Status:    "auth-failed",
				From:      username,
				RemoteIP:  s.remoteIP(),
				Service:   "smtp",
				Detail:    err.Error(),
			})
			return smtp.ErrAuthFailed
		}
		if !account.Mailbox.SMTPEnabled && account.Mailbox.ID != 0 {
			// SMTPEnabled defaults to false on older rows; only refuse when the
			// mailbox explicitly has sending disabled through the dashboard.
			if !account.Mailbox.IMAPEnabled && !account.Mailbox.POP3Enabled {
				return smtp.ErrAuthFailed
			}
		}
		s.account = account
		return nil
	}), nil
}

func (s *smtpSession) isEncrypted() bool {
	_, ok := s.conn.TLSConnectionState()
	return ok
}

func (s *smtpSession) remoteIP() string {
	if s.conn == nil || s.conn.Conn() == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(s.conn.Conn().RemoteAddr().String())
	if err != nil {
		return s.conn.Conn().RemoteAddr().String()
	}
	return host
}

// Mail records the envelope sender.
func (s *smtpSession) Mail(from string, _ *smtp.MailOptions) error {
	if s.backend.submission {
		if s.account == nil {
			return smtp.ErrAuthRequired
		}
		// A sender may only use their own address (or an alias resolving to it).
		envelope := normalizeAddress(from)
		if envelope != "" && !s.mayUseSender(envelope) {
			return &smtp.SMTPError{
				Code:         550,
				EnhancedCode: smtp.EnhancedCode{5, 7, 1},
				Message:      "Sender address not owned by the authenticated account",
			}
		}
	}

	s.from = normalizeAddress(from)
	s.recipients = nil
	return nil
}

func (s *smtpSession) mayUseSender(envelope string) bool {
	if s.account == nil {
		return false
	}
	if strings.EqualFold(envelope, s.account.Address()) {
		return true
	}
	resolved := s.backend.manager.LookupAccount(envelope)
	return resolved != nil && resolved.Mailbox != nil && s.account.Mailbox != nil &&
		resolved.Mailbox.ID == s.account.Mailbox.ID
}

// Rcpt validates a recipient.
func (s *smtpSession) Rcpt(to string, _ *smtp.RcptOptions) error {
	address := normalizeAddress(to)
	if address == "" {
		return &smtp.SMTPError{Code: 501, EnhancedCode: smtp.EnhancedCode{5, 1, 3}, Message: "Bad recipient address"}
	}

	cfg := s.backend.manager.nativeConfig()
	if cfg.MaxRecipients > 0 && len(s.recipients) >= cfg.MaxRecipients {
		return &smtp.SMTPError{Code: 452, EnhancedCode: smtp.EnhancedCode{4, 5, 3}, Message: "Too many recipients"}
	}

	if !s.backend.submission {
		// Inbound: only accept mail we can actually deliver. Anything else
		// would make this an open relay.
		if s.backend.manager.LookupAccount(address) == nil {
			s.backend.manager.logMailEvent(mailEvent{
				Direction: "in",
				Status:    "rejected",
				From:      s.from,
				To:        address,
				RemoteIP:  s.remoteIP(),
				Service:   "smtp",
				SMTPCode:  550,
				Detail:    "relay access denied: no such local recipient",
			})
			return &smtp.SMTPError{
				Code:         550,
				EnhancedCode: smtp.EnhancedCode{5, 7, 1},
				Message:      "Relay access denied",
			}
		}
	} else if s.account == nil {
		return smtp.ErrAuthRequired
	}

	s.recipients = append(s.recipients, address)
	return nil
}

// Data accepts the message body and either delivers it locally or queues it.
func (s *smtpSession) Data(r io.Reader) error {
	cfg := s.backend.manager.nativeConfig()

	limit := cfg.MaxMessageSize * 1024 * 1024
	if limit <= 0 {
		limit = 50 * 1024 * 1024
	}

	raw, err := readAllLimited(r, limit)
	if err != nil {
		return &smtp.SMTPError{Code: 552, EnhancedCode: smtp.EnhancedCode{5, 3, 4}, Message: err.Error()}
	}
	if len(s.recipients) == 0 {
		return &smtp.SMTPError{Code: 554, EnhancedCode: smtp.EnhancedCode{5, 5, 1}, Message: "No valid recipients"}
	}

	subject := headerValue(raw, "Subject")

	if s.backend.submission {
		return s.handleSubmission(cfg, raw, subject)
	}
	return s.handleInbound(cfg, raw, subject)
}

// handleInbound stores a message that arrived from the outside world.
func (s *smtpSession) handleInbound(cfg EmailServerConfig, raw []byte, subject string) error {
	remoteIP := net.ParseIP(s.remoteIP())
	results := s.backend.manager.checkInbound(remoteIP, s.conn.Hostname(), s.from, raw)

	if results.Reject {
		for _, rcpt := range s.recipients {
			s.backend.manager.logMailEvent(mailEvent{
				Direction: "in",
				Status:    "rejected",
				From:      s.from,
				To:        rcpt,
				Subject:   subject,
				RemoteIP:  s.remoteIP(),
				Service:   "smtp",
				SMTPCode:  550,
				Detail:    results.Reason,
			})
		}
		return &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 7, 1}, Message: results.Reason}
	}

	stamped := s.stampHeaders(cfg, raw, results)

	// SPF/DMARC failures are not rejected by default, but they do land in Junk
	// so the user still sees them without them looking legitimate.
	folder := inboxName
	suspicious := results.SPF == "fail" || results.DMARC == "fail"
	if suspicious {
		folder = "Junk"
	}

	var firstErr error
	for _, rcpt := range s.recipients {
		account := s.backend.manager.LookupAccount(rcpt)
		if account == nil {
			continue
		}
		if err := s.backend.manager.deliverLocal(account, folder, stamped, nil); err != nil {
			firstErr = err
			s.backend.manager.logMailEvent(mailEvent{
				Direction: "in",
				Status:    "error",
				From:      s.from,
				To:        rcpt,
				Subject:   subject,
				RemoteIP:  s.remoteIP(),
				Service:   "smtp",
				Detail:    err.Error(),
			})
			continue
		}

		s.backend.manager.logMailEvent(mailEvent{
			Direction: "in",
			Status:    "delivered",
			From:      s.from,
			To:        rcpt,
			Subject:   subject,
			Size:      int64(len(stamped)),
			RemoteIP:  s.remoteIP(),
			Service:   "smtp",
			MailboxID: account.Mailbox.ID,
			Detail:    fmt.Sprintf("stored in %s (spf=%s dkim=%s dmarc=%s)", folder, results.SPF, results.DKIM, results.DMARC),
		})
	}

	if firstErr != nil {
		return &smtp.SMTPError{Code: 451, EnhancedCode: smtp.EnhancedCode{4, 3, 0}, Message: "Temporary storage failure"}
	}
	return nil
}

// handleSubmission delivers locally-addressed copies and queues the rest.
func (s *smtpSession) handleSubmission(cfg EmailServerConfig, raw []byte, subject string) error {
	stamped := s.stampHeaders(cfg, raw, AuthResults{})

	var remote []string
	for _, rcpt := range s.recipients {
		if account := s.backend.manager.LookupAccount(rcpt); account != nil {
			if err := s.backend.manager.deliverLocal(account, inboxName, stamped, nil); err != nil {
				return &smtp.SMTPError{Code: 451, EnhancedCode: smtp.EnhancedCode{4, 3, 0}, Message: "Temporary storage failure"}
			}
			s.backend.manager.logMailEvent(mailEvent{
				Direction: "out",
				Status:    "delivered",
				From:      s.from,
				To:        rcpt,
				Subject:   subject,
				Size:      int64(len(stamped)),
				Service:   "submission",
				MailboxID: account.Mailbox.ID,
				Detail:    "delivered locally",
			})
			continue
		}
		remote = append(remote, rcpt)
	}

	if len(remote) == 0 {
		return nil
	}

	_, senderDomain := splitAddress(s.from)
	item := &QueueItem{
		From:       s.from,
		Recipients: remote,
		Domain:     senderDomain,
		Subject:    subject,
	}
	if s.account != nil && s.account.Mailbox != nil {
		item.MailboxID = s.account.Mailbox.ID
	}

	if err := s.backend.manager.queue().Enqueue(item, stamped); err != nil {
		log.Printf("mail: could not queue message: %v", err)
		return &smtp.SMTPError{Code: 451, EnhancedCode: smtp.EnhancedCode{4, 3, 0}, Message: "Could not queue message"}
	}

	for _, rcpt := range remote {
		s.backend.manager.logMailEvent(mailEvent{
			Direction: "out",
			Status:    "queued",
			From:      s.from,
			To:        rcpt,
			Subject:   subject,
			Size:      int64(len(stamped)),
			Service:   "submission",
			QueueID:   item.ID,
			MailboxID: item.MailboxID,
			Detail:    "accepted for delivery",
		})
	}
	return nil
}

// stampHeaders prepends the Received (and, for inbound, Authentication-Results)
// trace headers every MTA is expected to add.
func (s *smtpSession) stampHeaders(cfg EmailServerConfig, raw []byte, results AuthResults) []byte {
	var header strings.Builder

	if results.SPF != "" || results.DKIM != "" || results.DMARC != "" {
		header.WriteString(authResultsHeader(cfg.Hostname, results))
	}

	proto := "SMTP"
	if s.isEncrypted() {
		proto = "ESMTPS"
	}
	remote := s.remoteIP()
	if remote == "" {
		remote = "unknown"
	}

	header.WriteString(fmt.Sprintf("Received: from %s (%s)\r\n\tby %s with %s;\r\n\t%s\r\n",
		orDefault(s.conn.Hostname(), "unknown"), remote, cfg.Hostname, proto, time.Now().Format(time.RFC1123Z)))

	return append([]byte(header.String()), raw...)
}

func orDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// headerValue pulls one header out of a raw message (first occurrence,
// unfolded). Good enough for logging and queue metadata.
func headerValue(raw []byte, name string) string {
	limit := len(raw)
	if idx := strings.Index(string(raw), "\r\n\r\n"); idx >= 0 {
		limit = idx
	}

	lines := strings.Split(string(raw[:limit]), "\n")
	prefix := strings.ToLower(name) + ":"
	for i, line := range lines {
		if !strings.HasPrefix(strings.ToLower(line), prefix) {
			continue
		}
		value := strings.TrimSpace(line[len(prefix):])
		// Unfold continuation lines.
		for j := i + 1; j < len(lines); j++ {
			if !strings.HasPrefix(lines[j], " ") && !strings.HasPrefix(lines[j], "\t") {
				break
			}
			value += " " + strings.TrimSpace(lines[j])
		}
		return decodeMIMEHeader(strings.TrimSpace(value))
	}
	return ""
}
