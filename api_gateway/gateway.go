package api_gateway

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	dockermanager "redock/docker-manager"
)

var (
	gateway     *Gateway
	gatewayLock sync.Mutex
)

const (
	defaultRouteCacheLimit   = 2048
	defaultRouteCacheTTL     = 30 * time.Second
	maxLoggedBodyBytes       = 4096
	defaultClientStatsLimit  = 2048
	defaultTopClientLimit    = 1000
	defaultNoRouteThreshold  = 5
	defaultAutoBlockDuration = 5 * time.Minute
)

func defaultClientSecurityConfig() *ClientSecurityConfig {
	return &ClientSecurityConfig{
		TrackingEnabled:      true,
		TopClientLimit:       defaultTopClientLimit,
		AutoBlockEnabled:     true,
		NoRouteThreshold:     defaultNoRouteThreshold,
		AutoBlockDurationSec: int(defaultAutoBlockDuration / time.Second),
	}
}

func (g *Gateway) ensureClientSecurityDefaults() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.ensureClientSecurityDefaultsLocked()
}

func (g *Gateway) ensureClientSecurityDefaultsLocked() {
	if g.config == nil {
		return
	}
	if g.config.ClientSecurity == nil {
		g.config.ClientSecurity = defaultClientSecurityConfig()
	}
	cfg := g.config.ClientSecurity
	if cfg.TopClientLimit <= 0 {
		cfg.TopClientLimit = defaultTopClientLimit
	}
	if cfg.NoRouteThreshold <= 0 {
		cfg.NoRouteThreshold = defaultNoRouteThreshold
	}
	if cfg.AutoBlockDurationSec <= 0 {
		cfg.AutoBlockDurationSec = int(defaultAutoBlockDuration / time.Second)
	}
}

func (g *Gateway) refreshClientSecurity() {
	g.ensureClientSecurityDefaults()
	g.applyManualBlocks()
}

func (g *Gateway) applyManualBlocks() {
	g.mu.RLock()
	var manualBlocks []ManualBlockConfig
	if g.config != nil && g.config.ClientSecurity != nil {
		manualBlocks = append(manualBlocks, g.config.ClientSecurity.ManualBlocks...)
	}
	g.mu.RUnlock()

	g.clientStatsMu.Lock()
	now := time.Now()
	for _, tracker := range g.clientStats {
		if tracker.manualBlocked {
			tracker.manualBlocked = false
			if tracker.blockedUntil.IsZero() || tracker.blockedUntil.Before(now) {
				tracker.blockedUntil = time.Time{}
				tracker.blockReason = ""
				tracker.blockedAt = time.Time{}
			}
		}
	}

	applied := make([]BlockedClient, 0, len(manualBlocks))
	for _, entry := range manualBlocks {
		if entry.IP == "" {
			continue
		}
		tracker := g.getOrCreateClientTrackerLocked(entry.IP)
		tracker.manualBlocked = true
		tracker.blockReason = entry.Reason
		if tracker.blockReason == "" {
			tracker.blockReason = "manually blocked"
		}
		tracker.blockedAt = parseTimeOrNow(entry.BlockedAt)
		if entry.ExpiresAt != "" {
			tracker.blockedUntil = parseTime(entry.ExpiresAt)
		} else {
			tracker.blockedUntil = time.Time{}
		}
		applied = append(applied, BlockedClient{
			IP:           entry.IP,
			Manual:       true,
			BlockedAt:    tracker.blockedAt,
			BlockedUntil: tracker.blockedUntil,
			Reason:       tracker.blockReason,
		})
	}
	g.clientStatsMu.Unlock()

	for _, entry := range applied {
		g.persistBlockEntry(entry)
	}
}

func parseTimeOrNow(value string) time.Time {
	if t := parseTime(value); !t.IsZero() {
		return t
	}
	return time.Now()
}

func parseTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return t
}

func (g *Gateway) blockListFilePath() string {
	return filepath.Join(g.workDir, "data", "api_gateway_blocks.json")
}

func (g *Gateway) loadBlockList() {
	path := g.blockListFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			g.blockListMu.Lock()
			g.persistentBlocks = make(map[string]BlockedClient)
			g.blockListMu.Unlock()
		}
		return
	}

	var entries []BlockedClient
	if err := json.Unmarshal(data, &entries); err != nil {
		log.Printf("API Gateway: failed to parse block list: %v", err)
		return
	}

	now := time.Now()
	valid := make([]BlockedClient, 0, len(entries))
	for _, entry := range entries {
		if entry.IP == "" {
			continue
		}
		if !entry.BlockedUntil.IsZero() && entry.BlockedUntil.Before(now) && !entry.Manual {
			continue
		}
		valid = append(valid, entry)
	}

	g.blockListMu.Lock()
	g.persistentBlocks = make(map[string]BlockedClient, len(valid))
	for _, entry := range valid {
		g.persistentBlocks[entry.IP] = entry
	}
	g.blockListMu.Unlock()

	g.applyPersistentBlocks(valid)
	if len(valid) != len(entries) {
		g.writeBlockList(valid)
	}
}

func (g *Gateway) applyPersistentBlocks(entries []BlockedClient) {
	if len(entries) == 0 {
		return
	}
	g.clientStatsMu.Lock()
	defer g.clientStatsMu.Unlock()
	for _, entry := range entries {
		tracker := g.getOrCreateClientTrackerLocked(entry.IP)
		tracker.blockedAt = entry.BlockedAt
		tracker.blockedUntil = entry.BlockedUntil
		tracker.blockReason = entry.Reason
		tracker.manualBlocked = entry.Manual
	}
}

func (g *Gateway) persistBlockEntry(entry BlockedClient) {
	if entry.IP == "" {
		return
	}
	g.blockListMu.Lock()
	if g.persistentBlocks == nil {
		g.persistentBlocks = make(map[string]BlockedClient)
	}
	g.persistentBlocks[entry.IP] = entry
	entries := g.blockMapToSliceLocked()
	g.blockListMu.Unlock()
	g.writeBlockList(entries)
}

func (g *Gateway) removePersistentBlock(ip string) {
	if ip == "" {
		return
	}
	g.blockListMu.Lock()
	if g.persistentBlocks == nil {
		g.blockListMu.Unlock()
		return
	}
	if _, ok := g.persistentBlocks[ip]; !ok {
		g.blockListMu.Unlock()
		return
	}
	delete(g.persistentBlocks, ip)
	entries := g.blockMapToSliceLocked()
	g.blockListMu.Unlock()
	g.writeBlockList(entries)
}

func (g *Gateway) blockMapToSliceLocked() []BlockedClient {
	entries := make([]BlockedClient, 0, len(g.persistentBlocks))
	for _, entry := range g.persistentBlocks {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Manual == entries[j].Manual {
			return entries[i].IP < entries[j].IP
		}
		return entries[i].Manual && !entries[j].Manual
	})
	return entries
}

func (g *Gateway) writeBlockList(entries []BlockedClient) {
	path := g.blockListFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		log.Printf("API Gateway: failed to create block list directory: %v", err)
		return
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		log.Printf("API Gateway: failed to encode block list: %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		log.Printf("API Gateway: failed to write block list: %v", err)
	}
}

type cachedRoute struct {
	route     *Route
	expiresAt time.Time
}

// Init initializes the API Gateway
func Init(dm *dockermanager.DockerEnvironmentManager) {
	gateway = NewGateway(dm.GetWorkDir())
}

// GetGateway returns the singleton gateway instance
func GetGateway() *Gateway {
	return gateway
}

