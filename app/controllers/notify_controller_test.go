package controllers

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// The page reads its settings from here on every visit. Before a notifier
// exists — and on an install where the in-memory database has not been set up
// in this process — it still has to answer with usable settings rather than
// fail, or the page renders nothing at all.
func TestNotifySettingsAnswerBeforeAnythingIsConfigured(t *testing.T) {
	app := fiber.New()
	app.Get("/notifications", GetNotifySettings)

	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/notifications", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}

	body, _ := io.ReadAll(resp.Body)
	var payload struct {
		Error bool `json:"error"`
		Data  struct {
			Settings map[string]any `json:"settings"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("the answer is not JSON: %s", body)
	}
	if payload.Error {
		t.Fatalf("the endpoint reported an error: %s", body)
	}
	if payload.Data.Settings == nil {
		t.Fatal("no settings were returned, so the page would render nothing")
	}

	// The fields the page binds to have to be present, or its inputs bind to
	// undefined and silently do nothing.
	for _, field := range []string{
		"enabled", "mailbox_id", "recipient",
		"watch_certificate", "watch_queue", "watch_memory", "watch_blocked",
		"cert_days_before", "queue_threshold", "blocked_threshold", "repeat_hours",
	} {
		if _, ok := payload.Data.Settings[field]; !ok {
			t.Errorf("the settings are missing %q, which the page binds to", field)
		}
	}
}

// Sending a test before anything is configured must be refused with a reason,
// not a panic.
func TestNotifyTestIsRefusedBeforeSetup(t *testing.T) {
	app := fiber.New()
	app.Post("/test", SendNotifyTest)

	resp, err := app.Test(httptest.NewRequest(fiber.MethodPost, "/test", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == fiber.StatusOK {
		t.Error("a test send was accepted with nothing configured")
	}
}
