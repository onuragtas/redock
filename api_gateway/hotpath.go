package api_gateway

import (
	"bytes"
	"crypto/sha256"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ---------------------------------------------------------------------------
// Request body capture

// capturingBody passes the request body through to the upstream while keeping
// only its first `limit` bytes for the log and counting the total size.
// limit == 0 counts without buffering; limit < 0 keeps everything.
type capturingBody struct {
	io.ReadCloser
	limit int

	mu  sync.Mutex
	buf bytes.Buffer
	n   int64
}

func (c *capturingBody) Read(p []byte) (int, error) {
	n, err := c.ReadCloser.Read(p)
	if n > 0 {
		c.mu.Lock()
		c.n += int64(n)
		switch {
		case c.limit < 0:
			c.buf.Write(p[:n])
		case c.limit > 0:
			if room := c.limit - c.buf.Len(); room > 0 {
				c.buf.Write(p[:min(room, n)])
			}
		}
		c.mu.Unlock()
	}
	return n, err
}

// info reports what was read: the kept snippet, the total size, and whether
// the snippet is shorter than the body (only what the upstream actually read
// is known — an upstream that ignores the body leaves it unread).
func (c *capturingBody) info() bodyLogInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.n == 0 {
		return bodyLogInfo{}
	}
	return bodyLogInfo{
		body:      c.buf.String(),
		truncated: int64(c.buf.Len()) < c.n,
		size:      c.n,
	}
}

// bodiesLogged reports whether anything consumes request/response bodies.
func (g *Gateway) bodiesLogged() bool {
	cfg := g.config
	if cfg == nil {
		return false
	}
	// The console access log never carries bodies (see writeAccessLog), so
	// only telemetry needs them captured.
	return cfg.Observability != nil && cfg.Observability.Enabled
}

// ---------------------------------------------------------------------------
// Client IP

// cloudflareRanges are Cloudflare's published edge ranges
// (https://www.cloudflare.com/ips/). Requests proxied through Cloudflare
// arrive from these, carrying the visitor in CF-Connecting-IP.
var cloudflareRanges = []string{
	"173.245.48.0/20", "103.21.244.0/22", "103.22.200.0/22", "103.31.4.0/22",
	"141.101.64.0/18", "108.162.192.0/18", "190.93.240.0/20", "188.114.96.0/20",
	"197.234.240.0/22", "198.41.128.0/17", "162.158.0.0/15", "104.16.0.0/13",
	"104.24.0.0/14", "172.64.0.0/13", "131.0.72.0/22",
	"2400:cb00::/32", "2606:4700::/32", "2803:f800::/32", "2405:b500::/32",
	"2405:8100::/32", "2a06:98c0::/29", "2c0f:f248::/32",
}

var cloudflareNets = mustCIDRs(cloudflareRanges)

// extraTrustedProxies holds ClientSecurityConfig.TrustedProxies, parsed.
var extraTrustedProxies atomic.Pointer[[]*net.IPNet]

func mustCIDRs(list []string) []*net.IPNet {
	nets, _ := parseCIDRs(list)
	return nets
}

// parseCIDRs accepts CIDRs and bare IPs; invalid entries are skipped and
// returned separately.
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
				bits := 128
				if ip.To4() != nil {
					bits = 32
				}
				raw = ip.String() + "/" + strconv.Itoa(bits)
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

// setTrustedProxies installs the operator's extra trusted proxy ranges.
func setTrustedProxies(list []string) {
	nets, _ := parseCIDRs(list)
	extraTrustedProxies.Store(&nets)
}

func inNets(ip net.IP, nets []*net.IPNet) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// trustedProxy reports whether forwarding headers sent by ip are believed:
// loopback / private / link-local / CGNAT (local reverse proxies, tunnels,
// Docker, Tailscale), Cloudflare, and the configured extra ranges.
func trustedProxy(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || cgnat.Contains(ip) {
		return true
	}
	if inNets(ip, cloudflareNets) {
		return true
	}
	if p := extraTrustedProxies.Load(); p != nil && inNets(ip, *p) {
		return true
	}
	return false
}

var cgnat = mustCIDRs([]string{"100.64.0.0/10"})[0]

