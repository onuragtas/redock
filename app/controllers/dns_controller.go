package controllers

import (
	"redock/dns_server"
	"redock/platform/memory"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// GetDNSConfig returns DNS server configuration
// @Description Get DNS server configuration
// @Summary Get DNS config
// @Tags DNS
// @Accept json
// @Produce json
// @Success 200 {object} dns_server.DNSConfig
// @Router /v1/dns/config [get]
func GetDNSConfig(c *fiber.Ctx) error {
	server := dns_server.GetServer()
	if server == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "DNS server not initialized",
		})
	}

	config := server.GetConfig()

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  config,
	})
}

// UpdateDNSConfig updates DNS server configuration
// @Description Update DNS server configuration
// @Summary Update DNS config
// @Tags DNS
// @Accept json
// @Produce json
// @Param config body dns_server.DNSConfig true "DNS Config"
// @Success 200 {object} dns_server.DNSConfig
// @Router /v1/dns/config [put]
func UpdateDNSConfig(c *fiber.Ctx) error {
	server := dns_server.GetServer()
	if server == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "DNS server not initialized",
		})
	}

	var config dns_server.DNSConfig
	if err := c.BodyParser(&config); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid request body",
		})
	}

	if err := server.UpdateConfig(&config); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Configuration updated successfully",
		"data":  config,
	})
}

// StartDNSServer starts the DNS server
// @Description Start DNS server
// @Summary Start DNS server
// @Tags DNS
// @Accept json
// @Produce json
// @Success 200
// @Router /v1/dns/start [post]
func StartDNSServer(c *fiber.Ctx) error {
	server := dns_server.GetServer()
	if server == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "DNS server not initialized",
		})
	}

	if server.IsRunning() {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "DNS server is already running",
		})
	}

	if err := server.Start(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "DNS server started successfully",
	})
}

// StopDNSServer stops the DNS server
// @Description Stop DNS server
// @Summary Stop DNS server
// @Tags DNS
// @Accept json
// @Produce json
// @Success 200
// @Router /v1/dns/stop [post]
func StopDNSServer(c *fiber.Ctx) error {
	server := dns_server.GetServer()
	if server == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "DNS server not initialized",
		})
	}

	if !server.IsRunning() {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "DNS server is not running",
		})
	}

	if err := server.Stop(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "DNS server stopped successfully",
	})
}

// GetDNSStatus returns DNS server status
// @Description Get DNS server status
// @Summary Get DNS status
// @Tags DNS
// @Accept json
// @Produce json
// @Success 200
// @Router /v1/dns/status [get]
func GetDNSStatus(c *fiber.Ctx) error {
	server := dns_server.GetServer()
	if server == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "DNS server not initialized",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data": fiber.Map{
			"running": server.IsRunning(),
		},
	})
}

// GetDNSBlocklists returns all blocklists
// @Description Get DNS blocklists
// @Summary Get blocklists
// @Tags DNS
// @Accept json
// @Produce json
// @Success 200 {array} dns_server.DNSBlocklist
// @Router /v1/dns/blocklists [get]
func GetDNSBlocklists(c *fiber.Ctx) error {
	server := dns_server.GetServer()
	if server == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "DNS server not initialized",
		})
	}

	db, err := server.GetDB()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	blocklists := memory.FindAll[*dns_server.DNSBlocklist](db, "dns_blocklists")

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  blocklists,
	})
}

// CreateDNSBlocklist creates a new blocklist
// @Description Create DNS blocklist
// @Summary Create blocklist
// @Tags DNS
// @Accept json
// @Produce json
// @Param blocklist body dns_server.DNSBlocklist true "Blocklist"
// @Success 200 {object} dns_server.DNSBlocklist
// @Router /v1/dns/blocklists [post]
func CreateDNSBlocklist(c *fiber.Ctx) error {
	server := dns_server.GetServer()
	if server == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "DNS server not initialized",
		})
	}

	var blocklist dns_server.DNSBlocklist
	if err := c.BodyParser(&blocklist); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid request body",
		})
	}

	db, err := server.GetDB()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	if err := memory.Create[*dns_server.DNSBlocklist](db, "dns_blocklists", &blocklist); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Reload filters
	go server.ReloadFilters()

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Blocklist created successfully",
		"data":  blocklist,
	})
}

