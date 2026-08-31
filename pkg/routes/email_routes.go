package routes

import (
	"redock/app/controllers"
	"redock/pkg/middleware"

	"github.com/gofiber/fiber/v2"
)

// EmailRoutes sets up email server routes (JWT protected)
func EmailRoutes(app *fiber.App) {
	email := app.Group("/api/email", middleware.JWTProtected())

	// Server management
	email.Get("/server/status", controllers.GetEmailServerStatus)
	email.Put("/server/ip", controllers.UpdateServerIP)
	email.Get("/server/check-passwords", controllers.CheckMailboxPasswords)

	// Mail traffic logs (incoming / outgoing / rejected)
	email.Get("/logs", controllers.GetEmailLogs)
	email.Get("/logs/raw", controllers.GetEmailRawLogs)
	email.Get("/logs/connections", controllers.GetEmailConnections)

	// Native engine: mode switch, listener settings, outbound queue, DNS help
	email.Get("/engine", controllers.GetEmailEngine)
	email.Post("/control", controllers.ControlEmailServer)
	email.Put("/native/settings", controllers.UpdateEmailNativeSettings)
	email.Get("/queue", controllers.GetEmailQueue)
	email.Post("/queue/flush", controllers.FlushEmailQueue)
	email.Delete("/queue/:id", controllers.DeleteEmailQueueItem)
	email.Get("/certificate", controllers.GetEmailCertificate)
	email.Post("/certificate/request", controllers.RequestEmailCertificate)
	email.Get("/deliverability", controllers.CheckEmailDeliverability)
	email.Get("/dns-records", controllers.GetEmailDNSRecords)
	email.Get("/dns-records/preview", controllers.PreviewEmailDNS)
	email.Post("/dns-records/sync", controllers.SyncEmailDNS)
	email.Get("/legacy", controllers.GetEmailLegacyArtifacts)
	email.Delete("/legacy", controllers.CleanupEmailLegacyArtifacts)

	// Domain management
	email.Get("/domains", controllers.GetEmailDomains)
	email.Post("/domains", controllers.AddEmailDomain)
	email.Put("/domains/:domain_id", controllers.UpdateEmailDomain)
	email.Delete("/domains/:domain_id", controllers.DeleteEmailDomain)

	// Aliases
	email.Get("/aliases", controllers.GetEmailAliases)
	email.Post("/aliases", controllers.AddEmailAlias)
	email.Put("/aliases/:id", controllers.UpdateEmailAlias)
	email.Delete("/aliases/:id", controllers.DeleteEmailAlias)

	// Abuse protection
	email.Get("/blocked", controllers.GetEmailBlockedClients)
	email.Post("/blocked", controllers.BlockEmailClient)
	email.Delete("/blocked/:ip", controllers.UnblockEmailClient)

	// Mailbox management
	email.Get("/mailboxes", controllers.GetMailboxes)
	email.Post("/mailboxes", controllers.AddMailbox)
	email.Put("/mailboxes/:id", controllers.UpdateMailbox)
	email.Put("/mailboxes/:id/password", controllers.UpdateMailboxPassword)
	email.Delete("/mailboxes/:mailbox_id", controllers.DeleteMailbox)

	// Email operations
	email.Get("/mailboxes/:mailbox_id/folders", controllers.GetFolders)
	email.Get("/mailboxes/:mailbox_id/emails", controllers.GetEmails)
	email.Get("/mailboxes/:mailbox_id/thread", controllers.GetThread)
	email.Post("/mailboxes/:mailbox_id/send", controllers.SendEmail)
	email.Post("/mailboxes/:mailbox_id/drafts", controllers.SaveDraft)

	// Mailbox filters
	email.Get("/mailboxes/:mailbox_id/filters", controllers.GetEmailFilters)
	email.Post("/mailboxes/:mailbox_id/filters", controllers.AddEmailFilter)
	email.Put("/filters/:id", controllers.UpdateEmailFilter)
	email.Delete("/filters/:id", controllers.DeleteEmailFilter)

	// Message actions
	email.Put("/mailboxes/:mailbox_id/messages/:uid/flag", controllers.SetMessageFlag)
	email.Post("/mailboxes/:mailbox_id/messages/:uid/move", controllers.MoveMessage)
	email.Delete("/mailboxes/:mailbox_id/messages/:uid", controllers.DeleteMessage)
	email.Get("/mailboxes/:mailbox_id/messages/:uid/attachments", controllers.ListMessageAttachments)
	email.Get("/mailboxes/:mailbox_id/messages/:uid/attachments/:index", controllers.DownloadMessageAttachment)
	email.Get("/mailboxes/:mailbox_id/messages/:uid/raw", controllers.DownloadRawMessage)
}
