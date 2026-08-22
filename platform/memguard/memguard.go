// Package memguard keeps the process inside a memory budget instead of letting
// the OS kill it. It watches RSS/heap against a budget (cgroup limit or a share
// of physical RAM), wires that budget into the Go runtime as GOMEMLIMIT, and —
// as pressure rises — asks registered subsystems to give memory back: drop
// caches, shrink ring buffers, stop capturing payloads. Features degrade; the
// process stays up.
package memguard

import (
	"fmt"
	"log"
	"runtime"
	"runtime/debug"
	"sort"
	"sync"
	"time"

	"redock/platform/memory"
)

const (
	historySize = 720 // 1 hour at the default 5s interval
	eventsSize  = 200
	// hysteresisPercent is how far below a threshold usage must fall before the
	// guard steps back down a level, so it does not flap on the boundary.
	hysteresisPercent = 5
	// stepDownSamples is how many consecutive calm samples are required before
	// the level actually drops.
	stepDownSamples = 3
)

// Reliever is a subsystem's hook for giving memory back under pressure.
type Reliever struct {
	// Name identifies the reliever in the dashboard and logs.
	Name string
	// Description is a human-readable note about what gets sacrificed.
	Description string
	// MinLevel is the pressure level at which this reliever starts running.
	MinLevel Level
	// Priority orders relievers within a sweep; lower runs first. Cheap,
	// harmless releases should have lower numbers than destructive ones.
	Priority int
	// Release frees memory and reports roughly how many bytes went away plus a
	// short note for the event log. Must not block for long.
	Release func(level Level) (freedBytes int64, note string)
	// Restore is called once when pressure returns to normal, so a subsystem can
	// re-enable whatever it switched off. Optional.
	Restore func()
}

// Sample is one memory reading kept for the dashboard chart.
type Sample struct {
	Timestamp   int64   `json:"timestamp"`
	UsedBytes   int64   `json:"used_bytes"`
	RSSBytes    int64   `json:"rss_bytes"`
	HeapBytes   int64   `json:"heap_bytes"`
	GoSysBytes  int64   `json:"go_sys_bytes"`
	LimitBytes  int64   `json:"limit_bytes"`
	UsedPercent float64 `json:"used_percent"`
	Goroutines  int     `json:"goroutines"`
	NumGC       uint32  `json:"num_gc"`
	Level       Level   `json:"level"`
}

// Event records something the guard did, for the dashboard activity list.
type Event struct {
	Timestamp  int64  `json:"timestamp"`
	Level      Level  `json:"level"`
	Action     string `json:"action"`
	Detail     string `json:"detail"`
	UsedBytes  int64  `json:"used_bytes"`
	FreedBytes int64  `json:"freed_bytes"`
}

// ReliefResult reports what a single reliever did during a sweep.
type ReliefResult struct {
	Name       string `json:"name"`
	FreedBytes int64  `json:"freed_bytes"`
	Note       string `json:"note"`
	Error      string `json:"error,omitempty"`
}

// RelieverInfo describes a registered reliever for the API.
type RelieverInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	MinLevel    Level  `json:"min_level"`
	Priority    int    `json:"priority"`
	Runs        int64  `json:"runs"`
	FreedBytes  int64  `json:"freed_bytes"`
	LastRun     int64  `json:"last_run,omitempty"`
}

// Status is the full picture handed to the dashboard.
type Status struct {
	Enabled       bool           `json:"enabled"`
	Running       bool           `json:"running"`
	Level         Level          `json:"level"`
	LimitBytes    int64          `json:"limit_bytes"`
	LimitSource   string         `json:"limit_source"`
	SystemBytes   int64          `json:"system_bytes"`
	UsedBytes     int64          `json:"used_bytes"`
	UsedPercent   float64        `json:"used_percent"`
	RSSBytes      int64          `json:"rss_bytes"`
	HeapBytes     int64          `json:"heap_bytes"`
	HeapObjects   uint64         `json:"heap_objects"`
	GoSysBytes    int64          `json:"go_sys_bytes"`
	StackBytes    int64          `json:"stack_bytes"`
	Goroutines    int            `json:"goroutines"`
	NumGC         uint32         `json:"num_gc"`
	LastGC        int64          `json:"last_gc,omitempty"`
	GCPercent     int            `json:"gc_percent"`
	GoMemLimit    int64          `json:"go_mem_limit"`
	OOMProtection string         `json:"oom_protection"`
	Thresholds    Thresholds     `json:"thresholds"`
	Config        ConfigEntity   `json:"config"`
	Relievers     []RelieverInfo `json:"relievers"`
	LevelSince    int64          `json:"level_since"`
	ReliefCount   int64          `json:"relief_count"`
	TotalFreed    int64          `json:"total_freed_bytes"`
}

