package email_server

import (
	"strings"
	"testing"

	"redock/platform/memory"
)

func findCheck(checks []DeliverabilityCheck, id, domain string) *DeliverabilityCheck {
	for i := range checks {
		if checks[i].ID == id && (domain == "" || checks[i].Domain == domain) {
			return &checks[i]
		}
	}
	return nil
}

func TestDeliverabilityFailsOnAnUnusableHostname(t *testing.T) {
	m := newTestManager(t)
	m.config.Hostname = "redock.localhost"

	report := m.CheckDeliverability()

	helo := findCheck(report.Checks, "helo", "")
	if helo == nil {
		t.Fatal("the HELO name is not checked at all")
	}
	if helo.Level != CheckFail {
		t.Errorf("a .localhost HELO must fail: %+v", helo)
	}
	if helo.Advice == "" {
		t.Error("a failing check must say what to do about it")
	}
}

func TestDeliverabilityReportsAMissingMX(t *testing.T) {
	m := newTestManager(t)
	m.config.Hostname = "mx.example.com"

	// A reserved documentation domain has no MX, which is exactly the case
	// being detected: mail can leave, but bounces have nowhere to return to.
	domain := &EmailDomain{Domain: "no-mx.example.com", Enabled: true}
	if err := memory.Create(m.db, "email_domains", domain); err != nil {
		t.Fatalf("create domain: %v", err)
	}

	report := m.CheckDeliverability()

	mx := findCheck(report.Checks, "mx", "no-mx.example.com")
	if mx == nil {
		t.Fatal("the MX record is not checked")
	}
	if mx.Level != CheckFail {
		t.Errorf("a domain with no MX must be reported: %+v", mx)
	}
	if !strings.Contains(mx.Advice, "bounce") {
		t.Errorf("the advice should explain what a missing MX costs: %q", mx.Advice)
	}
}

func TestDeliverabilitySkipsPTRWithoutAKnownAddress(t *testing.T) {
	m := newTestManager(t)
	m.config.Hostname = "mx.example.com"
	m.config.IPAddress = ""

	report := m.CheckDeliverability()

	ptr := findCheck(report.Checks, "ptr", "")
	if ptr == nil {
		t.Fatal("the PTR check is missing")
	}
	if ptr.Level != CheckUnkn {
		t.Errorf("without a public address the PTR verdict must be unknown, not a pass or fail: %+v", ptr)
	}
}

func TestDeliverabilityCountsPassesAndTotals(t *testing.T) {
	m := newTestManager(t)
	m.config.Hostname = "redock.localhost" // guarantees at least one failure

	report := m.CheckDeliverability()

	if report.Total != len(report.Checks) {
		t.Errorf("Total must count every check: %d vs %d", report.Total, len(report.Checks))
	}
	if report.Passed > report.Total {
		t.Errorf("passed cannot exceed total: %d/%d", report.Passed, report.Total)
	}
	if report.CheckedAt.IsZero() {
		t.Error("the report should be timestamped")
	}
}

func TestPublicKeyMatchesIgnoresFormatting(t *testing.T) {
	expected := "v=DKIM1; k=rsa; p=MIIBIjANBgkq"

	if !publicKeyMatches(`v=DKIM1; k=rsa; p=MIIBIjANBgkq`, expected) {
		t.Error("an identical record should match")
	}
	if !publicKeyMatches("v=DKIM1;k=rsa;p=MIIB IjAN\tBgkq", expected) {
		t.Error("whitespace inside the key must be ignored — resolvers split long records")
	}
	if publicKeyMatches("v=DKIM1; k=rsa; p=DIFFERENTKEY", expected) {
		t.Error("a different key must not match")
	}
	if publicKeyMatches("v=DKIM1; k=rsa", expected) {
		t.Error("a record with no key must not match")
	}
	if publicKeyMatches("", expected) {
		t.Error("an empty record must not match")
	}
}
