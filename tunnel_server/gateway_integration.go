package tunnel_server

import (
	"fmt"
	"log"
	"strconv"

	"redock/api_gateway"
)

const (
	gatewayServicePrefix    = "tunnel-s-"
	gatewayRoutePrefix      = "tunnel-r-"
	gatewayUDPServicePrefix = "tunnel-su-"
	gatewayUDPRoutePrefix   = "tunnel-u-"
	gatewayTCPServicePrefix = "tunnel-st-"
	gatewayTCPRoutePrefix   = "tunnel-t-"
)

// AddTunnelDomainToGateway adds api_gateway Route+Service (HTTP), optionally TCPRoute+Service (raw TCP), and optionally UDPRoute+Service (UDP) for the tunnel domain.
// Backend: HTTP -> 127.0.0.1:domain.Port; raw TCP -> 127.0.0.1:internalTcpPort(domain.Port); UDP -> 127.0.0.1:internalUDPPort(domain.Port).
func AddTunnelDomainToGateway(d *TunnelDomain) error {
	gw := api_gateway.GetGateway()
	if gw == nil {
		return fmt.Errorf("api_gateway not initialized")
	}
	idStr := strconv.FormatUint(uint64(d.ID), 10)

	// HTTP/HTTPS: Service + Route (Host -> backend 127.0.0.1:Port)
	needHTTP := d.Protocol == "http" || d.Protocol == "https"
	if needHTTP {
		svc := api_gateway.Service{
			ID:       gatewayServicePrefix + idStr,
			Name:     "tunnel:" + d.FullDomain,
			Host:     "127.0.0.1",
			Port:     d.Port,
			Protocol: "http",
			Enabled:  true,
		}
		if err := gw.AddService(svc); err != nil {
			return fmt.Errorf("gateway AddService: %w", err)
		}
		d.GatewayServiceID = svc.ID

		route := api_gateway.Route{
			ID:         gatewayRoutePrefix + idStr,
			Name:       "tunnel:" + d.FullDomain,
			ServiceID:  svc.ID,
			Hosts:     []string{d.FullDomain},
			Paths:     []string{"/"},
			Priority:  100,
			StripPath: false,
			Enabled:   true,
		}
		if err := gw.AddRoute(route); err != nil {
			_ = gw.DeleteService(svc.ID)
			return fmt.Errorf("gateway AddRoute: %w", err)
		}
		d.GatewayRouteID = route.ID
	}
	if needHTTP {
		StartBackendListener(d.Port)
	}

	// Raw TCP (tcp / tcp+udp): TCPService + TCPRoute; gateway 0.0.0.0:Port -> 127.0.0.1:internalTcpPort(Port)
	needTCP := d.Protocol == "tcp" || d.Protocol == "tcp+udp"
	if needTCP {
		internalPort := internalTcpPort(d.Port)
		tcpSvc := api_gateway.Service{
			ID:       gatewayTCPServicePrefix + idStr,
			Name:     "tunnel-tcp:" + d.FullDomain,
			Host:     "127.0.0.1",
			Port:     internalPort,
			Protocol: "tcp",
			Enabled:  true,
		}
		if err := gw.AddService(tcpSvc); err != nil {
			if needHTTP {
				_ = gw.DeleteRoute(d.GatewayRouteID)
				_ = gw.DeleteService(d.GatewayServiceID)
				StopBackendListener(d.Port)
			}
			return fmt.Errorf("gateway AddService TCP: %w", err)
		}
		d.GatewayTCPServiceID = tcpSvc.ID

		tcpRoute := api_gateway.TCPRoute{
			ID:         gatewayTCPRoutePrefix + idStr,
			Name:       "tunnel:" + d.FullDomain,
			ListenPort: d.Port,
			ServiceID:  tcpSvc.ID,
			Enabled:    true,
		}
		if err := gw.AddTCPRoute(tcpRoute); err != nil {
			_ = gw.DeleteService(tcpSvc.ID)
			if needHTTP {
				_ = gw.DeleteRoute(d.GatewayRouteID)
				_ = gw.DeleteService(d.GatewayServiceID)
				StopBackendListener(d.Port)
			}
			return fmt.Errorf("gateway AddTCPRoute: %w", err)
		}
		d.GatewayTCPRouteID = tcpRoute.ID
		StartBackendTCPListener(internalPort)
	}

	// UDP (udp / tcp+udp): UDP Service + UDPRoute
	needUDP := d.Protocol == "udp" || d.Protocol == "tcp+udp"
	if needUDP {
		internalPort := internalUDPPort(d.Port)
		udpSvc := api_gateway.Service{
			ID:       gatewayUDPServicePrefix + idStr,
			Name:     "tunnel-udp:" + d.FullDomain,
			Host:     "127.0.0.1",
			Port:     internalPort,
			Protocol: "udp",
			Enabled:  true,
		}
		if err := gw.AddService(udpSvc); err != nil {
			if needTCP {
				StopBackendTCPListener(internalTcpPort(d.Port))
				_ = gw.RemoveTCPRoute(d.GatewayTCPRouteID)
				_ = gw.DeleteService(d.GatewayTCPServiceID)
			}
			if needHTTP {
				_ = gw.DeleteRoute(d.GatewayRouteID)
				_ = gw.DeleteService(d.GatewayServiceID)
				StopBackendListener(d.Port)
			}
			return fmt.Errorf("gateway AddService UDP: %w", err)
		}
		d.GatewayUDPServiceID = udpSvc.ID

		udpRoute := api_gateway.UDPRoute{
			ID:         gatewayUDPRoutePrefix + idStr,
			Name:       "tunnel:" + d.FullDomain,
			ListenPort: d.Port,
			ServiceID:  udpSvc.ID,
			Enabled:    true,
		}
		if err := gw.AddUDPRoute(udpRoute); err != nil {
			_ = gw.DeleteService(udpSvc.ID)
			if needTCP {
				StopBackendTCPListener(internalTcpPort(d.Port))
				_ = gw.RemoveTCPRoute(d.GatewayTCPRouteID)
				_ = gw.DeleteService(d.GatewayTCPServiceID)
			}
			if needHTTP {
				_ = gw.DeleteRoute(d.GatewayRouteID)
				_ = gw.DeleteService(d.GatewayServiceID)
				StopBackendListener(d.Port)
			}
			return fmt.Errorf("gateway AddUDPRoute: %w", err)
		}
		d.GatewayUDPRouteID = udpRoute.ID
	}
	// Backend UDP dinleyici: daemon internal portta dinler (port çakışması olmasın diye gateway 0.0.0.0:Port, daemon 127.0.0.1:(Port+offset))
	if needUDP {
		StartBackendUDPListener(internalUDPPort(d.Port))
	}

	// Domain eklendikten sonra gateway etkinse ve çalışmıyorsa otomatik başlat.
	gw.StartAll()

	return nil
}

