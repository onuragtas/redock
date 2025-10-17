package routes

import (
	"redock/app/controllers"

	"github.com/gofiber/fiber/v2"
)

// UsageRoutes func for describe group of usage routes.
func UsageRoutes(a *fiber.App) {
	// Create routes group.
	route := a.Group("/api/v1/usage")

	route.Get("/list", controllers.UsageList)
}
