package rtc_server

import (
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// testBaseDir is shared by every test in the package. Galène opens its static
// root once per process (os.OpenRoot on the first start), exactly as in
// production where the directory never changes; per-test temp dirs would leave
// later tests serving from a deleted directory.
var testBaseDir string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "rtc-test-")
	if err != nil {
		panic(err)
	}
	testBaseDir = dir
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	var lastErr error
	for i := 0; i < 50; i++ { // the listener comes up asynchronously
		resp, err := http.Get(url)
		if err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return resp.StatusCode, string(b)
		}
		lastErr = err
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("GET %s: %v", url, lastErr)
	return 0, ""
}

// The embedded SFU must serve its web client and the test room, and survive
// a stop/start cycle (Galène registers global handlers on the first start).
func TestEmbeddedGaleneStartStopStart(t *testing.T) {
	m := &Manager{baseDir: testBaseDir}
	cfg := &RTCConfigEntity{
		ListenAddr: freeAddr(t), TURN: "", // no TURN: avoid binding 1194 in tests
		AdminPassword: "admin-secret", TestRoomPassword: "room-secret",
	}
	start := func() {
		m.mu.Lock()
		err := m.startLocked(cfg)
		m.mu.Unlock()
		if err != nil {
			t.Fatalf("start: %v", err)
		}
	}

	start()
	base := "http://" + cfg.ListenAddr
	if code, body := get(t, base+"/"); code != http.StatusOK || !strings.Contains(body, "<html") {
		t.Fatalf("index: %d", code)
	}
	if code, _ := get(t, base+"/group/"+TestRoomName+"/"); code != http.StatusOK {
		t.Fatalf("test room page: %d", code)
	}
	if !m.Status().Running {
		t.Fatal("status not running")
	}

	if err := m.Stop(); err != nil {
		t.Fatal(err)
	}
	if _, err := http.Get(base + "/"); err == nil {
		t.Fatal("still serving after Stop")
	}

	cfg.ListenAddr = freeAddr(t) // restart on another port, as a config change would
	start()
	if code, _ := get(t, "http://"+cfg.ListenAddr+"/"); code != http.StatusOK {
		t.Fatalf("index after restart: %d", code)
	}
	_ = m.Stop()
}

func TestConfigInputValidation(t *testing.T) {
	ok := ConfigInput{ListenAddr: "127.0.0.1:8445", UDPMin: 40000, UDPMax: 40100, TURN: "auto", PublicHost: "RTC.Example.com"}
	if err := ok.validate(); err != nil || ok.PublicHost != "rtc.example.com" {
		t.Fatalf("valid input rejected: %v (%q)", err, ok.PublicHost)
	}
	for _, bad := range []ConfigInput{
		{ListenAddr: "8445"},
		{ListenAddr: "127.0.0.1:8445", UDPMin: 50000, UDPMax: 40000},
		{ListenAddr: "127.0.0.1:8445", UDPMin: 40000},
		{ListenAddr: "127.0.0.1:8445", TURN: "nope"},
		{ListenAddr: "127.0.0.1:8445", PublicHost: "bad host/x"},
	} {
		if err := bad.validate(); err == nil {
			t.Errorf("accepted invalid input %+v", bad)
		}
	}
}
