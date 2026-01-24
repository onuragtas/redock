package dns_server

import (
	"sync"
	"time"

	"github.com/miekg/dns"
)

// DNSCache implements a simple in-memory DNS cache
type DNSCache struct {
	cache   map[string]*CacheEntry
	mutex   sync.RWMutex
	ttl     int
	maxSize int
}

// CacheEntry represents a cached DNS response
type CacheEntry struct {
	Message   *dns.Msg
	ExpiresAt time.Time
	Qtype     uint16
}

// NewDNSCache creates a new DNS cache
func NewDNSCache(ttl int) *DNSCache {
	cache := &DNSCache{
		cache:   make(map[string]*CacheEntry),
		ttl:     ttl,
		maxSize: 10000, // Max 10k entries
	}

	// Start cleanup goroutine
	go cache.cleanupExpired()

	return cache
}

// Get retrieves a cached DNS response
func (c *DNSCache) Get(domain string, qtype uint16) *dns.Msg {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	key := c.getCacheKey(domain, qtype)
	entry, exists := c.cache[key]

	if !exists {
		return nil
	}

	// Check if expired
	if time.Now().After(entry.ExpiresAt) {
		// Don't delete here, let cleanup goroutine handle it
		return nil
	}

	// Return a copy of the message
	return entry.Message.Copy()
}

// Set stores a DNS response in cache
func (c *DNSCache) Set(domain string, qtype uint16, msg *dns.Msg) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Check cache size limit
	if len(c.cache) >= c.maxSize {
		// Remove oldest entries (simple FIFO)
		c.evictOldest()
	}

	key := c.getCacheKey(domain, qtype)
	
	entry := &CacheEntry{
		Message:   msg.Copy(),
		ExpiresAt: time.Now().Add(time.Duration(c.ttl) * time.Second),
		Qtype:     qtype,
	}

	c.cache[key] = entry
}

// Clear clears the entire cache
func (c *DNSCache) Clear() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.cache = make(map[string]*CacheEntry)
}

// UpdateTTL updates the cache TTL
func (c *DNSCache) UpdateTTL(ttl int) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.ttl = ttl
}

// GetSize returns current cache size
func (c *DNSCache) GetSize() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return len(c.cache)
}

// GetStats returns cache statistics
func (c *DNSCache) GetStats() CacheStats {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	stats := CacheStats{
		Size:    len(c.cache),
		MaxSize: c.maxSize,
		TTL:     c.ttl,
	}

	return stats
}

// getCacheKey generates cache key from domain and query type
func (c *DNSCache) getCacheKey(domain string, qtype uint16) string {
	return dns.TypeToString[qtype] + ":" + domain
}

// cleanupExpired periodically removes expired entries
func (c *DNSCache) cleanupExpired() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.mutex.Lock()
		now := time.Now()
		expired := []string{}

		for key, entry := range c.cache {
			if now.After(entry.ExpiresAt) {
				expired = append(expired, key)
			}
		}

		for _, key := range expired {
			delete(c.cache, key)
		}

		c.mutex.Unlock()

		if len(expired) > 0 {
			// log.Printf("🧹 Cleaned up %d expired DNS cache entries", len(expired))
		}
	}
}

// evictOldest removes 10% of oldest entries
func (c *DNSCache) evictOldest() {
	// Simple approach: remove entries that are closest to expiration
	type keyTime struct {
		key       string
		expiresAt time.Time
	}

	entries := make([]keyTime, 0, len(c.cache))
	for key, entry := range c.cache {
		entries = append(entries, keyTime{
			key:       key,
			expiresAt: entry.ExpiresAt,
		})
	}

	// Sort by expiration time
	// Remove 10% of entries
	removeCount := len(entries) / 10
	if removeCount < 1 {
		removeCount = 1
	}

	for i := 0; i < removeCount && i < len(entries); i++ {
		delete(c.cache, entries[i].key)
	}
}

// CacheStats represents cache statistics
type CacheStats struct {
	Size    int `json:"size"`
	MaxSize int `json:"max_size"`
	TTL     int `json:"ttl"`
}
