package email_server

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"
)

// pop3Server implements RFC 1939 over the same Maildir store the IMAP server
// uses. POP3 was never enabled in the container setup; it costs little here and
// some clients (and scripts) still want it.
type pop3Server struct {
	manager    *EmailManager
	tlsConfig  *tls.Config
	requireTLS bool
}

const pop3Timeout = 10 * time.Minute

// Serve accepts connections until the listener is closed.
func (s *pop3Server) Serve(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			log.Printf("mail: pop3 accept failed: %v", err)
			return
		}
		go s.handle(conn)
	}
}

// pop3Session is one client connection.
type pop3Session struct {
	server *pop3Server
	conn   net.Conn
	reader *bufio.Reader
	writer *bufio.Writer

	tls      bool
	username string
	account  *Account

	messages []MaildirMessage
	deleted  map[int]bool
}

func (s *pop3Server) handle(conn net.Conn) {
	defer conn.Close()

	_, isTLS := conn.(*tls.Conn)
	session := &pop3Session{
		server:  s,
		conn:    conn,
		reader:  bufio.NewReader(conn),
		writer:  bufio.NewWriter(conn),
		tls:     isTLS,
		deleted: make(map[int]bool),
	}

	defer func() {
		if r := recover(); r != nil {
			log.Printf("mail: pop3 session panicked: %v", r)
		}
	}()

	session.reply("+OK Redock POP3 ready")
	session.loop()
}

func (p *pop3Session) reply(format string, args ...any) {
	_ = p.conn.SetWriteDeadline(time.Now().Add(time.Minute))
	fmt.Fprintf(p.writer, format+"\r\n", args...)
	_ = p.writer.Flush()
}

func (p *pop3Session) loop() {
	for {
		_ = p.conn.SetReadDeadline(time.Now().Add(pop3Timeout))
		line, err := p.reader.ReadString('\n')
		if err != nil {
			return
		}

		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 0 {
			continue
		}
		command := strings.ToUpper(fields[0])
		args := fields[1:]

		switch command {
		case "CAPA":
			p.handleCapa()
		case "STLS":
			if !p.handleSTLS() {
				return
			}
		case "USER":
			p.handleUser(args)
		case "PASS":
			p.handlePass(args)
		case "STAT":
			p.handleStat()
		case "LIST":
			p.handleList(args)
		case "UIDL":
			p.handleUidl(args)
		case "RETR":
			p.handleRetr(args)
		case "TOP":
			p.handleTop(args)
		case "DELE":
			p.handleDele(args)
		case "RSET":
			p.deleted = make(map[int]bool)
			p.reply("+OK")
		case "NOOP":
			p.reply("+OK")
		case "QUIT":
			p.handleQuit()
			return
		default:
			p.reply("-ERR Unknown command")
		}
	}
}

func (p *pop3Session) handleCapa() {
	p.reply("+OK Capability list follows")
	p.reply("USER")
	p.reply("UIDL")
	p.reply("TOP")
	if !p.tls && p.server.tlsConfig != nil {
		p.reply("STLS")
	}
	p.reply(".")
}

// handleSTLS upgrades the connection in place. Returns false when the session
// must end.
func (p *pop3Session) handleSTLS() bool {
	if p.tls || p.server.tlsConfig == nil {
		p.reply("-ERR TLS not available")
		return true
	}

	p.reply("+OK Begin TLS negotiation")
	tlsConn := tls.Server(p.conn, p.server.tlsConfig)
	if err := tlsConn.Handshake(); err != nil {
		return false
	}

	p.conn = tlsConn
	p.reader = bufio.NewReader(tlsConn)
	p.writer = bufio.NewWriter(tlsConn)
	p.tls = true
	// RFC 2595: state resets after the upgrade.
	p.username = ""
	p.account = nil
	return true
}

func (p *pop3Session) handleUser(args []string) {
	if len(args) == 0 {
		p.reply("-ERR Username required")
		return
	}
	p.username = args[0]
	p.reply("+OK")
}

func (p *pop3Session) handlePass(args []string) {
	if p.server.requireTLS && !p.tls {
		p.reply("-ERR Encryption required, use STLS first")
		return
	}
	if p.username == "" || len(args) == 0 {
		p.reply("-ERR USER required first")
		return
	}

	account, err := p.server.manager.Authenticate(p.username, strings.Join(args, " "))
	if err != nil {
		remoteIP := hostOf(p.conn.RemoteAddr())
		p.server.manager.logMailEvent(mailEvent{
			Direction: "system",
			Status:    "auth-failed",
			From:      p.username,
			RemoteIP:  remoteIP,
			Service:   "pop3",
			Detail:    err.Error(),
		})
		p.server.manager.noteAuthFailure("pop3", remoteIP, p.username)
		p.reply("-ERR Authentication failed")
		return
	}
	if account.Mailbox != nil && !account.Mailbox.POP3Enabled && account.Mailbox.IMAPEnabled {
		p.reply("-ERR POP3 disabled for this mailbox")
		return
	}

	p.account = account
	if err := p.loadMessages(); err != nil {
		p.reply("-ERR Mailbox unavailable")
		return
	}

	p.server.manager.recordLogin(account, "pop3")
	p.reply("+OK %d messages", len(p.messages))
}

func (p *pop3Session) loadMessages() error {
	messages, err := p.server.manager.store().List(p.account.Base, inboxName)
	if err != nil {
		return err
	}
	p.messages = messages
	return nil
}

