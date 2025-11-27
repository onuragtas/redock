package api_gateway

import (
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
	"sort"
	"strings"
	"sync"
	"time"

	dockermanager "redock/docker-manager"
)

var (
	gateway         *Gateway
	gatewayLock     sync.Mutex
	dockerManager   *dockermanager.DockerEnvironmentManager
)

// Init initializes the API Gateway
func Init(dm *dockermanager.DockerEnvironmentManager) {
	dockerManager = dm
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
			HTTPPort:         80,
			HTTPSPort:        443,
			HTTPSEnabled:     false,
			LogLevel:         "info",
			AccessLogEnabled: true,
			Enabled:          false,
			Services:         []Service{},
			Routes:           []Route{},
		}
		return
	}

	var config GatewayConfig
	if err := json.Unmarshal(file, &config); err != nil {
		log.Printf("API Gateway: Error parsing config: %v", err)
		g.config = &GatewayConfig{
			HTTPPort:         80,
			HTTPSPort:        443,
			HTTPSEnabled:     false,
			LogLevel:         "info",
			AccessLogEnabled: true,
			Enabled:          false,
			Services:         []Service{},
			Routes:           []Route{},
		}
		return
	}

	g.config = &config
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
}

// GetConfig returns the current gateway configuration
func (g *Gateway) GetConfig() *GatewayConfig {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.config
}

