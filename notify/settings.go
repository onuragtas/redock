package notify

import (
	"strings"

	"redock/platform/memory"
)

// TableName is where the settings live in the in-memory database.
const TableName = "notify_settings"

// Settings is what the operator chose: where alerts go, and which of them are
// worth being told about.
type Settings struct {
	memory.BaseEntity

	Enabled bool `json:"enabled"`
	// MailboxID is the local mailbox alerts are sent from. Using a real mailbox
	// rather than a made-up address means the message is signed and passes the
	// domain's own SPF and DKIM, so it is not the alert that lands in spam.
	MailboxID uint `json:"mailbox_id"`
	// Recipient is where alerts are delivered. It is deliberately free text:
	// the address most operators want is not on this server.
	Recipient string `json:"recipient"`

	WatchCertificate bool `json:"watch_certificate"`
	WatchQueue       bool `json:"watch_queue"`
	WatchMemory      bool `json:"watch_memory"`
	WatchBlocked     bool `json:"watch_blocked"`

	// CertDaysBefore is how many days of remaining certificate life still
	// counts as fine.
	CertDaysBefore int `json:"cert_days_before"`
	// QueueThreshold is the number of stuck outbound messages worth reporting.
	QueueThreshold int `json:"queue_threshold"`
	// BlockedThreshold is how many blocked clients count as a burst rather than
	// the usual background noise.
	BlockedThreshold int `json:"blocked_threshold"`
	// RepeatHours is how long to stay quiet before repeating an alert that is
	// still true. A problem that lasts a week should not send a week of mail.
	RepeatHours int `json:"repeat_hours"`
}

// DefaultSettings is what a fresh install starts with: everything watched, but
// nothing sent until an address is chosen.
func DefaultSettings() *Settings {
	return &Settings{
		Enabled:          false,
		WatchCertificate: true,
		WatchQueue:       true,
		WatchMemory:      true,
		WatchBlocked:     true,
		CertDaysBefore:   14,
		QueueThreshold:   10,
		BlockedThreshold: 5,
		RepeatHours:      24,
	}
}

// normalize fills in values that would otherwise disable a watch by accident —
// a zero threshold would either never fire or fire constantly.
func (s *Settings) normalize() {
	s.Recipient = strings.TrimSpace(s.Recipient)

	if s.CertDaysBefore <= 0 {
		s.CertDaysBefore = 14
	}
	if s.QueueThreshold <= 0 {
		s.QueueThreshold = 10
	}
	if s.BlockedThreshold <= 0 {
		s.BlockedThreshold = 5
	}
	if s.RepeatHours <= 0 {
		s.RepeatHours = 24
	}
}

// deliverable reports whether an alert could actually be sent.
func (s *Settings) deliverable() bool {
	return s.Enabled && s.MailboxID != 0 && s.Recipient != ""
}
