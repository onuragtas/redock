package tunnel_server

import (
	"fmt"
	"redock/platform/memory"
	"time"
)

const (
	TableTunnelDomains   = "tunnel_domains"
	TableTunnelUsers     = "tunnel_users"
	TableTunnelCreds     = "tunnel_server_credentials"
	TableTunnelServers   = "tunnel_servers" // Federation: eklenen tünel sunucuları
)

// TunnelServer represents a tunnel server in the federation list (client tarafında sunucu seçimi).
// BaseURL = harici tünel sunucusu URL'i.
type TunnelServer struct {
	memory.BaseEntity
	Name      string `json:"name"`       // Görünen ad
	BaseURL   string `json:"base_url"`   // Harici tünel sunucusu API base URL
	IsDefault bool   `json:"is_default"` // Varsayılan seçili sunucu
	Order     int    `json:"order"`      // Sıralama (küçük önce)
}

// TunnelDomain represents a tunnel domain (subdomain + port, owned by a tunnel user).
type TunnelDomain struct {
	memory.BaseEntity
	UserID             uint      `json:"user_id"`               // TunnelUser.ID
	Subdomain          string    `json:"subdomain"`             // e.g. "myapp"
	FullDomain         string    `json:"full_domain"`           // subdomain + suffix
	Port               int       `json:"port"`                  // assigned TCP/UDP port
	Protocol           string    `json:"protocol"`              // http, https, udp, tcp, tcp+udp, or all (HTTP+HTTPS+TCP+UDP)
	CloudflareRecordID string    `json:"cloudflare_record_id"`  // A record ID for delete
	LastUsedAt         *time.Time `json:"last_used_at"`         // last time tunnel was started (BIND); nil = never
	GatewayServiceID    string   `json:"gateway_service_id"`    // api_gateway Service ID (HTTP/TCP backend)
	GatewayRouteID      string   `json:"gateway_route_id"`      // api_gateway Route ID (Host → Service)
	GatewayUDPServiceID   string `json:"gateway_udp_service_id"`   // api_gateway Service ID (UDP backend, udp/tcp+udp)
	GatewayUDPRouteID     string `json:"gateway_udp_route_id"`    // api_gateway UDPRoute ID
	GatewayTCPServiceID   string `json:"gateway_tcp_service_id"`   // api_gateway Service ID (raw TCP backend, tcp/tcp+udp)
	GatewayTCPRouteID     string `json:"gateway_tcp_route_id"`     // api_gateway TCPRoute ID
}

// TunnelUser represents a tunnel OAuth2 user (register/login).
type TunnelUser struct {
	memory.BaseEntity
	Email        string `json:"email"`
	PasswordHash string `json:"password_hash"`
}

// NextPort returns the next available port for a new domain (PortRangeStart + count or lastPort+1).
func NextPort(getDomains func() []*TunnelDomain) (int, error) {
	cfg := GetConfig()
	if cfg == nil {
		return 0, fmt.Errorf("tunnel server config not loaded")
	}

	domains := getDomains()
	if len(domains) == 0 {
		return cfg.PortRangeStart, nil
	}

	maxPort := cfg.PortRangeStart - 1
	for _, d := range domains {
		if d.Port > maxPort {
			maxPort = d.Port
		}
	}
	return maxPort + 1, nil
}

// FullDomainFor builds full domain from subdomain and suffix.
func FullDomainFor(subdomain, suffix string) string {
	if suffix != "" && suffix[0] != '.' {
		suffix = "." + suffix
	}
	return subdomain + suffix
}

// TunnelServerCredential stores OAuth2 tokens per tunnel server (client-side, in DB).
type TunnelServerCredential struct {
	memory.BaseEntity
	BaseURL      string    `json:"base_url"`       // tunnel server BaseURL
	AccessToken  string    `json:"access_token"`   // OAuth2 access token
	RefreshToken string    `json:"refresh_token"`  // OAuth2 refresh token (optional)
	ExpiresAt    time.Time `json:"expires_at"`     // access token expiry
	UserID       uint      `json:"user_id"`       // redock user who owns this credential (optional)
}
