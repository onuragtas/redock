package rtc_server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jech/galene/diskwriter"
	"github.com/jech/galene/group"
)

// DefaultRetentionDays is how long recordings are kept unless configured.
const DefaultRetentionDays = 30

// Recording is one file written by Galène's disk recorder. Galène records
// each participant's stream to its own file (audio+video WebM, or Ogg for
// audio only); there is no server-side composite.
type Recording struct {
	Room     string    `json:"room"`
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
	Active   bool      `json:"active"` // still being written
}

func (m *Manager) recordingsDir() string { return filepath.Join(m.baseDir, "recordings") }

// recorder returns the room's active disk recorder, if any.
func recorder(g *group.Group) *diskwriter.Client {
	for _, c := range g.GetClients(nil) {
		if d, ok := c.(*diskwriter.Client); ok {
			return d
		}
	}
	return nil
}

// IsRecording reports whether room is being recorded right now.
func (m *Manager) IsRecording(room string) bool {
	if !m.isRunning() {
		return false
	}
	g := group.Get(room)
	return g != nil && recorder(g) != nil
}

// StartRecording starts the server-side recorder in room. Only rooms that
// allow recording can be recorded, and the recorder appears in the room as
// "RECORDING", so every participant sees it (consent, never covert).
func (m *Manager) StartRecording(actor, room string) error {
	r := m.findRoom(room)
	if r == nil {
		return fmt.Errorf("room %q not found", room)
	}
	if !r.AllowRecording {
		return fmt.Errorf("recording is not allowed in this room (enable it in the room settings)")
	}
	return m.startRecorder(actor, room)
}

// startRecorder adds Galène's disk recorder to a live room.
func (m *Manager) startRecorder(actor, room string) error {
	g, err := m.liveGroup(room)
	if err != nil {
		return err
	}
	if recorder(g) != nil {
		return fmt.Errorf("already recording")
	}
	disk, err := diskwriter.New(g)
	if err != nil {
		return err
	}
	if _, err := group.AddClient(g.Name(), disk, group.ClientCredentials{System: true}); err != nil {
		disk.Close()
		return err
	}
	// Have every participant push their current streams to the recorder
	// (what Galène's own "record" action does).
	for _, c := range g.GetClients(disk) {
		_ = c.RequestConns(disk, g, "")
	}
	m.Audit(actor, "record-start", room, "", "")
	return nil
}

// StopRecording stops room's recorder; the files are finalised.
func (m *Manager) StopRecording(actor, room string) error {
	g, err := m.liveGroup(room)
	if err != nil {
		return err
	}
	d := recorder(g)
	if d == nil {
		return fmt.Errorf("not recording")
	}
	d.Close()
	group.DelClient(d)
	m.Audit(actor, "record-stop", room, "", "")
	return nil
}

// Recordings lists recorded files, newest first (room "" = all rooms).
func (m *Manager) Recordings(room string) ([]Recording, error) {
	base := m.recordingsDir()
	rooms := []string{room}
	if room == "" {
		entries, err := os.ReadDir(base)
		if err != nil {
			if os.IsNotExist(err) {
				return []Recording{}, nil
			}
			return nil, err
		}
		rooms = rooms[:0]
		for _, e := range entries {
			if e.IsDir() {
				rooms = append(rooms, e.Name())
			}
		}
	} else if !roomNamePattern.MatchString(room) {
		return nil, fmt.Errorf("invalid room name")
	}

	out := []Recording{}
	for _, rm := range rooms {
		entries, err := os.ReadDir(filepath.Join(base, rm))
		if err != nil {
			continue
		}
		active := m.IsRecording(rm)
		for _, e := range entries {
			info, err := e.Info()
			if err != nil || e.IsDir() {
				continue
			}
			out = append(out, Recording{
				Room: rm, Name: e.Name(), Size: info.Size(), Modified: info.ModTime(),
				// A file touched in the last seconds of an active recording is
				// still open; downloading it would give a truncated file.
				Active: active && time.Since(info.ModTime()) < 10*time.Second,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Modified.After(out[j].Modified) })
	return out, nil
}

// RecordingPath resolves a recording to its file, refusing anything that is
// not a plain file name inside a room directory.
func (m *Manager) RecordingPath(room, name string) (string, error) {
	if !roomNamePattern.MatchString(room) && room != TestRoomName {
		return "", fmt.Errorf("invalid room name")
	}
	if name == "" || strings.ContainsAny(name, `/\`) || strings.HasPrefix(name, ".") {
		return "", fmt.Errorf("invalid file name")
	}
	p := filepath.Join(m.recordingsDir(), room, name)
	info, err := os.Stat(p)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("recording not found")
	}
	return p, nil
}

// DeleteRecording removes one file.
func (m *Manager) DeleteRecording(actor, room, name string) error {
	p, err := m.RecordingPath(room, name)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil {
		return err
	}
	m.Audit(actor, "recording-deleted", room, name, "")
	return nil
}

// applyRetention deletes recordings older than the configured number of
// days (0 keeps them forever). Files of an active recording are kept.
func (m *Manager) applyRetention() {
	if cfg := m.loadConfig(); cfg != nil {
		m.applyRetentionDays(cfg.RetentionDays)
	}
}

func (m *Manager) applyRetentionDays(days int) {
	if days <= 0 {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	recs, err := m.Recordings("")
	if err != nil {
		return
	}
	removed := 0
	for _, r := range recs {
		if r.Active || r.Modified.After(cutoff) {
			continue
		}
		if p, err := m.RecordingPath(r.Room, r.Name); err == nil && os.Remove(p) == nil {
			removed++
		}
	}
	if removed > 0 {
		m.Audit("system", "retention-cleanup", "", "", fmt.Sprintf("%d recordings older than %d days deleted", removed, days))
	}
}

// RecordingUsage is the total size of stored recordings.
func (m *Manager) RecordingUsage() int64 {
	recs, _ := m.Recordings("")
	var n int64
	for _, r := range recs {
		n += r.Size
	}
	return n
}

// Download links: the dashboard authenticates with a JWT in a header, which a
// plain browser download can't send. An admin request mints a short-lived
// signed URL instead, so large files stream straight to disk.
var downloadKey = []byte(randomSecret(32)) // per process: links die on restart

const downloadLinkTTL = 5 * time.Minute

func downloadSig(room, name string, exp int64) string {
	mac := hmac.New(sha256.New, downloadKey)
	fmt.Fprintf(mac, "%s\x00%s\x00%d", room, name, exp)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// DownloadLink returns a signed, short-lived URL path for a recording.
func (m *Manager) DownloadLink(actor, room, name string) (string, error) {
	if _, err := m.RecordingPath(room, name); err != nil {
		return "", err
	}
	exp := time.Now().Add(downloadLinkTTL).Unix()
	q := url.Values{
		"room": {room}, "name": {name},
		"exp": {strconv.FormatInt(exp, 10)}, "sig": {downloadSig(room, name, exp)},
	}
	m.Audit(actor, "recording-download", room, name, "")
	return "/api/v1/rtc-recording?" + q.Encode(), nil
}

// VerifyDownload checks a signed link and returns the file path.
func (m *Manager) VerifyDownload(room, name, expStr, sig string) (string, error) {
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return "", fmt.Errorf("link expired")
	}
	if !hmac.Equal([]byte(sig), []byte(downloadSig(room, name, exp))) {
		return "", fmt.Errorf("invalid link")
	}
	return m.RecordingPath(room, name)
}
