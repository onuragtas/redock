package api_gateway

import (
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseGRPCTimeout(t *testing.T) {
	valid := map[string]time.Duration{
		"100m": 100 * time.Millisecond,
		"5S":   5 * time.Second,
		"2H":   2 * time.Hour,
		"3M":   3 * time.Minute,
		"7u":   7 * time.Microsecond,
		"9n":   9 * time.Nanosecond,
	}
	for in, want := range valid {
		got, ok := parseGRPCTimeout(in)
		assert.True(t, ok, in)
		assert.Equal(t, want, got, in)
	}
	for _, in := range []string{"", "5", "5x", "-1S", "1234567890S"} {
		_, ok := parseGRPCTimeout(in)
		assert.False(t, ok, in)
	}
}

func TestResponseGRPCStatus(t *testing.T) {
	h := http.Header{}
	h[http.TrailerPrefix+"Grpc-Status"] = []string{"14"}
	h[http.TrailerPrefix+"Grpc-Message"] = []string{"backend%20down"}
	code, msg, ok := responseGRPCStatus(h)
	assert.True(t, ok)
	assert.Equal(t, 14, code)
	assert.Equal(t, "backend down", msg)

	h = http.Header{}
	h.Set("Grpc-Status", "0")
	code, _, ok = responseGRPCStatus(h)
	assert.True(t, ok)
	assert.Equal(t, 0, code)

	_, _, ok = responseGRPCStatus(http.Header{})
	assert.False(t, ok)
}

func TestEncodeGRPCMessage(t *testing.T) {
	assert.Equal(t, "a%25b%0A%C3%A7", encodeGRPCMessage("a%b\nç"))
	assert.Equal(t, "plain text", encodeGRPCMessage("plain text"))
}

func TestParseHealthCheckResponse(t *testing.T) {
	status, err := parseHealthCheckResponse(grpcFrame([]byte{0x08, 0x01}))
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), status)

	status, err = parseHealthCheckResponse(grpcFrame(nil))
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), status)

	_, err = parseHealthCheckResponse([]byte{0, 0, 0, 0, 9, 0x08})
	assert.Error(t, err)
}

func TestIsGRPCRequest(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/pkg.Svc/Call", nil)
	r.Header.Set("Content-Type", "application/grpc+proto")
	assert.True(t, isGRPCRequest(r))
	r.Header.Set("Content-Type", "application/grpc-web+proto")
	assert.False(t, isGRPCRequest(r))
	r.Header.Set("Content-Type", "application/json")
	assert.False(t, isGRPCRequest(r))
}

// TestGRPCBidiStreamingThroughGateway sends messages one at a time and waits
// for each echo before sending the next, which only works if the gateway
// streams both directions without buffering. Declaring the service as plain
// "http" must also work: gRPC requests always go upstream over h2c.
func TestGRPCBidiStreamingThroughGateway(t *testing.T) {
	for _, protocol := range []string{ProtocolGRPC, "http"} {
		t.Run(protocol, func(t *testing.T) {
			host, port := startH2CBackend(t, grpcEchoBackend())
			g := newTestGateway(host, port, protocol, &Route{Paths: []string{"/echo.Echo/"}, StripPath: true})
			front := startGatewayFront(t, g)

			pr, pw := io.Pipe()
			req, err := http.NewRequest(http.MethodPost, front+"/echo.Echo/Chat", pr)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/grpc")
			req.Header.Set("Te", "trailers")
			req.Header.Set("Grpc-Timeout", "10S")

			resp, err := h2cClient().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, 2, resp.ProtoMajor)
			assert.Equal(t, http.StatusOK, resp.StatusCode)

			for _, m := range []string{"first", "second", "third"} {
				_, err := pw.Write(grpcFrame([]byte(m)))
				require.NoError(t, err)
				assert.Equal(t, m, string(readGRPCFrame(t, resp.Body)))
			}
			require.NoError(t, pw.Close())

			_, err = io.Copy(io.Discard, resp.Body)
			require.NoError(t, err)
			assert.Equal(t, "0", resp.Trailer.Get("Grpc-Status"))
		})
	}
}

