package traffic_inspector

import (
	"fmt"

	"redock/platform/memguard"
)

// degradedPayloadBytes is how much of a captured payload is kept per event once
// memory is tight. Enough to still identify the flow in the dashboard, small
// enough that 500 buffered events cost kilobytes instead of megabytes.
const degradedPayloadBytes = 256

// ringBytes estimates how much the flow ring is holding. Caller holds h.mu.
func (h *Hub) ringBytesLocked() int64 {
	var total int64
	for i := 0; i < h.ringLen; i++ {
		total += int64(len(h.ring[i].Data))
	}
	return total
}

// DropFlows clears the buffered flow events and reports the payload bytes that
// went away. Live WS streaming is unaffected — only the replay backlog is lost.
func (h *Hub) DropFlows() int64 {
	h.mu.Lock()
	defer h.mu.Unlock()

	freed := h.ringBytesLocked()
	h.ring = make([]FlowEvent, ringBufferSize)
	h.ringPos = 0
	h.ringLen = 0
	return freed
}

// TrimFlowPayloads keeps the buffered events but throws away their payload
// bodies, so the flow list still renders while the bulk of the memory is freed.
func (h *Hub) TrimFlowPayloads(keepBytes int) int64 {
	h.mu.Lock()
	defer h.mu.Unlock()

	var freed int64
	for i := 0; i < len(h.ring); i++ {
		if len(h.ring[i].Data) > keepBytes {
			freed += int64(len(h.ring[i].Data) - keepBytes)
			trimmed := make([]byte, keepBytes)
			copy(trimmed, h.ring[i].Data[:keepBytes])
			h.ring[i].Data = trimmed
		}
	}
	return freed
}

// DropLogs clears the buffered log events.
func (h *Hub) DropLogs() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.logRing = make([]LogEvent, logRingBufferSize)
	h.logRingPos = 0
	h.logRingLen = 0
}

// capturePayload applies the current pressure level to a captured payload
// before it is retained in the ring buffer. Decrypted bodies are by far the
// largest thing this process holds per connection, so they are the first thing
// to shrink when memory gets tight.
func capturePayload(data []byte) []byte {
	switch {
	case memguard.Emergency():
		return nil
	case memguard.Degraded() && len(data) > degradedPayloadBytes:
		trimmed := make([]byte, degradedPayloadBytes)
		copy(trimmed, data[:degradedPayloadBytes])
		return trimmed
	default:
		return data
	}
}

// registerMemoryRelievers lets the memory guard reclaim what the inspector is
// holding: first the payload bodies, then the whole backlog.
func registerMemoryRelievers(m *Manager) {
	memguard.RegisterReliever(memguard.Reliever{
		Name:        "traffic-inspector-payloads",
		Description: "Trims captured request/response bodies from the flow backlog",
		MinLevel:    memguard.LevelWarning,
		Priority:    10,
		Release: func(level memguard.Level) (int64, string) {
			if m == nil || m.Hub == nil {
				return 0, ""
			}
			if level >= memguard.LevelCritical {
				freed := m.Hub.DropFlows()
				m.Hub.DropLogs()
				return freed, "flow backlog dropped; new captures keep no payload"
			}
			freed := m.Hub.TrimFlowPayloads(degradedPayloadBytes)
			return freed, fmt.Sprintf("payloads trimmed to %d bytes each", degradedPayloadBytes)
		},
	})
}
