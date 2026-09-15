package api_gateway

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// Service protocols that speak gRPC to the upstream. "grpc" is cleartext
// HTTP/2 (h2c), "grpcs" is HTTP/2 over TLS.
const (
	ProtocolGRPC  = "grpc"
	ProtocolGRPCS = "grpcs"
)

// gRPC status codes used by the gateway (google.golang.org/grpc/codes).
const (
	grpcCodeOK                = 0
	grpcCodeDeadlineExceeded  = 4
	grpcCodePermissionDenied  = 7
	grpcCodeResourceExhausted = 8
	grpcCodeUnimplemented     = 12
	grpcCodeInternal          = 13
	grpcCodeUnavailable       = 14
	grpcCodeUnauthenticated   = 16
)

var grpcCodeNames = map[int]string{
	0: "OK", 1: "CANCELLED", 2: "UNKNOWN", 3: "INVALID_ARGUMENT", 4: "DEADLINE_EXCEEDED",
	5: "NOT_FOUND", 6: "ALREADY_EXISTS", 7: "PERMISSION_DENIED", 8: "RESOURCE_EXHAUSTED",
	9: "FAILED_PRECONDITION", 10: "ABORTED", 11: "OUT_OF_RANGE", 12: "UNIMPLEMENTED",
	13: "INTERNAL", 14: "UNAVAILABLE", 15: "DATA_LOSS", 16: "UNAUTHENTICATED",
}

const (
	grpcHealthCheckPath = "/grpc.health.v1.Health/Check"
	// grpcHealthServing is HealthCheckResponse.ServingStatus SERVING.
	grpcHealthServing = 1
	// maxGRPCHealthResponse caps how much of a health response is read.
	maxGRPCHealthResponse = 64 * 1024
)

func isGRPCProtocol(protocol string) bool {
	return protocol == ProtocolGRPC || protocol == ProtocolGRPCS
}

// upstreamScheme maps a service protocol to the URL scheme used to reach it.
func upstreamScheme(protocol string) string {
	switch protocol {
	case "", ProtocolGRPC:
		return "http"
	case ProtocolGRPCS:
		return "https"
	default:
		return protocol
	}
}

// isGRPCRequest reports whether r is a native gRPC call. gRPC-Web is excluded:
// it runs over HTTP/1.1 and needs a translation layer the gateway doesn't have.
func isGRPCRequest(r *http.Request) bool {
	ct := r.Header.Get("Content-Type")
	return strings.HasPrefix(ct, "application/grpc") && !strings.HasPrefix(ct, "application/grpc-web")
}

// grpcCodeForHTTPStatus maps a gateway-generated HTTP error to the gRPC status
// a client should see instead.
func grpcCodeForHTTPStatus(status int) int {
	switch status {
	case http.StatusUnauthorized:
		return grpcCodeUnauthenticated
	case http.StatusForbidden:
		return grpcCodePermissionDenied
	case http.StatusNotFound:
		return grpcCodeUnimplemented
	case http.StatusTooManyRequests:
		return grpcCodeResourceExhausted
	case http.StatusGatewayTimeout:
		return grpcCodeDeadlineExceeded
	case http.StatusBadGateway, http.StatusServiceUnavailable:
		return grpcCodeUnavailable
	default:
		return grpcCodeInternal
	}
}

// writeGatewayError writes a gateway-generated error. gRPC clients get a
// trailers-only response (HTTP 200 + grpc-status) because they ignore bodies
// and non-200 statuses carry no usable error detail for them.
func writeGatewayError(w http.ResponseWriter, grpcReq bool, status int, message string) {
	if !grpcReq {
		http.Error(w, message, status)
		return
	}
	writeGRPCStatus(w, grpcCodeForHTTPStatus(status), message)
}

func writeGRPCStatus(w http.ResponseWriter, code int, message string) {
	h := w.Header()
	h.Set("Content-Type", "application/grpc")
	h.Set("Grpc-Status", strconv.Itoa(code))
	if message != "" {
		h.Set("Grpc-Message", encodeGRPCMessage(message))
	}
	w.WriteHeader(http.StatusOK)
}

