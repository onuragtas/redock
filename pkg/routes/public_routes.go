package routes

import (
	"redock/app/controllers"

	"github.com/gofiber/fiber/v2"
)

// PublicRoutes func for describe group of public routes.
func PublicRoutes(a *fiber.App) {
	// Create routes group.
	route := a.Group("/api/v1")

	// Auth setup (no JWT): for login page register visibility
	route.Get("/auth/setup", controllers.AuthSetup)
	// Routes for POST method:
	route.Post("/user/sign/up", controllers.UserSignUp) // register a new user
	route.Post("/user/sign/in", controllers.UserSignIn) // auth, return Access & Refresh tokens
	// Token renew: accepts expired access token in Authorization + refresh_token in body
	route.Post("/token/renew", controllers.RenewTokens)
	// Tünel auth (public): giriş, kayıt, OAuth callback — JWT yok
	route.Post("/tunnel/auth/login", controllers.TunnelLogin)
	route.Post("/tunnel/auth/register", controllers.TunnelRegister)
	route.Get("/tunnel/auth/callback", controllers.TunnelAuthCallback)
}
