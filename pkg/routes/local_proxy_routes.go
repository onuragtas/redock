package routes

import (
	"redock/app/controllers"

	"github.com/gofiber/fiber/v2"
)

// TunnelRoutes func for describe group of private routes.
func LocalProxyRoutes(a *fiber.App) {
	// Create routes group.
	route := a.Group("/api/v1")

	route.Post("/local_proxy/create", controllers.LocalProxyCreate)
	route.Get("/local_proxy/list", controllers.LocalProxyList)
	route.Post("/local_proxy/start", controllers.LocalProxyStart)
	route.Post("/local_proxy/stop", controllers.LocalProxyStop)
	route.Post("/local_proxy/delete", controllers.LocalProxyDelete)
	route.Get("/local_proxy/start_all", controllers.LocalProxyStartAll)
}