// NewGateway creates a new Gateway instance
func NewGateway(workDir string) *Gateway {
	g := &Gateway{
		services:      make(map[string]*Service),
		serviceHealth: make(map[string]*ServiceHealth),
		stopChan:      make(chan struct{}),
		workDir:       workDir,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		stats: &gatewayStatsTracker{
			startTime:    time.Now(),
			serviceStats: make(map[string]*serviceStatsTracker),
		},
		routeCache:       make(map[string]*cachedRoute),
		routeCacheLimit:  defaultRouteCacheLimit,
		routeCacheTTL:    defaultRouteCacheTTL,
		clientStats:      make(map[string]*clientStatsTracker),
		clientStatsLimit: defaultClientStatsLimit,
		persistentBlocks: make(map[string]BlockedClient),
	}
	g.loadConfig()
	return g
}

// loadConfig loads the gateway configuration from file
func (g *Gateway) loadConfig() {
	configPath := g.workDir + "/data/api_gateway.json"
	file, err := os.ReadFile(configPath)
	if err != nil {
		// Default configuration
		g.config = &GatewayConfig{
			HTTPPort:         8080,
			HTTPSPort:        8081,
			HTTPSEnabled:     false,
			LogLevel:         "info",
			AccessLogEnabled: true,
			Enabled:          false,
			Services:         []Service{},
			Routes:           []Route{},
			UDPRoutes:        []UDPRoute{},
			TCPRoutes:        []TCPRoute{},
			ClientSecurity:   defaultClientSecurityConfig(),
		}
		g.refreshClientSecurity()
		g.loadBlockList()
		return
	}

	var config GatewayConfig
	if err := json.Unmarshal(file, &config); err != nil {
		log.Printf("API Gateway: Error parsing config: %v", err)
		g.config = &GatewayConfig{
			HTTPPort:         8080,
			HTTPSPort:        8081,
			HTTPSEnabled:     false,
			LogLevel:         "info",
			AccessLogEnabled: true,
			Enabled:          false,
			Services:         []Service{},
			Routes:           []Route{},
			UDPRoutes:        []UDPRoute{},
			TCPRoutes:        []TCPRoute{},
		}
		g.refreshClientSecurity()
		g.loadBlockList()
		return
	}

	g.config = &config
	g.refreshClientSecurity()
	g.loadBlockList()
	g.refreshServicesAndRoutes()
}

// SaveConfig saves the gateway configuration to file
func (g *Gateway) SaveConfig() error {
	g.mu.RLock()
	defer g.mu.RUnlock()

	return g.saveConfigLocked()
}

