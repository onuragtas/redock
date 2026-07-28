package controllers

import (
	"bytes"
	"encoding/base64"
	"io"
	"net/http"
	"redock/traffic_inspector"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
)

// maxReplayResponseBody caps how much of a replayed response body we read
// back, so a runaway/huge response can't exhaust memory.
const maxReplayResponseBody = 10 * 1024 * 1024

// replayHopByHopHeaders are framing/connection-management headers that must
// never be copied verbatim from a captured request/response — either
// net/http recomputes them itself (Content-Length, Transfer-Encoding), or
// they're meaningless/harmful to forward as-is on a fresh connection.
var replayHopByHopHeaders = map[string]bool{
	"content-length":    true,
	"transfer-encoding": true,
	"connection":        true,
	"keep-alive":        true,
	"accept-encoding":   true, // let net/http negotiate + auto-decompress gzip itself
	"host":              true, // derived from the URL instead
	"proxy-connection":  true,
	"upgrade":           true,
}

// ReplayFlowRequest replays a single captured HTTP request against its real
// destination — a plain outbound net/http request, entirely independent of
// the MITM interception path (this is not re-sent through our fake CA; it's
// an ordinary client request to the real server, same as curl/Postman would
// make). Used by the Live Traffic "Resend" action.
func ReplayFlowRequest(c *fiber.Ctx) error {
	var req struct {
		Method     string            `json:"method"`
		URL        string            `json:"url"`
		Headers    map[string]string `json:"headers"`
		Body       string            `json:"body"`
		BodyBase64 bool              `json:"body_base64"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid request body: " + err.Error(),
		})
	}
	if req.Method == "" || req.URL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "method and url are required",
		})
	}

	var bodyBytes []byte
	if req.Body != "" {
		if req.BodyBase64 {
			decoded, err := base64.StdEncoding.DecodeString(req.Body)
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
					"error": true,
					"msg":   "invalid base64 body: " + err.Error(),
				})
			}
			bodyBytes = decoded
		} else {
			bodyBytes = []byte(req.Body)
		}
	}

	httpReq, err := http.NewRequest(req.Method, req.URL, bytes.NewReader(bodyBytes))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "invalid request: " + err.Error(),
		})
	}
	for name, value := range req.Headers {
		if replayHopByHopHeaders[strings.ToLower(name)] {
			continue
		}
		httpReq.Header.Set(name, value)
	}

	client := &http.Client{Timeout: 20 * time.Second}

	start := time.Now()
	resp, err := client.Do(httpReq)
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"error": false,
			"msg":   nil,
			"data": fiber.Map{
				"error":       err.Error(),
				"duration_ms": durationMs,
			},
		})
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxReplayResponseBody))

	respHeaders := make(map[string]string, len(resp.Header))
	for name := range resp.Header {
		if replayHopByHopHeaders[strings.ToLower(name)] {
			continue
		}
		respHeaders[name] = resp.Header.Get(name)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data": fiber.Map{
			"status":      resp.StatusCode,
			"status_text": http.StatusText(resp.StatusCode),
			"headers":     respHeaders,
			"body_base64": base64.StdEncoding.EncodeToString(respBody),
			"duration_ms": durationMs,
		},
	})
}

// GetVPNCACert returns the traffic inspector's root CA certificate in PEM
// form, for the operator to install as a trusted root on an
// inspection-enabled device (fetched via ApiService + saved as a blob,
// same pattern as GetUserConfig).
func GetVPNCACert(c *fiber.Ctx) error {
	ti := traffic_inspector.GetManager()
	if ti == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Traffic inspector not initialized",
		})
	}

	c.Set("Content-Type", "application/x-pem-file")
	c.Set("Content-Disposition", "attachment; filename=redock-traffic-inspector-ca.pem")
	return c.Send(ti.CA.CACertPEM())
}

// GetVPNFlows returns the most recently captured traffic flow events (in
// memory only — nothing here is persisted to disk).
func GetVPNFlows(c *fiber.Ctx) error {
	ti := traffic_inspector.GetManager()
	if ti == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Traffic inspector not initialized",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  ti.Hub.Recent(),
	})
}

// GetVPNTrafficLogs returns the most recent interception warnings/errors
// (TLS handshake rejections, natlookup failures, etc.) — the backlog
// counterpart to the "kind":"log" messages streamed over /ws/traffic.
func GetVPNTrafficLogs(c *fiber.Ctx) error {
	ti := traffic_inspector.GetManager()
	if ti == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Traffic inspector not initialized",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  ti.Hub.RecentLogs(),
	})
}

// AttachTrafficStream upgrades to a WebSocket connection and streams every
// captured flow event (decrypted TLS/QUIC content and plain traffic alike)
// live to the dashboard as it's tapped from inspected connections.
func AttachTrafficStream(c *websocket.Conn) {
	ti := traffic_inspector.GetManager()
	if ti == nil {
		c.Close()
		return
	}

	ti.Hub.Register(c)
	defer ti.Hub.Unregister(c)

	// Keep the connection open, discarding any client->server messages
	// (this is a push-only stream); exits when the client disconnects.
	for {
		if _, _, err := c.ReadMessage(); err != nil {
			return
		}
	}
}