// UpdateDNSBlocklist updates a blocklist
// @Description Update DNS blocklist
// @Summary Update blocklist
// @Tags DNS
// @Accept json
// @Produce json
// @Param id path int true "Blocklist ID"
// @Param blocklist body dns_server.DNSBlocklist true "Blocklist"
// @Success 200 {object} dns_server.DNSBlocklist
// @Router /v1/dns/blocklists/{id} [put]
func UpdateDNSBlocklist(c *fiber.Ctx) error {
	server := dns_server.GetServer()
	if server == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "DNS server not initialized",
		})
	}

	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid ID",
		})
	}

	var blocklist dns_server.DNSBlocklist
	if err := c.BodyParser(&blocklist); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid request body",
		})
	}

	blocklist.ID = uint(id)

	db, err := server.GetDB()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	if err := memory.Update[*dns_server.DNSBlocklist](db, "dns_blocklists", &blocklist); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Reload filters
	go server.ReloadFilters()

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Blocklist updated successfully",
		"data":  blocklist,
	})
}

// DeleteDNSBlocklist deletes a blocklist
// @Description Delete DNS blocklist
// @Summary Delete blocklist
// @Tags DNS
// @Accept json
// @Produce json
// @Param id path int true "Blocklist ID"
// @Success 200
// @Router /v1/dns/blocklists/{id} [delete]
func DeleteDNSBlocklist(c *fiber.Ctx) error {
	server := dns_server.GetServer()
	if server == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "DNS server not initialized",
		})
	}

	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid ID",
		})
	}

	db, err := server.GetDB()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	if err := memory.Delete[*dns_server.DNSBlocklist](db, "dns_blocklists", uint(id)); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Reload filters
	go server.ReloadFilters()

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Blocklist deleted successfully",
	})
}

// GetDNSCustomFilters returns all custom filters
// @Description Get DNS custom filters
// @Summary Get custom filters
// @Tags DNS
// @Accept json
// @Produce json
// @Success 200 {array} dns_server.DNSCustomFilter
// @Router /v1/dns/filters [get]
func GetDNSCustomFilters(c *fiber.Ctx) error {
	server := dns_server.GetServer()
	if server == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "DNS server not initialized",
		})
	}

	db, err := server.GetDB()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	filters := memory.FindAll[*dns_server.DNSCustomFilter](db, "dns_custom_filters")

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  filters,
	})
}

// CreateDNSCustomFilter creates a new custom filter
// @Description Create DNS custom filter
// @Summary Create custom filter
// @Tags DNS
// @Accept json
// @Produce json
// @Param filter body dns_server.DNSCustomFilter true "Filter"
// @Success 200 {object} dns_server.DNSCustomFilter
// @Router /v1/dns/filters [post]
func CreateDNSCustomFilter(c *fiber.Ctx) error {
	server := dns_server.GetServer()
	if server == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "DNS server not initialized",
		})
	}

	var filter dns_server.DNSCustomFilter
	if err := c.BodyParser(&filter); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid request body",
		})
	}

	// Normalize domain (remove trailing dot, lowercase, trim)
	filter.Domain = strings.TrimSuffix(strings.TrimSpace(strings.ToLower(filter.Domain)), ".")

	db, err := server.GetDB()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	if err := memory.Create[*dns_server.DNSCustomFilter](db, "dns_custom_filters", &filter); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Reload filters
	go server.ReloadFilters()

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Filter created successfully",
		"data":  filter,
	})
}

