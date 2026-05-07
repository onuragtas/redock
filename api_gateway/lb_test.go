package api_gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
)

func newGatewayWithUpstream(t *testing.T, up Upstream, services map[string]*Service) *Gateway {
	t.Helper()
	g := &Gateway{
		services:         services,
		upstreams:        map[string]*Upstream{up.ID: &up},
		upstreamRuntimes: make(map[string]*upstreamRuntime),
		serviceHealth:    make(map[string]*ServiceHealth),
		config: &GatewayConfig{
			Services:  []Service{},
			Upstreams: []Upstream{up},
		},
	}
	return g
}

func TestPickBackendRoundRobin(t *testing.T) {
	g := newGatewayWithUpstream(t, Upstream{
		ID: "up", Enabled: true, Strategy: StrategyRoundRobin,
		Targets: []UpstreamTarget{{ServiceID: "a"}, {ServiceID: "b"}, {ServiceID: "c"}},
	}, map[string]*Service{
		"a": {ID: "a", Enabled: true},
		"b": {ID: "b", Enabled: true},
		"c": {ID: "c", Enabled: true},
	})

	picks := map[string]int{}
	for i := 0; i < 9; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		svc, _, release, err := g.pickBackend(&Route{UpstreamID: "up"}, req)
		assert.NoError(t, err)
		picks[svc.ID]++
		release()
	}
	assert.Equal(t, 3, picks["a"])
	assert.Equal(t, 3, picks["b"])
	assert.Equal(t, 3, picks["c"])
}

func TestPickBackendSkipsUnhealthy(t *testing.T) {
	g := newGatewayWithUpstream(t, Upstream{
		ID: "up", Enabled: true, Strategy: StrategyRoundRobin,
		Targets: []UpstreamTarget{{ServiceID: "a"}, {ServiceID: "b"}},
	}, map[string]*Service{
		"a": {ID: "a", Enabled: true},
		"b": {ID: "b", Enabled: true},
	})
	g.serviceHealth["b"] = &ServiceHealth{Healthy: false}

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		svc, _, release, err := g.pickBackend(&Route{UpstreamID: "up"}, req)
		assert.NoError(t, err)
		assert.Equal(t, "a", svc.ID)
		release()
	}
}

func TestPickBackendAllUnhealthy(t *testing.T) {
	g := newGatewayWithUpstream(t, Upstream{
		ID: "up", Enabled: true, Strategy: StrategyRoundRobin,
		Targets: []UpstreamTarget{{ServiceID: "a"}},
	}, map[string]*Service{"a": {ID: "a", Enabled: true}})
	g.serviceHealth["a"] = &ServiceHealth{Healthy: false}

	req := httptest.NewRequest("GET", "/", nil)
	_, _, _, err := g.pickBackend(&Route{UpstreamID: "up"}, req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no healthy targets")
}

func TestPickBackendIPHashSticky(t *testing.T) {
	g := newGatewayWithUpstream(t, Upstream{
		ID: "up", Enabled: true, Strategy: StrategyRoundRobin,
		Sticky:  &StickyConfig{Mode: StickyIPHash},
		Targets: []UpstreamTarget{{ServiceID: "a"}, {ServiceID: "b"}, {ServiceID: "c"}},
	}, map[string]*Service{
		"a": {ID: "a", Enabled: true},
		"b": {ID: "b", Enabled: true},
		"c": {ID: "c", Enabled: true},
	})

	pick := func(ip string) string {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Forwarded-For", ip)
		svc, _, release, err := g.pickBackend(&Route{UpstreamID: "up"}, req)
		assert.NoError(t, err)
		release()
		return svc.ID
	}
	first := pick("10.0.0.42")
	for i := 0; i < 50; i++ {
		assert.Equal(t, first, pick("10.0.0.42"))
	}
	// Another IP can land anywhere — just confirm it's a valid backend.
	assert.Contains(t, []string{"a", "b", "c"}, pick("203.0.113.7"))
}

func TestPickBackendCookieStickyHonorsCookie(t *testing.T) {
	g := newGatewayWithUpstream(t, Upstream{
		ID: "up", Enabled: true, Strategy: StrategyRandom,
		Sticky:  &StickyConfig{Mode: StickyCookie, CookieName: "lb"},
		Targets: []UpstreamTarget{{ServiceID: "a"}, {ServiceID: "b"}},
	}, map[string]*Service{
		"a": {ID: "a", Enabled: true},
		"b": {ID: "b", Enabled: true},
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "lb", Value: "b"})
	svc, _, release, err := g.pickBackend(&Route{UpstreamID: "up"}, req)
	assert.NoError(t, err)
	assert.Equal(t, "b", svc.ID)
	release()
}

func TestPickBackendCookieStickyFallsBackWhenStale(t *testing.T) {
	g := newGatewayWithUpstream(t, Upstream{
		ID: "up", Enabled: true, Strategy: StrategyRoundRobin,
		Sticky:  &StickyConfig{Mode: StickyCookie, CookieName: "lb"},
		Targets: []UpstreamTarget{{ServiceID: "a"}},
	}, map[string]*Service{
		"a": {ID: "a", Enabled: true},
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "lb", Value: "ghost"}) // not in healthy set
	svc, _, release, err := g.pickBackend(&Route{UpstreamID: "up"}, req)
	assert.NoError(t, err)
	assert.Equal(t, "a", svc.ID)
	release()
}

func TestPickBackendWeightedDistribution(t *testing.T) {
	g := newGatewayWithUpstream(t, Upstream{
		ID: "up", Enabled: true, Strategy: StrategyWeighted,
		Targets: []UpstreamTarget{{ServiceID: "a", Weight: 1}, {ServiceID: "b", Weight: 9}},
	}, map[string]*Service{
		"a": {ID: "a", Enabled: true},
		"b": {ID: "b", Enabled: true},
	})

	picks := map[string]int{}
	for i := 0; i < 5000; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		svc, _, release, err := g.pickBackend(&Route{UpstreamID: "up"}, req)
		assert.NoError(t, err)
		picks[svc.ID]++
		release()
	}
	// b should win roughly 90% — give it a generous band [80%, 95%].
	bShare := float64(picks["b"]) / 5000
	assert.GreaterOrEqual(t, bShare, 0.80, "weight 9 backend share too low: %v", bShare)
	assert.LessOrEqual(t, bShare, 0.95, "weight 9 backend share too high: %v", bShare)
}

