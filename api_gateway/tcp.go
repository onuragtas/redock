package api_gateway

import (
	"fmt"
	"io"
	"log"
	"net"
	"time"
)

// runTCPRoute listens on route.ListenPort (TCP) and forwards raw TCP to the backend service.
func (g *Gateway) runTCPRoute(route TCPRoute, stopChan <-chan struct{}) {
	if !route.Enabled {
		return
	}
	g.mu.RLock()
	svc, ok := g.services[route.ServiceID]
	g.mu.RUnlock()
	if !ok || svc == nil {
		log.Printf("API Gateway TCP: route %s: service %s not found", route.ID, route.ServiceID)
		return
	}
	backendAddr := net.JoinHostPort(svc.Host, fmt.Sprintf("%d", svc.Port))

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", route.ListenPort))
	if err != nil {
		log.Printf("API Gateway TCP: route %s: failed to listen on port %d: %v", route.ID, route.ListenPort, err)
		return
	}
	defer listener.Close()

	go func() {
		<-stopChan
		listener.Close()
	}()

	log.Printf("API Gateway TCP: route %s listening on 0.0.0.0:%d -> %s", route.ID, route.ListenPort, backendAddr)

	for {
		clientConn, err := listener.Accept()
		if err != nil {
			if netErr, ok := err.(*net.OpError); ok && netErr.Err != nil {
				if netErr.Err.Error() == "use of closed network connection" {
					return
				}
			}
			log.Printf("API Gateway TCP: route %s accept: %v", route.ID, err)
			continue
		}
		go g.proxyTCPConnection(route.ID, clientConn, backendAddr)
	}
}

func (g *Gateway) proxyTCPConnection(routeID string, clientConn net.Conn, backendAddr string) {
	defer clientConn.Close()
	backendConn, err := net.DialTimeout("tcp", backendAddr, 30*time.Second)
	if err != nil {
		log.Printf("API Gateway TCP: route %s dial backend %s: %v", routeID, backendAddr, err)
		return
	}
	defer backendConn.Close()
	go io.Copy(backendConn, clientConn)
	io.Copy(clientConn, backendConn)
}