func TestGRPCGatewayErrorsUseGRPCStatus(t *testing.T) {
	host, port := startH2CBackend(t, grpcEchoBackend())
	g := newTestGateway(host, port, ProtocolGRPC, &Route{Paths: []string{"/echo.Echo/"}})
	front := startGatewayFront(t, g)

	// No matching route → UNIMPLEMENTED
	code := grpcCall(t, front+"/other.Svc/Call")
	assert.Equal(t, strconv.Itoa(grpcCodeUnimplemented), code)

	// Upstream unreachable → UNAVAILABLE
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	deadPort := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	g.services["svc"].Port = deadPort
	code = grpcCall(t, front+"/echo.Echo/Chat")
	assert.Equal(t, strconv.Itoa(grpcCodeUnavailable), code)
}

func TestGRPCHealthCheck(t *testing.T) {
	host, port := startH2CBackend(t, grpcEchoBackend())
	g := newTestGateway(host, port, ProtocolGRPC, &Route{Paths: []string{"/"}})
	svc := g.services["svc"]

	svc.HealthCheck = &HealthCheck{Path: "/health", HealthyThreshold: 1, UnhealthyThreshold: 1}
	g.checkServiceHealth(svc)
	assert.True(t, g.serviceHealth["svc"].Healthy)
	assert.Empty(t, g.serviceHealth["svc"].LastError)

	svc.HealthCheck.Path = "down"
	g.checkServiceHealth(svc)
	assert.False(t, g.serviceHealth["svc"].Healthy)
	assert.Contains(t, g.serviceHealth["svc"].LastError, "not SERVING")
}

func TestGatewayServerNegotiatesH2OverTLS(t *testing.T) {
	certSrv := httptest.NewTLSServer(http.NotFoundHandler())
	defer certSrv.Close()
	cert := certSrv.TLS.Certificates[0]

	cfg := newServerTLSConfig(func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return &cert, nil })
	ln, err := tls.Listen("tcp", "127.0.0.1:0", cfg)
	require.NoError(t, err)
	srv := newGatewayServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, r.Proto)
	}), (&Gateway{}).resolveTimeouts(), cfg)
	go srv.Serve(ln)
	defer srv.Close()

	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig:   certSrv.Client().Transport.(*http.Transport).TLSClientConfig,
		ForceAttemptHTTP2: true,
	}}
	resp, err := client.Get("https://" + ln.Addr().String())
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "HTTP/2.0", string(body))
}

func TestRouteRateLimitPersistsAcrossRequests(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer backend.Close()
	addr := backend.Listener.Addr().(*net.TCPAddr)
	g := newTestGateway(addr.IP.String(), addr.Port, "http", &Route{
		Paths:             []string{"/api"},
		RateLimitEnabled:  true,
		RateLimitRequests: 1,
		RateLimitWindow:   60,
	})

	codes := make([]int, 0, 2)
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		g.handleRequest(rec, httptest.NewRequest(http.MethodGet, "/api/x", nil))
		codes = append(codes, rec.Code)
	}
	assert.Equal(t, []int{http.StatusOK, http.StatusTooManyRequests}, codes)
}

// --- helpers ---

func grpcFrame(msg []byte) []byte {
	frame := make([]byte, 5, 5+len(msg))
	binary.BigEndian.PutUint32(frame[1:], uint32(len(msg)))
	return append(frame, msg...)
}

func readGRPCFrame(t *testing.T, r io.Reader) []byte {
	t.Helper()
	hdr := make([]byte, 5)
	_, err := io.ReadFull(r, hdr)
	require.NoError(t, err)
	msg := make([]byte, binary.BigEndian.Uint32(hdr[1:]))
	_, err = io.ReadFull(r, msg)
	require.NoError(t, err)
	return msg
}

