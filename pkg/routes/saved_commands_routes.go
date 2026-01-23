package routes

import (
	"redock/app/controllers"

	"github.com/gofiber/fiber/v2"
)

// SavedCommandRoutes func for describe group of private routes.
func SavedCommandRoutes(a *fiber.App) {
	// Create routes group.
	route := a.Group("/api/v1/saved_commands")

	// RESTful routes
	route.Get("/", controllers.GetSavedCommands)        // GET /api/v1/saved_commands
	route.Post("/", controllers.AddCommand)             // POST /api/v1/saved_commands
	route.Get("/:id", controllers.GetCommandByID)       // GET /api/v1/saved_commands/:id
	route.Put("/:id", controllers.UpdateCommandByID)    // PUT /api/v1/saved_commands/:id
	route.Delete("/:id", controllers.RemoveCommandByID) // DELETE /api/v1/saved_commands/:id
}
