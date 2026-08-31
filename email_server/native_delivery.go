package email_server

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/textproto"
	"strings"
	"time"
)

// Delivery talks SMTP directly rather than through a client library, because
// the question that matters when mail does not arrive is "what exactly did the
// receiving server say" — a 421 is a temporary throttle that clears itself, a
// 550 is a reputation or authentication problem that never will. Both stdlib
// and go-smtp clients discard the text of a successful response, and that text
// carries the remote queue id support teams ask for.

// SMTPReply is one response from the far side.
type SMTPReply struct {
	Code int    `json:"code"`
	Text string `json:"text"`
}

func (r SMTPReply) String() string {
	if r.Code == 0 {
		return r.Text
	}
	return fmt.Sprintf("%d %s", r.Code, r.Text)
}

// Temporary reports whether the far side asked us to come back later.
func (r SMTPReply) Temporary() bool { return r.Code >= 400 && r.Code < 500 }

// Permanent reports a final refusal.
func (r SMTPReply) Permanent() bool { return r.Code >= 500 && r.Code < 600 }

// DeliveryError carries the remote's own words, so the log can show them.
type DeliveryError struct {
	Host  string
	Stage string // greeting, ehlo, starttls, mail, rcpt, data, body
	Reply SMTPReply
	Err   error
}

func (e *DeliveryError) Error() string {
	if e.Reply.Code != 0 {
		return fmt.Sprintf("%s at %s: %s", e.Stage, e.Host, e.Reply)
	}
	if e.Err != nil {
		return fmt.Sprintf("%s at %s: %v", e.Stage, e.Host, e.Err)
	}
	return e.Stage + " failed at " + e.Host
}

func (e *DeliveryError) Unwrap() error { return e.Err }

// DeliveryResult is what one successful transaction reported.
type DeliveryResult struct {
	Host       string    `json:"host"`
	TLS        bool      `json:"tls"`
	Accepted   SMTPReply `json:"accepted"`
	Recipients []string  `json:"recipients"`
}

const (
	smtpCommandTimeout = 2 * time.Minute
	smtpDataTimeout    = 10 * time.Minute
)

// deliverSMTP dials a mail exchanger and runs one transaction against it.
func deliverSMTP(host, helo, from string, recipients []string, raw []byte) (*DeliveryResult, error) {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, "25"), queueDialTimeout)
	if err != nil {
		return nil, &DeliveryError{Host: host, Stage: "connect", Err: err}
	}
	defer conn.Close()

	return deliverOverConn(conn, host, helo, from, recipients, raw)
}

// deliverOverConn runs the transaction on an already-open connection, which is
// what makes the conversation testable against a scripted server.
func deliverOverConn(conn net.Conn, host, helo, from string, recipients []string, raw []byte) (*DeliveryResult, error) {
	session := &smtpDelivery{host: host, conn: conn, text: textproto.NewConn(conn)}
	defer session.text.Close()

	// Greeting.
	if _, err := session.expect("greeting", 220); err != nil {
		return nil, err
	}

	extensions, err := session.ehlo(helo)
	if err != nil {
		return nil, err
	}

	// Opportunistic TLS. A certificate we cannot verify is still better than
	// plaintext for server-to-server mail, which is why verification is relaxed
	// here and only here.
	if _, ok := extensions["STARTTLS"]; ok {
		if err := session.startTLS(helo); err != nil {
			return nil, err
		}
	}

	if _, err := session.cmd("mail", 250, "MAIL FROM:<%s>", from); err != nil {
		return nil, err
	}

	accepted := make([]string, 0, len(recipients))
	var lastRcptErr error
	for _, rcpt := range recipients {
		if _, err := session.cmd("rcpt", 250, "RCPT TO:<%s>", rcpt); err != nil {
			lastRcptErr = err
			continue
		}
		accepted = append(accepted, rcpt)
	}
	if len(accepted) == 0 {
		if lastRcptErr != nil {
			return nil, lastRcptErr
		}
		return nil, &DeliveryError{Host: host, Stage: "rcpt", Err: fmt.Errorf("no recipient accepted")}
	}

	if _, err := session.cmd("data", 354, "DATA"); err != nil {
		return nil, err
	}

	_ = conn.SetDeadline(time.Now().Add(smtpDataTimeout))
	writer := session.text.DotWriter()
	if _, err := writer.Write(normalizeLineEndings(raw)); err != nil {
		writer.Close()
		return nil, &DeliveryError{Host: host, Stage: "body", Err: err}
	}
	if err := writer.Close(); err != nil {
		return nil, &DeliveryError{Host: host, Stage: "body", Err: err}
	}

	// This is the reply worth keeping: it usually contains the remote queue id.
	reply, err := session.expect("data", 250)
	if err != nil {
		return nil, err
	}

	_, _ = session.cmd("quit", 221, "QUIT")

	return &DeliveryResult{
		Host:       host,
		TLS:        session.tls,
		Accepted:   reply,
		Recipients: accepted,
	}, nil
}

