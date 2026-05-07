package api_gateway

import (
	"crypto/tls"
	"net"
	"net/http"
	"sync"
	"time"
)

// Service represents an upstream service that the gateway routes to
type Service struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Host        string            `json:"host"`
	Port        int               `json:"port"`
	Protocol    string            `json:"protocol"` // http, https, grpc
	Path        string            `json:"path"`     // base path for the service
	Retries     int               `json:"retries"`
	Timeout     int               `json:"timeout"` // overall upstream request timeout in seconds; 0 = inherit from global
	HealthCheck *HealthCheck      `json:"health_check,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"` // headers to add to requests
	Enabled     bool              `json:"enabled"`
}

// AuthHeader defines a required request header for auth type "header" (key must match value).
type AuthHeader struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Route represents a routing rule that maps incoming requests to an upstream pool
type Route struct {
	ID                   string            `json:"id"`
	Name                 string            `json:"name"`
	UpstreamID           string            `json:"upstream_id"`            // ID of the upstream pool to forward to
	Paths                []string          `json:"paths"`                  // URL paths to match
	Methods              []string          `json:"methods,omitempty"`      // HTTP methods to match (empty = all)
	Hosts                []string          `json:"hosts,omitempty"`        // Host headers to match (empty = all)
	Headers              map[string]string `json:"headers,omitempty"`      // Required headers to match
	StripPath            bool              `json:"strip_path"`             // Strip the matched path before forwarding
	PreserveHost         bool              `json:"preserve_host"`          // Forward original Host header
	HostRewrite          string            `json:"host_rewrite,omitempty"` // Override Host header when proxying
	Priority             int               `json:"priority"`               // Higher priority routes are matched first
	RateLimitEnabled     bool              `json:"rate_limit_enabled"`
	RateLimitRequests    int               `json:"rate_limit_requests"` // requests per window
	RateLimitWindow      int               `json:"rate_limit_window"`   // window in seconds
	AuthRequired         bool              `json:"auth_required"`
	AuthType             string            `json:"auth_type,omitempty"`     // basic, jwt, header
	AuthHeaders          []AuthHeader      `json:"auth_headers,omitempty"`   // required header key-value pairs when auth_type=header
	ObservabilityEnabled *bool             `json:"observability_enabled,omitempty"`
	CORS                 *CORSConfig       `json:"cors,omitempty"`             // CORS response headers for this route (incl. WebSocket)
	ResponseHeaders      map[string]string `json:"response_headers,omitempty"` // extra response headers for this route
	Timeout              int               `json:"timeout,omitempty"`          // overall upstream request timeout in seconds; 0 = inherit from service then global
	Enabled              bool              `json:"enabled"`
}

// HealthCheck represents health check configuration for a service
type HealthCheck struct {
	Path               string `json:"path"`
	Interval           int    `json:"interval"`            // in seconds
	Timeout            int    `json:"timeout"`             // in seconds
	HealthyThreshold   int    `json:"healthy_threshold"`   // number of successes before marking healthy
	UnhealthyThreshold int    `json:"unhealthy_threshold"` // number of failures before marking unhealthy
}

// UDPRoute maps a UDP listen port to a backend service (for UDP proxying).
type UDPRoute struct {
	ID         string `json:"id"`
	Name       string `json:"name,omitempty"`
	ListenPort int    `json:"listen_port"` // UDP port the gateway listens on
	ServiceID  string `json:"service_id"`  // ID of the backend service (Host:Port, Protocol=udp)
	Enabled    bool   `json:"enabled"`
}

// TCPRoute maps a TCP listen port to a backend service (raw TCP forwarding, e.g. for tunnel).
type TCPRoute struct {
	ID         string `json:"id"`
	Name       string `json:"name,omitempty"`
	ListenPort int    `json:"listen_port"` // TCP port the gateway listens on
	ServiceID  string `json:"service_id"`  // ID of the backend service (Host:Port)
	Enabled    bool   `json:"enabled"`
}

// CORSConfig holds CORS response header settings for the gateway
type CORSConfig struct {
	Enabled          bool     `json:"enabled"`
	AllowOrigins     []string `json:"allow_origins,omitempty"`     // e.g. ["*"] or ["https://app.example.com"]
	AllowMethods     []string `json:"allow_methods,omitempty"`     // e.g. ["GET","POST","PUT","DELETE","OPTIONS"]
	AllowHeaders     []string `json:"allow_headers,omitempty"`     // e.g. ["Content-Type","Authorization"]
	ExposeHeaders    []string `json:"expose_headers,omitempty"`   // headers exposed to the browser
	AllowCredentials bool     `json:"allow_credentials"`         // Access-Control-Allow-Credentials
	MaxAge           int      `json:"max_age"`                    // preflight cache in seconds (0 = no cache)
}

