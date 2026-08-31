package email_server

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"mime"
	"net"
	"strings"
	"sync"
	"time"

	"redock/platform/memory"

	"github.com/emersion/go-imap/server"
	"github.com/emersion/go-smtp"
)

// NativeServer owns every listener of the built-in mail server.
type NativeServer struct {
	manager *EmailManager

	mu         sync.Mutex
	running    bool
	startedAt  time.Time
	listeners  []*mailListener
	certs      *certManager
	guard      *connectionGuard
	stop       chan struct{}
	store      *MaildirStore
	queue      *OutboundQueue
	traceStore *traceStore
}

// mailListener is one bound port plus what is needed to shut it down.
type mailListener struct {
	Name     string `json:"name"` // smtp, submission, smtps, imap, imaps, pop3, pop3s
	Port     int    `json:"port"`
	TLS      string `json:"tls"` // none, starttls, implicit
	listener net.Listener
	closeFn  func() error
}

// ListenerStatus is the dashboard-facing view of a listener.
type ListenerStatus struct {
	Name string `json:"name"`
	Port int    `json:"port"`
	TLS  string `json:"tls"`
}

// NativeStatus reports what the built-in server is doing.
type NativeStatus struct {
	Running     bool             `json:"running"`
	StartedAt   *time.Time       `json:"started_at,omitempty"`
	Listeners   []ListenerStatus `json:"listeners"`
	CertSource  string           `json:"cert_source"`
	CertExpires *time.Time       `json:"cert_expires,omitempty"`
	QueueLength int              `json:"queue_length"`
	MailRoot    string           `json:"mail_root"`
}

// ---- manager-level accessors ----

// Native returns this manager's built-in server, creating it on first use.
func (m *EmailManager) Native() *NativeServer {
	m.nativeOnce.Do(func() {
		m.native = &NativeServer{manager: m}
	})
	return m.native
}

// store returns the Maildir store, rooted at the configured mail data path.
func (m *EmailManager) store() *MaildirStore {
	n := m.Native()
	n.mu.Lock()
	defer n.mu.Unlock()

	root := m.mailRoot()
	if n.store == nil || n.store.Root() != root {
		n.store = NewMaildirStore(root)
	}
	return n.store
}

// queue returns the outbound queue.
func (m *EmailManager) queue() *OutboundQueue {
	n := m.Native()
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.queue == nil {
		n.queue = newOutboundQueue(m, m.queueRoot())
	}
	return n.queue
}

func (m *EmailManager) mailRoot() string {
	if m.config != nil && m.config.DataPath != "" {
		return m.config.DataPath
	}
	return m.dataPath + "/mail"
}

func (m *EmailManager) queueRoot() string {
	return m.dataPath + "/queue"
}

// nativeConfig returns the persisted config with native defaults applied, so
// an install that predates the native engine still gets sane values.
func (m *EmailManager) nativeConfig() EmailServerConfig {
	m.mutex.RLock()
	cfg := m.config
	m.mutex.RUnlock()

	if cfg == nil {
		cfg = m.createDefaultConfig()
	}
	out := *cfg
	applyNativeDefaults(&out)
	return out
}

// applyNativeDefaults fills in zero values for the native engine.
func applyNativeDefaults(cfg *EmailServerConfig) {
	if cfg.SMTPPort == 0 {
		cfg.SMTPPort = 25
	}
	if cfg.SMTPSPort == 0 {
		cfg.SMTPSPort = 465
	}
	if cfg.SubmissionPort == 0 {
		cfg.SubmissionPort = 587
	}
	if cfg.IMAPPort == 0 {
		cfg.IMAPPort = 143
	}
	if cfg.IMAPsPort == 0 {
		cfg.IMAPsPort = 993
	}
	if cfg.POP3Port == 0 {
		cfg.POP3Port = 110
	}
	if cfg.POP3sPort == 0 {
		cfg.POP3sPort = 995
	}
	if cfg.MaxMessageSize <= 0 {
		cfg.MaxMessageSize = 50
	}
	if cfg.MaxRecipients <= 0 {
		cfg.MaxRecipients = 50
	}
	if cfg.MaxAuthFailures <= 0 {
		cfg.MaxAuthFailures = 10
	}
	if cfg.MaxConnectionsPerMinute <= 0 {
		cfg.MaxConnectionsPerMinute = 60
	}
	if cfg.BlockMinutes <= 0 {
		cfg.BlockMinutes = 30
	}
	if cfg.QueueMaxAttempts <= 0 {
		cfg.QueueMaxAttempts = defaultMaxAttempts
	}
	if cfg.QueueRetryMinutes <= 0 {
		cfg.QueueRetryMinutes = defaultRetryMin
	}
	if cfg.Hostname == "" {
		cfg.Hostname = "localhost"
	}
}

