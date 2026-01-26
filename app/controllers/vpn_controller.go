package controllers

import (
	"encoding/base64"
	"fmt"
	"redock/platform/memory"
	"redock/vpn_server"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/skip2/go-qrcode"
)

// GetVPNServers returns all VPN servers
func GetVPNServers(c *fiber.Ctx) error {
	manager := vpn_server.GetManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "VPN manager not initialized",
		})
	}

	db, err := manager.GetDB()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Database error: " + err.Error(),
		})
	}

	servers := memory.FindAll[*vpn_server.VPNServer](db, "vpn_servers")

	// Add running status to each server
	type ServerWithStatus struct {
		vpn_server.VPNServer
		Running bool `json:"running"`
	}

	serversWithStatus := make([]ServerWithStatus, len(servers))
	for i, server := range servers {
		serversWithStatus[i] = ServerWithStatus{
			VPNServer: *server,
			Running:   manager.IsServerRunning(server.ID),
		}
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  serversWithStatus,
	})
}

// CreateVPNServer creates a new VPN server
func CreateVPNServer(c *fiber.Ctx) error {
	manager := vpn_server.GetManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "VPN manager not initialized",
		})
	}

	var req struct {
		Name     string `json:"name" validate:"required"`
		Address  string `json:"address" validate:"required"`
		Endpoint string `json:"endpoint"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid request body: " + err.Error(),
		})
	}

	server, err := manager.CreateServer(req.Name, req.Address, req.Endpoint)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to create server: " + err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "VPN server created successfully",
		"data":  server,
	})
}

// GetVPNUsers returns all VPN users
func GetVPNUsers(c *fiber.Ctx) error {
	manager := vpn_server.GetManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "VPN manager not initialized",
		})
	}

	db, err := manager.GetDB()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Database error: " + err.Error(),
		})
	}

	serverID := c.Query("server_id")
	var users []*vpn_server.VPNUser
	if serverID != "" {
		serverIDUint, err := strconv.ParseUint(serverID, 10, 32)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": true,
				"msg":   "Invalid server_id parameter",
			})
		}
		users = memory.Filter[*vpn_server.VPNUser](db, "vpn_users", func(x *vpn_server.VPNUser) bool {
			return x.ServerID == uint(serverIDUint)
		})
	} else {
		users = memory.FindAll[*vpn_server.VPNUser](db, "vpn_users")
	}

	// Ensure last_connected_at is always included in response (even if nil)
	type UserResponse struct {
		vpn_server.VPNUser
		LastConnectedAt *time.Time `json:"last_connected_at"` // Remove omitempty to always include
	}

	userResponses := make([]UserResponse, len(users))
	for i, user := range users {
		userResponses[i] = UserResponse{
			VPNUser:         *user,
			LastConnectedAt: user.LastConnectedAt,
		}
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  userResponses,
	})
}

// CreateVPNUser creates a new VPN user
func CreateVPNUser(c *fiber.Ctx) error {
	manager := vpn_server.GetManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "VPN manager not initialized",
		})
	}

	var req struct {
		ServerID uint   `json:"server_id" validate:"required"`
		Username string `json:"username" validate:"required"`
		Email    string `json:"email"`
		FullName string `json:"full_name"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid request body: " + err.Error(),
		})
	}

	user, err := manager.AddUser(req.ServerID, req.Username, req.Email, req.FullName)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to create user: " + err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "VPN user created successfully",
		"data":  user,
	})
}

// GetUserConfig returns WireGuard configuration for a user
func GetUserConfig(c *fiber.Ctx) error {
	manager := vpn_server.GetManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "VPN manager not initialized",
		})
	}

	userID := c.Params("id")
	if userID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "User ID is required",
		})
	}

	var userIDUint uint
	if _, err := fmt.Sscanf(userID, "%d", &userIDUint); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid user ID",
		})
	}

	config, err := manager.GetUserConfig(userIDUint)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to get config: " + err.Error(),
		})
	}

	// Return as text/plain for direct download
	c.Set("Content-Type", "text/plain")
	c.Set("Content-Disposition", "attachment; filename=wg.conf")
	return c.SendString(config)
}

// GetVPNServer returns a specific VPN server
func GetVPNServer(c *fiber.Ctx) error {
	manager := vpn_server.GetManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "VPN manager not initialized",
		})
	}

	db, err := manager.GetDB()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Database error: " + err.Error(),
		})
	}

	id := c.Params("id")
	idUint, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid server ID",
		})
	}
	server, err := memory.FindByID[*vpn_server.VPNServer](db, "vpn_servers", uint(idUint))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "Server not found",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  server,
	})
}

