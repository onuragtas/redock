package routes

import (
	"strings"
	"testing"

	"redock/ssh_server"

	"github.com/gofiber/fiber/v2"
)

// While the web terminal kill switch is off, only the traffic stream may be
// served under /ws.
func TestWebTerminalOffWhenDisabled(t *testing.T) {
	if ssh_server.WebTerminalEnabled {
		t.Skip("web terminal enabled")
	}
	app := fiber.New()
	WebSocketRoutes(app)
	for _, r := range app.GetRoutes() {
		if strings.HasPrefix(r.Path, "/ws") && r.Path != "/ws/traffic" {
			t.Errorf("web terminal route registered while disabled: %s %s", r.Method, r.Path)
		}
	}
}