// EnableAllNativeServices turns on every listener and inbound check. Used for
// new installs and when migrating an old container-based setup, which left TLS
// and the retrieval protocols disabled.
func EnableAllNativeServices(cfg *EmailServerConfig) {
	cfg.SSLEnabled = true
	cfg.STARTTLSRequired = true
	cfg.SMTPSEnabled = true
	cfg.IMAPEnabled = true
	cfg.IMAPsEnabled = true
	cfg.POP3Enabled = true
	cfg.POP3sEnabled = true
	cfg.LogConnections = true
	cfg.GuardEnabled = true
	cfg.CheckSPF = true
	cfg.CheckDKIM = true
	cfg.CheckDMARC = true
	cfg.DKIMEnabled = true
	// Rejecting on a DMARC failure is correct but unforgiving on a fresh
	// install; failures land in Junk until the operator turns this on.
	cfg.RejectOnDMARCFail = false
	applyNativeDefaults(cfg)
}

// ---- lifecycle ----

// Start binds every enabled listener and starts the outbound queue.
func (n *NativeServer) Start() error {
	n.mu.Lock()
	if n.running {
		n.mu.Unlock()
		return nil
	}
	n.mu.Unlock()

	m := n.manager
	cfg := m.nativeConfig()

	if err := m.store().EnsureAllMailboxes(m); err != nil {
		log.Printf("mail: could not prepare mailboxes: %v", err)
	}

	certs := newCertManager(cfg.Hostname, m.dataPath, m.workDir(), cfg.SSLCertPath, cfg.SSLKeyPath,
		m.certificateNames(cfg), m.certificateIPs(cfg))
	tlsConfig := m.tlsConfigForService(certs.TLSConfig(), "tls")

	var listeners []*mailListener
	fail := func(err error) error {
		for _, l := range listeners {
			_ = l.closeFn()
		}
		return err
	}

	// --- SMTP family ---
	inbound := n.newSMTPServer(cfg, false, tlsConfig, true)
	if l, err := listenPlain(m, cfg.SMTPPort, "smtp", "starttls", inbound); err != nil {
		return fail(err)
	} else if l != nil {
		listeners = append(listeners, l)
	}

	submission := n.newSMTPServer(cfg, true, tlsConfig, true)
	if l, err := listenPlain(m, cfg.SubmissionPort, "submission", "starttls", submission); err != nil {
		return fail(err)
	} else if l != nil {
		listeners = append(listeners, l)
	}

	if cfg.SMTPSEnabled {
		smtps := n.newSMTPServer(cfg, true, tlsConfig, false)
		if l, err := listenTLS(m, cfg.SMTPSPort, "smtps", smtps, tlsConfig); err != nil {
			return fail(err)
		} else if l != nil {
			listeners = append(listeners, l)
		}
	}

	// --- IMAP family ---
	if cfg.IMAPEnabled || cfg.IMAPsEnabled {
		backend := &imapBackend{manager: m}

		if cfg.IMAPEnabled {
			imapSrv := server.New(backend)
			imapSrv.TLSConfig = tlsConfig
			imapSrv.AllowInsecureAuth = !cfg.STARTTLSRequired
			if l, err := listenIMAP(m, cfg.IMAPPort, "imap", "starttls", imapSrv, false); err != nil {
				return fail(err)
			} else if l != nil {
				listeners = append(listeners, l)
			}
		}

		if cfg.IMAPsEnabled {
			imapsSrv := server.New(backend)
			imapsSrv.TLSConfig = tlsConfig
			if l, err := listenIMAP(m, cfg.IMAPsPort, "imaps", "implicit", imapsSrv, true); err != nil {
				return fail(err)
			} else if l != nil {
				listeners = append(listeners, l)
			}
		}
	}

	// --- POP3 family ---
	if cfg.POP3Enabled {
		pop3 := &pop3Server{manager: m, tlsConfig: tlsConfig, requireTLS: cfg.STARTTLSRequired}
		if l, err := listenPOP3(m, cfg.POP3Port, "pop3", "starttls", pop3, false); err != nil {
			return fail(err)
		} else if l != nil {
			listeners = append(listeners, l)
		}
	}
	if cfg.POP3sEnabled {
		pop3s := &pop3Server{manager: m, tlsConfig: tlsConfig}
		if l, err := listenPOP3(m, cfg.POP3sPort, "pop3s", "implicit", pop3s, true); err != nil {
			return fail(err)
		} else if l != nil {
			listeners = append(listeners, l)
		}
	}

	if err := m.queue().Start(); err != nil {
		return fail(err)
	}

	stop := make(chan struct{})

	n.mu.Lock()
	n.listeners = listeners
	n.certs = certs
	n.running = true
	n.startedAt = time.Now()
	n.stop = stop
	n.mu.Unlock()

	// Keep the certificate and the usage figures current while the server runs.
	go n.startCertificateRenewal(stop)
	go n.startUsageRefresh(stop)
	go n.startGuardSweep(stop)

	ports := make([]string, 0, len(listeners))
	for _, l := range listeners {
		ports = append(ports, fmt.Sprintf("%s/%d", l.Name, l.Port))
	}
	log.Printf("mail: native server listening on %s (TLS: %s)", strings.Join(ports, ", "), certs.Source())
	return nil
}

