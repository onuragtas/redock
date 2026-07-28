//go:build linux

package traffic_inspector

import (
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

// transparentUDPConn is a net.PacketConn bound with IP_TRANSPARENT so it can
// receive UDP datagrams redirected via an iptables TPROXY rule (mangle
// table), recovering each datagram's true pre-redirect destination via the
// IP_ORIGDSTADDR control message — unlike `-j REDIRECT`, TPROXY never
// rewrites the packet's own headers, it only changes local delivery, so the
// original destination is recoverable per-packet without any extra syscall
// beyond the recvmsg that already happens.
//
// The TPROXY iptables/policy-routing setup (mangle PREROUTING + `ip rule`/
// `ip route` for fwmark-based local delivery) is a separate one-time system
// config applied when the first QUIC interceptor starts — see
// redirect_linux.go's UDP counterpart.
type transparentUDPConn struct {
	fd int

	mu      sync.RWMutex
	origDst map[string]string // client addr string -> "ip:port" original destination
}

func newTransparentUDPConn(port int) (*transparentUDPConn, error) {
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM, 0)
	if err != nil {
		return nil, fmt.Errorf("socket: %w", err)
	}

	if err := unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEADDR, 1); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("SO_REUSEADDR: %w", err)
	}
	if err := unix.SetsockoptInt(fd, unix.SOL_IP, unix.IP_TRANSPARENT, 1); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("IP_TRANSPARENT: %w", err)
	}
	if err := unix.SetsockoptInt(fd, unix.SOL_IP, unix.IP_RECVORIGDSTADDR, 1); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("IP_RECVORIGDSTADDR: %w", err)
	}

	addr := &unix.SockaddrInet4{Port: port}
	if err := unix.Bind(fd, addr); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("bind :%d: %w", port, err)
	}

	return &transparentUDPConn{fd: fd, origDst: make(map[string]string)}, nil
}

func (t *transparentUDPConn) ReadFrom(p []byte) (int, net.Addr, error) {
	oob := make([]byte, 128)

	n, oobn, _, from, err := unix.Recvmsg(t.fd, p, oob, 0)
	if err != nil {
		return n, nil, err
	}

	srcAddr := &net.UDPAddr{}
	if sa, ok := from.(*unix.SockaddrInet4); ok {
		srcAddr = &net.UDPAddr{IP: append(net.IP{}, sa.Addr[:]...), Port: sa.Port}
	}

	if oobn > 0 {
		if cmsgs, err := unix.ParseSocketControlMessage(oob[:oobn]); err == nil {
			for i := range cmsgs {
				sa, err := unix.ParseOrigDstAddr(&cmsgs[i])
				if err != nil {
					continue
				}
				if dst, ok := sa.(*unix.SockaddrInet4); ok {
					ip := net.IP(dst.Addr[:])
					t.mu.Lock()
					t.origDst[srcAddr.String()] = fmt.Sprintf("%s:%d", ip.String(), dst.Port)
					t.mu.Unlock()
				}
			}
		}
	}

	return n, srcAddr, nil
}

func (t *transparentUDPConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	udpAddr, ok := addr.(*net.UDPAddr)
	if !ok {
		return 0, fmt.Errorf("transparentUDPConn.WriteTo: not a UDP address: %T", addr)
	}

	sa := &unix.SockaddrInet4{Port: udpAddr.Port}
	copy(sa.Addr[:], udpAddr.IP.To4())

	if err := unix.Sendto(t.fd, p, 0, sa); err != nil {
		return 0, err
	}
	return len(p), nil
}

// OriginalDestinationFor returns the true pre-redirect destination for the
// given client address, if a datagram carrying an IP_ORIGDSTADDR control
// message has been seen from it yet.
func (t *transparentUDPConn) OriginalDestinationFor(addr net.Addr) (string, int, bool) {
	t.mu.RLock()
	v, ok := t.origDst[addr.String()]
	t.mu.RUnlock()
	if !ok {
		return "", 0, false
	}

	host, portStr, err := net.SplitHostPort(v)
	if err != nil {
		return "", 0, false
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, false
	}
	return host, port, true
}

func (t *transparentUDPConn) Close() error { return unix.Close(t.fd) }

func (t *transparentUDPConn) LocalAddr() net.Addr { return &net.UDPAddr{} }

func (t *transparentUDPConn) SetDeadline(time.Time) error      { return nil }
func (t *transparentUDPConn) SetReadDeadline(time.Time) error  { return nil }
func (t *transparentUDPConn) SetWriteDeadline(time.Time) error { return nil }
