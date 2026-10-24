package rtc_server

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func startTestSFU(t *testing.T) (*Manager, *RTCConfigEntity) {
	t.Helper()
	m := &Manager{baseDir: testBaseDir}
	cfg := &RTCConfigEntity{ListenAddr: freeAddr(t), AdminPassword: "a", TestRoomPassword: "room-secret"}
	m.mu.Lock()
	err := m.startLocked(cfg)
	m.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop() })
	return m, cfg
}

// The recorder joins the room as a visible "RECORDING" participant and
// leaves when stopped.
func TestRecorderIsVisibleAndStops(t *testing.T) {
	m, cfg := startTestSFU(t)
	room := &RTCRoomEntity{Name: "rec", Key: randomSecret(32), AllowRecording: true}
	if err := m.writeRoomFile(room, true); err != nil {
		t.Fatal(err)
	}
	inv, _ := signToken(room, "http://"+cfg.ListenAddr, "", RolePresenter, time.Hour)
	openJoined(t, cfg.ListenAddr, "rec", inv.Token, "bob")

	if err := m.startRecorder("admin", "rec"); err != nil {
		t.Fatal(err)
	}
	if !m.IsRecording("rec") {
		t.Fatal("not recording after start")
	}
	if err := m.startRecorder("admin", "rec"); err == nil {
		t.Error("a second recorder was started")
	}
	parts, _ := m.Participants("rec")
	visible := false
	for _, p := range parts {
		visible = visible || (p.System && p.Username == "RECORDING")
	}
	if !visible {
		t.Fatalf("recorder not visible among participants: %+v", parts)
	}
	if err := m.StopRecording("admin", "rec"); err != nil {
		t.Fatal(err)
	}
	if m.IsRecording("rec") {
		t.Fatal("still recording after stop")
	}
}

func TestRecordingFilesAndLinks(t *testing.T) {
	m := &Manager{baseDir: t.TempDir()}
	dir := filepath.Join(m.recordingsDir(), "team")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(dir, "2026-01-01T10:00:00.000-alice.webm")
	fresh := filepath.Join(dir, "2026-10-24T10:00:00.000-bob.webm")
	_ = os.WriteFile(old, []byte("old"), 0o600)
	_ = os.WriteFile(fresh, []byte("fresh"), 0o600)
	past := time.Now().AddDate(0, 0, -40)
	_ = os.Chtimes(old, past, past)

	recs, err := m.Recordings("")
	if err != nil || len(recs) != 2 || recs[0].Name != filepath.Base(fresh) {
		t.Fatalf("recordings = %+v, %v", recs, err)
	}

	for _, bad := range [][2]string{{"team", "../secret"}, {"team", ".hidden"}, {"team", "a/b.webm"}, {"../x", "f.webm"}, {"team", "missing.webm"}} {
		if _, err := m.RecordingPath(bad[0], bad[1]); err == nil {
			t.Errorf("RecordingPath(%q, %q) accepted", bad[0], bad[1])
		}
	}

	link, err := m.DownloadLink("admin", "team", filepath.Base(fresh))
	if err != nil {
		t.Fatal(err)
	}
	q, _ := url.ParseQuery(strings.SplitN(link, "?", 2)[1])
	if p, err := m.VerifyDownload(q.Get("room"), q.Get("name"), q.Get("exp"), q.Get("sig")); err != nil || p != fresh {
		t.Fatalf("valid link refused: %v", err)
	}
	if _, err := m.VerifyDownload("team", filepath.Base(old), q.Get("exp"), q.Get("sig")); err == nil {
		t.Error("signature accepted for another file")
	}
	if _, err := m.VerifyDownload("team", filepath.Base(fresh), "1", q.Get("sig")); err == nil {
		t.Error("expired link accepted")
	}

	m.applyRetentionDays(30)
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Error("recording past retention was kept")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Error("recent recording was deleted")
	}
}

// Galène serves /recordings/ itself; Redock rooms have no password users with
// the record permission, so it must stay closed — downloads go through
// Redock's signed links only.
func TestGaleneRecordingsEndpointIsClosed(t *testing.T) {
	_, cfg := startTestSFU(t)
	for _, auth := range [][2]string{{"", ""}, {"anyone", "room-secret"}, {"redock", "a"}} {
		req, _ := http.NewRequest("GET", "http://"+cfg.ListenAddr+"/recordings/"+TestRoomName+"/", nil)
		if auth[0] != "" {
			req.SetBasicAuth(auth[0], auth[1])
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			t.Errorf("/recordings/ reachable with %q", auth[0])
		}
	}
}
