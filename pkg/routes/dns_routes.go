package routes

import (
	"redock/app/controllers"
	"redock/pkg/middleware"

	"github.com/gofiber/fiber/v2"
)

// DNSRoutes returns DNS server routes (JWT protected)
func DNSRoutes(app *fiber.App) {
	route := app.Group("/v1/dns", middleware.JWTProtected())

	// Config routes
	route.Get("/config", controllers.GetDNSConfig)
	route.Put("/config", controllers.UpdateDNSConfig)

	// Server control
	route.Post("/start", controllers.StartDNSServer)
	route.Post("/stop", controllers.StopDNSServer)
	route.Get("/status", controllers.GetDNSStatus)

	// Blocklists
	route.Get("/blocklists", controllers.GetDNSBlocklists)
	route.Post("/blocklists", controllers.CreateDNSBlocklist)
	route.Put("/blocklists/:id", controllers.UpdateDNSBlocklist)
	route.Delete("/blocklists/:id", controllers.DeleteDNSBlocklist)

	// Custom filters
	route.Get("/filters", controllers.GetDNSCustomFilters)
	route.Post("/filters", controllers.CreateDNSCustomFilter)
	route.Delete("/filters/:id", controllers.DeleteDNSCustomFilter)
	route.Delete("/filters", controllers.DeleteDNSCustomFilterByDetails) // Delete by domain, type

	// DNS Rewrites
	route.Get("/rewrites", controllers.GetDNSRewrites)
	route.Post("/rewrites", controllers.CreateDNSRewrite)
	route.Put("/rewrites/:id", controllers.UpdateDNSRewrite)
	route.Delete("/rewrites/:id", controllers.DeleteDNSRewrite)

	// Query logs
	route.Get("/logs", controllers.GetDNSQueryLogs)

	// Statistics
	route.Get("/stats", controllers.GetDNSStatistics)
	route.Get("/stats/daily", controllers.GetDNSDailyStats)
	route.Get("/history", controllers.GetDNSQueryHistory)

	// Client settings
	route.Get("/clients", controllers.GetDNSClientSettings)
	route.Post("/clients", controllers.CreateDNSClientSettings)

	// Client blocking (IP Ban)
	route.Post("/clients/block", controllers.BlockClient)
	route.Post("/clients/:ip/unblock", controllers.UnblockClient)

	// Client-specific domain rules
	route.Get("/client-rules", controllers.GetClientDomainRules)
	route.Post("/client-rules", controllers.CreateClientDomainRule)
	route.Delete("/client-rules/:id", controllers.DeleteClientDomainRule)
	route.Delete("/client-rules", controllers.DeleteClientDomainRuleByDetails) // Delete by client_ip, domain, type

	// All custom rules (global + client-specific + banned clients)
	route.Get("/custom-rules", controllers.GetAllCustomRules)

	// Check domain status (real-time)
	route.Get("/check-domain-status", controllers.CheckDomainStatus)

	// Actions
	route.Post("/reload", controllers.ReloadDNSFilters)
}
