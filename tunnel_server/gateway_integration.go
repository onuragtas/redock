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
// Uses a single UpdateConfig so only one gateway restart happens (avoids double Stop/Start panic).
// Backend: HTTP -> 127.0.0.1:domain.Port; raw TCP -> 127.0.0.1:internalTcpPort(domain.Port); UDP -> 127.0.0.1:internalUDPPort(domain.Port).
func AddTunnelDomainToGateway(d *TunnelDomain) error {
	gw := api_gateway.GetGateway()
	if gw == nil {
		return fmt.Errorf("api_gateway not initialized")
	}
	idStr := strconv.FormatUint(uint64(d.ID), 10)
	needHTTP := d.Protocol == "http" || d.Protocol == "https"
	needTCP := d.Protocol == "tcp" || d.Protocol == "tcp+udp"
	needUDP := d.Protocol == "udp" || d.Protocol == "tcp+udp"

	cfg := gw.GetConfigCopy()
	if cfg == nil {
		return fmt.Errorf("gateway config copy failed")
	}

	// HTTP/HTTPS: Service + Route
	if needHTTP {
		svc := api_gateway.Service{
			ID:       gatewayServicePrefix + idStr,
			Name:     "tunnel:" + d.FullDomain,
			Host:     "127.0.0.1",
			Port:     d.Port,
			Protocol: "http",
			Enabled:  true,
		}
		route := api_gateway.Route{
			ID:         gatewayRoutePrefix + idStr,
			Name:       "tunnel:" + d.FullDomain,
			ServiceID:  svc.ID,
			Hosts:      []string{d.FullDomain},
			Paths:      []string{"/"},
			Priority:   100,
			StripPath:  false,
			Enabled:    true,
		}
		cfg.Services = append(cfg.Services, svc)
		cfg.Routes = append(cfg.Routes, route)
		d.GatewayServiceID = svc.ID
		d.GatewayRouteID = route.ID
	}

	// Raw TCP (tcp / tcp+udp)
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
		tcpRoute := api_gateway.TCPRoute{
			ID:         gatewayTCPRoutePrefix + idStr,
			Name:       "tunnel:" + d.FullDomain,
			ListenPort: d.Port,
			ServiceID:  tcpSvc.ID,
			Enabled:    true,
		}
		cfg.Services = append(cfg.Services, tcpSvc)
		if cfg.TCPRoutes == nil {
			cfg.TCPRoutes = []api_gateway.TCPRoute{}
		}
		cfg.TCPRoutes = append(cfg.TCPRoutes, tcpRoute)
		d.GatewayTCPServiceID = tcpSvc.ID
		d.GatewayTCPRouteID = tcpRoute.ID
	}

	// UDP (udp / tcp+udp)
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
		udpRoute := api_gateway.UDPRoute{
			ID:         gatewayUDPRoutePrefix + idStr,
			Name:       "tunnel:" + d.FullDomain,
			ListenPort: d.Port,
			ServiceID:  udpSvc.ID,
			Enabled:    true,
		}
		cfg.Services = append(cfg.Services, udpSvc)
		if cfg.UDPRoutes == nil {
			cfg.UDPRoutes = []api_gateway.UDPRoute{}
		}
		cfg.UDPRoutes = append(cfg.UDPRoutes, udpRoute)
		d.GatewayUDPServiceID = udpSvc.ID
		d.GatewayUDPRouteID = udpRoute.ID
	}

	// Single restart for all new routes
	if err := gw.UpdateConfig(cfg); err != nil {
		return fmt.Errorf("gateway UpdateConfig: %w", err)
	}

	// Start backend listeners after gateway config is applied
	if needHTTP {
		StartBackendListener(d.Port)
	}
	if needTCP {
		StartBackendTCPListener(internalTcpPort(d.Port))
	}
	if needUDP {
		StartBackendUDPListener(internalUDPPort(d.Port))
	}

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
