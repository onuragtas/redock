// Package turn_server is Redock's standalone TURN/STUN server (pion/turn),
// built for safe exposure on the internet: short-lived HMAC credentials per
// app, no relaying into private networks, per-IP and per-app quotas, bitrate
// caps, brute-force blocking, live sessions that can be killed, and a
// metadata event log. Galène and any external WebRTC app can use it.
package turn_server

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"redock/api_gateway"
	"redock/platform/database"
	"redock/platform/memory"

	"github.com/pion/turn/v5"
)

type appInfo struct {
	secret      string
	maxSessions int
	enabled     bool
}

type Manager struct {
	mu        sync.Mutex
	server    *turn.Server
	running   bool
	startedAt time.Time
	lastErr   string
	tlsActive bool

	cfg    TURNConfigEntity   // snapshot used by the running server
	apps   map[string]appInfo // name -> info, refreshed on every app change
	appsMu sync.RWMutex

	guard    *guard
	registry *sessionRegistry
	events   *eventLog
	policy   *relayPolicy

	deniedMu sync.Mutex
	denied   map[string]time.Time // client|peer -> last logged (dedupe)
}

var manager *Manager

// configHooks run after the server (re)starts with a new configuration, so
// consumers that embed TURN URLs (Galène's ICE config) can refresh them.
var (
	hooksMu     sync.Mutex
	configHooks []func()
)

// OnConfigApplied registers fn to run whenever the TURN configuration is
// applied (start, restart, stop).
func OnConfigApplied(fn func()) {
	hooksMu.Lock()
	configHooks = append(configHooks, fn)
	hooksMu.Unlock()
}

func runConfigHooks() {
	hooksMu.Lock()
	hooks := append([]func(){}, configHooks...)
	hooksMu.Unlock()
	for _, fn := range hooks {
		fn()
	}
}

// Init creates the manager and starts the server if it was enabled.
func Init() {
	manager = &Manager{
		guard:    newGuard(),
		registry: newSessionRegistry(),
		events:   &eventLog{},
		denied:   make(map[string]time.Time),
	}
	if cfg := manager.loadConfig(); cfg != nil && cfg.Enabled {
		go func() {
			if err := manager.Start(); err != nil {
				log.Printf("turn_server: start failed: %v", err)
			}
		}()
	}
}

// GetManager returns the manager (nil before Init).
func GetManager() *Manager { return manager }

func db() *memory.Database { return database.GetMemoryDB() }

// ---------------------------------------------------------------------------
// Configuration

func defaultRelayIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}

func (m *Manager) loadConfig() *TURNConfigEntity {
	d := db()
	if d == nil {
		return nil
	}
	if rows := memory.FindAll[*TURNConfigEntity](d, TableTURNConfig); len(rows) > 0 {
		return rows[0]
	}
	cfg := &TURNConfigEntity{
		Port: DefaultPort, TLSPort: DefaultTLSPort, RelayIP: defaultRelayIP(),
		RelayMin: DefaultRelayMin, RelayMax: DefaultRelayMax, Realm: DefaultRealm,
		MaxSessionsPerIP: DefaultSessionsIP, MaxBitrateKbps: DefaultBitrateKbps,
		AuthFailLimit: DefaultAuthFailLimit, BlockMinutes: DefaultBlockMinutes,
	}
	if err := memory.Create(d, TableTURNConfig, cfg); err != nil {
		log.Printf("turn_server: could not save default config: %v", err)
	}
	return cfg
}

// Config returns the stored configuration.
func (m *Manager) Config() (TURNConfigEntity, error) {
	cfg := m.loadConfig()
	if cfg == nil {
		return TURNConfigEntity{}, fmt.Errorf("database not ready")
	}
	return *cfg, nil
}

var hostPattern = regexp.MustCompile(`^(?i)[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$`)

func validPort(p int) bool { return p > 0 && p < 65536 }

