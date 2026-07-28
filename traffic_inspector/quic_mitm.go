package traffic_inspector

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"strconv"
	"time"

	"github.com/quic-go/quic-go"
)

// udpProxyPortBase + serverID gives a deterministic loopback UDP port for
// each VPN server's QUIC MITM listener, mirroring tcpProxyPortBase.
const udpProxyPortBase = 21000

// quicInterceptor is the per-VPN-server QUIC MITM listener.
type quicInterceptor struct {
	serverID     uint
	tunIface     string
	udpProxyPort int
	listener     *quic.Listener
	packetConn   *transparentUDPConn
}

func (m *Manager) startQUICInterceptor(serverID uint, tunIface string) (*quicInterceptor, error) {
	port := udpProxyPortBase + int(serverID)

	pc, err := newTransparentUDPConn(port)
	if err != nil {
		return nil, fmt.Errorf("transparent UDP listen on %d: %w", port, err)
	}

	tlsConf := &tls.Config{
		GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
			cert, err := m.CA.LeafCertificate(hello.ServerName)
			if err != nil {
				return nil, err
			}
			return &tls.Config{
				Certificates: []tls.Certificate{*cert},
				NextProtos:   hello.SupportedProtos,
			}, nil
		},
	}

	ln, err := quic.Listen(pc, tlsConf, &quic.Config{MaxIdleTimeout: 30 * time.Second})
	if err != nil {
		pc.Close()
		return nil, fmt.Errorf("quic listen: %w", err)
	}

	qi := &quicInterceptor{
		serverID:     serverID,
		tunIface:     tunIface,
		udpProxyPort: port,
		listener:     ln,
		packetConn:   pc,
	}

	go qi.acceptLoop(m)

	log.Printf("Traffic Inspector: QUIC MITM listener for VPN server %d on UDP 127.0.0.1:%d (tun=%s)", serverID, port, tunIface)
	return qi, nil
}

func (qi *quicInterceptor) stop() {
	qi.listener.Close()
	qi.packetConn.Close()
}

func (qi *quicInterceptor) acceptLoop(m *Manager) {
	ctx := context.Background()
	for {
		conn, err := qi.listener.Accept(ctx)
		if err != nil {
			return // listener closed (server stopped)
		}
		go handleQUICConn(m, qi, conn)
	}
}

// handleQUICConn terminates the client's QUIC/TLS1.3 handshake (via the
// dynamically-issued leaf cert from startQUICInterceptor's
// GetConfigForClient), recovers the true destination for this client's
// 5-tuple, opens an independent QUIC connection to it mirroring the
// negotiated ALPN, and relays every client-opened stream to a matching
// upstream stream.
func handleQUICConn(m *Manager, qi *quicInterceptor, conn *quic.Conn) {
	defer conn.CloseWithError(0, "")

	remoteAddr := conn.RemoteAddr()

	host, port, ok := qi.packetConn.OriginalDestinationFor(remoteAddr)
	if !ok {
		logWarn("traffic_inspector: no original destination known for QUIC client %s", remoteAddr)
		return
	}

	state := conn.ConnectionState()
	sni := state.TLS.ServerName
	alpn := state.TLS.NegotiatedProtocol

	var remoteIP string
	if udpAddr, ok := remoteAddr.(*net.UDPAddr); ok {
		remoteIP = udpAddr.IP.String()
	}
	var userID uint
	if user := m.lookupUserByIP(remoteIP); user != nil {
		userID = user.ID
	} else {
		logWarn("traffic_inspector: no VPNUser matched source IP %s (flow will be tagged with user_id=0)", remoteIP)
	}

	upstreamName := sni
	if upstreamName == "" {
		upstreamName = host
	}

	upstreamTLSConf := &tls.Config{ServerName: upstreamName}
	if alpn != "" {
		upstreamTLSConf.NextProtos = []string{alpn}
	}

	dialCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	upstreamConn, err := quic.DialAddr(dialCtx, net.JoinHostPort(host, strconv.Itoa(port)), upstreamTLSConf, nil)
	cancel()
	if err != nil {
		logWarn("traffic_inspector: QUIC dial upstream %s:%d failed (sni=%q, alpn=%q): %v", host, port, sni, alpn, err)
		return
	}
	defer upstreamConn.CloseWithError(0, "")

	for {
		clientStream, err := conn.AcceptStream(context.Background())
		if err != nil {
			return
		}
		go relayQUICStream(m, upstreamConn, clientStream, userID, sni, host, port)
	}
}

func relayQUICStream(m *Manager, upstreamConn *quic.Conn, clientStream *quic.Stream, userID uint, sni, host string, port int) {
	upstreamStream, err := upstreamConn.OpenStreamSync(context.Background())
	if err != nil {
		logWarn("traffic_inspector: failed to open upstream QUIC stream for %s:%d (sni=%q): %v", host, port, sni, err)
		clientStream.CancelRead(0)
		clientStream.CancelWrite(0)
		return
	}

	// relay (tcp_mitm.go) is generic over io.ReadWriteCloser, which
	// *quic.Stream already satisfies — same tap/forward logic as the TCP
	// path, just over QUIC streams instead of a TLS-wrapped TCP conn.
	relay(m, "quic", sni, host, port, userID, clientStream, upstreamStream)
}
