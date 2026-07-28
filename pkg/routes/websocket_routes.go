package routes

import (
	"redock/app/controllers"
	"redock/pkg/middleware"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
)

// WebSocketRoutes sets up WebSocket routes (access token required for /ws).
func WebSocketRoutes(app *fiber.App) {
	// Registered before the /ws/:containerID? wildcard below, since Fiber
	// matches routes in registration order and "traffic" would otherwise be
	// captured as a containerID.
	app.Get("/ws/traffic", middleware.WebSocketAccessToken(), websocket.New(controllers.AttachTrafficStream))
	app.Get("/ws/:containerID?", middleware.WebSocketAccessToken(), websocket.New(controllers.Attach))
}