func validateConfig(c *TURNConfigEntity) error {
	if !validPort(c.Port) {
		return fmt.Errorf("port must be 1-65535")
	}
	if c.TLSPort != 0 && (!validPort(c.TLSPort) || c.TLSPort == c.Port) {
		return fmt.Errorf("TLS port must be 1-65535 and differ from the main port (0 disables TLS)")
	}
	c.PublicHost = strings.ToLower(strings.TrimSpace(c.PublicHost))
	if c.PublicHost != "" && net.ParseIP(c.PublicHost) == nil && !hostPattern.MatchString(c.PublicHost) {
		return fmt.Errorf("public host must be a domain or an IP address")
	}
	c.RelayIP = strings.TrimSpace(c.RelayIP)
	if ip := net.ParseIP(c.RelayIP); ip == nil || ip.IsUnspecified() {
		return fmt.Errorf("relay IP must be the address clients reach this server at")
	}
	if c.RelayMin == 0 || c.RelayMax == 0 || c.RelayMin > c.RelayMax {
		return fmt.Errorf("relay port range must be set with min <= max")
	}
	if c.Realm = strings.TrimSpace(c.Realm); c.Realm == "" {
		c.Realm = DefaultRealm
	}
	for _, list := range [][]string{c.AllowCIDRs, c.DenyCIDRs} {
		if _, bad := parseCIDRs(list); len(bad) > 0 {
			return fmt.Errorf("invalid IP/CIDR: %s", strings.Join(bad, ", "))
		}
	}
	if c.MaxSessionsPerIP < 0 || c.MaxBitrateKbps < 0 || c.MaxSessionMinutes < 0 || c.AuthFailLimit < 0 || c.BlockMinutes < 0 {
		return fmt.Errorf("limits cannot be negative")
	}
	if c.BlockMinutes == 0 {
		c.BlockMinutes = DefaultBlockMinutes
	}
	return nil
}

// UpdateConfig validates and saves the configuration, then restarts the
// server (or stops it when disabled). Active sessions are dropped.
func (m *Manager) UpdateConfig(in TURNConfigEntity) error {
	if err := validateConfig(&in); err != nil {
		return err
	}
	cfg := m.loadConfig()
	if cfg == nil {
		return fmt.Errorf("database not ready")
	}
	in.BaseEntity = cfg.BaseEntity
	*cfg = in
	if err := memory.Update(db(), TableTURNConfig, cfg); err != nil {
		return err
	}
	if err := m.Stop(); err != nil {
		return err
	}
	var err error
	if cfg.Enabled {
		err = m.Start()
	}
	runConfigHooks()
	return err
}

// Running reports whether the server is up.
func (m *Manager) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// ---------------------------------------------------------------------------
// Apps

var appNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

func newSecret() string {
	return strings.ReplaceAll(strings.ReplaceAll(randomToken(32), "-", "x"), "_", "y")
}

// AppView is an app as listed in the dashboard; the secret is only returned
// when created or rotated.
type AppView struct {
	ID            uint      `json:"id"`
	Name          string    `json:"name"`
	Enabled       bool      `json:"enabled"`
	MaxSessions   int       `json:"max_sessions"`
	CredentialTTL int       `json:"credential_ttl"`
	System        bool      `json:"system"`
	Note          string    `json:"note"`
	CreatedAt     time.Time `json:"created_at"`
	Active        int       `json:"active_sessions"`
	Secret        string    `json:"secret,omitempty"`
}

func (m *Manager) appRows() []*TURNAppEntity {
	d := db()
	if d == nil {
		return nil
	}
	rows := memory.FindAll[*TURNAppEntity](d, TableTURNApps)
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	return rows
}

func (m *Manager) findApp(name string) *TURNAppEntity {
	for _, a := range m.appRows() {
		if a.Name == name {
			return a
		}
	}
	return nil
}

