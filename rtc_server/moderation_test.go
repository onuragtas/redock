package rtc_server

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// openJoined joins room over WebSocket and keeps the connection open.
func openJoined(t *testing.T, addr, room, tok, name string) *websocket.Conn {
	t.Helper()
	ws, _, err := websocket.DefaultDialer.Dial("ws://"+addr+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ws.Close() })
	_ = ws.WriteJSON(map[string]any{"type": "handshake", "version": []string{"2"}, "id": "c-" + name})
	_ = ws.WriteJSON(map[string]any{"type": "join", "kind": "join", "group": room, "username": name, "token": tok})
	_ = ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		_, data, err := ws.ReadMessage()
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		var m map[string]any
		_ = json.Unmarshal(data, &m)
		if m["type"] == "joined" {
			if m["kind"] != "join" {
				t.Fatalf("join refused: %v", m)
			}
			return ws
		}
	}
}

// waitClosed reads until the server closes the connection, returning the
// kinds of user messages seen on the way.
func waitClosed(t *testing.T, ws *websocket.Conn) []string {
	t.Helper()
	var kinds []string
	_ = ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		_, data, err := ws.ReadMessage()
		if err != nil {
			return kinds
		}
		var m map[string]any
		_ = json.Unmarshal(data, &m)
		if m["type"] == "usermessage" {
			k, _ := m["kind"].(string)
			kinds = append(kinds, k)
		}
	}
}

func TestPanelModeration(t *testing.T) {
	m := &Manager{baseDir: testBaseDir}
	cfg := &RTCConfigEntity{ListenAddr: freeAddr(t), AdminPassword: "a", TestRoomPassword: "b"}
	m.mu.Lock()
	err := m.startLocked(cfg)
	m.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop() })

	room := &RTCRoomEntity{Name: "mod", Key: randomSecret(32)}
	if err := m.writeRoomFile(room, true); err != nil {
		t.Fatal(err)
	}
	inv, _ := signToken(room, "http://"+cfg.ListenAddr, "", RolePresenter, time.Hour)
	bob := openJoined(t, cfg.ListenAddr, "mod", inv.Token, "bob")

	parts, err := m.Participants("mod")
	if err != nil || len(parts) != 1 {
		t.Fatalf("participants = %+v, %v", parts, err)
	}
	p := parts[0]
	if p.Username != "bob" || p.Role != RolePresenter || p.IP != "127.0.0.1" || p.System {
		t.Fatalf("participant = %+v", p)
	}

	if err := m.Kick("admin@example.com", "mod", p.ID, "testing"); err != nil {
		t.Fatal(err)
	}
	kinds := waitClosed(t, bob)
	found := false
	for _, k := range kinds {
		found = found || k == "kicked"
	}
	if !found {
		t.Errorf("kicked participant got no 'kicked' message (got %v)", kinds)
	}

	if err := m.SetLock("admin@example.com", "mod", true, "closed for now"); err != nil {
		t.Fatal(err)
	}
	ws, _, err := websocket.DefaultDialer.Dial("ws://"+cfg.ListenAddr+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	_ = ws.WriteJSON(map[string]any{"type": "handshake", "version": []string{"2"}, "id": "late"})
	_ = ws.WriteJSON(map[string]any{"type": "join", "kind": "join", "group": "mod", "username": "late", "token": inv.Token})
	_ = ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		_, data, err := ws.ReadMessage()
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		var msg map[string]any
		_ = json.Unmarshal(data, &msg)
		if msg["type"] == "joined" {
			if msg["kind"] != "fail" {
				t.Fatalf("joined a locked room: %v", msg)
			}
			break
		}
	}

	if err := m.SetLock("admin@example.com", "mod", false, ""); err != nil {
		t.Fatal(err)
	}
	again := openJoined(t, cfg.ListenAddr, "mod", inv.Token, "again")
	_ = again
	if n, err := m.KickAll("admin@example.com", "mod", ""); err != nil || n != 1 {
		t.Fatalf("end session: %d, %v", n, err)
	}

	if _, err := signToken(room, "http://x", "../evil", RolePresenter, time.Hour); err == nil {
		t.Error("token with a path-traversal username was issued")
	}
}
