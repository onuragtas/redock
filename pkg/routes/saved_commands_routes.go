package routes

import (
	"redock/app/controllers"
	"redock/pkg/middleware"

	"github.com/gofiber/fiber/v2"
)

// SavedCommandRoutes func for describe group of private routes (JWT protected).
func SavedCommandRoutes(a *fiber.App) {
	route := a.Group("/api/v1/saved_commands", middleware.JWTProtected())
	route.Get("/", controllers.GetSavedCommands)
	route.Post("/", controllers.AddCommand)
	route.Get("/:id", controllers.GetCommandByID)
	route.Put("/:id", controllers.UpdateCommandByID)
	route.Delete("/:id", controllers.RemoveCommandByID)
}