// requireAuth guards the transaction-state commands.
func (p *pop3Session) requireAuth() bool {
	if p.account == nil {
		p.reply("-ERR Authenticate first")
		return false
	}
	return true
}

// message resolves a 1-based message number, honouring deletions.
func (p *pop3Session) message(arg string) (int, MaildirMessage, bool) {
	index, err := strconv.Atoi(arg)
	if err != nil || index < 1 || index > len(p.messages) {
		p.reply("-ERR No such message")
		return 0, MaildirMessage{}, false
	}
	if p.deleted[index] {
		p.reply("-ERR Message deleted")
		return 0, MaildirMessage{}, false
	}
	return index, p.messages[index-1], true
}

func (p *pop3Session) handleStat() {
	if !p.requireAuth() {
		return
	}

	count := 0
	var size int64
	for i, msg := range p.messages {
		if p.deleted[i+1] {
			continue
		}
		count++
		size += msg.Size
	}
	p.reply("+OK %d %d", count, size)
}

func (p *pop3Session) handleList(args []string) {
	if !p.requireAuth() {
		return
	}

	if len(args) > 0 {
		index, msg, ok := p.message(args[0])
		if !ok {
			return
		}
		p.reply("+OK %d %d", index, msg.Size)
		return
	}

	p.reply("+OK Message list follows")
	for i, msg := range p.messages {
		if p.deleted[i+1] {
			continue
		}
		p.reply("%d %d", i+1, msg.Size)
	}
	p.reply(".")
}

func (p *pop3Session) handleUidl(args []string) {
	if !p.requireAuth() {
		return
	}

	if len(args) > 0 {
		index, msg, ok := p.message(args[0])
		if !ok {
			return
		}
		p.reply("+OK %d %s", index, msg.Key)
		return
	}

	p.reply("+OK Unique-id list follows")
	for i, msg := range p.messages {
		if p.deleted[i+1] {
			continue
		}
		p.reply("%d %s", i+1, msg.Key)
	}
	p.reply(".")
}

func (p *pop3Session) handleRetr(args []string) {
	if !p.requireAuth() || len(args) == 0 {
		if len(args) == 0 {
			p.reply("-ERR Message number required")
		}
		return
	}

	_, msg, ok := p.message(args[0])
	if !ok {
		return
	}

	raw, err := p.server.manager.store().Read(p.account.Base, inboxName, msg)
	if err != nil {
		p.reply("-ERR Message unavailable")
		return
	}

	p.reply("+OK %d octets", len(raw))
	p.writeDotStuffed(string(raw), -1)

	// Retrieval marks the message seen, matching what a POP3 user expects when
	// they also look at the mailbox over IMAP.
	if !hasFlag(msg.Flags, imapFlagSeen) {
		_, _ = p.server.manager.store().SetFlags(p.account.Base, inboxName, msg, append(msg.Flags, imapFlagSeen))
	}
}

func (p *pop3Session) handleTop(args []string) {
	if !p.requireAuth() {
		return
	}
	if len(args) < 2 {
		p.reply("-ERR TOP requires a message number and a line count")
		return
	}

	_, msg, ok := p.message(args[0])
	if !ok {
		return
	}
	lines, err := strconv.Atoi(args[1])
	if err != nil || lines < 0 {
		p.reply("-ERR Invalid line count")
		return
	}

	raw, err := p.server.manager.store().Read(p.account.Base, inboxName, msg)
	if err != nil {
		p.reply("-ERR Message unavailable")
		return
	}

	p.reply("+OK Top of message follows")
	p.writeDotStuffed(string(raw), lines)
}

// writeDotStuffed sends a multi-line response, escaping leading dots per the
// protocol. bodyLines < 0 sends the whole message; otherwise the header plus
// that many body lines.
func (p *pop3Session) writeDotStuffed(raw string, bodyLines int) {
	normalized := strings.ReplaceAll(strings.ReplaceAll(raw, "\r\n", "\n"), "\r", "\n")
	lines := strings.Split(normalized, "\n")

	inBody := false
	sent := 0
	for _, line := range lines {
		if !inBody && strings.TrimSpace(line) == "" {
			inBody = true
			p.writeLine(line)
			continue
		}
		if inBody && bodyLines >= 0 {
			if sent >= bodyLines {
				break
			}
			sent++
		}
		p.writeLine(line)
	}
	p.reply(".")
}

func (p *pop3Session) writeLine(line string) {
	if strings.HasPrefix(line, ".") {
		line = "." + line
	}
	fmt.Fprintf(p.writer, "%s\r\n", line)
}

func (p *pop3Session) handleDele(args []string) {
	if !p.requireAuth() || len(args) == 0 {
		if len(args) == 0 {
			p.reply("-ERR Message number required")
		}
		return
	}

	index, _, ok := p.message(args[0])
	if !ok {
		return
	}
	p.deleted[index] = true
	p.reply("+OK Message %d deleted", index)
}

// handleQuit applies pending deletions, as POP3 requires them to happen only
// on a clean QUIT.
func (p *pop3Session) handleQuit() {
	if p.account == nil {
		p.reply("+OK Bye")
		return
	}

	removed := 0
	for index := range p.deleted {
		if index < 1 || index > len(p.messages) {
			continue
		}
		if err := p.server.manager.store().Remove(p.account.Base, inboxName, p.messages[index-1]); err == nil {
			removed++
		}
	}
	p.reply("+OK %d messages deleted, bye", removed)
	_ = p.writer.Flush()
}
