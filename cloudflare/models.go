package cloudflare

import (
	"redock/platform/memory"
	"time"
)

// CloudflareAccount represents a Cloudflare account configuration
type CloudflareAccount struct {
	memory.SoftDeleteEntity
	Name      string `json:"name"`
	Email     string `json:"email"`
	APIKey    string `json:"api_key"`    // Encrypted
	APIToken  string `json:"api_token"`  // Encrypted (preferred over API Key)
	AccountID string `json:"account_id"`
	Enabled   bool   `json:"enabled"`
}

// CloudflareZone represents a Cloudflare zone (domain)
type CloudflareZone struct {
	memory.SoftDeleteEntity
	AccountID            uint      `json:"account_id"`
	ZoneID               string    `json:"zone_id"`              // Cloudflare zone ID
	Name                 string    `json:"name"`                 // Domain name
	Status               string    `json:"status"`               // active, pending, etc.
	Paused               bool      `json:"paused"`
	Type                 string    `json:"type"`                 // full, partial
	NameServers          string    `json:"name_servers"`         // JSON array
	OriginalNameServers  string    `json:"original_name_servers"` // JSON array
	OriginalRegistrar    string    `json:"original_registrar"`
	OriginalDNSHost      string    `json:"original_dns_host"`
	Plan                 string    `json:"plan"`                 // free, pro, business, enterprise
	LastSync             *time.Time `json:"last_sync"`
}

// CloudflareDNSRecord represents a DNS record in Cloudflare
type CloudflareDNSRecord struct {
	memory.SoftDeleteEntity
	ZoneID   string `json:"zone_id"`
	RecordID string `json:"record_id"` // Cloudflare record ID
	Type     string `json:"type"`      // A, AAAA, CNAME, MX, TXT, SRV, CAA, etc.
	Name     string `json:"name"`      // Full domain name
	Content  string `json:"content"`   // Record value
	TTL      int    `json:"ttl"`       // TTL in seconds (1 = auto)
	Priority int    `json:"priority"`  // For MX, SRV records
	Proxied  bool   `json:"proxied"`   // Orange cloud enabled
	Locked   bool   `json:"locked"`    // Locked from editing
	Comment  string `json:"comment"`
}

// CloudflareFirewallRule represents a firewall rule
type CloudflareFirewallRule struct {
	memory.SoftDeleteEntity
	ZoneID      string `json:"zone_id"`
	RuleID      string `json:"rule_id"`      // Cloudflare rule ID
	Description string `json:"description"`
	Expression  string `json:"expression"`   // Rule expression (e.g., ip.src eq 1.2.3.4)
	Action      string `json:"action"`       // block, challenge, js_challenge, allow, log
	Priority    int    `json:"priority"`
	Enabled     bool   `json:"enabled"`
	Products    string `json:"products"`     // JSON array (waf, rateLimit, etc.)
}

// CloudflarePageRule represents a page rule
type CloudflarePageRule struct {
	memory.SoftDeleteEntity
	ZoneID   string `json:"zone_id"`
	RuleID   string `json:"rule_id"`
	Targets  string `json:"targets"`  // JSON array of URL patterns
	Actions  string `json:"actions"`  // JSON array of actions
	Priority int    `json:"priority"`
	Status   string `json:"status"`   // active, disabled
}

// CloudflareZoneSettings represents various zone settings
type CloudflareZoneSettings struct {
	memory.SoftDeleteEntity
	ZoneID string `json:"zone_id"`
	
	// Security
	SecurityLevel        string `json:"security_level"`         // off, low, medium, high, under_attack
	SSL                  string `json:"ssl"`                    // off, flexible, full, strict
	AlwaysUseHTTPS       bool   `json:"always_use_https"`
	AutomaticHTTPSRewrites bool `json:"automatic_https_rewrites"`
	MinTLSVersion        string `json:"min_tls_version"`        // 1.0, 1.1, 1.2, 1.3
	OpportunisticEncryption bool `json:"opportunistic_encryption"`
	
	// Performance
	CacheLevel           string `json:"cache_level"`            // basic, simplified, aggressive
	BrowserCacheTTL      int    `json:"browser_cache_ttl"`
	AlwaysOnline         bool   `json:"always_online"`
	DevelopmentMode      bool   `json:"development_mode"`
	
	// Optimization
	MinifyHTML           bool   `json:"minify_html"`
	MinifyCSS            bool   `json:"minify_css"`
	MinifyJS             bool   `json:"minify_js"`
	Brotli               bool   `json:"brotli"`
	HTTP2                bool   `json:"http2"`
	HTTP3                bool   `json:"http3"`
	IPv6                 bool   `json:"ipv6"`
	WebSockets           bool   `json:"websockets"`
	
	// Other
	RocketLoader         bool   `json:"rocket_loader"`
	Mirage               bool   `json:"mirage"`
	Polish               string `json:"polish"`                 // off, lossless, lossy
	WebP                 bool   `json:"webp"`
	
	LastUpdated          *time.Time `json:"last_updated"`
}

// CloudflareAnalytics represents zone analytics
type CloudflareAnalytics struct {
	ZoneID           string    `json:"zone_id"`
	Date             time.Time `json:"date"`
	Requests         int64     `json:"requests"`
	Bandwidth        int64     `json:"bandwidth"`       // bytes
	CachedRequests   int64     `json:"cached_requests"`
	CachedBandwidth  int64     `json:"cached_bandwidth"`
	ThreatsBlocked   int64     `json:"threats_blocked"`
	PageViews        int64     `json:"page_views"`
	UniqueVisitors   int64     `json:"unique_visitors"`
}

// CloudflareEvent represents a security event
type CloudflareEvent struct {
	memory.SoftDeleteEntity
	ZoneID      string    `json:"zone_id"`
	EventID     string    `json:"event_id"`
	Timestamp   time.Time `json:"timestamp"`
	RayID       string    `json:"ray_id"`
	ClientIP    string    `json:"client_ip"`
	ClientCountry string  `json:"client_country"`
	Action      string    `json:"action"`        // block, challenge, etc.
	Source      string    `json:"source"`        // firewall, waf, rateLimit, etc.
	Description string    `json:"description"`
	URL         string    `json:"url"`
	UserAgent   string    `json:"user_agent"`
}
