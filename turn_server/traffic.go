package turn_server

import (
	"sync"
	"sync/atomic"
	"time"

	"redock/platform/memory"
)

const TableTURNStats = "turn_stats"

// TURNStatsEntity keeps relayed traffic across restarts (single row).
type TURNStatsEntity struct {
	memory.BaseEntity
	LifetimeIn  int64 `json:"lifetime_in"`  // bytes from peers to clients (clients' download)
	LifetimeOut int64 `json:"lifetime_out"` // bytes from clients to peers (clients' upload)
}

const (
	trafficSampleEvery  = 2 * time.Second
	trafficPersistEvery = 30 * time.Second
)

// trafficStats accounts relayed bytes beyond the life of a single session:
// bytes of closed sessions are kept, a sampler turns totals into a rate,
// and lifetime totals are persisted. The zero value is ready to use.
type trafficStats struct {
	endedIn, endedOut atomic.Int64 // closed sessions since this start
	rateIn, rateOut   atomic.Int64 // bytes per second, last sample

	mu      sync.Mutex
	baseIn  int64 // lifetime before this start (from the DB)
	baseOut int64
	stop    chan struct{}
}

// Traffic is the relayed traffic shown in the dashboard. "In" is what
// clients receive from their peers (download), "Out" what they send (upload).
type Traffic struct {
	RateIn      int64 `json:"rate_in"`  // bytes/s
	RateOut     int64 `json:"rate_out"` // bytes/s
	LiveIn      int64 `json:"live_in"`  // open sessions
	LiveOut     int64 `json:"live_out"`
	TotalIn     int64 `json:"total_in"` // since the server started
	TotalOut    int64 `json:"total_out"`
	LifetimeIn  int64 `json:"lifetime_in"` // all time, persisted
	LifetimeOut int64 `json:"lifetime_out"`
}

func (m *Manager) loadLifetime() (in, out int64) {
	d := db()
	if d == nil {
		return 0, 0
	}
	if rows := memory.FindAll[*TURNStatsEntity](d, TableTURNStats); len(rows) > 0 {
		return rows[0].LifetimeIn, rows[0].LifetimeOut
	}
	return 0, 0
}

func (m *Manager) saveLifetime(in, out int64) {
	d := db()
	if d == nil {
		return
	}
	if rows := memory.FindAll[*TURNStatsEntity](d, TableTURNStats); len(rows) > 0 {
		rows[0].LifetimeIn, rows[0].LifetimeOut = in, out
		_ = memory.Update(d, TableTURNStats, rows[0])
		return
	}
	_ = memory.Create(d, TableTURNStats, &TURNStatsEntity{LifetimeIn: in, LifetimeOut: out})
}

// sinceStart is closed + open sessions' bytes since the server started.
func (m *Manager) sinceStart() (in, out int64) {
	liveIn, liveOut := m.registry.totals()
	return m.traffic.endedIn.Load() + liveIn, m.traffic.endedOut.Load() + liveOut
}

// startTraffic resets the per-start counters and runs the sampler.
func (m *Manager) startTraffic() {
	t := &m.traffic
	t.endedIn.Store(0)
	t.endedOut.Store(0)
	t.rateIn.Store(0)
	t.rateOut.Store(0)
	in, out := m.loadLifetime()
	t.mu.Lock()
	t.baseIn, t.baseOut = in, out
	t.stop = make(chan struct{})
	stop := t.stop
	t.mu.Unlock()
	go m.sampleTraffic(stop)
}

// stopTraffic persists the lifetime totals (including still-open sessions)
// and stops the sampler. Call before sessions are torn down.
func (m *Manager) stopTraffic() {
	t := &m.traffic
	in, out := m.sinceStart()
	t.mu.Lock()
	if t.stop != nil {
		close(t.stop)
		t.stop = nil
	}
	baseIn, baseOut := t.baseIn, t.baseOut
	t.mu.Unlock()
	m.saveLifetime(baseIn+in, baseOut+out)
	t.rateIn.Store(0)
	t.rateOut.Store(0)
}

func (m *Manager) sampleTraffic(stop chan struct{}) {
	tick := time.NewTicker(trafficSampleEvery)
	defer tick.Stop()
	prevIn, prevOut := m.sinceStart()
	prevAt := time.Now()
	lastSave := time.Now()
	for {
		select {
		case <-stop:
			return
		case now := <-tick.C:
			in, out := m.sinceStart()
			if dt := now.Sub(prevAt).Seconds(); dt > 0 {
				// A session closing between samples can't make totals shrink:
				// its bytes move from live to ended. Clamp just in case.
				m.traffic.rateIn.Store(max(0, int64(float64(in-prevIn)/dt)))
				m.traffic.rateOut.Store(max(0, int64(float64(out-prevOut)/dt)))
			}
			prevIn, prevOut, prevAt = in, out, now
			if now.Sub(lastSave) >= trafficPersistEvery {
				m.traffic.mu.Lock()
				baseIn, baseOut := m.traffic.baseIn, m.traffic.baseOut
				m.traffic.mu.Unlock()
				m.saveLifetime(baseIn+in, baseOut+out)
				lastSave = now
			}
		}
	}
}

// Traffic reports rates and totals.
func (m *Manager) Traffic() Traffic {
	liveIn, liveOut := m.registry.totals()
	in, out := m.sinceStart()
	m.traffic.mu.Lock()
	baseIn, baseOut := m.traffic.baseIn, m.traffic.baseOut
	running := m.traffic.stop != nil
	m.traffic.mu.Unlock()
	if !running {
		// Stopped: everything is already folded into the persisted base.
		lifeIn, lifeOut := m.loadLifetime()
		return Traffic{LifetimeIn: lifeIn, LifetimeOut: lifeOut}
	}
	return Traffic{
		RateIn: m.traffic.rateIn.Load(), RateOut: m.traffic.rateOut.Load(),
		LiveIn: liveIn, LiveOut: liveOut,
		TotalIn: in, TotalOut: out,
		LifetimeIn: baseIn + in, LifetimeOut: baseOut + out,
	}
}
