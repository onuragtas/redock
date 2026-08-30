package email_server

import (
	"testing"
)

// A slice of a real docker-mailserver log: one inbound delivery, one outbound
// delivery, a rejection, a deferral, an IMAP login and noise that must not be
// mistaken for a message.
var sampleLog = []string{
	"Feb 12 10:23:45 mail postfix/smtpd[123]: connect from mail-1.example.com[203.0.113.5]",
	"Feb 12 10:23:46 mail postfix/smtpd[123]: A1B2C3D4E5: client=mail-1.example.com[203.0.113.5]",
	"Feb 12 10:23:46 mail postfix/cleanup[130]: A1B2C3D4E5: message-id=<abc123@example.com>",
	"Feb 12 10:23:46 mail postfix/qmgr[95]: A1B2C3D4E5: from=<sender@example.com>, size=2345, nrcpt=1 (queue active)",
	"Feb 12 10:23:47 mail postfix/lmtp[140]: A1B2C3D4E5: to=<user@mydomain.com>, relay=mail.mydomain.com[/var/run/dovecot/lmtp], delay=1.2, delays=0.5/0/0/0.7, dsn=2.0.0, status=sent (250 2.0.0 <user@mydomain.com> nZ1 Saved)",

	"Feb 12 11:00:01 mail postfix/smtpd[200]: BBCC112233: client=localhost[127.0.0.1]",
	"Feb 12 11:00:01 mail postfix/qmgr[95]: BBCC112233: from=<me@mydomain.com>, size=1200, nrcpt=1 (queue active)",
	"Feb 12 11:00:03 mail postfix/smtp[210]: BBCC112233: to=<friend@gmail.com>, relay=gmail-smtp-in.l.google.com[142.250.1.27]:25, delay=2.1, dsn=2.0.0, status=sent (250 2.0.0 OK)",

	"Feb 12 12:00:00 mail postfix/smtpd[300]: NOQUEUE: reject: RCPT from unknown[198.51.100.9]: 554 5.7.1 <spam@x.com>: Relay access denied; from=<spam@x.com> to=<victim@other.com> proto=ESMTP helo=<x>",

	"Feb 12 12:10:00 mail postfix/smtp[500]: DDEE449900: to=<slow@slow.test>, relay=none, delay=30, dsn=4.4.1, status=deferred (connect to slow.test[192.0.2.7]:25: Connection timed out)",

	"Feb 12 12:05:00 mail dovecot: imap-login: Login: user=<user@mydomain.com>, method=PLAIN, rip=192.168.1.10, lip=172.17.0.2, mpid=400, TLS",

	"Feb 12 12:06:00 mail postfix/smtpd[301]: warning: hostname does not resolve to address 198.51.100.9",
	"Feb 12 12:07:00 mail postfix/anvil[310]: statistics: max connection rate 1/60s for (smtp:203.0.113.5) at Feb 12 10:23:45",
}

func findEntry(t *testing.T, entries []MailLogEntry, queueID string) MailLogEntry {
	t.Helper()
	for _, e := range entries {
		if e.QueueID == queueID {
			return e
		}
	}
	t.Fatalf("no entry for queue %s in %d entries", queueID, len(entries))
	return MailLogEntry{}
}

func TestParseMailLogIncomingDelivery(t *testing.T) {
	entries := parseMailLog(sampleLog)
	e := findEntry(t, entries, "A1B2C3D4E5")

	if e.Direction != "in" {
		t.Errorf("expected an inbound message, got %q", e.Direction)
	}
	if e.From != "sender@example.com" {
		t.Errorf("unexpected sender: %q", e.From)
	}
	if len(e.To) != 1 || e.To[0] != "user@mydomain.com" {
		t.Errorf("unexpected recipients: %v", e.To)
	}
	if e.Status != "sent" {
		t.Errorf("unexpected status: %q", e.Status)
	}
	if e.Size != 2345 {
		t.Errorf("unexpected size: %d", e.Size)
	}
	if e.MessageID != "abc123@example.com" {
		t.Errorf("unexpected message id: %q", e.MessageID)
	}
	if e.RemoteIP != "203.0.113.5" || e.RemoteHost != "mail-1.example.com" {
		t.Errorf("unexpected remote: %s / %s", e.RemoteHost, e.RemoteIP)
	}
}

func TestParseMailLogOutgoingDelivery(t *testing.T) {
	entries := parseMailLog(sampleLog)
	e := findEntry(t, entries, "BBCC112233")

	if e.Direction != "out" {
		t.Errorf("expected an outbound message (submitted from localhost), got %q", e.Direction)
	}
	if e.From != "me@mydomain.com" || len(e.To) != 1 || e.To[0] != "friend@gmail.com" {
		t.Errorf("unexpected addresses: from=%q to=%v", e.From, e.To)
	}
	if e.Status != "sent" {
		t.Errorf("unexpected status: %q", e.Status)
	}
}

