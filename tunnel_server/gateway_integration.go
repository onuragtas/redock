package tunnel_server

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

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
	needHTTP := d.Protocol == "http" || d.Protocol == "https" || d.Protocol == "all"
	needTCP := d.Protocol == "tcp" || d.Protocol == "tcp+udp" || d.Protocol == "all"
	needUDP := d.Protocol == "udp" || d.Protocol == "tcp+udp" || d.Protocol == "all"

	cfg := gw.GetConfigCopy()
	if cfg == nil {
		return fmt.Errorf("gateway config copy failed")
	}
	// JSON copy can have nil slices if config file had null; ensure we can append and refreshServicesAndRoutes works.
	if cfg.Services == nil {
		cfg.Services = []api_gateway.Service{}
	}
	if cfg.Routes == nil {
		cfg.Routes = []api_gateway.Route{}
	}
	if cfg.TCPRoutes == nil {
		cfg.TCPRoutes = []api_gateway.TCPRoute{}
	}
	if cfg.UDPRoutes == nil {
		cfg.UDPRoutes = []api_gateway.UDPRoute{}
	}
	// Gateway must be enabled so UpdateConfig restarts it and StartAll() actually starts listeners.
	cfg.Enabled = true

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
			ID:        gatewayRoutePrefix + idStr,
			Name:      "tunnel:" + d.FullDomain,
			ServiceID: svc.ID,
			Hosts:     []string{d.FullDomain},
			Paths:     []string{"/"},
			Priority:  100,
			StripPath: false,
			Enabled:   true,
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

	// If Let's Encrypt is enabled, add domain to list and request certificate in background so the API response is not blocked (avoids 502/timeout)
	if needHTTP {
		go addTunnelDomainToLetsEncrypt(gw, d.FullDomain)
	}

	return nil
}

// addTunnelDomainToLetsEncrypt adds fullDomain to the gateway's Let's Encrypt domain list (if not already present) and requests a new certificate (one bulk SAN request for all domains).
func addTunnelDomainToLetsEncrypt(gw *api_gateway.Gateway, fullDomain string) {
	cfg := gw.GetConfig()
	if cfg == nil || cfg.LetsEncrypt == nil || !cfg.LetsEncrypt.Enabled {
		return
	}
	fullDomain = strings.TrimSpace(fullDomain)
	if fullDomain == "" {
		return
	}
	for _, d := range cfg.LetsEncrypt.Domains {
		if strings.TrimSpace(d) == fullDomain {
			return
		}
	}
	domains := make([]string, 0, len(cfg.LetsEncrypt.Domains)+1)
	domains = append(domains, cfg.LetsEncrypt.Domains...)
	domains = append(domains, fullDomain)
	leCopy := &api_gateway.LetsEncryptConfig{
		Enabled:          cfg.LetsEncrypt.Enabled,
		Email:            cfg.LetsEncrypt.Email,
		Domains:          domains,
		Staging:          cfg.LetsEncrypt.Staging,
		AutoRenew:        cfg.LetsEncrypt.AutoRenew,
		RenewBeforeDays:  cfg.LetsEncrypt.RenewBeforeDays,
		LastRenewAt:      cfg.LetsEncrypt.LastRenewAt,
		ExpiresAt:        cfg.LetsEncrypt.ExpiresAt,
		CertificateReady: cfg.LetsEncrypt.CertificateReady,
	}
	if err := gw.ConfigureLetsEncrypt(leCopy); err != nil {
		log.Printf("tunnel_server: add domain to Let's Encrypt config: %v", err)
		return
	}
	// Wait for gateway to be running before ACME HTTP-01 (with timeout to avoid spinning forever)
	const pollInterval = 200 * time.Millisecond
	const maxWait = 15 * time.Second
	deadline := time.Now().Add(maxWait)
	for !gw.IsRunning() {
		if time.Now().After(deadline) {
			log.Printf("tunnel_server: gateway did not start within %v, skipping certificate request for %s", maxWait, fullDomain)
			return
		}
		time.Sleep(pollInterval)
	}

	log.Println("tunnel_server: waiting for 15 seconds")
	time.Sleep(15 * time.Second)
	log.Println("tunnel_server: 15 seconds passed")
	// Request certificate for full domain list (one SAN cert: tls.crt / tls.key)
	if err := gw.RequestCertificateWithConfig(leCopy); err != nil {
		log.Printf("tunnel_server: request certificate for %s (full list): %v", fullDomain, err)
		return
	}
	log.Printf("tunnel_server: Let's Encrypt certificate updated to include %s", fullDomain)
}

