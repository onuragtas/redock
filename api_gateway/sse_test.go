package api_gateway

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sseBackend serves two events separated by gap, so a caller can tell whether
// a deadline cut the stream after the first one.
func sseBackend(t *testing.T, contentType string, gap time.Duration) (string, int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		rc := http.NewResponseController(w)
		_ = rc.SetWriteDeadline(time.Time{})
		for i := 1; i <= 2; i++ {
			fmt.Fprintf(w, "data: event-%d\n\n", i)
			_ = rc.Flush()
			if i == 1 {
				time.Sleep(gap)
			}
		}
	}))
	t.Cleanup(srv.Close)
	addr := srv.Listener.Addr().(*net.TCPAddr)
	return addr.IP.String(), addr.Port
}

// readEvents reads up to two "data:" lines, returning what arrived before the
// stream ended.
func readEvents(t *testing.T, body *bufio.Scanner) []string {
	t.Helper()
	var got []string
	for body.Scan() && len(got) < 2 {
		if line := body.Text(); line != "" {
			got = append(got, line)
		}
	}
	return got
}

// A stream must outlive Server.WriteTimeout, which cannot be disabled through
// configuration and would otherwise cap every SSE connection.
func TestSSEStreamSurvivesServerWriteTimeout(t *testing.T) {
	host, port := sseBackend(t, sseContentType, 1500*time.Millisecond)
	g := newTestGateway(host, port, "http", &Route{Paths: []string{"/events"}})
	g.config.Timeouts = &TimeoutsConfig{ServerWriteSec: 1}
	front := startGatewayFront(t, g)

	req, err := http.NewRequest(http.MethodGet, front+"/events", nil)
	require.NoError(t, err)
	req.Header.Set("Accept", sseContentType)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, []string{"data: event-1", "data: event-2"}, readEvents(t, bufio.NewScanner(resp.Body)))
}

// Clients that omit Accept are recognized from the upstream's Content-Type.
func TestSSEStreamSurvivesWriteTimeoutWithoutAcceptHeader(t *testing.T) {
	host, port := sseBackend(t, "text/event-stream; charset=utf-8", 1500*time.Millisecond)
	g := newTestGateway(host, port, "http", &Route{Paths: []string{"/events"}})
	g.config.Timeouts = &TimeoutsConfig{ServerWriteSec: 1}
	front := startGatewayFront(t, g)

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Get(front + "/events")
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, []string{"data: event-1", "data: event-2"}, readEvents(t, bufio.NewScanner(resp.Body)))
}

// A route timeout caps ordinary requests but must not cap a stream.
func TestSSERequestIgnoresRouteTimeout(t *testing.T) {
	host, port := sseBackend(t, sseContentType, 1500*time.Millisecond)
	g := newTestGateway(host, port, "http", &Route{Paths: []string{"/events"}, Timeout: 1})
	front := startGatewayFront(t, g)

	req, err := http.NewRequest(http.MethodGet, front+"/events", nil)
	require.NoError(t, err)
	req.Header.Set("Accept", sseContentType)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, []string{"data: event-1", "data: event-2"}, readEvents(t, bufio.NewScanner(resp.Body)))
}

func TestIsSSERequest(t *testing.T) {
	cases := map[string]bool{
		"":                                    false,
		"text/event-stream":                   true,
		"TEXT/EVENT-STREAM":                   true,
		"text/event-stream; charset=utf-8":    true,
		"application/json, text/event-stream": true,
		"text/event-stream;q=0.9, */*;q=0.1":  true,
		"*/*":                                 false,
		"application/json":                    false,
		"text/event-streamer":                 false,
	}
	for accept, want := range cases {
		r := httptest.NewRequest(http.MethodGet, "/events", nil)
		if accept != "" {
			r.Header.Set("Accept", accept)
		}
		assert.Equal(t, want, isSSERequest(r), "Accept: %q", accept)
	}
}

func TestIsSSEResponse(t *testing.T) {
	cases := map[string]bool{
		"":                                 false,
		"text/event-stream":                true,
		"text/event-stream; charset=utf-8": true,
		"application/json":                 false,
	}
	for ct, want := range cases {
		resp := &http.Response{Header: http.Header{}}
		if ct != "" {
			resp.Header.Set("Content-Type", ct)
		}
		assert.Equal(t, want, isSSEResponse(resp), "Content-Type: %q", ct)
	}
}