// Thresholds is the resolved byte value of each pressure level.
type Thresholds struct {
	WarningBytes   int64 `json:"warning_bytes"`
	CriticalBytes  int64 `json:"critical_bytes"`
	EmergencyBytes int64 `json:"emergency_bytes"`
}

// Guard is the memory supervisor. Use Get to reach the singleton.
type Guard struct {
	mu  sync.RWMutex
	db  *memory.Database
	cfg ConfigEntity

	limitBytes  int64
	limitSource string
	systemBytes int64

	level      Level
	levelSince time.Time
	calmStreak int

	relievers []*registeredReliever
	degraded  bool

	history    []Sample
	historyPos int
	historyLen int

	events    []Event
	eventsPos int
	eventsLen int

	lastRelief  time.Time
	reliefCount int64
	totalFreed  int64

	gcPercent     int
	oomProtection string

	running bool
	stop    chan struct{}
	wg      sync.WaitGroup
}

type registeredReliever struct {
	Reliever
	runs    int64
	freed   int64
	lastRun time.Time
}

var (
	guard     *Guard
	guardOnce sync.Once
	// currentLevel is read on hot paths (per request / per packet) so it is kept
	// outside the mutex.
	currentLevel atomicLevel
)

// Get returns the guard singleton, creating it with defaults if needed.
func Get() *Guard {
	guardOnce.Do(func() {
		guard = &Guard{
			cfg:       DefaultConfig(),
			history:   make([]Sample, historySize),
			events:    make([]Event, eventsSize),
			gcPercent: 100,
		}
		guard.systemBytes, guard.limitSource = detectSystemMemory()
		guard.limitBytes = guard.resolveLimitLocked()
	})
	return guard
}

// Init loads the persisted configuration, applies the runtime limits and starts
// the monitor loop. Safe to call once at boot; later calls are no-ops.
func Init(db *memory.Database) *Guard {
	g := Get()

	g.mu.Lock()
	g.db = db
	if db != nil {
		if rows := memory.FindAll[*ConfigEntity](db, TableName); len(rows) > 0 {
			g.cfg = *rows[0]
		} else {
			g.cfg = DefaultConfig()
			row := g.cfg
			if err := memory.Create(db, TableName, &row); err != nil {
				log.Printf("memguard: failed to persist default config: %v", err)
			} else {
				g.cfg.ID = row.ID
			}
		}
	}
	g.cfg.Normalize()
	g.mu.Unlock()

	g.applyRuntimeSettings()
	g.registerBuiltinRelievers()
	g.Start()
	return g
}

// resolveLimitLocked computes the effective budget. Caller holds g.mu.
func (g *Guard) resolveLimitLocked() int64 {
	if g.cfg.LimitBytes > 0 {
		return g.cfg.LimitBytes
	}
	if g.systemBytes <= 0 {
		return 0
	}
	pct := g.cfg.AutoLimitPercent
	if pct <= 0 {
		pct = DefaultConfig().AutoLimitPercent
	}
	return g.systemBytes * int64(pct) / 100
}

// applyRuntimeSettings pushes the budget into the Go runtime and (on Linux)
// asks the kernel not to pick this process for the OOM killer.
func (g *Guard) applyRuntimeSettings() {
	g.mu.Lock()
	if g.systemBytes <= 0 {
		g.systemBytes, g.limitSource = detectSystemMemory()
	}
	g.limitBytes = g.resolveLimitLocked()
	cfg := g.cfg
	limit := g.limitBytes
	g.mu.Unlock()

	if cfg.Enabled && cfg.ApplyGoMemLimit && limit > 0 {
		// GOMEMLIMIT is a soft limit: the GC works harder as the heap approaches
		// it rather than the process ballooning until the OS kills it. Leave a
		// small margin under our own emergency threshold.
		soft := limit * int64(cfg.EmergencyPercent) / 100
		debug.SetMemoryLimit(soft)
		g.record(LevelNormal, "gomemlimit", fmt.Sprintf("GOMEMLIMIT=%s (budget %s)", humanBytes(soft), humanBytes(limit)), 0)
	} else {
		debug.SetMemoryLimit(1<<63 - 1)
	}

	if cfg.Enabled && cfg.ProtectFromOOMKiller {
		note, ok := protectFromOOMKiller()
		g.mu.Lock()
		g.oomProtection = note
		g.mu.Unlock()
		if ok {
			g.record(LevelNormal, "oom-protection", note, 0)
		}
	}

	g.setGCPercent(100)
}

