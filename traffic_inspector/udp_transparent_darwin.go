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

func (t *transparentUDPConn) ReadFrom(p []byte) (int, net.Addr, error) {
	n, addr, err := t.UDPConn.ReadFrom(p)
	if err != nil {
		return n, addr, err
	}

	udpAddr, ok := addr.(*net.UDPAddr)
	if !ok {
		return n, addr, nil
	}

	key := udpAddr.String()
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
	}

	return n, addr, nil
}

// OriginalDestinationFor returns the true pre-redirect destination for the
// given client address, if a pf natlook has resolved one for it yet.
func (t *transparentUDPConn) OriginalDestinationFor(addr net.Addr) (string, int, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	d, ok := t.origDst[addr.String()]
	if !ok {
		return "", 0, false
	}
	return d.host, d.port, true
}