// Stop closes every listener and the queue worker.
func (n *NativeServer) Stop() {
	n.mu.Lock()
	if !n.running {
		n.mu.Unlock()
		return
	}
	listeners := n.listeners
	n.listeners = nil
	n.running = false
	if n.stop != nil {
		close(n.stop)
		n.stop = nil
	}
	n.mu.Unlock()

	for _, l := range listeners {
		if err := l.closeFn(); err != nil {
			log.Printf("mail: closing %s: %v", l.Name, err)
		}
	}
	n.manager.queue().Stop()
	log.Println("mail: native server stopped")
}

// IsRunning reports whether the native listeners are up.
func (n *NativeServer) IsRunning() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.running
}

// Status describes the running server for the dashboard.
func (n *NativeServer) Status() NativeStatus {
	n.mu.Lock()
	running := n.running
	startedAt := n.startedAt
	certs := n.certs
	listeners := make([]ListenerStatus, 0, len(n.listeners))
	for _, l := range n.listeners {
		listeners = append(listeners, ListenerStatus{Name: l.Name, Port: l.Port, TLS: l.TLS})
	}
	n.mu.Unlock()

	status := NativeStatus{
		Running:     running,
		Listeners:   listeners,
		MailRoot:    n.manager.mailRoot(),
		QueueLength: len(n.manager.queue().List()),
	}
	if running {
		started := startedAt
		status.StartedAt = &started
	}
	if certs != nil {
		status.CertSource = certs.Source()
		if expiry := certs.Expiry(); !expiry.IsZero() {
			status.CertExpires = &expiry
		}
	}
	return status
}

func (n *NativeServer) newSMTPServer(cfg EmailServerConfig, submission bool, tlsConfig *tls.Config, starttls bool) *smtp.Server {
	serviceName := "smtp"
	if submission {
		serviceName = "submission"
	}

	backend := &smtpBackend{
		manager:    n.manager,
		submission: submission,
		requireTLS: cfg.STARTTLSRequired,
	}

	srv := smtp.NewServer(backend)
	srv.Domain = cfg.Hostname
	srv.MaxRecipients = cfg.MaxRecipients
	srv.MaxMessageBytes = cfg.MaxMessageSize * 1024 * 1024
	srv.ReadTimeout = 5 * time.Minute
	srv.WriteTimeout = 5 * time.Minute
	srv.AllowInsecureAuth = !cfg.STARTTLSRequired
	srv.EnableSMTPUTF8 = true
	// Protocol failures the library handles itself (bad commands, aborted
	// connections) would otherwise only reach stdout.
	srv.ErrorLog = &mailLogger{manager: n.manager, service: serviceName}
	if starttls {
		srv.TLSConfig = tlsConfig
	}
	return srv
}

