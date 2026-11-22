package routes

import (
	"redock/app/controllers"
	"redock/pkg/middleware"

	"github.com/gofiber/fiber/v2"
)

// BackupRoutes registers backup management endpoints (JWT-protected).
func BackupRoutes(a *fiber.App) {
	route := a.Group("/api/v1", middleware.JWTProtected())
	// Admin only, scoped to this file's own prefix: a middleware on the "/api/v1" group itself would run for
	// every /api/v1 route registered after it.
	route.Use("/backups", middleware.AdminOnly())

	route.Get("/backups", controllers.BackupList)
	route.Post("/backups", controllers.BackupCreate)
	route.Delete("/backups", controllers.BackupDelete)
	route.Get("/backups/config", controllers.BackupGetConfig)
	route.Put("/backups/config", controllers.BackupUpdateConfig)
	route.Get("/backups/download", controllers.BackupDownload)
	route.Post("/backups/upload", controllers.BackupUpload)
	route.Post("/backups/restore", controllers.BackupRestore)
}
