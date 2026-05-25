package api_gateway

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/bcrypt"
)

func TestMatchPath(t *testing.T) {
	tests := []struct {
		pattern  string
		path     string
		expected bool
	}{
		{"/api", "/api", true},
		{"/api", "/api/users", true},
		{"/api/", "/api/users", true},
		{"/api/*", "/api/users", true},
		{"/api/*", "/api/users/123", true},
		{"/api", "/other", false},
		{"/api/users", "/api/products", false},
		{"/observability", "/observability", true},
		{"/observability", "/observability/dashboard", true},
		{"/observability/", "/observability/api/v1", true},
	}

	for _, test := range tests {
		result := matchPath(test.pattern, test.path)
		assert.Equal(t, test.expected, result, "Pattern: %s, Path: %s", test.pattern, test.path)
	}
}

func TestMatchWildcard(t *testing.T) {
	tests := []struct {
		pattern  string
		value    string
		expected bool
	}{
		{"*", "example.com", true},
		{"*.example.com", "api.example.com", true},
		{"*.example.com", "www.example.com", true},
		{"*.example.com", "example.com", true},
		{"api.example.com", "api.example.com", true},
		{"api.example.com", "www.example.com", false},
		{"*.example.com", "other.domain.com", false},
	}

	for _, test := range tests {
		result := matchWildcard(test.pattern, test.value)
		assert.Equal(t, test.expected, result, "Pattern: %s, Value: %s", test.pattern, test.value)
	}
}

func TestRateLimiter(t *testing.T) {
	limiter := &rateLimiter{
		clients:  make(map[string]*clientRateLimit),
		requests: 3,
		window:   time.Second,
	}

	g := &Gateway{}

	// First 3 requests should succeed
	for i := 0; i < 3; i++ {
		result := g.checkRateLimit(limiter, "192.168.1.1")
		assert.True(t, result, "Request %d should succeed", i+1)
	}

	// 4th request should fail
	result := g.checkRateLimit(limiter, "192.168.1.1")
	assert.False(t, result, "4th request should be rate limited")

	// Different IP should succeed
	result = g.checkRateLimit(limiter, "192.168.1.2")
	assert.True(t, result, "Different IP should succeed")
}

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name       string
		headers    map[string]string
		remoteAddr string
		expected   string
	}{
		{
			name:       "X-Forwarded-For single IP",
			headers:    map[string]string{"X-Forwarded-For": "192.168.1.1"},
			remoteAddr: "10.0.0.1:12345",
			expected:   "192.168.1.1",
		},
		{
			name:       "X-Forwarded-For multiple IPs",
			headers:    map[string]string{"X-Forwarded-For": "192.168.1.1, 10.0.0.1"},
			remoteAddr: "10.0.0.1:12345",
			expected:   "192.168.1.1",
		},
		{
			name:       "X-Real-IP",
			headers:    map[string]string{"X-Real-IP": "192.168.1.100"},
			remoteAddr: "10.0.0.1:12345",
			expected:   "192.168.1.100",
		},
		{
			name:       "RemoteAddr fallback",
			headers:    map[string]string{},
			remoteAddr: "10.0.0.1:12345",
			expected:   "10.0.0.1",
		},
	}

	for _, test := range tests {
		req := httptest.NewRequest("GET", "/test", nil)
		for k, v := range test.headers {
			req.Header.Set(k, v)
		}
		req.RemoteAddr = test.remoteAddr

		result := getClientIP(req)
		assert.Equal(t, test.expected, result, test.name)
	}
}