// DeleteDNSCustomFilter deletes a custom filter
// @Description Delete DNS custom filter
// @Summary Delete custom filter
// @Tags DNS
// @Accept json
// @Produce json
// @Param id path int true "Filter ID"
// @Success 200
// @Router /v1/dns/filters/{id} [delete]
func DeleteDNSCustomFilter(c *fiber.Ctx) error {
	server := dns_server.GetServer()
	if server == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "DNS server not initialized",
		})
	}

	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid ID",
		})
	}

	db, err := server.GetDB()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	if err := memory.Delete[*dns_server.DNSCustomFilter](db, "dns_custom_filters", uint(id)); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Reload filters
	go server.ReloadFilters()

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Filter deleted successfully",
	})
}

// DeleteDNSCustomFilterByDetails deletes a filter by domain and type
func DeleteDNSCustomFilterByDetails(c *fiber.Ctx) error {
	server := dns_server.GetServer()
	db, err := server.GetDB()
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "DNS server not initialized",
		})
	}

	domain := c.Query("domain")
	filterType := c.Query("type") // "blacklist" or "whitelist"

	if domain == "" || filterType == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "domain and type are required",
		})
	}

	// Normalize domain
	domain = strings.TrimSuffix(strings.TrimSpace(strings.ToLower(domain)), ".")

	// Delete the filter
	filters := memory.Filter[*dns_server.DNSCustomFilter](db, "dns_custom_filters", func(f *dns_server.DNSCustomFilter) bool {
		return f.Domain == domain && f.Type == filterType
	})

	deletedCount := 0
	for _, filter := range filters {
		if err := memory.Delete[*dns_server.DNSCustomFilter](db, "dns_custom_filters", filter.GetID()); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": true,
				"msg":   "Failed to delete filter: " + err.Error(),
			})
		}
		deletedCount++
	}

	// Reload filters
	go server.ReloadFilters()

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Filter deleted successfully",
		"data": fiber.Map{
			"deleted_count": deletedCount,
		},
	})
}

// GetDNSQueryLogs returns DNS query logs with pagination
// @Description Get DNS query logs
// @Summary Get query logs
// @Tags DNS
// @Accept json
// @Produce json
// @Param page query int false "Page number"
// @Param limit query int false "Items per page"
// @Success 200 {array} dns_server.DNSQueryLog
// @Router /v1/dns/logs [get]
func GetDNSQueryLogs(c *fiber.Ctx) error {
	server := dns_server.GetServer()
	if server == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "DNS server not initialized",
		})
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "50"))

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 1000 {
		limit = 50
	}

	// Read logs from JSONL files (last 24 hours)
	logs := []dns_server.DNSQueryLog{}
	since := time.Now().Add(-24 * time.Hour)
	
	logWriter := server.GetLogWriter()
	if logWriter == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Log writer not initialized",
		})
	}
	
	err := logWriter.ReadLogs(since, func(log dns_server.DNSQueryLog) error {
		logs = append(logs, log)
		return nil
	})
	
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to read logs: " + err.Error(),
		})
	}
	
	// Sort by time descending (newest first)
	sort.Slice(logs, func(i, j int) bool {
		return logs[i].CreatedAt.After(logs[j].CreatedAt)
	})
	
	// Pagination
	total := len(logs)
	start := (page - 1) * limit
	end := start + limit
	
	if start >= total {
		logs = []dns_server.DNSQueryLog{}
	} else {
		if end > total {
			end = total
		}
		logs = logs[start:end]
	}
	
	// Assign sequential IDs for frontend (based on position)
	for i := range logs {
		logs[i].ID = uint(total - start - i) // Descending ID (newest = highest)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data": fiber.Map{
			"logs":  logs,
			"total": total,
			"page":  page,
			"limit": limit,
		},
	})
}

