package routes

import (
	"redock/app/controllers"
	"redock/pkg/middleware"

	"github.com/gofiber/fiber/v2"
)

// UsageRoutes func for describe group of usage routes (JWT protected).
func UsageRoutes(a *fiber.App) {
	route := a.Group("/api/v1/usage", middleware.JWTProtected())
	route.Get("/list", controllers.UsageList)
}
