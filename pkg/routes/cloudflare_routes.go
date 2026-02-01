package routes

import (
	"redock/app/controllers"
	"redock/pkg/middleware"

	"github.com/gofiber/fiber/v2"
)

func CloudflareRoutes(app *fiber.App) {
	cloudflare := app.Group("/api/cloudflare", middleware.JWTProtected())

	// Account management
	cloudflare.Post("/accounts", controllers.AddCloudflareAccount)
	cloudflare.Get("/accounts", controllers.GetCloudflareAccounts)
	cloudflare.Delete("/accounts/:account_id", controllers.DeleteCloudflareAccount)

	// Zone management
	cloudflare.Post("/accounts/:account_id/sync-zones", controllers.SyncCloudflareZones)
	cloudflare.Get("/accounts/:account_id/zones", controllers.GetCloudflareZones)
	cloudflare.Get("/zones", controllers.GetCloudflareZones)

	// DNS records
	cloudflare.Get("/zones/:zone_id/dns", controllers.GetCloudflareDNSRecords)
	cloudflare.Post("/zones/:zone_id/dns", controllers.CreateCloudflareDNSRecord)
	cloudflare.Put("/zones/:zone_id/dns/:record_id", controllers.UpdateCloudflareDNSRecord)
	cloudflare.Delete("/zones/:zone_id/dns/:record_id", controllers.DeleteCloudflareDNSRecord)
}
