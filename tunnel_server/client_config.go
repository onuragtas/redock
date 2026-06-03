package tunnel_server

import "redock/platform/memory"

// TableTunnelClientConfigs is the memory-DB table for persisted tunnel client start settings.
const TableTunnelClientConfigs = "tunnel_client_configs"

// TunnelClientConfig persists the local "start" settings for a tunnel domain so
// they survive restarts (no need to re-enter ip/port) and can be auto-started on
// boot. One row per Domain.
type TunnelClientConfig struct {
	memory.BaseEntity
	Domain                   string `json:"domain"`     // full domain to bind (unique key)
	DomainID                 uint   `json:"domain_id"`  // associated TunnelDomain.ID (best effort)
	LocalHttpIp              string `json:"local_http_ip"`
	LocalHttpPort            int    `json:"local_http_port"`
	LocalTcpIp               string `json:"local_tcp_ip"`
	LocalTcpPort             int    `json:"local_tcp_port"`
	LocalUdpIp               string `json:"local_udp_ip"`
	LocalUdpPort             int    `json:"local_udp_port"`
	SourceBindIp             string `json:"source_bind_ip"`
	HostRewrite              string `json:"host_rewrite"`
	KeepaliveIntervalSeconds int    `json:"keepalive_interval_seconds"`
	AutoStart                bool   `json:"auto_start"` // start automatically when redock boots
}

// AllClientConfigs returns every persisted tunnel client config.
func AllClientConfigs() []*TunnelClientConfig {
	return memory.FindAll[*TunnelClientConfig](GetDB(), TableTunnelClientConfigs)
}

// FindClientConfigByDomain returns the persisted config for a domain, or nil.
func FindClientConfigByDomain(domain string) *TunnelClientConfig {
	for _, c := range AllClientConfigs() {
		if c.Domain == domain {
			return c
		}
	}
	return nil
}

// UpsertClientConfig creates or updates the config for cfg.Domain.
func UpsertClientConfig(cfg *TunnelClientConfig) error {
	if existing := FindClientConfigByDomain(cfg.Domain); existing != nil {
		cfg.ID = existing.ID
		cfg.CreatedAt = existing.CreatedAt
		return memory.Update[*TunnelClientConfig](GetDB(), TableTunnelClientConfigs, cfg)
	}
	return memory.Create[*TunnelClientConfig](GetDB(), TableTunnelClientConfigs, cfg)
}

// SetClientConfigAutoStart updates only the auto-start flag for a domain. The
// config row is created (with just the flag) if it does not exist yet.
func SetClientConfigAutoStart(domain string, autoStart bool) error {
	existing := FindClientConfigByDomain(domain)
	if existing == nil {
		existing = &TunnelClientConfig{Domain: domain}
	}
	existing.AutoStart = autoStart
	return UpsertClientConfig(existing)
}

// DeleteClientConfigByDomain removes the persisted config for a domain (no-op if absent).
func DeleteClientConfigByDomain(domain string) error {
	existing := FindClientConfigByDomain(domain)
	if existing == nil {
		return nil
	}
	return memory.Delete[*TunnelClientConfig](GetDB(), TableTunnelClientConfigs, existing.ID)
}
