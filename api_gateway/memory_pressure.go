package api_gateway

import (
	"fmt"
	"sort"
	"time"

	"redock/platform/memguard"
)

// degradedClientStats is how many per-IP trackers survive a relief sweep; the
// busiest clients are kept so auto-blocking keeps working.
const degradedClientStats = 200

// registerMemoryRelievers hooks the gateway's caches into the memory guard.
// Everything released here is a cache or a metric — request handling itself is
// never dropped.
func registerMemoryRelievers() {
	memguard.RegisterReliever(memguard.Reliever{
		Name:        "gateway-route-cache",
		Description: "Clears the route match cache (matching falls back to full lookup)",
		MinLevel:    memguard.LevelWarning,
		Priority:    15,
		Release: func(memguard.Level) (int64, string) {
			g := GetGateway()
			if g == nil {
				return 0, ""
			}
			g.routeCacheMu.RLock()
			n := len(g.routeCache)
			g.routeCacheMu.RUnlock()
			if n == 0 {
				return 0, ""
			}
			g.clearRouteCache()
			return int64(n) * 256, fmt.Sprintf("%d route cache entries cleared", n)
		},
	})

	memguard.RegisterReliever(memguard.Reliever{
		Name:        "gateway-client-stats",
		Description: "Trims per-client-IP statistics to the busiest clients",
		MinLevel:    memguard.LevelCritical,
		Priority:    35,
		Release: func(memguard.Level) (int64, string) {
			g := GetGateway()
			if g == nil {
				return 0, ""
			}
			dropped := g.trimClientStats(degradedClientStats)
			if dropped == 0 {
				return 0, ""
			}
			return int64(dropped) * 200, fmt.Sprintf("%d client stat trackers dropped", dropped)
		},
	})

	memguard.RegisterReliever(memguard.Reliever{
		Name:        "gateway-telemetry-buffer",
		Description: "Flushes the pending observability export buffer",
		MinLevel:    memguard.LevelWarning,
		Priority:    25,
		Release: func(memguard.Level) (int64, string) {
			e := GetTelemetryExporter()
			if e == nil {
				return 0, ""
			}
			n := e.pendingCount()
			if n == 0 {
				return 0, ""
			}
			e.flush()
			return int64(n) * 512, fmt.Sprintf("%d buffered telemetry records flushed", n)
		},
	})
}

// trimClientStats keeps the `keep` busiest client trackers and drops the rest.
// Returns how many were dropped.
func (g *Gateway) trimClientStats(keep int) int {
	g.clientStatsMu.Lock()
	defer g.clientStatsMu.Unlock()

	if len(g.clientStats) <= keep {
		return 0
	}

	type entry struct {
		ip       string
		requests int64
	}
	entries := make([]entry, 0, len(g.clientStats))
	for ip, tracker := range g.clientStats {
		entries = append(entries, entry{ip: ip, requests: tracker.requests})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].requests > entries[j].requests })

	now := time.Now()
	dropped := 0
	for _, e := range entries[keep:] {
		// Never drop a client that is currently blocked — that state must
		// outlive a memory sweep.
		if tracker, ok := g.clientStats[e.ip]; ok && tracker.blockedUntil.After(now) {
			continue
		}
		delete(g.clientStats, e.ip)
		dropped++
	}
	return dropped
}

// pendingCount reports how many records are waiting in the export buffer.
func (e *TelemetryExporter) pendingCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.buffer)
}