// saveConfigLocked saves the configuration without acquiring a lock (caller must hold lock)
func (g *Gateway) saveConfigLocked() error {
	configPath := g.workDir + "/data/api_gateway.json"
	data, err := json.MarshalIndent(g.config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

// refreshServicesAndRoutes refreshes internal maps from config
func (g *Gateway) refreshServicesAndRoutes() {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Rebuild services map
	g.services = make(map[string]*Service)
	for i := range g.config.Services {
		svc := &g.config.Services[i]
		g.services[svc.ID] = svc
	}

	// Rebuild routes slice and sort by priority
	g.routes = make([]*Route, len(g.config.Routes))
	for i := range g.config.Routes {
		g.routes[i] = &g.config.Routes[i]
	}
	sort.Slice(g.routes, func(i, j int) bool {
		return g.routes[i].Priority > g.routes[j].Priority
	})

	// Initialize rate limiters
	if g.config.GlobalRateLimit != nil && g.config.GlobalRateLimit.Enabled {
		g.globalLimiter = &rateLimiter{
			clients:  make(map[string]*clientRateLimit),
			requests: g.config.GlobalRateLimit.Requests,
			window:   time.Duration(g.config.GlobalRateLimit.Window) * time.Second,
		}
	}

	g.rateLimiter = &rateLimiter{
		clients: make(map[string]*clientRateLimit),
	}

	g.clearRouteCache()
}

// GetConfig returns the current gateway configuration
func (g *Gateway) GetConfig() *GatewayConfig {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.config
}

// GetConfigCopy returns a deep copy of the current config (for building merged config then UpdateConfig).
func (g *Gateway) GetConfigCopy() *GatewayConfig {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.config == nil {
		return nil
	}
	data, err := json.Marshal(g.config)
	if err != nil {
		return nil
	}
	var copy GatewayConfig
	if err := json.Unmarshal(data, &copy); err != nil {
		return nil
	}
	return &copy
}

// UpdateConfig updates the gateway configuration
func (g *Gateway) UpdateConfig(config *GatewayConfig) error {
	gatewayLock.Lock()
	defer gatewayLock.Unlock()

	wasRunning := g.running
	if wasRunning {
		if err := g.stopLocked(); err != nil {
			return fmt.Errorf("failed to stop gateway: %w", err)
		}
	}

	g.mu.Lock()
	g.config = config
	g.mu.Unlock()

	g.refreshServicesAndRoutes()
	g.refreshClientSecurity()

	if err := g.SaveConfig(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	if wasRunning && config.Enabled {
		if err := g.startLocked(); err != nil {
			return fmt.Errorf("failed to restart gateway: %w", err)
		}
	}

	return nil
}

// Start starts the API Gateway servers
func (g *Gateway) Start() error {
	gatewayLock.Lock()
	defer gatewayLock.Unlock()

	return g.startLocked()
}

func (g *Gateway) startLocked() error {
	if g.running {
		return fmt.Errorf("gateway is already running")
	}

	g.refreshServicesAndRoutes()
	g.refreshClientSecurity()
	g.stopChan = make(chan struct{})

	// Create HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/", g.handleRequest)

	g.httpServer = &http.Server{
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start HTTP listener
	httpAddr := fmt.Sprintf(":%d", g.config.HTTPPort)
	var err error
	g.httpListener, err = net.Listen("tcp", httpAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", httpAddr, err)
	}

	go func() {
		log.Printf("API Gateway: HTTP server listening on %s", httpAddr)
		if err := g.httpServer.Serve(g.httpListener); err != nil && err != http.ErrServerClosed {
			log.Printf("API Gateway: HTTP server error: %v", err)
		}
	}()

	// Start HTTPS if enabled
	if g.config.HTTPSEnabled && g.config.TLSCertFile != "" && g.config.TLSKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(g.config.TLSCertFile, g.config.TLSKeyFile)
		if err != nil {
			log.Printf("API Gateway: Failed to load TLS certificates: %v", err)
		} else {
			g.tlsConfig = &tls.Config{
				Certificates: []tls.Certificate{cert},
				MinVersion:   tls.VersionTLS12,
			}

			g.httpsServer = &http.Server{
				Handler:      mux,
				TLSConfig:    g.tlsConfig,
				ReadTimeout:  30 * time.Second,
				WriteTimeout: 30 * time.Second,
				IdleTimeout:  60 * time.Second,
			}

			httpsAddr := fmt.Sprintf(":%d", g.config.HTTPSPort)
			g.httpsListener, err = tls.Listen("tcp", httpsAddr, g.tlsConfig)
			if err != nil {
				log.Printf("API Gateway: Failed to listen on %s: %v", httpsAddr, err)
			} else {
				go func() {
					log.Printf("API Gateway: HTTPS server listening on %s", httpsAddr)
					if err := g.httpsServer.Serve(g.httpsListener); err != nil && err != http.ErrServerClosed {
						log.Printf("API Gateway: HTTPS server error: %v", err)
					}
				}()
			}
		}
	}

	// Start UDP route listeners
	for _, r := range g.config.UDPRoutes {
		if r.Enabled {
			go g.runUDPRoute(r, g.stopChan)
		}
	}

	// Start TCP route listeners
	for _, r := range g.config.TCPRoutes {
		if r.Enabled {
			go g.runTCPRoute(r, g.stopChan)
		}
	}

	// Start health checks
	go g.runHealthChecks()

	g.running = true
	g.mu.Lock()
	g.config.Enabled = true
	g.mu.Unlock()
	g.SaveConfig()

	log.Println("API Gateway: Started successfully")
	return nil
}

// Stop stops the API Gateway servers
func (g *Gateway) Stop() error {
	gatewayLock.Lock()
	defer gatewayLock.Unlock()

	return g.stopLocked()
}

func (g *Gateway) stopLocked() error {
	if !g.running {
		return nil
	}

	close(g.stopChan)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if g.httpServer != nil {
		if err := g.httpServer.Shutdown(ctx); err != nil {
			log.Printf("API Gateway: HTTP server shutdown error: %v", err)
		}
		g.httpServer = nil
		g.httpListener = nil
	}

	if g.httpsServer != nil {
		if err := g.httpsServer.Shutdown(ctx); err != nil {
			log.Printf("API Gateway: HTTPS server shutdown error: %v", err)
		}
		g.httpsServer = nil
		g.httpsListener = nil
	}

	g.running = false
	g.mu.Lock()
	g.config.Enabled = false
	g.mu.Unlock()
	g.SaveConfig()

	log.Println("API Gateway: Stopped successfully")
	return nil
}

// IsRunning returns whether the gateway is running
func (g *Gateway) IsRunning() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.running
}

// handleRequest handles incoming HTTP requests
func (g *Gateway) handleRequest(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	lw := newLoggingResponseWriter(w, maxLoggedBodyBytes)
	reqInfo, err := captureRequestBody(r, maxLoggedBodyBytes)
	if err != nil {
		g.recordError()
		status := http.StatusBadRequest
		http.Error(lw, "Invalid request body", status)
		g.logRequest(r, status, startTime, "", "", "", "", true, "failed to read request body", reqInfo, lw.LogInfo())
		return
	}

	clientIP := getClientIP(r)
	trackClient := clientIP != ""
	statusCode := http.StatusOK
	routeID := ""
	routeName := ""
	serviceID := ""
	serviceName := ""
	matchedRoute := false

	defer func() {
		if trackClient {
			g.trackClientActivity(clientIP, r.URL.Path, routeID, statusCode, matchedRoute)
		}
	}()

	if blocked, reason := g.isClientBlocked(clientIP); blocked {
		statusCode = http.StatusForbidden
		message := reason
		if message == "" {
			message = "Client blocked"
		}
		http.Error(lw, message, statusCode)
		g.logRequest(r, statusCode, startTime, "", "", "", "", true, message, reqInfo, lw.LogInfo())
		trackClient = false
		return
	}

	// Handle ACME challenges for Let's Encrypt
	if HandleACMEChallenge(lw, r) {
		statusCode = lw.StatusCode()
		g.logRequest(r, statusCode, startTime, "", "", "", "", true, "", reqInfo, lw.LogInfo())
		trackClient = false
		return
	}

	// Update stats
	g.stats.mu.Lock()
	g.stats.totalRequests++
	g.stats.mu.Unlock()

	// Check global rate limit
	if g.globalLimiter != nil {
		if !g.checkRateLimit(g.globalLimiter, clientIP) {
			g.recordError()
			g.recordRateLimited()
			statusCode = http.StatusTooManyRequests
			http.Error(lw, "Rate limit exceeded", statusCode)
			g.logRequest(r, statusCode, startTime, "", "", "", "", true, "rate limit exceeded", reqInfo, lw.LogInfo())
			return
		}
	}

	// Find matching route
	route := g.matchRoute(r)
	if route == nil {
		g.recordError()
		statusCode = http.StatusNotFound
		http.Error(lw, "Not Found", statusCode)
		g.logRequest(r, statusCode, startTime, "", "", "", "", true, "no matching route", reqInfo, lw.LogInfo())
		return
	}
	matchedRoute = true
	routeID = route.ID
	routeName = route.Name

	routeObservability := g.isRouteObservabilityEnabled(route)

	// Check route-level rate limit
	if route.RateLimitEnabled && route.RateLimitRequests > 0 {
		routeLimiter := &rateLimiter{
			clients:  make(map[string]*clientRateLimit),
			requests: route.RateLimitRequests,
			window:   time.Duration(route.RateLimitWindow) * time.Second,
		}
		if !g.checkRateLimit(routeLimiter, clientIP) {
			g.recordError()
			g.recordRateLimited()
			statusCode = http.StatusTooManyRequests
			http.Error(lw, "Rate limit exceeded", statusCode)
			g.logRequest(r, statusCode, startTime, routeID, routeName, "", "", routeObservability, "rate limit exceeded", reqInfo, lw.LogInfo())
			return
		}
	}

	// Check authentication if required
	if route.AuthRequired {
		if !g.checkAuth(r, route) {
			g.recordError()
			statusCode = http.StatusUnauthorized
			http.Error(lw, "Unauthorized", statusCode)
			g.logRequest(r, statusCode, startTime, routeID, routeName, "", "", routeObservability, "authentication failed", reqInfo, lw.LogInfo())
			return
		}
	}

	// Get service
	g.mu.RLock()
	service := g.services[route.ServiceID]
	g.mu.RUnlock()

	if service == nil || !service.Enabled {
		g.recordError()
		statusCode = http.StatusServiceUnavailable
		http.Error(lw, "Service Unavailable", statusCode)
		g.logRequest(r, statusCode, startTime, routeID, routeName, route.ServiceID, serviceName, routeObservability, "service not available", reqInfo, lw.LogInfo())
		return
	}
	serviceID = service.ID
	serviceName = service.Name

	// Check service health
	g.mu.RLock()
	health := g.serviceHealth[serviceID]
	g.mu.RUnlock()

	if health != nil && !health.Healthy {
		g.recordError()
		statusCode = http.StatusServiceUnavailable
		http.Error(lw, "Service Unavailable", statusCode)
		g.logRequest(r, statusCode, startTime, routeID, routeName, serviceID, serviceName, routeObservability, "service unhealthy", reqInfo, lw.LogInfo())
		return
	}

	// Update service stats
	g.stats.mu.Lock()
	if g.stats.serviceStats[service.ID] == nil {
		g.stats.serviceStats[service.ID] = &serviceStatsTracker{}
	}
	g.stats.serviceStats[service.ID].requests++
	g.stats.mu.Unlock()

	// Proxy the request
	err = g.proxyRequest(lw, r, route, service)
	statusCode = lw.StatusCode()
	if err != nil {
		g.recordServiceError(serviceID)
		g.logRequest(r, statusCode, startTime, routeID, routeName, serviceID, serviceName, routeObservability, err.Error(), reqInfo, lw.LogInfo())
	} else {
		g.logRequest(r, statusCode, startTime, routeID, routeName, serviceID, serviceName, routeObservability, "", reqInfo, lw.LogInfo())
	}

	// Record latency
	latency := time.Since(startTime).Milliseconds()
	g.stats.mu.Lock()
	g.stats.totalLatency += latency
	if g.stats.serviceStats[serviceID] != nil {
		g.stats.serviceStats[serviceID].totalLatency += latency
	}
	g.stats.mu.Unlock()
}

// matchRoute finds the first matching route for the request
func (g *Gateway) matchRoute(r *http.Request) *Route {
	cacheKey := g.buildRouteCacheKey(r)
	if cached := g.getRouteFromCache(cacheKey); cached != nil {
		return cached
	}

	g.mu.RLock()
	defer g.mu.RUnlock()

	var bestRoute *Route
	bestPathScore := -1

	for _, route := range g.routes {
		if !route.Enabled {
			continue
		}

		// Check methods
		if len(route.Methods) > 0 {
			methodMatch := false
			for _, m := range route.Methods {
				if strings.EqualFold(m, r.Method) {
					methodMatch = true
					break
				}
			}
			if !methodMatch {
				continue
			}
		}

		// Check hosts
		if len(route.Hosts) > 0 {
			hostMatch := false
			requestHost := normalizeHost(r.Host)
			for _, h := range route.Hosts {
				if matchWildcard(h, requestHost) {
					hostMatch = true
					break
				}
			}
			if !hostMatch {
				continue
			}
		}

		// Check paths and compute specificity score
		score := pathMatchScore(route.Paths, r.URL.Path)
		if score < 0 {
			continue
		}

		// Check required headers
		if len(route.Headers) > 0 {
			headersMatch := true
			for key, value := range route.Headers {
				if r.Header.Get(key) != value {
					headersMatch = false
					break
				}
			}
			if !headersMatch {
				continue
			}
		}

		// Select the most specific match, falling back to priority order
		if bestRoute == nil || route.Priority > bestRoute.Priority || (route.Priority == bestRoute.Priority && score > bestPathScore) {
			bestRoute = route
			bestPathScore = score
		}
	}

	if bestRoute != nil {
		g.storeRouteInCache(cacheKey, bestRoute)
	}

	return bestRoute
}

// matchPath checks if a request path matches a route path pattern
func matchPath(pattern, path string) bool {
	// Exact match
	if pattern == path {
		return true
	}

	// Prefix match (pattern ends with *)
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(path, prefix)
	}

	// Prefix match (pattern ends with /)
	if strings.HasSuffix(pattern, "/") {
		return strings.HasPrefix(path, pattern)
	}

	// Check if path starts with pattern
	return strings.HasPrefix(path, pattern+"/") || path == pattern
}

func normalizeHost(host string) string {
	if idx := strings.Index(host, ":"); idx != -1 {
		return host[:idx]
	}
	return host
}

// pathMatchScore returns the highest specificity score for the provided patterns
// or -1 if none of the patterns match the incoming path.
func pathMatchScore(patterns []string, requestPath string) int {
	best := -1
	for _, p := range patterns {
		if matchPath(p, requestPath) {
			score := pathSpecificity(p)
			if score > best {
				best = score
			}
		}
	}
	return best
}

// pathSpecificity approximates how "specific" a route path is so more detailed
// definitions (like /api/users) win over generic ones (like /).
func pathSpecificity(pattern string) int {
	clean := strings.TrimSuffix(pattern, "*")
	if clean == "" {
		clean = pattern
	}
	score := len(clean)
	if !strings.HasSuffix(pattern, "*") {
		// Reward exact/prefix matches without wildcards so they outrank catch-alls.
		score += 10
	}
	return score
}

func (g *Gateway) buildRouteCacheKey(r *http.Request) string {
	method := strings.ToUpper(r.Method)
	host := normalizeHost(r.Host)
	path := r.URL.Path
	return method + "|" + host + "|" + path
}

func (g *Gateway) getRouteFromCache(key string) *Route {
	if g.routeCacheTTL <= 0 || g.routeCacheLimit <= 0 {
		return nil
	}

	g.routeCacheMu.RLock()
	entry, ok := g.routeCache[key]
	g.routeCacheMu.RUnlock()
	if !ok || entry == nil {
		return nil
	}
	if entry.route == nil || time.Now().After(entry.expiresAt) || !entry.route.Enabled {
		g.removeRouteCacheKey(key)
		return nil
	}
	return entry.route
}

func (g *Gateway) storeRouteInCache(key string, route *Route) {
	if route == nil || g.routeCacheTTL <= 0 || g.routeCacheLimit <= 0 {
		return
	}

	g.routeCacheMu.Lock()
	defer g.routeCacheMu.Unlock()
	g.routeCache[key] = &cachedRoute{
		route:     route,
		expiresAt: time.Now().Add(g.routeCacheTTL),
	}
	g.routeCacheOrder = append(g.routeCacheOrder, key)
	g.pruneRouteCacheLocked()
}

func (g *Gateway) removeRouteCacheKey(key string) {
	g.routeCacheMu.Lock()
	defer g.routeCacheMu.Unlock()
	g.removeRouteCacheKeyLocked(key)
}

func (g *Gateway) removeRouteCacheKeyLocked(key string) {
	delete(g.routeCache, key)
	for i, cachedKey := range g.routeCacheOrder {
		if cachedKey == key {
			g.routeCacheOrder = append(g.routeCacheOrder[:i], g.routeCacheOrder[i+1:]...)
			break
		}
	}
}

func (g *Gateway) pruneRouteCacheLocked() {
	if g.routeCacheLimit <= 0 {
		g.routeCache = make(map[string]*cachedRoute)
		g.routeCacheOrder = nil
		return
	}
	for len(g.routeCacheOrder) > g.routeCacheLimit {
		oldest := g.routeCacheOrder[0]
		g.routeCacheOrder = g.routeCacheOrder[1:]
		delete(g.routeCache, oldest)
	}
}

func (g *Gateway) clearRouteCache() {
	g.routeCacheMu.Lock()
	defer g.routeCacheMu.Unlock()
	g.routeCache = make(map[string]*cachedRoute)
	g.routeCacheOrder = g.routeCacheOrder[:0]
}

// matchWildcard matches a host pattern with wildcards
func matchWildcard(pattern, value string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:] // e.g., ".example.com"
		return strings.HasSuffix(value, suffix) || value == pattern[2:]
	}
	return pattern == value
}