// UpdateConfig updates the gateway configuration
func (g *Gateway) UpdateConfig(config *GatewayConfig) error {
	gatewayLock.Lock()
	defer gatewayLock.Unlock()

	wasRunning := g.running

	// Stop if running
	if wasRunning {
		if err := g.Stop(); err != nil {
			return fmt.Errorf("failed to stop gateway: %w", err)
		}
	}

	g.mu.Lock()
	g.config = config
	g.mu.Unlock()

	g.refreshServicesAndRoutes()

	if err := g.SaveConfig(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// Restart if was running
	if wasRunning && config.Enabled {
		if err := g.Start(); err != nil {
			return fmt.Errorf("failed to restart gateway: %w", err)
		}
	}

	return nil
}

// Start starts the API Gateway servers
func (g *Gateway) Start() error {
	gatewayLock.Lock()
	defer gatewayLock.Unlock()

	if g.running {
		return fmt.Errorf("gateway is already running")
	}

	g.refreshServicesAndRoutes()
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
	}

	if g.httpsServer != nil {
		if err := g.httpsServer.Shutdown(ctx); err != nil {
			log.Printf("API Gateway: HTTPS server shutdown error: %v", err)
		}
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

	// Update stats
	g.stats.mu.Lock()
	g.stats.totalRequests++
	g.stats.mu.Unlock()

	// Check global rate limit
	if g.globalLimiter != nil {
		clientIP := getClientIP(r)
		if !g.checkRateLimit(g.globalLimiter, clientIP) {
			g.recordError()
			g.recordRateLimited()
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			g.logRequest(r, http.StatusTooManyRequests, startTime, "", "", "rate limit exceeded")
			return
		}
	}

	// Find matching route
	route := g.matchRoute(r)
	if route == nil {
		g.recordError()
		http.Error(w, "Not Found", http.StatusNotFound)
		g.logRequest(r, http.StatusNotFound, startTime, "", "", "no matching route")
		return
	}

	// Check route-level rate limit
	if route.RateLimitEnabled && route.RateLimitRequests > 0 {
		clientIP := getClientIP(r)
		routeLimiter := &rateLimiter{
			clients:  make(map[string]*clientRateLimit),
			requests: route.RateLimitRequests,
			window:   time.Duration(route.RateLimitWindow) * time.Second,
		}
		if !g.checkRateLimit(routeLimiter, clientIP) {
			g.recordError()
			g.recordRateLimited()
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			g.logRequest(r, http.StatusTooManyRequests, startTime, route.ID, "", "rate limit exceeded")
			return
		}
	}

	// Check authentication if required
	if route.AuthRequired {
		if !g.checkAuth(r, route) {
			g.recordError()
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			g.logRequest(r, http.StatusUnauthorized, startTime, route.ID, "", "authentication failed")
			return
		}
	}

	// Get service
	g.mu.RLock()
	service := g.services[route.ServiceID]
	g.mu.RUnlock()

	if service == nil || !service.Enabled {
		g.recordError()
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		g.logRequest(r, http.StatusServiceUnavailable, startTime, route.ID, route.ServiceID, "service not available")
		return
	}

	// Check service health
	g.mu.RLock()
	health := g.serviceHealth[service.ID]
	g.mu.RUnlock()

	if health != nil && !health.Healthy {
		g.recordError()
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		g.logRequest(r, http.StatusServiceUnavailable, startTime, route.ID, service.ID, "service unhealthy")
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
	statusCode, err := g.proxyRequest(w, r, route, service)
	if err != nil {
		g.recordServiceError(service.ID)
		g.logRequest(r, statusCode, startTime, route.ID, service.ID, err.Error())
	} else {
		g.logRequest(r, statusCode, startTime, route.ID, service.ID, "")
	}

	// Record latency
	latency := time.Since(startTime).Milliseconds()
	g.stats.mu.Lock()
	g.stats.totalLatency += latency
	if g.stats.serviceStats[service.ID] != nil {
		g.stats.serviceStats[service.ID].totalLatency += latency
	}
	g.stats.mu.Unlock()
}

// matchRoute finds the first matching route for the request
func (g *Gateway) matchRoute(r *http.Request) *Route {
	g.mu.RLock()
	defer g.mu.RUnlock()

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
			requestHost := r.Host
			if idx := strings.Index(requestHost, ":"); idx != -1 {
				requestHost = requestHost[:idx]
			}
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

		// Check paths
		pathMatch := false
		for _, p := range route.Paths {
			if matchPath(p, r.URL.Path) {
				pathMatch = true
				break
			}
		}
		if !pathMatch {
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

		return route
	}

	return nil
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
func (g *Gateway) proxyRequest(w http.ResponseWriter, r *http.Request, route *Route, service *Service) (int, error) {
	// Build target URL
	protocol := service.Protocol
	if protocol == "" {
		protocol = "http"
	}

	targetURL := fmt.Sprintf("%s://%s:%d", protocol, service.Host, service.Port)
	target, err := url.Parse(targetURL)
	if err != nil {
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return http.StatusBadGateway, err
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
		if !route.PreserveHost {
			req.Host = target.Host
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

	// Capture status code
	statusCode := http.StatusOK
	proxy.ModifyResponse = func(resp *http.Response) error {
		statusCode = resp.StatusCode
		return nil
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		statusCode = http.StatusBadGateway
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}

	proxy.ServeHTTP(w, r)
	return statusCode, nil
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
	} else {
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			health.SuccessCount++
			health.FailureCount = 0
			health.LastError = ""
			if health.SuccessCount >= service.HealthCheck.HealthyThreshold {
				health.Healthy = true
			}
		} else {
			health.FailureCount++
			health.SuccessCount = 0
			health.LastError = fmt.Sprintf("HTTP %d", resp.StatusCode)
			if health.FailureCount >= service.HealthCheck.UnhealthyThreshold {
				health.Healthy = false
			}
		}
	}
}

// logRequest logs an access log entry
func (g *Gateway) logRequest(r *http.Request, statusCode int, startTime time.Time, routeID, serviceID, errMsg string) {
	if !g.config.AccessLogEnabled {
		return
	}

	logEntry := RequestLog{
		Timestamp:  startTime,
		Method:     r.Method,
		Path:       r.URL.Path,
		Host:       r.Host,
		RemoteAddr: getClientIP(r),
		RouteID:    routeID,
		ServiceID:  serviceID,
		StatusCode: statusCode,
		Duration:   time.Since(startTime).Milliseconds(),
		UserAgent:  r.UserAgent(),
		Error:      errMsg,
	}

	logJSON, _ := json.Marshal(logEntry)
	log.Printf("API Gateway Access: %s", string(logJSON))
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

	return GatewayStats{
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