// GetDNSStatistics returns DNS statistics
// @Description Get DNS statistics
// @Summary Get statistics
// @Tags DNS
// @Accept json
// @Produce json
// @Success 200
// @Router /v1/dns/stats [get]
func GetDNSStatistics(c *fiber.Ctx) error {
	server := dns_server.GetServer()
	if server == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "DNS server not initialized",
		})
	}

	stats := server.GetRealtimeStats()
	
	// Transform to frontend expected format (backward compatible)
	response := fiber.Map{
		"total_queries_24h":    stats.TotalQueries,
		"blocked_queries_24h":  stats.BlockedQueries,
		"block_percentage":     stats.BlockRate,
		"queries_per_minute":   stats.QueriesPerMinute,
		"avg_response_time":    stats.AvgResponseTime,
		"active_clients":       stats.ActiveClients,
		"cache_hit_rate":       stats.CacheHitRate,
		"top_domains":          stats.TopDomains,
		"top_blocked":          stats.TopBlocked,
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  response,
	})
}

// GetDNSQueryHistory returns query history for charts
// @Description Get DNS query history
// @Summary Get query history
// @Tags DNS
// @Accept json
// @Produce json
// @Param hours query int false "Hours to look back"
// @Success 200
// @Router /v1/dns/history [get]
func GetDNSQueryHistory(c *fiber.Ctx) error {
	server := dns_server.GetServer()
	if server == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "DNS server not initialized",
		})
	}

	hours, _ := strconv.Atoi(c.Query("hours", "24"))
	if hours < 1 || hours > 168 {
		hours = 24
	}

	history := server.GetQueryHistory(hours)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  history,
	})
}

// GetDNSDailyStats returns daily statistics
// @Description Get DNS daily statistics for a date range
// @Summary Get daily stats
// @Tags DNS
// @Accept json
// @Produce json
// @Param start_date query string false "Start date (YYYY-MM-DD)"
// @Param end_date query string false "End date (YYYY-MM-DD)"
// @Param period query string false "Period: 7days, 30days, custom"
// @Success 200
// @Router /v1/dns/stats/daily [get]
func GetDNSDailyStats(c *fiber.Ctx) error {
	server := dns_server.GetServer()
	if server == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "DNS server not initialized",
		})
	}

	period := c.Query("period", "7days")
	
	var stats []*dns_server.DailyStats
	
	switch period {
	case "7days":
		stats = server.GetLast7Days()
	case "30days":
		stats = server.GetLast30Days()
	case "custom":
		startDateStr := c.Query("start_date")
		endDateStr := c.Query("end_date")
		
		if startDateStr == "" || endDateStr == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": true,
				"msg":   "start_date and end_date required for custom period",
			})
		}
		
		startDate, err := time.Parse("2006-01-02", startDateStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": true,
				"msg":   "Invalid start_date format (use YYYY-MM-DD)",
			})
		}
		
		endDate, err := time.Parse("2006-01-02", endDateStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": true,
				"msg":   "Invalid end_date format (use YYYY-MM-DD)",
			})
		}
		
		stats = server.GetDailyStatsRange(startDate, endDate)
	default:
		stats = server.GetLast7Days()
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  stats,
	})
}