// proxyRequest forwards the request to the upstream service
func (g *Gateway) proxyRequest(w http.ResponseWriter, r *http.Request, route *Route, service *Service) error {
	// Build target URL
	protocol := service.Protocol
	if protocol == "" {
		protocol = "http"
	}

	targetURL := fmt.Sprintf("%s://%s:%d", protocol, service.Host, service.Port)
	target, err := url.Parse(targetURL)
	if err != nil {
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return err
	}

	// Create reverse proxy
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Configure transport with default timeout if not specified
	dialTimeout := time.Duration(service.Timeout) * time.Second
	if dialTimeout <= 0 {
		dialTimeout = 30 * time.Second
	}
	proxy.Transport = &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   dialTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	// Determine the incoming protocol
	incomingProto := "http"
	if r.TLS != nil {
		incomingProto = "https"
	}

	// Modify the request
	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host

		// Handle path transformation
		originalPath := req.URL.Path
		if route.StripPath {
			for _, p := range route.Paths {
				stripped := strings.TrimSuffix(p, "*")
				stripped = strings.TrimSuffix(stripped, "/")
				if strings.HasPrefix(originalPath, stripped) {
					originalPath = strings.TrimPrefix(originalPath, stripped)
					if originalPath == "" {
						originalPath = "/"
					}
					break
				}
			}
		}

		if service.Path != "" {
			req.URL.Path = strings.TrimSuffix(service.Path, "/") + originalPath
		} else {
			req.URL.Path = originalPath
		}

		// Set host header
		targetHost := target.Host
		if route.HostRewrite != "" {
			targetHost = route.HostRewrite
		}
		if !route.PreserveHost {
			req.Host = targetHost
		}

		// Add service headers
		for key, value := range service.Headers {
			req.Header.Set(key, value)
		}

		// Add X-Forwarded headers
		if clientIP := getClientIP(r); clientIP != "" {
			req.Header.Set("X-Forwarded-For", clientIP)
		}
		req.Header.Set("X-Forwarded-Proto", incomingProto)
		req.Header.Set("X-Forwarded-Host", r.Host)
	}

	var proxyErr error
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		proxyErr = fmt.Errorf("proxy error: %w", err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}

	proxy.ServeHTTP(w, r)
	return proxyErr
}

