package dns_server

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	dockermanager "redock/docker-manager"
	"redock/platform/memory"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

var (
	dnsServerInstance *DNSServer
	dnsServerOnce     sync.Once
)

// DNSServer manages DNS server operations
type DNSServer struct {
	db              *memory.Database
	dockerManager   *dockermanager.DockerEnvironmentManager
	config          *DNSConfig
	udpServer       *dns.Server
	tcpServer       *dns.Server
	dohServer       *dns.Server
	dotServer       *dns.Server
	filterEngine    *FilterEngine
	upstreamManager *UpstreamManager
	cache           *DNSCache
	stats           *StatsCollector
	jsonlWriter     *DNSLogWriter
	mutex           sync.RWMutex
	running         bool
	ctx             context.Context
	cancel          context.CancelFunc

	// Log buffering: single writer goroutine with in-memory buffer
	logChannel chan DNSQueryLog // Async log channel (non-blocking)
}

// GetDNSServer returns singleton instance
func GetDNSServer() *DNSServer {
	dnsServerOnce.Do(func() {
		dnsServerInstance = &DNSServer{
			running: false,
		}
	})
	return dnsServerInstance
}

// Init initializes DNS server with database
func (s *DNSServer) Init(db *memory.Database, dockerManager *dockermanager.DockerEnvironmentManager) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.db = db
	s.dockerManager = dockerManager
	s.ctx, s.cancel = context.WithCancel(context.Background())

	// Load or create config
	if err := s.loadConfig(); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Load default blocklists if none exist
	if err := s.loadDefaultBlocklists(); err != nil {
		log.Printf("Warning: Failed to load default blocklists: %v", err)
	}

	// Initialize components
	s.filterEngine = NewFilterEngine(db)
	s.upstreamManager = NewUpstreamManager(s.config.GetUpstreamDNSList())
	s.cache = NewDNSCache(s.config.CacheTTL)
	s.stats = NewStatsCollector(db)

	// Initialize JSONL log writer
	jsonlWriter, err := NewDNSLogWriter(dockerManager)
	if err != nil {
		return fmt.Errorf("failed to initialize log writer: %w", err)
	}
	s.jsonlWriter = jsonlWriter
	
	// Set log writer for stats (for historical query reading)
	s.stats.SetLogWriter(jsonlWriter)

	// Initialize log channel for async logging (single writer goroutine)
	s.logChannel = make(chan DNSQueryLog, 1000) // Large buffer to avoid blocking DNS queries

	// Start single writer goroutine
	go s.logWriter()

	// Load filters
	if err := s.filterEngine.LoadFilters(); err != nil {
		log.Printf("Warning: Failed to load filters: %v", err)
	}

	// Preload stats from last 24 hours (async, non-blocking)
	go s.preloadStats()

	return nil
}

// preloadStats loads stats from JSONL files to populate in-memory counters after restart
func (s *DNSServer) preloadStats() {
	log.Printf("🔄 Preloading stats from last 24 hours...")
	startTime := time.Now()
	
	since := time.Now().Add(-24 * time.Hour)
	var count int64
	
	err := s.jsonlWriter.ReadLogs(since, func(logEntry DNSQueryLog) error {
		// Update in-memory stats just like live queries
		responseTimeMicros := int64(logEntry.ResponseTime) * 1000 // ms to microseconds
		s.stats.RecordQueryDetails(logEntry.Domain, logEntry.ClientIP, logEntry.Blocked, logEntry.Cached, responseTimeMicros)
		count++
		return nil
	})
	
	if err != nil {
		log.Printf("⚠️  Failed to preload stats: %v", err)
		return
	}
	
	duration := time.Since(startTime)
	log.Printf("✅ Preloaded %d queries from last 24 hours in %v", count, duration)
}

// loadConfig loads or creates default config
func (s *DNSServer) loadConfig() error {
	// Get all configs (should only be one)
	configs := memory.FindAll[*DNSConfig](s.db, "dns_config")

	if len(configs) == 0 {
		// Create default config
		config := &DNSConfig{
			Enabled:          false,
			UDPPort:          53,
			TCPPort:          53,
			DoHEnabled:       false,
			DoHPort:          443,
			DoTEnabled:       false,
			DoTPort:          853,
			BlockingEnabled:  true,
			QueryLogging:     true,
			LogRetentionDays: 7,
			CacheEnabled:     true,
			CacheTTL:         3600,
		}
		config.SetUpstreamDNSList([]string{
			"94.140.14.14:53",
			"94.140.15.15:53",
			"1.1.1.1:53",
			"8.8.8.8:53",
		})

		if err := memory.Create[*DNSConfig](s.db, "dns_config", config); err != nil {
			return err
		}
		s.config = config
	} else {
		s.config = configs[0]
	}

	return nil
}