func (m *Manager) refreshApps() {
	if db() == nil {
		return // keep what is loaded (tests run without the memory DB)
	}
	apps := make(map[string]appInfo)
	for _, a := range m.appRows() {
		apps[a.Name] = appInfo{secret: a.Secret, maxSessions: a.MaxSessions, enabled: a.Enabled}
	}
	m.appsMu.Lock()
	m.apps = apps
	m.appsMu.Unlock()
}

func (m *Manager) app(name string) (appInfo, bool) {
	m.appsMu.RLock()
	defer m.appsMu.RUnlock()
	a, ok := m.apps[name]
	return a, ok
}

func (m *Manager) toView(a *TURNAppEntity, withSecret bool) AppView {
	_, active := m.registry.counts("", a.Name)
	v := AppView{
		ID: a.ID, Name: a.Name, Enabled: a.Enabled, MaxSessions: a.MaxSessions,
		CredentialTTL: a.CredentialTTL, System: a.System, Note: a.Note, CreatedAt: a.CreatedAt, Active: active,
	}
	if withSecret {
		v.Secret = a.Secret
	}
	return v
}

func (m *Manager) ListApps() []AppView {
	rows := m.appRows()
	out := make([]AppView, 0, len(rows))
	for _, a := range rows {
		out = append(out, m.toView(a, false))
	}
	return out
}

// AppInput is what the dashboard sends to create/update an app.
type AppInput struct {
	Name          string `json:"name"`
	Enabled       bool   `json:"enabled"`
	MaxSessions   int    `json:"max_sessions"`
	CredentialTTL int    `json:"credential_ttl"`
	Note          string `json:"note"`
}

func (in *AppInput) normalize() error {
	in.Name = strings.ToLower(strings.TrimSpace(in.Name))
	if !appNamePattern.MatchString(in.Name) {
		return fmt.Errorf("app name: lowercase letters, digits, '-' or '_' (max 32)")
	}
	if in.MaxSessions < 0 {
		return fmt.Errorf("max sessions cannot be negative")
	}
	if in.CredentialTTL <= 0 {
		in.CredentialTTL = DefaultCredentialTTL
	}
	if in.CredentialTTL > 7*24*3600 {
		return fmt.Errorf("credential TTL is capped at 7 days")
	}
	return nil
}

// CreateApp registers an app and returns it with its secret (shown once).
func (m *Manager) CreateApp(in AppInput) (AppView, error) {
	if err := in.normalize(); err != nil {
		return AppView{}, err
	}
	if m.findApp(in.Name) != nil {
		return AppView{}, fmt.Errorf("app %q already exists", in.Name)
	}
	a := &TURNAppEntity{
		Name: in.Name, Secret: newSecret(), Enabled: true, MaxSessions: in.MaxSessions,
		CredentialTTL: in.CredentialTTL, Note: in.Note, CreatedAt: time.Now(),
	}
	if err := memory.Create(db(), TableTURNApps, a); err != nil {
		return AppView{}, err
	}
	m.refreshApps()
	m.events.add(Event{Kind: "app-created", App: a.Name})
	return m.toView(a, true), nil
}

// UpdateApp changes an app's limits, TTL, note or enabled flag.
func (m *Manager) UpdateApp(in AppInput) (AppView, error) {
	if err := in.normalize(); err != nil {
		return AppView{}, err
	}
	a := m.findApp(in.Name)
	if a == nil {
		return AppView{}, fmt.Errorf("app %q not found", in.Name)
	}
	a.Enabled, a.MaxSessions, a.CredentialTTL, a.Note = in.Enabled, in.MaxSessions, in.CredentialTTL, in.Note
	if err := memory.Update(db(), TableTURNApps, a); err != nil {
		return AppView{}, err
	}
	m.refreshApps()
	return m.toView(a, false), nil
}

// RotateSecret replaces an app's secret; credentials signed with the old one
// stop working immediately (existing sessions keep running until they end).
func (m *Manager) RotateSecret(name string) (AppView, error) {
	a := m.findApp(name)
	if a == nil {
		return AppView{}, fmt.Errorf("app %q not found", name)
	}
	a.Secret = newSecret()
	if err := memory.Update(db(), TableTURNApps, a); err != nil {
		return AppView{}, err
	}
	m.refreshApps()
	m.events.add(Event{Kind: "secret-rotated", App: name})
	return m.toView(a, true), nil
}