// GetDNSClientSettings returns all client settings
// @Description Get DNS client settings
// @Summary Get client settings
// @Tags DNS
// @Accept json
// @Produce json
// @Success 200 {array} dns_server.DNSClientSettings
// @Router /v1/dns/clients [get]
func GetDNSClientSettings(c *fiber.Ctx) error {
	server := dns_server.GetServer()
	if server == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "DNS server not initialized",
		})
	}

	type ClientStats struct {
		IP           string    `json:"ip"`
		QueryCount   int64     `json:"query_count"`
		BlockedCount int64     `json:"blocked_count"`
		LastSeen     time.Time `json:"last_seen"`
		IsBanned     bool      `json:"is_banned"`
	}

	// Aggregate client stats from JSONL files (last 24 hours)
	clientMap := make(map[string]*ClientStats)
	since := time.Now().Add(-24 * time.Hour)
	
	logWriter := server.GetLogWriter()
	if logWriter == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Log writer not initialized",
		})
	}
	
	err := logWriter.ReadLogs(since, func(log dns_server.DNSQueryLog) error {
		if _, exists := clientMap[log.ClientIP]; !exists {
			clientMap[log.ClientIP] = &ClientStats{
				IP:       log.ClientIP,
				LastSeen: log.CreatedAt,
			}
		}
		
		stats := clientMap[log.ClientIP]
		stats.QueryCount++
		if log.Blocked {
			stats.BlockedCount++
		}
		if log.CreatedAt.After(stats.LastSeen) {
			stats.LastSeen = log.CreatedAt
		}
		
		return nil
	})
	
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to read logs: " + err.Error(),
		})
	}
	
	// Convert map to slice
	var clients []ClientStats
	for _, stats := range clientMap {
		clients = append(clients, *stats)
	}
	
	// Sort by query count descending
	sort.Slice(clients, func(i, j int) bool {
		return clients[i].QueryCount > clients[j].QueryCount
	})
	
	// Limit to top 100
	if len(clients) > 100 {
		clients = clients[:100]
	}

	// Check ban status for each client
	filterEngine := server.GetFilterEngine()
	for i := range clients {
		clients[i].IsBanned = filterEngine.IsClientBanned(clients[i].IP)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  clients,
	})
}

// CreateDNSClientSettings creates client settings
// @Description Create DNS client settings
// @Summary Create client settings
// @Tags DNS
// @Accept json
// @Produce json
// @Param client body dns_server.DNSClientSettings true "Client Settings"
// @Success 200 {object} dns_server.DNSClientSettings
// @Router /v1/dns/clients [post]
func CreateDNSClientSettings(c *fiber.Ctx) error {
	server := dns_server.GetServer()
	if server == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "DNS server not initialized",
		})
	}

	var client dns_server.DNSClientSettings
	if err := c.BodyParser(&client); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid request body",
		})
	}

	db, err := server.GetDB()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	if err := memory.Create[*dns_server.DNSClientSettings](db, "dns_client_settings", &client); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Client settings created successfully",
		"data":  client,
	})
}

// ReloadDNSFilters reloads all DNS filters
// @Description Reload DNS filters
// @Summary Reload filters
// @Tags DNS
// @Accept json
// @Produce json
// @Success 200
// @Router /v1/dns/reload [post]
func ReloadDNSFilters(c *fiber.Ctx) error {
	server := dns_server.GetServer()
	if server == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "DNS server not initialized",
		})
	}

	if err := server.ReloadFilters(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Filters reloaded successfully",
	})
}

// GetDNSRewrites returns all DNS rewrites
func GetDNSRewrites(c *fiber.Ctx) error {
	server := dns_server.GetServer()
	db, err := server.GetDB()
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "DNS server not initialized",
		})
	}

	rewrites := memory.FindAll[*dns_server.DNSRewrite](db, "dns_rewrites")
	// Sort by CreatedAt descending
	sort.Slice(rewrites, func(i, j int) bool {
		return rewrites[i].CreatedAt.After(rewrites[j].CreatedAt)
	})

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error":    false,
		"msg":      "DNS rewrites fetched successfully",
		"rewrites": rewrites,
	})
}

// CreateDNSRewrite creates a new DNS rewrite rule
func CreateDNSRewrite(c *fiber.Ctx) error {
	server := dns_server.GetServer()
	db, err := server.GetDB()
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "DNS server not initialized",
		})
	}

	var rewrite dns_server.DNSRewrite
	if err := c.BodyParser(&rewrite); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid request body: " + err.Error(),
		})
	}

	// Validate domain
	if rewrite.Domain == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Domain is required",
		})
	}

	// Validate answer
	if rewrite.Answer == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Answer (IP/Domain) is required",
		})
	}

	// Validate type
	if rewrite.Type != "A" && rewrite.Type != "AAAA" && rewrite.Type != "CNAME" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Type must be A, AAAA, or CNAME",
		})
	}

	if err := memory.Create[*dns_server.DNSRewrite](db, "dns_rewrites", &rewrite); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to create rewrite: " + err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error":   false,
		"msg":     "DNS rewrite created successfully",
		"rewrite": rewrite,
	})
}