// RemoveTunnelDomainFromGateway removes api_gateway Route(s), Service(s), UDPRoute and TCPRoute for the tunnel domain.
func RemoveTunnelDomainFromGateway(d *TunnelDomain) error {
	// Backend HTTP dinleyiciyi durdur (sadece http/https)
	if d.Protocol == "http" || d.Protocol == "https" {
		StopBackendListener(d.Port)
	}
	// Backend raw TCP dinleyiciyi durdur (tcp/tcp+udp)
	if d.Protocol == "tcp" || d.Protocol == "tcp+udp" {
		StopBackendTCPListener(internalTcpPort(d.Port))
	}
	// Backend UDP dinleyiciyi durdur (udp/tcp+udp)
	if d.Protocol == "udp" || d.Protocol == "tcp+udp" {
		StopBackendUDPListener(internalUDPPort(d.Port))
	}
	gw := api_gateway.GetGateway()
	if gw == nil {
		return nil
	}
	var errs []error
	// TCP route/service (tcp, tcp+udp)
	if d.GatewayTCPRouteID != "" {
		if err := gw.RemoveTCPRoute(d.GatewayTCPRouteID); err != nil {
			log.Printf("tunnel_server: RemoveTCPRoute %s: %v", d.GatewayTCPRouteID, err)
			errs = append(errs, err)
		}
	}
	if d.GatewayTCPServiceID != "" {
		if err := gw.DeleteService(d.GatewayTCPServiceID); err != nil {
			log.Printf("tunnel_server: DeleteService TCP %s: %v", d.GatewayTCPServiceID, err)
			errs = append(errs, err)
		}
	}
	if d.GatewayUDPRouteID != "" {
		if err := gw.RemoveUDPRoute(d.GatewayUDPRouteID); err != nil {
			log.Printf("tunnel_server: RemoveUDPRoute %s: %v", d.GatewayUDPRouteID, err)
			errs = append(errs, err)
		}
	}
	if d.GatewayUDPServiceID != "" {
		if err := gw.DeleteService(d.GatewayUDPServiceID); err != nil {
			log.Printf("tunnel_server: DeleteService UDP %s: %v", d.GatewayUDPServiceID, err)
			errs = append(errs, err)
		}
	}
	if d.GatewayRouteID != "" {
		if err := gw.DeleteRoute(d.GatewayRouteID); err != nil {
			log.Printf("tunnel_server: DeleteRoute %s: %v", d.GatewayRouteID, err)
			errs = append(errs, err)
		}
	}
	if d.GatewayServiceID != "" {
		if err := gw.DeleteService(d.GatewayServiceID); err != nil {
			log.Printf("tunnel_server: DeleteService %s: %v", d.GatewayServiceID, err)
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}
