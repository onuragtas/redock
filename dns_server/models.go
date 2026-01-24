package dns_server

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// DNSConfig holds DNS server configuration
type DNSConfig struct {
	ID                     uint           `gorm:"primarykey" json:"id"`
	CreatedAt              time.Time      `json:"created_at"`
	UpdatedAt              time.Time      `json:"updated_at"`
	DeletedAt              gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	Enabled                bool           `gorm:"default:false" json:"enabled"`
	UDPPort                int            `gorm:"default:53" json:"udp_port"`
	TCPPort                int            `gorm:"default:53" json:"tcp_port"`
	DoHEnabled             bool           `gorm:"default:false" json:"doh_enabled"`
	DoHPort                int            `gorm:"default:443" json:"doh_port"`
	DoTEnabled             bool           `gorm:"default:false" json:"dot_enabled"`
	DoTPort                int            `gorm:"default:853" json:"dot_port"`
	UpstreamDNS            string         `gorm:"type:text" json:"upstream_dns"` // JSON array
	BlockingEnabled        bool           `gorm:"default:true" json:"blocking_enabled"`
	QueryLogging           bool           `gorm:"default:true" json:"query_logging"`
	LogRetentionDays       int            `gorm:"default:7" json:"log_retention_days"`
	RateLimitEnabled       bool           `gorm:"default:false" json:"rate_limit_enabled"`
	RateLimitQPS           int            `gorm:"default:100" json:"rate_limit_qps"`
	CacheEnabled           bool           `gorm:"default:true" json:"cache_enabled"`
	CacheTTL               int            `gorm:"default:3600" json:"cache_ttl"` // seconds
	SafeBrowsingEnabled    bool           `gorm:"default:false" json:"safe_browsing_enabled"`
	ParentalControlEnabled bool           `gorm:"default:false" json:"parental_control_enabled"`
}

// DNSBlocklist represents a blocklist source
type DNSBlocklist struct {
	ID             uint           `gorm:"primarykey" json:"id"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	Name           string         `gorm:"not null;uniqueIndex" json:"name"`
	URL            string         `gorm:"not null" json:"url"`
	Enabled        bool           `gorm:"default:true" json:"enabled"`
	Format         string         `gorm:"default:''" json:"format"` // hosts, domains, adblock, auto (empty string means auto-detect)
	LastUpdated    *time.Time     `json:"last_updated,omitempty"`
	LastError      string         `gorm:"type:text" json:"last_error,omitempty"`
	DomainCount    int            `gorm:"default:0" json:"domain_count"`
	UpdateInterval int            `gorm:"default:86400" json:"update_interval"` // seconds
}

// DNSCustomFilter represents custom blocked or allowed domains
type DNSCustomFilter struct {
	ID         uint           `gorm:"primarykey" json:"id"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	Domain     string         `gorm:"not null;uniqueIndex" json:"domain"`
	Type       string         `gorm:"not null" json:"type"` // blacklist, whitelist
	Comment    string         `gorm:"type:text" json:"comment,omitempty"`
	IsRegex    bool           `gorm:"default:false" json:"is_regex"`
	IsWildcard bool           `gorm:"default:false" json:"is_wildcard"`
}

// DNSQueryLog represents a logged DNS query
type DNSQueryLog struct {
	ID           uint      `gorm:"primarykey" json:"id"`
	CreatedAt    time.Time `gorm:"index" json:"created_at"`
	ClientIP     string    `gorm:"index;not null" json:"client_ip"`
	Domain       string    `gorm:"index;not null" json:"domain"`
	QueryType    string    `gorm:"not null" json:"query_type"` // A, AAAA, CNAME, MX, etc.
	Response     string    `gorm:"type:text" json:"response"`
	Blocked      bool      `gorm:"index;default:false" json:"blocked"`
	BlockReason  string    `json:"block_reason,omitempty"`
	ResponseTime int       `json:"response_time"` // milliseconds
	Cached       bool      `gorm:"default:false" json:"cached"`
}

// DNSStatistics represents aggregated statistics
type DNSStatistics struct {
	ID                uint      `gorm:"primarykey" json:"id"`
	Date              time.Time `gorm:"uniqueIndex;not null" json:"date"`
	TotalQueries      int64     `gorm:"default:0" json:"total_queries"`
	BlockedQueries    int64     `gorm:"default:0" json:"blocked_queries"`
	CachedQueries     int64     `gorm:"default:0" json:"cached_queries"`
	AvgResponseTime   float64   `json:"avg_response_time"` // milliseconds
	UniqueClients     int       `gorm:"default:0" json:"unique_clients"`
	TopDomains        string    `gorm:"type:text" json:"top_domains"`         // JSON
	TopBlockedDomains string    `gorm:"type:text" json:"top_blocked_domains"` // JSON
	TopClients        string    `gorm:"type:text" json:"top_clients"`         // JSON
}

// DNSClientSettings represents per-client DNS settings
type DNSClientSettings struct {
	ID                     uint           `gorm:"primarykey" json:"id"`
	CreatedAt              time.Time      `json:"created_at"`
	UpdatedAt              time.Time      `json:"updated_at"`
	DeletedAt              gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	ClientIP               string         `gorm:"uniqueIndex;not null" json:"client_ip"`
	ClientName             string         `json:"client_name,omitempty"`
	Blocked                bool           `gorm:"default:false" json:"blocked"` // IP Ban
	BlockReason            string         `gorm:"type:text" json:"block_reason,omitempty"`
	BlockedAt              *time.Time     `json:"blocked_at,omitempty"`
	BlockingEnabled        bool           `gorm:"default:true" json:"blocking_enabled"`
	SafeBrowsingEnabled    bool           `gorm:"default:false" json:"safe_browsing_enabled"`
	ParentalControlEnabled bool           `gorm:"default:false" json:"parental_control_enabled"`
	CustomUpstreamDNS      string         `gorm:"type:text" json:"custom_upstream_dns,omitempty"` // JSON array
	Tags                   string         `gorm:"type:text" json:"tags,omitempty"`                // JSON array
}

// DNSClientDomainRule represents client-specific domain rules
type DNSClientDomainRule struct {
	ID         uint           `gorm:"primarykey" json:"id"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	ClientIP   string         `gorm:"not null;index" json:"client_ip"`
	Domain     string         `gorm:"not null;index" json:"domain"`
	Type       string         `gorm:"not null" json:"type"` // block, allow
	Comment    string         `gorm:"type:text" json:"comment,omitempty"`
	IsRegex    bool           `gorm:"default:false" json:"is_regex"`
	IsWildcard bool           `gorm:"default:false" json:"is_wildcard"`
}

// DNSRewrite represents DNS rewrite rules
type DNSRewrite struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	Domain    string         `gorm:"not null;uniqueIndex" json:"domain"`
	Answer    string         `gorm:"not null" json:"answer"` // IP address or domain
	Type      string         `gorm:"not null" json:"type"`   // A, AAAA, CNAME
	Comment   string         `gorm:"type:text" json:"comment,omitempty"`
	Enabled   bool           `gorm:"default:true" json:"enabled"`
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
			"1.1.1.1:53", // Cloudflare
			"8.8.8.8:53", // Google
			"9.9.9.9:53", // Quad9
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
