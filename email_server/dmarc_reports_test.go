package email_server

import (
	"strings"
	"testing"
)

func TestDMARCReportAddressParsing(t *testing.T) {
	tests := []struct {
		name   string
		record string
		tag    string
		want   []string
	}{
		{"a plain address", `v=DMARC1; p=none; rua=mailto:dmarc@example.com`, "rua", []string{"dmarc@example.com"}},
		{"quoted record", `"v=DMARC1; p=quarantine; rua=mailto:dmarc@example.com"`, "rua", []string{"dmarc@example.com"}},
		// A size limit may follow the address.
		{"with a size limit", `v=DMARC1; p=none; rua=mailto:dmarc@example.com!10m`, "rua", []string{"dmarc@example.com"}},
		{"several addresses", `v=DMARC1; p=none; rua=mailto:a@example.com,mailto:b@example.net`, "rua",
			[]string{"a@example.com", "b@example.net"}},
		{"forensic tag", `v=DMARC1; p=none; rua=mailto:a@example.com; ruf=mailto:f@example.com`, "ruf",
			[]string{"f@example.com"}},
		{"tag absent", `v=DMARC1; p=none`, "rua", nil},
		// Anything that is not a mailto is not an address we can check.
		{"a non-mailto URI", `v=DMARC1; p=none; rua=https://example.com/report`, "rua", nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := dmarcReportAddresses(tc.record, tc.tag)
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// The failure this check exists for: a record pointing at an address that has
// no mailbox. Every DMARC report for the domain is refused, silently.
func TestDMARCReportAddressWithNoMailboxIsReported(t *testing.T) {
	m := newTestManager(t)
	domain, err := m.AddDomain("example.com", "test")
	if err != nil {
		t.Fatalf("AddDomain: %v", err)
	}

	checks := m.checkDMARCReportAddresses(domain.Domain,
		`v=DMARC1; p=quarantine; rua=mailto:dmarc@example.com`)

	if len(checks) != 1 {
		t.Fatalf("got %d checks, want 1: %+v", len(checks), checks)
	}
	if checks[0].Level != CheckFail {
		t.Errorf("level = %q, want %q", checks[0].Level, CheckFail)
	}
	if !strings.Contains(checks[0].Detail, "dmarc@example.com") {
		t.Errorf("the finding does not name the address: %q", checks[0].Detail)
	}
}

// The address the server publishes by default is the postmaster mailbox every
// domain gets, so a default setup must come back clean.
func TestDMARCReportAddressThatExistsPasses(t *testing.T) {
	m := newTestManager(t)
	domain, err := m.AddDomain("example.com", "test")
	if err != nil {
		t.Fatalf("AddDomain: %v", err)
	}

	checks := m.checkDMARCReportAddresses(domain.Domain,
		`v=DMARC1; p=none; rua=mailto:postmaster@example.com`)

	if len(checks) != 1 || checks[0].Level != CheckOK {
		t.Fatalf("the default report address was not accepted: %+v", checks)
	}
}

// An alias is a perfectly good home for reports.
func TestDMARCReportAddressResolvedThroughAnAliasPasses(t *testing.T) {
	m := newTestManager(t)
	domain, err := m.AddDomain("example.com", "test")
	if err != nil {
		t.Fatalf("AddDomain: %v", err)
	}
	if _, err := m.AddAlias("dmarc@example.com", "postmaster@example.com", true); err != nil {
		t.Fatalf("AddAlias: %v", err)
	}

	checks := m.checkDMARCReportAddresses(domain.Domain,
		`v=DMARC1; p=quarantine; rua=mailto:dmarc@example.com`)

	if len(checks) != 1 || checks[0].Level != CheckOK {
		t.Fatalf("an aliased report address was not accepted: %+v", checks)
	}
}

// Reports may be sent to another domain, but that domain has to authorise it.
func TestExternalDMARCReportAddressIsFlagged(t *testing.T) {
	m := newTestManager(t)
	domain, err := m.AddDomain("example.com", "test")
	if err != nil {
		t.Fatalf("AddDomain: %v", err)
	}

	checks := m.checkDMARCReportAddresses(domain.Domain,
		`v=DMARC1; p=none; rua=mailto:reports@analyzer.test`)

	if len(checks) != 1 || checks[0].Level != CheckWarn {
		t.Fatalf("an external report address was not flagged: %+v", checks)
	}
	if !strings.Contains(checks[0].Advice, "_report._dmarc") {
		t.Errorf("the advice does not mention the authorisation record: %q", checks[0].Advice)
	}
}