// checkRateLimit checks if a client has exceeded the rate limit
func (g *Gateway) checkRateLimit(limiter *rateLimiter, clientIP string) bool {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	now := time.Now()

	client, exists := limiter.clients[clientIP]
	if !exists || now.After(client.windowEnd) {
		limiter.clients[clientIP] = &clientRateLimit{
			requests:  1,
			windowEnd: now.Add(limiter.window),
		}
		return true
	}

	if client.requests >= limiter.requests {
		return false
	}

	client.requests++
	return true
}

// checkAuth verifies authentication for the request
func (g *Gateway) checkAuth(r *http.Request, route *Route) bool {
	switch route.AuthType {
	case "basic":
		username, password, ok := r.BasicAuth()
		if !ok {
			return false
		}
		// For now, just check that credentials are provided
		// In a real implementation, this would validate against a database
		return username != "" && password != ""

	case "jwt":
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			return false
		}
		// Check if it starts with "Bearer "
		if !strings.HasPrefix(authHeader, "Bearer ") {
			return false
		}
		// For now, just check that a token is provided
		// In a real implementation, this would validate the JWT
		token := strings.TrimPrefix(authHeader, "Bearer ")
		return token != ""

	case "api-key":
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			apiKey = r.URL.Query().Get("api_key")
		}
		// For now, just check that an API key is provided
		// In a real implementation, this would validate against a database
		return apiKey != ""

	default:
		return true
	}
}

// runHealthChecks runs periodic health checks on services
func (g *Gateway) runHealthChecks() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-g.stopChan:
			return
		case <-ticker.C:
			g.checkAllServices()
		}
	}
}

// checkAllServices performs health checks on all services
func (g *Gateway) checkAllServices() {
	g.mu.RLock()
	services := make([]*Service, 0, len(g.services))
	for _, svc := range g.services {
		if svc.Enabled && svc.HealthCheck != nil {
			services = append(services, svc)
		}
	}
	g.mu.RUnlock()

	for _, svc := range services {
		go g.checkServiceHealth(svc)
	}
}

// checkServiceHealth performs a health check on a single service
func (g *Gateway) checkServiceHealth(service *Service) {
	protocol := service.Protocol
	if protocol == "" {
		protocol = "http"
	}

	healthURL := fmt.Sprintf("%s://%s:%d%s", protocol, service.Host, service.Port, service.HealthCheck.Path)

	timeout := time.Duration(service.HealthCheck.Timeout) * time.Second
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	client := &http.Client{Timeout: timeout}

	startTime := time.Now()
	resp, err := client.Get(healthURL)
	responseTime := time.Since(startTime).Milliseconds()

	g.mu.Lock()
	defer g.mu.Unlock()

	if g.serviceHealth[service.ID] == nil {
		g.serviceHealth[service.ID] = &ServiceHealth{
			ServiceID: service.ID,
			Healthy:   true,
		}
	}

	health := g.serviceHealth[service.ID]
	health.LastCheck = time.Now()
	health.ResponseTime = responseTime

	if err != nil {
		health.FailureCount++
		health.SuccessCount = 0
		health.LastError = err.Error()
		if health.FailureCount >= service.HealthCheck.UnhealthyThreshold {
			health.Healthy = false
		}
		return
	}

	resp.Body.Close()
	switch {
	case resp.StatusCode >= 500:
		health.FailureCount++
		health.SuccessCount = 0
		health.LastError = fmt.Sprintf("HTTP %d", resp.StatusCode)
		if health.FailureCount >= service.HealthCheck.UnhealthyThreshold {
			health.Healthy = false
		}
	case resp.StatusCode >= 400:
		// Client errors still prove the service is reachable, so treat them as successes
		health.SuccessCount++
		health.FailureCount = 0
		health.LastError = fmt.Sprintf("HTTP %d (client error)", resp.StatusCode)
		if health.SuccessCount >= service.HealthCheck.HealthyThreshold {
			health.Healthy = true
		}
	default:
		health.SuccessCount++
		health.FailureCount = 0
		health.LastError = ""
		if health.SuccessCount >= service.HealthCheck.HealthyThreshold {
			health.Healthy = true
		}
	}
}

// logRequest logs an access log entry
func (g *Gateway) logRequest(r *http.Request, statusCode int, startTime time.Time, routeID, routeName, serviceID, serviceName string, allowTelemetry bool, errMsg string, reqInfo bodyLogInfo, respInfo bodyLogInfo) {
	logEntry := RequestLog{
		Timestamp:             startTime,
		Method:                r.Method,
		Path:                  r.URL.Path,
		Host:                  r.Host,
		RemoteAddr:            getClientIP(r),
		RouteID:               routeID,
		RouteName:             routeName,
		ServiceID:             serviceID,
		ServiceName:           serviceName,
		StatusCode:            statusCode,
		Duration:              time.Since(startTime).Milliseconds(),
		BytesSent:             respInfo.size,
		BytesReceived:         reqInfo.size,
		RequestBody:           reqInfo.body,
		ResponseBody:          respInfo.body,
		RequestBodyTruncated:  reqInfo.truncated,
		ResponseBodyTruncated: respInfo.truncated,
		UserAgent:             r.UserAgent(),
		Error:                 errMsg,
	}

	// Log to console if enabled
	if g.config.AccessLogEnabled {
		logJSON, _ := json.Marshal(logEntry)
		log.Printf("API Gateway Access: %s", string(logJSON))
	}

	// Send to telemetry exporter if configured
	if allowTelemetry && g.config.Observability != nil && g.config.Observability.Enabled {
		GetTelemetryExporter().Record(logEntry)
	}
}

func (g *Gateway) isRouteObservabilityEnabled(route *Route) bool {
	if route == nil || route.ObservabilityEnabled == nil {
		return true
	}
	return *route.ObservabilityEnabled
}

// getClientIP extracts the client IP from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Use RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// recordError increments the error counter
func (g *Gateway) recordError() {
	g.stats.mu.Lock()
	g.stats.totalErrors++
	g.stats.mu.Unlock()
}

// recordServiceError increments the service error counter
func (g *Gateway) recordServiceError(serviceID string) {
	g.stats.mu.Lock()
	if g.stats.serviceStats[serviceID] != nil {
		g.stats.serviceStats[serviceID].errors++
	}
	g.stats.totalErrors++
	g.stats.mu.Unlock()
}

// recordRateLimited increments the rate limited counter
func (g *Gateway) recordRateLimited() {
	g.stats.mu.Lock()
	g.stats.rateLimited++
	g.stats.mu.Unlock()
}

func (g *Gateway) getClientSecurityConfig() *ClientSecurityConfig {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.config == nil {
		return nil
	}
	if g.config.ClientSecurity == nil {
		return nil
	}
	cfg := *g.config.ClientSecurity
	return &cfg
}