// certificateNames is every hostname the TLS certificate must vouch for: the
// mail hostname itself plus mail.<domain> for each served domain, so a client
// configured with either name verifies cleanly.
func (m *EmailManager) certificateNames(cfg EmailServerConfig) []string {
	names := []string{cfg.Hostname}

	if m.db != nil {
		for _, domain := range memory.FindAll[*EmailDomain](m.db, "email_domains") {
			if domain == nil || domain.IsDeleted() {
				continue
			}
			names = append(names, "mail."+domain.Domain, domain.Domain)
			if domain.MXRecord != "" {
				names = append(names, domain.MXRecord)
			}
		}
	}
	return names
}

// certificateIPs is every address a client might dial: the configured public
// address plus whatever this machine answers on. Without these a client
// connecting by IP fails with "doesn't contain any IP SANs".
func (m *EmailManager) certificateIPs(cfg EmailServerConfig) []net.IP {
	ips := localAddresses()
	if cfg.IPAddress != "" {
		if ip := net.ParseIP(cfg.IPAddress); ip != nil {
			ips = append(ips, ip)
		}
	}
	return ips
}

// ---- listener helpers ----

func listenPlain(m *EmailManager, port int, name, tlsMode string, srv *smtp.Server) (*mailListener, error) {
	if port <= 0 {
		return nil, nil
	}
	raw, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, fmt.Errorf("mail: cannot listen on %s port %d: %w", name, port, err)
	}
	listener := &tracedListener{Listener: raw, manager: m, service: name}

	go func() {
		if err := srv.Serve(listener); err != nil && !errors.Is(err, net.ErrClosed) {
			log.Printf("mail: %s listener stopped: %v", name, err)
		}
	}()

	return &mailListener{Name: name, Port: port, TLS: tlsMode, listener: listener, closeFn: srv.Close}, nil
}

func listenTLS(m *EmailManager, port int, name string, srv *smtp.Server, tlsConfig *tls.Config) (*mailListener, error) {
	if port <= 0 {
		return nil, nil
	}
	raw, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, fmt.Errorf("mail: cannot listen on %s port %d: %w", name, port, err)
	}
	// Trace before the TLS layer so the connection itself is recorded even when
	// the handshake never completes.
	listener := tls.NewListener(&tracedListener{Listener: raw, manager: m, service: name, implicitTLS: true}, tlsConfig)

	go func() {
		if err := srv.Serve(listener); err != nil && !errors.Is(err, net.ErrClosed) {
			log.Printf("mail: %s listener stopped: %v", name, err)
		}
	}()

	return &mailListener{Name: name, Port: port, TLS: "implicit", listener: listener, closeFn: srv.Close}, nil
}

func listenIMAP(m *EmailManager, port int, name, tlsMode string, srv *server.Server, implicitTLS bool) (*mailListener, error) {
	if port <= 0 {
		return nil, nil
	}

	raw, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, fmt.Errorf("mail: cannot listen on %s port %d: %w", name, port, err)
	}

	var listener net.Listener = &tracedListener{Listener: raw, manager: m, service: name, implicitTLS: implicitTLS}
	if implicitTLS {
		listener = tls.NewListener(listener, srv.TLSConfig)
	}

	go func() {
		if err := srv.Serve(listener); err != nil && !errors.Is(err, net.ErrClosed) {
			log.Printf("mail: %s listener stopped: %v", name, err)
		}
	}()

	return &mailListener{Name: name, Port: port, TLS: tlsMode, listener: listener, closeFn: srv.Close}, nil
}