func TestParseMailLogRejection(t *testing.T) {
	entries := parseMailLog(sampleLog)

	var found bool
	for _, e := range entries {
		if e.Status != "rejected" {
			continue
		}
		found = true
		if e.Direction != "in" {
			t.Errorf("a rejected inbound attempt must be inbound, got %q", e.Direction)
		}
		if e.RemoteIP != "198.51.100.9" {
			t.Errorf("unexpected remote ip: %q", e.RemoteIP)
		}
		if e.From != "spam@x.com" || len(e.To) != 1 || e.To[0] != "victim@other.com" {
			t.Errorf("unexpected addresses: from=%q to=%v", e.From, e.To)
		}
		if e.Detail == "" {
			t.Error("expected the rejection reason to be captured")
		}
	}
	if !found {
		t.Fatal("the NOQUEUE rejection was not parsed")
	}
}

func TestParseMailLogDeferral(t *testing.T) {
	entries := parseMailLog(sampleLog)
	e := findEntry(t, entries, "DDEE449900")

	if e.Status != "deferred" {
		t.Errorf("unexpected status: %q", e.Status)
	}
	if e.Direction != "out" {
		t.Errorf("a delivery attempt to an external relay is outbound, got %q", e.Direction)
	}
	if e.Detail == "" {
		t.Error("expected the deferral reason to be captured")
	}
}

func TestParseMailLogDovecotLogin(t *testing.T) {
	entries := parseMailLog(sampleLog)

	for _, e := range entries {
		if e.Status == "login" {
			if e.Direction != "system" {
				t.Errorf("a login is a system event, got %q", e.Direction)
			}
			if e.From != "user@mydomain.com" || e.RemoteIP != "192.168.1.10" {
				t.Errorf("unexpected login details: user=%q ip=%q", e.From, e.RemoteIP)
			}
			return
		}
	}
	t.Fatal("the IMAP login was not parsed")
}

func TestParseMailLogIgnoresNonMessageLines(t *testing.T) {
	entries := parseMailLog(sampleLog)

	for _, e := range entries {
		if e.QueueID == "warning" || e.QueueID == "statistics" || e.QueueID == "NOQUEUE" {
			t.Fatalf("a keyword line was mistaken for a message: %+v", e)
		}
	}

	// Exactly three queued messages appear in the sample.
	queued := 0
	for _, e := range entries {
		if e.QueueID != "" {
			queued++
		}
	}
	if queued != 3 {
		t.Fatalf("expected 3 queued messages, got %d", queued)
	}
}

func TestSplitQueueLine(t *testing.T) {
	cases := []struct {
		message string
		id      string
		ok      bool
	}{
		{"A1B2C3D4E5: client=x[1.2.3.4]", "A1B2C3D4E5", true},
		{"4XyZ12abcd: from=<a@b>, size=1", "4XyZ12abcd", true}, // long queue IDs
		{"warning: hostname does not resolve", "", false},
		{"statistics: max connection rate", "", false},
		{"NOQUEUE: reject: RCPT from x", "", false},
		{"connect from mail.example.com[1.2.3.4]", "", false},
	}

	for _, tc := range cases {
		id, _, ok := splitQueueLine(tc.message)
		if ok != tc.ok || id != tc.id {
			t.Errorf("splitQueueLine(%q) = (%q, %v), want (%q, %v)", tc.message, id, ok, tc.id, tc.ok)
		}
	}
}

func TestMatchesQuery(t *testing.T) {
	entry := MailLogEntry{
		Direction: "in",
		Status:    "sent",
		From:      "sender@example.com",
		To:        []string{"user@mydomain.com"},
		Detail:    "250 OK",
	}

	if !matchesQuery(entry, MailLogQuery{}) {
		t.Error("an empty query must match everything")
	}
	if matchesQuery(entry, MailLogQuery{Direction: "out"}) {
		t.Error("direction filter did not apply")
	}
	if matchesQuery(entry, MailLogQuery{Status: "bounced"}) {
		t.Error("status filter did not apply")
	}
	if !matchesQuery(entry, MailLogQuery{Search: "MYDOMAIN"}) {
		t.Error("search must be case-insensitive and cover recipients")
	}
	if matchesQuery(entry, MailLogQuery{Search: "nothing-here"}) {
		t.Error("search matched an absent term")
	}
}

func TestParseMailLogHandlesDockerTimestampPrefix(t *testing.T) {
	lines := []string{
		"2026-02-12T10:23:46.123456789Z Feb 12 10:23:46 mail postfix/qmgr[95]: A1B2C3D4E5: from=<sender@example.com>, size=99, nrcpt=1 (queue active)",
	}

	entries := parseMailLog(lines)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].From != "sender@example.com" || entries[0].Size != 99 {
		t.Fatalf("unexpected entry: %+v", entries[0])
	}
	if entries[0].Timestamp.Year() != 2026 {
		t.Fatalf("expected the year from the Docker timestamp, got %d", entries[0].Timestamp.Year())
	}
}
