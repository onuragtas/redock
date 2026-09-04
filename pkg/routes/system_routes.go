package routes

import (
	"redock/app/controllers"
	"redock/pkg/middleware"

	"github.com/gofiber/fiber/v2"
)

// SystemRoutes registers the process-level system routes: the memory guard and
// the alerts that tell the operator when something needs attention.
func SystemRoutes(a *fiber.App) {
	route := a.Group("/api/v1/system", middleware.JWTProtected())

	route.Get("/memory", controllers.MemoryStatus)
	route.Get("/memory/history", controllers.MemoryHistory)
	route.Get("/memory/events", controllers.MemoryEvents)
	route.Put("/memory/config", controllers.MemoryUpdateConfig)
	route.Post("/memory/release", controllers.MemoryRelease)

	route.Get("/notifications", controllers.GetNotifySettings)
	route.Put("/notifications", controllers.UpdateNotifySettings)
	route.Post("/notifications/test", controllers.SendNotifyTest)
	route.Post("/notifications/check", controllers.RunNotifyCheck)
}
