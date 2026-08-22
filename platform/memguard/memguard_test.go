package memguard

import (
	"testing"
)

func newTestGuard() *Guard {
	cfg := DefaultConfig()
	return &Guard{
		cfg:       cfg,
		history:   make([]Sample, historySize),
		events:    make([]Event, eventsSize),
		gcPercent: 100,
	}
}

func TestNormalizeKeepsThresholdsOrdered(t *testing.T) {
	cfg := ConfigEntity{WarningPercent: 90, CriticalPercent: 40, EmergencyPercent: 10}
	cfg.Normalize()

	if !(cfg.WarningPercent < cfg.CriticalPercent && cfg.CriticalPercent < cfg.EmergencyPercent) {
		t.Fatalf("thresholds not ordered after normalize: %d/%d/%d",
			cfg.WarningPercent, cfg.CriticalPercent, cfg.EmergencyPercent)
	}
	if cfg.EmergencyPercent > 99 {
		t.Fatalf("emergency threshold must stay below 100, got %d", cfg.EmergencyPercent)
	}

	zero := ConfigEntity{}
	zero.Normalize()
	d := DefaultConfig()
	if zero.IntervalSeconds != d.IntervalSeconds || zero.WarningPercent != d.WarningPercent {
		t.Fatalf("zero config did not pick up defaults: %+v", zero)
	}
}

func TestEvaluateStepsUpImmediately(t *testing.T) {
	g := newTestGuard()
	cfg := DefaultConfig()

	sample := Sample{LimitBytes: 1000, UsedPercent: float64(cfg.CriticalPercent) + 1}
	if got := g.evaluate(sample, cfg, LevelNormal); got != LevelCritical {
		t.Fatalf("expected an immediate jump to critical, got %s", got)
	}
}

func TestEvaluateHoldsLevelUntilCalm(t *testing.T) {
	g := newTestGuard()
	cfg := DefaultConfig()

	// Just below the critical threshold but inside the hysteresis band: stay put.
	inBand := Sample{LimitBytes: 1000, UsedPercent: float64(cfg.CriticalPercent) - 1}
	if got := g.evaluate(inBand, cfg, LevelCritical); got != LevelCritical {
		t.Fatalf("expected the level to hold inside the hysteresis band, got %s", got)
	}

	// Clearly below: it still takes several consecutive calm samples.
	calm := Sample{LimitBytes: 1000, UsedPercent: float64(cfg.WarningPercent) - 10}
	for i := 1; i < stepDownSamples; i++ {
		if got := g.evaluate(calm, cfg, LevelCritical); got != LevelCritical {
			t.Fatalf("sample %d: expected the level to hold until the streak completes, got %s", i, got)
		}
	}
	if got := g.evaluate(calm, cfg, LevelCritical); got != LevelWarning {
		t.Fatalf("expected a single step down to warning, got %s", got)
	}
}

func TestEvaluateWithoutLimitStaysNormal(t *testing.T) {
	g := newTestGuard()
	cfg := DefaultConfig()

	if got := g.evaluate(Sample{LimitBytes: 0, UsedPercent: 99}, cfg, LevelNormal); got != LevelNormal {
		t.Fatalf("without a budget the guard must not report pressure, got %s", got)
	}
}

func TestRelieveRunsEligibleRelieversInPriorityOrder(t *testing.T) {
	g := newTestGuard()
	cfg := DefaultConfig()

	var order []string
	g.relievers = []*registeredReliever{
		{Reliever: Reliever{
			Name: "late", MinLevel: LevelWarning, Priority: 50,
			Release: func(Level) (int64, string) { order = append(order, "late"); return 10, "ok" },
		}},
		{Reliever: Reliever{
			Name: "early", MinLevel: LevelWarning, Priority: 1,
			Release: func(Level) (int64, string) { order = append(order, "early"); return 20, "ok" },
		}},
		{Reliever: Reliever{
			Name: "critical-only", MinLevel: LevelCritical, Priority: 10,
			Release: func(Level) (int64, string) { order = append(order, "critical-only"); return 5, "ok" },
		}},
	}

	results := g.relieve(LevelWarning, cfg)

	if len(results) != 2 {
		t.Fatalf("expected only the warning-level relievers to run, got %d", len(results))
	}
	if len(order) != 2 || order[0] != "early" || order[1] != "late" {
		t.Fatalf("relievers ran out of priority order: %v", order)
	}
	if g.relievers[1].runs != 1 || g.relievers[1].freed != 20 {
		t.Fatalf("per-reliever accounting not updated: runs=%d freed=%d", g.relievers[1].runs, g.relievers[1].freed)
	}
}

