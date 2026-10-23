package api_gateway

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// The request body must stream to the upstream: the upstream sees the first
// bytes while the client is still sending. Buffering the whole body first (the
// old behavior) holds everything back until the client finishes.
func TestRequestBodyStreamsToUpstream(t *testing.T) {
	gotFirst := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 5)
		_, _ = io.ReadFull(r.Body, buf)
		gotFirst <- string(buf)
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(upstream.Close)
	addr := upstream.Listener.Addr().(*net.TCPAddr)

	g := newTestGateway(addr.IP.String(), addr.Port, "http", &Route{Paths: []string{"/upload"}})
	front := startGatewayFront(t, g)

	pr, pw := io.Pipe()
	req, err := http.NewRequest(http.MethodPost, front+"/upload", pr)
	require.NoError(t, err)
	respDone := make(chan error, 1)
	go func() {
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			resp.Body.Close()
		}
		respDone <- err
	}()

	_, err = pw.Write([]byte("hello")) // the rest of the body is still pending
	require.NoError(t, err)
	select {
	case first := <-gotFirst:
		assert.Equal(t, "hello", first)
	case <-time.After(5 * time.Second):
		t.Fatal("upstream never saw the first bytes: the gateway buffered the body instead of streaming")
	}
	_, _ = pw.Write([]byte(" world"))
	require.NoError(t, pw.Close())
	require.NoError(t, <-respDone)
}

func TestCapturingBodyKeepsOnlyLimit(t *testing.T) {
	c := &capturingBody{ReadCloser: io.NopCloser(strings.NewReader("hello world")), limit: 4}
	data, err := io.ReadAll(c)
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(data), "body must pass through untouched")
	info := c.info()
	assert.Equal(t, "hell", info.body)
	assert.Equal(t, int64(11), info.size)
	assert.True(t, info.truncated)

	c = &capturingBody{ReadCloser: io.NopCloser(strings.NewReader("abc")), limit: 0}
	_, _ = io.ReadAll(c)
	assert.Equal(t, bodyLogInfo{size: 3, truncated: true}, c.info())
}

func TestGetClientIPTrustsOnlyProxies(t *testing.T) {
	cases := []struct {
		name, remote string
		headers      map[string]string
		want         string
	}{
		{"public peer cannot spoof XFF", "203.0.113.7:1", map[string]string{"X-Forwarded-For": "1.2.3.4"}, "203.0.113.7"},
		{"public peer cannot spoof X-Real-IP", "203.0.113.7:1", map[string]string{"X-Real-IP": "1.2.3.4"}, "203.0.113.7"},
		{"local proxy XFF honored", "127.0.0.1:1", map[string]string{"X-Forwarded-For": "198.51.100.9"}, "198.51.100.9"},
		{"rightmost untrusted hop wins", "10.0.0.2:1", map[string]string{"X-Forwarded-For": "6.6.6.6, 198.51.100.9, 10.0.0.1"}, "198.51.100.9"},
		{"cloudflare edge uses CF-Connecting-IP", "172.68.1.1:1", map[string]string{"CF-Connecting-IP": "198.51.100.20", "X-Forwarded-For": "9.9.9.9"}, "198.51.100.20"},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = c.remote
		for k, v := range c.headers {
			req.Header.Set(k, v)
		}
		assert.Equal(t, c.want, getClientIP(req), c.name)
	}

	setTrustedProxies([]string{"203.0.113.7"})
	t.Cleanup(func() { setTrustedProxies(nil) })
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.7:1"
	req.Header.Set("X-Forwarded-For", "198.51.100.30")
	assert.Equal(t, "198.51.100.30", getClientIP(req), "configured trusted proxy")
}

func TestBasicAuthCachesOnlySuccess(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("s3cret"), bcrypt.MinCost)
	require.NoError(t, err)
	route := &Route{ID: "r-auth", AuthType: "basic", BasicAuthUsers: []BasicAuthUser{{Username: "u", PasswordHash: string(hash)}}}
	g := &Gateway{}

	check := func(pw string) bool {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.SetBasicAuth("u", pw)
		ok, _ := g.checkAuth(req, route)
		return ok
	}
	assert.True(t, check("s3cret"))
	assert.True(t, basicAuthOK.hit(basicAuthKey("r-auth", "u", string(hash), "s3cret")), "success is cached")
	assert.False(t, check("wrong"))
	assert.False(t, basicAuthOK.hit(basicAuthKey("r-auth", "u", string(hash), "wrong")), "failure is never cached")
	assert.True(t, check("s3cret"))
}

func writeTestCert(t testing.TB, dir, cn string) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: cn},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	certFile, keyFile := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	require.NoError(t, os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600))
	require.NoError(t, os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600))
	return certFile, keyFile
}

func TestCertCacheReloadsOnChange(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := writeTestCert(t, dir, "first")
	var c certCache

	first, err := c.get(certFile, keyFile)
	require.NoError(t, err)
	again, err := c.get(certFile, keyFile)
	require.NoError(t, err)
	assert.Same(t, first, again, "served from memory")

	writeTestCert(t, dir, "second")
	future := time.Now().Add(2 * time.Second)
	require.NoError(t, os.Chtimes(certFile, future, future))
	require.NoError(t, os.Chtimes(keyFile, future, future))
	c.checkedAt = time.Time{} // skip the 1s recheck throttle

	renewed, err := c.get(certFile, keyFile)
	require.NoError(t, err)
	leaf, err := x509.ParseCertificate(renewed.Certificate[0])
	require.NoError(t, err)
	assert.Equal(t, "second", leaf.Subject.CommonName)

	_, err = (&certCache{}).get(filepath.Join(dir, "missing.pem"), keyFile)
	assert.True(t, errors.Is(err, os.ErrNotExist))
}