// DeleteApp removes an app (system apps are managed by Redock).
func (m *Manager) DeleteApp(name string) error {
	a := m.findApp(name)
	if a == nil {
		return fmt.Errorf("app %q not found", name)
	}
	if a.System {
		return fmt.Errorf("app %q is managed by Redock", name)
	}
	if err := memory.Delete[*TURNAppEntity](db(), TableTURNApps, a.ID); err != nil {
		return err
	}
	m.refreshApps()
	m.events.add(Event{Kind: "app-deleted", App: name})
	return nil
}

// EnsureSystemApp returns the secret of a Redock-managed app, creating it on
// first use (e.g. "galene").
func (m *Manager) EnsureSystemApp(name string) (string, error) {
	if a := m.findApp(name); a != nil {
		return a.Secret, nil
	}
	a := &TURNAppEntity{
		Name: name, Secret: newSecret(), Enabled: true, CredentialTTL: DefaultCredentialTTL,
		System: true, Note: "Managed by Redock", CreatedAt: time.Now(),
	}
	if err := memory.Create(db(), TableTURNApps, a); err != nil {
		return "", err
	}
	m.refreshApps()
	return a.Secret, nil
}

// IssueCredentials signs credentials for app (dashboard testing, Redock's
// own consumers). External platforms sign their own with the app secret.
func (m *Manager) IssueCredentials(appName, user string) (Credentials, error) {
	a := m.findApp(appName)
	if a == nil {
		return Credentials{}, fmt.Errorf("app %q not found", appName)
	}
	ttl := time.Duration(a.CredentialTTL) * time.Second
	if ttl <= 0 {
		ttl = DefaultCredentialTTL * time.Second
	}
	return GenerateCredentials(a.Name, a.Secret, user, ttl), nil
}

// ICEServerURLs are the URLs clients should use, from the configuration.
func (m *Manager) ICEServerURLs() []string {
	cfg := m.loadConfig()
	if cfg == nil {
		return nil
	}
	host := cfg.PublicHost
	if host == "" {
		host = cfg.RelayIP
	}
	if strings.Contains(host, ":") { // IPv6 literal
		host = "[" + host + "]"
	}
	urls := []string{
		fmt.Sprintf("stun:%s:%d", host, cfg.Port),
		fmt.Sprintf("turn:%s:%d?transport=udp", host, cfg.Port),
		fmt.Sprintf("turn:%s:%d?transport=tcp", host, cfg.Port),
	}
	m.mu.Lock()
	tlsOn := m.tlsActive
	m.mu.Unlock()
	if tlsOn && cfg.PublicHost != "" && net.ParseIP(cfg.PublicHost) == nil {
		urls = append(urls, fmt.Sprintf("turns:%s:%d?transport=tcp", cfg.PublicHost, cfg.TLSPort))
	}
	return urls
}

// ---------------------------------------------------------------------------
// Server lifecycle

func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return nil
	}
	cfg := m.loadConfig()
	if cfg == nil {
		return fmt.Errorf("database not ready")
	}
	if err := validateConfig(cfg); err != nil {
		m.lastErr = err.Error()
		return err
	}
	err := m.startLocked(*cfg)
	if err != nil {
		m.lastErr = err.Error()
	} else {
		m.lastErr = ""
	}
	return err
}