// grpcEchoBackend serves /echo.Echo/Chat (bidi echo) and grpc.health.v1
// (NOT_SERVING when the requested service name is "down").
func grpcEchoBackend() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor != 2 {
			http.Error(w, "HTTP/2 required", http.StatusHTTPVersionNotSupported)
			return
		}
		w.Header().Set("Content-Type", "application/grpc")
		switch r.URL.Path {
		case grpcHealthCheckPath:
			body, _ := io.ReadAll(r.Body)
			status := byte(grpcHealthServing)
			if bytes.Contains(body, []byte("down")) {
				status = 2
			}
			w.Write(grpcFrame([]byte{0x08, status}))
			w.Header().Set(http.TrailerPrefix+"Grpc-Status", "0")
		case "/echo.Echo/Chat":
			w.WriteHeader(http.StatusOK)
			rc := http.NewResponseController(w)
			rc.Flush()
			hdr := make([]byte, 5)
			for {
				if _, err := io.ReadFull(r.Body, hdr); err != nil {
					break
				}
				msg := make([]byte, binary.BigEndian.Uint32(hdr[1:]))
				if _, err := io.ReadFull(r.Body, msg); err != nil {
					break
				}
				w.Write(grpcFrame(msg))
				rc.Flush()
			}
			w.Header().Set(http.TrailerPrefix+"Grpc-Status", "0")
		default:
			w.Header().Set("Grpc-Status", strconv.Itoa(grpcCodeUnimplemented))
			w.WriteHeader(http.StatusOK)
		}
	})
}

func startH2CBackend(t *testing.T, handler http.Handler) (string, int) {
	t.Helper()
	srv := httptest.NewUnstartedServer(handler)
	var p http.Protocols
	p.SetHTTP1(true)
	p.SetUnencryptedHTTP2(true)
	srv.Config.Protocols = &p
	srv.Start()
	t.Cleanup(srv.Close)
	addr := srv.Listener.Addr().(*net.TCPAddr)
	return addr.IP.String(), addr.Port
}

func startGatewayFront(t *testing.T, g *Gateway) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := newGatewayServer(http.HandlerFunc(g.handleRequest), g.resolveTimeouts(), nil)
	go srv.Serve(ln)
	t.Cleanup(func() {
		srv.Close()
		g.resetTransports()
	})
	return "http://" + ln.Addr().String()
}

func h2cClient() *http.Client {
	var p http.Protocols
	p.SetUnencryptedHTTP2(true)
	return &http.Client{Transport: &http.Transport{Protocols: &p}, Timeout: 10 * time.Second}
}

// grpcCall makes a unary-shaped call and returns the grpc-status it got.
func grpcCall(t *testing.T, url string) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(grpcFrame([]byte("x"))))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/grpc")
	req.Header.Set("Te", "trailers")
	resp, err := h2cClient().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	if s := resp.Header.Get("Grpc-Status"); s != "" {
		return s
	}
	return resp.Trailer.Get("Grpc-Status")
}

func newTestGateway(host string, port int, protocol string, route *Route) *Gateway {
	route.ID = "route"
	route.UpstreamID = "up"
	route.Enabled = true
	return &Gateway{
		services: map[string]*Service{
			"svc": {ID: "svc", Name: "svc", Host: host, Port: port, Protocol: protocol, Enabled: true},
		},
		upstreams: map[string]*Upstream{
			"up": {ID: "up", Strategy: StrategyRoundRobin, Enabled: true, Targets: []UpstreamTarget{{ServiceID: "svc", Weight: 1}}},
		},
		upstreamRuntimes: make(map[string]*upstreamRuntime),
		routes:           []*Route{route},
		serviceHealth:    make(map[string]*ServiceHealth),
		lastHealthCheck:  make(map[string]time.Time),
		config:           &GatewayConfig{Enabled: true},
		stats: &gatewayStatsTracker{
			startTime:    time.Now(),
			serviceStats: make(map[string]*serviceStatsTracker),
		},
		clientStats:      make(map[string]*clientStatsTracker),
		persistentBlocks: make(map[string]BlockedClient),
	}
}
