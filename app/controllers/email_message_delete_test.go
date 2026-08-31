package controllers

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// A delete request that does not name a folder must be refused rather than
// answered from a default. UIDs are per-folder, so guessing means deleting a
// message the caller never asked about — which is exactly what happened when
// the dashboard's HTTP layer dropped the folder from the query string.
func TestDeleteMessageRequiresAFolder(t *testing.T) {
	app := fiber.New()
	app.Delete("/mailboxes/:mailbox_id/messages/:uid", DeleteMessage)

	tests := []struct {
		name string
		url  string
	}{
		{"no folder at all", "/mailboxes/1/messages/1"},
		{"an empty folder", "/mailboxes/1/messages/1?folder="},
		{"only whitespace", "/mailboxes/1/messages/1?folder=%20"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := app.Test(httptest.NewRequest(fiber.MethodDelete, tc.url, nil))
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != fiber.StatusBadRequest {
				t.Errorf("status = %d, want %d", resp.StatusCode, fiber.StatusBadRequest)
			}

			// Check the reason, not just the status: without a running mail
			// server the handler also refuses for its own reasons, and a test
			// that accepts any 400 would pass while the guard was gone.
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if !strings.Contains(string(body), "folder is required") {
				t.Errorf("refused for the wrong reason: %s", body)
			}
		})
	}
}
