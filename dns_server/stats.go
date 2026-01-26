package dns_server

import (
	"redock/platform/memory"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// StatsCollector collects DNS statistics (in-memory + JSONL)
type StatsCollector struct {
	db        *memory.Database
	logWriter *DNSLogWriter

	// In-memory counters (lock-free) - reset on restart
	totalQueries      int64
	blockedQueries    int64
	cachedQueries     int64
	totalResponseTime int64 // microseconds

	// In-memory top domains/clients tracking
	topDomainsMutex sync.RWMutex
	topDomains      map[string]int64 // domain -> count
	topBlocked      map[string]int64 // blocked domain -> count
	topClients      map[string]int64 // client IP -> count

	// Time-based tracking (for queries per minute)
	recentQueries      []int64 // timestamps of recent queries (last 5 minutes)
	recentQueriesMutex sync.Mutex

	// Daily aggregated stats (last 30 days)
	dailyStatsMutex sync.RWMutex
	dailyStats      map[string]*DailyStats // date -> stats
}

// NewStatsCollector creates a new stats collector
func NewStatsCollector(db *memory.Database) *StatsCollector {
	return &StatsCollector{
		db:            db,
		topDomains:    make(map[string]int64),
		topBlocked:    make(map[string]int64),
		topClients:    make(map[string]int64),
		dailyStats:    make(map[string]*DailyStats),
		recentQueries: make([]int64, 0, 1000),
	}
}

// SetLogWriter sets the log writer for reading historical data
func (s *StatsCollector) SetLogWriter(lw *DNSLogWriter) {
	s.logWriter = lw
}

// UpdateDailyStats aggregates daily statistics from JSONL logs
func (s *StatsCollector) UpdateDailyStats() {
	if s.logWriter == nil {
		return
	}

	// Aggregate stats for last 30 days
	now := time.Now()
	for i := 0; i < 30; i++ {
		date := now.AddDate(0, 0, -i).Truncate(24 * time.Hour)
		dateKey := date.Format("2006-01-02")

		// Check if already aggregated today
		s.dailyStatsMutex.RLock()
		existing, exists := s.dailyStats[dateKey]
		s.dailyStatsMutex.RUnlock()

		if exists && existing.LastUpdated.Day() == now.Day() {
			continue // Already updated today
		}

		// Aggregate this day's stats
		stats := s.aggregateDayStats(date)
		if stats != nil {
			s.dailyStatsMutex.Lock()
			s.dailyStats[dateKey] = stats
			// Cleanup old stats (keep last 30 days)
			s.cleanupOldDailyStats(30)
			s.dailyStatsMutex.Unlock()
		}
	}
}

// aggregateDayStats aggregates stats for a specific day from JSONL
func (s *StatsCollector) aggregateDayStats(date time.Time) *DailyStats {
	startOfDay := date.Truncate(24 * time.Hour)
	endOfDay := startOfDay.Add(24 * time.Hour)

	stats := &DailyStats{
		Date:        startOfDay,
		LastUpdated: time.Now(),
		TopDomains:  make(map[string]int64),
		TopBlocked:  make(map[string]int64),
		TopClients:  make(map[string]int64),
	}

	// Read logs for this day
	err := s.logWriter.ReadLogs(startOfDay, func(log DNSQueryLog) error {
		// Only process logs from this specific day
		if log.CreatedAt.Before(startOfDay) || log.CreatedAt.After(endOfDay) {
			return nil
		}

		stats.TotalQueries++
		stats.TotalResponseTime += int64(log.ResponseTime)

		if log.Blocked {
			stats.BlockedQueries++
			stats.TopBlocked[log.Domain]++
		} else {
			stats.TopDomains[log.Domain]++
		}

		if log.Cached {
			stats.CachedQueries++
		}

		stats.TopClients[log.ClientIP]++

		// Track unique clients
		if _, exists := stats.TopClients[log.ClientIP]; !exists {
			stats.UniqueClients++
		}

		return nil
	})

	if err != nil || stats.TotalQueries == 0 {
		return nil
	}

	// Calculate averages
	if stats.TotalQueries > 0 {
		stats.AvgResponseTime = float64(stats.TotalResponseTime) / float64(stats.TotalQueries)
		stats.BlockRate = float64(stats.BlockedQueries) / float64(stats.TotalQueries) * 100
		stats.CacheHitRate = float64(stats.CachedQueries) / float64(stats.TotalQueries) * 100
	}

	return stats
}

// cleanupOldDailyStats removes stats older than maxDays
func (s *StatsCollector) cleanupOldDailyStats(maxDays int) {
	cutoff := time.Now().AddDate(0, 0, -maxDays).Format("2006-01-02")

	for dateKey := range s.dailyStats {
		if dateKey < cutoff {
			delete(s.dailyStats, dateKey)
		}
	}
}

// GetRealtimeStats returns realtime statistics
func (s *StatsCollector) GetRealtimeStats() RealtimeStats {
	total := atomic.LoadInt64(&s.totalQueries)
	blocked := atomic.LoadInt64(&s.blockedQueries)
	cached := atomic.LoadInt64(&s.cachedQueries)
	totalRespTime := atomic.LoadInt64(&s.totalResponseTime)

	var blockRate float64
	if total > 0 {
		blockRate = float64(blocked) / float64(total) * 100
	}

	var cacheHitRate float64
	if total > 0 {
		cacheHitRate = float64(cached) / float64(total) * 100
	}

	var avgResponseTime float64
	if total > 0 {
		// Convert microseconds to milliseconds
		avgResponseTime = float64(totalRespTime) / float64(total) / 1000.0
	}

	// Calculate queries per minute (from last 5 minutes)
	queriesPerMinute := s.calculateQPM()

	// Calculate active clients from last hour (from JSONL or in-memory)
	activeClients := s.calculateActiveClients()

	// Get top domains and clients
	s.topDomainsMutex.RLock()
	topDomains := s.getTopN(s.topDomains, 20)
	topBlocked := s.getTopN(s.topBlocked, 20)
	topClients := s.getTopN(s.topClients, 10)
	s.topDomainsMutex.RUnlock()

	return RealtimeStats{
		TotalQueries:     total,
		BlockedQueries:   blocked,
		CachedQueries:    cached,
		BlockRate:        blockRate,
		CacheHitRate:     cacheHitRate,
		TopDomains:       topDomains,
		TopBlocked:       topBlocked,
		TopClients:       topClients,
		QueriesPerMinute: queriesPerMinute,
		AvgResponseTime:  avgResponseTime,
		ActiveClients:    activeClients,
	}
}

// calculateActiveClients counts unique clients from last hour (JSONL or in-memory)
func (s *StatsCollector) calculateActiveClients() int64 {
	if s.logWriter == nil {
		// Fallback: use in-memory tracking
		s.topDomainsMutex.RLock()
		count := int64(len(s.topClients))
		s.topDomainsMutex.RUnlock()
		return count
	}

	// Count unique clients from last hour (from JSONL)
	uniqueClients := make(map[string]bool)
	since := time.Now().Add(-1 * time.Hour)

	_ = s.logWriter.ReadLogs(since, func(log DNSQueryLog) error {
		uniqueClients[log.ClientIP] = true
		return nil
	})

	return int64(len(uniqueClients))
}

// calculateQPM calculates queries per minute from recent queries
func (s *StatsCollector) calculateQPM() float64 {
	s.recentQueriesMutex.Lock()
	defer s.recentQueriesMutex.Unlock()

	if len(s.recentQueries) == 0 {
		return 0
	}

	now := time.Now().Unix()
	fiveMinutesAgo := now - 300

	// Clean old queries (older than 5 minutes)
	validQueries := make([]int64, 0, len(s.recentQueries))
	for _, ts := range s.recentQueries {
		if ts > fiveMinutesAgo {
			validQueries = append(validQueries, ts)
		}
	}
	s.recentQueries = validQueries

	// Calculate QPM
	if len(validQueries) == 0 {
		return 0
	}

	// Get time span in minutes
	oldestQuery := validQueries[0]
	timeSpanMinutes := float64(now-oldestQuery) / 60.0
	if timeSpanMinutes < 0.1 {
		timeSpanMinutes = 0.1 // Minimum 6 seconds
	}

	return float64(len(validQueries)) / timeSpanMinutes
}

// getTopN returns top N entries from map
func (s *StatsCollector) getTopN(m map[string]int64, n int) []DomainCount {
	type kv struct {
		key   string
		value int64
	}

	var items []kv
	for k, v := range m {
		items = append(items, kv{k, v})
	}

	// Sort by count descending
	sort.Slice(items, func(i, j int) bool {
		return items[i].value > items[j].value
	})

	// Take top N
	if len(items) > n {
		items = items[:n]
	}

	result := make([]DomainCount, len(items))
	for i, item := range items {
		result[i] = DomainCount{
			Domain: item.key,
			Count:  item.value,
		}
	}

	return result
}

// GetDailyStats returns daily stats for a specific date
func (s *StatsCollector) GetDailyStats(date time.Time) *DailyStats {
	dateKey := date.Format("2006-01-02")

	s.dailyStatsMutex.RLock()
	stats, exists := s.dailyStats[dateKey]
	s.dailyStatsMutex.RUnlock()

	if !exists {
		// Try to aggregate on-the-fly
		stats = s.aggregateDayStats(date)
		if stats != nil {
			s.dailyStatsMutex.Lock()
			s.dailyStats[dateKey] = stats
			s.dailyStatsMutex.Unlock()
		}
	}

	return stats
}

// GetDailyStatsRange returns daily stats for a date range
func (s *StatsCollector) GetDailyStatsRange(startDate, endDate time.Time) []*DailyStats {
	var result []*DailyStats

	current := startDate.Truncate(24 * time.Hour)
	end := endDate.Truncate(24 * time.Hour)

	for current.Before(end) || current.Equal(end) {
		if stats := s.GetDailyStats(current); stats != nil {
			result = append(result, stats)
		}
		current = current.AddDate(0, 0, 1)
	}

	return result
}

// GetLast7Days returns stats for last 7 days
func (s *StatsCollector) GetLast7Days() []*DailyStats {
	now := time.Now().Truncate(24 * time.Hour)
	weekAgo := now.AddDate(0, 0, -7)
	return s.GetDailyStatsRange(weekAgo, now)
}

// GetLast30Days returns stats for last 30 days
func (s *StatsCollector) GetLast30Days() []*DailyStats {
	now := time.Now().Truncate(24 * time.Hour)
	monthAgo := now.AddDate(0, 0, -30)
	return s.GetDailyStatsRange(monthAgo, now)
}

// GetQueryHistory returns query history (hourly aggregation from JSONL)
func (s *StatsCollector) GetQueryHistory(hours int) []QueryHistoryPoint {
	if s.logWriter == nil || hours <= 0 {
		return []QueryHistoryPoint{}
	}

	since := time.Now().Add(-time.Duration(hours) * time.Hour)

	// Aggregate by hour
	hourlyStats := make(map[string]*QueryHistoryPoint)

	err := s.logWriter.ReadLogs(since, func(log DNSQueryLog) error {
		// Truncate to hour
		hourKey := log.CreatedAt.Truncate(time.Hour).Format(time.RFC3339)

		if _, exists := hourlyStats[hourKey]; !exists {
			hourlyStats[hourKey] = &QueryHistoryPoint{
				Time: log.CreatedAt.Truncate(time.Hour),
			}
		}

		stats := hourlyStats[hourKey]
		stats.Queries++
		if log.Blocked {
			stats.Blocked++
		}

		return nil
	})

	if err != nil {
		return []QueryHistoryPoint{}
	}

	// Convert map to sorted slice
	var result []QueryHistoryPoint
	for _, stats := range hourlyStats {
		result = append(result, *stats)
	}

	// Sort by time ascending
	sort.Slice(result, func(i, j int) bool {
		return result[i].Time.Before(result[j].Time)
	})

	return result
}

// RecordQuery records a query (lock-free counters)
func (s *StatsCollector) RecordQuery(blocked, cached bool) {
	atomic.AddInt64(&s.totalQueries, 1)
	if blocked {
		atomic.AddInt64(&s.blockedQueries, 1)
	}
	if cached {
		atomic.AddInt64(&s.cachedQueries, 1)
	}
}

// RecordQueryDetails records query with domain and client tracking
func (s *StatsCollector) RecordQueryDetails(domain, clientIP string, blocked, cached bool, responseTimeMicros int64) {
	// Update counters first (lock-free)
	s.RecordQuery(blocked, cached)

	// Track response time
	atomic.AddInt64(&s.totalResponseTime, responseTimeMicros)

	// Track timestamp for QPM calculation
	now := time.Now().Unix()
	s.recentQueriesMutex.Lock()
	s.recentQueries = append(s.recentQueries, now)
	// Keep only last 1000 queries
	if len(s.recentQueries) > 1000 {
		s.recentQueries = s.recentQueries[len(s.recentQueries)-1000:]
	}
	s.recentQueriesMutex.Unlock()

	// Update domain/client tracking (with mutex, but fast)
	s.topDomainsMutex.Lock()
	if blocked {
		s.topBlocked[domain]++
	} else {
		s.topDomains[domain]++
	}
	s.topClients[clientIP]++

	// Cleanup old entries if maps get too large (keep top 1000)
	if len(s.topDomains) > 1000 {
		s.cleanupTopMap(s.topDomains)
	}
	if len(s.topBlocked) > 1000 {
		s.cleanupTopMap(s.topBlocked)
	}
	if len(s.topClients) > 500 {
		s.cleanupTopMap(s.topClients)
	}
	s.topDomainsMutex.Unlock()
}

// cleanupTopMap removes entries with lowest counts (keep top 50%)
func (s *StatsCollector) cleanupTopMap(m map[string]int64) {
	if len(m) < 100 {
		return
	}

	type kv struct {
		key   string
		value int64
	}

	// Convert to slice
	var items []kv
	for k, v := range m {
		items = append(items, kv{k, v})
	}

	// Sort by count descending
	sort.Slice(items, func(i, j int) bool {
		return items[i].value > items[j].value
	})

	// Keep only top 50%
	keepCount := len(items) / 2
	for k := range m {
		delete(m, k)
	}
	for i := 0; i < keepCount; i++ {
		m[items[i].key] = items[i].value
	}
}

// RealtimeStats represents realtime statistics
type RealtimeStats struct {
	TotalQueries     int64         `json:"total_queries"`
	BlockedQueries   int64         `json:"blocked_queries"`
	CachedQueries    int64         `json:"cached_queries"`
	BlockRate        float64       `json:"block_rate"`
	CacheHitRate     float64       `json:"cache_hit_rate"`
	QueriesPerMinute float64       `json:"queries_per_minute"`
	AvgResponseTime  float64       `json:"avg_response_time"`
	ActiveClients    int64         `json:"active_clients"`
	TopDomains       []DomainCount `json:"top_domains"`
	TopBlocked       []DomainCount `json:"top_blocked"`
	TopClients       []DomainCount `json:"top_clients"`
}

// DomainCount represents domain/client with count
type DomainCount struct {
	Domain string `json:"domain"`
	Count  int64  `json:"count"`
}

// QueryHistoryPoint represents a point in query history
type QueryHistoryPoint struct {
	Time    time.Time `json:"time"`
	Queries int64     `json:"queries"`
	Blocked int64     `json:"blocked"`
}

// DailyStats represents aggregated statistics for a day
type DailyStats struct {
	Date              time.Time        `json:"date"`
	TotalQueries      int64            `json:"total_queries"`
	BlockedQueries    int64            `json:"blocked_queries"`
	CachedQueries     int64            `json:"cached_queries"`
	UniqueClients     int64            `json:"unique_clients"`
	TotalResponseTime int64            `json:"-"` // Internal use only
	AvgResponseTime   float64          `json:"avg_response_time"`
	BlockRate         float64          `json:"block_rate"`
	CacheHitRate      float64          `json:"cache_hit_rate"`
	TopDomains        map[string]int64 `json:"top_domains"`
	TopBlocked        map[string]int64 `json:"top_blocked"`
	TopClients        map[string]int64 `json:"top_clients"`
	LastUpdated       time.Time        `json:"last_updated"`
}
