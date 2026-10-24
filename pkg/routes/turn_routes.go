package routes

import (
	"redock/app/controllers"
	"redock/pkg/middleware"

	"github.com/gofiber/fiber/v2"
)

// TURNRoutes registers the TURN/STUN server endpoints. Everything that
// changes the server, its apps or reveals secrets is admin-only.
func TURNRoutes(a *fiber.App) {
	route := a.Group("/api/v1/turn", middleware.JWTProtected())
	admin := middleware.AdminOnly()

	route.Get("/status", controllers.TURNStatus)
	route.Get("/config", controllers.TURNGetConfig)
	route.Post("/config", admin, controllers.TURNUpdateConfig)
	route.Post("/start", admin, controllers.TURNStart)
	route.Post("/stop", admin, controllers.TURNStop)

	route.Get("/apps", controllers.TURNListApps)
	route.Post("/apps", admin, controllers.TURNCreateApp)
	route.Post("/apps/update", admin, controllers.TURNUpdateApp)
	route.Post("/apps/rotate", admin, controllers.TURNRotateSecret)
	route.Post("/apps/delete", admin, controllers.TURNDeleteApp)
	route.Post("/credentials", admin, controllers.TURNIssueCredentials)

	route.Get("/sessions", controllers.TURNSessions)
	route.Post("/sessions/kill", admin, controllers.TURNKillSession)
	route.Get("/events", controllers.TURNEvents)
	route.Get("/blocked", controllers.TURNBlocked)
	route.Post("/blocked/unblock", admin, controllers.TURNUnblock)
}
