package rtc_server

import (
	"strings"
	"testing"
	"time"

	"github.com/jech/galene/token"
)

func roomKeys(r *RTCRoomEntity) []map[string]any {
	return []map[string]any{{"kty": "oct", "alg": "HS256", "k": r.Key}}
}

// Tokens Redock signs must be accepted by Galène for their room only, with
// the role's permissions, and rejected under any other room key.
func TestSignedTokensAreAcceptedByGalene(t *testing.T) {
	room := &RTCRoomEntity{Name: "team", Key: randomSecret(32), AllowRecording: true}
	inv, err := signToken(room, "https://rtc.example.com", "alice", RoleModerator, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(inv.JoinURL, "https://rtc.example.com/redock/join.html#room=team&token=") {
		t.Fatalf("join URL = %s", inv.JoinURL)
	}

	tok, err := token.Parse(inv.Token, roomKeys(room))
	if err != nil || tok == nil {
		t.Fatalf("Galène rejected the token: %v", err)
	}
	user, perms, err := tok.Check("", "team")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if user != "alice" {
		t.Errorf("user = %q", user)
	}
	want := map[string]bool{"op": true, "present": true, "record": true}
	got := map[string]bool{}
	for _, p := range perms {
		got[p] = true
	}
	for p := range want {
		if !got[p] {
			t.Errorf("moderator token lacks %q (perms %v)", p, perms)
		}
	}

	if _, _, err := tok.Check("", "other-room"); err == nil {
		t.Error("token accepted for another room")
	}
	other := &RTCRoomEntity{Name: "team", Key: randomSecret(32)}
	if tok, err := token.Parse(inv.Token, roomKeys(other)); err == nil && tok != nil {
		t.Error("token verified under a different room key")
	}
}

func TestTokenRolesAndLimits(t *testing.T) {
	room := &RTCRoomEntity{Name: "team", Key: randomSecret(32)}
	viewer, err := signToken(room, "http://127.0.0.1:8445", "", RoleViewer, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	tok, _ := token.Parse(viewer.Token, roomKeys(room))
	user, perms, err := tok.Check("", "team")
	if err != nil || user != "" || len(perms) != 1 || perms[0] != "message" {
		t.Fatalf("viewer: user=%q perms=%v err=%v", user, perms, err)
	}
	// Recording is only granted where the room allows it.
	mod, _ := signToken(room, "http://x", "m", RoleModerator, time.Hour)
	tok, _ = token.Parse(mod.Token, roomKeys(room))
	_, perms, _ = tok.Check("", "team")
	for _, p := range perms {
		if p == "record" {
			t.Error("record granted in a room without recording")
		}
	}
	if _, err := signToken(room, "http://x", "", "superuser", time.Hour); err == nil {
		t.Error("unknown role accepted")
	}
	if _, err := signToken(room, "http://x", "", RolePresenter, 31*24*time.Hour); err == nil {
		t.Error("token lifetime above the cap accepted")
	}
}

func TestRoomInputValidation(t *testing.T) {
	for _, bad := range []string{"", "Has Space", "../x", TestRoomName, strings.Repeat("a", 65)} {
		in := RoomInput{Name: bad}
		if err := in.normalize(); err == nil {
			t.Errorf("accepted room name %q", bad)
		}
	}
	in := RoomInput{Name: " Team-1 "}
	if err := in.normalize(); err != nil || in.Name != "team-1" {
		t.Fatalf("normalize: %q %v", in.Name, err)
	}
}