func (g *Gateway) trackClientActivity(ip, path, routeID string, statusCode int, matchedRoute bool) {
	cfg := g.getClientSecurityConfig()
	if cfg == nil || !cfg.TrackingEnabled || ip == "" {
		return
	}
	now := time.Now()
	g.clientStatsMu.Lock()
	tracker := g.getOrCreateClientTrackerLocked(ip)
	tracker.requests++
	tracker.lastSeen = now
	tracker.lastPath = path
	tracker.lastRouteID = routeID
	tracker.lastStatus = statusCode
	if matchedRoute {
		tracker.consecutiveMisses = 0
	} else {
		tracker.consecutiveMisses++
		tracker.totalMisses++
	}
	var autoBlockReason string
	if !matchedRoute && cfg.AutoBlockEnabled && cfg.NoRouteThreshold > 0 && tracker.consecutiveMisses >= cfg.NoRouteThreshold {
		duration := defaultAutoBlockDuration
		if cfg.AutoBlockDurationSec > 0 {
			duration = time.Duration(cfg.AutoBlockDurationSec) * time.Second
		}
		autoBlockReason = fmt.Sprintf("blocked after %d unmatched requests", tracker.consecutiveMisses)
		if !g.autoBlockClientLocked(tracker, duration, autoBlockReason) {
			autoBlockReason = ""
		}
	}
	g.clientStatsMu.Unlock()
	if autoBlockReason != "" {
		log.Printf("API Gateway: Auto-blocked client %s (%s)", ip, autoBlockReason)
	}
}

func (g *Gateway) getOrCreateClientTrackerLocked(ip string) *clientStatsTracker {
	if tracker, ok := g.clientStats[ip]; ok {
		return tracker
	}
	if g.clientStatsLimit > 0 && len(g.clientStats) >= g.clientStatsLimit {
		g.evictOldestClientLocked()
	}
	tracker := &clientStatsTracker{ip: ip}
	g.clientStats[ip] = tracker
	return tracker
}

func (g *Gateway) evictOldestClientLocked() {
	var oldestKey string
	var oldestTime time.Time
	first := true
	for key, tracker := range g.clientStats {
		if first || tracker.lastSeen.Before(oldestTime) {
			oldestKey = key
			oldestTime = tracker.lastSeen
			first = false
		}
	}
	if oldestKey != "" {
		delete(g.clientStats, oldestKey)
	}
}

func (g *Gateway) autoBlockClientLocked(tracker *clientStatsTracker, duration time.Duration, reason string) bool {
	now := time.Now()
	if tracker.manualBlocked {
		return false
	}
	if !tracker.blockedUntil.IsZero() && tracker.blockedUntil.After(now) {
		return false
	}
	if duration <= 0 {
		duration = defaultAutoBlockDuration
	}
	tracker.blockedAt = now
	tracker.blockedUntil = now.Add(duration)
	tracker.blockReason = reason
	tracker.consecutiveMisses = 0
	return true
}

func (g *Gateway) isClientBlocked(ip string) (bool, string) {
	if ip == "" {
		return false, ""
	}
	g.clientStatsMu.Lock()
	defer g.clientStatsMu.Unlock()
	tracker, ok := g.clientStats[ip]
	if !ok {
		return false, ""
	}
	now := time.Now()
	if tracker.manualBlocked {
		if tracker.blockedUntil.IsZero() || tracker.blockedUntil.After(now) {
			return true, tracker.blockReason
		}
		tracker.manualBlocked = false
		if tracker.blockedUntil.Before(now) {
			tracker.blockedUntil = time.Time{}
		}
		tracker.blockReason = ""
	}
	if !tracker.blockedUntil.IsZero() && tracker.blockedUntil.After(now) {
		return true, tracker.blockReason
	}
	if !tracker.blockedUntil.IsZero() && tracker.blockedUntil.Before(now) {
		tracker.blockedUntil = time.Time{}
		tracker.blockReason = ""
	}
	return false, ""
}

func (g *Gateway) ManualBlockClient(ip string, duration time.Duration, reason string) error {
	if ip == "" {
		return fmt.Errorf("ip is required")
	}
	blockedAt := time.Now()
	expiresAt := ""
	if duration > 0 {
		expiresAt = blockedAt.Add(duration).Format(time.RFC3339)
	}
	entry := ManualBlockConfig{
		IP:        ip,
		Reason:    reason,
		BlockedAt: blockedAt.Format(time.RFC3339),
		ExpiresAt: expiresAt,
	}
	g.mu.Lock()
	g.ensureClientSecurityDefaultsLocked()
	cfg := g.config.ClientSecurity
	filtered := make([]ManualBlockConfig, 0, len(cfg.ManualBlocks))
	for _, mb := range cfg.ManualBlocks {
		if mb.IP != ip {
			filtered = append(filtered, mb)
		}
	}
	cfg.ManualBlocks = append(filtered, entry)
	err := g.saveConfigLocked()
	g.mu.Unlock()
	if err != nil {
		return err
	}
	g.applyManualBlocks()
	return nil
}

func (g *Gateway) ManualUnblockClient(ip string) error {
	if ip == "" {
		return fmt.Errorf("ip is required")
	}
	g.mu.Lock()
	if g.config == nil || g.config.ClientSecurity == nil {
		g.mu.Unlock()
		return fmt.Errorf("client security not configured")
	}
	cfg := g.config.ClientSecurity
	changed := false
	filtered := make([]ManualBlockConfig, 0, len(cfg.ManualBlocks))
	for _, mb := range cfg.ManualBlocks {
		if mb.IP == ip {
			changed = true
			continue
		}
		filtered = append(filtered, mb)
	}
	cfg.ManualBlocks = filtered
	var err error
	if changed {
		err = g.saveConfigLocked()
	}
	g.mu.Unlock()
	if err != nil {
		return err
	}
	if changed {
		g.applyManualBlocks()
	}
	g.clientStatsMu.Lock()
	if tracker, ok := g.clientStats[ip]; ok {
		tracker.manualBlocked = false
		tracker.blockReason = ""
		tracker.blockedUntil = time.Time{}
		tracker.blockedAt = time.Time{}
		tracker.consecutiveMisses = 0
	}
	g.clientStatsMu.Unlock()
	return nil
}

func (g *Gateway) getTopClients(limit int) []ClientStats {
	if limit <= 0 {
		return nil
	}
	g.clientStatsMu.RLock()
	defer g.clientStatsMu.RUnlock()
	if len(g.clientStats) == 0 {
		return nil
	}
	results := make([]ClientStats, 0, len(g.clientStats))
	now := time.Now()
	for _, tracker := range g.clientStats {
		blocked, manual := tracker.snapshotBlocked(now)
		results = append(results, ClientStats{
			IP:                tracker.ip,
			RequestCount:      tracker.requests,
			LastSeen:          tracker.lastSeen,
			LastPath:          tracker.lastPath,
			LastRouteID:       tracker.lastRouteID,
			LastStatus:        tracker.lastStatus,
			ConsecutiveMisses: tracker.consecutiveMisses,
			TotalMisses:       tracker.totalMisses,
			Blocked:           blocked,
			BlockedUntil:      tracker.blockedUntil,
			BlockedReason:     tracker.blockReason,
			ManualBlock:       manual,
		})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].RequestCount == results[j].RequestCount {
			return results[i].LastSeen.After(results[j].LastSeen)
		}
		return results[i].RequestCount > results[j].RequestCount
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results
}

func (g *Gateway) getBlockedClients() []BlockedClient {
	g.clientStatsMu.RLock()
	defer g.clientStatsMu.RUnlock()
	blocked := make([]BlockedClient, 0)
	now := time.Now()
	for _, tracker := range g.clientStats {
		isBlocked, manual := tracker.snapshotBlocked(now)
		if !isBlocked {
			continue
		}
		blocked = append(blocked, BlockedClient{
			IP:           tracker.ip,
			Manual:       manual,
			BlockedAt:    tracker.blockedAt,
			BlockedUntil: tracker.blockedUntil,
			Reason:       tracker.blockReason,
		})
	}
	sort.Slice(blocked, func(i, j int) bool {
		return blocked[i].BlockedAt.After(blocked[j].BlockedAt)
	})
	return blocked
}