func listenPOP3(m *EmailManager, port int, name, tlsMode string, srv *pop3Server, implicitTLS bool) (*mailListener, error) {
	if port <= 0 {
		return nil, nil
	}

	raw, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, fmt.Errorf("mail: cannot listen on %s port %d: %w", name, port, err)
	}

	var listener net.Listener = &tracedListener{Listener: raw, manager: m, service: name, implicitTLS: implicitTLS}
	if implicitTLS {
		listener = tls.NewListener(listener, srv.tlsConfig)
	}

	go srv.Serve(listener)

	return &mailListener{Name: name, Port: port, TLS: tlsMode, listener: listener, closeFn: listener.Close}, nil
}

// ---- delivery + logging ----

// EnsureAllMailboxes creates the Maildir of every configured mailbox, so a
// freshly switched-over install has somewhere to put incoming mail.
func (s *MaildirStore) EnsureAllMailboxes(m *EmailManager) error {
	if m.db == nil {
		return nil
	}

	mailboxes := memory.FindAll[*EmailMailbox](m.db, "email_mailboxes")
	for _, mb := range mailboxes {
		if mb == nil || mb.IsDeleted() {
			continue
		}
		domain, err := memory.FindByID[*EmailDomain](m.db, "email_domains", mb.DomainID)
		if err != nil || domain == nil {
			continue
		}
		if err := s.EnsureMailbox(domain.Domain, mb.Username); err != nil {
			return err
		}
	}
	return nil
}

// deliverLocal writes a message into a local mailbox's folder.
func (m *EmailManager) deliverLocal(account *Account, folder string, raw []byte, flags []string) error {
	if account == nil || account.Mailbox == nil {
		return fmt.Errorf("no such account")
	}
	if err := m.store().EnsureMailbox(account.Domain.Domain, account.Mailbox.Username); err != nil {
		return err
	}
	_, err := m.store().Deliver(account.Base, folder, raw, flags, time.Now())
	return err
}

// mailEvent is one structured entry for the mail log. Because the native
// server writes these directly, the dashboard no longer has to parse a
// container's syslog output to know what happened.
type mailEvent struct {
	Direction  string // in, out, system
	Status     string // delivered, sent, queued, deferred, bounced, rejected, error, login, auth-failed
	From       string
	To         string
	Subject    string
	Size       int64
	RemoteIP   string
	Service    string
	SMTPCode   int
	RemoteHost string
	QueueID    string
	MailboxID  uint
	Detail     string
}

// logMailEvent records an event in the email_logs table (capped by the memory
// guard) and mirrors it to the process log.
func (m *EmailManager) logMailEvent(event mailEvent) {
	if m.db != nil {
		entry := &EmailLog{
			MailboxID:     event.MailboxID,
			Type:          event.Direction,
			From:          event.From,
			To:            event.To,
			Subject:       event.Subject,
			Status:        event.Status,
			StatusMessage: event.Detail,
			Size:          event.Size,
			Timestamp:     time.Now(),
			SMTPCode:      event.SMTPCode,
			Service:       event.Service,
			RemoteIP:      event.RemoteIP,
			RemoteHost:    event.RemoteHost,
			QueueID:       event.QueueID,
		}
		if err := memory.Create(m.db, "email_logs", entry); err != nil {
			log.Printf("mail: could not record log entry: %v", err)
		}
	}

	log.Printf("mail[%s/%s]: %s → %s %s", event.Direction, event.Status, orDefault(event.From, "-"), orDefault(event.To, "-"), event.Detail)
}

// decodeMIMEHeader decodes RFC 2047 encoded words so subjects are readable in
// the log and the dashboard.
func decodeMIMEHeader(value string) string {
	decoder := new(mime.WordDecoder)
	decoded, err := decoder.DecodeHeader(value)
	if err != nil {
		return value
	}
	return decoded
}

// errorsAs is a tiny wrapper so callers do not each import "errors".
func errorsAs(err error, target any) bool { return errors.As(err, target) }

// workDir returns the Redock working directory (the parent of the email data
// directory's parent), used to find the API Gateway's TLS certificate.
func (m *EmailManager) workDir() string {
	// dataPath is <workDir>/data/email, so two levels up is the work dir.
	return strings.TrimSuffix(strings.TrimSuffix(m.dataPath, "/email"), "/data")
}