// UpdateVPNServer updates a VPN server
func UpdateVPNServer(c *fiber.Ctx) error {
	manager := vpn_server.GetManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "VPN manager not initialized",
		})
	}

	db, err := manager.GetDB()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Database error: " + err.Error(),
		})
	}

	id := c.Params("id")
	idUint, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid server ID",
		})
	}
	server, err := memory.FindByID[*vpn_server.VPNServer](db, "vpn_servers", uint(idUint))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "Server not found",
		})
	}

	if err := c.BodyParser(server); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid request body: " + err.Error(),
		})
	}

	if err := memory.Update[*vpn_server.VPNServer](db, "vpn_servers", server); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to update server: " + err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Server updated successfully",
		"data":  server,
	})
}

// DeleteVPNServer deletes a VPN server
func DeleteVPNServer(c *fiber.Ctx) error {
	manager := vpn_server.GetManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "VPN manager not initialized",
		})
	}

	db, err := manager.GetDB()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Database error: " + err.Error(),
		})
	}

	id := c.Params("id")
	idUint, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid server ID",
		})
	}
	if err := memory.Delete[*vpn_server.VPNServer](db, "vpn_servers", uint(idUint)); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to delete server: " + err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Server deleted successfully",
	})
}

// GetVPNUser returns a specific VPN user
func GetVPNUser(c *fiber.Ctx) error {
	manager := vpn_server.GetManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "VPN manager not initialized",
		})
	}

	db, err := manager.GetDB()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Database error: " + err.Error(),
		})
	}

	id := c.Params("id")
	idUint, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid user ID",
		})
	}
	user, err := memory.FindByID[*vpn_server.VPNUser](db, "vpn_users", uint(idUint))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "User not found",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  user,
	})
}

// UpdateVPNUser updates a VPN user
func UpdateVPNUser(c *fiber.Ctx) error {
	manager := vpn_server.GetManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "VPN manager not initialized",
		})
	}

	db, err := manager.GetDB()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Database error: " + err.Error(),
		})
	}

	id := c.Params("id")
	idUint, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid user ID",
		})
	}
	user, err := memory.FindByID[*vpn_server.VPNUser](db, "vpn_users", uint(idUint))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "User not found",
		})
	}

	if err := c.BodyParser(user); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid request body: " + err.Error(),
		})
	}

	if err := memory.Update[*vpn_server.VPNUser](db, "vpn_users", user); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to update user: " + err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "User updated successfully",
		"data":  user,
	})
}

// DeleteVPNUser deletes a VPN user
func DeleteVPNUser(c *fiber.Ctx) error {
	manager := vpn_server.GetManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "VPN manager not initialized",
		})
	}

	db, err := manager.GetDB()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Database error: " + err.Error(),
		})
	}

	id := c.Params("id")
	idUint, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid user ID",
		})
	}
	if err := memory.Delete[*vpn_server.VPNUser](db, "vpn_users", uint(idUint)); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to delete user: " + err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "User deleted successfully",
	})
}

// GetUserQRCode generates QR code for user config
func GetUserQRCode(c *fiber.Ctx) error {
	manager := vpn_server.GetManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "VPN manager not initialized",
		})
	}

	userID := c.Params("id")
	var userIDUint uint
	if _, err := fmt.Sscanf(userID, "%d", &userIDUint); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid user ID",
		})
	}

	config, err := manager.GetUserConfig(userIDUint)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to get config: " + err.Error(),
		})
	}

	// Generate QR code from config (PNG format, base64 encoded)
	qrCode, err := qrcode.New(config, qrcode.Medium)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to generate QR code: " + err.Error(),
		})
	}

	// Generate PNG bytes
	qrPNG, err := qrCode.PNG(256)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to generate QR code PNG: " + err.Error(),
		})
	}

	// Encode to base64 for frontend
	qrBase64 := base64.StdEncoding.EncodeToString(qrPNG)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data": fiber.Map{
			"config": config,
			"qrcode": "data:image/png;base64," + qrBase64,
		},
	})
}

// GetVPNStatistics returns overall VPN statistics
func GetVPNStatistics(c *fiber.Ctx) error {
	manager := vpn_server.GetManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "VPN manager not initialized",
		})
	}

	db, err := manager.GetDB()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Database error: " + err.Error(),
		})
	}

	totalServers := int64(memory.Count[*vpn_server.VPNServer](db, "vpn_servers"))
	totalUsers := int64(memory.Count[*vpn_server.VPNUser](db, "vpn_users"))
	activeConnections := int64(len(memory.Filter[*vpn_server.VPNConnection](db, "vpn_connections", func(x *vpn_server.VPNConnection) bool {
		return x.Status == "connected"
	})))

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data": fiber.Map{
			"total_servers":      totalServers,
			"total_users":        totalUsers,
			"active_connections": activeConnections,
		},
	})
}