// Start begins monitoring. Idempotent.
func (g *Guard) Start() {
	g.mu.Lock()
	if g.running || !g.cfg.Enabled {
		g.mu.Unlock()
		return
	}
	g.running = true
	g.stop = make(chan struct{})
	interval := time.Duration(g.cfg.IntervalSeconds) * time.Second
	stop := g.stop
	g.mu.Unlock()

	g.wg.Add(1)
	go g.loop(interval, stop)
	log.Printf("memguard: watching memory, budget %s (%s)", humanBytes(g.Limit()), g.LimitSource())
}

// Stop halts monitoring and restores unrestricted runtime settings.
func (g *Guard) Stop() {
	g.mu.Lock()
	if !g.running {
		g.mu.Unlock()
		return
	}
	g.running = false
	close(g.stop)
	g.mu.Unlock()

	g.wg.Wait()
	g.setGCPercent(100)
	g.setLevel(LevelNormal)
}

func (g *Guard) loop(interval time.Duration, stop chan struct{}) {
	defer g.wg.Done()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			g.tick()
		}
	}
}

// tick takes one sample, updates the pressure level and relieves if needed.
func (g *Guard) tick() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("memguard: monitor tick panicked: %v", r)
		}
	}()

	sample := g.sample()
	g.pushSample(sample)

	g.mu.RLock()
	cfg := g.cfg
	prev := g.level
	lastRelief := g.lastRelief
	g.mu.RUnlock()

	next := g.evaluate(sample, cfg, prev)

	if next != prev {
		g.setLevel(next)
		if next > prev {
			g.record(next, "pressure-up", fmt.Sprintf("%s → %s at %.1f%% (%s)", prev, next, sample.UsedPercent, humanBytes(sample.UsedBytes)), 0)
			g.applyLevelSettings(next, cfg)
			g.relieve(next, cfg)
		} else {
			g.record(next, "pressure-down", fmt.Sprintf("%s → %s at %.1f%%", prev, next, sample.UsedPercent), 0)
			g.applyLevelSettings(next, cfg)
			if next == LevelNormal {
				g.restore()
			}
		}
		return
	}

	// Still under pressure: keep relieving on a cooldown instead of only once.
	if next >= LevelWarning && time.Since(lastRelief) >= time.Duration(cfg.ReliefCooldownSeconds)*time.Second {
		g.relieve(next, cfg)
	}
}

// evaluate maps a sample to a level, requiring a few calm samples and a margin
// below the threshold before stepping down.
func (g *Guard) evaluate(s Sample, cfg ConfigEntity, prev Level) Level {
	if s.LimitBytes <= 0 {
		return LevelNormal
	}

	pct := s.UsedPercent
	raw := LevelNormal
	switch {
	case pct >= float64(cfg.EmergencyPercent):
		raw = LevelEmergency
	case pct >= float64(cfg.CriticalPercent):
		raw = LevelCritical
	case pct >= float64(cfg.WarningPercent):
		raw = LevelWarning
	}

	if raw >= prev {
		g.mu.Lock()
		g.calmStreak = 0
		g.mu.Unlock()
		return raw
	}

	// Stepping down: require the reading to be a margin below the threshold of
	// the level we are leaving, for several consecutive samples.
	var leaving int
	switch prev {
	case LevelEmergency:
		leaving = cfg.EmergencyPercent
	case LevelCritical:
		leaving = cfg.CriticalPercent
	case LevelWarning:
		leaving = cfg.WarningPercent
	default:
		return raw
	}
	if pct > float64(leaving-hysteresisPercent) {
		g.mu.Lock()
		g.calmStreak = 0
		g.mu.Unlock()
		return prev
	}

	g.mu.Lock()
	g.calmStreak++
	streak := g.calmStreak
	g.mu.Unlock()

	if streak < stepDownSamples {
		return prev
	}
	g.mu.Lock()
	g.calmStreak = 0
	g.mu.Unlock()
	return prev - 1
}

