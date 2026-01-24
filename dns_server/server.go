package dns_server

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	"gorm.io/gorm"
)

var (
	dnsServerInstance *DNSServer
	dnsServerOnce     sync.Once
)

// DNSServer manages DNS server operations
type DNSServer struct {
	db              *gorm.DB
	config          *DNSConfig
	udpServer       *dns.Server
	tcpServer       *dns.Server
	dohServer       *dns.Server
	dotServer       *dns.Server
	filterEngine    *FilterEngine
	upstreamManager *UpstreamManager
	cache           *DNSCache
	stats           *StatsCollector
	mutex           sync.RWMutex
	running         bool
	ctx             context.Context
	cancel          context.CancelFunc

	// Log buffering for batch insert
	logBuffer      []DNSQueryLog
	logBufferMutex sync.Mutex
	logBufferSize  int
	logFlushTicker *time.Ticker
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
func (s *DNSServer) Init(db *gorm.DB) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.db = db
	s.ctx, s.cancel = context.WithCancel(context.Background())

	// Auto-migrate tables with SkipDefaultTransaction for SQLite compatibility
	migrator := s.db.Session(&gorm.Session{
		SkipDefaultTransaction: true,
	})

	if err := migrator.AutoMigrate(
		&DNSConfig{},
		&DNSBlocklist{},
		&DNSCustomFilter{},
		&DNSClientDomainRule{},
		&DNSQueryLog{},
		&DNSStatistics{},
		&DNSClientSettings{},
		&DNSRewrite{},
	); err != nil {
		return fmt.Errorf("failed to migrate DNS tables: %w", err)
	}

	// Load or create config
	if err := s.loadConfig(); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Initialize components
	s.filterEngine = NewFilterEngine(db)
	s.upstreamManager = NewUpstreamManager(s.config.GetUpstreamDNSList())
	s.cache = NewDNSCache(s.config.CacheTTL)
	s.stats = NewStatsCollector(db)

	// Initialize log buffer for batch insert (reduces DB lock contention)
	s.logBuffer = make([]DNSQueryLog, 0, 100)
	s.logBufferSize = 50                               // Flush when 50 logs accumulated
	s.logFlushTicker = time.NewTicker(5 * time.Second) // Or flush every 5 seconds

	// Start background log flusher
	go s.logFlusher()

	// Load filters
	if err := s.filterEngine.LoadFilters(); err != nil {
		log.Printf("Warning: Failed to load filters: %v", err)
	}

	return nil
}