func (t *clientStatsTracker) snapshotBlocked(now time.Time) (bool, bool) {
	manual := false
	blocked := false
	if t.manualBlocked {
		manual = t.blockedUntil.IsZero() || t.blockedUntil.After(now)
		blocked = manual
	}
	if !blocked && !t.blockedUntil.IsZero() && t.blockedUntil.After(now) {
		blocked = true
	}
	return blocked, manual && blocked
}

// GetStats returns the current gateway statistics
func (g *Gateway) GetStats() GatewayStats {
	g.stats.mu.RLock()
	defer g.stats.mu.RUnlock()

	uptime := time.Since(g.stats.startTime).Seconds()

	var avgLatency float64
	if g.stats.totalRequests > 0 {
		avgLatency = float64(g.stats.totalLatency) / float64(g.stats.totalRequests)
	}

	var requestsPerSec float64
	if uptime > 0 {
		requestsPerSec = float64(g.stats.totalRequests) / uptime
	}

	serviceStats := make([]ServiceStats, 0, len(g.stats.serviceStats))
	for serviceID, stats := range g.stats.serviceStats {
		var svcAvgLatency float64
		if stats.requests > 0 {
			svcAvgLatency = float64(stats.totalLatency) / float64(stats.requests)
		}
		serviceStats = append(serviceStats, ServiceStats{
			ServiceID:      serviceID,
			Requests:       stats.requests,
			Errors:         stats.errors,
			AverageLatency: svcAvgLatency,
		})
	}

	stats := GatewayStats{
		TotalRequests:  g.stats.totalRequests,
		TotalErrors:    g.stats.totalErrors,
		Uptime:         int64(uptime),
		RequestsPerSec: requestsPerSec,
		AverageLatency: avgLatency,
		ServiceStats:   serviceStats,
		RateLimitStats: RateLimitStats{
			TotalLimited: g.stats.rateLimited,
		},
	}
	if cfg := g.getClientSecurityConfig(); cfg != nil && cfg.TrackingEnabled {
		stats.TopClients = g.getTopClients(cfg.TopClientLimit)
		stats.BlockedClients = g.getBlockedClients()
	}
	return stats
}

// GetServiceHealth returns the health status of all services
func (g *Gateway) GetServiceHealth() []ServiceHealth {
	g.mu.RLock()
	defer g.mu.RUnlock()

	health := make([]ServiceHealth, 0, len(g.serviceHealth))
	for _, h := range g.serviceHealth {
		health = append(health, *h)
	}
	return health
}

// AddService adds a new service to the gateway
func (g *Gateway) AddService(service Service) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Check for duplicate ID
	for _, svc := range g.config.Services {
		if svc.ID == service.ID {
			return fmt.Errorf("service with ID %s already exists", service.ID)
		}
	}

	g.config.Services = append(g.config.Services, service)
	g.services[service.ID] = &g.config.Services[len(g.config.Services)-1]

	return g.saveConfigLocked()
}

// UpdateService updates an existing service
func (g *Gateway) UpdateService(service Service) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	for i, svc := range g.config.Services {
		if svc.ID == service.ID {
			g.config.Services[i] = service
			g.services[service.ID] = &g.config.Services[i]
			return g.saveConfigLocked()
		}
	}

	return fmt.Errorf("service with ID %s not found", service.ID)
}

// DeleteService removes a service from the gateway
func (g *Gateway) DeleteService(serviceID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	for i, svc := range g.config.Services {
		if svc.ID == serviceID {
			g.config.Services = append(g.config.Services[:i], g.config.Services[i+1:]...)
			delete(g.services, serviceID)
			delete(g.serviceHealth, serviceID)
			return g.saveConfigLocked()
		}
	}

	return fmt.Errorf("service with ID %s not found", serviceID)
}

// AddRoute adds a new route to the gateway
func (g *Gateway) AddRoute(route Route) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Check for duplicate ID
	for _, r := range g.config.Routes {
		if r.ID == route.ID {
			return fmt.Errorf("route with ID %s already exists", route.ID)
		}
	}

	g.config.Routes = append(g.config.Routes, route)
	g.refreshRoutes()

	return g.saveConfigLocked()
}

// UpdateRoute updates an existing route
func (g *Gateway) UpdateRoute(route Route) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	for i, r := range g.config.Routes {
		if r.ID == route.ID {
			g.config.Routes[i] = route
			g.refreshRoutes()
			return g.saveConfigLocked()
		}
	}

	return fmt.Errorf("route with ID %s not found", route.ID)
}

// DeleteRoute removes a route from the gateway
func (g *Gateway) DeleteRoute(routeID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	for i, r := range g.config.Routes {
		if r.ID == routeID {
			g.config.Routes = append(g.config.Routes[:i], g.config.Routes[i+1:]...)
			g.refreshRoutes()
			return g.saveConfigLocked()
		}
	}

	return fmt.Errorf("route with ID %s not found", routeID)
}

// ListUDPRoutes returns all UDP routes from the current config.
func (g *Gateway) ListUDPRoutes() []UDPRoute {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.config == nil || len(g.config.UDPRoutes) == 0 {
		return nil
	}
	out := make([]UDPRoute, len(g.config.UDPRoutes))
	copy(out, g.config.UDPRoutes)
	return out
}

// AddUDPRoute adds a new UDP route and restarts the gateway if running.
func (g *Gateway) AddUDPRoute(route UDPRoute) error {
	g.mu.RLock()
	for _, r := range g.config.UDPRoutes {
		if r.ID == route.ID {
			g.mu.RUnlock()
			return fmt.Errorf("UDP route with ID %s already exists", route.ID)
		}
	}
	if _, ok := g.services[route.ServiceID]; !ok {
		g.mu.RUnlock()
		return fmt.Errorf("service %s not found", route.ServiceID)
	}
	cfgCopy := *g.config
	cfgCopy.UDPRoutes = make([]UDPRoute, len(g.config.UDPRoutes)+1)
	copy(cfgCopy.UDPRoutes, g.config.UDPRoutes)
	cfgCopy.UDPRoutes[len(cfgCopy.UDPRoutes)-1] = route
	g.mu.RUnlock()

	return g.UpdateConfig(&cfgCopy)
}

// RemoveUDPRoute removes a UDP route by ID and restarts the gateway if running.
func (g *Gateway) RemoveUDPRoute(routeID string) error {
	g.mu.RLock()
	for i, r := range g.config.UDPRoutes {
		if r.ID == routeID {
			cfgCopy := *g.config
			cfgCopy.UDPRoutes = append(append([]UDPRoute{}, g.config.UDPRoutes[:i]...), g.config.UDPRoutes[i+1:]...)
			g.mu.RUnlock()
			return g.UpdateConfig(&cfgCopy)
		}
	}
	g.mu.RUnlock()
	return fmt.Errorf("UDP route with ID %s not found", routeID)
}

// ListTCPRoutes returns all TCP routes from the current config.
func (g *Gateway) ListTCPRoutes() []TCPRoute {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.config == nil || len(g.config.TCPRoutes) == 0 {
		return nil
	}
	out := make([]TCPRoute, len(g.config.TCPRoutes))
	copy(out, g.config.TCPRoutes)
	return out
}

