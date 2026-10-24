package turn_server

import (
	"net"
	"strings"
	"sync"
	"time"
)

// relayPolicy decides which peer addresses a client may relay to.
type relayPolicy struct {
	allowPrivate bool
	allow        []*net.IPNet
	deny         []*net.IPNet
}

// blockedRanges are never reachable through the relay unless explicitly
// allowed: the host itself, the LAN, Docker/Tailscale networks and
// non-unicast space. Relaying there would turn TURN into an open door
// into the internal network.
var blockedRanges = mustCIDRs(
	"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16",
	"172.16.0.0/12", "192.0.0.0/24", "192.168.0.0/16", "198.18.0.0/15",
	"224.0.0.0/4", "240.0.0.0/4", "255.255.255.255/32",
	"::/128", "::1/128", "fc00::/7", "fe80::/10", "ff00::/8",
	// IPv4-mapped IPv6 (::ffff:a.b.c.d) is unwrapped to IPv4 before matching.
)

func mustCIDRs(list ...string) []*net.IPNet {
	nets, _ := parseCIDRs(list)
	return nets
}

// parseCIDRs accepts CIDRs and bare IPs; invalid entries are returned.
func parseCIDRs(list []string) ([]*net.IPNet, []string) {
	var nets []*net.IPNet
	var bad []string
	for _, raw := range list {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if !strings.Contains(raw, "/") {
			if ip := net.ParseIP(raw); ip != nil {
				if ip.To4() != nil {
					raw += "/32"
				} else {
					raw += "/128"
				}
			}
		}
		if _, n, err := net.ParseCIDR(raw); err == nil {
			nets = append(nets, n)
		} else {
			bad = append(bad, raw)
		}
	}
	return nets, bad
}

func inNets(ip net.IP, nets []*net.IPNet) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// permits reports whether relaying to peer is allowed. Explicit allows win,
// then explicit denies, then the private-range ban.
func (p *relayPolicy) permits(peer net.IP) bool {
	if peer == nil {
		return false
	}
	if v4 := peer.To4(); v4 != nil {
		peer = v4
	}
	if inNets(peer, p.allow) {
		return true
	}
	if inNets(peer, p.deny) {
		return false
	}
	if !p.allowPrivate && inNets(peer, blockedRanges) {
		return false
	}
	return true
}

// guard tracks failed authentications per source IP and blocks IPs that
// exceed the limit, plus credentials revoked by killing their session.
type guard struct {
	mu       sync.Mutex
	failures map[string][]time.Time // ip -> recent failure times
	blocked  map[string]blockInfo   // ip -> until
	revoked  map[string]time.Time   // TURN username -> until (its expiry)
}

type blockInfo struct {
	Until  time.Time `json:"until"`
	Reason string    `json:"reason"`
}

func newGuard() *guard {
	return &guard{
		failures: make(map[string][]time.Time),
		blocked:  make(map[string]blockInfo),
		revoked:  make(map[string]time.Time),
	}
}

func hostOf(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	h, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return h
}

// isBlocked reports whether ip is currently blocked.
func (g *guard) isBlocked(ip string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	b, ok := g.blocked[ip]
	if !ok {
		return false
	}
	if time.Now().After(b.Until) {
		delete(g.blocked, ip)
		return false
	}
	return true
}

// authFailed counts a failure; returns true when this one triggered a block.
func (g *guard) authFailed(ip string, limit int, blockFor time.Duration) bool {
	if ip == "" || limit <= 0 {
		return false
	}
	now := time.Now()
	g.mu.Lock()
	defer g.mu.Unlock()
	recent := g.failures[ip][:0]
	for _, t := range g.failures[ip] {
		if now.Sub(t) < time.Minute {
			recent = append(recent, t)
		}
	}
	recent = append(recent, now)
	g.failures[ip] = recent
	if len(recent) < limit {
		return false
	}
	delete(g.failures, ip)
	g.blocked[ip] = blockInfo{Until: now.Add(blockFor), Reason: "too many failed authentications"}
	return true
}

func (g *guard) block(ip string, d time.Duration, reason string) {
	g.mu.Lock()
	g.blocked[ip] = blockInfo{Until: time.Now().Add(d), Reason: reason}
	g.mu.Unlock()
}

func (g *guard) unblock(ip string) {
	g.mu.Lock()
	delete(g.blocked, ip)
	delete(g.failures, ip)
	g.mu.Unlock()
}

func (g *guard) blockedList() map[string]blockInfo {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	out := make(map[string]blockInfo, len(g.blocked))
	for ip, b := range g.blocked {
		if now.After(b.Until) {
			delete(g.blocked, ip)
			continue
		}
		out[ip] = b
	}
	return out
}

// revoke refuses a username until it would have expired anyway.
func (g *guard) revoke(username string, until time.Time) {
	g.mu.Lock()
	g.revoked[username] = until
	g.mu.Unlock()
}

func (g *guard) isRevoked(username string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	until, ok := g.revoked[username]
	if !ok {
		return false
	}
	if time.Now().After(until) {
		delete(g.revoked, username)
		return false
	}
	return true
}

// filteredPacketConn drops datagrams from blocked source IPs before the TURN
// server parses them, so a blocked client costs nothing beyond the read.
type filteredPacketConn struct {
	net.PacketConn
	g *guard
}

func (c *filteredPacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	for {
		n, addr, err := c.PacketConn.ReadFrom(p)
		if err != nil || !c.g.isBlocked(hostOf(addr)) {
			return n, addr, err
		}
	}
}

// filteredListener refuses TCP/TLS connections from blocked source IPs.
type filteredListener struct {
	net.Listener
	g *guard
}

func (l *filteredListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		if l.g.isBlocked(hostOf(c.RemoteAddr())) {
			_ = c.Close()
			continue
		}
		return c, nil
	}
}
