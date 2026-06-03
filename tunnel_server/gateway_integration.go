package tunnel_server

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"redock/api_gateway"
)

// protoNeeds reports which transports a tunnel protocol string enables. It
// accepts "all" plus any "+"-joined combination of http/https/tcp/udp
// (e.g. "http+tcp"), so partial combinations don't fall back to "all".
func protoNeeds(p string) (http, tcp, udp bool) {
	if p == "all" {
		return true, true, true
	}
	return strings.Contains(p, "http"), strings.Contains(p, "tcp"), strings.Contains(p, "udp")
}

const (
	gatewayServicePrefix    = "tunnel-s-"
	gatewayUpstreamPrefix   = "tunnel-up-"
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
	needHTTP, needTCP, needUDP := protoNeeds(d.Protocol)

	cfg := gw.GetConfigCopy()
	if cfg == nil {
		return fmt.Errorf("gateway config copy failed")
	}
	// JSON copy can have nil slices if config file had null; ensure we can append and refreshServicesAndRoutes works.
	if cfg.Services == nil {
		cfg.Services = []api_gateway.Service{}
	}
	if cfg.Upstreams == nil {
		cfg.Upstreams = []api_gateway.Upstream{}
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

	// HTTP/HTTPS: Service + Upstream (single target) + Route (backend 127.0.0.1:internalHttpPort; 0.0.0.0:d.Port gateway TCP/UDP için serbest, PORTS.md)
	if needHTTP {
		svc := api_gateway.Service{
			ID:       gatewayServicePrefix + idStr,
			Name:     "tunnel:" + d.FullDomain,
			Host:     "127.0.0.1",
			Port:     internalHttpPort(d.Port),
			Protocol: "http",
			Enabled:  true,
		}
		upstream := api_gateway.Upstream{
			ID:       gatewayUpstreamPrefix + idStr,
			Name:     "tunnel:" + d.FullDomain,
			Strategy: api_gateway.StrategyRoundRobin,
			Targets:  []api_gateway.UpstreamTarget{{ServiceID: svc.ID, Weight: 1}},
			Enabled:  true,
		}
		route := api_gateway.Route{
			ID:          gatewayRoutePrefix + idStr,
			Name:        "tunnel:" + d.FullDomain,
			UpstreamID:  upstream.ID,
			Hosts:       []string{d.FullDomain},
			Paths:       []string{"/"},
			Priority:    100,
			StripPath:   false,
			LetsEncrypt: true,
			Enabled:     true,
		}
		cfg.Services = append(cfg.Services, svc)
		cfg.Upstreams = append(cfg.Upstreams, upstream)
		cfg.Routes = append(cfg.Routes, route)
		d.GatewayServiceID = svc.ID
		d.GatewayUpstreamID = upstream.ID
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

	// The HTTP route was created with LetsEncrypt=true, so its host is already
	// part of the cert domain set. Request the certificate in the background so
	// the API response is not blocked (avoids 502/timeout).
	if needHTTP {
		go requestTunnelCertificate(gw, d.FullDomain)
	}

	return nil
}

// requestTunnelCertificate waits for the gateway to be serving HTTP-01 and then
// re-issues the certificate, which now covers all Let's Encrypt routes including
// the tunnel route just added.
func requestTunnelCertificate(gw *api_gateway.Gateway, fullDomain string) {
	cfg := gw.GetConfig()
	if cfg == nil || cfg.LetsEncrypt == nil || !cfg.LetsEncrypt.Enabled {
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

	time.Sleep(15 * time.Second)
	if err := gw.RequestCertificate(); err != nil {
		log.Printf("tunnel_server: request certificate for %s: %v", fullDomain, err)
		return
	}
	log.Printf("tunnel_server: Let's Encrypt certificate updated to include %s", fullDomain)
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
	needHTTP, needTCP, needUDP := protoNeeds(d.Protocol)
	if needHTTP {
		StopBackendListener(d.Port)
	}
	if needTCP {
		StopBackendTCPListener(internalTcpPort(d.Port))
	}
	if needUDP {
		StopBackendUDPListener(internalUDPPort(d.Port))
	}
	gw := api_gateway.GetGateway()
	if gw == nil {
		return nil
	}
	// The HTTP route is deleted below; since cert domains derive from routes,
	// the host drops out of the certificate on the next issuance automatically.
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
	// Pre-migration domains stored only GatewayServiceID; their HTTP route was
	// auto-bound to "auto-<serviceID>" during config migration. Try to delete
	// that derived upstream too so we don't leak orphans.
	upstreamToDelete := d.GatewayUpstreamID
	if upstreamToDelete == "" && d.GatewayServiceID != "" && d.GatewayRouteID != "" {
		upstreamToDelete = "auto-" + d.GatewayServiceID
	}
	if upstreamToDelete != "" {
		if err := gw.DeleteUpstream(upstreamToDelete); err != nil {
			// Non-fatal: upstream may have been removed manually or never existed.
			log.Printf("tunnel_server: DeleteUpstream %s: %v", upstreamToDelete, err)
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

// RefreshTunnelDomainIntegrations re-applies all integrations for an existing domain (same as create flow):
// removes from gateway, ensures DNS A record first (required for Let's Encrypt HTTP-01), then re-adds to gateway
// with correct ports and Let's Encrypt. Use for "renew" or to fix stale gateway/DNS state.
func RefreshTunnelDomainIntegrations(d *TunnelDomain) error {
	// 1. Remove from gateway (stops backend listeners, removes routes/services, removes from Let's Encrypt list)
	if err := RemoveTunnelDomainFromGateway(d); err != nil {
		return fmt.Errorf("remove from gateway: %w", err)
	}
	// 2. DNS (Cloudflare) first: Let's Encrypt HTTP-01 requires the domain to resolve to this server
	cfg := GetConfig()
	if cfg != nil && cfg.CloudflareZoneID != "" && d.FullDomain != "" {
		serverIP := cfg.ServerPublicIP
		if serverIP == "" {
			serverIP = DetectPublicIP()
		}
		if serverIP != "" {
			if d.CloudflareRecordID != "" {
				if err := UpdateTunnelDNSRecord(cfg.CloudflareZoneID, d.CloudflareRecordID, d.FullDomain, serverIP); err != nil {
					log.Printf("tunnel_server: refresh DNS update A record for %s: %v", d.FullDomain, err)
					// non-fatal
				}
			} else {
				recordID, err := CreateTunnelDNSRecord(cfg.CloudflareZoneID, d.FullDomain, serverIP)
				if err != nil {
					log.Printf("tunnel_server: refresh DNS create A record for %s: %v", d.FullDomain, err)
					// non-fatal
				} else {
					d.CloudflareRecordID = recordID
				}
			}
		}
	}
	// 3. Re-add to gateway (correct HTTP port internalHttpPort(d.Port), TCP/UDP, backend listeners, Let's Encrypt)
	if err := AddTunnelDomainToGateway(d); err != nil {
		return fmt.Errorf("add to gateway: %w", err)
	}
	// 4. Persist domain (new gateway IDs, possibly CloudflareRecordID)
	if err := UpdateDomain(d); err != nil {
		return fmt.Errorf("update domain: %w", err)
	}
	return nil
}
