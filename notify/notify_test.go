package notify

import (
	"errors"
	"strings"
	"testing"
	"time"
)

type recorder struct {
	sent []string
	err  error
}

func (r *recorder) send(mailboxID uint, to, subject, body string) error {
	if r.err != nil {
		return r.err
	}
	r.sent = append(r.sent, subject)
	return nil
}

func testSetup(t *testing.T) (*Settings, *recorder) {
	t.Helper()
	settings := DefaultSettings()
	settings.Enabled = true
	settings.MailboxID = 1
	settings.Recipient = "ops@example.com"
	return settings, &recorder{}
}

// An alert marks a change. A problem that is still true must not send the same
// mail on every check, or the first real outage buries the inbox.
func TestAProblemIsReportedOnceUntilItChanges(t *testing.T) {
	settings, rec := testSetup(t)
	days := 3
	n := New(settings, Sources{
		CertificateDaysLeft: func() (int, bool) { return days, true },
	}, rec.send)

	if raised := n.Check(); len(raised) != 1 {
		t.Fatalf("first check raised %d alerts, want 1", len(raised))
	}
	if len(rec.sent) != 1 {
		t.Fatalf("sent %d mails, want 1", len(rec.sent))
	}

	// Same state, checked again and again.
	for i := 0; i < 5; i++ {
		if raised := n.Check(); len(raised) != 0 {
			t.Fatalf("a repeat check raised %d alerts, want none", len(raised))
		}
	}
	if len(rec.sent) != 1 {
		t.Errorf("sent %d mails for one unchanged problem, want 1", len(rec.sent))
	}
}

// When it clears, say so once — otherwise nobody knows whether to keep worrying.
func TestRecoveryIsReportedOnce(t *testing.T) {
	settings, rec := testSetup(t)
	days := 3
	n := New(settings, Sources{
		CertificateDaysLeft: func() (int, bool) { return days, true },
	}, rec.send)

	n.Check()
	days = 90

	raised := n.Check()
	if len(raised) != 1 || raised[0].Level != LevelOK {
		t.Fatalf("recovery was not reported: %+v", raised)
	}
	if len(n.Check()) != 0 {
		t.Error("recovery was reported twice")
	}
}

// A problem that lasts is worth a reminder, but only after the quiet period.
func TestAStandingProblemRepeatsAfterTheQuietPeriod(t *testing.T) {
	settings, rec := testSetup(t)
	settings.RepeatHours = 1
	n := New(settings, Sources{
		CertificateDaysLeft: func() (int, bool) { return 3, true },
	}, rec.send)

	n.Check()
	if len(rec.sent) != 1 {
		t.Fatalf("sent %d, want 1", len(rec.sent))
	}

	// Pretend the quiet period has passed.
	n.mu.Lock()
	n.states["certificate"] = state{level: LevelWarning, lastSent: time.Now().Add(-2 * time.Hour)}
	n.mu.Unlock()

	if raised := n.Check(); len(raised) != 1 {
		t.Fatalf("the standing problem was not repeated: %+v", raised)
	}
}

// Escalation is a change, so it goes out immediately rather than waiting.
func TestGettingWorseIsReportedImmediately(t *testing.T) {
	settings, rec := testSetup(t)
	days := 3
	n := New(settings, Sources{
		CertificateDaysLeft: func() (int, bool) { return days, true },
	}, rec.send)

	n.Check()
	days = 0 // expired

	raised := n.Check()
	if len(raised) != 1 || raised[0].Level != LevelCritical {
		t.Fatalf("escalation was not reported: %+v", raised)
	}
}

