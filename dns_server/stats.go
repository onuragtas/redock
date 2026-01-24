package dns_server

import (
	"encoding/json"
	"log"
	"time"

	"gorm.io/gorm"
)

// StatsCollector collects and aggregates DNS statistics
type StatsCollector struct {
	db *gorm.DB
}

// NewStatsCollector creates a new stats collector
func NewStatsCollector(db *gorm.DB) *StatsCollector {
	return &StatsCollector{
		db: db,
	}
}

// UpdateDailyStats updates daily statistics
func (s *StatsCollector) UpdateDailyStats() {
	today := time.Now().Truncate(24 * time.Hour)

	var stats DNSStatistics
	result := s.db.Where("date = ?", today).First(&stats)

	if result.Error == gorm.ErrRecordNotFound {
		stats = DNSStatistics{
			Date: today,
		}
	}

	// Count total queries
	var totalQueries int64
	s.db.Model(&DNSQueryLog{}).
		Where("DATE(created_at) = ?", today.Format("2006-01-02")).
		Count(&totalQueries)
	stats.TotalQueries = totalQueries

	// Count blocked queries
	var blockedQueries int64
	s.db.Model(&DNSQueryLog{}).
		Where("DATE(created_at) = ? AND blocked = ?", today.Format("2006-01-02"), true).
		Count(&blockedQueries)
	stats.BlockedQueries = blockedQueries

	// Count cached queries
	var cachedQueries int64
	s.db.Model(&DNSQueryLog{}).
		Where("DATE(created_at) = ? AND cached = ?", today.Format("2006-01-02"), true).
		Count(&cachedQueries)
	stats.CachedQueries = cachedQueries

	// Calculate average response time
	var avgResponseTime float64
	s.db.Model(&DNSQueryLog{}).
		Where("DATE(created_at) = ?", today.Format("2006-01-02")).
		Select("AVG(response_time)").
		Scan(&avgResponseTime)
	stats.AvgResponseTime = avgResponseTime

	// Count unique clients
	var uniqueClients int64
	s.db.Model(&DNSQueryLog{}).
		Where("DATE(created_at) = ?", today.Format("2006-01-02")).
		Distinct("client_ip").
		Count(&uniqueClients)
	stats.UniqueClients = int(uniqueClients)

	// Get top domains
	topDomains := s.getTopDomains(today, false, 10)
	if data, err := json.Marshal(topDomains); err == nil {
		stats.TopDomains = string(data)
	}

	// Get top blocked domains
	topBlockedDomains := s.getTopDomains(today, true, 10)
	if data, err := json.Marshal(topBlockedDomains); err == nil {
		stats.TopBlockedDomains = string(data)
	}

	// Get top clients
	topClients := s.getTopClients(today, 10)
	if data, err := json.Marshal(topClients); err == nil {
		stats.TopClients = string(data)
	}

	// Save or update stats
	if result.Error == gorm.ErrRecordNotFound {
		s.db.Create(&stats)
	} else {
		s.db.Save(&stats)
	}

	log.Printf("📊 Updated daily stats: %d queries, %d blocked, %d cached",
		totalQueries, blockedQueries, cachedQueries)
}

// getTopDomains returns top queried domains
func (s *StatsCollector) getTopDomains(date time.Time, blocked bool, limit int) []DomainStat {
	type Result struct {
		Domain string
		Count  int64
	}

	var results []Result
	query := s.db.Model(&DNSQueryLog{}).
		Select("domain, COUNT(*) as count").
		Where("DATE(created_at) = ?", date.Format("2006-01-02"))

	if blocked {
		query = query.Where("blocked = ?", true)
	}

	query.Group("domain").
		Order("count DESC").
		Limit(limit).
		Scan(&results)

	stats := make([]DomainStat, len(results))
	for i, r := range results {
		stats[i] = DomainStat{
			Domain: r.Domain,
			Count:  r.Count,
		}
	}

	return stats
}

// getTopClients returns top clients by query count
func (s *StatsCollector) getTopClients(date time.Time, limit int) []ClientStat {
	type Result struct {
		ClientIP string
		Count    int64
	}

	var results []Result
	s.db.Model(&DNSQueryLog{}).
		Select("client_ip, COUNT(*) as count").
		Where("DATE(created_at) = ?", date.Format("2006-01-02")).
		Group("client_ip").
		Order("count DESC").
		Limit(limit).
		Scan(&results)

	stats := make([]ClientStat, len(results))
	for i, r := range results {
		stats[i] = ClientStat{
			ClientIP: r.ClientIP,
			Count:    r.Count,
		}
	}

	return stats
}

