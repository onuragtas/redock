package api_gateway

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

const udpBufferSize = 64 * 1024
const udpSessionIdleTimeout = 2 * time.Minute

// runUDPRoute listens on route.ListenPort (UDP) and forwards packets to the backend service.
// Client address is preserved: responses from backend are sent back to the originating client.
func (g *Gateway) runUDPRoute(route UDPRoute, stopChan <-chan struct{}) {
	if !route.Enabled {
		return
	}
	g.mu.RLock()
	svc, ok := g.services[route.ServiceID]
	g.mu.RUnlock()
	if !ok || svc == nil {
		log.Printf("API Gateway UDP: route %s: service %s not found", route.ID, route.ServiceID)
		return
	}
	backendAddr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(svc.Host, fmtPort(svc.Port)))
	if err != nil {
		log.Printf("API Gateway UDP: route %s: invalid backend %s:%d: %v", route.ID, svc.Host, svc.Port, err)
		return
	}

	listener, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: route.ListenPort})
	if err != nil {
		log.Printf("API Gateway UDP: route %s: failed to listen on port %d: %v", route.ID, route.ListenPort, err)
		return
	}
	defer listener.Close()

	// Close listener when stop is requested so ReadFromUDP unblocks
	go func() {
		<-stopChan
		listener.Close()
	}()

	log.Printf("API Gateway UDP: route %s listening on 0.0.0.0:%d -> %s", route.ID, route.ListenPort, backendAddr)

	type udpSession struct {
		conn       *net.UDPConn
		clientAddr *net.UDPAddr
	}
	var sessionsMu sync.Mutex
	sessions := make(map[string]*udpSession)

	readBuf := make([]byte, udpBufferSize)
	for {
		n, clientAddr, err := listener.ReadFromUDP(readBuf)
		if err != nil {
			if netErr, ok := err.(*net.OpError); ok && netErr.Err != nil {
				if netErr.Err.Error() == "use of closed network connection" {
					return
				}
			}
			log.Printf("API Gateway UDP: route %s read error: %v", route.ID, err)
			continue
		}
		if n == 0 {
			continue
		}
		packet := make([]byte, n)
		copy(packet, readBuf[:n])

		key := clientAddr.String()
		sessionsMu.Lock()
		sess, exists := sessions[key]
		if !exists {
			backendConn, err := net.DialUDP("udp", nil, backendAddr)
			if err != nil {
				sessionsMu.Unlock()
				log.Printf("API Gateway UDP: route %s: dial backend: %v", route.ID, err)
				continue
			}
			sess = &udpSession{conn: backendConn, clientAddr: clientAddr}
			sessions[key] = sess
			// Goroutine: read from backend, forward to client
			go func(s *udpSession, k string) {
				defer func() {
					s.conn.Close()
					sessionsMu.Lock()
					delete(sessions, k)
					sessionsMu.Unlock()
				}()
				buf := make([]byte, udpBufferSize)
				for {
					s.conn.SetReadDeadline(time.Now().Add(udpSessionIdleTimeout))
					m, err := s.conn.Read(buf)
					if err != nil {
						return
					}
					if m > 0 {
						if _, err := listener.WriteToUDP(buf[:m], s.clientAddr); err != nil {
							return
						}
					}
				}
			}(sess, key)
		}
		sessionsMu.Unlock()

		if _, err := sess.conn.Write(packet); err != nil {
			log.Printf("API Gateway UDP: route %s write to backend: %v", route.ID, err)
		}
	}
}

func fmtPort(port int) string {
	return fmt.Sprintf("%d", port)
}
