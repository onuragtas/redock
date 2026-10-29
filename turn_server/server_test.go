package turn_server

import (
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pion/turn/v5"
)

// freePort returns a port free for both UDP and TCP on every interface — the
// server binds ":port", and on macOS a wildcard bind fails while any other
// socket uses the port on a specific address, so checking 127.0.0.1 alone is
// not enough.
func freePort(t *testing.T) int {
	t.Helper()
	for i := 0; i < 50; i++ {
		u, err := net.ListenPacket("udp4", ":0")
		if err != nil {
			continue
		}
		port := u.LocalAddr().(*net.UDPAddr).Port
		l, err := net.Listen("tcp4", ":"+strconv.Itoa(port))
		u.Close()
		if err == nil {
			l.Close()
			return port
		}
	}
	t.Fatal("no free port")
	return 0
}

const testSecret = "test-secret"

func newTestManager(t *testing.T, mutate func(*TURNConfigEntity)) (*Manager, string) {
	t.Helper()
	m := &Manager{guard: newGuard(), registry: newSessionRegistry(), events: &eventLog{}, denied: map[string]time.Time{}}
	m.apps = map[string]appInfo{"app1": {secret: testSecret, enabled: true}}

	// Another process can still grab a port between the check and the bind,
	// so pick fresh ports and retry a few times on "address already in use".
	var cfg TURNConfigEntity
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		port := freePort(t)
		relay := freePort(t)
		// The relay range must fit below 65535 (uint16) and stay clear of the main port.
		for relay > 65535-20 || (relay <= port && port <= relay+20) {
			relay = freePort(t)
		}
		cfg = TURNConfigEntity{
			Port: port, RelayIP: "127.0.0.1", RelayMin: uint16(relay), RelayMax: uint16(relay + 20), Realm: "redock",
			MaxSessionsPerIP: 5, AuthFailLimit: 3, BlockMinutes: 1,
		}
		if mutate != nil {
			mutate(&cfg)
		}
		m.mu.Lock()
		err = m.startLocked(cfg)
		m.mu.Unlock()
		if err == nil || !strings.Contains(err.Error(), "address already in use") {
			break
		}
	}
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = m.Stop() })
	return m, "127.0.0.1:" + strconv.Itoa(cfg.Port)
}