func (m *Manager) startLocked(cfg TURNConfigEntity) error {
	m.cfg = cfg
	m.refreshApps()
	allow, _ := parseCIDRs(cfg.AllowCIDRs)
	deny, _ := parseCIDRs(cfg.DenyCIDRs)
	m.policy = &relayPolicy{allowPrivate: cfg.AllowPrivate, allow: allow, deny: deny}

	gen := &relayGenerator{
		inner: &turn.RelayAddressGeneratorPortRange{
			RelayAddress: net.ParseIP(cfg.RelayIP), Address: "0.0.0.0",
			MinPort: cfg.RelayMin, MaxPort: cfg.RelayMax,
		},
		registry: m.registry,
		kbps:     cfg.MaxBitrateKbps,
	}
	addr := ":" + strconv.Itoa(cfg.Port)

	udp, err := net.ListenPacket("udp4", addr)
	if err != nil {
		return fmt.Errorf("listen udp %s: %w", addr, err)
	}
	tcp, err := net.Listen("tcp4", addr)
	if err != nil {
		udp.Close()
		return fmt.Errorf("listen tcp %s: %w", addr, err)
	}
	listeners := []turn.ListenerConfig{{
		Listener: &filteredListener{tcp, m.guard}, RelayAddressGenerator: gen, PermissionHandler: m.permit,
	}}

	m.tlsActive = false
	if cfg.TLSPort != 0 {
		if tlsLn, err := m.listenTLS(cfg.TLSPort); err != nil {
			log.Printf("turn_server: TURNS disabled: %v", err)
		} else {
			listeners = append(listeners, turn.ListenerConfig{
				Listener: &filteredListener{tlsLn, m.guard}, RelayAddressGenerator: gen, PermissionHandler: m.permit,
			})
			m.tlsActive = true
		}
	}

	server, err := turn.NewServer(turn.ServerConfig{
		Realm:        cfg.Realm,
		AuthHandler:  m.authenticate,
		QuotaHandler: m.quota,
		EventHandler: turn.EventHandler{
			OnAuth:              m.onAuth,
			OnAllocationCreated: m.onAllocationCreated,
			OnAllocationDeleted: m.onAllocationDeleted,
			OnPermissionCreated: func(_, _ net.Addr, _, _, _ string, relay net.Addr, peer net.IP) {
				m.registry.peerAdded(relay, peer)
			},
		},
		PacketConnConfigs: []turn.PacketConnConfig{{
			PacketConn: &filteredPacketConn{udp, m.guard}, RelayAddressGenerator: gen, PermissionHandler: m.permit,
		}},
		ListenerConfigs: listeners,
	})
	if err != nil {
		udp.Close()
		for _, l := range listeners {
			l.Listener.Close()
		}
		return err
	}
	m.server = server
	m.running = true
	m.startedAt = time.Now()
	m.events.add(Event{Kind: "server-started", Detail: fmt.Sprintf("port %d, TLS %v", cfg.Port, m.tlsActive)})
	log.Printf("turn_server: listening on %s (UDP/TCP), TLS %v, relay %s:%d-%d",
		addr, m.tlsActive, cfg.RelayIP, cfg.RelayMin, cfg.RelayMax)
	return nil
}

// listenTLS serves TURNS with the API gateway's certificate (Let's Encrypt),
// re-read when the files change so renewals apply without a restart.
func (m *Manager) listenTLS(port int) (net.Listener, error) {
	gw := api_gateway.GetGateway()
	if gw == nil {
		return nil, fmt.Errorf("API gateway not initialized")
	}
	cache := &tlsCertCache{}
	if _, err := cache.get(gw); err != nil {
		return nil, err
	}
	conf := &tls.Config{
		MinVersion:     tls.VersionTLS12,
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return cache.get(gw) },
	}
	return tls.Listen("tcp4", ":"+strconv.Itoa(port), conf)
}

// tlsCertCache loads the gateway certificate and reloads it on change.
type tlsCertCache struct {
	mu      sync.Mutex
	path    string
	modTime time.Time
	cert    *tls.Certificate
}