// GetRealtimeStats returns realtime statistics
func (s *StatsCollector) GetRealtimeStats() RealtimeStats {
	now := time.Now()
	last24h := now.Add(-24 * time.Hour)

	var stats RealtimeStats

	// Total queries in last 24h
	s.db.Model(&DNSQueryLog{}).
		Where("created_at >= ?", last24h).
		Count(&stats.TotalQueries24h)

	// Blocked queries in last 24h
	s.db.Model(&DNSQueryLog{}).
		Where("created_at >= ? AND blocked = ?", last24h, true).
		Count(&stats.BlockedQueries24h)

	// Calculate block percentage
	if stats.TotalQueries24h > 0 {
		stats.BlockPercentage = float64(stats.BlockedQueries24h) / float64(stats.TotalQueries24h) * 100
	}

	// Queries per minute (last 5 minutes)
	last5min := now.Add(-5 * time.Minute)
	var queriesLast5min int64
	s.db.Model(&DNSQueryLog{}).
		Where("created_at >= ?", last5min).
		Count(&queriesLast5min)
	stats.QueriesPerMinute = float64(queriesLast5min) / 5.0

	// Average response time (last 1 hour)
	lastHour := now.Add(-1 * time.Hour)
	s.db.Model(&DNSQueryLog{}).
		Where("created_at >= ?", lastHour).
		Select("AVG(response_time)").
		Scan(&stats.AvgResponseTime)

	// Active clients (last 1 hour)
	s.db.Model(&DNSQueryLog{}).
		Where("created_at >= ?", lastHour).
		Distinct("client_ip").
		Count(&stats.ActiveClients)

	// Cache hit rate (last 1 hour)
	var totalLastHour int64
	var cachedLastHour int64
	s.db.Model(&DNSQueryLog{}).
		Where("created_at >= ?", lastHour).
		Count(&totalLastHour)
	s.db.Model(&DNSQueryLog{}).
		Where("created_at >= ? AND cached = ?", lastHour, true).
		Count(&cachedLastHour)

	if totalLastHour > 0 {
		stats.CacheHitRate = float64(cachedLastHour) / float64(totalLastHour) * 100
	}

	// Top queried domains (last 24h)
	var topDomains []DomainCount
	s.db.Model(&DNSQueryLog{}).
		Select("domain, COUNT(*) as count").
		Where("created_at >= ?", last24h).
		Group("domain").
		Order("count DESC").
		Limit(20).
		Scan(&topDomains)
	stats.TopDomains = topDomains

	// Top blocked domains (last 24h)
	var topBlocked []DomainCount
	s.db.Model(&DNSQueryLog{}).
		Select("domain, COUNT(*) as count").
		Where("created_at >= ? AND blocked = ?", last24h, true).
		Group("domain").
		Order("count DESC").
		Limit(20).
		Scan(&topBlocked)
	stats.TopBlocked = topBlocked

	return stats
}

// GetQueryHistory returns query history for charts
func (s *StatsCollector) GetQueryHistory(hours int) []QueryHistoryPoint {
	now := time.Now()
	startTime := now.Add(-time.Duration(hours) * time.Hour)

	type Result struct {
		Hour    string
		Total   int64
		Blocked int64
	}

	var results []Result
	s.db.Model(&DNSQueryLog{}).
		Select("strftime('%Y-%m-%d %H:00:00', created_at) as hour, COUNT(*) as total, SUM(CASE WHEN blocked THEN 1 ELSE 0 END) as blocked").
		Where("created_at >= ?", startTime).
		Group("hour").
		Order("hour ASC").
		Scan(&results)

	points := make([]QueryHistoryPoint, len(results))
	for i, r := range results {
		timestamp, _ := time.Parse("2006-01-02 15:04:05", r.Hour)
		points[i] = QueryHistoryPoint{
			Timestamp: timestamp,
			Total:     r.Total,
			Blocked:   r.Blocked,
		}
	}

	return points
}

// DomainStat represents domain statistics
type DomainStat struct {
	Domain string `json:"domain"`
	Count  int64  `json:"count"`
}

// ClientStat represents client statistics
type ClientStat struct {
	ClientIP string `json:"client_ip"`
	Count    int64  `json:"count"`
}

// RealtimeStats represents realtime statistics
type RealtimeStats struct {
	TotalQueries24h   int64         `json:"total_queries_24h"`
	BlockedQueries24h int64         `json:"blocked_queries_24h"`
	BlockPercentage   float64       `json:"block_percentage"`
	QueriesPerMinute  float64       `json:"queries_per_minute"`
	AvgResponseTime   float64       `json:"avg_response_time"`
	ActiveClients     int64         `json:"active_clients"`
	CacheHitRate      float64       `json:"cache_hit_rate"`
	TopDomains        []DomainCount `json:"top_domains"`
	TopBlocked        []DomainCount `json:"top_blocked"`
}

// DomainCount represents domain with count
type DomainCount struct {
	Domain string `json:"domain"`
	Count  int64  `json:"count"`
}

// QueryHistoryPoint represents a point in query history
type QueryHistoryPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Total     int64     `json:"total"`
	Blocked   int64     `json:"blocked"`
}