// smtpDelivery is one outbound conversation.
type smtpDelivery struct {
	host string
	conn net.Conn
	text *textproto.Conn
	tls  bool
}

// expect reads a response and turns anything unexpected into a DeliveryError
// that carries the remote's exact words.
func (s *smtpDelivery) expect(stage string, want int) (SMTPReply, error) {
	_ = s.conn.SetDeadline(time.Now().Add(smtpCommandTimeout))

	code, message, err := s.text.ReadResponse(want)
	if err != nil {
		var protoErr *textproto.Error
		if errorsAs(err, &protoErr) {
			reply := SMTPReply{Code: protoErr.Code, Text: strings.TrimSpace(protoErr.Msg)}
			return reply, &DeliveryError{Host: s.host, Stage: stage, Reply: reply}
		}
		return SMTPReply{}, &DeliveryError{Host: s.host, Stage: stage, Err: err}
	}
	return SMTPReply{Code: code, Text: strings.TrimSpace(message)}, nil
}

func (s *smtpDelivery) cmd(stage string, want int, format string, args ...any) (SMTPReply, error) {
	_ = s.conn.SetDeadline(time.Now().Add(smtpCommandTimeout))

	id, err := s.text.Cmd(format, args...)
	if err != nil {
		return SMTPReply{}, &DeliveryError{Host: s.host, Stage: stage, Err: err}
	}
	s.text.StartResponse(id)
	defer s.text.EndResponse(id)

	return s.expect(stage, want)
}

// ehlo greets the far side and returns the extensions it advertises.
func (s *smtpDelivery) ehlo(helo string) (map[string]string, error) {
	reply, err := s.cmd("ehlo", 250, "EHLO %s", helo)
	if err != nil {
		// A server too old for EHLO still has to answer HELO.
		if _, heloErr := s.cmd("helo", 250, "HELO %s", helo); heloErr != nil {
			return nil, err
		}
		return map[string]string{}, nil
	}

	extensions := make(map[string]string)
	for _, line := range strings.Split(reply.Text, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 0 {
			continue
		}
		name := strings.ToUpper(fields[0])
		extensions[name] = strings.Join(fields[1:], " ")
	}
	return extensions, nil
}

func (s *smtpDelivery) startTLS(helo string) error {
	if _, err := s.cmd("starttls", 220, "STARTTLS"); err != nil {
		return err
	}

	tlsConn := tls.Client(s.conn, &tls.Config{ServerName: s.host, InsecureSkipVerify: true})
	if err := tlsConn.Handshake(); err != nil {
		return &DeliveryError{Host: s.host, Stage: "starttls", Err: err}
	}

	s.conn = tlsConn
	s.text = textproto.NewConn(tlsConn)
	s.tls = true

	// RFC 3207: the session resets, so greet again.
	if _, err := s.ehlo(helo); err != nil {
		return err
	}
	return nil
}

// normalizeLineEndings makes sure the message uses CRLF, as the protocol
// requires; DotWriter handles the dot-stuffing and the terminating line.
func normalizeLineEndings(raw []byte) []byte {
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	return []byte(strings.ReplaceAll(text, "\n", "\r\n"))
}

// replyOf digs the remote's response out of a delivery error, so callers can
// log the code and the text separately.
func replyOf(err error) SMTPReply {
	var deliveryErr *DeliveryError
	if errorsAs(err, &deliveryErr) {
		return deliveryErr.Reply
	}
	var protoErr *textproto.Error
	if errorsAs(err, &protoErr) {
		return SMTPReply{Code: protoErr.Code, Text: strings.TrimSpace(protoErr.Msg)}
	}
	return SMTPReply{}
}

// hostOfError reports which server produced a delivery failure.
func hostOfError(err error) string {
	var deliveryErr *DeliveryError
	if errorsAs(err, &deliveryErr) {
		return deliveryErr.Host
	}
	return ""
}
