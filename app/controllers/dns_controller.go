package controllers

import (
	"redock/dns_server"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
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

	var blocklists []dns_server.DNSBlocklist
	db, err := server.GetDB()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	if err := db.Find(&blocklists).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

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

	if err := db.Create(&blocklist).Error; err != nil {
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

	if err := db.Save(&blocklist).Error; err != nil {
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

	if err := db.Delete(&dns_server.DNSBlocklist{}, id).Error; err != nil {
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

	var filters []dns_server.DNSCustomFilter
	db, err := server.GetDB()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	if err := db.Find(&filters).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

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

	if err := db.Create(&filter).Error; err != nil {
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

	if err := db.Delete(&dns_server.DNSCustomFilter{}, id).Error; err != nil {
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
	result := db.Where("domain = ? AND type = ?", domain, filterType).
		Delete(&dns_server.DNSCustomFilter{})

	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to delete filter: " + result.Error.Error(),
		})
	}

	// Reload filters
	go server.ReloadFilters()

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Filter deleted successfully",
		"data": fiber.Map{
			"deleted_count": result.RowsAffected,
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

	offset := (page - 1) * limit

	var logs []dns_server.DNSQueryLog
	var total int64

	db, err := server.GetDB()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	db.Model(&dns_server.DNSQueryLog{}).Count(&total)
	err = db.Order("created_at DESC").Limit(limit).Offset(offset).Find(&logs).Error

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
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

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  stats,
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

	// Get active clients from query logs (last 24h)
	db, err := server.GetDB()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	type ClientStats struct {
		IP           string `json:"ip"`
		QueryCount   int64  `json:"query_count"`
		BlockedCount int64  `json:"blocked_count"`
		LastSeen     string `json:"last_seen"` // SQLite MAX() returns string in GROUP BY
		IsBanned     bool   `json:"is_banned"`
	}

	var clients []ClientStats
	last24h := time.Now().Add(-24 * time.Hour)

	err = db.Model(&dns_server.DNSQueryLog{}).
		Select("client_ip as ip, COUNT(*) as query_count, SUM(CASE WHEN blocked THEN 1 ELSE 0 END) as blocked_count, MAX(created_at) as last_seen").
		Where("created_at >= ?", last24h).
		Group("client_ip").
		Order("query_count DESC").
		Limit(100).
		Scan(&clients).Error

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
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

	if err := db.Create(&client).Error; err != nil {
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

	var rewrites []dns_server.DNSRewrite
	if err := db.Order("created_at DESC").Find(&rewrites).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to fetch rewrites: " + err.Error(),
		})
	}

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

	if err := db.Create(&rewrite).Error; err != nil {
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

	id := c.Params("id")
	var rewrite dns_server.DNSRewrite
	if err := db.First(&rewrite, id).Error; err != nil {
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

	if err := db.Save(&rewrite).Error; err != nil {
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

	id := c.Params("id")
	if err := db.Delete(&dns_server.DNSRewrite{}, id).Error; err != nil {
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
	var clientSettings dns_server.DNSClientSettings
	result := db.Where("client_ip = ?", req.ClientIP).First(&clientSettings)

	now := time.Now()
	if result.Error == gorm.ErrRecordNotFound {
		clientSettings = dns_server.DNSClientSettings{
			ClientIP:    req.ClientIP,
			Blocked:     true,
			BlockReason: req.Reason,
			BlockedAt:   &now,
		}
		if err := db.Create(&clientSettings).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": true,
				"msg":   "Failed to block client: " + err.Error(),
			})
		}
	} else {
		clientSettings.Blocked = true
		clientSettings.BlockReason = req.Reason
		clientSettings.BlockedAt = &now
		if err := db.Save(&clientSettings).Error; err != nil {
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

	var clientSettings dns_server.DNSClientSettings
	if err := db.Where("client_ip = ?", clientIP).First(&clientSettings).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "Client not found",
		})
	}

	clientSettings.Blocked = false
	clientSettings.BlockReason = ""
	clientSettings.BlockedAt = nil

	if err := db.Save(&clientSettings).Error; err != nil {
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
	var rules []dns_server.DNSClientDomainRule

	query := db.Model(&dns_server.DNSClientDomainRule{})
	if clientIP != "" {
		query = query.Where("client_ip = ?", clientIP)
	}

	if err := query.Order("created_at DESC").Find(&rules).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to fetch rules: " + err.Error(),
		})
	}

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

	if err := db.Create(&rule).Error; err != nil {
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

	id := c.Params("id")
	if err := db.Delete(&dns_server.DNSClientDomainRule{}, id).Error; err != nil {
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
	result := db.Where("client_ip = ? AND domain = ? AND type = ?", clientIP, domain, ruleType).
		Delete(&dns_server.DNSClientDomainRule{})

	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to delete rule: " + result.Error.Error(),
		})
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
			"deleted_count": result.RowsAffected,
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
	var globalFilters []dns_server.DNSCustomFilter
	if err := db.Order("created_at DESC").Find(&globalFilters).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to fetch global filters: " + err.Error(),
		})
	}

	// Get client-specific rules
	var clientRules []dns_server.DNSClientDomainRule
	if err := db.Order("created_at DESC").Find(&clientRules).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to fetch client rules: " + err.Error(),
		})
	}

	// Get banned clients
	var bannedClients []dns_server.DNSClientSettings
	if err := db.Where("blocked = ?", true).Order("blocked_at DESC").Find(&bannedClients).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to fetch banned clients: " + err.Error(),
		})
	}

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
