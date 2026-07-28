//go:build darwin

package traffic_inspector

import (
	"net"
	"sync"
)

// transparentUDPConn on macOS is a plain loopback UDP socket — pf's
// `rdr-to` rewrites both the destination IP and port for UDP the same way
// it does for TCP, so (unlike Linux TPROXY) there's no need for a
// transparent socket option; the original destination is instead recovered
// per client 5-tuple via DIOCNATLOOK (OriginalDestinationUDP in
// natlookup_darwin.go), cached here since QUIC connections send many
// packets and a pf ioctl per packet would be wasteful.
type transparentUDPConn struct {
	*net.UDPConn
	port int

	mu      sync.RWMutex
	origDst map[string]cachedDst
}

type cachedDst struct {
	host string
	port int
}

func newTransparentUDPConn(port int) (*transparentUDPConn, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		return nil, err
	}

	return &transparentUDPConn{UDPConn: conn, port: port, origDst: make(map[string]cachedDst)}, nil
}

// canonicalUDPAddrKey normalizes a net.Addr to a stable "ip:port" cache key.
// The address cached here (from this type's own raw ReadFrom, backed by
// net.UDPConn) and the address later looked up with (quic-go's own
// Conn.RemoteAddr(), which wraps/reconstructs its own net.Addr internally)
// are not guaranteed to produce byte-identical .String() output even for
// the same peer — e.g. an IPv4 address round-tripped through a
// netip.AddrPort-based representation can come back "as-4-in-6" or
// otherwise differently formatted. Re-deriving the key from the parsed
// IP/port rather than trusting the raw string avoids that mismatch.
func canonicalUDPAddrKey(addr net.Addr) string {
	host, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return addr.String()
	}
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	return net.JoinHostPort(ip.String(), port)
}

func (t *transparentUDPConn) ReadFrom(p []byte) (int, net.Addr, error) {
	n, addr, err := t.UDPConn.ReadFrom(p)
	if err != nil {
		return n, addr, err
	}

	udpAddr, ok := addr.(*net.UDPAddr)
	if !ok {
		return n, addr, nil
	}

	key := canonicalUDPAddrKey(udpAddr)
	t.mu.RLock()
	_, known := t.origDst[key]
	t.mu.RUnlock()
	if known {
		return n, addr, nil
	}

	host, port, lookupErr := OriginalDestinationUDP(udpAddr, t.port)
	if lookupErr == nil {
		t.mu.Lock()
		t.origDst[key] = cachedDst{host: host, port: port}
		t.mu.Unlock()
	} else {
		logWarn("traffic_inspector: QUIC natlookup failed for %s: %v", key, lookupErr)
	}

	return n, addr, nil
}

// OriginalDestinationFor returns the true pre-redirect destination for the
// given client address, if a pf natlook has resolved one for it yet.
func (t *transparentUDPConn) OriginalDestinationFor(addr net.Addr) (string, int, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	d, ok := t.origDst[canonicalUDPAddrKey(addr)]
	if !ok {
		return "", 0, false
	}
	return d.host, d.port, true
}
