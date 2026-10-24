package rtc_server

import (
	"fmt"
	"net"
	"path"
	"sort"
	"strings"
	"time"

	"redock/platform/database"
	"redock/platform/memory"

	"github.com/jech/galene/group"
)

const TableRTCAudit = "rtc_audit"

// RTCAuditEntity is one moderation / access record: who did what to which
// room and participant. Kept in the memory DB (persisted), newest-trimmed.
type RTCAuditEntity struct {
	memory.BaseEntity
	Time   time.Time `json:"time"`
	Actor  string    `json:"actor"` // Redock user email, or "api:<key name>"
	Action string    `json:"action"`
	Room   string    `json:"room,omitempty"`
	Target string    `json:"target,omitempty"`
	Detail string    `json:"detail,omitempty"`
}

const (
	auditKeep  = 5000
	auditSlack = 500 // trim in batches, not on every insert
)

// Audit records an action. It never fails the caller: losing an audit line
// is logged, but must not block moderation.
func (m *Manager) Audit(actor, action, room, target, detail string) {
	d := database.GetMemoryDB()
	if d == nil {
		return
	}
	_ = memory.Create(d, TableRTCAudit, &RTCAuditEntity{
		Time: time.Now(), Actor: actor, Action: action, Room: room, Target: target, Detail: detail,
	})
	rows := memory.FindAll[*RTCAuditEntity](d, TableRTCAudit)
	if len(rows) <= auditKeep+auditSlack {
		return
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Time.Before(rows[j].Time) })
	for _, r := range rows[:len(rows)-auditKeep] {
		_ = memory.Delete[*RTCAuditEntity](d, TableRTCAudit, r.ID)
	}
}

// AuditLog returns records newest first, optionally for one room.
func (m *Manager) AuditLog(room string, limit int) []RTCAuditEntity {
	d := database.GetMemoryDB()
	if d == nil {
		return nil
	}
	rows := memory.FindAll[*RTCAuditEntity](d, TableRTCAudit)
	sort.Slice(rows, func(i, j int) bool { return rows[i].Time.After(rows[j].Time) })
	out := make([]RTCAuditEntity, 0, min(limit, len(rows)))
	for _, r := range rows {
		if room != "" && r.Room != room {
			continue
		}
		out = append(out, *r)
		if len(out) >= limit {
			break
		}
	}
	return out
}

// validUsername mirrors Galène's check (no path tricks, no backslash) so a
// token with an unusable name is refused when issued, not when joining.
func validUsername(name string) bool {
	if name == "" {
		return true
	}
	if strings.ContainsRune(name, '\\') {
		return false
	}
	return path.Clean("/"+name) == "/"+name
}

// Participant is a client in a room as seen by the dashboard.
type Participant struct {
	ID          string   `json:"id"`
	Username    string   `json:"username"`
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"`
	IP          string   `json:"ip"`
	System      bool     `json:"system"` // e.g. the recorder
}

func roleOf(perms []string) string {
	has := func(p string) bool {
		for _, x := range perms {
			if x == p {
				return true
			}
		}
		return false
	}
	switch {
	case has("op"):
		return RoleModerator
	case has("present"):
		return RolePresenter
	case has("message"):
		return RoleViewer
	default:
		return RoleObserver
	}
}

func (m *Manager) liveGroup(room string) (*group.Group, error) {
	if !m.isRunning() {
		return nil, fmt.Errorf("the WebRTC server is not running")
	}
	g := group.Get(room)
	if g == nil {
		return nil, fmt.Errorf("room %q has no active session", room)
	}
	return g, nil
}

// Participants lists who is in room right now.
func (m *Manager) Participants(room string) ([]Participant, error) {
	if !m.isRunning() {
		return []Participant{}, nil
	}
	g := group.Get(room)
	if g == nil {
		return []Participant{}, nil
	}
	clients := g.GetClients(nil)
	out := make([]Participant, 0, len(clients))
	for _, c := range clients {
		p := Participant{ID: c.Id(), Username: c.Username(), Permissions: c.Permissions()}
		if p.Permissions == nil {
			p.Permissions = []string{}
		}
		p.Role = roleOf(p.Permissions)
		if a := c.Addr(); a != nil {
			if h, _, err := net.SplitHostPort(a.String()); err == nil {
				p.IP = h
			} else {
				p.IP = a.String()
			}
		} else {
			p.System = true // server-side clients (the disk recorder) have no address
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Username < out[j].Username })
	return out, nil
}

// Kick removes one participant; the reason is shown to them.
func (m *Manager) Kick(actor, room, id, reason string) error {
	g, err := m.liveGroup(room)
	if err != nil {
		return err
	}
	c := g.GetClient(id)
	if c == nil {
		return fmt.Errorf("participant not found (they may have left)")
	}
	if reason = strings.TrimSpace(reason); reason == "" {
		reason = "removed by a moderator"
	}
	by := "Redock"
	if err := c.Kick("", &by, reason); err != nil {
		return err
	}
	m.Audit(actor, "kick", room, c.Username(), reason)
	return nil
}

// KickAll ends the session for everyone in room (the room stays; invites
// remain valid unless the room is also locked or deleted).
func (m *Manager) KickAll(actor, room, reason string) (int, error) {
	g, err := m.liveGroup(room)
	if err != nil {
		return 0, err
	}
	if reason = strings.TrimSpace(reason); reason == "" {
		reason = "the session was ended by a moderator"
	}
	by := "Redock"
	n := 0
	for _, c := range g.GetClients(nil) {
		if c.Kick("", &by, reason) == nil {
			n++
		}
	}
	m.Audit(actor, "end-session", room, "", fmt.Sprintf("%d participants, %s", n, reason))
	return n, nil
}

// SetLock locks (no new participants) or unlocks room.
func (m *Manager) SetLock(actor, room string, locked bool, message string) error {
	g, err := m.liveGroup(room)
	if err != nil {
		return err
	}
	g.SetLocked(locked, strings.TrimSpace(message))
	action := "unlock"
	if locked {
		action = "lock"
	}
	m.Audit(actor, action, room, "", message)
	return nil
}

// ClearChat wipes the room's chat history for everyone.
func (m *Manager) ClearChat(actor, room string) error {
	g, err := m.liveGroup(room)
	if err != nil {
		return err
	}
	g.ClearChatHistory("", "")
	m.Audit(actor, "clear-chat", room, "", "")
	return nil
}

// ModeratorJoin issues a short-lived moderator token for a Redock user. The
// name is pinned and marked, so the moderator is visible to everyone in the
// room as such — moderation is never silent.
func (m *Manager) ModeratorJoin(actor, room string) (Invite, error) {
	name := actor
	if i := strings.IndexByte(name, '@'); i > 0 {
		name = name[:i]
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "admin"
	}
	inv, err := m.IssueToken(room, name+" (Redock)", RoleModerator, 2*time.Hour)
	if err != nil {
		return Invite{}, err
	}
	m.Audit(actor, "moderator-join", room, inv.Username, "visible moderator link, 2h")
	return inv, nil
}