func newClient(t *testing.T, server, username, password string) *turn.Client {
	t.Helper()
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	c, err := turn.NewClient(&turn.ClientConfig{
		STUNServerAddr: server, TURNServerAddr: server, Conn: conn,
		Username: username, Password: password, Realm: "redock",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Listen(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close(); conn.Close() })
	return c
}

// udpPeer echoes every datagram back.
func udpPeer(t *testing.T) *net.UDPConn {
	t.Helper()
	pc, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	go func() {
		buf := make([]byte, 1500)
		for {
			n, addr, err := pc.ReadFromUDP(buf)
			if err != nil {
				return
			}
			_, _ = pc.WriteToUDP(buf[:n], addr)
		}
	}()
	return pc
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func hasEvent(m *Manager, kind string) bool {
	for _, e := range m.Events() {
		if e.Kind == kind {
			return true
		}
	}
	return false
}

func TestRelayWorksAndSessionCanBeKilled(t *testing.T) {
	m, server := newTestManager(t, func(c *TURNConfigEntity) { c.AllowPrivate = true })
	creds := GenerateCredentials("app1", testSecret, "alice", time.Hour)
	client := newClient(t, server, creds.Username, creds.Password)

	if _, err := client.SendBindingRequest(); err != nil {
		t.Fatalf("STUN binding: %v", err)
	}
	relay, err := client.Allocate()
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	peer := udpPeer(t)

	if _, err := relay.WriteTo([]byte("ping"), peer.LocalAddr()); err != nil {
		t.Fatalf("relay write: %v", err)
	}
	buf := make([]byte, 64)
	_ = relay.SetReadDeadline(time.Now().Add(3 * time.Second))
	n, _, err := relay.ReadFrom(buf)
	if err != nil || string(buf[:n]) != "ping" {
		t.Fatalf("echo through relay: %q, %v", buf[:n], err)
	}

	sessions := m.Sessions()
	if len(sessions) != 1 || sessions[0].App != "app1" || sessions[0].User != "alice" {
		t.Fatalf("sessions = %+v", sessions)
	}
	if sessions[0].BytesOut == 0 || sessions[0].BytesIn == 0 {
		t.Fatalf("bytes not counted: %+v", sessions[0])
	}
	if tr := m.Traffic(); tr.LiveIn != sessions[0].BytesIn || tr.TotalOut != sessions[0].BytesOut {
		t.Fatalf("live traffic = %+v, session = %+v", tr, sessions[0])
	}

	if err := m.KillSession(sessions[0].ID, false); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "session removal", func() bool { return len(m.Sessions()) == 0 })
	// "session-ended" is logged right after the registry drops the session.
	waitFor(t, "session-ended event", func() bool { return hasEvent(m, "session-ended") })
	// The closed session's bytes stay in the totals.
	if tr := m.Traffic(); tr.LiveIn != 0 || tr.TotalIn != sessions[0].BytesIn || tr.TotalOut != sessions[0].BytesOut {
		t.Fatalf("traffic after close = %+v, session = %+v", tr, sessions[0])
	}
	if !hasEvent(m, "session-killed") {
		t.Fatal("kill not recorded in the event log")
	}
	// The killed credential is revoked.
	again := newClient(t, server, creds.Username, creds.Password)
	if _, err := again.Allocate(); err == nil {
		t.Fatal("revoked credential could allocate again")
	}
}

func TestRelayToPrivateNetworkIsRefused(t *testing.T) {
	m, server := newTestManager(t, nil) // AllowPrivate off: 127.0.0.1 is off limits
	creds := GenerateCredentials("app1", testSecret, "", time.Hour)
	client := newClient(t, server, creds.Username, creds.Password)
	relay, err := client.Allocate()
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	peer := udpPeer(t)
	got := make(chan struct{}, 1)
	go func() {
		// The echo peer would answer; nothing must arrive through the relay.
		buf := make([]byte, 64)
		_ = relay.SetReadDeadline(time.Now().Add(time.Second))
		if _, _, err := relay.ReadFrom(buf); err == nil {
			got <- struct{}{}
		}
	}()
	_, _ = relay.WriteTo([]byte("ping"), peer.LocalAddr())
	select {
	case <-got:
		t.Fatal("relayed into a private address")
	case <-time.After(1200 * time.Millisecond):
	}
	waitFor(t, "relay-denied event", func() bool { return hasEvent(m, "relay-denied") })
}

func TestBadCredentialsGetIPBlocked(t *testing.T) {
	m, server := newTestManager(t, nil)
	good := GenerateCredentials("app1", testSecret, "", time.Hour)
	for i := 0; i < 3; i++ {
		c := newClient(t, server, good.Username, "wrong-password")
		if _, err := c.Allocate(); err == nil {
			t.Fatal("allocated with a wrong password")
		}
	}
	waitFor(t, "ip block", func() bool { return len(m.Blocked()) == 1 })
	// Now even correct credentials from this IP are ignored.
	c := newClient(t, server, good.Username, good.Password)
	if _, err := c.Allocate(); err == nil {
		t.Fatal("blocked IP could allocate")
	}
	m.Unblock("127.0.0.1")
	c = newClient(t, server, good.Username, good.Password)
	if _, err := c.Allocate(); err != nil {
		t.Fatalf("allocate after unblock: %v", err)
	}
}

func TestExpiredAndUnknownCredentialsRejected(t *testing.T) {
	_, server := newTestManager(t, func(c *TURNConfigEntity) { c.AuthFailLimit = 0 })
	expired := GenerateCredentials("app1", testSecret, "", -time.Minute)
	if _, err := newClient(t, server, expired.Username, expired.Password).Allocate(); err == nil {
		t.Fatal("expired credential accepted")
	}
	unknown := GenerateCredentials("nope", testSecret, "", time.Hour)
	if _, err := newClient(t, server, unknown.Username, unknown.Password).Allocate(); err == nil {
		t.Fatal("unknown app accepted")
	}
}

func TestRelayPolicyAndUsernames(t *testing.T) {
	allow, _ := parseCIDRs([]string{"10.1.2.3"})
	deny, _ := parseCIDRs([]string{"8.8.8.0/24"})
	p := &relayPolicy{allow: allow, deny: deny}
	cases := map[string]bool{
		"1.1.1.1": true, "8.8.8.8": false, "10.0.0.1": false, "10.1.2.3": true,
		"127.0.0.1": false, "192.168.1.5": false, "169.254.1.1": false, "100.64.0.1": false,
		"::1": false, "fd00::1": false, "2606:4700::1111": true, "::ffff:10.0.0.1": false, "::ffff:1.1.1.1": true,
	}
	for ip, want := range cases {
		if got := p.permits(net.ParseIP(ip)); got != want {
			t.Errorf("permits(%s) = %v, want %v", ip, got, want)
		}
	}

	c := GenerateCredentials("myapp", "s", "bob:x", time.Hour)
	parsed, err := parseUsername(c.Username)
	if err != nil || parsed.app != "myapp" || parsed.user != "bob:x" {
		t.Fatalf("parse %q = %+v, %v", c.Username, parsed, err)
	}
	for _, bad := range []string{"", "123", "abc:app", "123:"} {
		if _, err := parseUsername(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	if !strings.HasPrefix(c.Username, strconv.FormatInt(c.ExpiresAt.Unix(), 10)+":") {
		t.Error("username must start with the expiry")
	}
}
