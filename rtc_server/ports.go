package rtc_server

import (
	"fmt"
	"net"
	"strconv"

	"redock/api_gateway"
	"redock/turn_server"
)

// PortRule is one inbound rule to open on the router / firewall: traffic from
// the internet to External on the router goes to Internal on this server.
type PortRule struct {
	Service  string `json:"service"`  // "gateway", "sfu", "turn", "galene-turn"
	Protocol string `json:"protocol"` // "TCP", "UDP", "TCP+UDP"
	External string `json:"external"` // port or range on the router
	Internal string `json:"internal"` // port or range on this server
	Purpose  string `json:"purpose"`  // key for the dashboard's text
	Optional bool   `json:"optional"` // works without it, with limits
}

// PortPlan is everything that must be reachable for WebRTC, computed from
// the gateway, SFU and TURN settings so the dashboard has one source of truth.
type PortPlan struct {
	LocalIP  string     `json:"local_ip"`
	Rules    []PortRule `json:"rules"`
	Warnings []string   `json:"warnings"` // keys for the dashboard's text
}

func localIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}

func portRange(lo, hi int) string {
	if lo == hi {
		return strconv.Itoa(lo)
	}
	return fmt.Sprintf("%d-%d", lo, hi)
}

// Ports computes the inbound rules for the current configuration.
func (m *Manager) Ports() PortPlan {
	cfg := m.loadConfig()
	if cfg == nil {
		return PortPlan{LocalIP: localIP(), Rules: []PortRule{}, Warnings: []string{}}
	}
	httpPort, httpsPort := 80, 443
	if g := api_gateway.GetGateway(); g != nil {
		if gc := g.GetConfigCopy(); gc != nil {
			if gc.HTTPPort > 0 {
				httpPort = gc.HTTPPort
			}
			if gc.HTTPSPort > 0 {
				httpsPort = gc.HTTPSPort
			}
		}
	}
	var tc *turn_server.TURNConfigEntity
	if tm := turn_server.GetManager(); tm != nil {
		if c, err := tm.Config(); err == nil {
			tc = &c
		}
	}
	plan := portsFor(cfg, httpPort, httpsPort, tc)
	plan.LocalIP = localIP()
	return plan
}

// portsFor builds the plan from the settings (pure, for testing). The
// gateway may listen on non-standard ports internally while browsers use
// 80/443, which is exactly what the router's port forward has to translate.
func portsFor(cfg *RTCConfigEntity, httpPort, httpsPort int, tc *turn_server.TURNConfigEntity) PortPlan {
	plan := PortPlan{Rules: []PortRule{}, Warnings: []string{}}
	if cfg.PublicHost != "" {
		plan.Rules = append(plan.Rules,
			PortRule{Service: "gateway", Protocol: "TCP", External: "443", Internal: strconv.Itoa(httpsPort), Purpose: "https"},
			PortRule{Service: "gateway", Protocol: "TCP", External: "80", Internal: strconv.Itoa(httpPort), Purpose: "acme"},
		)
	} else {
		plan.Warnings = append(plan.Warnings, "no-public-host")
	}

	// SFU media: same port outside and inside (ICE advertises the local port).
	if cfg.UDPMin > 0 && cfg.UDPMax > 0 {
		r := portRange(int(cfg.UDPMin), int(cfg.UDPMax))
		plan.Rules = append(plan.Rules, PortRule{Service: "sfu", Protocol: "UDP", External: r, Internal: r, Purpose: "media"})
	} else {
		plan.Warnings = append(plan.Warnings, "media-any-port")
	}

	switch cfg.TURN {
	case TURNRedock:
		// Covered by the TURN server's rules below.
	case "":
		plan.Warnings = append(plan.Warnings, "no-turn")
	default:
		p := "1194"
		if cfg.TURN != "auto" {
			if _, port, err := net.SplitHostPort(cfg.TURN); err == nil {
				p = port
			}
		}
		plan.Rules = append(plan.Rules, PortRule{Service: "galene-turn", Protocol: "TCP+UDP", External: p, Internal: p, Purpose: "turn", Optional: true})
	}

	if tc != nil && (tc.Enabled || cfg.TURN == TURNRedock) {
		p := strconv.Itoa(tc.Port)
		plan.Rules = append(plan.Rules, PortRule{Service: "turn", Protocol: "TCP+UDP", External: p, Internal: p, Purpose: "turn", Optional: true})
		if tc.TLSPort > 0 {
			tp := strconv.Itoa(tc.TLSPort)
			plan.Rules = append(plan.Rules, PortRule{Service: "turn", Protocol: "TCP", External: tp, Internal: tp, Purpose: "turns", Optional: true})
		}
		r := portRange(int(tc.RelayMin), int(tc.RelayMax))
		plan.Rules = append(plan.Rules, PortRule{Service: "turn", Protocol: "UDP", External: r, Internal: r, Purpose: "relay", Optional: true})
		if !tc.Enabled {
			plan.Warnings = append(plan.Warnings, "turn-not-enabled")
		}
	}
	return plan
}