// GatewayConfig represents the overall gateway configuration
type GatewayConfig struct {
	ConfigVersion    int                   `json:"config_version"` // schema version; auto-migrated on load
	HTTPPort         int                   `json:"http_port"`
	HTTPSPort        int                   `json:"https_port"`
	HTTPSEnabled     bool                  `json:"https_enabled"`
	TLSCertFile      string                `json:"tls_cert_file,omitempty"`
	TLSKeyFile       string                `json:"tls_key_file,omitempty"`
	LetsEncrypt      *LetsEncryptConfig    `json:"lets_encrypt,omitempty"`
	Services         []Service             `json:"services"`
	Upstreams        []Upstream            `json:"upstreams,omitempty"`
	Routes           []Route               `json:"routes"`
	UDPRoutes        []UDPRoute            `json:"udp_routes,omitempty"`
	TCPRoutes        []TCPRoute            `json:"tcp_routes,omitempty"`
	GlobalRateLimit  *RateLimitConfig      `json:"global_rate_limit,omitempty"`
	LogLevel         string                `json:"log_level"`
	AccessLogEnabled bool                  `json:"access_log_enabled"`
	Observability    *ObservabilityConfig  `json:"observability,omitempty"`
	ClientSecurity   *ClientSecurityConfig `json:"client_security,omitempty"`
	Timeouts         *TimeoutsConfig       `json:"timeouts,omitempty"`
	Enabled          bool                  `json:"enabled"`
}

// Upstream is a pool of backend services with a load balancing strategy and
// optional session affinity. Routes forward to an Upstream rather than a
// single Service, which lets one route distribute traffic across many
// backends.
type Upstream struct {
	ID       string           `json:"id"`
	Name     string           `json:"name"`
	Strategy string           `json:"strategy"`           // round_robin (default) | weighted | random | least_conn
	Sticky   *StickyConfig    `json:"sticky,omitempty"`   // nil = no session affinity
	Targets  []UpstreamTarget `json:"targets"`
	Enabled  bool             `json:"enabled"`
}

// UpstreamTarget references a Service with an optional weight (default 1).
type UpstreamTarget struct {
	ServiceID string `json:"service_id"`
	Weight    int    `json:"weight,omitempty"` // > 0; defaults to 1 if missing
}

// StickyConfig pins a client to one target across requests.
//
// Modes:
//   - "ip_hash": stateless, hashes client IP (Rendezvous/HRW). Honors weights when Strategy=weighted.
//   - "cookie":  stateless cookie holds selected service ID; first request distributes via Strategy and sets cookie.
//   - "header":  stateless, hashes a header value just like ip_hash but using the given header.
//
// When Strategy is "weighted" combined with a hash-based sticky mode the
// distribution honors weights via weighted HRW; for any other strategy the
// hash distributes uniformly across healthy targets.
type StickyConfig struct {
	Mode       string `json:"mode"`                  // ip_hash | cookie | header
	CookieName string `json:"cookie_name,omitempty"` // for cookie mode (default: "redock_lb")
	HeaderName string `json:"header_name,omitempty"` // for header mode (e.g. "X-Session-Id")
	TTLSeconds int    `json:"ttl_seconds,omitempty"` // cookie max-age; 0 = session cookie
}

// TimeoutsConfig holds global default timeouts. Per-service / per-route
// timeouts override RequestTimeoutSec; the rest are transport/server level.
// Values are in seconds; 0 falls back to the built-in default.
type TimeoutsConfig struct {
	ServerReadSec        int `json:"server_read_seconds"`
	ServerWriteSec       int `json:"server_write_seconds"`
	ServerIdleSec        int `json:"server_idle_seconds"`
	ShutdownSec          int `json:"shutdown_seconds"`
	RequestTimeoutSec    int `json:"request_timeout_seconds"`
	UpstreamDialSec      int `json:"upstream_dial_seconds"`
	UpstreamKeepAliveSec int `json:"upstream_keep_alive_seconds"`
	UpstreamIdleConnSec  int `json:"upstream_idle_conn_seconds"`
	TLSHandshakeSec      int `json:"tls_handshake_seconds"`
	ExpectContinueSec    int `json:"expect_continue_seconds"`
	HealthCheckSec       int `json:"health_check_seconds"`
}

// ClientSecurityConfig toggles request tracking and auto-blocking behaviour
type ClientSecurityConfig struct {
	TrackingEnabled      bool                `json:"tracking_enabled"`
	TopClientLimit       int                 `json:"top_client_limit"`
	AutoBlockEnabled     bool                `json:"auto_block_enabled"`
	NoRouteThreshold     int                 `json:"no_route_threshold"`
	AutoBlockDurationSec int                 `json:"auto_block_duration_seconds"`
	ManualBlocks         []ManualBlockConfig `json:"manual_blocks,omitempty"`
}