func TestGatewayMatchRoute(t *testing.T) {
	g := &Gateway{
		routes: []*Route{
			{
				ID:        "route1",
				Name:      "API Route",
				UpstreamID: "up1",
				Paths:     []string{"/api"},
				Methods:   []string{"GET", "POST"},
				Enabled:   true,
				Priority:  100,
			},
			{
				ID:        "route2",
				Name:      "Observability Route",
				UpstreamID: "up2",
				Paths:     []string{"/observability"},
				Hosts:     []string{"metrics.example.com"},
				Enabled:   true,
				Priority:  50,
			},
			{
				ID:        "route3",
				Name:      "Disabled Route",
				UpstreamID: "up3",
				Paths:     []string{"/disabled"},
				Enabled:   false,
				Priority:  200,
			},
		},
	}

	tests := []struct {
		name       string
		method     string
		path       string
		host       string
		expectedID string
	}{
		{
			name:       "Match API route",
			method:     "GET",
			path:       "/api/users",
			host:       "localhost",
			expectedID: "route1",
		},
		{
			name:       "Match API route with POST",
			method:     "POST",
			path:       "/api/users",
			host:       "localhost",
			expectedID: "route1",
		},
		{
			name:       "No match for DELETE on API route",
			method:     "DELETE",
			path:       "/api/users",
			host:       "localhost",
			expectedID: "",
		},
		{
			name:       "Match observability route with host",
			method:     "GET",
			path:       "/observability/dashboard",
			host:       "metrics.example.com",
			expectedID: "route2",
		},
		{
			name:       "No match for observability route with wrong host",
			method:     "GET",
			path:       "/observability/dashboard",
			host:       "other.example.com",
			expectedID: "",
		},
		{
			name:       "Disabled route should not match",
			method:     "GET",
			path:       "/disabled",
			host:       "localhost",
			expectedID: "",
		},
		{
			name:       "No match for unknown path",
			method:     "GET",
			path:       "/unknown",
			host:       "localhost",
			expectedID: "",
		},
	}

	for _, test := range tests {
		req := httptest.NewRequest(test.method, "http://"+test.host+test.path, nil)
		req.Host = test.host

		route := g.matchRoute(req)
		if test.expectedID == "" {
			assert.Nil(t, route, test.name)
		} else {
			assert.NotNil(t, route, test.name)
			if route != nil {
				assert.Equal(t, test.expectedID, route.ID, test.name)
			}
		}
	}
}

