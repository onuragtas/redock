package dns_server

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// UpstreamManager manages upstream DNS servers
type UpstreamManager struct {
	upstreams   []string
	client      *dns.Client
	mutex       sync.RWMutex
	failureMap  map[string]int
	lastAttempt map[string]time.Time
}

// NewUpstreamManager creates a new upstream manager
func NewUpstreamManager(upstreams []string) *UpstreamManager {
	return &UpstreamManager{
		upstreams: upstreams,
		client: &dns.Client{
			Timeout: 5 * time.Second,
		},
		failureMap:  make(map[string]int),
		lastAttempt: make(map[string]time.Time),
	}
}

// Query sends DNS query to upstream servers with fallback
func (u *UpstreamManager) Query(msg *dns.Msg) (*dns.Msg, error) {
	u.mutex.RLock()
	upstreams := make([]string, len(u.upstreams))
	copy(upstreams, u.upstreams)
	u.mutex.RUnlock()

	var lastErr error

	// Try each upstream server
	for _, upstream := range upstreams {
		// Check if server is in cooldown after failures
		if u.isInCooldown(upstream) {
			continue
		}

		response, _, err := u.client.Exchange(msg, upstream)

		if err == nil && response != nil {
			// Success - reset failure count
			u.mutex.Lock()
			u.failureMap[upstream] = 0
			u.lastAttempt[upstream] = time.Now()
			u.mutex.Unlock()

			return response, nil
		}

		// Record failure
		u.mutex.Lock()
		u.failureMap[upstream]++
		u.lastAttempt[upstream] = time.Now()
		u.mutex.Unlock()

		lastErr = err
	}

	if lastErr != nil {
		return nil, fmt.Errorf("all upstream servers failed: %w", lastErr)
	}

	return nil, fmt.Errorf("no upstream servers available")
}

// isInCooldown checks if upstream is in cooldown period
func (u *UpstreamManager) isInCooldown(upstream string) bool {
	u.mutex.RLock()
	defer u.mutex.RUnlock()

	failures := u.failureMap[upstream]
	lastAttempt := u.lastAttempt[upstream]

	// If no failures, not in cooldown
	if failures == 0 {
		return false
	}

	// Calculate cooldown duration based on failures (exponential backoff)
	cooldownDuration := time.Duration(failures) * 10 * time.Second
	if cooldownDuration > 5*time.Minute {
		cooldownDuration = 5 * time.Minute
	}

	return time.Since(lastAttempt) < cooldownDuration
}

// UpdateUpstreams updates the list of upstream servers
func (u *UpstreamManager) UpdateUpstreams(upstreams []string) {
	u.mutex.Lock()
	defer u.mutex.Unlock()

	u.upstreams = upstreams

	// Clear failure map for removed servers
	newMap := make(map[string]int)
	newAttempt := make(map[string]time.Time)

	for _, upstream := range upstreams {
		if count, ok := u.failureMap[upstream]; ok {
			newMap[upstream] = count
		}
		if attempt, ok := u.lastAttempt[upstream]; ok {
			newAttempt[upstream] = attempt
		}
	}

	u.failureMap = newMap
	u.lastAttempt = newAttempt

	log.Printf("Upstream DNS servers updated: %v", upstreams)
}

// GetUpstreams returns current upstream list
func (u *UpstreamManager) GetUpstreams() []string {
	u.mutex.RLock()
	defer u.mutex.RUnlock()

	upstreams := make([]string, len(u.upstreams))
	copy(upstreams, u.upstreams)
	return upstreams
}

// GetHealth returns health status of each upstream
func (u *UpstreamManager) GetHealth() map[string]UpstreamHealth {
	u.mutex.RLock()
	defer u.mutex.RUnlock()

	health := make(map[string]UpstreamHealth)

	for _, upstream := range u.upstreams {
		failures := u.failureMap[upstream]
		lastAttempt := u.lastAttempt[upstream]
		inCooldown := u.isInCooldown(upstream)

		status := "healthy"
		if failures > 0 {
			status = "degraded"
		}
		if inCooldown {
			status = "unhealthy"
		}

		health[upstream] = UpstreamHealth{
			Status:      status,
			Failures:    failures,
			LastAttempt: lastAttempt,
			InCooldown:  inCooldown,
		}
	}

	return health
}

// UpstreamHealth represents health status of an upstream server
type UpstreamHealth struct {
	Status      string    `json:"status"`
	Failures    int       `json:"failures"`
	LastAttempt time.Time `json:"last_attempt"`
	InCooldown  bool      `json:"in_cooldown"`
}
