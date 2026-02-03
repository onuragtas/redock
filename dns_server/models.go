package dns_server

import (
	"encoding/json"
	"redock/platform/memory"
	"time"
)

// DNSConfig holds DNS server configuration
type DNSConfig struct {
	memory.SoftDeleteEntity
	Enabled                bool   `json:"enabled"`
	UDPPort                int    `json:"udp_port"`
	TCPPort                int    `json:"tcp_port"`
	DoHEnabled             bool   `json:"doh_enabled"`
	DoHPort                int    `json:"doh_port"`
	DoTEnabled             bool   `json:"dot_enabled"`
	DoTPort                int    `json:"dot_port"`
	UpstreamDNS            string `json:"upstream_dns"` // JSON array
	BlockingEnabled        bool   `json:"blocking_enabled"`
	QueryLogging           bool   `json:"query_logging"`
	LogRetentionDays       int    `json:"log_retention_days"`
	RateLimitEnabled       bool   `json:"rate_limit_enabled"`
	RateLimitQPS           int    `json:"rate_limit_qps"`
	CacheEnabled           bool   `json:"cache_enabled"`
	CacheTTL               int    `json:"cache_ttl"` // seconds
	SafeBrowsingEnabled    bool   `json:"safe_browsing_enabled"`
	ParentalControlEnabled bool   `json:"parental_control_enabled"`
}

// DNSBlocklist represents a blocklist source
type DNSBlocklist struct {
	memory.SoftDeleteEntity
	Name           string     `json:"name"`
	URL            string     `json:"url"`
	Enabled        bool       `json:"enabled"`
	Format         string     `json:"format"` // hosts, domains, adblock, auto
	LastUpdated    *time.Time `json:"last_updated,omitempty"`
	LastError      string     `json:"last_error,omitempty"`
	DomainCount    int        `json:"domain_count"`
	UpdateInterval int        `json:"update_interval"` // seconds
}

// DNSCustomFilter represents custom blocked or allowed domains
type DNSCustomFilter struct {
	memory.SoftDeleteEntity
	Domain     string `json:"domain"`
	Type       string `json:"type"` // blacklist, whitelist
	Comment    string `json:"comment,omitempty"`
	IsRegex    bool   `json:"is_regex"`
	IsWildcard bool   `json:"is_wildcard"`
}

// DNSQueryLog represents a logged DNS query (stored in memory DB for fast /logs; retention 24h)
type DNSQueryLog struct {
	memory.BaseEntity
	ClientIP     string `json:"client_ip"`
	Domain       string `json:"domain"`
	QueryType    string `json:"query_type"` // A, AAAA, CNAME, MX, etc.
	Response     string `json:"response"`
	Blocked      bool   `json:"blocked"`
	BlockReason  string `json:"block_reason,omitempty"`
	ResponseTime int    `json:"response_time"` // milliseconds
	Cached       bool   `json:"cached"`
}

// DNSStatistics represents aggregated statistics (computed in-memory, not stored)
type DNSStatistics struct {
	Date              time.Time `json:"date"`
	TotalQueries      int64     `json:"total_queries"`
	BlockedQueries    int64     `json:"blocked_queries"`
	CachedQueries     int64     `json:"cached_queries"`
	AvgResponseTime   float64   `json:"avg_response_time"` // milliseconds
	UniqueClients     int       `json:"unique_clients"`
	TopDomains        string    `json:"top_domains"`         // JSON
	TopBlockedDomains string    `json:"top_blocked_domains"` // JSON
	TopClients        string    `json:"top_clients"`         // JSON
}

// DNSClientSettings represents per-client DNS settings
type DNSClientSettings struct {
	memory.SoftDeleteEntity
	ClientIP               string     `json:"client_ip"`
	ClientName             string     `json:"client_name,omitempty"`
	Blocked                bool       `json:"blocked"` // IP Ban
	BlockReason            string     `json:"block_reason,omitempty"`
	BlockedAt              *time.Time `json:"blocked_at,omitempty"`
	BlockingEnabled        bool       `json:"blocking_enabled"`
	SafeBrowsingEnabled    bool       `json:"safe_browsing_enabled"`
	ParentalControlEnabled bool       `json:"parental_control_enabled"`
	CustomUpstreamDNS      string     `json:"custom_upstream_dns,omitempty"` // JSON array
	Tags                   string     `json:"tags,omitempty"`                // JSON array
}

// DNSClientDomainRule represents client-specific domain rules
type DNSClientDomainRule struct {
	memory.SoftDeleteEntity
	ClientIP   string `json:"client_ip"`
	Domain     string `json:"domain"`
	Type       string `json:"type"` // block, allow
	Comment    string `json:"comment,omitempty"`
	IsRegex    bool   `json:"is_regex"`
	IsWildcard bool   `json:"is_wildcard"`
}

// DNSRewrite represents DNS rewrite rules
type DNSRewrite struct {
	memory.SoftDeleteEntity
	Domain  string `json:"domain"`
	Answer  string `json:"answer"` // IP address or domain
	Type    string `json:"type"`   // A, AAAA, CNAME
	Comment string `json:"comment,omitempty"`
	Enabled bool   `json:"enabled"`
}

// GetUpstreamDNSList parses upstream DNS JSON array
func (c *DNSConfig) GetUpstreamDNSList() []string {
	var upstreams []string
	if c.UpstreamDNS != "" {
		json.Unmarshal([]byte(c.UpstreamDNS), &upstreams)
	}
	if len(upstreams) == 0 {
		// Default upstream DNS servers
		upstreams = []string{
			"94.140.14.14:53", // AdGuard DNS
			"94.140.15.15:53", // AdGuard DNS
			"1.1.1.1:53",      // Cloudflare
			"8.8.8.8:53",      // Google
		}
	}
	return upstreams
}

// SetUpstreamDNSList sets upstream DNS list as JSON
func (c *DNSConfig) SetUpstreamDNSList(upstreams []string) error {
	data, err := json.Marshal(upstreams)
	if err != nil {
		return err
	}
	c.UpstreamDNS = string(data)
	return nil
}

// GetDefaultBlocklists returns the default blocklists
func GetDefaultBlocklists() []DNSBlocklist {
	return []DNSBlocklist{
		{
			Name:           "AdGuard DNS filter",
			URL:            "https://adguardteam.github.io/HostlistsRegistry/assets/filter_1.txt",
			Enabled:        true,
			Format:         "auto",
			UpdateInterval: 3600,
			LastUpdated:    nil, // nil means it will be downloaded immediately on startup
			DomainCount:    0,
		},
		{
			Name:           "AdAway Default Blocklist",
			URL:            "https://adguardteam.github.io/HostlistsRegistry/assets/filter_2.txt",
			Enabled:        true,
			Format:         "auto",
			UpdateInterval: 3600,
			LastUpdated:    nil, // nil means it will be downloaded immediately on startup
			DomainCount:    0,
		},
	}
}
