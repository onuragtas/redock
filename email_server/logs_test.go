package email_server

import (
	"testing"
	"time"
)

// The events view is the complete record: message deliveries and system events
// alike. A filter has to be asked for explicitly.
func TestGetMailLogsReturnsEveryKindOfEvent(t *testing.T) {
	m := newTestManager(t)
	seedDomain(t, m, "example.com", "alice", "secret")

	events := []mailEvent{
		{Direction: "in", Status: "delivered", From: "sender@outside.test", To: "alice@example.com", Service: "smtp"},
		{Direction: "out", Status: "queued", From: "alice@example.com", To: "bob@far.test", Service: "submission"},
		{Direction: "system", Status: "connect", RemoteIP: "203.0.113.5", Service: "smtp"},
		{Direction: "system", Status: "auth-failed", From: "alice@example.com", Service: "imap"},
		{Direction: "system", Status: "tls-handshake", RemoteIP: "203.0.113.5", Service: "tls"},
	}
	for _, event := range events {
		m.logMailEvent(event)
	}

	result, err := m.GetMailLogs(MailLogQuery{})
	if err != nil {
		t.Fatalf("GetMailLogs: %v", err)
	}

	if len(result.Entries) != len(events) {
		t.Fatalf("expected every event to be listed, got %d of %d", len(result.Entries), len(events))
	}
	if result.Total != len(events) {
		t.Errorf("Total should count everything retained, got %d", result.Total)
	}

	seen := map[string]bool{}
	for _, entry := range result.Entries {
		seen[entry.Status] = true
	}
	for _, want := range []string{"delivered", "queued", "connect", "auth-failed", "tls-handshake"} {
		if !seen[want] {
			t.Errorf("%q is missing from the events view: %+v", want, result.Entries)
		}
	}
}

func TestGetMailLogsHonoursTheLimitAndReportsTheTotal(t *testing.T) {
	m := newTestManager(t)

	for i := 0; i < 10; i++ {
		m.logMailEvent(mailEvent{Direction: "system", Status: "connect", Service: "smtp"})
		time.Sleep(time.Millisecond)
	}

	result, err := m.GetMailLogs(MailLogQuery{Limit: 3})
	if err != nil {
		t.Fatalf("GetMailLogs: %v", err)
	}

	if len(result.Entries) != 3 {
		t.Fatalf("the limit was ignored: got %d entries", len(result.Entries))
	}
	if result.Total != 10 {
		t.Errorf("the total must count everything retained, not the page: got %d", result.Total)
	}

	// Newest first, so the page is the tail of the log.
	if !result.Entries[0].Timestamp.After(result.Entries[2].Timestamp) &&
		!result.Entries[0].Timestamp.Equal(result.Entries[2].Timestamp) {
		t.Error("entries should come back newest first")
	}
}

func TestGetMailLogsFiltersDoNotHideTheTotal(t *testing.T) {
	m := newTestManager(t)

	m.logMailEvent(mailEvent{Direction: "in", Status: "delivered", Service: "smtp"})
	m.logMailEvent(mailEvent{Direction: "system", Status: "connect", Service: "smtp"})

	result, err := m.GetMailLogs(MailLogQuery{Direction: "in"})
	if err != nil {
		t.Fatalf("GetMailLogs: %v", err)
	}

	if len(result.Entries) != 1 {
		t.Fatalf("the direction filter did not apply: %+v", result.Entries)
	}
	if result.Total != 2 {
		t.Errorf("filtering must not change how much is retained: got %d", result.Total)
	}
	if result.Stats.Incoming != 1 || result.Stats.Outgoing != 0 {
		t.Errorf("stats are computed over everything retained: %+v", result.Stats)
	}
}