// UpdateDNSRewrite updates an existing DNS rewrite rule
func UpdateDNSRewrite(c *fiber.Ctx) error {
	server := dns_server.GetServer()
	db, err := server.GetDB()
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "DNS server not initialized",
		})
	}

	idStr := c.Params("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid ID",
		})
	}

	rewrite, err := memory.FindByID[*dns_server.DNSRewrite](db, "dns_rewrites", uint(id))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "Rewrite not found",
		})
	}

	var updateData dns_server.DNSRewrite
	if err := c.BodyParser(&updateData); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid request body: " + err.Error(),
		})
	}

	// Validate if provided
	if updateData.Domain != "" {
		rewrite.Domain = updateData.Domain
	}
	if updateData.Answer != "" {
		rewrite.Answer = updateData.Answer
	}
	if updateData.Type != "" {
		if updateData.Type != "A" && updateData.Type != "AAAA" && updateData.Type != "CNAME" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": true,
				"msg":   "Type must be A, AAAA, or CNAME",
			})
		}
		rewrite.Type = updateData.Type
	}
	rewrite.Comment = updateData.Comment
	rewrite.Enabled = updateData.Enabled

	if err := memory.Update[*dns_server.DNSRewrite](db, "dns_rewrites", rewrite); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to update rewrite: " + err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error":   false,
		"msg":     "DNS rewrite updated successfully",
		"rewrite": rewrite,
	})
}

// DeleteDNSRewrite deletes a DNS rewrite rule
func DeleteDNSRewrite(c *fiber.Ctx) error {
	server := dns_server.GetServer()
	db, err := server.GetDB()
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "DNS server not initialized",
		})
	}

	idStr := c.Params("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid ID",
		})
	}

	if err := memory.Delete[*dns_server.DNSRewrite](db, "dns_rewrites", uint(id)); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to delete rewrite: " + err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "DNS rewrite deleted successfully",
	})
}

// BlockClient blocks a client IP (IP Ban)
func BlockClient(c *fiber.Ctx) error {
	server := dns_server.GetServer()
	db, err := server.GetDB()
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "DNS server not initialized",
		})
	}

	type BlockRequest struct {
		ClientIP string `json:"client_ip"`
		Reason   string `json:"reason"`
	}

	var req BlockRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid request body: " + err.Error(),
		})
	}

	if req.ClientIP == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Client IP is required",
		})
	}

	// Find or create client settings
	clients := memory.Filter[*dns_server.DNSClientSettings](db, "dns_client_settings", func(c *dns_server.DNSClientSettings) bool {
		return c.ClientIP == req.ClientIP
	})

	now := time.Now()
	var clientSettings dns_server.DNSClientSettings
	if len(clients) == 0 {
		clientSettings = dns_server.DNSClientSettings{
			ClientIP:    req.ClientIP,
			Blocked:     true,
			BlockReason: req.Reason,
			BlockedAt:   &now,
		}
		if err := memory.Create[*dns_server.DNSClientSettings](db, "dns_client_settings", &clientSettings); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": true,
				"msg":   "Failed to block client: " + err.Error(),
			})
		}
	} else {
		clientSettings = *clients[0]
		clientSettings.Blocked = true
		clientSettings.BlockReason = req.Reason
		clientSettings.BlockedAt = &now
		if err := memory.Update[*dns_server.DNSClientSettings](db, "dns_client_settings", &clientSettings); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": true,
				"msg":   "Failed to block client: " + err.Error(),
			})
		}
	}

	// Invalidate client cache after blocking
	filterEngine := server.GetFilterEngine()
	filterEngine.InvalidateClientCache(req.ClientIP)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Client blocked successfully",
		"data":  clientSettings,
	})
}

