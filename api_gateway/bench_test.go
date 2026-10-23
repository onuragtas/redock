package api_gateway

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// benchGateway is a gateway with one route to a loopback upstream, client
// tracking and a global rate limit on — the default production shape.
func benchGateway(b *testing.B, accessLog bool) *Gateway {
	b.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	b.Cleanup(upstream.Close)
	addr := upstream.Listener.Addr().(*net.TCPAddr)

	g := newTestGateway(addr.IP.String(), addr.Port, "http", &Route{Paths: []string{"/api/*"}})
	g.config.AccessLogEnabled = accessLog
	g.config.ClientSecurity = defaultClientSecurityConfig()
	g.clientStatsLimit = 1000
	g.globalLimiter = newRateLimiter(1<<30, time.Minute)
	b.Cleanup(g.resetTransports)
	return g
}

func runGatewayBench(b *testing.B, accessLog bool) {
	g := benchGateway(b, accessLog)
	// A real file write (syscall per line) like stdout/journald, minus the terminal.
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		b.Fatal(err)
	}
	prev := log.Writer()
	log.SetOutput(devNull)
	b.Cleanup(func() { drainAccessLog(); log.SetOutput(prev); devNull.Close() })

	var seq atomic.Int64
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			n := seq.Add(1)
			req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/users/%d", n), strings.NewReader(`{"name":"x"}`))
			req.RemoteAddr = fmt.Sprintf("198.51.%d.%d:1234", (n/250)%250, n%250) // many distinct clients
			rec := httptest.NewRecorder()
			g.handleRequest(rec, req)
			if rec.Code != http.StatusOK {
				b.Fatalf("status %d", rec.Code)
			}
		}
	})
}

func BenchmarkGatewayProxyAccessLog(b *testing.B) { runGatewayBench(b, true) }
func BenchmarkGatewayProxyQuiet(b *testing.B)     { runGatewayBench(b, false) }

// BenchmarkGatewayPreProxy measures the gateway's own per-request work around
// the proxy hop (client checks, rate limit, route match, backend pick, stats,
// logging) without the upstream round trip, which dominates end-to-end runs.
func BenchmarkGatewayPreProxy(b *testing.B) {
	g := benchGateway(b, true)
	devNull, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	prev := log.Writer()
	log.SetOutput(devNull)
	b.Cleanup(func() { drainAccessLog(); log.SetOutput(prev); devNull.Close() })

	var seq atomic.Int64
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			n := seq.Add(1)
			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/users/%d", n), nil)
			req.RemoteAddr = fmt.Sprintf("198.51.%d.%d:1234", (n/250)%250, n%250)
			ip := getClientIP(req)
			if blocked, _ := g.isClientBlocked(ip); blocked {
				b.Fatal("blocked")
			}
			g.checkRateLimit(g.globalLimiter, ip)
			route := g.matchRoute(req)
			svc, _, release, err := g.pickBackend(route, req, ip)
			if err != nil {
				b.Fatal(err)
			}
			st := g.stats.service(svc.ID)
			st.requests.Add(1)
			g.logRequest(req, ip, 200, time.Now(), route.ID, route.Name, route.UpstreamID, svc.ID, svc.Name, true, "", bodyLogInfo{}, bodyLogInfo{})
			g.trackClientActivity(ip, req.URL.Path, route.ID, 200, true)
			release()
		}
	})
}

// drainAccessLog waits for the async access-log queue to empty so lines don't
// land on the restored (terminal) log output.
func drainAccessLog() {
	deadline := time.Now().Add(2 * time.Second)
	for accessLog.queue != nil && len(accessLog.queue) > 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond) // the batch being written
}
