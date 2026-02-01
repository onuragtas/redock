package routes

import (
	"redock/app/controllers"
	"redock/pkg/middleware"

	"github.com/gofiber/fiber/v2"
)

// TunnelRoutes func for tunnel routes: login/register public, rest require JWT.
func TunnelRoutes(a *fiber.App) {
	api := a.Group("/api/v1")
	// Public: tunnel login and register (no JWT)
	api.Post("/tunnel/login", controllers.TunnelLogin)
	api.Post("/tunnel/register", controllers.TunnelRegister)
	// Protected: require Redock JWT for all other tunnel operations
	route := api.Group("", middleware.JWTProtected())
	route.Get("/tunnel/check_login", controllers.CheckUser)
	route.Get("/tunnel/logout", controllers.TunnelLogout)
	route.Get("/tunnel/list", controllers.TunnelList)
	route.Post("/tunnel/delete", controllers.TunnelDelete)
	route.Post("/tunnel/add", controllers.TunnelAdd)
	route.Post("/tunnel/start", controllers.TunnelStart)
	route.Post("/tunnel/stop", controllers.TunnelStop)
	route.Post("/tunnel/renew", controllers.TunnelRenewDomain)
	route.Get("/tunnel/user_info", controllers.TunnelUserInfo)
}
