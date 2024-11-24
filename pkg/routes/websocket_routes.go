package routes

import (
	"redock/app/controllers"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
)

// SetupRoutes function to setup all routes
func WebSocketRoutes(app *fiber.App) {
	app.Get("/ws/:containerID?", websocket.New(controllers.Attach))
}
