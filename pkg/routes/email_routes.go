package routes

import (
	"redock/app/controllers"

	"github.com/gofiber/fiber/v2"
)

// EmailRoutes sets up email server routes
func EmailRoutes(app *fiber.App) {
	email := app.Group("/api/email")

	// Server management
	email.Get("/server/status", controllers.GetEmailServerStatus)
	email.Get("/server/config", controllers.GetServerConfig)
	email.Put("/server/ip", controllers.UpdateServerIP)
	email.Post("/server/start", controllers.StartEmailServer)
	email.Post("/server/stop", controllers.StopEmailServer)
	email.Get("/server/check-passwords", controllers.CheckMailboxPasswords)

	// Domain management
	email.Get("/domains", controllers.GetEmailDomains)
	email.Post("/domains", controllers.AddEmailDomain)
	email.Put("/domains/:domain_id", controllers.UpdateEmailDomain)
	email.Delete("/domains/:domain_id", controllers.DeleteEmailDomain)

	// Mailbox management
	email.Get("/mailboxes", controllers.GetMailboxes)
	email.Post("/mailboxes", controllers.AddMailbox)
	email.Put("/mailboxes/:id", controllers.UpdateMailbox)
	email.Put("/mailboxes/:id/password", controllers.UpdateMailboxPassword)
	email.Delete("/mailboxes/:mailbox_id", controllers.DeleteMailbox)

	// Email operations
	email.Get("/mailboxes/:mailbox_id/folders", controllers.GetFolders)
	email.Get("/mailboxes/:mailbox_id/emails", controllers.GetEmails)
	email.Post("/mailboxes/:mailbox_id/send", controllers.SendEmail)
}
