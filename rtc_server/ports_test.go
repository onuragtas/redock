package rtc_server

import (
	"testing"

	"redock/turn_server"
)

func findRule(p PortPlan, purpose string) *PortRule {
	for i := range p.Rules {
		if p.Rules[i].Purpose == purpose {
			return &p.Rules[i]
		}
	}
	return nil
}

func hasWarning(p PortPlan, w string) bool {
	for _, x := range p.Warnings {
		if x == w {
			return true
		}
	}
	return false
}

// Browsers use 80/443; a gateway listening on 8080/8443 means the router must
// forward 443→8443 and 80→8080. TURN, TLS and relay follow the TURN settings.
func TestPortPlanTranslatesGatewayPortsAndIncludesTURN(t *testing.T) {
	cfg := &RTCConfigEntity{PublicHost: "rtc.example.com", UDPMin: 40000, UDPMax: 40199, TURN: TURNRedock}
	tc := &turn_server.TURNConfigEntity{Enabled: true, Port: 3478, TLSPort: 5349, RelayMin: 49160, RelayMax: 49359}
	p := portsFor(cfg, 8080, 8443, tc)

	cases := []struct{ purpose, proto, ext, in string }{
		{"https", "TCP", "443", "8443"},
		{"acme", "TCP", "80", "8080"},
		{"media", "UDP", "40000-40199", "40000-40199"},
		{"turn", "TCP+UDP", "3478", "3478"},
		{"turns", "TCP", "5349", "5349"},
		{"relay", "UDP", "49160-49359", "49160-49359"},
	}
	for _, c := range cases {
		r := findRule(p, c.purpose)
		if r == nil || r.Protocol != c.proto || r.External != c.ext || r.Internal != c.in {
			t.Errorf("%s: got %+v, want %s %s→%s", c.purpose, r, c.proto, c.ext, c.in)
		}
	}
	if len(p.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", p.Warnings)
	}
}

func TestPortPlanWarnings(t *testing.T) {
	// No domain, random media ports, Redock TURN selected but switched off.
	cfg := &RTCConfigEntity{TURN: TURNRedock}
	tc := &turn_server.TURNConfigEntity{Port: 3478, RelayMin: 49160, RelayMax: 49359}
	p := portsFor(cfg, 80, 443, tc)
	for _, w := range []string{"no-public-host", "media-any-port", "turn-not-enabled"} {
		if !hasWarning(p, w) {
			t.Errorf("missing warning %q (got %v)", w, p.Warnings)
		}
	}
	if findRule(p, "https") != nil {
		t.Error("HTTPS rule without a public host")
	}

	// Galène's built-in TURN on its default port; TLS-less Redock TURN not listed.
	p = portsFor(&RTCConfigEntity{PublicHost: "x.example.com", UDPMin: 1, UDPMax: 2, TURN: "auto"}, 80, 443, &turn_server.TURNConfigEntity{})
	if r := findRule(p, "turn"); r == nil || r.Service != "galene-turn" || r.External != "1194" {
		t.Errorf("built-in TURN rule = %+v", r)
	}
	if p := portsFor(&RTCConfigEntity{PublicHost: "x.example.com", UDPMin: 1, UDPMax: 2}, 80, 443, nil); !hasWarning(p, "no-turn") {
		t.Error("no warning when TURN is off")
	}
}
