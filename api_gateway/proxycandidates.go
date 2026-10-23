package api_gateway

import (
	"net"
	"net/http"
	"sort"
	"sync"
	"time"
)

// ProxyCandidate is a direct peer that sends forwarding headers but is not a
// trusted proxy. Usually it is a reverse proxy in front of the gateway that
// should be trusted (otherwise every request looks like it comes from it);
// occasionally it is a client trying to spoof its address. The dashboard
// lists these so the operator can trust the real ones with one click.
type ProxyCandidate struct {
	IP             string    `json:"ip"`
	Requests       int64     `json:"requests"`
	FirstSeen      time.Time `json:"first_seen"`
	LastSeen       time.Time `json:"last_seen"`
	ForwardedFor   string    `json:"forwarded_for"`   // last X-Forwarded-For (or X-Real-IP) value seen
	Host           string    `json:"host"`            // last Host header seen
	ForwardedProto string    `json:"forwarded_proto"` // last X-Forwarded-Proto, a strong proxy hint
}

const (
	proxyCandidateLimit    = 64
	proxyCandidateFieldMax = 200
)

type proxyCandidateTracker struct {
	mu      sync.Mutex
	entries map[string]*ProxyCandidate
}

var proxyCandidates = &proxyCandidateTracker{entries: make(map[string]*ProxyCandidate)}

func clip(s string) string {
	if len(s) > proxyCandidateFieldMax {
		return s[:proxyCandidateFieldMax]
	}
	return s
}

// noteProxyCandidate records the direct peer of r when it sends forwarding
// headers without being trusted. Requests without those headers (the vast
// majority) return after two header lookups.
func noteProxyCandidate(r *http.Request) {
	fwd := r.Header.Get("X-Forwarded-For")
	if fwd == "" {
		fwd = r.Header.Get("X-Real-IP")
	}
	if fwd == "" {
		return
	}
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		peer = r.RemoteAddr
	}
	ip := net.ParseIP(peer)
	if ip == nil || trustedProxy(ip) {
		return
	}
	proxyCandidates.note(ip.String(), clip(fwd), clip(r.Host), clip(r.Header.Get("X-Forwarded-Proto")))
}

func (t *proxyCandidateTracker) note(ip, fwd, host, proto string) {
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	c := t.entries[ip]
	if c == nil {
		if len(t.entries) >= proxyCandidateLimit {
			t.evictOldestLocked()
		}
		c = &ProxyCandidate{IP: ip, FirstSeen: now}
		t.entries[ip] = c
	}
	c.Requests++
	c.LastSeen = now
	c.ForwardedFor = fwd
	c.Host = host
	c.ForwardedProto = proto
}

func (t *proxyCandidateTracker) evictOldestLocked() {
	var oldest *ProxyCandidate
	for _, c := range t.entries {
		if oldest == nil || c.LastSeen.Before(oldest.LastSeen) {
			oldest = c
		}
	}
	if oldest != nil {
		delete(t.entries, oldest.IP)
	}
}

// list returns the candidates that are still untrusted (a peer trusted since
// it was seen drops out), busiest first.
func (t *proxyCandidateTracker) list() []ProxyCandidate {
	t.mu.Lock()
	out := make([]ProxyCandidate, 0, len(t.entries))
	for ip, c := range t.entries {
		if trustedProxy(net.ParseIP(ip)) {
			delete(t.entries, ip)
			continue
		}
		out = append(out, *c)
	}
	t.mu.Unlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Requests == out[j].Requests {
			return out[i].LastSeen.After(out[j].LastSeen)
		}
		return out[i].Requests > out[j].Requests
	})
	return out
}

// ProxyCandidates returns the untrusted peers seen sending forwarding headers.
func (g *Gateway) ProxyCandidates() []ProxyCandidate {
	return proxyCandidates.list()
}
