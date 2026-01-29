package dns_server

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net/http"
	"redock/platform/memory"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ClientRules holds cached client-specific rules
type ClientRules struct {
	Blocked        bool             // Client IP banned
	BlockedDomains map[string]bool  // Exact match blocked domains
	AllowedDomains map[string]bool  // Exact match allowed domains
	RegexRules     []*regexp.Regexp // Pre-compiled regex rules
	WildcardRules  []string         // Wildcard patterns
	AllowRegex     []*regexp.Regexp // Pre-compiled allow regex
	AllowWildcard  []string         // Allow wildcard patterns
	LastUpdate     time.Time
}

// FilterEngine manages domain filtering (blocklists and custom filters)
type FilterEngine struct {
	db               *memory.Database
	blockedDomains   map[string]bool
	whitelistDomains map[string]bool
	regexFilters     []*regexp.Regexp
	wildcardFilters  []string
	mutex            sync.RWMutex
	lastUpdate       time.Time

	// Client rules cache (NEW - Performance optimization)
	clientRulesCache map[string]*ClientRules // clientIP -> rules
	clientCacheMutex sync.RWMutex
	clientCacheTTL   time.Duration
}

// NewFilterEngine creates a new filter engine
func NewFilterEngine(db *memory.Database) *FilterEngine {
	return &FilterEngine{
		db:               db,
		blockedDomains:   make(map[string]bool),
		whitelistDomains: make(map[string]bool),
		regexFilters:     make([]*regexp.Regexp, 0),
		wildcardFilters:  make([]string, 0),
		clientRulesCache: make(map[string]*ClientRules),
		clientCacheTTL:   5 * time.Minute, // Cache for 5 minutes
	}
}

// LoadFilters loads all filters from database
func (f *FilterEngine) LoadFilters() error {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	// Clear existing filters
	f.blockedDomains = make(map[string]bool)
	f.whitelistDomains = make(map[string]bool)
	f.regexFilters = make([]*regexp.Regexp, 0)
	f.wildcardFilters = make([]string, 0)

	// Load custom filters
	if err := f.loadCustomFilters(); err != nil {
		return fmt.Errorf("failed to load custom filters: %w", err)
	}

	// Load blocklists
	if err := f.loadBlocklists(); err != nil {
		return fmt.Errorf("failed to load blocklists: %w", err)
	}

	f.lastUpdate = time.Now()

	return nil
}

// loadCustomFilters loads custom blacklist/whitelist from database
func (f *FilterEngine) loadCustomFilters() error {
	filters := memory.FindAll[*DNSCustomFilter](f.db, "dns_custom_filters")

	for _, filter := range filters {
		domain := strings.TrimSpace(strings.ToLower(filter.Domain))
		domain = strings.TrimSuffix(domain, ".") // Remove trailing dot

		if filter.IsRegex {
			if re, err := regexp.Compile(domain); err == nil {
				f.regexFilters = append(f.regexFilters, re)
			} else {
				log.Printf("Warning: Invalid regex filter: %s - %v", domain, err)
			}
		} else if filter.IsWildcard {
			f.wildcardFilters = append(f.wildcardFilters, domain)
		} else {
			if filter.Type == "blacklist" {
				f.blockedDomains[domain] = true
			} else if filter.Type == "whitelist" {
				f.whitelistDomains[domain] = true
			}
		}
	}

	return nil
}

// loadBlocklists loads enabled blocklists
func (f *FilterEngine) loadBlocklists() error {
	blocklists := memory.Filter[*DNSBlocklist](f.db, "dns_blocklists", func(b *DNSBlocklist) bool {
		return b.Enabled
	})

	for _, blocklist := range blocklists {
		// Check if needs update
		if blocklist.LastUpdated == nil ||
			time.Since(*blocklist.LastUpdated) > time.Duration(blocklist.UpdateInterval)*time.Second {
			go f.updateBlocklist(blocklist)
		}
	}

	return nil
}