// encodeGRPCMessage percent-encodes grpc-message as the gRPC HTTP/2 spec requires.
func encodeGRPCMessage(msg string) string {
	var b strings.Builder
	for i := 0; i < len(msg); i++ {
		c := msg[i]
		if c >= 0x20 && c <= 0x7E && c != '%' {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}

// responseGRPCStatus extracts grpc-status from a proxied response's headers.
// ReverseProxy copies trailers into the header map either under their own name
// (when announced) or with http.TrailerPrefix; trailers-only responses carry it
// as a plain header.
func responseGRPCStatus(h http.Header) (code int, message string, ok bool) {
	raw := h.Get("Grpc-Status")
	msg := h.Get("Grpc-Message")
	if raw == "" {
		if vv := h[http.TrailerPrefix+"Grpc-Status"]; len(vv) > 0 {
			raw = vv[0]
		}
		if vv := h[http.TrailerPrefix+"Grpc-Message"]; len(vv) > 0 {
			msg = vv[0]
		}
	}
	if raw == "" {
		return 0, "", false
	}
	code, err := strconv.Atoi(raw)
	if err != nil {
		return 0, "", false
	}
	if decoded, err := url.PathUnescape(msg); err == nil {
		msg = decoded
	}
	return code, msg, true
}

// isGRPCServerFailure reports whether a status points at the backend or the
// transport (counted as a service error) rather than an application outcome
// like NOT_FOUND or INVALID_ARGUMENT.
func isGRPCServerFailure(code int) bool {
	switch code {
	case 2, grpcCodeDeadlineExceeded, grpcCodeUnimplemented, grpcCodeInternal, grpcCodeUnavailable, 15:
		return true
	}
	return false
}

func grpcStatusError(code int, message string) string {
	name := grpcCodeNames[code]
	if name == "" {
		name = "CODE_" + strconv.Itoa(code)
	}
	if message == "" {
		return "grpc status " + name
	}
	return "grpc status " + name + ": " + message
}

// parseGRPCTimeout parses the grpc-timeout header (e.g. "100m", "5S").
func parseGRPCTimeout(v string) (time.Duration, bool) {
	if len(v) < 2 || len(v) > 9 {
		return 0, false
	}
	n, err := strconv.ParseInt(v[:len(v)-1], 10, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	var unit time.Duration
	switch v[len(v)-1] {
	case 'H':
		unit = time.Hour
	case 'M':
		unit = time.Minute
	case 'S':
		unit = time.Second
	case 'm':
		unit = time.Millisecond
	case 'u':
		unit = time.Microsecond
	case 'n':
		unit = time.Nanosecond
	default:
		return 0, false
	}
	return time.Duration(n) * unit, true
}

// announceGRPCTrailers pre-declares grpc-status/grpc-message as trailers on a
// streaming gRPC response. ReverseProxy flushes response headers right away
// only when trailers are announced; otherwise a server-streaming client would
// not see the headers until the first message. The HTTP/2 transport fills the
// same Trailer map when the real trailers arrive.
func announceGRPCTrailers(resp *http.Response) {
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/grpc") || resp.Header.Get("Grpc-Status") != "" {
		return // not gRPC, or a trailers-only response
	}
	if resp.Trailer == nil {
		resp.Trailer = make(http.Header)
	}
	for _, k := range []string{"Grpc-Status", "Grpc-Message"} {
		if _, ok := resp.Trailer[k]; !ok {
			resp.Trailer[k] = nil
		}
	}
}

// countingReadCloser counts bytes read from a streamed request body so gRPC
// calls can be logged without buffering the (possibly endless) stream. The
// count is atomic because the HTTP/2 transport reads the body on its own
// goroutine.
type countingReadCloser struct {
	io.ReadCloser
	n atomic.Int64
}

func (c *countingReadCloser) Read(p []byte) (int, error) {
	n, err := c.ReadCloser.Read(p)
	c.n.Add(int64(n))
	return n, err
}

// grpcHealthServiceName interprets HealthCheck.Path for gRPC services: a value
// starting with "/" (like the UI default "/health") or empty checks the whole
// server, anything else is the service name passed to grpc.health.v1.
func grpcHealthServiceName(path string) string {
	if path == "" || strings.HasPrefix(path, "/") {
		return ""
	}
	return path
}

// errGRPCHealthUnimplemented signals the backend answered but has no health
// service; like an HTTP 4xx it still proves the backend is reachable.
var errGRPCHealthUnimplemented = errors.New("grpc health service not implemented")

// probeGRPCHealth calls grpc.health.v1.Health/Check. The protobuf messages are
// tiny, so they are encoded by hand instead of pulling in grpc-go.
func probeGRPCHealth(ctx context.Context, client *http.Client, baseURL, serviceName string) error {
	var msg []byte
	if serviceName != "" {
		// HealthCheckRequest{ string service = 1; }
		msg = append([]byte{0x0A}, binary.AppendUvarint(nil, uint64(len(serviceName)))...)
		msg = append(msg, serviceName...)
	}
	frame := make([]byte, 5, 5+len(msg))
	binary.BigEndian.PutUint32(frame[1:], uint32(len(msg)))
	frame = append(frame, msg...)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+grpcHealthCheckPath, bytes.NewReader(frame))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/grpc")
	req.Header.Set("Te", "trailers")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxGRPCHealthResponse))
	if err != nil {
		return err
	}

	code, message, ok := responseGRPCStatus(resp.Trailer)
	if !ok {
		code, message, ok = responseGRPCStatus(resp.Header)
	}
	if !ok {
		return errors.New("grpc health response missing grpc-status")
	}
	switch code {
	case grpcCodeOK:
	case grpcCodeUnimplemented:
		return errGRPCHealthUnimplemented
	default:
		return errors.New(grpcStatusError(code, message))
	}

	status, err := parseHealthCheckResponse(body)
	if err != nil {
		return err
	}
	if status != grpcHealthServing {
		return fmt.Errorf("grpc health status %d (not SERVING)", status)
	}
	return nil
}