// AddTCPRoute adds a new TCP route and restarts the gateway if running.
func (g *Gateway) AddTCPRoute(route TCPRoute) error {
	g.mu.RLock()
	for _, r := range g.config.TCPRoutes {
		if r.ID == route.ID {
			g.mu.RUnlock()
			return fmt.Errorf("TCP route with ID %s already exists", route.ID)
		}
	}
	if _, ok := g.services[route.ServiceID]; !ok {
		g.mu.RUnlock()
		return fmt.Errorf("service %s not found", route.ServiceID)
	}
	cfgCopy := *g.config
	cfgCopy.TCPRoutes = make([]TCPRoute, len(g.config.TCPRoutes)+1)
	copy(cfgCopy.TCPRoutes, g.config.TCPRoutes)
	cfgCopy.TCPRoutes[len(cfgCopy.TCPRoutes)-1] = route
	g.mu.RUnlock()

	return g.UpdateConfig(&cfgCopy)
}

// RemoveTCPRoute removes a TCP route by ID and restarts the gateway if running.
func (g *Gateway) RemoveTCPRoute(routeID string) error {
	g.mu.RLock()
	for i, r := range g.config.TCPRoutes {
		if r.ID == routeID {
			cfgCopy := *g.config
			cfgCopy.TCPRoutes = append(append([]TCPRoute{}, g.config.TCPRoutes[:i]...), g.config.TCPRoutes[i+1:]...)
			g.mu.RUnlock()
			return g.UpdateConfig(&cfgCopy)
		}
	}
	g.mu.RUnlock()
	return fmt.Errorf("TCP route with ID %s not found", routeID)
}

// refreshRoutes rebuilds and sorts the routes slice (must be called with lock held)
func (g *Gateway) refreshRoutes() {
	g.routes = make([]*Route, len(g.config.Routes))
	for i := range g.config.Routes {
		g.routes[i] = &g.config.Routes[i]
	}
	sort.Slice(g.routes, func(i, j int) bool {
		return g.routes[i].Priority > g.routes[j].Priority
	})
}

// StartAll starts the gateway if configured to be enabled
func (g *Gateway) StartAll() {
	if g.config.Enabled && !g.running {
		if err := g.Start(); err != nil {
			log.Printf("API Gateway: Failed to auto-start: %v", err)
		}
	}

	// Start certificate renewer if Let's Encrypt is enabled with auto-renew
	if g.config.LetsEncrypt != nil && g.config.LetsEncrypt.Enabled && g.config.LetsEncrypt.AutoRenew {
		GetCertificateRenewer(g).Start()
	}

	// Start telemetry exporter if observability is enabled
	if g.config.Observability != nil && g.config.Observability.Enabled {
		exporter := GetTelemetryExporter()
		exporter.Configure(g.config.Observability)
		exporter.Start()
	}
}

// Validate validates a request manually for testing
func (g *Gateway) Validate(method, path, host string, headers map[string]string) (*Route, *Service, error) {
	// Create a mock request
	req, err := http.NewRequest(method, "http://"+host+path, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Host = host
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	route := g.matchRoute(req)
	if route == nil {
		return nil, nil, fmt.Errorf("no matching route")
	}

	g.mu.RLock()
	service := g.services[route.ServiceID]
	g.mu.RUnlock()

	if service == nil {
		return route, nil, fmt.Errorf("service not found")
	}

	return route, service, nil
}

// HealthCheckNow triggers an immediate health check for a service
func (g *Gateway) HealthCheckNow(serviceID string) (*ServiceHealth, error) {
	g.mu.RLock()
	service := g.services[serviceID]
	g.mu.RUnlock()

	if service == nil {
		return nil, fmt.Errorf("service not found")
	}

	if service.HealthCheck == nil {
		return nil, fmt.Errorf("service has no health check configured")
	}

	g.checkServiceHealth(service)

	g.mu.RLock()
	health := g.serviceHealth[serviceID]
	g.mu.RUnlock()

	if health == nil {
		return nil, fmt.Errorf("health check not completed")
	}

	return health, nil
}

// TestUpstream tests connectivity to an upstream service
func (g *Gateway) TestUpstream(host string, port int, path string) (int, int64, error) {
	targetURL := fmt.Sprintf("http://%s:%d%s", host, port, path)

	startTime := time.Now()
	resp, err := g.httpClient.Get(targetURL)
	latency := time.Since(startTime).Milliseconds()

	if err != nil {
		return 0, latency, err
	}
	defer resp.Body.Close()

	// Read and discard body
	io.Copy(io.Discard, resp.Body)

	return resp.StatusCode, latency, nil
}

// GetAccessLogs returns recent access logs (placeholder for future implementation)
func (g *Gateway) GetAccessLogs(limit int) []RequestLog {
	// In a real implementation, this would read from a log file or database
	return []RequestLog{}
}

type bodyLogInfo struct {
	body      string
	truncated bool
	size      int64
}

func captureRequestBody(r *http.Request, limit int) (bodyLogInfo, error) {
	if r.Body == nil || r.Body == http.NoBody {
		return bodyLogInfo{}, nil
	}
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		return bodyLogInfo{}, err
	}
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	r.ContentLength = int64(len(bodyBytes))
	if r.GetBody != nil {
		copyBytes := append([]byte(nil), bodyBytes...)
		r.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(copyBytes)), nil
		}
	}
	return bodyLogInfoFromBytes(bodyBytes, limit), nil
}

func bodyLogInfoFromBytes(data []byte, limit int) bodyLogInfo {
	if len(data) == 0 {
		return bodyLogInfo{}
	}
	truncated := limit > 0 && len(data) > limit
	snippet := data
	if truncated {
		snippet = data[:limit]
	}
	return bodyLogInfo{
		body:      string(snippet),
		truncated: truncated,
		size:      int64(len(data)),
	}
}

type loggingResponseWriter struct {
	http.ResponseWriter
	body         bytes.Buffer
	limit        int
	statusCode   int
	bytesWritten int64
}

func newLoggingResponseWriter(w http.ResponseWriter, limit int) *loggingResponseWriter {
	return &loggingResponseWriter{ResponseWriter: w, limit: limit}
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

func (lrw *loggingResponseWriter) Write(b []byte) (int, error) {
	if lrw.statusCode == 0 {
		lrw.statusCode = http.StatusOK
	}
	if lrw.limit <= 0 {
		lrw.body.Write(b)
	} else {
		remaining := lrw.limit - lrw.body.Len()
		if remaining > 0 {
			copyLen := remaining
			if copyLen > len(b) {
				copyLen = len(b)
			}
			if copyLen > 0 {
				lrw.body.Write(b[:copyLen])
			}
		}
	}
	lrw.bytesWritten += int64(len(b))
	return lrw.ResponseWriter.Write(b)
}

func (lrw *loggingResponseWriter) StatusCode() int {
	if lrw.statusCode == 0 {
		return http.StatusOK
	}
	return lrw.statusCode
}

func (lrw *loggingResponseWriter) LogInfo() bodyLogInfo {
	truncated := lrw.limit > 0 && lrw.body.Len() >= lrw.limit && lrw.bytesWritten > int64(lrw.limit)
	return bodyLogInfo{
		body:      lrw.body.String(),
		truncated: truncated,
		size:      lrw.bytesWritten,
	}
}

func (lrw *loggingResponseWriter) Flush() {
	if flusher, ok := lrw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (lrw *loggingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := lrw.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, fmt.Errorf("http.Hijacker not supported")
}

func (lrw *loggingResponseWriter) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := lrw.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return http.ErrNotSupported
}