// loadDefaultBlocklists loads default blocklists if none exist
func (s *DNSServer) loadDefaultBlocklists() error {
	// Check if any blocklists exist
	existingBlocklists := memory.FindAll[*DNSBlocklist](s.db, "dns_blocklists")
	
	if len(existingBlocklists) == 0 {
		// Get default blocklists
		defaultBlocklists := GetDefaultBlocklists()
		
		// Add each default blocklist to database
		for _, blocklist := range defaultBlocklists {
			bl := blocklist // Create a copy for pointer
			if err := memory.Create[*DNSBlocklist](s.db, "dns_blocklists", &bl); err != nil {
				log.Printf("⚠️  Failed to create default blocklist %s: %v", bl.Name, err)
				continue
			}
		}
	}
	
	return nil
}

// Start starts all enabled DNS servers
func (s *DNSServer) Start() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.running {
		return fmt.Errorf("DNS server already running")
	}

	if !s.config.Enabled {
		return fmt.Errorf("DNS server is disabled in configuration")
	}

	// Start UDP server
	if err := s.startUDPServer(); err != nil {
		return fmt.Errorf("failed to start UDP server: %w", err)
	}

	// Start TCP server
	if err := s.startTCPServer(); err != nil {
		s.stopUDPServer()
		return fmt.Errorf("failed to start TCP server: %w", err)
	}

	// Start DoH if enabled
	if s.config.DoHEnabled {
		if err := s.startDoHServer(); err != nil {
			log.Printf("Warning: Failed to start DoH server: %v", err)
		}
	}

	// Start DoT if enabled
	if s.config.DoTEnabled {
		if err := s.startDoTServer(); err != nil {
			log.Printf("Warning: Failed to start DoT server: %v", err)
		}
	}

	// Start background tasks
	go s.cleanupOldLogs()
	go s.updateStatistics()

	s.running = true
	return nil
}

// Stop stops all DNS servers
func (s *DNSServer) Stop() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.running {
		return nil
	}

	// Cancel context to stop log writer goroutine (it will flush remaining logs)
	s.cancel()

	// Close channel to drain remaining logs (logWriter will process them before exiting)
	close(s.logChannel)

	s.stopUDPServer()
	s.stopTCPServer()
	s.stopDoHServer()
	s.stopDoTServer()

	s.running = false
	return nil
}

// startUDPServer starts UDP DNS server
func (s *DNSServer) startUDPServer() error {
	handler := dns.HandlerFunc(s.handleDNSRequest)

	s.udpServer = &dns.Server{
		Addr:    fmt.Sprintf(":%d", s.config.UDPPort),
		Net:     "udp",
		Handler: handler,
	}

	go func() {
		if err := s.udpServer.ListenAndServe(); err != nil {
			log.Printf("UDP server error: %v", err)
		}
	}()

	log.Printf("DNS UDP server listening on port %d", s.config.UDPPort)
	return nil
}

// startTCPServer starts TCP DNS server
func (s *DNSServer) startTCPServer() error {
	handler := dns.HandlerFunc(s.handleDNSRequest)

	s.tcpServer = &dns.Server{
		Addr:    fmt.Sprintf(":%d", s.config.TCPPort),
		Net:     "tcp",
		Handler: handler,
	}

	go func() {
		if err := s.tcpServer.ListenAndServe(); err != nil {
			log.Printf("TCP server error: %v", err)
		}
	}()

	log.Printf("DNS TCP server listening on port %d", s.config.TCPPort)
	return nil
}

// startDoHServer starts DNS-over-HTTPS server
func (s *DNSServer) startDoHServer() error {
	// DoH is complex and requires HTTP/2 support
	// For now, we'll skip DoH implementation
	// You can implement using a separate HTTP server with DNS message handling
	log.Printf("⚠️  DNS-over-HTTPS (DoH) not yet fully implemented")
	return nil
}