// ManualBlockConfig persists manually blocked clients in configuration
type ManualBlockConfig struct {
	IP        string `json:"ip"`
	Reason    string `json:"reason,omitempty"`
	BlockedAt string `json:"blocked_at"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

// ObservabilityConfig represents configuration for sending telemetry data
type ObservabilityConfig struct {
	Enabled            bool                      `json:"enabled"`
	LokiEnabled        bool                      `json:"loki_enabled"`
	Loki               *LokiDatasourceConfig     `json:"loki,omitempty"`
	InfluxEnabled      bool                      `json:"influx_enabled"`
	InfluxDB           *InfluxDBDatasourceConfig `json:"influx,omitempty"`
	GraylogEnabled     bool                      `json:"graylog_enabled"`
	Graylog            *GraylogConfig            `json:"graylog,omitempty"`
	OTLPEnabled        bool                      `json:"otlp_enabled"`
	OTLPEndpoint       string                    `json:"otlp_endpoint,omitempty"`
	OTLPHeaders        map[string]string         `json:"otlp_headers,omitempty"`
	ClickHouseEnabled  bool                      `json:"clickhouse_enabled"`
	ClickHouseEndpoint string                    `json:"clickhouse_endpoint,omitempty"`
	ClickHouseDatabase string                    `json:"clickhouse_database,omitempty"`
	ClickHouseTable    string                    `json:"clickhouse_table,omitempty"`
	ClickHouseUsername string                    `json:"clickhouse_username,omitempty"`
	ClickHousePassword string                    `json:"clickhouse_password,omitempty"`
	BatchSize          int                       `json:"batch_size"`
	FlushInterval      int                       `json:"flush_interval"` // in seconds
}

// LokiDatasourceConfig holds Loki datasource details
type LokiDatasourceConfig struct {
	URL      string            `json:"url"`
	TenantID string            `json:"tenant_id,omitempty"`
	APIKey   string            `json:"api_key,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
}

// InfluxDBDatasourceConfig holds InfluxDB datasource details
type InfluxDBDatasourceConfig struct {
	URL    string `json:"url"`
	Org    string `json:"org"`
	Bucket string `json:"bucket"`
	Token  string `json:"token"`
}

// GraylogConfig holds Graylog endpoint details
type GraylogConfig struct {
	Endpoint     string            `json:"endpoint"`
	APIKey       string            `json:"api_key,omitempty"`
	APIKeyHeader string            `json:"api_key_header,omitempty"`
	StreamID     string            `json:"stream_id,omitempty"`
	ExtraFields  map[string]string `json:"extra_fields,omitempty"`
}

// LetsEncryptConfig represents Let's Encrypt certificate configuration
type LetsEncryptConfig struct {
	Enabled          bool     `json:"enabled"`
	Email            string   `json:"email"`
	Domains          []string `json:"domains"`
	Staging          bool     `json:"staging"`           // Use staging server for testing
	AutoRenew        bool     `json:"auto_renew"`        // Auto-renew before expiry
	RenewBeforeDays  int      `json:"renew_before_days"` // Days before expiry to renew
	LastRenewAt      string   `json:"last_renew_at,omitempty"`
	ExpiresAt        string   `json:"expires_at,omitempty"`
	CertificateReady bool     `json:"certificate_ready"`
}

// RateLimitConfig represents global rate limiting configuration
type RateLimitConfig struct {
	Enabled  bool `json:"enabled"`
	Requests int  `json:"requests"` // requests per window
	Window   int  `json:"window"`   // window in seconds
}

// ServiceHealth represents the health status of a service
type ServiceHealth struct {
	ServiceID    string    `json:"service_id"`
	Healthy      bool      `json:"healthy"`
	LastCheck    time.Time `json:"last_check"`
	SuccessCount int       `json:"success_count"`
	FailureCount int       `json:"failure_count"`
	ResponseTime int64     `json:"response_time_ms"`
	LastError    string    `json:"last_error,omitempty"`
}

// RequestLog represents an access log entry
type RequestLog struct {
	Timestamp             time.Time `json:"timestamp"`
	Method                string    `json:"method"`
	Path                  string    `json:"path"`
	Host                  string    `json:"host"`
	RemoteAddr            string    `json:"remote_addr"`
	RouteID               string    `json:"route_id"`
	RouteName             string    `json:"route_name,omitempty"`
	UpstreamID            string    `json:"upstream_id,omitempty"`
	ServiceID             string    `json:"service_id"`
	ServiceName           string    `json:"service_name,omitempty"`
	StatusCode            int       `json:"status_code"`
	Duration              int64     `json:"duration_ms"`
	BytesSent             int64     `json:"bytes_sent"`
	BytesReceived         int64     `json:"bytes_received"`
	RequestBody           string    `json:"request_body,omitempty"`
	ResponseBody          string    `json:"response_body,omitempty"`
	RequestBodyTruncated  bool      `json:"request_body_truncated,omitempty"`
	ResponseBodyTruncated bool      `json:"response_body_truncated,omitempty"`
	UserAgent             string    `json:"user_agent"`
	Error                 string    `json:"error,omitempty"`
}

