package routes

import (
	"redock/app/controllers"

	"github.com/gofiber/fiber/v2"
)

// PHPXDebugAdapterRoutes func for describe group of private routes.
func PHPXDebugAdapterRoutes(a *fiber.App) {
	// Create routes group.
	route := a.Group("/api/v1")

	route.Get("/php_xdebug_adapter/settings", controllers.GetXDebugAdapterSettings)
	route.Post("/php_xdebug_adapter/add", controllers.AddXDebugAdapterListener)
	route.Post("/php_xdebug_adapter/remove", controllers.RemoveXDebugAdapterListener)
	route.Get("/php_xdebug_adapter/start", controllers.XDebugAdapterStart)
	route.Get("/php_xdebug_adapter/stop", controllers.XDebugAdapterStop)
	route.Post("/php_xdebug_adapter/update", controllers.UpdateXDebugAdapterConfiguration)
}
