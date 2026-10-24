package rtc_server

import (
	"fmt"
	"net"
	"strconv"

	"redock/api_gateway"
)

// gatewayID names the service, upstream and route Redock manages for the SFU.
const gatewayID = "rtc-galene"

// ensureGatewayRoute publishes the SFU at https://<host>/ through the API
// gateway: a service pointing at Galène's listener, an upstream, and a route
// for the host. PreserveHost keeps the browser's Host so Galène's same-origin
// WebSocket check passes. The gateway's Let's Encrypt domains are derived
// from route hosts, so the certificate is (re)issued to cover the host.
func ensureGatewayRoute(host, listenAddr string) error {
	g := api_gateway.GetGateway()
	if g == nil {
		return fmt.Errorf("API gateway is not initialized")
	}
	svcHost, portStr, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return fmt.Errorf("listen address: %w", err)
	}
	port, _ := strconv.Atoi(portStr)
	if svcHost == "" || svcHost == "0.0.0.0" || svcHost == "::" {
		svcHost = "127.0.0.1"
	}

	svc := api_gateway.Service{
		ID: gatewayID, Name: "WebRTC (Galène)", Host: svcHost, Port: port,
		Protocol: "http", Enabled: true,
	}
	if err := g.AddService(svc); err != nil {
		if err := g.UpdateService(svc); err != nil {
			return fmt.Errorf("gateway service: %w", err)
		}
	}

	up := api_gateway.Upstream{
		ID: gatewayID, Name: "WebRTC (Galène)", Strategy: api_gateway.StrategyRoundRobin,
		Targets: []api_gateway.UpstreamTarget{{ServiceID: gatewayID, Weight: 1}}, Enabled: true,
	}
	if err := g.AddUpstream(up); err != nil {
		if err := g.UpdateUpstream(up); err != nil {
			return fmt.Errorf("gateway upstream: %w", err)
		}
	}

	route := api_gateway.Route{
		ID: gatewayID, Name: "WebRTC (Galène)", UpstreamID: gatewayID,
		Paths: []string{"/"}, Hosts: []string{host},
		PreserveHost: true, Priority: 100, Enabled: true,
	}
	if err := g.AddRoute(route); err != nil {
		if err := g.UpdateRoute(route); err != nil {
			return fmt.Errorf("gateway route: %w", err)
		}
	}
	g.ReissueCertificateIfNeeded()
	return nil
}

// removeGatewayRoute deletes what ensureGatewayRoute created (missing parts
// are ignored).
func removeGatewayRoute() {
	g := api_gateway.GetGateway()
	if g == nil {
		return
	}
	_ = g.DeleteRoute(gatewayID)
	_ = g.DeleteUpstream(gatewayID)
	_ = g.DeleteService(gatewayID)
}
