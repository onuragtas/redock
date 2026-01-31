package utils

import (
	"log"
	"os"
	"os/signal"

	"github.com/gofiber/fiber/v2"
)

// ShutdownCallback is called when shutdown is requested (tüm portları kapatır + DB flush).
var ShutdownCallback func()

// RequestGracefulShutdown is set by StartServerWithGracefulShutdown. Kod içinden shutdown tetiklemek için (örn. update).
// callback verilirse shutdown bittikten sonra çalışır (örn. UpdateWithRestart).
var RequestGracefulShutdown func(callback func())

// StartServerWithGracefulShutdown function for starting server with a graceful shutdown.
func StartServerWithGracefulShutdown(a *fiber.App) {
	idleConnsClosed := make(chan struct{})
	shutdownRequest := make(chan func(), 1)

	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)

		var afterShutdown func()
		select {
		case <-sigint:
			// SIGINT (Ctrl+C)
		case afterShutdown = <-shutdownRequest:
			// Kod içinden (örn. update)
		}

		// 1) Tüm portları kapat + DB flush
		if ShutdownCallback != nil {
			ShutdownCallback()
		}

		// 2) Fiber shutdown
		if err := a.Shutdown(); err != nil {
			log.Printf("Oops... Server is not shutting down! Reason: %v", err)
		}

		close(idleConnsClosed)

		if afterShutdown != nil {
			afterShutdown()
		}
	}()

	RequestGracefulShutdown = func(callback func()) {
		shutdownRequest <- callback
	}

	fiberConnURL, _ := ConnectionURLBuilder("fiber")
	if err := a.Listen(fiberConnURL); err != nil {
		log.Printf("Oops... Server is not running! Reason: %v", err)
	}

	<-idleConnsClosed
}

// StartServer func for starting a simple server.
func StartServer(a *fiber.App) {
	// Build Fiber connection URL.
	fiberConnURL, _ := ConnectionURLBuilder("fiber")

	// Run server.
	if err := a.Listen(fiberConnURL); err != nil {
		log.Printf("Oops... Server is not running! Reason: %v", err)
	}
}
