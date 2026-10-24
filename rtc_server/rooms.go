package rtc_server

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"redock/platform/database"
	"redock/platform/memory"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jech/galene/group"
)

const (
	TableRTCRooms   = "rtc_rooms"
	TableRTCAPIKeys = "rtc_api_keys"
)

// RTCRoomEntity is a room managed by Redock. Rooms accept only signed
// tokens (no passwords): each room has its own HS256 key, written to the
// Galène group file as an authKey, and Redock signs invites with it.
type RTCRoomEntity struct {
	memory.BaseEntity
	Name           string    `json:"name"` // URL-safe id, the Galène group name
	DisplayName    string    `json:"display_name"`
	Description    string    `json:"description"`
	MaxClients     int       `json:"max_clients"` // 0 = unlimited
	AllowRecording bool      `json:"allow_recording"`
	Key            string    `json:"-"` // base64url HS256 key (JWK "k"); never leaves the server
	CreatedAt      time.Time `json:"created_at"`
}

// RTCAPIKeyEntity lets an external platform create rooms and tokens through
// the REST API. Only a SHA-256 of the key is stored.
type RTCAPIKeyEntity struct {
	memory.BaseEntity
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"` // first characters, to recognise the key
	Hash       string     `json:"-"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

var roomNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// Roles map to Galène's internal permissions carried in tokens.
const (
	RoleModerator = "moderator" // op: kick, mute, lock, record (if allowed)
	RolePresenter = "presenter" // camera, microphone, screen, chat
	RoleViewer    = "viewer"    // watch and chat
	RoleObserver  = "observer"  // watch only
)

func rolePermissions(role string, allowRecording bool) ([]string, error) {
	switch role {
	case RoleModerator:
		p := []string{"op", "present", "message", "caption", "token"}
		if allowRecording {
			p = append(p, "record")
		}
		return p, nil
	case RolePresenter, "":
		return []string{"present", "message"}, nil
	case RoleViewer:
		return []string{"message"}, nil
	case RoleObserver:
		return []string{}, nil
	}
	return nil, fmt.Errorf("unknown role %q (moderator, presenter, viewer, observer)", role)
}

// RoomInput is what the dashboard / API sends for a room.
type RoomInput struct {
	Name           string `json:"name"`
	DisplayName    string `json:"display_name"`
	Description    string `json:"description"`
	MaxClients     int    `json:"max_clients"`
	AllowRecording bool   `json:"allow_recording"`
}

func (in *RoomInput) normalize() error {
	in.Name = strings.ToLower(strings.TrimSpace(in.Name))
	if !roomNamePattern.MatchString(in.Name) {
		return fmt.Errorf("room name: lowercase letters, digits, '-' or '_' (max 64)")
	}
	if in.Name == TestRoomName {
		return fmt.Errorf("%q is reserved", TestRoomName)
	}
	if in.MaxClients < 0 {
		return fmt.Errorf("max participants cannot be negative")
	}
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	return nil
}

// RoomView is a room with its live state.
type RoomView struct {
	RTCRoomEntity
	Participants int  `json:"participants"`
	Locked       bool `json:"locked"`
	Recording    bool `json:"recording"`
}

func (m *Manager) roomRows() []*RTCRoomEntity {
	d := database.GetMemoryDB()
	if d == nil {
		return nil
	}
	rows := memory.FindAll[*RTCRoomEntity](d, TableRTCRooms)
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	return rows
}

func (m *Manager) findRoom(name string) *RTCRoomEntity {
	for _, r := range m.roomRows() {
		if r.Name == name {
			return r
		}
	}
	return nil
}

func (m *Manager) Rooms() []RoomView {
	rows := m.roomRows()
	out := make([]RoomView, 0, len(rows))
	for _, r := range rows {
		v := RoomView{RTCRoomEntity: *r}
		if g := group.Get(r.Name); g != nil {
			v.Participants = g.ClientCount()
			v.Locked, _ = g.Locked()
			v.Recording = recorder(g) != nil
		}
		out = append(out, v)
	}
	return out
}

// SaveRoom creates the room or updates an existing one (the key is kept).
func (m *Manager) SaveRoom(in RoomInput) (RoomView, error) {
	if err := in.normalize(); err != nil {
		return RoomView{}, err
	}
	d := database.GetMemoryDB()
	r := m.findRoom(in.Name)
	if r == nil {
		r = &RTCRoomEntity{Name: in.Name, Key: randomSecret(32), CreatedAt: time.Now()}
		r.DisplayName, r.Description, r.MaxClients, r.AllowRecording = in.DisplayName, in.Description, in.MaxClients, in.AllowRecording
		if err := memory.Create(d, TableRTCRooms, r); err != nil {
			return RoomView{}, err
		}
	} else {
		r.DisplayName, r.Description, r.MaxClients, r.AllowRecording = in.DisplayName, in.Description, in.MaxClients, in.AllowRecording
		if err := memory.Update(d, TableRTCRooms, r); err != nil {
			return RoomView{}, err
		}
	}
	if err := m.writeRoomFile(r, m.isRunning()); err != nil {
		return RoomView{}, err
	}
	return RoomView{RTCRoomEntity: *r}, nil
}

// DeleteRoom removes the room and disconnects everyone in it. Its key is
// gone with it, so every token issued for it stops working.
func (m *Manager) DeleteRoom(name string) error {
	r := m.findRoom(name)
	if r == nil {
		return fmt.Errorf("room %q not found", name)
	}
	if err := memory.Delete[*RTCRoomEntity](database.GetMemoryDB(), TableRTCRooms, r.ID); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(m.baseDir, "groups", name+".json"))
	if g := group.Get(name); g != nil {
		for _, c := range g.GetClients(nil) {
			_ = c.Kick("", nil, "this room was closed")
		}
	}
	if m.isRunning() {
		group.Update()
	}
	return nil
}

// writeRoomFile renders the Galène group definition for r. No users or
// wildcard user: the only way in is a token signed with the room key.
// reload asks a running Galène to pick the change up now (it otherwise
// rereads group files on its next maintenance pass).
func (m *Manager) writeRoomFile(r *RTCRoomEntity, reload bool) error {
	dir := filepath.Join(m.baseDir, "groups")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	desc := map[string]any{
		"displayName": r.DisplayName,
		"description": r.Description,
		"public":      false,
		"authKeys": []map[string]any{{
			"kty": "oct", "alg": "HS256", "k": r.Key,
		}},
	}
	if r.MaxClients > 0 {
		desc["max-clients"] = r.MaxClients
	}
	if r.AllowRecording {
		desc["allow-recording"] = true
	}
	if err := writeJSON(filepath.Join(dir, r.Name+".json"), desc); err != nil {
		return err
	}
	if reload {
		group.Update()
	}
	return nil
}

func (m *Manager) isRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// syncRoomFiles makes groups/ match the database: every room rendered,
// stale group files (from deleted rooms) removed. Runs at start.
func (m *Manager) syncRoomFiles(groupsDir string) error {
	keep := map[string]bool{TestRoomName + ".json": true}
	for _, r := range m.roomRows() {
		if err := m.writeRoomFile(r, false); err != nil {
			return err
		}
		keep[r.Name+".json"] = true
	}
	entries, _ := os.ReadDir(groupsDir)
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") && !keep[e.Name()] {
			_ = os.Remove(filepath.Join(groupsDir, e.Name()))
		}
	}
	return nil
}

// baseURL is where clients reach the SFU.
func (m *Manager) baseURL() string {
	cfg := m.loadConfig()
	if cfg == nil {
		return ""
	}
	if cfg.PublicHost != "" {
		return "https://" + cfg.PublicHost
	}
	return "http://" + cfg.ListenAddr
}

// Invite is a signed token plus the link that uses it.
type Invite struct {
	Room      string    `json:"room"`
	Role      string    `json:"role"`
	Username  string    `json:"username,omitempty"`
	Token     string    `json:"token"`
	JoinURL   string    `json:"join_url"`
	ExpiresAt time.Time `json:"expires_at"`
}

const maxTokenTTL = 30 * 24 * time.Hour

// IssueToken signs a token for room. An empty username lets the person
// choose a name when joining (invite links); platforms usually pin it.
// The token travels in the URL fragment, so it never reaches server logs
// or Referer headers.
func (m *Manager) IssueToken(roomName, username, role string, ttl time.Duration) (Invite, error) {
	r := m.findRoom(roomName)
	if r == nil {
		return Invite{}, fmt.Errorf("room %q not found", roomName)
	}
	return signToken(r, m.baseURL(), username, role, ttl)
}

// signToken builds the Galène token for room r, as reached at base.
func signToken(r *RTCRoomEntity, base, username, role string, ttl time.Duration) (Invite, error) {
	perms, err := rolePermissions(role, r.AllowRecording)
	if err != nil {
		return Invite{}, err
	}
	if role == "" {
		role = RolePresenter
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	if ttl > maxTokenTTL {
		return Invite{}, fmt.Errorf("token lifetime is capped at 30 days")
	}
	username = strings.TrimSpace(username)
	if len(username) > 64 {
		return Invite{}, fmt.Errorf("username is too long")
	}
	if !validUsername(username) {
		return Invite{}, fmt.Errorf("username cannot contain path separators like / or \\")
	}
	key, err := decodeKey(r.Key)
	if err != nil {
		return Invite{}, err
	}
	now := time.Now()
	exp := now.Add(ttl)
	claims := jwt.MapClaims{
		"iss":         "redock",
		"sub":         username,
		"aud":         []string{base + "/group/" + r.Name + "/"},
		"iat":         now.Unix(),
		"exp":         exp.Unix(),
		"permissions": perms,
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(key)
	if err != nil {
		return Invite{}, err
	}
	return Invite{
		Room: r.Name, Role: role, Username: username, Token: signed, ExpiresAt: exp,
		JoinURL: base + "/redock/join.html#room=" + r.Name + "&token=" + signed,
	}, nil
}

// ---------------------------------------------------------------------------
// API keys

// APIKeyView is listed in the dashboard; Key is only set on creation.
type APIKeyView struct {
	ID         uint       `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	Key        string     `json:"key,omitempty"`
}

