package routes

import (
	"redock/app/controllers"

	"github.com/gofiber/fiber/v2"
)

// TunnelRoutes func for describe group of private routes.
func TunnelRoutes(a *fiber.App) {
	// Create routes group.
	route := a.Group("/api/v1")

	route.Get("/tunnel/check_login", controllers.CheckUser)
	route.Post("/tunnel/login", controllers.TunnelLogin)
	route.Post("/tunnel/register", controllers.TunnelRegister)
	route.Get("/tunnel/logout", controllers.TunnelLogout)
	route.Get("/tunnel/list", controllers.TunnelList)
	route.Post("/tunnel/delete", controllers.TunnelDelete)
	route.Post("/tunnel/add", controllers.TunnelAdd)
	route.Post("/tunnel/start", controllers.TunnelStart)
	route.Post("/tunnel/stop", controllers.TunnelStop)
	route.Get("/tunnel/user_info", controllers.TunnelUserInfo)
}