// updateBlocklist downloads and updates a blocklist
func (f *FilterEngine) updateBlocklist(blocklist *DNSBlocklist) {
	resp, err := http.Get(blocklist.URL)
	if err != nil {
		f.handleBlocklistError(blocklist, fmt.Errorf("download failed: %w", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		f.handleBlocklistError(blocklist, fmt.Errorf("HTTP %d", resp.StatusCode))
		return
	}

	domains, err := f.parseBlocklist(resp.Body, blocklist.Format)
	if err != nil {
		f.handleBlocklistError(blocklist, fmt.Errorf("parse failed: %w", err))
		return
	}

	// Add domains to blocked list
	f.mutex.Lock()
	for _, domain := range domains {
		f.blockedDomains[domain] = true
	}
	f.mutex.Unlock()

	// Update blocklist record
	now := time.Now()
	blocklist.LastUpdated = &now
	blocklist.DomainCount = len(domains)
	blocklist.LastError = ""

	if err := memory.Update[*DNSBlocklist](f.db, "dns_blocklists", blocklist); err != nil {
		log.Printf("Failed to update blocklist record: %v", err)
	}
}

// DetectBlocklistFormat detects the format of a blocklist by analyzing its content
func (f *FilterEngine) DetectBlocklistFormat(reader io.Reader) string {
	scanner := bufio.NewScanner(reader)

	hostsCount := 0
	adblockCount := 0
	domainCount := 0
	totalLines := 0

	// Analyze first 100 non-empty, non-comment lines
	for scanner.Scan() && totalLines < 100 {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}

		totalLines++

		// Check for adblock format FIRST (||domain^)
		// This must be checked before plain domain format
		if strings.HasPrefix(line, "||") && strings.Contains(line, "^") {
			adblockCount++
			continue // Don't check other formats for this line
		}

		// Check for hosts format (starts with IP address)
		if strings.HasPrefix(line, "0.0.0.0 ") || strings.HasPrefix(line, "127.0.0.1 ") ||
			strings.HasPrefix(line, "::1 ") || strings.HasPrefix(line, ":: ") {
			hostsCount++
			continue
		}

		// Check for plain domain format (only if not adblock or hosts)
		parts := strings.Fields(line)
		if len(parts) == 1 && f.isValidDomain(parts[0]) {
			domainCount++
		}
	}

	// Determine format based on analysis
	if adblockCount > hostsCount && adblockCount > domainCount {
		return "adblock"
	}
	if hostsCount > adblockCount && hostsCount > domainCount {
		return "hosts"
	}
	// Default to domains format
	return "domains"
}

// parseBlocklist parses blocklist based on format
func (f *FilterEngine) parseBlocklist(reader io.Reader, format string) ([]string, error) {
	// Read all content first for format detection if needed
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	// Auto-detect format if not specified or empty
	if format == "" || format == "auto" {
		format = f.DetectBlocklistFormat(strings.NewReader(string(content)))
		log.Printf("📋 Auto-detected format: %s", format)
	}

	domains := make([]string, 0)
	scanner := bufio.NewScanner(strings.NewReader(string(content)))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}

		var domain string

		switch format {
		case "hosts":
			// Format: 0.0.0.0 domain.com or 127.0.0.1 domain.com
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				domain = strings.ToLower(parts[1])
			}
		case "domains":
			// Format: domain.com
			domain = strings.ToLower(line)
		case "adblock":
			// AdBlock Plus format: ||domain.com^
			if strings.HasPrefix(line, "||") && strings.HasSuffix(line, "^") {
				domain = strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(line, "||"), "^"))
			}
		default:
			// Try to parse as domain
			domain = strings.ToLower(line)
		}

		if domain != "" && f.isValidDomain(domain) {
			domains = append(domains, domain)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return domains, nil
}

// isValidDomain checks if domain is valid
func (f *FilterEngine) isValidDomain(domain string) bool {
	if len(domain) == 0 || len(domain) > 253 {
		return false
	}

	// Basic domain validation
	if strings.Contains(domain, " ") || strings.Contains(domain, "\t") {
		return false
	}

	return true
}

