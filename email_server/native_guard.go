package email_server

import (
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// The container setup ran fail2ban. Ports 25 and 587 face the internet, so
// something has to answer password guessing and connection floods; this is
// that something. It works on the accept path, before a session exists, so a
// blocked address costs nothing but a closed socket.

const (
	// authFailureWindow is how long failed logins are remembered.
	authFailureWindow = 10 * time.Minute
	// connectionWindow is the window connection rates are measured over.
	connectionWindow = 1 * time.Minute
	// guardSweepInterval is how often expired entries are dropped.
	guardSweepInterval = 5 * time.Minute
)

// BlockedClient is an address the guard is currently refusing.
type BlockedClient struct {
	IP        string    `json:"ip"`
	Reason    string    `json:"reason"`
	BlockedAt time.Time `json:"blocked_at"`
	Until     time.Time `json:"until"`
	Failures  int       `json:"failures"`
	Manual    bool      `json:"manual"`
}

// connectionGuard tracks per-address behaviour and blocks the addresses that
// misbehave.
type connectionGuard struct {
	mu sync.Mutex

	// failures counts recent authentication failures per address.
	failures map[string][]time.Time
	// relays counts recent attempts to have mail delivered somewhere this
	// server has no business delivering to.
	relays map[string][]time.Time
	// connections counts recent connections per address.
	connections map[string][]time.Time
	// blocked maps an address to when its block expires.
	blocked map[string]*BlockedClient
	// allowList never gets blocked, whatever it does.
	allowList []*net.IPNet
}

func newConnectionGuard() *connectionGuard {
	guard := &connectionGuard{
		failures:    make(map[string][]time.Time),
		relays:      make(map[string][]time.Time),
		connections: make(map[string][]time.Time),
		blocked:     make(map[string]*BlockedClient),
	}

	// Loopback and private ranges are where the dashboard and local clients
	// live; locking those out would break the machine itself.
	for _, cidr := range []string{"127.0.0.0/8", "::1/128", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fc00::/7"} {
		if _, network, err := net.ParseCIDR(cidr); err == nil {
			guard.allowList = append(guard.allowList, network)
		}
	}
	return guard
}

// guard returns the manager's connection guard.
func (m *EmailManager) guard() *connectionGuard {
	n := m.Native()
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.guard == nil {
		n.guard = newConnectionGuard()
	}
	return n.guard
}

// exempt reports whether an address is on the allow list.
func (g *connectionGuard) exempt(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, network := range g.allowList {
		if network.Contains(parsed) {
			return true
		}
	}
	return false
}

// Blocked reports whether an address is currently refused.
func (g *connectionGuard) Blocked(ip string) (bool, *BlockedClient) {
	if ip == "" || g.exempt(ip) {
		return false, nil
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	entry, ok := g.blocked[ip]
	if !ok {
		return false, nil
	}
	if time.Now().After(entry.Until) {
		delete(g.blocked, ip)
		return false, nil
	}
	return true, entry
}

// RecordConnection notes a new connection and reports whether the address has
// exceeded the allowed rate.
func (g *connectionGuard) RecordConnection(ip string, limit int) bool {
	if ip == "" || limit <= 0 || g.exempt(ip) {
		return false
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	cutoff := time.Now().Add(-connectionWindow)
	kept := g.connections[ip][:0]
	for _, when := range g.connections[ip] {
		if when.After(cutoff) {
			kept = append(kept, when)
		}
	}
	kept = append(kept, time.Now())
	g.connections[ip] = kept

	return len(kept) > limit
}

// RecordAuthFailure counts a failed login and reports whether the address has
// now earned a block.
func (g *connectionGuard) RecordAuthFailure(ip string, limit int) (bool, int) {
	if ip == "" || limit <= 0 || g.exempt(ip) {
		return false, 0
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	cutoff := time.Now().Add(-authFailureWindow)
	kept := g.failures[ip][:0]
	for _, when := range g.failures[ip] {
		if when.After(cutoff) {
			kept = append(kept, when)
		}
	}
	kept = append(kept, time.Now())
	g.failures[ip] = kept

	return len(kept) >= limit, len(kept)
}

// RecordRelayAttempt counts a refused relay and reports whether the address has
// now earned a block.
func (g *connectionGuard) RecordRelayAttempt(ip string, limit int) (bool, int) {
	if ip == "" || limit <= 0 || g.exempt(ip) {
		return false, 0
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	cutoff := time.Now().Add(-authFailureWindow)
	kept := g.relays[ip][:0]
	for _, when := range g.relays[ip] {
		if when.After(cutoff) {
			kept = append(kept, when)
		}
	}
	kept = append(kept, time.Now())
	g.relays[ip] = kept

	return len(kept) >= limit, len(kept)
}

// Block refuses an address for a while.
func (g *connectionGuard) Block(ip, reason string, duration time.Duration, failures int, manual bool) *BlockedClient {
	if ip == "" || (!manual && g.exempt(ip)) {
		return nil
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now()
	entry := &BlockedClient{
		IP:        ip,
		Reason:    reason,
		BlockedAt: now,
		Until:     now.Add(duration),
		Failures:  failures,
		Manual:    manual,
	}
	g.blocked[ip] = entry
	return entry
}

// Unblock lifts a block.
func (g *connectionGuard) Unblock(ip string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, ok := g.blocked[ip]; !ok {
		return false
	}
	delete(g.blocked, ip)
	delete(g.failures, ip)
	delete(g.relays, ip)
	return true
}

// List returns the current blocks, soonest to expire last.
func (g *connectionGuard) List() []BlockedClient {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now()
	out := make([]BlockedClient, 0, len(g.blocked))
	for ip, entry := range g.blocked {
		if now.After(entry.Until) {
			delete(g.blocked, ip)
			continue
		}
		out = append(out, *entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Until.After(out[j].Until) })
	return out
}

// sweep drops expired bookkeeping so the maps cannot grow without bound.
func (g *connectionGuard) sweep() {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now()
	for ip, entry := range g.blocked {
		if now.After(entry.Until) {
			delete(g.blocked, ip)
		}
	}
	for ip, times := range g.relays {
		if len(times) == 0 || now.Sub(times[len(times)-1]) > authFailureWindow {
			delete(g.relays, ip)
		}
	}
	for ip, times := range g.failures {
		if len(times) == 0 || now.Sub(times[len(times)-1]) > authFailureWindow {
			delete(g.failures, ip)
		}
	}
	for ip, times := range g.connections {
		if len(times) == 0 || now.Sub(times[len(times)-1]) > connectionWindow {
			delete(g.connections, ip)
		}
	}
}

// ---- manager-level hooks ----

// allowConnection is called on every accepted connection. It closes the door on
// a blocked address and enforces the connection rate.
func (m *EmailManager) allowConnection(service, ip string) bool {
	cfg := m.nativeConfig()
	if !cfg.GuardEnabled {
		return true
	}

	guard := m.guard()

	if blocked, entry := guard.Blocked(ip); blocked {
		m.logMailEvent(mailEvent{
			Direction: "system", Status: "blocked", Service: service, RemoteIP: ip,
			Detail: fmt.Sprintf("refused: %s (until %s)", entry.Reason, entry.Until.Format(time.RFC3339)),
		})
		return false
	}

	if guard.RecordConnection(ip, cfg.MaxConnectionsPerMinute) {
		entry := guard.Block(ip, "too many connections", time.Duration(cfg.BlockMinutes)*time.Minute, 0, false)
		if entry != nil {
			m.logMailEvent(mailEvent{
				Direction: "system", Status: "blocked", Service: service, RemoteIP: ip,
				Detail: fmt.Sprintf("more than %d connections in a minute; blocked for %d minutes",
					cfg.MaxConnectionsPerMinute, cfg.BlockMinutes),
			})
		}
		return false
	}
	return true
}

// noteAuthFailure records a failed login and blocks the address once it has
// failed too often.
func (m *EmailManager) noteAuthFailure(service, ip, username string) {
	cfg := m.nativeConfig()
	if !cfg.GuardEnabled || ip == "" {
		return
	}

	guard := m.guard()
	shouldBlock, failures := guard.RecordAuthFailure(ip, cfg.MaxAuthFailures)
	if !shouldBlock {
		return
	}

	entry := guard.Block(ip,
		fmt.Sprintf("%d failed logins", failures),
		time.Duration(cfg.BlockMinutes)*time.Minute, failures, false)
	if entry == nil {
		return
	}

	m.logMailEvent(mailEvent{
		Direction: "system", Status: "blocked", Service: service, RemoteIP: ip, From: username,
		Detail: fmt.Sprintf("%d failed logins in %s; blocked for %d minutes",
			failures, authFailureWindow, cfg.BlockMinutes),
	})
}

// noteRelayAttempt records a refused relay and blocks the address once it has
// asked too often. Unlike a failed login, which an ordinary user can produce by
// mistyping, this is not something a correctly configured client ever does.
func (m *EmailManager) noteRelayAttempt(service, ip, recipient string) {
	cfg := m.nativeConfig()
	if !cfg.GuardEnabled || ip == "" {
		return
	}

	guard := m.guard()
	shouldBlock, attempts := guard.RecordRelayAttempt(ip, cfg.MaxRelayAttempts)
	if !shouldBlock {
		return
	}

	entry := guard.Block(ip,
		fmt.Sprintf("%d relay attempts", attempts),
		time.Duration(cfg.BlockMinutes)*time.Minute, attempts, false)
	if entry == nil {
		return
	}

	m.logMailEvent(mailEvent{
		Direction: "system", Status: "blocked", Service: service, RemoteIP: ip, To: recipient,
		Detail: fmt.Sprintf("%d relay attempts in %s; blocked for %d minutes",
			attempts, authFailureWindow, cfg.BlockMinutes),
	})
}

// BlockedClients exposes the current blocks to the dashboard.
func (m *EmailManager) BlockedClients() []BlockedClient { return m.guard().List() }

// BlockClient blocks an address by hand.
func (m *EmailManager) BlockClient(ip, reason string, minutes int) (*BlockedClient, error) {
	if net.ParseIP(strings.TrimSpace(ip)) == nil {
		return nil, fmt.Errorf("%q is not an IP address", ip)
	}
	if minutes <= 0 {
		minutes = m.nativeConfig().BlockMinutes
	}
	if reason == "" {
		reason = "blocked by the operator"
	}

	entry := m.guard().Block(strings.TrimSpace(ip), reason, time.Duration(minutes)*time.Minute, 0, true)
	m.logMailEvent(mailEvent{
		Direction: "system", Status: "blocked", Service: "guard", RemoteIP: ip,
		Detail: fmt.Sprintf("blocked by the operator for %d minutes: %s", minutes, reason),
	})
	return entry, nil
}

// UnblockClient lifts a block.
func (m *EmailManager) UnblockClient(ip string) error {
	if !m.guard().Unblock(strings.TrimSpace(ip)) {
		return fmt.Errorf("%s is not blocked", ip)
	}
	m.logMailEvent(mailEvent{
		Direction: "system", Status: "unblocked", Service: "guard", RemoteIP: ip,
		Detail: "block lifted by the operator",
	})
	return nil
}

// clientGuard returns the running guard, or nil when the server has never
// started and there is nothing tracked yet.
func (n *NativeServer) clientGuard() *connectionGuard {
	if n == nil {
		return nil
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.guard
}

// clearCounters forgets the per-address rate and failure history, reporting how
// many addresses were being tracked. Active blocks are deliberately left in
// place: they are a decision already made about a client, and re-earning one
// takes another round of failures the counters no longer remember.
func (g *connectionGuard) clearCounters() int {
	g.mu.Lock()
	defer g.mu.Unlock()

	dropped := len(g.failures) + len(g.connections) + len(g.relays)
	if dropped == 0 {
		return 0
	}
	g.failures = make(map[string][]time.Time)
	g.connections = make(map[string][]time.Time)
	g.relays = make(map[string][]time.Time)
	return dropped
}

// startGuardSweep clears expired entries while the server runs.
func (n *NativeServer) startGuardSweep(stop chan struct{}) {
	ticker := time.NewTicker(guardSweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			n.manager.guard().sweep()
		}
	}
}
