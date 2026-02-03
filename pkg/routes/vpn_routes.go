package routes

import (
	"redock/app/controllers"
	"redock/pkg/middleware"

	"github.com/gofiber/fiber/v2"
)

// SetupVPNRoutes sets up VPN server routes (JWT protected)
func SetupVPNRoutes(app *fiber.App) {
	vpn := app.Group("/v1/vpn", middleware.JWTProtected())

	// Server management
	vpn.Get("/servers", controllers.GetVPNServers)
	vpn.Post("/servers", controllers.CreateVPNServer)
	vpn.Get("/servers/:id", controllers.GetVPNServer)
	vpn.Put("/servers/:id", controllers.UpdateVPNServer)
	vpn.Delete("/servers/:id", controllers.DeleteVPNServer)
	vpn.Post("/servers/:id/start", controllers.StartVPNServer)
	vpn.Post("/servers/:id/stop", controllers.StopVPNServer)

	// User management
	vpn.Get("/users", controllers.GetVPNUsers)
	vpn.Post("/users", controllers.CreateVPNUser)
	vpn.Get("/users/:id", controllers.GetVPNUser)
	vpn.Put("/users/:id", controllers.UpdateVPNUser)
	vpn.Delete("/users/:id", controllers.DeleteVPNUser)
	vpn.Get("/users/:id/config", controllers.GetUserConfig)
	vpn.Get("/users/:id/qrcode", controllers.GetUserQRCode)

	// Statistics
	vpn.Get("/statistics", controllers.GetVPNStatistics)
	vpn.Get("/statistics/bandwidth", controllers.GetBandwidthStatistics)
	vpn.Get("/statistics/connections", controllers.GetConnectionStatistics)

	// Real-time
	vpn.Get("/connections", controllers.GetActiveConnections)
	vpn.Get("/connections/:id", controllers.GetConnectionDetails)
}