// applyLevelSettings tunes the GC for the given pressure level.
func (g *Guard) applyLevelSettings(level Level, cfg ConfigEntity) {
	if !cfg.AdaptiveGC {
		return
	}
	// Lower GOGC = collect sooner = less RAM, more CPU. Trading CPU for
	// survival is exactly the point under pressure.
	switch level {
	case LevelWarning:
		g.setGCPercent(70)
	case LevelCritical:
		g.setGCPercent(40)
	case LevelEmergency:
		g.setGCPercent(20)
	default:
		g.setGCPercent(100)
	}
}

func (g *Guard) setGCPercent(pct int) {
	g.mu.Lock()
	if g.gcPercent == pct {
		g.mu.Unlock()
		return
	}
	g.gcPercent = pct
	g.mu.Unlock()
	debug.SetGCPercent(pct)
}

// relieve runs every reliever eligible for the level, in priority order.
func (g *Guard) relieve(level Level, cfg ConfigEntity) []ReliefResult {
	g.mu.Lock()
	g.lastRelief = time.Now()
	g.reliefCount++
	if level >= LevelCritical {
		g.degraded = true
	}
	eligible := make([]*registeredReliever, 0, len(g.relievers))
	if cfg.ShedLoad {
		for _, r := range g.relievers {
			if level >= r.MinLevel {
				eligible = append(eligible, r)
			}
		}
	}
	g.mu.Unlock()

	sort.SliceStable(eligible, func(i, j int) bool { return eligible[i].Priority < eligible[j].Priority })

	before := currentUsage()
	results := make([]ReliefResult, 0, len(eligible))
	var freedTotal int64

	for _, r := range eligible {
		res := ReliefResult{Name: r.Name}
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					res.Error = fmt.Sprintf("panic: %v", rec)
					log.Printf("memguard: reliever %s panicked: %v", r.Name, rec)
				}
			}()
			if r.Release != nil {
				res.FreedBytes, res.Note = r.Release(level)
			}
		}()
		freedTotal += res.FreedBytes
		results = append(results, res)

		g.mu.Lock()
		r.runs++
		r.freed += res.FreedBytes
		r.lastRun = time.Now()
		g.mu.Unlock()

		if res.FreedBytes > 0 || res.Note != "" {
			g.record(level, "release:"+r.Name, res.Note, res.FreedBytes)
		}
	}

	// Give the memory back to the OS — RSS is what the OOM killer scores, and a
	// freed Go heap alone does not shrink it.
	if level >= LevelCritical && cfg.ReturnMemoryToOS {
		debug.FreeOSMemory()
	} else if level >= LevelWarning {
		runtime.GC()
	}

	after := currentUsage()
	reclaimed := before - after
	if reclaimed < 0 {
		reclaimed = 0
	}

	g.mu.Lock()
	g.totalFreed += reclaimed
	g.mu.Unlock()

	g.record(level, "relief-sweep",
		fmt.Sprintf("%d reliever ran, %s reported, RSS/heap %s → %s", len(results), humanBytes(freedTotal), humanBytes(before), humanBytes(after)),
		reclaimed)

	return results
}

// restore re-enables whatever the relievers switched off.
func (g *Guard) restore() {
	g.mu.Lock()
	if !g.degraded {
		g.mu.Unlock()
		return
	}
	g.degraded = false
	list := make([]*registeredReliever, len(g.relievers))
	copy(list, g.relievers)
	g.mu.Unlock()

	for _, r := range list {
		if r.Restore == nil {
			continue
		}
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					log.Printf("memguard: restore %s panicked: %v", r.Name, rec)
				}
			}()
			r.Restore()
		}()
	}
	g.record(LevelNormal, "restore", "pressure cleared, degraded features re-enabled", 0)
}

// ReleaseNow runs a relief sweep on demand (dashboard button / API).
func (g *Guard) ReleaseNow(level Level) []ReliefResult {
	g.mu.RLock()
	cfg := g.cfg
	g.mu.RUnlock()
	cfg.ShedLoad = true // an explicit request always sheds
	return g.relieve(level, cfg)
}

