package routes

import (
	"redock/app/controllers"
	"redock/pkg/middleware"

	"github.com/gofiber/fiber/v2"
)

func UpdateRoutes(app *fiber.App) {
	updates := app.Group("/api/updates", middleware.JWTProtected())

	// Get current version
	updates.Get("/version", controllers.GetCurrentVersion)

	// Get available updates
	updates.Get("/available", controllers.GetAvailableUpdates)

	// Apply an update
	updates.Post("/apply", controllers.ApplyUpdate)
}
