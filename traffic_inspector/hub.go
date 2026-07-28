package traffic_inspector

import (
	"encoding/json"
	"maps"
	"sync"

	"github.com/gofiber/websocket/v2"
)

// ringBufferSize bounds how many recent flow events / log events are kept
// in memory for the GET /v1/vpn/flows and /v1/vpn/logs endpoints. Decrypted
// payloads are not persisted to disk anywhere — only these bounded,
// process-lifetime buffers.
const ringBufferSize = 500
const logRingBufferSize = 200

// LogEvent is a warning/error emitted by the interception pipeline (e.g. a
// TLS handshake rejected by a pinned client, a natlookup failure) streamed
// over the same /ws/traffic connection as FlowEvents, distinguished by the
// "kind" discriminator so the dashboard can render them in a separate panel
// instead of mixing them into the flow table.
type LogEvent struct {
	Kind      string `json:"kind"` // always "log" — FlowEvent messages have no such field
	Timestamp int64  `json:"timestamp"`
	Level     string `json:"level"` // "warning" | "error"
	Message   string `json:"message"`
}

// Hub fans out captured FlowEvents and LogEvents to connected dashboard WS
// clients and keeps a bounded backlog of each for clients that connect
// after the fact.
type Hub struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]*sync.Mutex

	ring    []FlowEvent
	ringPos int
	ringLen int

	logRing    []LogEvent
	logRingPos int
	logRingLen int
}

// NewHub creates an empty capture hub.
func NewHub() *Hub {
	return &Hub{
		clients: make(map[*websocket.Conn]*sync.Mutex),
		ring:    make([]FlowEvent, ringBufferSize),
		logRing: make([]LogEvent, logRingBufferSize),
	}
}

// Register adds a dashboard WS connection to the broadcast set.
func (h *Hub) Register(conn *websocket.Conn) {
	h.mu.Lock()
	h.clients[conn] = &sync.Mutex{}
	h.mu.Unlock()
}

// Unregister removes a dashboard WS connection from the broadcast set.
func (h *Hub) Unregister(conn *websocket.Conn) {
	h.mu.Lock()
	delete(h.clients, conn)
	h.mu.Unlock()
}

// Publish records a flow event in the ring buffer and broadcasts it to every
// connected dashboard client. Safe to call concurrently from many proxy
// connection goroutines at once.
func (h *Hub) Publish(event FlowEvent) {
	h.mu.Lock()
	h.ring[h.ringPos] = event
	h.ringPos = (h.ringPos + 1) % ringBufferSize
	if h.ringLen < ringBufferSize {
		h.ringLen++
	}
	h.mu.Unlock()

	h.broadcast(event)
}

// PublishLog records a warning/error in the log ring buffer and broadcasts
// it to every connected dashboard client, same as Publish but for LogEvents.
func (h *Hub) PublishLog(event LogEvent) {
	event.Kind = "log"

	h.mu.Lock()
	h.logRing[h.logRingPos] = event
	h.logRingPos = (h.logRingPos + 1) % logRingBufferSize
	if h.logRingLen < logRingBufferSize {
		h.logRingLen++
	}
	h.mu.Unlock()

	h.broadcast(event)
}

func (h *Hub) broadcast(event any) {
	h.mu.RLock()
	clients := make(map[*websocket.Conn]*sync.Mutex, len(h.clients))
	maps.Copy(clients, h.clients)
	h.mu.RUnlock()

	if len(clients) == 0 {
		return
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return
	}

	for c, lock := range clients {
		lock.Lock()
		writeErr := c.WriteMessage(websocket.TextMessage, payload)
		lock.Unlock()

		if writeErr != nil {
			h.Unregister(c)
			c.Close()
		}
	}
}

// Recent returns the most recent captured flow events, oldest first.
func (h *Hub) Recent() []FlowEvent {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]FlowEvent, 0, h.ringLen)
	if h.ringLen < ringBufferSize {
		result = append(result, h.ring[:h.ringLen]...)
		return result
	}
	result = append(result, h.ring[h.ringPos:]...)
	result = append(result, h.ring[:h.ringPos]...)
	return result
}

// RecentLogs returns the most recent log events, oldest first.
func (h *Hub) RecentLogs() []LogEvent {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]LogEvent, 0, h.logRingLen)
	if h.logRingLen < logRingBufferSize {
		result = append(result, h.logRing[:h.logRingLen]...)
		return result
	}
	result = append(result, h.logRing[h.logRingPos:]...)
	result = append(result, h.logRing[:h.logRingPos]...)
	return result
}
