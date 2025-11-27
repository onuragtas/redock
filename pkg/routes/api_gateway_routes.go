package routes

import (
	"redock/app/controllers"

	"github.com/gofiber/fiber/v2"
)

// APIGatewayRoutes func for describe group of API Gateway routes.
func APIGatewayRoutes(a *fiber.App) {
	// Create routes group.
	route := a.Group("/api/v1")

	// Gateway control
	route.Get("/api_gateway/config", controllers.APIGatewayGetConfig)
	route.Post("/api_gateway/config", controllers.APIGatewayUpdateConfig)
	route.Post("/api_gateway/start", controllers.APIGatewayStart)
	route.Post("/api_gateway/stop", controllers.APIGatewayStop)
	route.Get("/api_gateway/status", controllers.APIGatewayStatus)
	route.Get("/api_gateway/stats", controllers.APIGatewayGetStats)
	route.Get("/api_gateway/health", controllers.APIGatewayGetServiceHealth)

	// Services management
	route.Get("/api_gateway/services", controllers.APIGatewayListServices)
	route.Post("/api_gateway/services", controllers.APIGatewayAddService)
	route.Put("/api_gateway/services", controllers.APIGatewayUpdateService)
	route.Delete("/api_gateway/services", controllers.APIGatewayDeleteService)

	// Routes management
	route.Get("/api_gateway/routes", controllers.APIGatewayListRoutes)
	route.Post("/api_gateway/routes", controllers.APIGatewayAddRoute)
	route.Put("/api_gateway/routes", controllers.APIGatewayUpdateRoute)
	route.Delete("/api_gateway/routes", controllers.APIGatewayDeleteRoute)

	// Testing and validation
	route.Post("/api_gateway/test_upstream", controllers.APIGatewayTestUpstream)
	route.Post("/api_gateway/health_check", controllers.APIGatewayHealthCheckNow)
	route.Post("/api_gateway/validate", controllers.APIGatewayValidateRoute)
}
