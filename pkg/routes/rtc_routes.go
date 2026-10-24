package routes

import (
	"redock/app/controllers"
	"redock/pkg/middleware"

	"github.com/gofiber/fiber/v2"
)

// RTCRoutes registers the WebRTC (embedded Galène SFU) endpoints. Changing
// the SFU opens ports and gateway routes on the host, so writes are admin-only.
func RTCRoutes(a *fiber.App) {
	route := a.Group("/api/v1/rtc", middleware.JWTProtected())
	route.Get("/status", controllers.RTCStatus)
	route.Get("/config", controllers.RTCGetConfig)
	route.Post("/config", middleware.AdminOnly(), controllers.RTCUpdateConfig)
	route.Post("/start", middleware.AdminOnly(), controllers.RTCStart)
	route.Post("/stop", middleware.AdminOnly(), controllers.RTCStop)

	route.Get("/rooms", controllers.RTCListRooms)
	route.Post("/rooms", middleware.AdminOnly(), controllers.RTCSaveRoom)
	route.Post("/rooms/delete", middleware.AdminOnly(), controllers.RTCDeleteRoom)
	route.Post("/rooms/invite", middleware.AdminOnly(), controllers.RTCInvite)
	route.Get("/rooms/:name/participants", middleware.AdminOnly(), controllers.RTCParticipants)
	route.Post("/rooms/kick", middleware.AdminOnly(), controllers.RTCKick)
	route.Post("/rooms/end", middleware.AdminOnly(), controllers.RTCEndSession)
	route.Post("/rooms/lock", middleware.AdminOnly(), controllers.RTCLock)
	route.Post("/rooms/clearchat", middleware.AdminOnly(), controllers.RTCClearChat)
	route.Post("/rooms/moderate", middleware.AdminOnly(), controllers.RTCModeratorJoin)
	route.Get("/audit", middleware.AdminOnly(), controllers.RTCAudit)
	route.Get("/recordings", middleware.AdminOnly(), controllers.RTCRecordings)
	route.Post("/rooms/record", middleware.AdminOnly(), controllers.RTCRecord)
	route.Post("/recordings/link", middleware.AdminOnly(), controllers.RTCRecordingLink)
	route.Post("/recordings/delete", middleware.AdminOnly(), controllers.RTCDeleteRecording)
	route.Get("/apikeys", middleware.AdminOnly(), controllers.RTCListAPIKeys)
	route.Post("/apikeys", middleware.AdminOnly(), controllers.RTCCreateAPIKey)
	route.Post("/apikeys/delete", middleware.AdminOnly(), controllers.RTCDeleteAPIKey)
}

// RTCExternalRoutes registers the API used by external platforms, which
// authenticate with an X-API-Key instead of a dashboard login. It must be
// registered before PrivateRoutes: that one installs JWT checking for the
// whole /api/v1 prefix, which would otherwise run first and reject these.
func RTCExternalRoutes(a *fiber.App) {
	auth := controllers.RTCAPIKeyAuth
	a.Post("/api/v1/rtc/ext/rooms", auth, controllers.RTCExtSaveRoom)
	a.Post("/api/v1/rtc/ext/tokens", auth, controllers.RTCExtToken)
	a.Get("/api/v1/rtc/ext/rooms/:name", auth, controllers.RTCExtRoom)

	// Signed, short-lived recording download links (minted by an admin).
	a.Get("/api/v1/rtc-recording", controllers.RTCDownloadRecording)
}
