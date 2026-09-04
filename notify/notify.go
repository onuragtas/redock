// Package notify tells the operator when something needs attention.
//
// Everything this platform runs already reports its state somewhere: the
// certificate knows when it expires, the outbound queue knows what is stuck,
// the memory guard knows it is shedding load, the mail guard knows who it is
// refusing. All of it is visible — to somebody who happens to be looking at the
// dashboard. Nobody watches a dashboard at three in the morning.
//
// So this reads those same states on a timer and sends mail when one of them
// turns bad. It sends through the server's own mailboxes, which means the alert
// is signed by the domain it comes from and is not itself treated as spam.
package notify

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// Level says how much a finding matters.
const (
	LevelOK       = "ok"
	LevelWarning  = "warning"
	LevelCritical = "critical"
)

// Alert is one thing worth telling the operator about.
type Alert struct {
	// Key identifies what the alert is about, not when it happened, so the same
	// problem seen twice is recognised as the same problem.
	Key    string    `json:"key"`
	Level  string    `json:"level"`
	Title  string    `json:"title"`
	Detail string    `json:"detail"`
	At     time.Time `json:"at"`
	// Sent records whether the mail actually went out. An alert nobody could
	// deliver still belongs in the list, or the page would claim all is well.
	Sent      bool   `json:"sent"`
	SendError string `json:"send_error,omitempty"`
}

// Sender delivers one alert. Kept as a function so the notifier does not need
// to know about the mail server, and so tests can watch what it was asked to
// send without a mail server existing.
type Sender func(mailboxID uint, to, subject, body string) error

// Sources are the states this package watches. They are injected rather than
// imported: every one of them lives in a different subsystem, and reaching into
// all of them from here would tie those subsystems together through this one.
type Sources struct {
	// CertificateDaysLeft reports the days of certificate life remaining, and
	// whether a certificate could be read at all.
	CertificateDaysLeft func() (int, bool)
	// QueueStuck reports how many outbound messages have failed at least once.
	QueueStuck func() int
	// MemoryLevel reports the memory guard's current level as a plain string.
	MemoryLevel func() string
	// BlockedClients reports how many addresses are currently refused.
	BlockedClients func() int
}

// state remembers what was last seen for one key, so a problem that persists
// does not send the same mail on every check.
type state struct {
	level    string
	lastSent time.Time
}

// Notifier watches the injected sources and sends what changes.
type Notifier struct {
	mu sync.Mutex

	settings *Settings
	sources  Sources
	send     Sender

	states map[string]state
	recent []Alert
}

// maxRecent bounds the list shown on the page.
const maxRecent = 50

// New creates a notifier. The settings pointer is read on each check, so a
// change made in the dashboard takes effect without a restart.
func New(settings *Settings, sources Sources, send Sender) *Notifier {
	return &Notifier{
		settings: settings,
		sources:  sources,
		send:     send,
		states:   make(map[string]state),
	}
}

// SetSettings replaces the settings the notifier reads.
func (n *Notifier) SetSettings(settings *Settings) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.settings = settings
}

// Recent returns the alerts raised so far, newest first.
func (n *Notifier) Recent() []Alert {
	n.mu.Lock()
	defer n.mu.Unlock()

	out := make([]Alert, len(n.recent))
	for i, alert := range n.recent {
		out[len(n.recent)-1-i] = alert
	}
	return out
}

// Check reads every watched state once and delivers what changed. It returns
// the alerts it raised, which is what the tests and the dashboard read.
func (n *Notifier) Check() []Alert {
	n.mu.Lock()
	settings := n.settings
	n.mu.Unlock()

	if settings == nil {
		return nil
	}
	settings.normalize()

	findings := n.evaluate(settings)

	var raised []Alert
	for _, finding := range findings {
		if alert, ok := n.consider(settings, finding); ok {
			raised = append(raised, alert)
		}
	}
	return raised
}