// handleBlocklistError handles blocklist update errors
func (f *FilterEngine) handleBlocklistError(blocklist *DNSBlocklist, err error) {
	log.Printf("❌ Failed to update blocklist %s: %v", blocklist.Name, err)

	blocklist.LastError = err.Error()
	if updateErr := memory.Update[*DNSBlocklist](f.db, "dns_blocklists", blocklist); updateErr != nil {
		log.Printf("Failed to save blocklist error: %v", updateErr)
	}
}

// getClientRules returns cached client rules or loads them from DB
func (f *FilterEngine) getClientRules(clientIP string) *ClientRules {
	// Check cache first
	f.clientCacheMutex.RLock()
	if cached, exists := f.clientRulesCache[clientIP]; exists {
		if time.Since(cached.LastUpdate) < f.clientCacheTTL {
			f.clientCacheMutex.RUnlock()
			return cached
		}
	}
	f.clientCacheMutex.RUnlock()

	// Load from DB
	rules := &ClientRules{
		BlockedDomains: make(map[string]bool),
		AllowedDomains: make(map[string]bool),
		RegexRules:     make([]*regexp.Regexp, 0),
		WildcardRules:  make([]string, 0),
		AllowRegex:     make([]*regexp.Regexp, 0),
		AllowWildcard:  make([]string, 0),
		LastUpdate:     time.Now(),
	}

	// Check if client is banned
	clientSettings := memory.Filter[*DNSClientSettings](f.db, "dns_client_settings", func(c *DNSClientSettings) bool {
		return c.ClientIP == clientIP
	})
	if len(clientSettings) > 0 {
		rules.Blocked = clientSettings[0].Blocked
	}

	// Load client-specific domain rules
	domainRules := memory.Filter[*DNSClientDomainRule](f.db, "dns_client_rules", func(r *DNSClientDomainRule) bool {
		return r.ClientIP == clientIP
	})

	for _, rule := range domainRules {
		domain := strings.TrimSpace(strings.ToLower(rule.Domain))
		domain = strings.TrimSuffix(domain, ".")

		if rule.Type == "block" {
			if rule.IsRegex {
				if re, err := regexp.Compile(domain); err == nil {
					rules.RegexRules = append(rules.RegexRules, re)
				}
			} else if rule.IsWildcard {
				rules.WildcardRules = append(rules.WildcardRules, domain)
			} else {
				rules.BlockedDomains[domain] = true
			}
		} else if rule.Type == "allow" {
			if rule.IsRegex {
				if re, err := regexp.Compile(domain); err == nil {
					rules.AllowRegex = append(rules.AllowRegex, re)
				}
			} else if rule.IsWildcard {
				rules.AllowWildcard = append(rules.AllowWildcard, domain)
			} else {
				rules.AllowedDomains[domain] = true
			}
		}
	}

	// Cache the rules
	f.clientCacheMutex.Lock()
	f.clientRulesCache[clientIP] = rules
	f.clientCacheMutex.Unlock()

	return rules
}

// InvalidateClientCache invalidates cache for a specific client
func (f *FilterEngine) InvalidateClientCache(clientIP string) {
	f.clientCacheMutex.Lock()
	delete(f.clientRulesCache, clientIP)
	f.clientCacheMutex.Unlock()
}

// ClearClientCache clears all cached client rules
func (f *FilterEngine) ClearClientCache() {
	f.clientCacheMutex.Lock()
	f.clientRulesCache = make(map[string]*ClientRules)
	f.clientCacheMutex.Unlock()
}