// startDoTServer starts DNS-over-TLS server
func (s *DNSServer) startDoTServer() error {
	handler := dns.HandlerFunc(s.handleDNSRequest)

	s.dotServer = &dns.Server{
		Addr:    fmt.Sprintf(":%d", s.config.DoTPort),
		Net:     "tcp-tls",
		Handler: handler,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}

	go func() {
		// Note: You need to provide certificate files for TLS
		// For now, this is a placeholder
		log.Printf("⚠️  DNS-over-TLS (DoT) requires TLS certificates")
		// if err := s.dotServer.ListenAndServe(); err != nil {
		// 	log.Printf("DoT server error: %v", err)
		// }
	}()

	log.Printf("DNS-over-TLS server configured on port %d (certificates required)", s.config.DoTPort)
	return nil
}

// stopUDPServer stops UDP server
func (s *DNSServer) stopUDPServer() {
	if s.udpServer != nil {
		s.udpServer.Shutdown()
	}
}

// stopTCPServer stops TCP server
func (s *DNSServer) stopTCPServer() {
	if s.tcpServer != nil {
		s.tcpServer.Shutdown()
	}
}

// stopDoHServer stops DoH server
func (s *DNSServer) stopDoHServer() {
	if s.dohServer != nil {
		s.dohServer.Shutdown()
	}
}

// stopDoTServer stops DoT server
func (s *DNSServer) stopDoTServer() {
	if s.dotServer != nil {
		s.dotServer.Shutdown()
	}
}

// handleDNSRequest handles incoming DNS queries
func (s *DNSServer) handleDNSRequest(w dns.ResponseWriter, r *dns.Msg) {
	startTime := time.Now()

	msg := new(dns.Msg)
	msg.SetReply(r)
	msg.Authoritative = false
	msg.RecursionAvailable = true

	if len(r.Question) == 0 {
		w.WriteMsg(msg)
		return
	}

	question := r.Question[0]
	domain := question.Name
	qtype := dns.TypeToString[question.Qtype]

	clientIP := getClientIP(w)

	// Check cache first
	var cached bool
	var blocked bool
	var blockReason string

	if s.config.CacheEnabled {
		if cachedMsg := s.cache.Get(domain, question.Qtype); cachedMsg != nil {
			cachedMsg.SetReply(r)
			w.WriteMsg(cachedMsg)
			cached = true

			// Log query
			if s.config.QueryLogging {
				s.logQuery(clientIP, domain, qtype, cachedMsg, blocked, blockReason, time.Since(startTime), cached)
			}
			return
		}
	}

	// Check if domain should be blocked
	if s.config.BlockingEnabled {
		shouldBlock, reason := s.filterEngine.ShouldBlock(domain, clientIP)
		if shouldBlock {
			blocked = true
			blockReason = reason

			// Return NXDOMAIN
			msg.Rcode = dns.RcodeNameError
			w.WriteMsg(msg)

			// Log blocked query
			if s.config.QueryLogging {
				s.logQuery(clientIP, domain, qtype, msg, blocked, blockReason, time.Since(startTime), false)
			}
			return
		}
	}

	// Check for DNS rewrites
	if rewrite := s.getRewrite(domain, question.Qtype); rewrite != nil {
		msg = rewrite
		msg.SetReply(r)
		w.WriteMsg(msg)

		if s.config.QueryLogging {
			s.logQuery(clientIP, domain, qtype, msg, false, "Rewrite", time.Since(startTime), false)
		}
		return
	}

	// Forward to upstream DNS
	response, err := s.upstreamManager.Query(r)
	if err != nil {
		log.Printf("Upstream query error for %s: %v", domain, err)
		msg.Rcode = dns.RcodeServerFailure
		w.WriteMsg(msg)
		return
	}

	// Cache the response
	if s.config.CacheEnabled && response != nil {
		s.cache.Set(domain, question.Qtype, response)
	}

	// Write response
	w.WriteMsg(response)

	// Log query
	if s.config.QueryLogging {
		s.logQuery(clientIP, domain, qtype, response, blocked, blockReason, time.Since(startTime), cached)
	}
}

// getClientIP extracts client IP from DNS writer
func getClientIP(w dns.ResponseWriter) string {
	addr := w.RemoteAddr()
	if addr != nil {
		if udpAddr, ok := addr.(*net.UDPAddr); ok {
			return udpAddr.IP.String()
		}
		if tcpAddr, ok := addr.(*net.TCPAddr); ok {
			return tcpAddr.IP.String()
		}
	}
	return "unknown"
}