func TestPickBackendLeastConn(t *testing.T) {
	g := newGatewayWithUpstream(t, Upstream{
		ID: "up", Enabled: true, Strategy: StrategyLeastConn,
		Targets: []UpstreamTarget{{ServiceID: "a"}, {ServiceID: "b"}},
	}, map[string]*Service{
		"a": {ID: "a", Enabled: true},
		"b": {ID: "b", Enabled: true},
	})

	rt := g.runtime("up")
	// Pre-load a with two in-flight requests; b should win the next pick.
	atomic.AddInt64(rt.inflightFor("a"), 2)

	req := httptest.NewRequest("GET", "/", nil)
	svc, _, release, err := g.pickBackend(&Route{UpstreamID: "up"}, req)
	assert.NoError(t, err)
	assert.Equal(t, "b", svc.ID)
	release()
}

func TestMigrateConfigSharesUpstreamForRepeatedService(t *testing.T) {
	// Three routes pointing at the same service must share one auto-upstream.
	rawJSON := `{
		"services": [{"id":"svc","enabled":true}],
		"routes":   [
			{"id":"a","service_id":"svc"},
			{"id":"b","service_id":"svc"},
			{"id":"c","service_id":"svc"}
		]
	}`
	g := &Gateway{config: &GatewayConfig{}, upstreams: map[string]*Upstream{}, upstreamRuntimes: map[string]*upstreamRuntime{}}
	if err := json.Unmarshal([]byte(rawJSON), g.config); err != nil {
		t.Fatal(err)
	}
	assert.True(t, g.migrateConfig(rawJSON))
	assert.Len(t, g.config.Upstreams, 1)
	for _, r := range g.config.Routes {
		assert.Equal(t, "auto-svc", r.UpstreamID)
	}
}

func TestMigrateConfigOrphanRouteStaysEmpty(t *testing.T) {
	// Route with no service_id stays orphaned (UpstreamID empty) — same broken
	// state it was already in pre-migration. No upstream is fabricated for it.
	rawJSON := `{"services":[{"id":"svc","enabled":true}],"routes":[{"id":"orphan"},{"id":"good","service_id":"svc"}]}`
	g := &Gateway{config: &GatewayConfig{}, upstreams: map[string]*Upstream{}, upstreamRuntimes: map[string]*upstreamRuntime{}}
	if err := json.Unmarshal([]byte(rawJSON), g.config); err != nil {
		t.Fatal(err)
	}
	assert.True(t, g.migrateConfig(rawJSON))
	for _, r := range g.config.Routes {
		switch r.ID {
		case "orphan":
			assert.Equal(t, "", r.UpstreamID, "orphan route must not get a fabricated upstream")
		case "good":
			assert.Equal(t, "auto-svc", r.UpstreamID)
		}
	}
}

