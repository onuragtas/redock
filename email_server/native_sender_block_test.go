package email_server

import (
	"strings"
	"testing"

	"redock/platform/memory"
)

func TestParseBlockedSenders(t *testing.T) {
	rules := parseBlockedSenders("Example.COM, *.spam.net\nbad@nuisance.org; @other.com\n\n example.com ")

	want := []senderRule{
		{value: "example.com"},
		{value: "spam.net"},
		{value: "bad@nuisance.org", address: true},
		{value: "other.com"},
	}
	if len(rules) != len(want) {
		t.Fatalf("parsed %d rules, want %d: %+v", len(rules), len(want), rules)
	}
	for i, rule := range rules {
		if rule != want[i] {
			t.Errorf("rule %d = %+v, want %+v", i, rule, want[i])
		}
	}
}

func TestParseBlockedSendersRejectsUselessEntries(t *testing.T) {
	for _, configured := range []string{"", "   ", "@", "@@", "bad@", "@example", ",,;"} {
		if rules := parseBlockedSenders(configured); len(rules) != 0 {
			// "@example" survives only as the domain "example", which is what
			// the operator wrote; everything else here means nothing at all.
			if configured == "@example" && len(rules) == 1 && rules[0].value == "example" {
				continue
			}
			t.Errorf("parseBlockedSenders(%q) = %+v, want nothing", configured, rules)
		}
	}
}

func TestBlockedSender(t *testing.T) {
	rules := parseBlockedSenders("example.com, bad@nuisance.org")

	cases := []struct {
		from    string
		blocked bool
		entry   string
	}{
		{"someone@example.com", true, "example.com"},
		{"SOMEONE@EXAMPLE.COM", true, "example.com"},
		{"someone@mail.example.com", true, "example.com"}, // subdomains too
		{"someone@example.com.", true, "example.com"},
		{"bad@nuisance.org", true, "bad@nuisance.org"},
		{"good@nuisance.org", false, ""},      // the rule named one address
		{"someone@notexample.com", false, ""}, // suffix, not subdomain
		{"someone@example.com.evil.net", false, ""},
		{"", false, ""}, // the null sender a bounce carries
		{"malformed", false, ""},
	}

	for _, tc := range cases {
		entry, blocked := blockedSender(tc.from, rules)
		if blocked != tc.blocked || entry != tc.entry {
			t.Errorf("blockedSender(%q) = (%q, %v), want (%q, %v)", tc.from, entry, blocked, tc.entry, tc.blocked)
		}
	}
}

func TestBlockedSenderWithoutRules(t *testing.T) {
	if entry, blocked := blockedSender("someone@example.com", nil); blocked {
		t.Errorf("an empty list blocked %q via %q", "someone@example.com", entry)
	}
}

func TestNormalizeBlockedSenders(t *testing.T) {
	got := normalizeBlockedSenders(" Example.com , example.com\n*.Spam.net ")
	want := "example.com\nspam.net"
	if got != want {
		t.Errorf("normalizeBlockedSenders() = %q, want %q", got, want)
	}
	if got := normalizeBlockedSenders("  "); got != "" {
		t.Errorf("normalizeBlockedSenders(blank) = %q, want empty", got)
	}
}

func TestInboundSessionRefusesBlockedSenderDomain(t *testing.T) {
	m := newTestManager(t)
	seedDomain(t, m, "example.com", "alice", "secret")
	m.config.BlockedSenderDomains = "spam.test\nbad@nuisance.test"

	session := &smtpSession{backend: &smtpBackend{manager: m, submission: false}}

	err := session.Mail("someone@mail.spam.test", nil)
	if err == nil {
		t.Fatal("a blocked sender domain must be refused at MAIL FROM")
	}
	if !strings.Contains(err.Error(), "sender domain is blocked") {
		t.Fatalf("expected a sender-block refusal, got %v", err)
	}

	if err := session.Mail("bad@nuisance.test", nil); err == nil {
		t.Fatal("a blocked sender address must be refused")
	}
	if err := session.Mail("good@nuisance.test", nil); err != nil {
		t.Fatalf("an address the rules do not name must be accepted: %v", err)
	}
	if err := session.Mail("someone@elsewhere.test", nil); err != nil {
		t.Fatalf("an unrelated sender must be accepted: %v", err)
	}
}

func TestSubmissionIgnoresBlockedSenderDomains(t *testing.T) {
	m := newTestManager(t)
	_, mailbox := seedDomain(t, m, "example.com", "alice", "secret")
	// The operator's own domain on the block list must not lock their users
	// out of sending: the list is about inbound mail.
	m.config.BlockedSenderDomains = "example.com"

	account := m.LookupAccount(mailbox.Email)
	session := &smtpSession{backend: &smtpBackend{manager: m, submission: true}, account: account}

	if err := session.Mail("alice@example.com", nil); err != nil {
		t.Fatalf("an authenticated user must still be able to send: %v", err)
	}
}

func TestBlockedSenderRejectionIsLogged(t *testing.T) {
	m := newTestManager(t)
	seedDomain(t, m, "example.com", "alice", "secret")
	m.config.BlockedSenderDomains = "spam.test"

	session := &smtpSession{backend: &smtpBackend{manager: m, submission: false}}
	if err := session.Mail("someone@spam.test", nil); err == nil {
		t.Fatal("expected the sender to be refused")
	}

	logs := memory.Filter[*EmailLog](m.db, "email_logs", func(l *EmailLog) bool {
		return l != nil && l.Status == "rejected" && strings.Contains(l.StatusMessage, "sender blocked")
	})
	if len(logs) != 1 {
		t.Fatalf("expected one rejection log entry, got %d", len(logs))
	}
}
