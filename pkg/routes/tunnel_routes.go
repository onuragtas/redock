package routes

import (
	"redock/app/controllers"
	"redock/pkg/middleware"

	"github.com/gofiber/fiber/v2"
)

// TunnelServerRoutes — Tünel sunucusu yönetimi. Sadece Redock JWT (admin).
// Prefix: /api/v1/tunnel/server
func TunnelServerRoutes(a *fiber.App) {
	api := a.Group("/api/v1")
	server := api.Group("/tunnel/server", middleware.JWTProtected())
	server.Get("/config", controllers.TunnelServerGetConfig)
	server.Patch("/config", controllers.TunnelServerUpdateConfig)
}

// TunnelClientRoutes — Tünel istemcisi (federation, proxy, oturum). Sadece Redock JWT.
// Prefix: /api/v1/tunnel/client
func TunnelClientRoutes(a *fiber.App) {
	api := a.Group("/api/v1")
	client := api.Group("/tunnel/client", middleware.JWTProtected())
	client.Get("/check-login", controllers.CheckUser)
	client.Get("/logout", controllers.TunnelLogout)
	client.Get("/user-info", controllers.TunnelUserInfo)
	// Federation: eklenen tünel sunucuları
	client.Get("/servers", controllers.TunnelServerListServers)
	client.Post("/servers", controllers.TunnelServerCreateServer)
	client.Patch("/servers/:id", controllers.TunnelServerUpdateServer)
	client.Delete("/servers/:id", controllers.TunnelServerDeleteServer)
	client.Get("/credentials", controllers.TunnelCredentialList)
	client.Post("/credentials", controllers.TunnelCredentialSave)
	client.Post("/auth/prepare", controllers.TunnelAuthPrepare)
	// Proxy: server_id ile harici sunucuya istek
	client.Get("/proxy/domains", controllers.TunnelProxyDomainsList)
	client.Post("/proxy/domains", controllers.TunnelProxyDomainCreate)
	client.Delete("/proxy/domains/:id", controllers.TunnelProxyDomainDelete)
	client.Get("/proxy/list", controllers.TunnelProxyList)
	client.Post("/proxy/add", controllers.TunnelProxyAdd)
	client.Post("/proxy/delete", controllers.TunnelProxyDelete)
	client.Post("/proxy/start", controllers.TunnelProxyStart)
	client.Post("/proxy/stop", controllers.TunnelProxyStop)
	client.Post("/proxy/renew", controllers.TunnelProxyRenew)
}

// TunnelApiRoutes — Tünel API: domain listesi / oluşturma / silme, start/stop/renew.
// Bearer = tunnel token veya Redock JWT (controller getTunnelServerAuth).
// Prefix: /api/v1/tunnel (auth middleware yok; controller token kontrolü yapar).
func TunnelApiRoutes(a *fiber.App) {
	api := a.Group("/api/v1")
	tunnel := api.Group("/tunnel")
	tunnel.Get("/domains", controllers.TunnelServerListDomains)
	tunnel.Post("/domains", controllers.TunnelServerCreateDomain)
	tunnel.Delete("/domains/:id", controllers.TunnelServerDeleteDomain)
	tunnel.Post("/start", controllers.TunnelStart)
	tunnel.Post("/stop", controllers.TunnelStop)
	tunnel.Post("/renew", controllers.TunnelRenew)
}