// getClientIP extracts the client IP from the request. Forwarding headers
// are only honoured when the direct peer is a trusted proxy — otherwise any
// client could pick its own address and slip past rate limits and blocks.
// X-Forwarded-For is walked right to left, skipping trusted hops, so the
// first untrusted address (the one the nearest trusted proxy saw) wins.
func getClientIP(r *http.Request) string {
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		peer = r.RemoteAddr
	}
	peerIP := net.ParseIP(peer)
	if !trustedProxy(peerIP) {
		return peer
	}

	if inNets(peerIP, cloudflareNets) {
		if cf := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); cf != "" && net.ParseIP(cf) != nil {
			return cf
		}
	}

	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		hops := strings.Split(xff, ",")
		leftmost := ""
		for i := len(hops) - 1; i >= 0; i-- {
			hop := strings.TrimSpace(hops[i])
			ip := net.ParseIP(hop)
			if ip == nil {
				continue
			}
			leftmost = hop
			if !trustedProxy(ip) {
				return hop
			}
		}
		if leftmost != "" {
			return leftmost // every hop is trusted: the origin is the first one
		}
	}

	if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" && net.ParseIP(xri) != nil {
		return xri
	}
	return peer
}

// ---------------------------------------------------------------------------
// Proxy copy buffers

// proxyBufferPool recycles ReverseProxy copy buffers; without it every proxied
// response allocates a fresh 32 KB buffer.
type proxyBufferPool struct{ pool sync.Pool }

func (p *proxyBufferPool) Get() []byte {
	if b, ok := p.pool.Get().(*[]byte); ok {
		return *b
	}
	return make([]byte, 32*1024)
}

func (p *proxyBufferPool) Put(b []byte) {
	if cap(b) < 32*1024 {
		return
	}
	b = b[:32*1024]
	p.pool.Put(&b)
}

var sharedProxyBuffers = &proxyBufferPool{}

// ---------------------------------------------------------------------------
// Basic auth verification cache

// basicAuthCache remembers recent successful basic-auth checks. bcrypt is
// deliberately slow (tens of ms of CPU), and a browser re-sends the same
// credentials with every request, so verifying each time caps a basic-auth
// route at a few hundred requests per second per core. Entries are keyed by a
// hash of (route, user, password hash, password) — the password itself is
// never stored — and expire quickly, so a changed password or user takes
// effect within the TTL (and immediately when the stored hash changes).
type basicAuthCache struct {
	mu      sync.Mutex
	entries map[[32]byte]time.Time
}

const (
	basicAuthCacheTTL = time.Minute
	basicAuthCacheMax = 4096
)

var basicAuthOK = &basicAuthCache{entries: make(map[[32]byte]time.Time)}

func basicAuthKey(routeID, username, passwordHash, password string) [32]byte {
	h := sha256.New()
	for _, part := range []string{routeID, username, passwordHash, password} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	var key [32]byte
	copy(key[:], h.Sum(nil))
	return key
}

func (c *basicAuthCache) hit(key [32]byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	exp, ok := c.entries[key]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(c.entries, key)
		return false
	}
	return true
}

func (c *basicAuthCache) store(key [32]byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if len(c.entries) >= basicAuthCacheMax {
		for k, exp := range c.entries {
			if now.After(exp) {
				delete(c.entries, k)
			}
		}
		if len(c.entries) >= basicAuthCacheMax {
			c.entries = make(map[[32]byte]time.Time) // bounded: start over
		}
	}
	c.entries[key] = now.Add(basicAuthCacheTTL)
}

// ---------------------------------------------------------------------------
// Rate limiting

const rateLimitMinSweep = 10 * time.Second

func newRateLimiter(requests int, window time.Duration) *rateLimiter {
	return &rateLimiter{requests: requests, window: window}
}

// fnv32a hashes s without allocating (shard selection).
func fnv32a(s string) uint32 {
	h := uint32(2166136261)
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

// allow counts one request from clientIP and reports whether it is within
// the limit for the current window.
func (l *rateLimiter) allow(clientIP string) bool {
	sh := &l.shards[fnv32a(clientIP)%rateLimitShards]
	now := time.Now()

	sh.mu.Lock()
	defer sh.mu.Unlock()
	if sh.clients == nil {
		sh.clients = make(map[string]*clientRateLimit)
	}
	if now.After(sh.nextSweep) {
		for ip, c := range sh.clients {
			if now.After(c.windowEnd) {
				delete(sh.clients, ip)
			}
		}
		sh.nextSweep = now.Add(max(l.window, rateLimitMinSweep))
	}

	client, exists := sh.clients[clientIP]
	if !exists || now.After(client.windowEnd) {
		sh.clients[clientIP] = &clientRateLimit{requests: 1, windowEnd: now.Add(l.window)}
		return true
	}
	if client.requests >= l.requests {
		return false
	}
	client.requests++
	return true
}

// size reports how many clients are tracked (tests / diagnostics).
func (l *rateLimiter) size() int {
	n := 0
	for i := range l.shards {
		sh := &l.shards[i]
		sh.mu.Lock()
		n += len(sh.clients)
		sh.mu.Unlock()
	}
	return n
}