// An alert nobody could deliver still has to be recorded, or the page would
// show nothing wrong while the server was on fire.
func TestAnUndeliverableAlertIsStillRecorded(t *testing.T) {
	settings, rec := testSetup(t)
	rec.err = errors.New("connection refused")
	n := New(settings, Sources{
		CertificateDaysLeft: func() (int, bool) { return 1, true },
	}, rec.send)

	raised := n.Check()
	if len(raised) != 1 {
		t.Fatalf("raised %d alerts, want 1", len(raised))
	}
	if raised[0].Sent {
		t.Error("the alert claims it was sent")
	}
	if !strings.Contains(raised[0].SendError, "connection refused") {
		t.Errorf("the failure reason was lost: %q", raised[0].SendError)
	}
	if len(n.Recent()) != 1 {
		t.Error("the alert is not in the recent list")
	}
}

// Turning a watch off means silence from it, whatever the state.
func TestADisabledWatchSaysNothing(t *testing.T) {
	settings, rec := testSetup(t)
	settings.WatchCertificate = false
	n := New(settings, Sources{
		CertificateDaysLeft: func() (int, bool) { return 0, true },
	}, rec.send)

	if raised := n.Check(); len(raised) != 0 {
		t.Errorf("a disabled watch raised %+v", raised)
	}
}

// A source that was never wired must not be treated as reporting zero, which
// would read as "the certificate expired" rather than "nobody asked".
func TestAMissingSourceIsNotAProblem(t *testing.T) {
	settings, rec := testSetup(t)
	n := New(settings, Sources{}, rec.send)

	if raised := n.Check(); len(raised) != 0 {
		t.Errorf("an unwired source raised %+v", raised)
	}
}

func TestThresholdsDecideWhatCounts(t *testing.T) {
	settings, rec := testSetup(t)
	settings.WatchCertificate = false
	settings.QueueThreshold = 10

	stuck := 9
	n := New(settings, Sources{QueueStuck: func() int { return stuck }}, rec.send)

	if raised := n.Check(); len(raised) != 0 {
		t.Fatalf("below the threshold raised %+v", raised)
	}
	stuck = 10
	if raised := n.Check(); len(raised) != 1 {
		t.Fatalf("at the threshold raised %d alerts, want 1", len(raised))
	}
}

// Settings with nothing filled in must not pretend to send.
func TestNothingIsSentWithoutARecipient(t *testing.T) {
	settings := DefaultSettings()
	settings.Enabled = true
	rec := &recorder{}
	n := New(settings, Sources{CertificateDaysLeft: func() (int, bool) { return 1, true }}, rec.send)

	raised := n.Check()
	if len(raised) != 1 {
		t.Fatalf("raised %d, want 1", len(raised))
	}
	if raised[0].Sent || len(rec.sent) != 0 {
		t.Error("an alert was sent with no recipient configured")
	}
	if raised[0].SendError == "" {
		t.Error("the alert does not say why it was not sent")
	}
}

func TestSendTestRefusesUntilConfigured(t *testing.T) {
	settings := DefaultSettings()
	rec := &recorder{}
	n := New(settings, Sources{}, rec.send)

	if err := n.SendTest(); err == nil {
		t.Fatal("a test was accepted with nothing configured")
	}

	settings.MailboxID = 1
	settings.Recipient = "ops@example.com"
	if err := n.SendTest(); err != nil {
		t.Fatalf("SendTest: %v", err)
	}
	if len(rec.sent) != 1 {
		t.Errorf("sent %d test mails, want 1", len(rec.sent))
	}
}

// The refusal has to name what is missing: the operator is looking at a form
// with the other half filled in, and "choose both" sends them hunting.
func TestSendTestSaysWhichHalfIsMissing(t *testing.T) {
	tests := []struct {
		name      string
		mailboxID uint
		recipient string
		want      string
	}{
		{"neither", 0, "", "mailbox to send from and an address"},
		{"no mailbox", 0, "ops@example.com", "mailbox to send from first"},
		{"no recipient", 1, "", "address to send to first"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			settings := DefaultSettings()
			settings.MailboxID = tc.mailboxID
			settings.Recipient = tc.recipient

			err := New(settings, Sources{}, (&recorder{}).send).SendTest()
			if err == nil {
				t.Fatal("the test was accepted with something missing")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}
