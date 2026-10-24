package routes

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// External RTC endpoints authenticate with X-API-Key. They must be reached
// before PrivateRoutes' /api/v1-wide JWT middleware, or every platform call
// would be rejected for lacking a dashboard login.
func TestRTCExternalRoutesBypassJWT(t *testing.T) {
	app := fiber.New()
	RTCExternalRoutes(app)
	// Stand-in for PrivateRoutes: a JWT check installed on the whole /api/v1
	// prefix (the real one needs JWT secrets from the memory DB).
	app.Use("/api/v1", func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusUnauthorized).SendString("missing or malformed JWT")
	})

	req := httptest.NewRequest("POST", "/api/v1/rtc/ext/tokens", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	// Our handler answers (401 for the key, or 503 when the RTC manager isn't
	// initialised as here) — never the JWT middleware.
	if strings.Contains(string(body), "JWT") || !strings.Contains(string(body), `"error":true`) {
		t.Fatalf("got %d %s; want the API-key handler, not the JWT middleware", resp.StatusCode, body)
	}
}
