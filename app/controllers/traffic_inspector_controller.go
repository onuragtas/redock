package controllers

import (
	"crypto/tls"
	"encoding/base64"
	"io"
	"net"
	"redock/traffic_inspector"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
)

// maxReplayResponseBody caps how much of a replayed response we read back,
// so a runaway/huge response can't exhaust memory.
const maxReplayResponseBody = 10 * 1024 * 1024

// replayIdleTimeout: once no further bytes arrive for this long, the
// response is considered complete. Raw-socket replay has no framing
// knowledge of its own (unlike net/http, which knows when a
// Content-Length/chunked body ends) and keep-alive connections don't close
// on their own — an idle gap is the simplest reliable stopping point for a
// manual "replay and inspect" tool.
const replayIdleTimeout = 2 * time.Second

// replayTotalTimeout bounds the whole replay attempt (dial + handshake +
// write + read), regardless of how many idle-timeout read cycles occur.
const replayTotalTimeout = 20 * time.Second

// ReplayFlowRequest replays a single captured HTTP request **byte-for-byte**
// against its real destination: the exact raw request bytes as captured
// (original header casing, order, and whitespace preserved) are written
// verbatim to a fresh TCP (+ TLS, if the original flow was encrypted)
// connection — not reconstructed through net/http, which would silently
// normalize header casing/order and inject its own default headers. This is
// intentionally independent of the MITM interception path (not re-sent
// through our fake CA); it's an ordinary raw client connection to the real
// server, same as `nc`/`openssl s_client` would produce. Used by the Live
// Traffic "Resend" action.
func ReplayFlowRequest(c *fiber.Ctx) error {
	var req struct {
		Host      string `json:"host"`
		Port      int    `json:"port"`
		TLS       bool   `json:"tls"`
		SNI       string `json:"sni"`
		RawBase64 string `json:"raw_base64"` // exact captured request bytes, verbatim
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid request body: " + err.Error(),
		})
	}
	if req.Host == "" || req.Port == 0 || req.RawBase64 == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "host, port, and raw_base64 are required",
		})
	}

	rawBytes, err := base64.StdEncoding.DecodeString(req.RawBase64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "invalid base64 in raw_base64: " + err.Error(),
		})
	}

	start := time.Now()
	respBytes, replayErr := replayRawRequest(req.Host, req.Port, req.TLS, req.SNI, rawBytes)
	durationMs := time.Since(start).Milliseconds()

	if replayErr != nil {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"error": false,
			"msg":   nil,
			"data": fiber.Map{
				"error":       replayErr.Error(),
				"duration_ms": durationMs,
			},
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data": fiber.Map{
			"raw_response_base64": base64.StdEncoding.EncodeToString(respBytes),
			"duration_ms":         durationMs,
		},
	})
}

// replayRawRequest dials host:port (TLS-wrapped if useTLS), writes rawBytes
// verbatim, and reads back whatever the server sends until an idle gap or
// the total timeout — returning the raw response bytes unmodified.
func replayRawRequest(host string, port int, useTLS bool, sni string, rawBytes []byte) ([]byte, error) {
	addr := net.JoinHostPort(host, strconv.Itoa(port))

	dialer := net.Dialer{Timeout: replayTotalTimeout}
	rawConn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	defer rawConn.Close()

	var conn net.Conn = rawConn
	if useTLS {
		serverName := sni
		if serverName == "" {
			serverName = host
		}
		tlsConn := tls.Client(rawConn, &tls.Config{ServerName: serverName})
		if err := tlsConn.Handshake(); err != nil {
			return nil, err
		}
		conn = tlsConn
	}

	deadline := time.Now().Add(replayTotalTimeout)
	conn.SetWriteDeadline(deadline)
	if _, err := conn.Write(rawBytes); err != nil {
		return nil, err
	}

	var response []byte
	buf := make([]byte, 32*1024)
	for {
		idleDeadline := time.Now().Add(replayIdleTimeout)
		if idleDeadline.After(deadline) {
			idleDeadline = deadline
		}
		conn.SetReadDeadline(idleDeadline)

		n, err := conn.Read(buf)
		if n > 0 {
			response = append(response, buf[:n]...)
			if len(response) >= maxReplayResponseBody {
				response = response[:maxReplayResponseBody]
				break
			}
		}
		if err != nil {
			// Idle timeout (response considered complete), EOF (server
			// closed), or the hard total-timeout deadline — all just mean
			// "stop reading," not a hard failure, as long as we got
			// something; an error with zero bytes read at all is reported
			// as a genuine failure.
			if len(response) == 0 {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					return nil, err
				}
				if err != io.EOF {
					return nil, err
				}
			}
			break
		}
		if time.Now().After(deadline) {
			break
		}
	}

	return response, nil
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