// loadConfig loads or creates default config
func (s *DNSServer) loadConfig() error {
	var config DNSConfig
	result := s.db.First(&config)

	if result.Error == gorm.ErrRecordNotFound {
		// Create default config
		config = DNSConfig{
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
			"1.1.1.1:53",
			"8.8.8.8:53",
		})

		if err := s.db.Create(&config).Error; err != nil {
			return err
		}
	} else if result.Error != nil {
		return result.Error
	}

	s.config = &config
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

	// Stop log flusher and flush remaining logs
	if s.logFlushTicker != nil {
		s.logFlushTicker.Stop()
	}

	s.cancel()

	// Final flush of any remaining logs
	s.flushLogBuffer()

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

	// 📥 LOG: Incoming DNS Request
	log.Printf("📥 IN  | %s | %s | Type: %s | ID: %d", clientIP, domain, qtype, r.Id)

	// Check cache first
	var cached bool
	var blocked bool
	var blockReason string

	if s.config.CacheEnabled {
		if cachedMsg := s.cache.Get(domain, question.Qtype); cachedMsg != nil {
			cachedMsg.SetReply(r)
			w.WriteMsg(cachedMsg)
			cached = true
			responseTime := time.Since(startTime)

			// 📤 LOG: Cache Hit Response
			log.Printf("📤 OUT | %s | %s | Type: %s | Status: CACHED | Time: %dms | Answers: %d",
				clientIP, domain, qtype, responseTime.Milliseconds(), len(cachedMsg.Answer))

			// Log query
			if s.config.QueryLogging {
				s.logQuery(clientIP, domain, qtype, cachedMsg, blocked, blockReason, responseTime, cached)
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
			responseTime := time.Since(startTime)

			// 📤 LOG: Blocked Response
			log.Printf("📤 OUT | %s | %s | Type: %s | Status: BLOCKED (%s) | Time: %dms",
				clientIP, domain, qtype, blockReason, responseTime.Milliseconds())

			// Log blocked query
			if s.config.QueryLogging {
				s.logQuery(clientIP, domain, qtype, msg, blocked, blockReason, responseTime, false)
			}
			return
		}
	}

	// Check for DNS rewrites
	if rewrite := s.getRewrite(domain, question.Qtype); rewrite != nil {
		msg = rewrite
		msg.SetReply(r)
		w.WriteMsg(msg)
		responseTime := time.Since(startTime)

		// 📤 LOG: Rewrite Response
		log.Printf("📤 OUT | %s | %s | Type: %s | Status: REWRITTEN | Time: %dms | Answer: %v",
			clientIP, domain, qtype, responseTime.Milliseconds(), rewrite.Answer)

		if s.config.QueryLogging {
			s.logQuery(clientIP, domain, qtype, msg, false, "Rewrite", responseTime, false)
		}
		return
	}

	// Forward to upstream DNS
	response, err := s.upstreamManager.Query(r)
	responseTime := time.Since(startTime)

	if err != nil {
		// 📤 LOG: Error Response
		log.Printf("📤 OUT | %s | %s | Type: %s | Status: ERROR (%v) | Time: %dms",
			clientIP, domain, qtype, err, responseTime.Milliseconds())

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

	// 📤 LOG: Successful Response
	if response != nil {
		log.Printf("📤 OUT | %s | %s | Type: %s | Status: SUCCESS | Time: %dms | Answers: %d | Rcode: %s",
			clientIP, domain, qtype, responseTime.Milliseconds(), len(response.Answer), dns.RcodeToString[response.Rcode])
	}

	// Log query
	if s.config.QueryLogging {
		s.logQuery(clientIP, domain, qtype, response, blocked, blockReason, responseTime, cached)
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

// logQuery logs DNS query to database
func (s *DNSServer) logQuery(clientIP, domain, qtype string, response *dns.Msg, blocked bool, blockReason string, responseTime time.Duration, cached bool) {
	// Build response string
	var responseStr string
	if response != nil && len(response.Answer) > 0 {
		for _, ans := range response.Answer {
			responseStr += ans.String() + "; "
		}
	}

	logEntry := DNSQueryLog{
		ClientIP:     clientIP,
		Domain:       domain,
		QueryType:    qtype,
		Response:     responseStr,
		Blocked:      blocked,
		BlockReason:  blockReason,
		ResponseTime: int(responseTime.Milliseconds()),
		Cached:       cached,
	}

	// Add to buffer instead of direct insert
	s.logBufferMutex.Lock()
	s.logBuffer = append(s.logBuffer, logEntry)
	shouldFlush := len(s.logBuffer) >= s.logBufferSize
	s.logBufferMutex.Unlock()

	// Flush if buffer is full
	if shouldFlush {
		go s.flushLogBuffer()
	}
}

// getRewrite checks for DNS rewrite rules
// getRewrite checks for DNS rewrite rules with wildcard support
func (s *DNSServer) getRewrite(domain string, qtype uint16) *dns.Msg {
	var rewrites []DNSRewrite
	qtypeStr := dns.TypeToString[qtype]

	// Normalize domain (remove trailing dot for comparison)
	normalizedDomain := strings.TrimSuffix(domain, ".")

	// Get all enabled rewrites for this query type
	if err := s.db.Where("enabled = ? AND type = ?", true, qtypeStr).Find(&rewrites).Error; err != nil {
		return nil
	}

	// Check for exact match first
	for _, rewrite := range rewrites {
		rewriteDomain := strings.TrimSuffix(rewrite.Domain, ".")
		if rewriteDomain == normalizedDomain {
			return s.buildRewriteResponse(domain, qtype, &rewrite)
		}
	}

	// Check for wildcard match (*.example.org)
	for _, rewrite := range rewrites {
		rewriteDomain := strings.TrimSuffix(rewrite.Domain, ".")
		if strings.HasPrefix(rewriteDomain, "*.") {
			baseDomain := strings.TrimPrefix(rewriteDomain, "*.")
			if strings.HasSuffix(normalizedDomain, "."+baseDomain) || normalizedDomain == baseDomain {
				return s.buildRewriteResponse(domain, qtype, &rewrite)
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
			cutoffDate := time.Now().AddDate(0, 0, -s.config.LogRetentionDays)
			result := s.db.Where("created_at < ?", cutoffDate).Delete(&DNSQueryLog{})
			if result.Error == nil && result.RowsAffected > 0 {
				log.Printf("🧹 Cleaned up %d old DNS query logs", result.RowsAffected)
			}
		}
	}
}

// updateStatistics updates daily statistics
func (s *DNSServer) updateStatistics() {
	ticker := time.NewTicker(5 * time.Minute)
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

	if err := s.db.Save(config).Error; err != nil {
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
func (s *DNSServer) GetDB() (*gorm.DB, error) {
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

// logFlusher periodically flushes log buffer to database
func (s *DNSServer) logFlusher() {
	for {
		select {
		case <-s.ctx.Done():
			// Final flush on shutdown
			s.flushLogBuffer()
			return
		case <-s.logFlushTicker.C:
			s.flushLogBuffer()
		}
	}
}

// flushLogBuffer writes buffered logs to database in batch
func (s *DNSServer) flushLogBuffer() {
	s.logBufferMutex.Lock()
	if len(s.logBuffer) == 0 {
		s.logBufferMutex.Unlock()
		return
	}

	// Take current buffer and create new one
	logsToFlush := s.logBuffer
	s.logBuffer = make([]DNSQueryLog, 0, 100)
	s.logBufferMutex.Unlock()

	// Batch insert with SkipDefaultTransaction for better SQLite performance
	if err := s.db.Session(&gorm.Session{
		SkipDefaultTransaction: true,
	}).CreateInBatches(logsToFlush, 50).Error; err != nil {
		log.Printf("⚠️  Failed to flush DNS query logs: %v", err)
	}
}
