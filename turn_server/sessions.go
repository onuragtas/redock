package turn_server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/turn/v5"
	"golang.org/x/time/rate"
)

// relayGenerator hands out relay sockets from the configured port range and
// wraps each one so the server can count its bytes, cap its bitrate and
// close it (which ends the allocation) when a session is killed.
type relayGenerator struct {
	inner    *turn.RelayAddressGeneratorPortRange
	registry *sessionRegistry
	kbps     int
	local    localRelays
}

func (r *relayGenerator) Validate() error { return r.inner.Validate() }

func (r *relayGenerator) AllocatePacketConn(conf turn.AllocateListenerConfig) (net.PacketConn, net.Addr, error) {
	conn, addr, err := r.inner.AllocatePacketConn(conf)
	if err != nil {
		return nil, nil, err
	}
	rc := newRelayConn(conn, r.kbps)
	if pub, ok := addr.(*net.UDPAddr); ok {
		if lp, ok := conn.LocalAddr().(*net.UDPAddr); ok {
			rc.local, rc.port = &r.local, lp.Port
			r.local.add(pub, lp.Port)
		}
	}
	r.registry.addRelay(conn.LocalAddr().String(), rc)
	return rc, addr, nil
}

// TCP relaying (RFC 6062) is not offered: WebRTC uses UDP relays, and every
// extra relay type is extra attack surface.
var errTCPRelayDisabled = errors.New("TCP relay allocations are disabled")

func (r *relayGenerator) AllocateListener(turn.AllocateListenerConfig) (net.Listener, net.Addr, error) {
	return nil, nil, errTCPRelayDisabled
}

func (r *relayGenerator) AllocateConn(turn.AllocateConnConfig) (net.Conn, error) {
	return nil, errTCPRelayDisabled
}

// localRelays maps the advertised addresses of this server's relays to
// their sockets' ports. Two clients that both force relay-only ICE send
// media from one relay to another on the same server; sent to the public
// address, those packets leave the host and, behind NAT, come back (if at
// all) from an internal source the peer's permission doesn't match. So
// relay-to-relay packets take the loopback instead, and the receiving relay
// reports them as coming from the sender's public address.
type localRelays struct {
	mu       sync.RWMutex
	byPublic map[string]int       // advertised "ip:port" -> socket port
	byPort   map[int]*net.UDPAddr // socket port -> advertised address
}

var loopback = net.IPv4(127, 0, 0, 1)

func (l *localRelays) add(public *net.UDPAddr, port int) {
	l.mu.Lock()
	if l.byPublic == nil {
		l.byPublic, l.byPort = map[string]int{}, map[int]*net.UDPAddr{}
	}
	l.byPublic[public.String()] = port
	l.byPort[port] = public
	l.mu.Unlock()
}

func (l *localRelays) remove(port int) {
	l.mu.Lock()
	if pub := l.byPort[port]; pub != nil {
		delete(l.byPublic, pub.String())
		delete(l.byPort, port)
	}
	l.mu.Unlock()
}

// toLocal returns the loopback address of the relay advertised as addr.
func (l *localRelays) toLocal(addr net.Addr) (net.Addr, bool) {
	l.mu.RLock()
	port, ok := l.byPublic[addr.String()]
	l.mu.RUnlock()
	if !ok {
		return addr, false
	}
	return &net.UDPAddr{IP: loopback, Port: port}, true
}

// fromLocal maps a loopback source that is one of our relays to that
// relay's public address. Only our own relay sockets hold those ports, so
// nothing else on the host can pose as a relay.
func (l *localRelays) fromLocal(addr net.Addr) net.Addr {
	u, ok := addr.(*net.UDPAddr)
	if !ok || !u.IP.IsLoopback() {
		return addr
	}
	l.mu.RLock()
	pub := l.byPort[u.Port]
	l.mu.RUnlock()
	if pub == nil {
		return addr
	}
	return pub
}