func TestGatewayCheckAuth(t *testing.T) {
	g := &Gateway{}

	// helpers
	bcryptHash := func(pw string) string {
		h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.MinCost)
		if err != nil {
			t.Fatalf("bcrypt: %v", err)
		}
		return string(h)
	}
	signHS256 := func(secret string, claims jwt.MapClaims) string {
		tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		s, err := tok.SignedString([]byte(secret))
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		return s
	}

	jwtSecret := "supersecret"
	validToken := signHS256(jwtSecret, jwt.MapClaims{
		"iss": "issuer-a",
		"aud": "aud-a",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	expiredToken := signHS256(jwtSecret, jwt.MapClaims{
		"exp": time.Now().Add(-time.Hour).Unix(),
	})
	wrongSecretToken := signHS256("other-secret", jwt.MapClaims{
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	basicRoute := &Route{
		AuthRequired:   true,
		AuthType:       "basic",
		BasicAuthUsers: []BasicAuthUser{{Username: "user", PasswordHash: bcryptHash("pass")}},
	}
	jwtRoute := &Route{
		AuthRequired: true,
		AuthType:     "jwt",
		JWT:          &JWTConfig{Secret: jwtSecret, Issuer: "issuer-a", Audience: "aud-a"},
	}
	jwtRouteNoSecret := &Route{AuthRequired: true, AuthType: "jwt", JWT: &JWTConfig{}}

	tests := []struct {
		name     string
		route    *Route
		headers  map[string]string
		expected bool
	}{
		{"No auth required", &Route{AuthRequired: false}, nil, true},

		// basic
		{"Basic ok", basicRoute, map[string]string{"Authorization": "Basic dXNlcjpwYXNz"}, true},      // user:pass
		{"Basic wrong password", basicRoute, map[string]string{"Authorization": "Basic dXNlcjp4eHg="}, false}, // user:xxx
		{"Basic unknown user", basicRoute, map[string]string{"Authorization": "Basic Zm9vOnBhc3M="}, false},   // foo:pass
		{"Basic no header", basicRoute, nil, false},
		{"Basic with no users configured",
			&Route{AuthRequired: true, AuthType: "basic"},
			map[string]string{"Authorization": "Basic dXNlcjpwYXNz"},
			false},

		// jwt
		{"JWT valid", jwtRoute, map[string]string{"Authorization": "Bearer " + validToken}, true},
		{"JWT expired", jwtRoute, map[string]string{"Authorization": "Bearer " + expiredToken}, false},
		{"JWT wrong secret", jwtRoute, map[string]string{"Authorization": "Bearer " + wrongSecretToken}, false},
		{"JWT garbage", jwtRoute, map[string]string{"Authorization": "Bearer not-a-jwt"}, false},
		{"JWT no header", jwtRoute, nil, false},
		{"JWT no secret configured", jwtRouteNoSecret, map[string]string{"Authorization": "Bearer " + validToken}, false},

		// header
		{"Header ok",
			&Route{AuthRequired: true, AuthType: "header", AuthHeaders: []AuthHeader{{Key: "X-API-Key", Value: "my-api-key"}}},
			map[string]string{"X-API-Key": "my-api-key"}, true},
		{"Header missing",
			&Route{AuthRequired: true, AuthType: "header", AuthHeaders: []AuthHeader{{Key: "X-API-Key", Value: "my-api-key"}}},
			nil, false},
	}

	for _, test := range tests {
		req := httptest.NewRequest("GET", "/test", nil)
		for k, v := range test.headers {
			req.Header.Set(k, v)
		}

		allowed, _ := g.checkAuth(req, test.route)
		assert.Equal(t, test.expected, allowed, test.name)
	}
}

func TestCheckAuthSendsBasicChallenge(t *testing.T) {
	g := &Gateway{}
	route := &Route{
		AuthRequired:   true,
		AuthType:       "basic",
		BasicAuthRealm: "MyZone",
		BasicAuthUsers: []BasicAuthUser{{Username: "u", PasswordHash: "$2a$04$invalid"}},
	}
	req := httptest.NewRequest("GET", "/test", nil)
	allowed, challenge := g.checkAuth(req, route)
	assert.False(t, allowed)
	assert.Contains(t, challenge, `Basic realm="MyZone"`)
}

func TestGatewayAddDeleteService(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	g := &Gateway{
		services:      make(map[string]*Service),
		serviceHealth: make(map[string]*ServiceHealth),
		config: &GatewayConfig{
			Services: []Service{},
			Routes:   []Route{},
		},
		workDir: tmpDir,
	}

	// Create data directory
	os.MkdirAll(tmpDir+"/data", 0755)

	// Add a service
	service := Service{
		ID:       "svc1",
		Name:     "Test Service",
		Host:     "localhost",
		Port:     8080,
		Protocol: "http",
		Enabled:  true,
	}

	err := g.AddService(service)
	assert.NoError(t, err)
	assert.Len(t, g.config.Services, 1)
	assert.Equal(t, "svc1", g.config.Services[0].ID)

	// Try to add duplicate
	err = g.AddService(service)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")

	// Delete the service
	err = g.DeleteService("svc1")
	assert.NoError(t, err)
	assert.Len(t, g.config.Services, 0)

	// Delete non-existent service
	err = g.DeleteService("non-existent")
	assert.Error(t, err)
}

func TestGatewayAddDeleteRoute(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	g := &Gateway{
		services:         make(map[string]*Service),
		upstreams:        map[string]*Upstream{"up1": {ID: "up1", Enabled: true, Targets: []UpstreamTarget{{ServiceID: "svc1", Weight: 1}}}},
		upstreamRuntimes: make(map[string]*upstreamRuntime),
		serviceHealth:    make(map[string]*ServiceHealth),
		config: &GatewayConfig{
			Services:  []Service{},
			Upstreams: []Upstream{{ID: "up1", Enabled: true, Targets: []UpstreamTarget{{ServiceID: "svc1", Weight: 1}}}},
			Routes:    []Route{},
		},
		routes:  make([]*Route, 0),
		workDir: tmpDir,
	}

	// Create data directory
	os.MkdirAll(tmpDir+"/data", 0755)

	// Add a route
	route := Route{
		ID:         "route1",
		Name:       "Test Route",
		UpstreamID: "up1",
		Paths:      []string{"/api"},
		Enabled:    true,
	}

	err := g.AddRoute(route)
	assert.NoError(t, err)
	assert.Len(t, g.config.Routes, 1)
	assert.Equal(t, "route1", g.config.Routes[0].ID)

	// Try to add duplicate
	err = g.AddRoute(route)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")

	// Delete the route
	err = g.DeleteRoute("route1")
	assert.NoError(t, err)
	assert.Len(t, g.config.Routes, 0)

	// Delete non-existent route
	err = g.DeleteRoute("non-existent")
	assert.Error(t, err)
}

func TestGatewayStats(t *testing.T) {
	g := &Gateway{
		stats: &gatewayStatsTracker{
			startTime:     time.Now().Add(-time.Hour), // Started 1 hour ago
			totalRequests: 1000,
			totalErrors:   50,
			totalLatency:  5000, // 5000ms total
			serviceStats:  make(map[string]*serviceStatsTracker),
			rateLimited:   10,
		},
	}

	g.stats.serviceStats["svc1"] = &serviceStatsTracker{
		requests:     500,
		errors:       25,
		totalLatency: 2500,
	}

	stats := g.GetStats()

	assert.Equal(t, int64(1000), stats.TotalRequests)
	assert.Equal(t, int64(50), stats.TotalErrors)
	assert.InDelta(t, 5.0, stats.AverageLatency, 0.1)
	assert.True(t, stats.Uptime >= 3599) // Should be approximately 3600 seconds
	assert.Len(t, stats.ServiceStats, 1)
	assert.Equal(t, int64(10), stats.RateLimitStats.TotalLimited)
}

func TestProxyIntegration(t *testing.T) {
	// Create a simple backend server
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok", "path": "` + r.URL.Path + `"}`))
	}))
	defer backend.Close()

	// Parse backend URL
	backendHost := backend.Listener.Addr().String()
	hostParts := splitHostPort(backendHost)

	g := &Gateway{
		services: map[string]*Service{
			"backend": {
				ID:       "backend",
				Name:     "Backend Service",
				Host:     hostParts[0],
				Port:     mustParseInt(hostParts[1]),
				Protocol: "http",
				Enabled:  true,
			},
		},
		upstreams: map[string]*Upstream{
			"up-backend": {
				ID:       "up-backend",
				Strategy: StrategyRoundRobin,
				Enabled:  true,
				Targets:  []UpstreamTarget{{ServiceID: "backend", Weight: 1}},
			},
		},
		upstreamRuntimes: make(map[string]*upstreamRuntime),
		routes: []*Route{
			{
				ID:         "test-route",
				UpstreamID: "up-backend",
				Paths:      []string{"/api"},
				Enabled:    true,
			},
		},
		serviceHealth: make(map[string]*ServiceHealth),
		config: &GatewayConfig{
			HTTPPort:         0,
			HTTPSEnabled:     false,
			AccessLogEnabled: false,
			Enabled:          true,
		},
		stats: &gatewayStatsTracker{
			startTime:    time.Now(),
			serviceStats: make(map[string]*serviceStatsTracker),
		},
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}

	// Test validate
	route, service, err := g.Validate("GET", "/api/test", "localhost", nil)
	assert.NoError(t, err)
	assert.NotNil(t, route)
	assert.NotNil(t, service)
	assert.Equal(t, "test-route", route.ID)
	assert.Equal(t, "backend", service.ID)
}

// Helper functions
func splitHostPort(addr string) []string {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return []string{addr[:i], addr[i+1:]}
		}
	}
	return []string{addr, "80"}
}

func mustParseInt(s string) int {
	var result int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			result = result*10 + int(c-'0')
		}
	}
	return result
}
