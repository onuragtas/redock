package tunnel_server

import (
	"redock/platform/memory"
)

const TableTunnelServerConfig = "tunnel_server_config"

// TunnelServerConfig holds tunnel server configuration (single row in memory DB).
type TunnelServerConfig struct {
	memory.BaseEntity
	Enabled             bool   `json:"enabled"`
	DomainSuffix        string `json:"domain_suffix"`        // Cloudflare zone name
	CloudflareZoneID    string `json:"cloudflare_zone_id"`   // Cloudflare zone for tunnel subdomains
	PortRangeStart      int    `json:"port_range_start"`      // e.g. 9100
	TunnelListenAddr    string `json:"tunnel_listen_addr"`   // e.g. ":8443" (TLS TCP or wss)
	UnusedDomainTTLDays int    `json:"unused_domain_ttl_days"` // delete domains not used for this many days (0 = disable)
}

// DefaultConfig returns a default tunnel server config (no ID, for creating first row).
func DefaultConfig() *TunnelServerConfig {
	return &TunnelServerConfig{
		Enabled:             false,
		DomainSuffix:        "",
		CloudflareZoneID:    "",
		PortRangeStart:      9100,
		TunnelListenAddr:    ":8443",
		UnusedDomainTTLDays: 30,
	}
}

// GetConfig returns the current config from memory DB (single row). If none exists, creates default and returns it.
func GetConfig() *TunnelServerConfig {
	all := memory.FindAll[*TunnelServerConfig](GetDB(), TableTunnelServerConfig)
	if len(all) == 0 {
		def := DefaultConfig()
		_ = memory.Create[*TunnelServerConfig](GetDB(), TableTunnelServerConfig, def)
		return def
	}
	return all[0]
}

// UpdateConfig updates the config row in memory DB (config must have ID set, e.g. from GetConfig).
func UpdateConfig(c *TunnelServerConfig) error {
	if c.ID == 0 {
		all := memory.FindAll[*TunnelServerConfig](GetDB(), TableTunnelServerConfig)
		if len(all) > 0 {
			c.ID = all[0].ID
			c.CreatedAt = all[0].CreatedAt
		}
	}
	if c.ID == 0 {
		return memory.Create[*TunnelServerConfig](GetDB(), TableTunnelServerConfig, c)
	}
	return memory.Update[*TunnelServerConfig](GetDB(), TableTunnelServerConfig, c)
}
