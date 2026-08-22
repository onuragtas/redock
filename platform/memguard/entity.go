package memguard

import "redock/platform/memory"

// TableName is the memory DB table holding the single-row memory guard
// configuration (same pattern as platform/jwtsecrets).
const TableName = "memguard_config"

// ConfigEntity is the persisted tuning of the memory guard. A single row is
// kept; defaults are applied by Normalize when fields are zero so an older
// row missing a field still boots with sane values.
type ConfigEntity struct {
	memory.BaseEntity

	// Enabled turns the whole guard (monitoring + relief) on or off.
	Enabled bool `json:"enabled"`

	// LimitBytes is the memory budget the process must stay inside. 0 means
	// "derive it automatically" from the cgroup limit / physical RAM.
	LimitBytes int64 `json:"limit_bytes"`

	// AutoLimitPercent is the share of the detected container/host memory used
	// as the budget when LimitBytes is 0.
	AutoLimitPercent int `json:"auto_limit_percent"`

	// Pressure thresholds as a percentage of the budget.
	WarningPercent   int `json:"warning_percent"`
	CriticalPercent  int `json:"critical_percent"`
	EmergencyPercent int `json:"emergency_percent"`

	// IntervalSeconds is how often memory is sampled.
	IntervalSeconds int `json:"interval_seconds"`

	// ApplyGoMemLimit wires the budget into the Go runtime (GOMEMLIMIT) so the
	// GC gets aggressive before the OS ever considers killing the process.
	ApplyGoMemLimit bool `json:"apply_go_mem_limit"`

	// AdaptiveGC lowers GOGC as pressure rises (more CPU, less RAM).
	AdaptiveGC bool `json:"adaptive_gc"`

	// ShedLoad lets subsystems degrade themselves (drop caches, stop capturing
	// payloads, shrink buffers) instead of letting the process die.
	ShedLoad bool `json:"shed_load"`

	// ReturnMemoryToOS calls debug.FreeOSMemory() at critical level so RSS —
	// what the OOM killer actually looks at — follows the heap down.
	ReturnMemoryToOS bool `json:"return_memory_to_os"`

	// ProtectFromOOMKiller lowers /proc/self/oom_score_adj on Linux so the
	// kernel picks another process first. No-op elsewhere.
	ProtectFromOOMKiller bool `json:"protect_from_oom_killer"`

	// ReliefCooldownSeconds is the minimum gap between two relief sweeps at
	// the same pressure level.
	ReliefCooldownSeconds int `json:"relief_cooldown_seconds"`
}

// DefaultConfig returns the built-in configuration.
func DefaultConfig() ConfigEntity {
	return ConfigEntity{
		Enabled:               true,
		LimitBytes:            0,
		AutoLimitPercent:      70,
		WarningPercent:        70,
		CriticalPercent:       85,
		EmergencyPercent:      93,
		IntervalSeconds:       5,
		ApplyGoMemLimit:       true,
		AdaptiveGC:            true,
		ShedLoad:              true,
		ReturnMemoryToOS:      true,
		ProtectFromOOMKiller:  true,
		ReliefCooldownSeconds: 20,
	}
}

// Normalize fills zero/out-of-range fields with defaults and keeps the
// thresholds strictly increasing so level detection cannot misbehave.
func (c *ConfigEntity) Normalize() {
	d := DefaultConfig()

	if c.AutoLimitPercent <= 0 || c.AutoLimitPercent > 95 {
		c.AutoLimitPercent = d.AutoLimitPercent
	}
	if c.WarningPercent <= 0 || c.WarningPercent >= 100 {
		c.WarningPercent = d.WarningPercent
	}
	if c.CriticalPercent <= 0 || c.CriticalPercent >= 100 {
		c.CriticalPercent = d.CriticalPercent
	}
	if c.EmergencyPercent <= 0 || c.EmergencyPercent > 99 {
		c.EmergencyPercent = d.EmergencyPercent
	}
	if c.CriticalPercent <= c.WarningPercent {
		c.CriticalPercent = c.WarningPercent + 5
	}
	if c.EmergencyPercent <= c.CriticalPercent {
		c.EmergencyPercent = c.CriticalPercent + 5
	}
	if c.EmergencyPercent > 99 {
		c.EmergencyPercent = 99
	}
	if c.IntervalSeconds < 1 {
		c.IntervalSeconds = d.IntervalSeconds
	}
	if c.IntervalSeconds > 300 {
		c.IntervalSeconds = 300
	}
	if c.ReliefCooldownSeconds < 1 {
		c.ReliefCooldownSeconds = d.ReliefCooldownSeconds
	}
	if c.LimitBytes < 0 {
		c.LimitBytes = 0
	}
}