func TestRelieveSurvivesAPanickingReliever(t *testing.T) {
	g := newTestGuard()
	cfg := DefaultConfig()

	ran := false
	g.relievers = []*registeredReliever{
		{Reliever: Reliever{
			Name: "boom", MinLevel: LevelWarning, Priority: 1,
			Release: func(Level) (int64, string) { panic("boom") },
		}},
		{Reliever: Reliever{
			Name: "after", MinLevel: LevelWarning, Priority: 2,
			Release: func(Level) (int64, string) { ran = true; return 1, "ok" },
		}},
	}

	results := g.relieve(LevelWarning, cfg)

	if !ran {
		t.Fatal("a panicking reliever must not stop the sweep")
	}
	if results[0].Error == "" {
		t.Fatal("expected the panic to be reported in the result")
	}
}

func TestRelieveSkipsEverythingWhenShedLoadIsOff(t *testing.T) {
	g := newTestGuard()
	cfg := DefaultConfig()
	cfg.ShedLoad = false

	called := false
	g.relievers = []*registeredReliever{
		{Reliever: Reliever{
			Name: "cache", MinLevel: LevelWarning, Priority: 1,
			Release: func(Level) (int64, string) { called = true; return 1, "ok" },
		}},
	}

	g.relieve(LevelCritical, cfg)
	if called {
		t.Fatal("relievers must not run when load shedding is disabled")
	}
}

func TestRestoreRunsOnlyAfterDegradation(t *testing.T) {
	g := newTestGuard()

	restored := 0
	g.relievers = []*registeredReliever{
		{Reliever: Reliever{Name: "x", Restore: func() { restored++ }}},
	}

	g.restore() // not degraded yet
	if restored != 0 {
		t.Fatal("restore ran without prior degradation")
	}

	g.degraded = true
	g.restore()
	if restored != 1 {
		t.Fatalf("expected restore to run once, ran %d times", restored)
	}

	g.restore() // degraded flag already cleared
	if restored != 1 {
		t.Fatalf("restore must not run twice, ran %d times", restored)
	}
}

func TestHistoryReturnsSamplesOldestFirst(t *testing.T) {
	g := newTestGuard()

	for i := 1; i <= historySize+5; i++ {
		g.pushSample(Sample{Timestamp: int64(i)})
	}

	got := g.History()
	if len(got) != historySize {
		t.Fatalf("expected a full ring of %d samples, got %d", historySize, len(got))
	}
	if got[0].Timestamp != 6 {
		t.Fatalf("expected the oldest surviving sample first, got %d", got[0].Timestamp)
	}
	if got[len(got)-1].Timestamp != int64(historySize+5) {
		t.Fatalf("expected the newest sample last, got %d", got[len(got)-1].Timestamp)
	}
}

func TestResolveLimitPrefersManualValue(t *testing.T) {
	g := newTestGuard()
	g.systemBytes = 1000

	g.cfg.LimitBytes = 0
	g.cfg.AutoLimitPercent = 70
	if got := g.resolveLimitLocked(); got != 700 {
		t.Fatalf("expected 70%% of the system total, got %d", got)
	}

	g.cfg.LimitBytes = 123
	if got := g.resolveLimitLocked(); got != 123 {
		t.Fatalf("expected the manual budget to win, got %d", got)
	}
}
