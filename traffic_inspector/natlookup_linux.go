//go:build linux

package traffic_inspector

import (
	"fmt"
	"net"
	"syscall"
	"unsafe"
)

// soOriginalDst is SO_ORIGINAL_DST from linux/netfilter_ipv4.h — recovers
// the pre-REDIRECT destination of a connection that arrived via an iptables
// `-j REDIRECT` rule (the standard technique used by transparent proxies on
// Linux, e.g. redsocks/shadowsocks-go).
const soOriginalDst = 80

// OriginalDestination recovers the true destination address/port of a
// transparently-redirected TCP connection, before the iptables REDIRECT
// rule rewrote it to our local proxy port.
func OriginalDestination(conn net.Conn) (string, int, error) {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return "", 0, fmt.Errorf("not a TCP connection")
	}

	sysConn, err := tcpConn.SyscallConn()
	if err != nil {
		return "", 0, err
	}

	var addr syscall.RawSockaddrInet4
	size := uint32(syscall.SizeofSockaddrInet4)
	var sockErr error

	ctrlErr := sysConn.Control(func(fd uintptr) {
		_, _, errno := syscall.Syscall6(
			syscall.SYS_GETSOCKOPT,
			fd,
			uintptr(syscall.IPPROTO_IP),
			uintptr(soOriginalDst),
			uintptr(unsafe.Pointer(&addr)),
			uintptr(unsafe.Pointer(&size)),
			0,
		)
		if errno != 0 {
			sockErr = errno
		}
	})
	if ctrlErr != nil {
		return "", 0, ctrlErr
	}
	if sockErr != nil {
		return "", 0, fmt.Errorf("SO_ORIGINAL_DST: %w", sockErr)
	}

	ip := net.IPv4(addr.Addr[0], addr.Addr[1], addr.Addr[2], addr.Addr[3])
	// addr.Port is stored in network byte order (big-endian) by the kernel
	// but read back as a host-endian uint16 — swap it back (ntohs).
	port := int(addr.Port&0xff)<<8 | int(addr.Port>>8)
	return ip.String(), port, nil
}
