package routes

import (
	"redock/app/controllers"
	"redock/pkg/middleware"

	"github.com/gofiber/fiber/v2"
)

// NetworkRoutes sets up IP alias / network interface routes (JWT protected).
func NetworkRoutes(a *fiber.App) {
	route := a.Group("/api/v1", middleware.JWTProtected())
	// Admin only, scoped to this file's own prefix: a middleware on the "/api/v1" group itself would run for
	// every /api/v1 route registered after it.
	route.Use("/network", middleware.AdminOnly())
	route.Get("/network/interfaces", controllers.NetworkListInterfaces)
	route.Get("/network/addresses", controllers.NetworkListAddresses)
	route.Post("/network/alias/add", controllers.NetworkAddAlias)
	route.Post("/network/alias/remove", controllers.NetworkRemoveAlias)
	route.Get("/network/client-command", controllers.NetworkClientCommand)
}
