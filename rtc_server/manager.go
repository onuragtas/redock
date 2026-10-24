// Package rtc_server embeds the Galène SFU (github.com/jech/galene) for
// WebRTC conferencing. Galène is a library here: it runs inside the Redock
// process, behind the API gateway, configured from the dashboard.
//
// Galène is configured through package-level variables and registers its
// HTTP handlers on http.DefaultServeMux, so there is exactly one instance per
// process; the Manager serialises start/stop around that.
package rtc_server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	dockermanager "redock/docker-manager"
	"redock/platform/database"
	"redock/platform/memory"

	"redock/turn_server"

	"github.com/jech/galene/diskwriter"
	"github.com/jech/galene/group"
	"github.com/jech/galene/ice"
	"github.com/jech/galene/token"
	"github.com/jech/galene/turnserver"
	"github.com/jech/galene/webserver"
	"golang.org/x/crypto/bcrypt"
)

const (
	adminUser    = "redock"
	TestRoomName = "redock-test"
)

type Manager struct {
	mu      sync.Mutex
	baseDir string // <workdir>/data/rtc

	running   bool
	startedAt time.Time
	lastErr   string

	ownServer *http.Server  // our listener after the first Galène-run one is stopped
	stopLoop  chan struct{} // ends the periodic maintenance goroutine
}

var (
	manager *Manager
	// galeneServed records that webserver.Serve ran once: it registers
	// Galène's handlers on http.DefaultServeMux, and a second call would
	// panic on duplicate registration. Later starts serve that mux ourselves.
	galeneServed bool
)

// Init creates the manager and starts the SFU if it was enabled.
func Init(dm *dockermanager.DockerEnvironmentManager) {
	manager = &Manager{baseDir: filepath.Join(dm.GetWorkDir(), "data", "rtc")}
	turn_server.OnConfigApplied(manager.refreshICE)
	if cfg := manager.loadConfig(); cfg != nil && cfg.Enabled {
		go func() {
			if err := manager.Start(); err != nil {
				log.Printf("rtc_server: start failed: %v", err)
			}
		}()
	}
}

// GetManager returns the manager (nil before Init).
func GetManager() *Manager { return manager }

// decodeKey decodes a room key (base64url, JWK "k" encoding).
func decodeKey(k string) ([]byte, error) {
	b, err := base64.RawURLEncoding.DecodeString(k)
	if err != nil || len(b) < 16 {
		return nil, fmt.Errorf("invalid room key")
	}
	return b, nil
}