func (c *tlsCertCache) get(gw *api_gateway.Gateway) (*tls.Certificate, error) {
	cfg := gw.GetConfigCopy()
	if cfg == nil || cfg.TLSCertFile == "" || cfg.TLSKeyFile == "" {
		return nil, fmt.Errorf("the API gateway has no TLS certificate configured")
	}
	info, err := os.Stat(cfg.TLSCertFile)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cert != nil && c.path == cfg.TLSCertFile && info.ModTime().Equal(c.modTime) {
		return c.cert, nil
	}
	cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
	if err != nil {
		if c.cert != nil {
			return c.cert, nil // mid-renewal: keep serving the previous one
		}
		return nil, err
	}
	c.cert, c.path, c.modTime = &cert, cfg.TLSCertFile, info.ModTime()
	return c.cert, nil
}

func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running {
		return nil
	}
	m.registry.closeAll()
	err := m.server.Close()
	m.server = nil
	m.running = false
	m.tlsActive = false
	m.events.add(Event{Kind: "server-stopped"})
	log.Printf("turn_server: stopped")
	return err
}

// ---------------------------------------------------------------------------
// TURN callbacks

func (m *Manager) authenticate(ra *turn.RequestAttributes) (string, []byte, bool) {
	if m.guard.isBlocked(hostOf(ra.SrcAddr)) {
		return "", nil, false
	}
	p, err := parseUsername(ra.Username)
	if err != nil || time.Now().After(p.expiry) {
		return "", nil, false
	}
	a, ok := m.app(p.app)
	if !ok || !a.enabled || m.guard.isRevoked(ra.Username) {
		return "", nil, false
	}
	key := turn.GenerateAuthKey(ra.Username, ra.Realm, sign(a.secret, ra.Username))
	return ra.Username, key, true
}

func (m *Manager) quota(username, _ string, src net.Addr) bool {
	p, err := parseUsername(username)
	if err != nil {
		return false
	}
	byIP, byApp := m.registry.counts(hostOf(src), p.app)
	if m.cfg.MaxSessionsPerIP > 0 && byIP >= m.cfg.MaxSessionsPerIP {
		m.events.add(Event{Kind: "quota-exceeded", App: p.app, User: p.user, Client: src.String(), Detail: "per-IP session limit"})
		return false
	}
	limit := DefaultSessionsApp
	if a, ok := m.app(p.app); ok && a.maxSessions > 0 {
		limit = a.maxSessions
	}
	if byApp >= limit {
		m.events.add(Event{Kind: "quota-exceeded", App: p.app, User: p.user, Client: src.String(), Detail: "app session limit"})
		return false
	}
	return true
}

// permit applies the relay policy; refusals are logged once per client/peer
// per minute so a misbehaving client can't flood the log.
func (m *Manager) permit(client net.Addr, peer net.IP) bool {
	if m.policy.permits(peer) {
		return true
	}
	key := hostOf(client) + "|" + peer.String()
	m.deniedMu.Lock()
	last, seen := m.denied[key]
	now := time.Now()
	if !seen || now.Sub(last) > time.Minute {
		m.denied[key] = now
		if len(m.denied) > 4096 {
			m.denied = make(map[string]time.Time)
		}
		m.deniedMu.Unlock()
		m.events.add(Event{Kind: "relay-denied", Client: client.String(), Detail: "peer " + peer.String()})
		return false
	}
	m.deniedMu.Unlock()
	return false
}

func (m *Manager) onAuth(src, _ net.Addr, _, username, _ string, method string, verdict bool) {
	if verdict {
		return
	}
	ip := hostOf(src)
	p, _ := parseUsername(username)
	if username == "" {
		return // the unauthenticated first Allocate: a normal challenge, not a failure
	}
	m.events.add(Event{Kind: "auth-failed", App: p.app, User: p.user, Client: src.String(), Detail: method})
	if m.guard.authFailed(ip, m.cfg.AuthFailLimit, time.Duration(m.cfg.BlockMinutes)*time.Minute) {
		m.events.add(Event{Kind: "ip-blocked", Client: ip, Detail: "too many failed authentications"})
		log.Printf("turn_server: blocked %s for %d min after repeated auth failures", ip, m.cfg.BlockMinutes)
	}
}