// GetBandwidthStatistics returns bandwidth statistics
func GetBandwidthStatistics(c *fiber.Ctx) error {
	manager := vpn_server.GetManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "VPN manager not initialized",
		})
	}

	db, err := manager.GetDB()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Database error: " + err.Error(),
		})
	}

	// Total bandwidth across all users
	var totalReceived, totalSent int64
	users := memory.FindAll[*vpn_server.VPNUser](db, "vpn_users")
	for _, user := range users {
		totalReceived += user.TotalBytesReceived
		totalSent += user.TotalBytesSent
	}

	// Top 10 users by bandwidth
	type UserBandwidth struct {
		Username string `json:"username"`
		Received int64  `json:"received"`
		Sent     int64  `json:"sent"`
		Total    int64  `json:"total"`
	}
	var topUsers []UserBandwidth
	for _, user := range users {
		total := user.TotalBytesReceived + user.TotalBytesSent
		topUsers = append(topUsers, UserBandwidth{
			Username: user.Username,
			Received: user.TotalBytesReceived,
			Sent:     user.TotalBytesSent,
			Total:    total,
		})
	}
	// Sort by total descending and take top 10
	for i := 0; i < len(topUsers)-1; i++ {
		for j := i + 1; j < len(topUsers); j++ {
			if topUsers[i].Total < topUsers[j].Total {
				topUsers[i], topUsers[j] = topUsers[j], topUsers[i]
			}
		}
	}
	if len(topUsers) > 10 {
		topUsers = topUsers[:10]
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data": fiber.Map{
			"total_received":  totalReceived,
			"total_sent":      totalSent,
			"total_bandwidth": totalReceived + totalSent,
			"top_users":       topUsers,
		},
	})
}

// GetConnectionStatistics returns connection statistics
func GetConnectionStatistics(c *fiber.Ctx) error {
	manager := vpn_server.GetManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "VPN manager not initialized",
		})
	}

	db, err := manager.GetDB()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Database error: " + err.Error(),
		})
	}

	// Total connections
	var totalConnections int64
	users := memory.FindAll[*vpn_server.VPNUser](db, "vpn_users")
	for _, user := range users {
		totalConnections += int64(user.TotalConnections)
	}

	// Total duration (seconds)
	var totalDuration int64
	for _, user := range users {
		totalDuration += user.TotalDuration
	}

	// Users with connections in last 24h
	twentyFourHoursAgo := time.Now().Add(-24 * time.Hour)
	activeUsers24h := int64(len(memory.Filter[*vpn_server.VPNUser](db, "vpn_users", func(x *vpn_server.VPNUser) bool {
		return x.LastConnectedAt != nil && x.LastConnectedAt.After(twentyFourHoursAgo)
	})))

	// Average connection duration
	var avgDuration float64
	if totalConnections > 0 {
		avgDuration = float64(totalDuration) / float64(totalConnections)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data": fiber.Map{
			"total_connections": totalConnections,
			"total_duration":    totalDuration,
			"avg_duration":      avgDuration,
			"active_users_24h":  activeUsers24h,
		},
	})
}

// GetActiveConnections returns active VPN connections
func GetActiveConnections(c *fiber.Ctx) error {
	manager := vpn_server.GetManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "VPN manager not initialized",
		})
	}

	db, err := manager.GetDB()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Database error: " + err.Error(),
		})
	}

	connections := memory.Filter[*vpn_server.VPNConnection](db, "vpn_connections", func(x *vpn_server.VPNConnection) bool {
		return x.Status == "connected"
	})

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  connections,
	})
}

// GetConnectionDetails returns details of a specific connection
func GetConnectionDetails(c *fiber.Ctx) error {
	manager := vpn_server.GetManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "VPN manager not initialized",
		})
	}

	db, err := manager.GetDB()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Database error: " + err.Error(),
		})
	}

	id := c.Params("id")
	idUint, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid connection ID",
		})
	}
	connection, err := memory.FindByID[*vpn_server.VPNConnection](db, "vpn_connections", uint(idUint))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "Connection not found",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  connection,
	})
}

// StartVPNServer starts a VPN server
func StartVPNServer(c *fiber.Ctx) error {
	manager := vpn_server.GetManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "VPN manager not initialized",
		})
	}

	id := c.Params("id")
	var serverID uint
	if _, err := fmt.Sscanf(id, "%d", &serverID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid server ID",
		})
	}

	if err := manager.StartServer(serverID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to start server: " + err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Server started successfully",
	})
}

// StopVPNServer stops a VPN server
func StopVPNServer(c *fiber.Ctx) error {
	manager := vpn_server.GetManager()
	if manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "VPN manager not initialized",
		})
	}

	id := c.Params("id")
	var serverID uint
	if _, err := fmt.Sscanf(id, "%d", &serverID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid server ID",
		})
	}

	if err := manager.StopServer(serverID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to stop server: " + err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "Server stopped successfully",
	})
}