// relayConn is a relay socket with byte counters and an optional rate cap
// applied to each direction.
type relayConn struct {
	net.PacketConn
	in, out  atomic.Int64 // bytes from peers / to peers
	inLimit  *rate.Limiter
	outLimit *rate.Limiter
	closed   atomic.Bool

	local *localRelays // this server's relays, for relay-to-relay traffic
	port  int          // this socket's port in local
}

func newRelayConn(c net.PacketConn, kbps int) *relayConn {
	rc := &relayConn{PacketConn: c}
	if kbps > 0 {
		bytesPerSec := rate.Limit(kbps * 1000 / 8)
		burst := max(kbps*1000/8, 64*1024) // a burst must fit any single datagram
		rc.inLimit = rate.NewLimiter(bytesPerSec, burst)
		rc.outLimit = rate.NewLimiter(bytesPerSec, burst)
	}
	return rc
}

func (c *relayConn) ReadFrom(p []byte) (int, net.Addr, error) {
	n, addr, err := c.PacketConn.ReadFrom(p)
	if addr != nil && c.local != nil {
		addr = c.local.fromLocal(addr)
	}
	if n > 0 {
		c.in.Add(int64(n))
		if c.inLimit != nil {
			_ = c.inLimit.WaitN(context.Background(), n)
		}
	}
	return n, addr, err
}

func (c *relayConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	if c.outLimit != nil && !c.outLimit.AllowN(time.Now(), len(p)) {
		return len(p), nil // over the cap: drop, as a congested link would
	}
	if c.local != nil {
		addr, _ = c.local.toLocal(addr)
	}
	n, err := c.PacketConn.WriteTo(p, addr)
	c.out.Add(int64(n))
	return n, err
}

func (c *relayConn) Close() error {
	c.closed.Store(true)
	if c.local != nil {
		c.local.remove(c.port)
	}
	return c.PacketConn.Close()
}

// Session is one TURN allocation as shown in the dashboard.
type Session struct {
	ID         string    `json:"id"` // relay address
	App        string    `json:"app"`
	User       string    `json:"user"`
	Username   string    `json:"-"`
	ClientAddr string    `json:"client_addr"`
	Protocol   string    `json:"protocol"`
	RelayAddr  string    `json:"relay_addr"`
	Peers      []string  `json:"peers"`
	StartedAt  time.Time `json:"started_at"`
	BytesIn    int64     `json:"bytes_in"`
	BytesOut   int64     `json:"bytes_out"`

	tuple    string // protocol|client|server: TURN's identity for the allocation
	conn     *relayConn
	peers    map[string]bool
	deadline *time.Timer
}

// sessionRegistry links relay sockets to the allocations TURN reports.
type sessionRegistry struct {
	mu       sync.Mutex
	relays   map[string]*relayConn // relay local addr -> conn, until the allocation event
	sessions map[string]*Session   // relay addr -> session
	byIP     map[string]int        // client IP -> active sessions
	byApp    map[string]int        // app -> active sessions
}

func newSessionRegistry() *sessionRegistry {
	return &sessionRegistry{
		relays:   make(map[string]*relayConn),
		sessions: make(map[string]*Session),
		byIP:     make(map[string]int),
		byApp:    make(map[string]int),
	}
}

func (r *sessionRegistry) addRelay(localAddr string, c *relayConn) {
	r.mu.Lock()
	r.relays[localAddr] = c
	r.mu.Unlock()
}

// relayKey matches the relay address TURN reports (advertised IP) with the
// socket we created (bound address) by port, which is unique per relay.
func relayKey(addr net.Addr) string {
	_, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return port
}

func (r *sessionRegistry) created(s *Session, relay net.Addr) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := relayKey(relay)
	for local, c := range r.relays {
		if relayKey(stringAddr(local)) == key {
			s.conn = c
			delete(r.relays, local)
			break
		}
	}
	s.ID = key
	s.peers = map[string]bool{}
	r.sessions[key] = s
	r.byIP[hostOf(stringAddr(s.ClientAddr))]++
	r.byApp[s.App]++
}

func fiveTuple(protocol string, src, dst net.Addr) string {
	return protocol + "|" + src.String() + "|" + dst.String()
}

