package memguard

import "sync/atomic"

// Level describes how close the process is to its memory budget.
type Level int32

const (
	// LevelNormal — plenty of headroom, nothing is degraded.
	LevelNormal Level = iota
	// LevelWarning — caches shrink, optional buffers get trimmed.
	LevelWarning
	// LevelCritical — expensive features stop allocating (payload capture,
	// body logging, telemetry buffering) and memory is returned to the OS.
	LevelCritical
	// LevelEmergency — everything droppable is dropped; the process trades
	// features for staying alive.
	LevelEmergency
)

// String returns the wire/UI representation of the level.
func (l Level) String() string {
	switch l {
	case LevelWarning:
		return "warning"
	case LevelCritical:
		return "critical"
	case LevelEmergency:
		return "emergency"
	default:
		return "normal"
	}
}

// MarshalJSON encodes the level as its name so the dashboard does not have to
// know the numeric mapping.
func (l Level) MarshalJSON() ([]byte, error) {
	return []byte(`"` + l.String() + `"`), nil
}

// ParseLevel maps a level name back to a Level, defaulting to LevelNormal.
func ParseLevel(s string) Level {
	switch s {
	case "warning":
		return LevelWarning
	case "critical":
		return LevelCritical
	case "emergency":
		return LevelEmergency
	default:
		return LevelNormal
	}
}

// atomicLevel is a lock-free holder for the current pressure level, read on hot
// paths (per request, per captured packet) where taking the guard mutex would
// be far too expensive.
type atomicLevel struct {
	v atomic.Int32
}

func (a *atomicLevel) Load() Level   { return Level(a.v.Load()) }
func (a *atomicLevel) Store(l Level) { a.v.Store(int32(l)) }