// ShouldBlock checks if a domain should be blocked
// Priority order:
// 1. Client IP Ban -> Block everything
// 2. Global Whitelist -> Allow
// 3. Client-specific Whitelist -> Allow
// 4. Client-specific Blacklist -> Block
// 5. Global Blacklist -> Block
// 6. Allow
func (f *FilterEngine) ShouldBlock(domain string, clientIP string) (bool, string) {
	domain = strings.TrimSpace(strings.ToLower(domain))
	domain = strings.TrimSuffix(domain, ".")

	// Get client rules from cache (FAST!)
	clientRules := f.getClientRules(clientIP)

	// 1. Check if client is banned (from cache)
	if clientRules.Blocked {
		return true, "client IP banned"
	}

	f.mutex.RLock()
	defer f.mutex.RUnlock()

	// 2. Check global whitelist first
	if f.whitelistDomains[domain] {
		return false, ""
	}
	if f.isParentWhitelisted(domain) {
		return false, ""
	}

	// 3. Check client-specific whitelist (from cache)
	if clientRules.AllowedDomains[domain] {
		return false, ""
	}
	// Check client allow regex (pre-compiled)
	for _, re := range clientRules.AllowRegex {
		if re.MatchString(domain) {
			return false, ""
		}
	}
	// Check client allow wildcard
	for _, wildcard := range clientRules.AllowWildcard {
		if f.matchWildcard(domain, wildcard) {
			return false, ""
		}
	}

	// 4. Check client-specific blacklist (from cache)
	if clientRules.BlockedDomains[domain] {
		return true, "client-specific block"
	}
	// Check client block regex (pre-compiled)
	for _, re := range clientRules.RegexRules {
		if re.MatchString(domain) {
			return true, "client-specific regex block"
		}
	}
	// Check client block wildcard
	for _, wildcard := range clientRules.WildcardRules {
		if f.matchWildcard(domain, wildcard) {
			return true, "client-specific wildcard block"
		}
	}

	// 5. Check global blocklist
	if f.blockedDomains[domain] {
		return true, "blocklist"
	}
	if f.isParentBlocked(domain) {
		return true, "blocklist (parent)"
	}

	// Check global wildcard filters
	for _, wildcard := range f.wildcardFilters {
		if f.matchWildcard(domain, wildcard) {
			return true, "wildcard filter"
		}
	}

	// Check global regex filters
	for _, re := range f.regexFilters {
		if re.MatchString(domain) {
			return true, "regex filter"
		}
	}

	return false, ""
}

// isParentBlocked checks if any parent domain is blocked
func (f *FilterEngine) isParentBlocked(domain string) bool {
	parts := strings.Split(domain, ".")

	// Check each parent domain
	for i := 1; i < len(parts); i++ {
		parentDomain := strings.Join(parts[i:], ".")
		if f.blockedDomains[parentDomain] {
			return true
		}
	}

	return false
}

// isParentWhitelisted checks if any parent domain is whitelisted
func (f *FilterEngine) isParentWhitelisted(domain string) bool {
	parts := strings.Split(domain, ".")

	// Check each parent domain
	for i := 1; i < len(parts); i++ {
		parentDomain := strings.Join(parts[i:], ".")
		if f.whitelistDomains[parentDomain] {
			return true
		}
	}

	return false
}

// matchWildcard matches domain against wildcard pattern
func (f *FilterEngine) matchWildcard(domain, pattern string) bool {
	// Simple wildcard matching (* matches any characters)
	if pattern == "*" {
		return true
	}

	if strings.HasPrefix(pattern, "*.") {
		// *.example.com matches any subdomain of example.com
		suffix := strings.TrimPrefix(pattern, "*.")
		return strings.HasSuffix(domain, "."+suffix) || domain == suffix
	}

	if strings.HasSuffix(pattern, ".*") {
		// example.* matches example.com, example.net, etc.
		prefix := strings.TrimSuffix(pattern, ".*")
		return strings.HasPrefix(domain, prefix+".")
	}

	return domain == pattern
}

