package routes

import (
	"redock/app/controllers"
	"redock/pkg/middleware"

	"github.com/gofiber/fiber/v2"
)

// PrivateRoutes func for describe group of private routes (all require JWT).
func PrivateRoutes(a *fiber.App) {
	// Create routes group with JWT protection for all private routes.
	route := a.Group("/api/v1", middleware.JWTProtected())

	// Auth: current user from JWT
	route.Get("/auth/me", controllers.AuthMe)
	// Menus: kullanıcının görebileceği menü listesi (path, name, icon)
	route.Get("/menus", controllers.Menus)

	// Users (admin only)
	adminRoute := route.Group("/users", middleware.AdminOnly())
	adminRoute.Get("/", controllers.ListUsers)
	adminRoute.Post("/", controllers.CreateUser)
	adminRoute.Get("/menu-options", controllers.GetMenuOptions)
	adminRoute.Put("/:id", controllers.UpdateUser)
	adminRoute.Delete("/:id", controllers.DeleteUser)
	// User / token
	route.Post("/user/sign/out", controllers.UserSignOut)
	// Docker
	route.Get("/docker/env", controllers.GetEnv)
	route.Post("/docker/env", controllers.SetEnv)
	route.Post("/docker/regenerate", controllers.Regenerate)
	route.Get("/docker/ip", controllers.GetLocalIp)
	route.Get("/docker/services", controllers.GetAllServices)
	route.Get("/docker/service_settings", controllers.GetServiceSettings)
	route.Post("/docker/service_settings", controllers.UpdateServiceSettings)
	route.Get("/docker/vhosts", controllers.GetAllVHosts)
	route.Post("/docker/star_vhost", controllers.StarVHost)
	route.Post("/docker/unstar_vhost", controllers.UnstarVHost)
	route.Post("/docker/get_vhost", controllers.GetVHostContent)
	route.Post("/docker/set_vhost", controllers.SetVHostContent)
	route.Post("/docker/delete_vhost", controllers.DeleteVHost)
	route.Post("/docker/vhost_env_mode", controllers.GetVHostEnvMode)
	route.Post("/docker/toggle_vhost_env", controllers.ToggleVHostEnvMode)
	route.Post("/docker/vhost_terminal_info", controllers.GetVHostTerminalInfo)
	route.Get("/docker/php_services", controllers.GetPhpServices)
	route.Post("/docker/create_vhost", controllers.CreateVHost)
	route.Get("/docker/devenv", controllers.GetDevEnv)
	route.Post("/docker/create_devenv", controllers.CreateDevEnv)
	route.Post("/docker/edit_devenv", controllers.EditDevEnv)
	route.Post("/docker/delete_devenv", controllers.DeleteDevEnv)
	route.Get("/docker/regenerate_devenv", controllers.RegenerateDevEnv)
	route.Get("/docker/install", controllers.Install)
	route.Get("/docker/add_xdebug", controllers.AddXDebug)
	route.Get("/docker/remove_xdebug", controllers.RemoveXDebug)
	route.Get("/docker/restart_nginx_httpd", controllers.RestartNginxHttpd)
	route.Get("/docker/self_update", controllers.SelfUpdate)
	route.Get("/docker/update_docker", controllers.UpdateDocker)
	route.Get("/docker/update_docker_images", controllers.UpdateDockerImages)
	route.Post("/docker/add_service", controllers.AddService)
	route.Post("/docker/remove_service", controllers.RemoveService)
}
