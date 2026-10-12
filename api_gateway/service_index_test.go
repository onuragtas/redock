package api_gateway

import (
	"testing"
	"time"
)

// Deleting a service shifts g.config.Services in place; g.services must be
// rebuilt so every ID still points at its own entry.
func TestDeleteServiceKeepsIndexConsistent(t *testing.T) {
	g := &Gateway{
		serviceHealth:   make(map[string]*ServiceHealth),
		lastHealthCheck: make(map[string]time.Time),
		config: &GatewayConfig{Services: []Service{
			{ID: "a", HealthCheck: &HealthCheck{Path: "/a"}},
			{ID: "b"},
			{ID: "c", HealthCheck: &HealthCheck{Path: "/c"}},
		}},
	}
	// Same layout as a config loaded at startup: all pointers into one backing array.
	g.services = make(map[string]*Service)
	for i := range g.config.Services {
		g.services[g.config.Services[i].ID] = &g.config.Services[i]
	}

	if err := g.DeleteService("a"); err != nil {
		t.Fatal(err)
	}

	if len(g.services) != 2 {
		t.Fatalf("services len = %d, want 2", len(g.services))
	}
	for id, svc := range g.services {
		if svc.ID != id {
			t.Fatalf("g.services[%q] points at service %q", id, svc.ID)
		}
	}
	if g.services["c"].HealthCheck == nil || g.services["c"].HealthCheck.Path != "/c" {
		t.Fatalf("service c lost its health check: %+v", g.services["c"].HealthCheck)
	}
}