func randomSecret(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// loadConfig returns the single config row, creating it with defaults (and
// generated secrets) on first use.
func (m *Manager) loadConfig() *RTCConfigEntity {
	db := database.GetMemoryDB()
	if db == nil {
		return nil
	}
	if rows := memory.FindAll[*RTCConfigEntity](db, TableRTCConfig); len(rows) > 0 {
		return rows[0]
	}
	cfg := &RTCConfigEntity{
		ListenAddr:       DefaultListenAddr,
		UDPMin:           DefaultUDPMin,
		UDPMax:           DefaultUDPMax,
		TURN:             DefaultTURN,
		RetentionDays:    DefaultRetentionDays,
		AdminPassword:    randomSecret(24),
		TestRoomPassword: randomSecret(9),
	}
	if err := memory.Create(db, TableRTCConfig, cfg); err != nil {
		log.Printf("rtc_server: could not save default config: %v", err)
	}
	return cfg
}

// ConfigView is the configuration as shown in the dashboard (no API secret).
type ConfigView struct {
	Enabled          bool   `json:"enabled"`
	ListenAddr       string `json:"listen_addr"`
	RetentionDays    int    `json:"retention_days"`
	UDPMin           uint16 `json:"udp_min"`
	UDPMax           uint16 `json:"udp_max"`
	TURN             string `json:"turn"`
	PublicHost       string `json:"public_host"`
	TestRoomName     string `json:"test_room_name"`
	TestRoomPassword string `json:"test_room_password"`
}

func (m *Manager) Config() (ConfigView, error) {
	cfg := m.loadConfig()
	if cfg == nil {
		return ConfigView{}, fmt.Errorf("database not ready")
	}
	return ConfigView{
		Enabled: cfg.Enabled, ListenAddr: cfg.ListenAddr, RetentionDays: cfg.RetentionDays, UDPMin: cfg.UDPMin, UDPMax: cfg.UDPMax,
		TURN: cfg.TURN, PublicHost: cfg.PublicHost,
		TestRoomName: TestRoomName, TestRoomPassword: cfg.TestRoomPassword,
	}, nil
}

// ConfigInput is what the dashboard may change.
type ConfigInput struct {
	Enabled       bool   `json:"enabled"`
	RetentionDays int    `json:"retention_days"`
	ListenAddr    string `json:"listen_addr"`
	UDPMin        uint16 `json:"udp_min"`
	UDPMax        uint16 `json:"udp_max"`
	TURN          string `json:"turn"`
	PublicHost    string `json:"public_host"`
}

var hostPattern = regexp.MustCompile(`^(?i)[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)

func (in *ConfigInput) validate() error {
	in.ListenAddr = strings.TrimSpace(in.ListenAddr)
	if in.ListenAddr == "" {
		in.ListenAddr = DefaultListenAddr
	}
	if _, port, err := net.SplitHostPort(in.ListenAddr); err != nil || port == "" || port == "0" {
		return fmt.Errorf("listen address must be host:port, e.g. %s", DefaultListenAddr)
	}
	if (in.UDPMin == 0) != (in.UDPMax == 0) || in.UDPMin > in.UDPMax {
		return fmt.Errorf("UDP range: give both ends with min <= max (or 0/0 for any port)")
	}
	in.TURN = strings.TrimSpace(in.TURN)
	if in.TURN != "" && in.TURN != "auto" && in.TURN != TURNRedock {
		if _, _, err := net.SplitHostPort(in.TURN); err != nil {
			return fmt.Errorf(`TURN must be "redock", "auto", empty (disabled) or host:port`)
		}
	}
	if in.RetentionDays < 0 || in.RetentionDays > 3650 {
		return fmt.Errorf("recording retention must be 0-3650 days (0 = keep forever)")
	}
	in.PublicHost = strings.ToLower(strings.TrimSpace(in.PublicHost))
	if in.PublicHost != "" && !hostPattern.MatchString(in.PublicHost) {
		return fmt.Errorf("public host must be a domain name like rtc.example.com")
	}
	return nil
}

// UpdateConfig saves the configuration, syncs the gateway route and
// restarts (or stops) the SFU so the change takes effect.
func (m *Manager) UpdateConfig(in ConfigInput) error {
	if err := in.validate(); err != nil {
		return err
	}
	cfg := m.loadConfig()
	if cfg == nil {
		return fmt.Errorf("database not ready")
	}
	prevHost := cfg.PublicHost
	cfg.Enabled, cfg.ListenAddr, cfg.UDPMin, cfg.UDPMax = in.Enabled, in.ListenAddr, in.UDPMin, in.UDPMax
	cfg.TURN, cfg.PublicHost, cfg.RetentionDays = in.TURN, in.PublicHost, in.RetentionDays
	if err := memory.Update(database.GetMemoryDB(), TableRTCConfig, cfg); err != nil {
		return err
	}

	switch {
	case cfg.PublicHost != "":
		if err := ensureGatewayRoute(cfg.PublicHost, cfg.ListenAddr); err != nil {
			return fmt.Errorf("saved, but the gateway route failed: %w", err)
		}
	case prevHost != "":
		removeGatewayRoute()
	}

	if err := m.Stop(); err != nil {
		return err
	}
	if cfg.Enabled {
		return m.Start()
	}
	return nil
}

// Start brings the SFU up with the current configuration.
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
	err := m.startLocked(cfg)
	if err != nil {
		m.lastErr = err.Error()
	} else {
		m.lastErr = ""
	}
	return err
}

func (m *Manager) startLocked(cfg *RTCConfigEntity) error {
	dataDir := filepath.Join(m.baseDir, "data")
	groupsDir := filepath.Join(m.baseDir, "groups")
	recDir := filepath.Join(m.baseDir, "recordings")
	staticDir := filepath.Join(m.baseDir, "static")
	for _, d := range []string{dataDir, groupsDir, recDir, filepath.Join(dataDir, "var")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return err
		}
	}
	if err := extractStatic(staticDir); err != nil {
		return fmt.Errorf("unpack web client: %w", err)
	}
	if err := writeGaleneConfig(dataDir, cfg); err != nil {
		return err
	}
	if err := writeTestRoom(groupsDir, cfg.TestRoomPassword); err != nil {
		return err
	}
	if err := extractJoinUI(filepath.Join(staticDir, "redock")); err != nil {
		return fmt.Errorf("unpack join page: %w", err)
	}

	// Galène's configuration lives in package variables.
	group.DataDirectory = dataDir
	group.Directory = groupsDir
	group.UDPMin, group.UDPMax = cfg.UDPMin, cfg.UDPMax
	diskwriter.Directory = recDir
	webserver.StaticRoot = staticDir
	webserver.Insecure = true // TLS terminates at the API gateway
	ice.ICEFilename = filepath.Join(dataDir, "ice-servers.json")
	if err := writeICEServers(ice.ICEFilename, cfg.TURN); err != nil {
		return fmt.Errorf("TURN: %w", err)
	}
	if cfg.TURN == TURNRedock {
		turnserver.Address = "" // Redock's TURN replaces the built-in one
	} else {
		turnserver.Address = cfg.TURN
	}
	token.SetStatefulFilename(filepath.Join(dataDir, "var", "tokens.jsonl"))

	if err := m.syncRoomFiles(groupsDir); err != nil {
		return err
	}
	group.Update()
	ice.Update() // also starts/stops the built-in TURN server

	if err := m.serveLocked(cfg.ListenAddr, dataDir); err != nil {
		_ = turnserver.Stop()
		return fmt.Errorf("listen on %s: %w", cfg.ListenAddr, err)
	}

	m.stopLoop = make(chan struct{})
	go maintenance(m.stopLoop, m.applyRetention)
	go m.applyRetention()
	m.running = true
	m.startedAt = time.Now()
	log.Printf("rtc_server: Galène SFU listening on %s", cfg.ListenAddr)
	return nil
}

// serveLocked starts the HTTP listener: through webserver.Serve the first
// time (which registers Galène's handlers), then with our own server on the
// already-populated default mux.
func (m *Manager) serveLocked(addr, dataDir string) error {
	if !galeneServed {
		galeneServed = true // handlers are registered even if listening fails
		return webserver.Serve(addr, dataDir)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s := &http.Server{Addr: addr, ReadHeaderTimeout: 60 * time.Second, IdleTimeout: 120 * time.Second}
	s.RegisterOnShutdown(func() { group.Shutdown("server is shutting down") })
	m.ownServer = s
	go func() {
		if err := s.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("rtc_server: listener error: %v", err)
		}
	}()
	return nil
}

// maintenance mirrors Galène's main loop: refresh group definitions and
// expire tokens periodically.
func maintenance(stop chan struct{}, retention func()) {
	t := time.NewTicker(15 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			group.Update()
			_ = token.Expire()
			retention()
		}
	}
}

// Stop disconnects everyone and shuts the SFU down.
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running {
		return nil
	}
	close(m.stopLoop)
	if m.ownServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = m.ownServer.Shutdown(ctx)
		cancel()
		m.ownServer = nil
	} else {
		webserver.Shutdown()
	}
	_ = turnserver.Stop()
	m.running = false
	log.Printf("rtc_server: Galène SFU stopped")
	return nil
}

// RoomStatus is one active room.
type RoomStatus struct {
	Name    string `json:"name"`
	Clients int    `json:"clients"`
	Locked  bool   `json:"locked"`
}

// Status is the runtime state shown in the dashboard.
type Status struct {
	Running   bool         `json:"running"`
	StartedAt *time.Time   `json:"started_at,omitempty"`
	LastError string       `json:"last_error,omitempty"`
	Version   string       `json:"version"`
	Rooms     []RoomStatus `json:"rooms"`
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	st := Status{Running: m.running, LastError: m.lastErr, Version: galeneVersion, Rooms: []RoomStatus{}}
	if m.running {
		t := m.startedAt
		st.StartedAt = &t
	}
	m.mu.Unlock()
	if !st.Running {
		return st
	}
	for _, name := range group.GetNames() {
		g := group.Get(name)
		if g == nil {
			continue
		}
		locked, _ := g.Locked()
		st.Rooms = append(st.Rooms, RoomStatus{Name: name, Clients: g.ClientCount(), Locked: locked})
	}
	return st
}

// writeGaleneConfig writes data/config.json: the admin user Redock uses for
// Galène's API (bcrypt-hashed) and, behind the gateway, the public URL so
// Galène builds correct links.
func writeGaleneConfig(dataDir string, cfg *RTCConfigEntity) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(cfg.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	conf := map[string]any{
		"users": map[string]any{
			adminUser: map[string]any{
				"password":    map[string]any{"type": "bcrypt", "key": string(hash)},
				"permissions": "admin",
			},
		},
		"writableGroups": true,
	}
	if cfg.PublicHost != "" {
		conf["proxyURL"] = "https://" + cfg.PublicHost + "/"
	}
	return writeJSON(filepath.Join(dataDir, "config.json"), conf)
}

// writeTestRoom (re)writes the built-in test room: unlisted, any name, the
// configured password, permission to present.
func writeTestRoom(groupsDir, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	room := map[string]any{
		"description": "Redock test room",
		"public":      false,
		"wildcard-user": map[string]any{
			"password":    map[string]any{"type": "bcrypt", "key": string(hash)},
			"permissions": "present",
		},
	}
	return writeJSON(filepath.Join(groupsDir, TestRoomName+".json"), room)
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// TURNRedock selects Redock's own TURN server for Galène.
const TURNRedock = "redock"

// galeneTURNApp is the TURN app Galène signs its relay credentials with.
const galeneTURNApp = "galene"

// writeICEServers points Galène at Redock's TURN server (Galène signs
// short-lived hmac-sha1 credentials with the "galene" app secret), or removes
// the file so Galène falls back to its built-in TURN.
func writeICEServers(path, mode string) error {
	if mode != TURNRedock {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	tm := turn_server.GetManager()
	if tm == nil {
		return fmt.Errorf("Redock TURN server is not initialized")
	}
	secret, err := tm.EnsureSystemApp(galeneTURNApp)
	if err != nil {
		return err
	}
	servers := []map[string]any{{
		"urls":           tm.ICEServerURLs(),
		"username":       galeneTURNApp,
		"credential":     secret,
		"credentialType": "hmac-sha1",
	}}
	return writeJSON(path, servers)
}

// refreshICE rewrites Galène's ICE servers after the TURN configuration
// changed (new host, ports, TLS), when Galène uses Redock's TURN.
func (m *Manager) refreshICE() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running {
		return
	}
	cfg := m.loadConfig()
	if cfg == nil || cfg.TURN != TURNRedock {
		return
	}
	if err := writeICEServers(ice.ICEFilename, cfg.TURN); err != nil {
		log.Printf("rtc_server: refresh ICE servers: %v", err)
		return
	}
	ice.Update()
}
