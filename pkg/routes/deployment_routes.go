package routes

import (
	"redock/app/controllers"
	"redock/pkg/middleware"

	"github.com/gofiber/fiber/v2"
)

// DeploymentRoutes func for describe group of deployment routes (JWT protected).
func DeploymentRoutes(a *fiber.App) {
	route := a.Group("/api/v1/deployment", middleware.JWTProtected())
	route.Get("/list", controllers.DeploymentList)
	route.Post("/add", controllers.DeploymentAdd)
	route.Post("/update", controllers.DeploymentUpdate)
	route.Post("/delete", controllers.DeploymentDelete)
	route.Get("/get", controllers.DeploymentGet)
	route.Post("/set_credentials", controllers.DeploymentSetCredentials)
	route.Get("/settings", controllers.DeploymentGetSettings)
}