func hashKey(k string) string {
	sum := sha256.Sum256([]byte(k))
	return hex.EncodeToString(sum[:])
}

func (m *Manager) apiKeyRows() []*RTCAPIKeyEntity {
	d := database.GetMemoryDB()
	if d == nil {
		return nil
	}
	return memory.FindAll[*RTCAPIKeyEntity](d, TableRTCAPIKeys)
}

func (m *Manager) APIKeys() []APIKeyView {
	rows := m.apiKeyRows()
	out := make([]APIKeyView, 0, len(rows))
	for _, k := range rows {
		out = append(out, APIKeyView{ID: k.ID, Name: k.Name, Prefix: k.Prefix, CreatedAt: k.CreatedAt, LastUsedAt: k.LastUsedAt})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

// CreateAPIKey returns the new key once; only its hash is stored.
func (m *Manager) CreateAPIKey(name string) (APIKeyView, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 64 {
		return APIKeyView{}, fmt.Errorf("give the key a name (max 64 characters)")
	}
	key := "rtk_" + randomSecret(32)
	e := &RTCAPIKeyEntity{Name: name, Prefix: key[:10], Hash: hashKey(key), CreatedAt: time.Now()}
	if err := memory.Create(database.GetMemoryDB(), TableRTCAPIKeys, e); err != nil {
		return APIKeyView{}, err
	}
	return APIKeyView{ID: e.ID, Name: e.Name, Prefix: e.Prefix, CreatedAt: e.CreatedAt, Key: key}, nil
}

func (m *Manager) DeleteAPIKey(id uint) error {
	return memory.Delete[*RTCAPIKeyEntity](database.GetMemoryDB(), TableRTCAPIKeys, id)
}

// CheckAPIKey reports whether key is valid (constant-time over the hash),
// records its use and returns the key's name (for the audit log).
func (m *Manager) CheckAPIKey(key string) (string, bool) {
	if !strings.HasPrefix(key, "rtk_") {
		return "", false
	}
	h := hashKey(key)
	for _, k := range m.apiKeyRows() {
		if subtle.ConstantTimeCompare([]byte(h), []byte(k.Hash)) == 1 {
			now := time.Now()
			k.LastUsedAt = &now
			_ = memory.Update(database.GetMemoryDB(), TableRTCAPIKeys, k)
			return k.Name, true
		}
	}
	return "", false
}