func TestMigrateConfigMixedPreMigratedRoutes(t *testing.T) {
	// Routes already on v2 (UpstreamID set) must be left alone.
	rawJSON := `{
		"config_version": 0,
		"upstreams":[{"id":"manual","strategy":"weighted","enabled":true,"targets":[{"service_id":"svc","weight":1}]}],
		"services":[{"id":"svc","enabled":true}],
		"routes":[
			{"id":"old","service_id":"svc"},
			{"id":"new","upstream_id":"manual"}
		]
	}`
	g := &Gateway{config: &GatewayConfig{}, upstreams: map[string]*Upstream{}, upstreamRuntimes: map[string]*upstreamRuntime{}}
	if err := json.Unmarshal([]byte(rawJSON), g.config); err != nil {
		t.Fatal(err)
	}
	assert.True(t, g.migrateConfig(rawJSON))

	// "old" gets a freshly created auto-svc upstream; "new" keeps "manual".
	for _, r := range g.config.Routes {
		switch r.ID {
		case "old":
			assert.Equal(t, "auto-svc", r.UpstreamID)
		case "new":
			assert.Equal(t, "manual", r.UpstreamID)
		}
	}
	// We should now have 2 upstreams: the original "manual" plus "auto-svc".
	ids := []string{}
	for _, u := range g.config.Upstreams {
		ids = append(ids, u.ID)
	}
	assert.ElementsMatch(t, []string{"manual", "auto-svc"}, ids)
}

func TestMigrateConfigParseFailureDoesNotBumpVersion(t *testing.T) {
	// Malformed JSON: re-parse fails. We MUST NOT bump ConfigVersion, otherwise
	// a subsequent SaveConfig overwrites the original blob and loses the
	// route→service binding for good.
	g := &Gateway{config: &GatewayConfig{ConfigVersion: 0}, upstreams: map[string]*Upstream{}, upstreamRuntimes: map[string]*upstreamRuntime{}}
	assert.False(t, g.migrateConfig("{not valid json"))
	assert.Equal(t, 0, g.config.ConfigVersion)
}

func TestMigrateConfigEmptyConfigBumpsVersion(t *testing.T) {
	// A brand-new install (no routes, no services) is trivially current.
	rawJSON := `{}`
	g := &Gateway{config: &GatewayConfig{}, upstreams: map[string]*Upstream{}, upstreamRuntimes: map[string]*upstreamRuntime{}}
	if err := json.Unmarshal([]byte(rawJSON), g.config); err != nil {
		t.Fatal(err)
	}
	assert.True(t, g.migrateConfig(rawJSON))
	assert.Equal(t, currentConfigVersion, g.config.ConfigVersion)
	assert.Len(t, g.config.Upstreams, 0)
	assert.Len(t, g.config.Routes, 0)
}

func TestMigrateConfigBuildsAutoUpstreams(t *testing.T) {
	rawJSON := `{
		"services": [{"id":"svc1","enabled":true},{"id":"svc2","enabled":true}],
		"routes":   [{"id":"r1","service_id":"svc1"},{"id":"r2","service_id":"svc1"},{"id":"r3","service_id":"svc2"}]
	}`

	g := &Gateway{
		config:           &GatewayConfig{},
		upstreams:        make(map[string]*Upstream),
		upstreamRuntimes: make(map[string]*upstreamRuntime),
	}
	if err := json.Unmarshal([]byte(rawJSON), g.config); err != nil {
		t.Fatal(err)
	}
	assert.True(t, g.migrateConfig(rawJSON))

	assert.Equal(t, currentConfigVersion, g.config.ConfigVersion)
	// Two unique service IDs → two auto-upstreams.
	assert.Len(t, g.config.Upstreams, 2)
	for _, r := range g.config.Routes {
		switch r.ID {
		case "r1", "r2":
			assert.Equal(t, "auto-svc1", r.UpstreamID)
		case "r3":
			assert.Equal(t, "auto-svc2", r.UpstreamID)
		}
	}
	// Idempotent: running again on already-current config does nothing.
	assert.False(t, g.migrateConfig(rawJSON))
}

