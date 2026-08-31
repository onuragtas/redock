package routes

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// Public routes are the ones served without a JWT, so the list is a security
// boundary: anything that appears here by accident is reachable by anyone who
// can open the port. Pinning the set means adding to it has to be deliberate.
func TestPublicRoutesAreOnlyTheExpectedOnes(t *testing.T) {
	app := fiber.New()
	PublicRoutes(app)

	want := []string{
		"GET /api/v1/auth/setup",
		"GET /api/v1/tunnel/auth/callback",
		"POST /api/v1/token/renew",
		"POST /api/v1/tunnel/auth/login",
		"POST /api/v1/tunnel/auth/register",
		"POST /api/v1/user/sign/in",
		"POST /api/v1/user/sign/up",
	}

	seen := make(map[string]bool)
	var got []string
	for _, route := range app.GetRoutes() {
		// Fiber registers a HEAD alongside every GET, and a catch-all for
		// unmatched paths; neither is a route this file declared.
		if route.Method == fiber.MethodHead || route.Path == "/" {
			continue
		}
		key := route.Method + " " + route.Path
		if seen[key] {
			continue
		}
		seen[key] = true
		got = append(got, key)
	}
	sort.Strings(got)

	if len(got) != len(want) {
		t.Fatalf("public routes changed:\n got: %v\nwant: %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("public route %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// A path nobody registered must not be answered by the public group.
func TestUnknownPublicPathIsNotFound(t *testing.T) {
	app := fiber.New()
	PublicRoutes(app)

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/nothing-here", http.NoBody), -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, fiber.StatusNotFound)
	}
}