// isClientDomainBlocked checks if domain is blocked for specific client
func (f *FilterEngine) isClientDomainBlocked(clientIP, domain string) (bool, string) {
	// Get all block rules for this client
	rules := memory.Filter[*DNSClientDomainRule](f.db, "dns_client_rules", func(r *DNSClientDomainRule) bool {
		return r.ClientIP == clientIP && r.Type == "block"
	})

	// Check exact match
	for _, rule := range rules {
		if !rule.IsRegex && !rule.IsWildcard && rule.Domain == domain {
			return true, "client-specific block"
		}
	}

	// Check regex rules
	for _, rule := range rules {
		if rule.IsRegex {
			if re, err := regexp.Compile(rule.Domain); err == nil {
				if re.MatchString(domain) {
					return true, "client-specific regex block"
				}
			}
		}
	}

	// Check wildcard rules
	for _, rule := range rules {
		if rule.IsWildcard && f.matchWildcard(domain, rule.Domain) {
			return true, "client-specific wildcard block"
		}
	}

	return false, ""
}

// isClientDomainAllowed checks if domain is whitelisted for specific client
func (f *FilterEngine) isClientDomainAllowed(clientIP, domain string) bool {
	// Get all allow rules for this client
	rules := memory.Filter[*DNSClientDomainRule](f.db, "dns_client_rules", func(r *DNSClientDomainRule) bool {
		return r.ClientIP == clientIP && r.Type == "allow"
	})

	// Check exact match
	for _, rule := range rules {
		if !rule.IsRegex && !rule.IsWildcard && rule.Domain == domain {
			return true
		}
	}

	// Check regex rules
	for _, rule := range rules {
		if rule.IsRegex {
			if re, err := regexp.Compile(rule.Domain); err == nil {
				if re.MatchString(domain) {
					return true
				}
			}
		}
	}

	// Check wildcard rules
	for _, rule := range rules {
		if rule.IsWildcard && f.matchWildcard(domain, rule.Domain) {
			return true
		}
	}

	return false
}

// GetStats returns filter statistics
func (f *FilterEngine) GetStats() FilterStats {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	return FilterStats{
		BlockedDomains:   len(f.blockedDomains),
		WhitelistDomains: len(f.whitelistDomains),
		RegexFilters:     len(f.regexFilters),
		WildcardFilters:  len(f.wildcardFilters),
		LastUpdate:       f.lastUpdate,
	}
}

// IsGloballyBlocked checks if a domain is in the global blocklist
func (f *FilterEngine) IsGloballyBlocked(domain string) bool {
	domain = strings.TrimSpace(strings.ToLower(domain))
	domain = strings.TrimSuffix(domain, ".")

	f.mutex.RLock()
	defer f.mutex.RUnlock()

	// Check exact match
	if f.blockedDomains[domain] {
		return true
	}

	// Check if any parent domain is blocked
	if f.isParentBlocked(domain) {
		return true
	}

	// Check wildcard filters
	for _, wildcard := range f.wildcardFilters {
		if f.matchWildcard(domain, wildcard) {
			return true
		}
	}

	// Check regex filters
	for _, re := range f.regexFilters {
		if re.MatchString(domain) {
			return true
		}
	}

	return false
}

// IsClientBlocked checks if a domain is blocked for a specific client
func (f *FilterEngine) IsClientBlocked(clientIP, domain string) bool {
	blocked, _ := f.isClientDomainBlocked(clientIP, domain)
	return blocked
}

// IsClientBanned checks if a client IP is banned
func (f *FilterEngine) IsClientBanned(clientIP string) bool {
	settings := memory.Filter[*DNSClientSettings](f.db, "dns_client_settings", func(c *DNSClientSettings) bool {
		return c.ClientIP == clientIP
	})
	if len(settings) > 0 && settings[0].Blocked {
		return true
	}
	return false
}

// FilterStats represents filter statistics
type FilterStats struct {
	BlockedDomains   int       `json:"blocked_domains"`
	WhitelistDomains int       `json:"whitelist_domains"`
	RegexFilters     int       `json:"regex_filters"`
	WildcardFilters  int       `json:"wildcard_filters"`
	LastUpdate       time.Time `json:"last_update"`
}