// removeTunnelDomainFromLetsEncrypt removes fullDomain from the gateway's Let's Encrypt domain list.
// The current certificate is not re-issued; the domain is dropped from the list so the next renewal excludes it.
func removeTunnelDomainFromLetsEncrypt(gw *api_gateway.Gateway, fullDomain string) {
	cfg := gw.GetConfig()
	if cfg == nil || cfg.LetsEncrypt == nil || !cfg.LetsEncrypt.Enabled {
		return
	}
	fullDomain = strings.TrimSpace(fullDomain)
	if fullDomain == "" {
		return
	}
	var newDomains []string
	for _, d := range cfg.LetsEncrypt.Domains {
		if strings.TrimSpace(d) != fullDomain {
			newDomains = append(newDomains, d)
		}
	}
	if len(newDomains) == len(cfg.LetsEncrypt.Domains) {
		return
	}
	leCopy := &api_gateway.LetsEncryptConfig{
		Enabled:          cfg.LetsEncrypt.Enabled,
		Email:            cfg.LetsEncrypt.Email,
		Domains:          newDomains,
		Staging:          cfg.LetsEncrypt.Staging,
		AutoRenew:        cfg.LetsEncrypt.AutoRenew,
		RenewBeforeDays:  cfg.LetsEncrypt.RenewBeforeDays,
		LastRenewAt:      cfg.LetsEncrypt.LastRenewAt,
		ExpiresAt:        cfg.LetsEncrypt.ExpiresAt,
		CertificateReady: cfg.LetsEncrypt.CertificateReady,
	}
	if err := gw.ConfigureLetsEncrypt(leCopy); err != nil {
		log.Printf("tunnel_server: remove domain from Let's Encrypt config: %v", err)
		return
	}
	log.Printf("tunnel_server: %s removed from Let's Encrypt domain list", fullDomain)
}

// SetTunnelRouteHostRewrite updates the HTTP route's HostRewrite for the tunnel domain (only for http/https). Empty string clears the override.
func SetTunnelRouteHostRewrite(d *TunnelDomain, hostRewrite string) error {
	if d.GatewayRouteID == "" {
		return nil // no HTTP route (e.g. tcp/udp-only)
	}
	gw := api_gateway.GetGateway()
	if gw == nil {
		return nil
	}
	cfg := gw.GetConfigCopy()
	if cfg == nil {
		return nil
	}
	for i := range cfg.Routes {
		if cfg.Routes[i].ID == d.GatewayRouteID {
			cfg.Routes[i].HostRewrite = hostRewrite
			return gw.UpdateRoute(cfg.Routes[i])
		}
	}
	return nil
}

// RemoveTunnelDomainFromGateway removes api_gateway Route(s), Service(s), UDPRoute and TCPRoute for the tunnel domain.
func RemoveTunnelDomainFromGateway(d *TunnelDomain) error {
	// Backend HTTP dinleyiciyi durdur (http/https/all)
	if d.Protocol == "http" || d.Protocol == "https" || d.Protocol == "all" {
		StopBackendListener(d.Port)
	}
	// Backend raw TCP dinleyiciyi durdur (tcp/tcp+udp/all)
	if d.Protocol == "tcp" || d.Protocol == "tcp+udp" || d.Protocol == "all" {
		StopBackendTCPListener(internalTcpPort(d.Port))
	}
	// Backend UDP dinleyiciyi durdur (udp/tcp+udp/all)
	if d.Protocol == "udp" || d.Protocol == "tcp+udp" || d.Protocol == "all" {
		StopBackendUDPListener(internalUDPPort(d.Port))
	}
	gw := api_gateway.GetGateway()
	if gw == nil {
		return nil
	}
	// Remove this HTTP(S) domain from Let's Encrypt list so next renewal doesn't include it
	if d.Protocol == "http" || d.Protocol == "https" || d.Protocol == "all" {
		removeTunnelDomainFromLetsEncrypt(gw, d.FullDomain)
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
