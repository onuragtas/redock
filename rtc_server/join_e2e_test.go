package rtc_server

import (
	"encoding/json"
	"net/http"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gorilla/websocket"
)

// joinWithToken performs Galène's WebSocket handshake and join, returning
// the "joined" message.
func joinWithToken(t *testing.T, addr, room, tok string) map[string]any {
	t.Helper()
	ws, _, err := websocket.DefaultDialer.Dial("ws://"+addr+"/ws", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer ws.Close()
	_ = ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	send := func(m map[string]any) {
		if err := ws.WriteJSON(m); err != nil {
			t.Fatal(err)
		}
	}
	send(map[string]any{"type": "handshake", "version": []string{"2"}, "id": "test-client"})
	send(map[string]any{"type": "join", "kind": "join", "group": room, "username": "bob", "token": tok})
	for {
		_, data, err := ws.ReadMessage()
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		var m map[string]any
		_ = json.Unmarshal(data, &m)
		if m["type"] == "joined" {
			return m
		}
	}
}

// End to end: a room written by Redock, a token signed by Redock, a real
// WebSocket join against the embedded Galène.
func TestJoinRoomWithRedockToken(t *testing.T) {
	SetJoinUI(fstest.MapFS{"join.html": {Data: []byte("<html>redock join</html>")}})
	t.Cleanup(func() { SetJoinUI(nil) })

	m := &Manager{baseDir: testBaseDir}
	cfg := &RTCConfigEntity{ListenAddr: freeAddr(t), AdminPassword: "a", TestRoomPassword: "b"}
	m.mu.Lock()
	err := m.startLocked(cfg)
	m.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop() })

	room := &RTCRoomEntity{Name: "team", Key: randomSecret(32)}
	if err := m.writeRoomFile(room, true); err != nil {
		t.Fatal(err)
	}

	inv, err := signToken(room, "http://"+cfg.ListenAddr, "", RolePresenter, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	joined := joinWithToken(t, cfg.ListenAddr, "team", inv.Token)
	if joined["kind"] != "join" {
		t.Fatalf("join refused: %v", joined)
	}
	perms, _ := joined["permissions"].([]any)
	hasPresent := false
	for _, p := range perms {
		hasPresent = hasPresent || p == "present"
	}
	if !hasPresent {
		t.Errorf("presenter lacks present: %v", perms)
	}
	if joined["username"] != "bob" {
		t.Errorf("username = %v (an invite without a name lets the user choose)", joined["username"])
	}

	forged, _ := signToken(&RTCRoomEntity{Name: "team", Key: randomSecret(32)}, "http://x", "", RoleModerator, time.Hour)
	if refused := joinWithToken(t, cfg.ListenAddr, "team", forged.Token); refused["kind"] != "fail" {
		t.Fatalf("forged token was not refused: %v", refused)
	}

	resp, err := http.Get("http://" + cfg.ListenAddr + "/redock/join.html")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("join page: %d", resp.StatusCode)
	}
}
