package routes

import (
	"redock/app/controllers"

	"github.com/gofiber/fiber/v2"
)

// PHPXDebugAdapterRoutes func for describe group of private routes.
func SavedCommandRoutes(a *fiber.App) {
	// Create routes group.
	route := a.Group("/api/v1")

	route.Get("/saved_commands/list", controllers.GetSavedCommands)
	route.Post("/saved_commands/add", controllers.AddCommand)
	route.Post("/saved_commands/remove", controllers.RemoveCommand)
}
