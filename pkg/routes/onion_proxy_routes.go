package routes

import (
	"redock/app/controllers"
	"redock/pkg/middleware"

	"github.com/gofiber/fiber/v2"
)

// OnionProxyRoutes registers /api/v1/onion CRUD endpoints (JWT-protected).
func OnionProxyRoutes(a *fiber.App) {
	route := a.Group("/api/v1/onion", middleware.JWTProtected())

	route.Get("/status", controllers.OnionStatus)
	route.Get("/list", controllers.OnionList)
	route.Post("/create", controllers.OnionCreate)
	route.Put("/:id", controllers.OnionUpdate)
	route.Delete("/:id", controllers.OnionDelete)
}
