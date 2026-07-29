package traffic_inspector

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync/atomic"
	"time"
)

var flowCounter uint64

func nextFlowID() string {
	return fmt.Sprintf("f%d", atomic.AddUint64(&flowCounter, 1))
}

// peekConn lets us inspect a connection's first bytes (to distinguish TLS
// from plaintext) without consuming them — all reads (ours and the TLS
// handshake's) go through the same buffered reader.
type peekConn struct {
	net.Conn
	r *bufio.Reader
}

func newPeekConn(c net.Conn) *peekConn {
	return &peekConn{Conn: c, r: bufio.NewReader(c)}
}

func (p *peekConn) Read(b []byte) (int, error) {
	return p.r.Read(b)
}

// handleInterceptedConn is the entry point for every connection accepted on
// a server's TCP MITM listener (i.e. every connection from an
// inspection-enabled peer that was redirected here). It recovers the true
// destination, detects TLS vs plaintext, and dispatches accordingly.
func handleInterceptedConn(m *Manager, rawConn net.Conn) {
	defer rawConn.Close()

	host, port, err := OriginalDestination(rawConn)
	if err != nil {
		logWarn("traffic_inspector: original destination lookup failed for %s: %v", rawConn.RemoteAddr(), err)
		return
	}

	var remoteIP string
	if tcpAddr, ok := rawConn.RemoteAddr().(*net.TCPAddr); ok {
		remoteIP = tcpAddr.IP.String()
	}

	var userID uint
	if user := m.lookupUserByIP(remoteIP); user != nil {
		userID = user.ID
	} else {
		logWarn("traffic_inspector: no VPNUser matched source IP %s (flow will be tagged with user_id=0)", remoteIP)
	}

	pc := newPeekConn(rawConn)

	peeked, err := pc.r.Peek(3)
	isTLS := err == nil && len(peeked) == 3 && peeked[0] == 0x16 && peeked[1] == 0x03

	if isTLS {
		// Peek enough of the ClientHello to recover the SNI *before* we try to
		// terminate the handshake, so a connection the client refuses (cert
		// pinning) can still be reported to the dashboard with its target host.
		sni := ""
		if hello, perr := pc.r.Peek(2048); perr == nil || len(hello) > 0 {
			sni = parseTLSClientHelloSNI(hello)
		}
		handleTLSConn(m, pc, userID, sni, host, port)
		return
	}

	handlePlainConn(m, pc, userID, host, port)
}

// handleTLSConn terminates the client's TLS handshake using a leaf
// certificate issued on the fly for the requested SNI (signed by our root
// CA), opens an independent TLS connection to the real destination, and
// relays decrypted application data between the two, tapping every chunk
// into the capture hub.
func handleTLSConn(m *Manager, pc *peekConn, userID uint, sni, host string, port int) {
	tlsConn := tls.Server(pc, &tls.Config{
		GetCertificate: m.CA.GetCertificateFunc(),
		MinVersion:     tls.VersionTLS12,
	})
	defer tlsConn.Close()

	if err := tlsConn.Handshake(); err != nil {
		logWarn("traffic_inspector: client TLS handshake failed for %s:%d: %v", host, port, err)
		// The client refused our injected certificate — almost always cert
		// pinning. Surface it in the dashboard with the SNI we peeked upfront.
		m.publishConnEvent("tcp-tls", sni, host, port, userID, "blocked", err.Error())
		return
	}
	if cs := tlsConn.ConnectionState().ServerName; cs != "" {
		sni = cs
	}

	upstreamName := sni
	if upstreamName == "" {
		upstreamName = host
	}

	upstreamRaw, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), 10*time.Second)
	if err != nil {
		logWarn("traffic_inspector: dial upstream %s:%d failed: %v", host, port, err)
		m.publishConnEvent("tcp-tls", sni, host, port, userID, "failed", err.Error())
		return
	}
	upstreamConn := tls.Client(upstreamRaw, &tls.Config{ServerName: upstreamName})
	defer upstreamConn.Close()

	if err := upstreamConn.Handshake(); err != nil {
		logWarn("traffic_inspector: upstream TLS handshake failed for %s:%d (sni=%s): %v", host, port, sni, err)
		m.publishConnEvent("tcp-tls", sni, host, port, userID, "failed", err.Error())
		return
	}
	relay(m, "tcp-tls", sni, host, port, userID, tlsConn, upstreamConn)
}

// handlePlainConn relays already-plaintext TCP traffic directly, no MITM
// needed — just tap and forward.
func handlePlainConn(m *Manager, pc *peekConn, userID uint, host string, port int) {
	upstream, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), 10*time.Second)
	if err != nil {
		logWarn("traffic_inspector: dial upstream %s:%d failed: %v", host, port, err)
		m.publishConnEvent("tcp-plain", "", host, port, userID, "failed", err.Error())
		return
	}
	defer upstream.Close()

	relay(m, "tcp-plain", "", host, port, userID, pc, upstream)
}

// relay pumps bytes in both directions between an already-decrypted client
// leg and server leg, publishing every chunk to the capture hub before
// forwarding it onward. Blocks until both directions have closed.
func relay(m *Manager, protocol, sni, host string, port int, userID uint, client, upstream io.ReadWriteCloser) {
	flowID := nextFlowID()
	done := make(chan struct{}, 2)

	pump := func(dst io.Writer, src io.Reader, direction string) {
		defer func() { done <- struct{}{} }()

		buf := make([]byte, 32*1024)
		for {
			n, readErr := src.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])

				m.Hub.Publish(FlowEvent{
					Timestamp: time.Now().Unix(),
					UserID:    userID,
					FlowID:    flowID,
					Protocol:  protocol,
					SNI:       sni,
					Host:      host,
					Port:      port,
					Direction: direction,
					Data:      chunk,
				})

				if _, writeErr := dst.Write(chunk); writeErr != nil {
					return
				}
			}
			if readErr != nil {
				return
			}
		}
	}

	go pump(upstream, client, "client->server")
	go pump(client, upstream, "server->client")

	<-done
	<-done

	m.Hub.Publish(FlowEvent{
		Timestamp: time.Now().Unix(),
		UserID:    userID,
		FlowID:    flowID,
		Protocol:  protocol,
		SNI:       sni,
		Host:      host,
		Port:      port,
		Closed:    true,
	})
}