func (m *Manager) onAllocationCreated(src, dst net.Addr, protocol, userID, _ string, relay net.Addr, _ int) {
	p, _ := parseUsername(userID)
	s := &Session{
		App: p.app, User: p.user, Username: userID, ClientAddr: src.String(), Protocol: protocol,
		RelayAddr: relay.String(), StartedAt: time.Now(), tuple: fiveTuple(protocol, src, dst),
	}
	m.registry.created(s, relay)
	if m.cfg.MaxSessionMinutes > 0 && s.conn != nil {
		conn := s.conn
		s.deadline = time.AfterFunc(time.Duration(m.cfg.MaxSessionMinutes)*time.Minute, func() {
			m.events.add(Event{Kind: "session-expired", App: p.app, User: p.user, Client: src.String()})
			_ = conn.Close()
		})
	}
	m.events.add(Event{Kind: "session-started", App: p.app, User: p.user, Client: src.String(), Detail: protocol + " relay " + relay.String()})
}

func (m *Manager) onAllocationDeleted(src, dst net.Addr, protocol, _, _ string) {
	s := m.registry.deleted(fiveTuple(protocol, src, dst))
	if s == nil {
		return
	}
	var in, out int64
	if s.conn != nil {
		in, out = s.conn.in.Load(), s.conn.out.Load()
	}
	m.events.add(Event{
		Kind: "session-ended", App: s.App, User: s.User, Client: s.ClientAddr,
		Detail: fmt.Sprintf("%s, in %d B, out %d B, %d peers", time.Since(s.StartedAt).Round(time.Second), in, out, len(s.peers)),
	})
}

// ---------------------------------------------------------------------------
// Dashboard operations

// KillSession ends a session and revokes its credential so the client can't
// immediately reconnect with it; optionally blocks the client IP too.
func (m *Manager) KillSession(id string, blockIP bool) error {
	s, err := m.registry.kill(id)
	if err != nil {
		return err
	}
	if p, perr := parseUsername(s.Username); perr == nil {
		m.guard.revoke(s.Username, p.expiry)
	}
	detail := "credential revoked"
	if blockIP {
		ip := hostOf(stringAddr(s.ClientAddr))
		m.guard.block(ip, time.Duration(m.cfg.BlockMinutes)*time.Minute, "session killed by an administrator")
		detail += ", IP blocked"
	}
	m.events.add(Event{Kind: "session-killed", App: s.App, User: s.User, Client: s.ClientAddr, Detail: detail})
	return nil
}

func (m *Manager) Sessions() []Session { return m.registry.list() }

func (m *Manager) Events() []Event { return m.events.recent() }

// BlockedIP is an entry of the block list.
type BlockedIP struct {
	IP     string    `json:"ip"`
	Until  time.Time `json:"until"`
	Reason string    `json:"reason"`
}

func (m *Manager) Blocked() []BlockedIP {
	list := m.guard.blockedList()
	out := make([]BlockedIP, 0, len(list))
	for ip, b := range list {
		out = append(out, BlockedIP{IP: ip, Until: b.Until, Reason: b.Reason})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Until.After(out[j].Until) })
	return out
}

func (m *Manager) Unblock(ip string) {
	m.guard.unblock(ip)
	m.events.add(Event{Kind: "ip-unblocked", Client: ip})
}

// Status is the runtime state for the dashboard.
type Status struct {
	Running   bool       `json:"running"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	LastError string     `json:"last_error,omitempty"`
	TLS       bool       `json:"tls"`
	Sessions  int        `json:"sessions"`
	URLs      []string   `json:"urls"`
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	st := Status{Running: m.running, LastError: m.lastErr, TLS: m.tlsActive}
	if m.running {
		t := m.startedAt
		st.StartedAt = &t
	}
	m.mu.Unlock()
	st.Sessions = len(m.registry.list())
	st.URLs = m.ICEServerURLs()
	return st
}