// sample reads the current memory picture.
func (g *Guard) sample() Sample {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	rss := processRSS()
	goInUse := int64(ms.Sys - ms.HeapReleased)
	used := goInUse
	if rss > used {
		used = rss
	}

	g.mu.RLock()
	limit := g.limitBytes
	level := g.level
	g.mu.RUnlock()

	pct := 0.0
	if limit > 0 {
		pct = float64(used) / float64(limit) * 100
	}

	return Sample{
		Timestamp:   time.Now().Unix(),
		UsedBytes:   used,
		RSSBytes:    rss,
		HeapBytes:   int64(ms.HeapAlloc),
		GoSysBytes:  int64(ms.Sys),
		LimitBytes:  limit,
		UsedPercent: pct,
		Goroutines:  runtime.NumGoroutine(),
		NumGC:       ms.NumGC,
		Level:       level,
	}
}

// currentUsage is a cheap usage read used to measure a sweep's effect.
func currentUsage() int64 {
	if rss := processRSS(); rss > 0 {
		return rss
	}
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return int64(ms.Sys - ms.HeapReleased)
}

func (g *Guard) pushSample(s Sample) {
	g.mu.Lock()
	g.history[g.historyPos] = s
	g.historyPos = (g.historyPos + 1) % historySize
	if g.historyLen < historySize {
		g.historyLen++
	}
	g.mu.Unlock()
}

func (g *Guard) record(level Level, action, detail string, freed int64) {
	ev := Event{
		Timestamp:  time.Now().Unix(),
		Level:      level,
		Action:     action,
		Detail:     detail,
		UsedBytes:  0,
		FreedBytes: freed,
	}
	g.mu.Lock()
	g.events[g.eventsPos] = ev
	g.eventsPos = (g.eventsPos + 1) % eventsSize
	if g.eventsLen < eventsSize {
		g.eventsLen++
	}
	g.mu.Unlock()

	if action != "relief-sweep" && detail != "" {
		log.Printf("memguard[%s]: %s — %s", level, action, detail)
	}
}

func (g *Guard) setLevel(l Level) {
	g.mu.Lock()
	g.level = l
	g.levelSince = time.Now()
	g.mu.Unlock()
	currentLevel.Store(l)
}

// RegisterReliever adds a subsystem hook. Safe to call before Init.
func RegisterReliever(r Reliever) {
	g := Get()
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, existing := range g.relievers {
		if existing.Name == r.Name {
			existing.Reliever = r
			return
		}
	}
	g.relievers = append(g.relievers, &registeredReliever{Reliever: r})
}

// registerBuiltinRelievers wires the runtime-level releases that need no
// knowledge of any subsystem.
func (g *Guard) registerBuiltinRelievers() {
	RegisterReliever(Reliever{
		Name:        "runtime-gc",
		Description: "Forces a GC cycle and returns free spans to the OS",
		MinLevel:    LevelWarning,
		Priority:    900, // last: measure after the subsystems have dropped data
		Release: func(level Level) (int64, string) {
			before := currentUsage()
			if level >= LevelCritical {
				debug.FreeOSMemory()
			} else {
				runtime.GC()
			}
			freed := before - currentUsage()
			if freed < 0 {
				freed = 0
			}
			return freed, fmt.Sprintf("GC returned %s", humanBytes(freed))
		},
	})
}

// ---- read-only accessors used by the API and by hot paths ----

// CurrentLevel returns the pressure level. Cheap enough for per-request use.
func CurrentLevel() Level { return currentLevel.Load() }

// UnderPressure reports whether memory is tight (warning or worse).
func UnderPressure() bool { return currentLevel.Load() >= LevelWarning }

// Degraded reports whether expensive, droppable work should be skipped
// (critical or worse). Subsystems should consult this before allocating
// anything large and optional.
func Degraded() bool { return currentLevel.Load() >= LevelCritical }

// Emergency reports the last-resort level where everything droppable is dropped.
func Emergency() bool { return currentLevel.Load() >= LevelEmergency }

// Limit returns the effective memory budget in bytes.
func (g *Guard) Limit() int64 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.limitBytes
}

// LimitSource says where the budget came from.
func (g *Guard) LimitSource() string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.cfg.LimitBytes > 0 {
		return "manual"
	}
	return g.limitSource
}

// Config returns a copy of the current configuration.
func (g *Guard) Config() ConfigEntity {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.cfg
}