// UnblockClient unblocks a client IP
func UnblockClient(c *fiber.Ctx) error {
	server := dns_server.GetServer()
	db, err := server.GetDB()
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "DNS server not initialized",
		})
	}

	clientIP := c.Params("ip")
	if clientIP == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Client IP is required",
		})
	}

	clients := memory.Filter[*dns_server.DNSClientSettings](db, "dns_client_settings", func(c *dns_server.DNSClientSettings) bool {
		return c.ClientIP == clientIP
	})

	if len(clients) == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "Client not found",
		})
	}

	clientSettings := *clients[0]

	clientSettings.Blocked = false
	clientSettings.BlockReason = ""
	clientSettings.BlockedAt = nil

	if err := memory.Update[*dns_server.DNSClientSettings](db, "dns_client_settings", &clientSettings); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to unblock client: " + err.Error(),
		})
	}

	// Invalidate client cache after unblocking
	filterEngine := server.GetFilterEngine()
	filterEngine.InvalidateClientCache(clientIP)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Client unblocked successfully",
		"data":  clientSettings,
	})
}

// GetClientDomainRules returns client-specific domain rules
func GetClientDomainRules(c *fiber.Ctx) error {
	server := dns_server.GetServer()
	db, err := server.GetDB()
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "DNS server not initialized",
		})
	}

	clientIP := c.Query("client_ip")
	var rules []*dns_server.DNSClientDomainRule

	if clientIP != "" {
		rules = memory.Filter[*dns_server.DNSClientDomainRule](db, "dns_client_rules", func(r *dns_server.DNSClientDomainRule) bool {
			return r.ClientIP == clientIP
		})
	} else {
		rules = memory.FindAll[*dns_server.DNSClientDomainRule](db, "dns_client_rules")
	}

	// Sort by CreatedAt descending
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].CreatedAt.After(rules[j].CreatedAt)
	})

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  rules,
	})
}

// CreateClientDomainRule creates a client-specific domain rule
func CreateClientDomainRule(c *fiber.Ctx) error {
	server := dns_server.GetServer()
	db, err := server.GetDB()
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "DNS server not initialized",
		})
	}

	var rule dns_server.DNSClientDomainRule
	if err := c.BodyParser(&rule); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid request body: " + err.Error(),
		})
	}

	// Validate
	if rule.ClientIP == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Client IP is required",
		})
	}
	if rule.Domain == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Domain is required",
		})
	}
	if rule.Type != "block" && rule.Type != "allow" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Type must be 'block' or 'allow'",
		})
	}

	// Normalize domain (remove trailing dot, lowercase, trim)
	rule.Domain = strings.TrimSuffix(strings.TrimSpace(strings.ToLower(rule.Domain)), ".")

	if err := memory.Create[*dns_server.DNSClientDomainRule](db, "dns_client_rules", &rule); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to create rule: " + err.Error(),
		})
	}

	// Invalidate client cache
	filterEngine := server.GetFilterEngine()
	filterEngine.InvalidateClientCache(rule.ClientIP)

	// Reload filters
	go server.ReloadFilters()

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Client domain rule created successfully",
		"data":  rule,
	})
}

// DeleteClientDomainRule deletes a client-specific domain rule
func DeleteClientDomainRule(c *fiber.Ctx) error {
	server := dns_server.GetServer()
	db, err := server.GetDB()
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "DNS server not initialized",
		})
	}

	idStr := c.Params("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid ID",
		})
	}

	if err := memory.Delete[*dns_server.DNSClientDomainRule](db, "dns_client_rules", uint(id)); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to delete rule: " + err.Error(),
		})
	}

	// Reload filters
	go server.ReloadFilters()

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Client domain rule deleted successfully",
	})
}

