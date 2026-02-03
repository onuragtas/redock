package routes

import (
	"redock/app/controllers"
	"redock/pkg/middleware"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
)

// WebSocketRoutes sets up WebSocket routes (access token required for /ws).
func WebSocketRoutes(app *fiber.App) {
	app.Get("/ws/:containerID?", middleware.WebSocketAccessToken(), websocket.New(controllers.Attach))
}