// UpdateConfig validates, persists and applies a new configuration.
func (g *Guard) UpdateConfig(cfg ConfigEntity) (ConfigEntity, error) {
	cfg.Normalize()

	g.mu.Lock()
	cfg.ID = g.cfg.ID
	cfg.CreatedAt = g.cfg.CreatedAt
	g.cfg = cfg
	db := g.db
	wasRunning := g.running
	g.mu.Unlock()

	if db != nil {
		row := cfg
		var err error
		if row.ID == 0 {
			err = memory.Create(db, TableName, &row)
			if err == nil {
				g.mu.Lock()
				g.cfg.ID = row.ID
				g.mu.Unlock()
			}
		} else {
			err = memory.Update(db, TableName, &row)
		}
		if err != nil {
			return cfg, fmt.Errorf("failed to persist memguard config: %w", err)
		}
	}

	if wasRunning {
		g.Stop()
	}
	g.applyRuntimeSettings()
	if cfg.Enabled {
		g.Start()
	}
	return g.Config(), nil
}

// Snapshot builds the status payload for the API.
func (g *Guard) Snapshot() Status {
	s := g.sample()

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	g.mu.RLock()
	defer g.mu.RUnlock()

	relievers := make([]RelieverInfo, 0, len(g.relievers))
	for _, r := range g.relievers {
		info := RelieverInfo{
			Name:        r.Name,
			Description: r.Description,
			MinLevel:    r.MinLevel,
			Priority:    r.Priority,
			Runs:        r.runs,
			FreedBytes:  r.freed,
		}
		if !r.lastRun.IsZero() {
			info.LastRun = r.lastRun.Unix()
		}
		relievers = append(relievers, info)
	}
	sort.SliceStable(relievers, func(i, j int) bool { return relievers[i].Priority < relievers[j].Priority })

	limitSource := g.limitSource
	if g.cfg.LimitBytes > 0 {
		limitSource = "manual"
	}

	status := Status{
		Enabled:     g.cfg.Enabled,
		Running:     g.running,
		Level:       g.level,
		LimitBytes:  g.limitBytes,
		LimitSource: limitSource,
		SystemBytes: g.systemBytes,
		UsedBytes:   s.UsedBytes,
		UsedPercent: s.UsedPercent,
		RSSBytes:    s.RSSBytes,
		HeapBytes:   int64(ms.HeapAlloc),
		HeapObjects: ms.HeapObjects,
		GoSysBytes:  int64(ms.Sys),
		StackBytes:  int64(ms.StackInuse),
		Goroutines:  runtime.NumGoroutine(),
		NumGC:       ms.NumGC,
		GCPercent:   g.gcPercent,
		GoMemLimit:  debug.SetMemoryLimit(-1),
		Thresholds: Thresholds{
			WarningBytes:   g.limitBytes * int64(g.cfg.WarningPercent) / 100,
			CriticalBytes:  g.limitBytes * int64(g.cfg.CriticalPercent) / 100,
			EmergencyBytes: g.limitBytes * int64(g.cfg.EmergencyPercent) / 100,
		},
		OOMProtection: g.oomProtection,
		Config:        g.cfg,
		Relievers:     relievers,
		ReliefCount:   g.reliefCount,
		TotalFreed:    g.totalFreed,
	}
	if ms.LastGC > 0 {
		status.LastGC = int64(ms.LastGC / 1e9)
	}
	if !g.levelSince.IsZero() {
		status.LevelSince = g.levelSince.Unix()
	}
	return status
}

// History returns the samples oldest-first.
func (g *Guard) History() []Sample {
	g.mu.RLock()
	defer g.mu.RUnlock()

	out := make([]Sample, 0, g.historyLen)
	if g.historyLen < historySize {
		out = append(out, g.history[:g.historyLen]...)
		return out
	}
	out = append(out, g.history[g.historyPos:]...)
	out = append(out, g.history[:g.historyPos]...)
	return out
}

// Events returns the recorded actions newest-last.
func (g *Guard) Events() []Event {
	g.mu.RLock()
	defer g.mu.RUnlock()

	out := make([]Event, 0, g.eventsLen)
	if g.eventsLen < eventsSize {
		out = append(out, g.events[:g.eventsLen]...)
		return out
	}
	out = append(out, g.events[g.eventsPos:]...)
	out = append(out, g.events[:g.eventsPos]...)
	return out
}

// humanBytes formats a byte count for logs and event details.
func humanBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit && exp < 4; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTP"[exp])
}