// DeleteClientDomainRuleByDetails deletes a client rule by client_ip, domain, and type
func DeleteClientDomainRuleByDetails(c *fiber.Ctx) error {
	server := dns_server.GetServer()
	db, err := server.GetDB()
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "DNS server not initialized",
		})
	}

	clientIP := c.Query("client_ip")
	domain := c.Query("domain")
	ruleType := c.Query("type") // "block" or "allow"

	if clientIP == "" || domain == "" || ruleType == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "client_ip, domain, and type are required",
		})
	}

	// Normalize domain
	domain = strings.TrimSuffix(strings.TrimSpace(strings.ToLower(domain)), ".")

	// Delete the rule
	rules := memory.Filter[*dns_server.DNSClientDomainRule](db, "dns_client_rules", func(r *dns_server.DNSClientDomainRule) bool {
		return r.ClientIP == clientIP && r.Domain == domain && r.Type == ruleType
	})

	deletedCount := 0
	for _, rule := range rules {
		if err := memory.Delete[*dns_server.DNSClientDomainRule](db, "dns_client_rules", rule.GetID()); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": true,
				"msg":   "Failed to delete rule: " + err.Error(),
			})
		}
		deletedCount++
	}

	// Invalidate client cache
	filterEngine := server.GetFilterEngine()
	filterEngine.InvalidateClientCache(clientIP)

	// Reload filters
	go server.ReloadFilters()

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Client domain rule deleted successfully",
		"data": fiber.Map{
			"deleted_count": deletedCount,
		},
	})
}

// GetAllCustomRules returns all custom rules (global + client-specific)
func GetAllCustomRules(c *fiber.Ctx) error {
	server := dns_server.GetServer()
	db, err := server.GetDB()
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "DNS server not initialized",
		})
	}

	// Get global custom filters
	globalFilters := memory.FindAll[*dns_server.DNSCustomFilter](db, "dns_custom_filters")
	sort.Slice(globalFilters, func(i, j int) bool {
		return globalFilters[i].CreatedAt.After(globalFilters[j].CreatedAt)
	})

	// Get client-specific rules
	clientRules := memory.FindAll[*dns_server.DNSClientDomainRule](db, "dns_client_rules")
	sort.Slice(clientRules, func(i, j int) bool {
		return clientRules[i].CreatedAt.After(clientRules[j].CreatedAt)
	})

	// Get banned clients
	bannedClients := memory.Filter[*dns_server.DNSClientSettings](db, "dns_client_settings", func(c *dns_server.DNSClientSettings) bool {
		return c.Blocked
	})
	sort.Slice(bannedClients, func(i, j int) bool {
		if bannedClients[i].BlockedAt == nil && bannedClients[j].BlockedAt == nil {
			return false
		}
		if bannedClients[i].BlockedAt == nil {
			return false
		}
		if bannedClients[j].BlockedAt == nil {
			return true
		}
		return bannedClients[i].BlockedAt.After(*bannedClients[j].BlockedAt)
	})

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data": fiber.Map{
			"global_filters": globalFilters,
			"client_rules":   clientRules,
			"banned_clients": bannedClients,
		},
	})
}

// CheckDomainStatus checks the current rule status for a domain and client
func CheckDomainStatus(c *fiber.Ctx) error {
	server := dns_server.GetServer()
	if server == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "DNS server not initialized",
		})
	}

	domain := c.Query("domain")
	clientIP := c.Query("client_ip")

	if domain == "" || clientIP == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "domain and client_ip are required",
		})
	}

	// Normalize domain (trim, lowercase, remove trailing dot)
	domain = strings.TrimSpace(strings.ToLower(domain))
	domain = strings.TrimSuffix(domain, ".")

	filterEngine := server.GetFilterEngine()

	// Get individual rule statuses
	globallyBlocked := filterEngine.IsGloballyBlocked(domain)
	clientBlocked := filterEngine.IsClientBlocked(clientIP, domain)
	clientBanned := filterEngine.IsClientBanned(clientIP)

	// Return simple block status - true means blocked, false means not blocked
	// Frontend: if true → show "Allow", if false → show "Block"
	actions := fiber.Map{
		"global_domain_block":   globallyBlocked, // Domain globally blocked?
		"client_specific_block": clientBlocked,   // Domain blocked for this specific client?
		"client_block":          clientBanned,    // Client IP banned?
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  actions,
	})
}