// evaluate turns the watched states into findings, one per key, each either OK
// or not. Every watch reports on every check so recovery can be noticed too.
func (n *Notifier) evaluate(settings *Settings) []Alert {
	var findings []Alert
	now := time.Now()

	if settings.WatchCertificate && n.sources.CertificateDaysLeft != nil {
		days, ok := n.sources.CertificateDaysLeft()
		switch {
		case !ok:
			findings = append(findings, Alert{
				Key: "certificate", Level: LevelWarning, At: now,
				Title:  "The TLS certificate could not be read",
				Detail: "Mail clients will refuse the connection until this is fixed.",
			})
		case days <= 0:
			findings = append(findings, Alert{
				Key: "certificate", Level: LevelCritical, At: now,
				Title:  "The TLS certificate has expired",
				Detail: "Clients are already refusing to connect.",
			})
		case days <= settings.CertDaysBefore:
			findings = append(findings, Alert{
				Key: "certificate", Level: LevelWarning, At: now,
				Title:  fmt.Sprintf("The TLS certificate expires in %d days", days),
				Detail: "Renew it before clients start refusing the connection.",
			})
		default:
			findings = append(findings, Alert{
				Key: "certificate", Level: LevelOK, At: now,
				Title: fmt.Sprintf("The TLS certificate is valid for another %d days", days),
			})
		}
	}

	if settings.WatchQueue && n.sources.QueueStuck != nil {
		stuck := n.sources.QueueStuck()
		if stuck >= settings.QueueThreshold {
			findings = append(findings, Alert{
				Key: "queue", Level: LevelWarning, At: now,
				Title:  fmt.Sprintf("%d messages are stuck in the outbound queue", stuck),
				Detail: "They have each failed at least once. Check the queue for the reason the far side gave.",
			})
		} else {
			findings = append(findings, Alert{
				Key: "queue", Level: LevelOK, At: now,
				Title: "The outbound queue is moving again",
			})
		}
	}

	if settings.WatchMemory && n.sources.MemoryLevel != nil {
		level := strings.ToLower(n.sources.MemoryLevel())
		switch level {
		case "critical", "emergency":
			findings = append(findings, Alert{
				Key: "memory", Level: LevelCritical, At: now,
				Title:  "Memory pressure is " + level,
				Detail: "The memory guard is releasing caches and logs to stay alive.",
			})
		case "warning":
			findings = append(findings, Alert{
				Key: "memory", Level: LevelWarning, At: now,
				Title: "Memory pressure is rising",
			})
		default:
			findings = append(findings, Alert{
				Key: "memory", Level: LevelOK, At: now,
				Title: "Memory use is back to normal",
			})
		}
	}

	if settings.WatchBlocked && n.sources.BlockedClients != nil {
		blocked := n.sources.BlockedClients()
		if blocked >= settings.BlockedThreshold {
			findings = append(findings, Alert{
				Key: "blocked", Level: LevelWarning, At: now,
				Title:  fmt.Sprintf("%d clients are being refused", blocked),
				Detail: "That is more than usual; somebody may be working through passwords or probing for a relay.",
			})
		} else {
			findings = append(findings, Alert{
				Key: "blocked", Level: LevelOK, At: now,
				Title: "The number of refused clients is back to normal",
			})
		}
	}

	return findings
}

// consider decides whether a finding is worth sending, and sends it.
//
// The rule is that a notification marks a *change*. A problem that has already
// been reported is repeated only after the quiet period, and a recovery is
// reported once — so a week-long outage is one mail and a reminder a day, not a
// mail every minute.
func (n *Notifier) consider(settings *Settings, finding Alert) (Alert, bool) {
	n.mu.Lock()
	previous, seen := n.states[finding.Key]

	switch {
	case finding.Level == LevelOK && (!seen || previous.level == LevelOK):
		// Nothing was wrong and nothing is wrong.
		n.states[finding.Key] = state{level: LevelOK}
		n.mu.Unlock()
		return Alert{}, false

	case finding.Level == previous.level && time.Since(previous.lastSent) < time.Duration(settings.RepeatHours)*time.Hour:
		// Same problem, still inside the quiet period.
		n.mu.Unlock()
		return Alert{}, false
	}

	n.states[finding.Key] = state{level: finding.Level, lastSent: time.Now()}
	n.mu.Unlock()

	return n.deliver(settings, finding), true
}

// deliver sends one alert and records what happened to it.
func (n *Notifier) deliver(settings *Settings, alert Alert) Alert {
	if !settings.deliverable() {
		alert.SendError = "no recipient is configured"
	} else if n.send == nil {
		alert.SendError = "no mail server is available to send through"
	} else {
		subject := "[redock] " + alert.Title
		if err := n.send(settings.MailboxID, settings.Recipient, subject, alertBody(alert)); err != nil {
			alert.SendError = err.Error()
			log.Printf("notify: could not send %q: %v", alert.Key, err)
		} else {
			alert.Sent = true
		}
	}

	n.mu.Lock()
	n.recent = append(n.recent, alert)
	if len(n.recent) > maxRecent {
		n.recent = n.recent[len(n.recent)-maxRecent:]
	}
	n.mu.Unlock()

	return alert
}

// alertBody writes the message. Plain text on purpose: an alert is read on a
// phone, often through a notification preview, and the first line has to carry
// the whole meaning.
func alertBody(alert Alert) string {
	var b strings.Builder
	b.WriteString(alert.Title)
	b.WriteString("\n\n")
	if alert.Detail != "" {
		b.WriteString(alert.Detail)
		b.WriteString("\n\n")
	}
	fmt.Fprintf(&b, "Level: %s\n", alert.Level)
	fmt.Fprintf(&b, "Time:  %s\n", alert.At.Format(time.RFC1123))
	b.WriteString("\nSent by redock because this check is enabled in Notifications.\n")
	return b.String()
}

// SendTest delivers a message so the operator can confirm the address works
// before relying on it — the worst time to discover a wrong address is when
// something is actually broken.
func (n *Notifier) SendTest() error {
	n.mu.Lock()
	settings := n.settings
	n.mu.Unlock()

	// Say which half is missing. "Choose both first" is unhelpful when one of
	// them is filled in on screen and the other is what the server is missing.
	if settings == nil {
		return fmt.Errorf("the alert settings have not been loaded yet")
	}
	missingMailbox := settings.MailboxID == 0
	missingRecipient := strings.TrimSpace(settings.Recipient) == ""
	switch {
	case missingMailbox && missingRecipient:
		return fmt.Errorf("save a mailbox to send from and an address to send to first")
	case missingMailbox:
		return fmt.Errorf("save a mailbox to send from first")
	case missingRecipient:
		return fmt.Errorf("save an address to send to first")
	}
	if n.send == nil {
		return fmt.Errorf("no mail server is available to send through")
	}

	return n.send(settings.MailboxID, strings.TrimSpace(settings.Recipient),
		"[redock] Test notification",
		"This is a test.\n\nIf you are reading it, alerts from this server will reach you.\n")
}
