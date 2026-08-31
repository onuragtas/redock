package email_server

import (
	"strings"
	"testing"
	"time"

	"redock/platform/memory"
)

// A log is read from the top, so the newest line has to be the first one. The
// raw view used to print the oldest first, which meant scrolling to the bottom
// of a thousand lines to see what had just happened.
func TestRawMailLogIsNewestFirst(t *testing.T) {
	m := newTestManager(t)

	// Write them out of order on purpose, so passing cannot be an accident of
	// the order they happened to be created in.
	base := time.Now().Add(-time.Hour)
	for _, subject := range []string{"middle", "oldest", "newest"} {
		m.logMailEvent(mailEvent{
			Direction: "out", Status: "sent", Service: "smtp",
			From: "alice@example.com", To: "bob@example.com", Subject: subject,
			Detail: subject,
		})
	}

	// storedLogEntries hands back copies, so the timestamps have to be set on
	// the stored records themselves.
	offsets := map[string]time.Duration{"oldest": 0, "middle": time.Minute, "newest": 2 * time.Minute}
	for _, row := range memory.FindAll[*EmailLog](m.db, "email_logs") {
		if offset, ok := offsets[row.StatusMessage]; ok {
			row.Timestamp = base.Add(offset)
		}
	}

	lines, source, err := m.GetRawMailLog(0)
	if err != nil {
		t.Fatalf("GetRawMailLog: %v", err)
	}
	if source != "native" {
		t.Errorf("source = %q, want %q", source, "native")
	}
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(lines))
	}

	for i, want := range []string{"newest", "middle", "oldest"} {
		if !strings.Contains(lines[i], want) {
			t.Errorf("line %d = %q, want it to be the %q entry", i, lines[i], want)
		}
	}
}

// The connection view is built from a ring buffer that is walked backwards.
// It already reads newest first; this keeps it that way.
func TestConnectionTracesAreNewestFirst(t *testing.T) {
	store := newTraceStore()

	base := time.Now()
	for i := 1; i <= 3; i++ {
		store.begin(&ConnectionTrace{
			ID: uint64(i), Service: "smtp", RemoteIP: "10.0.0.1",
			StartedAt: base.Add(time.Duration(i) * time.Minute),
		})
	}

	list := store.List(0)
	if len(list) != 3 {
		t.Fatalf("got %d traces, want 3", len(list))
	}
	for i, wantID := range []uint64{3, 2, 1} {
		if list[i].ID != wantID {
			t.Errorf("traces[%d].ID = %d, want %d", i, list[i].ID, wantID)
		}
	}
}