func TestMatchWildcardIgnoresCase(t *testing.T) {
	assert.True(t, matchWildcard("API.example.com", "api.EXAMPLE.com"))
	assert.True(t, matchWildcard("*.example.com", "Foo.Example.COM"))
	assert.True(t, matchWildcard("*.example.com", "EXAMPLE.com"))
	assert.False(t, matchWildcard("*.example.com", "badexample.com"))
}

func TestClientEvictionKeepsBlocked(t *testing.T) {
	g := &Gateway{clientStats: make(map[string]*clientStatsTracker), clientStatsLimit: 20}
	base := time.Now().Add(-time.Hour)
	for i := 0; i < 20; i++ {
		ip := "198.51.100." + string(rune('a'+i))
		g.clientStats[ip] = &clientStatsTracker{ip: ip, lastSeen: base.Add(time.Duration(i) * time.Second)}
	}
	oldest := "198.51.100.a"
	g.clientStats[oldest].manualBlocked = true

	g.getOrCreateClientTrackerLocked("203.0.113.1")
	assert.LessOrEqual(t, len(g.clientStats), 20)
	assert.Contains(t, g.clientStats, oldest, "blocked client survives eviction")
	assert.NotContains(t, g.clientStats, "198.51.100.b", "least recently seen unblocked client evicted")
}

func TestTelemetrySinkBackoff(t *testing.T) {
	e := &TelemetryExporter{}
	assert.True(t, e.sinkAllowed("Graylog"))
	e.sinkFailed("Graylog", errors.New("connection refused"))
	assert.False(t, e.sinkAllowed("Graylog"), "backed off after a failure")
	assert.True(t, e.sinkAllowed("Loki"), "other sinks unaffected")
	e.sinkSucceeded("Graylog")
	assert.True(t, e.sinkAllowed("Graylog"))
}

func TestProxyCandidatesTracksUntrustedForwarders(t *testing.T) {
	proxyCandidates = &proxyCandidateTracker{entries: make(map[string]*ProxyCandidate)}
	t.Cleanup(func() { setTrustedProxies(nil) })

	send := func(remote string, headers map[string]string) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = remote
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		noteProxyCandidate(req)
	}
	send("203.0.113.50:1", map[string]string{"X-Forwarded-For": "198.51.100.1", "X-Forwarded-Proto": "https"})
	send("203.0.113.50:2", map[string]string{"X-Forwarded-For": "198.51.100.2"})
	send("203.0.113.60:1", nil)                                              // no forwarding headers
	send("10.0.0.5:1", map[string]string{"X-Forwarded-For": "198.51.100.3"}) // already trusted

	list := proxyCandidates.list()
	require.Len(t, list, 1)
	assert.Equal(t, "203.0.113.50", list[0].IP)
	assert.Equal(t, int64(2), list[0].Requests)
	assert.Equal(t, "198.51.100.2", list[0].ForwardedFor)

	setTrustedProxies([]string{"203.0.113.50"})
	assert.Empty(t, proxyCandidates.list(), "trusted peers drop out of the list")
}

func TestRateLimiterSweepsExpiredClients(t *testing.T) {
	l := newRateLimiter(1, 10*time.Millisecond)
	for i := 0; i < 50; i++ {
		l.allow("198.51.100." + strconv.Itoa(i))
	}
	assert.Equal(t, 50, l.size())
	time.Sleep(20 * time.Millisecond)
	for i := range l.shards { // force the next request in every shard to sweep
		l.shards[i].nextSweep = time.Time{}
	}
	for i := 0; i < rateLimitShards*4; i++ {
		l.allow("203.0.113." + strconv.Itoa(i%250))
	}
	assert.LessOrEqual(t, l.size(), rateLimitShards*4, "expired windows are swept")
	assert.False(t, l.allow("203.0.113.0"), "limit still enforced within the window")
}

func TestAccessLogOmitsBodies(t *testing.T) {
	w := &accessLogWriter{}
	w.queue = make(chan []byte, 1)
	w.once.Do(func() {}) // don't start the writer goroutine
	prev := accessLog
	accessLog = w
	t.Cleanup(func() { accessLog = prev })

	writeAccessLog(RequestLog{Path: "/login", RequestBody: `{"password":"hunter2"}`, ResponseBody: "token-abc"})
	line := string(<-w.queue)
	assert.Contains(t, line, `"/login"`)
	assert.NotContains(t, line, "hunter2")
	assert.NotContains(t, line, "token-abc")

	w.enqueue([]byte("a"))
	w.enqueue([]byte("b")) // queue full: dropped, never blocks
	assert.Equal(t, int64(1), w.dropped.Load())
}

// Concurrent tracking of the same and different clients, with readers and
// auto-blocking, must be race-free (run with -race).
func TestTrackClientActivityConcurrent(t *testing.T) {
	g := &Gateway{
		clientStats:      make(map[string]*clientStatsTracker),
		clientStatsLimit: 100,
		config: &GatewayConfig{ClientSecurity: &ClientSecurityConfig{
			TrackingEnabled: true, AutoBlockEnabled: true, NoRouteThreshold: 50, TopClientLimit: 10,
		}},
	}
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				ip := "198.51.100." + strconv.Itoa((w*7+i)%20)
				g.trackClientActivity(ip, "/x", "", 404, false)
				g.isClientBlocked(ip)
				if i%50 == 0 {
					g.getTopClients(5)
				}
			}
		}(w)
	}
	wg.Wait()
	assert.NotEmpty(t, g.getBlockedClients(), "clients past the no-route threshold get auto-blocked")
}