// logQuery sends DNS query to async channel for batching
func (s *DNSServer) logQuery(clientIP, domain, qtype string, response *dns.Msg, blocked bool, blockReason string, responseTime time.Duration, cached bool) {
	var responseStr string
	if response != nil && len(response.Answer) > 0 {
		for _, ans := range response.Answer {
			responseStr += ans.String() + "; "
		}
	}

	logEntry := DNSQueryLog{
		CreatedAt:    time.Now(),
		ClientIP:     clientIP,
		Domain:       domain,
		QueryType:    qtype,
		Response:     responseStr,
		Blocked:      blocked,
		BlockReason:  blockReason,
		ResponseTime: int(responseTime.Milliseconds()),
		Cached:       cached,
	}

	// Non-blocking send to channel (drop if channel full to avoid DNS slowdown)
	select {
	case s.logChannel <- logEntry:
		// Log sent successfully
	default:
		// Channel full, drop log (better than blocking DNS queries)
	}
}

// logWriter is the single writer goroutine that processes logs from channel
func (s *DNSServer) logWriter() {
	defer func() {
		// Stop the JSONL log writer on shutdown
		if s.jsonlWriter != nil {
			s.jsonlWriter.Stop()
		}
	}()

	for {
		select {
		case <-s.ctx.Done():
			// Drain remaining logs before exiting
			for {
				select {
				case logEntry := <-s.logChannel:
					s.jsonlWriter.LogQuery(logEntry)
					responseTimeMicros := int64(logEntry.ResponseTime) * 1000 // ms to microseconds
					s.stats.RecordQueryDetails(logEntry.Domain, logEntry.ClientIP, logEntry.Blocked, logEntry.Cached, responseTimeMicros)
				default:
					return
				}
			}

		case logEntry := <-s.logChannel:
			// Write to JSONL file
			s.jsonlWriter.LogQuery(logEntry)
			// Update real-time stats with domain/client tracking
			responseTimeMicros := int64(logEntry.ResponseTime) * 1000 // ms to microseconds
			s.stats.RecordQueryDetails(logEntry.Domain, logEntry.ClientIP, logEntry.Blocked, logEntry.Cached, responseTimeMicros)
		}
	}
}

// getRewrite checks for DNS rewrite rules
// getRewrite checks for DNS rewrite rules with wildcard support
func (s *DNSServer) getRewrite(domain string, qtype uint16) *dns.Msg {
	qtypeStr := dns.TypeToString[qtype]

	// Normalize domain (remove trailing dot for comparison)
	normalizedDomain := strings.TrimSuffix(domain, ".")

	// Get all enabled rewrites for this query type
	rewrites := memory.Filter[*DNSRewrite](s.db, "dns_rewrites", func(r *DNSRewrite) bool {
		return r.Enabled && r.Type == qtypeStr
	})

	// Check for exact match first
	for _, rewrite := range rewrites {
		rewriteDomain := strings.TrimSuffix(rewrite.Domain, ".")
		if rewriteDomain == normalizedDomain {
			return s.buildRewriteResponse(domain, qtype, rewrite)
		}
	}

	// Check for wildcard match (*.example.org)
	for _, rewrite := range rewrites {
		rewriteDomain := strings.TrimSuffix(rewrite.Domain, ".")
		if strings.HasPrefix(rewriteDomain, "*.") {
			baseDomain := strings.TrimPrefix(rewriteDomain, "*.")
			if strings.HasSuffix(normalizedDomain, "."+baseDomain) || normalizedDomain == baseDomain {
				return s.buildRewriteResponse(domain, qtype, rewrite)
			}
		}
	}

	return nil
}