// parseHealthCheckResponse decodes HealthCheckResponse{ ServingStatus status = 1; }
// from a single length-prefixed gRPC frame. A missing field means 0 (UNKNOWN).
func parseHealthCheckResponse(body []byte) (uint64, error) {
	if len(body) < 5 {
		return 0, errors.New("grpc health response too short")
	}
	if body[0] != 0 {
		return 0, errors.New("compressed grpc health response not supported")
	}
	size := binary.BigEndian.Uint32(body[1:5])
	if uint64(len(body)-5) < uint64(size) {
		return 0, errors.New("truncated grpc health response")
	}
	msg := body[5 : 5+size]
	var status uint64
	for len(msg) > 0 {
		tag, n := binary.Uvarint(msg)
		if n <= 0 {
			return 0, errors.New("malformed grpc health response")
		}
		msg = msg[n:]
		switch tag & 7 {
		case 0: // varint
			v, n := binary.Uvarint(msg)
			if n <= 0 {
				return 0, errors.New("malformed grpc health response")
			}
			msg = msg[n:]
			if tag>>3 == 1 {
				status = v
			}
		case 2: // length-delimited
			l, n := binary.Uvarint(msg)
			if n <= 0 || uint64(len(msg)-n) < l {
				return 0, errors.New("malformed grpc health response")
			}
			msg = msg[n+int(l):]
		case 5: // fixed32
			if len(msg) < 4 {
				return 0, errors.New("malformed grpc health response")
			}
			msg = msg[4:]
		case 1: // fixed64
			if len(msg) < 8 {
				return 0, errors.New("malformed grpc health response")
			}
			msg = msg[8:]
		default:
			return 0, errors.New("malformed grpc health response")
		}
	}
	return status, nil
}