func (r *sessionRegistry) deleted(tuple string) *Session {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, s := range r.sessions {
		if s.tuple != tuple {
			continue
		}
		delete(r.sessions, key)
		if s.deadline != nil {
			s.deadline.Stop()
		}
		ip := hostOf(stringAddr(s.ClientAddr))
		if r.byIP[ip]--; r.byIP[ip] <= 0 {
			delete(r.byIP, ip)
		}
		if r.byApp[s.App]--; r.byApp[s.App] <= 0 {
			delete(r.byApp, s.App)
		}
		return s
	}
	return nil
}

func (r *sessionRegistry) peerAdded(relay net.Addr, peer net.IP) {
	r.mu.Lock()
	if s := r.sessions[relayKey(relay)]; s != nil {
		s.peers[peer.String()] = true
	}
	r.mu.Unlock()
}

// totals sums the bytes of the open sessions.
func (r *sessionRegistry) totals() (in, out int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.sessions {
		if s.conn != nil {
			in += s.conn.in.Load()
			out += s.conn.out.Load()
		}
	}
	return in, out
}

func (r *sessionRegistry) counts(ip, app string) (int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.byIP[ip], r.byApp[app]
}

func (r *sessionRegistry) list() []Session {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Session, 0, len(r.sessions))
	for _, s := range r.sessions {
		cp := *s
		cp.Peers = make([]string, 0, len(s.peers))
		for p := range s.peers {
			cp.Peers = append(cp.Peers, p)
		}
		sort.Strings(cp.Peers)
		if s.conn != nil {
			cp.BytesIn, cp.BytesOut = s.conn.in.Load(), s.conn.out.Load()
		}
		cp.conn, cp.peers, cp.deadline = nil, nil, nil
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.Before(out[j].StartedAt) })
	return out
}

// kill closes the session's relay socket; TURN then deletes the allocation.
func (r *sessionRegistry) kill(id string) (*Session, error) {
	r.mu.Lock()
	s := r.sessions[id]
	r.mu.Unlock()
	if s == nil {
		return nil, fmt.Errorf("session %s not found", id)
	}
	if s.conn == nil {
		return nil, fmt.Errorf("session %s has no relay socket", id)
	}
	_ = s.conn.Close()
	return s, nil
}

func (r *sessionRegistry) closeAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.sessions {
		if s.conn != nil {
			_ = s.conn.Close()
		}
		if s.deadline != nil {
			s.deadline.Stop()
		}
	}
	for _, c := range r.relays {
		_ = c.Close()
	}
	r.relays = make(map[string]*relayConn)
	r.sessions = make(map[string]*Session)
	r.byIP = make(map[string]int)
	r.byApp = make(map[string]int)
}

// stringAddr adapts an "ip:port" string to net.Addr for hostOf/relayKey.
type stringAddr string

func (a stringAddr) Network() string { return "udp" }
func (a stringAddr) String() string  { return string(a) }

// ---------------------------------------------------------------------------
// Event log

// Event is one metadata record: sessions, auth failures, blocks, kills.
// Media content is never visible to TURN (it is end-to-end encrypted).
type Event struct {
	Time   time.Time `json:"time"`
	Kind   string    `json:"kind"`
	App    string    `json:"app,omitempty"`
	User   string    `json:"user,omitempty"`
	Client string    `json:"client,omitempty"`
	Detail string    `json:"detail,omitempty"`
}

const eventLogSize = 500

type eventLog struct {
	mu     sync.Mutex
	events []Event
}

func (l *eventLog) add(e Event) {
	e.Time = time.Now()
	l.mu.Lock()
	l.events = append(l.events, e)
	if len(l.events) > eventLogSize {
		l.events = l.events[len(l.events)-eventLogSize:]
	}
	l.mu.Unlock()
}

// recent returns events newest first.
func (l *eventLog) recent() []Event {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Event, len(l.events))
	for i, e := range l.events {
		out[len(l.events)-1-i] = e
	}
	return out
}
