package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
)

// The embedded UI is served with no modification time, so unless we say
// otherwise the browser is left to guess how long it may reuse index.html —
// and a guess that goes the wrong way keeps a stale dashboard on screen after
// an upgrade, pointing at asset names the new binary no longer serves.
func TestStaticCacheHeaders(t *testing.T) {
	app := fiber.New()
	app.Use("/", staticCacheHeaders, filesystem.New(filesystem.Config{
		Root:       http.FS(embedDirStatic),
		PathPrefix: "web/dist",
	}))

	assets, err := embedDirStatic.ReadDir("web/dist/assets")
	if err != nil {
		t.Fatalf("embedded assets missing: %v", err)
	}
	var hashedAsset string
	for _, entry := range assets {
		if !entry.IsDir() {
			hashedAsset = "/assets/" + entry.Name()
			break
		}
	}
	if hashedAsset == "" {
		t.Fatal("no embedded asset to test against")
	}

	tests := []struct {
		name string
		path string
		want string
	}{
		// index.html names the current asset hashes, so it has to be rechecked
		// on every load or a new release never reaches an open tab.
		{"entry point is revalidated", "/", "no-cache"},
		{"index.html is revalidated", "/index.html", "no-cache"},
		// A hashed name can only ever mean one thing, so it is safe to keep.
		{"hashed asset is immutable", hashedAsset, "public, max-age=31536000, immutable"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, tc.path, nil))
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != fiber.StatusOK {
				t.Fatalf("%s: status = %d, want 200", tc.path, resp.StatusCode)
			}
			if got := resp.Header.Get(fiber.HeaderCacheControl); got != tc.want {
				t.Errorf("%s: Cache-Control = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}
