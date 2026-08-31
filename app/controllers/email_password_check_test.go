package controllers

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// Without a running mail server the endpoint still has to answer rather than
// hang or panic, because the dashboard calls it on every visit to Mailboxes.
func TestCheckMailboxPasswordsAnswersWithoutAServer(t *testing.T) {
	app := fiber.New()
	app.Get("/check", CheckMailboxPasswords)

	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/check", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusServiceUnavailable)
	}

	body, _ := io.ReadAll(resp.Body)
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("the answer is not JSON: %s", body)
	}
	if payload["error"] != true {
		t.Errorf("expected an error flag, got: %s", body)
	}
}