// GatewayStats represents gateway statistics
type GatewayStats struct {
	TotalRequests  int64           `json:"total_requests"`
	TotalErrors    int64           `json:"total_errors"`
	Uptime         int64           `json:"uptime_seconds"`
	RequestsPerSec float64         `json:"requests_per_second"`
	AverageLatency float64         `json:"average_latency_ms"`
	ServiceStats   []ServiceStats  `json:"service_stats"`
	RateLimitStats RateLimitStats  `json:"rate_limit_stats"`
	TopClients     []ClientStats   `json:"top_clients,omitempty"`
	BlockedClients []BlockedClient `json:"blocked_clients,omitempty"`
}

// ServiceStats represents per-service statistics
type ServiceStats struct {
	ServiceID      string  `json:"service_id"`
	Requests       int64   `json:"requests"`
	Errors         int64   `json:"errors"`
	AverageLatency float64 `json:"average_latency_ms"`
}

// RateLimitStats represents rate limiting statistics
type RateLimitStats struct {
	TotalLimited int64 `json:"total_limited"`
	CurrentUsage int   `json:"current_usage"`
}

// ClientStats represents tracked metrics for an individual client IP
type ClientStats struct {
	IP                string    `json:"ip"`
	RequestCount      int64     `json:"request_count"`
	LastSeen          time.Time `json:"last_seen"`
	LastPath          string    `json:"last_path"`
	LastRouteID       string    `json:"last_route_id"`
	LastStatus        int       `json:"last_status"`
	ConsecutiveMisses int       `json:"consecutive_misses"`
	TotalMisses       int64     `json:"total_misses"`
	Blocked           bool      `json:"blocked"`
	BlockedUntil      time.Time `json:"blocked_until,omitempty"`
	BlockedReason     string    `json:"blocked_reason,omitempty"`
	ManualBlock       bool      `json:"manual_block"`
}

// BlockedClient describes a client currently blocked
type BlockedClient struct {
	IP           string    `json:"ip"`
	Manual       bool      `json:"manual"`
	BlockedAt    time.Time `json:"blocked_at"`
	BlockedUntil time.Time `json:"blocked_until,omitempty"`
	Reason       string    `json:"reason"`
}

// rateLimiter manages rate limiting for clients
type rateLimiter struct {
	mu       sync.RWMutex
	clients  map[string]*clientRateLimit
	requests int
	window   time.Duration
}

// clientRateLimit tracks rate limiting for a specific client
type clientRateLimit struct {
	requests  int
	windowEnd time.Time
}

// Gateway represents the API gateway server
type Gateway struct {
	config            *GatewayConfig
	httpServer        *http.Server
	httpsServer       *http.Server
	httpListener      net.Listener
	httpsListener     net.Listener
	services          map[string]*Service
	upstreams         map[string]*Upstream
	upstreamRuntimes  map[string]*upstreamRuntime
	upstreamRuntimeMu sync.Mutex
	routes            []*Route
	serviceHealth     map[string]*ServiceHealth
	rateLimiter       *rateLimiter
	globalLimiter     *rateLimiter
	stats             *gatewayStatsTracker
	mu                sync.RWMutex
	running           bool
	stopChan          chan struct{}
	workDir           string
	httpClient        *http.Client
	tlsConfig         *tls.Config
	routeCache        map[string]*cachedRoute
	routeCacheOrder   []string
	routeCacheLimit   int
	routeCacheTTL     time.Duration
	routeCacheMu      sync.RWMutex
	clientStats       map[string]*clientStatsTracker
	clientStatsLimit  int
	clientStatsMu     sync.RWMutex
	persistentBlocks  map[string]BlockedClient
	blockListMu       sync.Mutex
}

// gatewayStatsTracker tracks gateway statistics
type gatewayStatsTracker struct {
	mu            sync.RWMutex
	startTime     time.Time
	totalRequests int64
	totalErrors   int64
	totalLatency  int64
	serviceStats  map[string]*serviceStatsTracker
	rateLimited   int64
}

// serviceStatsTracker tracks per-service statistics
type serviceStatsTracker struct {
	requests     int64
	errors       int64
	totalLatency int64
}

// clientStatsTracker keeps in-memory metrics per client IP
type clientStatsTracker struct {
	ip                string
	requests          int64
	lastSeen          time.Time
	lastPath          string
	lastRouteID       string
	lastStatus        int
	consecutiveMisses int
	totalMisses       int64
	blockedUntil      time.Time
	blockedAt         time.Time
	manualBlocked     bool
	blockReason       string
}