// buildRewriteResponse builds DNS response for rewrite rule
func (s *DNSServer) buildRewriteResponse(domain string, qtype uint16, rewrite *DNSRewrite) *dns.Msg {
	msg := new(dns.Msg)

	// Ensure domain is in FQDN format (with trailing dot)
	fqdnDomain := dns.Fqdn(domain)

	// Special values: A and AAAA mean "keep upstream records"
	// These will be handled by not returning a rewrite response
	if rewrite.Answer == "A" || rewrite.Answer == "AAAA" {
		return nil
	}

	switch qtype {
	case dns.TypeA:
		if rewrite.Type == "CNAME" {
			// Return CNAME record
			rr := &dns.CNAME{
				Hdr: dns.RR_Header{
					Name:   fqdnDomain,
					Rrtype: dns.TypeCNAME,
					Class:  dns.ClassINET,
					Ttl:    300,
				},
				Target: dns.Fqdn(rewrite.Answer),
			}
			msg.Answer = append(msg.Answer, rr)
		} else {
			// Return A record
			rr := &dns.A{
				Hdr: dns.RR_Header{
					Name:   fqdnDomain,
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    300,
				},
				A: net.ParseIP(rewrite.Answer),
			}
			if rr.A != nil {
				msg.Answer = append(msg.Answer, rr)
			}
		}
	case dns.TypeAAAA:
		if rewrite.Type == "CNAME" {
			// Return CNAME record
			rr := &dns.CNAME{
				Hdr: dns.RR_Header{
					Name:   fqdnDomain,
					Rrtype: dns.TypeCNAME,
					Class:  dns.ClassINET,
					Ttl:    300,
				},
				Target: dns.Fqdn(rewrite.Answer),
			}
			msg.Answer = append(msg.Answer, rr)
		} else {
			// Return AAAA record
			rr := &dns.AAAA{
				Hdr: dns.RR_Header{
					Name:   fqdnDomain,
					Rrtype: dns.TypeAAAA,
					Class:  dns.ClassINET,
					Ttl:    300,
				},
				AAAA: net.ParseIP(rewrite.Answer),
			}
			if rr.AAAA != nil {
				msg.Answer = append(msg.Answer, rr)
			}
		}
	}

	return msg
}

// cleanupOldLogs removes old query logs based on retention policy
func (s *DNSServer) cleanupOldLogs() {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			// Cleanup is handled by DNSLogWriter automatically (every hour)
			// It keeps logs based on LogRetentionDays config
			if s.jsonlWriter != nil && s.config != nil {
				s.jsonlWriter.cleanup(s.config.LogRetentionDays)
			}
		}
	}
}

// updateStatistics updates daily statistics
func (s *DNSServer) updateStatistics() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.stats.UpdateDailyStats()
		}
	}
}

// IsRunning returns if server is running
func (s *DNSServer) IsRunning() bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.running
}

// GetConfig returns current config
func (s *DNSServer) GetConfig() *DNSConfig {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.config
}

// UpdateConfig updates server configuration
func (s *DNSServer) UpdateConfig(config *DNSConfig) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if err := memory.Update[*DNSConfig](s.db, "dns_config", config); err != nil {
		return err
	}

	s.config = config

	// Update components
	s.upstreamManager.UpdateUpstreams(config.GetUpstreamDNSList())
	s.cache.UpdateTTL(config.CacheTTL)

	return nil
}

// ReloadFilters reloads filter lists
func (s *DNSServer) ReloadFilters() error {
	return s.filterEngine.LoadFilters()
}

// GetDB returns database connection
func (s *DNSServer) GetDB() (*memory.Database, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return s.db, nil
}

// GetFilterEngine returns the filter engine
func (s *DNSServer) GetFilterEngine() *FilterEngine {
	return s.filterEngine
}

// GetRealtimeStats returns realtime statistics
func (s *DNSServer) GetRealtimeStats() RealtimeStats {
	return s.stats.GetRealtimeStats()
}

// GetQueryHistory returns query history
func (s *DNSServer) GetQueryHistory(hours int) []QueryHistoryPoint {
	return s.stats.GetQueryHistory(hours)
}

// GetLogWriter returns the log writer instance
func (s *DNSServer) GetLogWriter() *DNSLogWriter {
	return s.jsonlWriter
}

// GetDailyStats returns daily statistics for a specific date
func (s *DNSServer) GetDailyStats(date time.Time) *DailyStats {
	return s.stats.GetDailyStats(date)
}

// GetDailyStatsRange returns daily statistics for a date range
func (s *DNSServer) GetDailyStatsRange(startDate, endDate time.Time) []*DailyStats {
	return s.stats.GetDailyStatsRange(startDate, endDate)
}

// GetLast7Days returns statistics for last 7 days
func (s *DNSServer) GetLast7Days() []*DailyStats {
	return s.stats.GetLast7Days()
}

// GetLast30Days returns statistics for last 30 days
func (s *DNSServer) GetLast30Days() []*DailyStats {
	return s.stats.GetLast30Days()
}

