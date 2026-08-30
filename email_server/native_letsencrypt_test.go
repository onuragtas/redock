package email_server

import (
	"testing"

	"redock/platform/memory"
)

func TestIsPublicNameRejectsUnissuableNames(t *testing.T) {
	public := []string{"mail.example.com", "mx1.redock.dev", "example.org"}
	private := []string{"", "localhost", "redock.localhost", "mail.local", "box.internal", "mail.test", "host", "a.invalid"}

	for _, name := range public {
		if !isPublicName(name) {
			t.Errorf("%q should be requestable from a public CA", name)
		}
	}
	for _, name := range private {
		if isPublicName(name) {
			t.Errorf("%q must not be sent to a public CA", name)
		}
	}
}

func TestCertificateWantedNamesCoversHostAndDomains(t *testing.T) {
	m := newTestManager(t)
	m.config.Hostname = "mx.redock.dev"

	// example.com is a reserved example domain; use something requestable.
	domain := &EmailDomain{Domain: "acikkaynak.dev", Enabled: true}
	if err := memory.Create(m.db, "email_domains", domain); err != nil {
		t.Fatalf("create domain: %v", err)
	}
	disabled := &EmailDomain{Domain: "kapali.dev", Enabled: false}
	if err := memory.Create(m.db, "email_domains", disabled); err != nil {
		t.Fatalf("create disabled domain: %v", err)
	}

	names := m.certificateWantedNames()

	if !containsString(names, "mx.redock.dev") {
		t.Errorf("the mail hostname is missing: %v", names)
	}
	if !containsString(names, "mail.acikkaynak.dev") {
		t.Errorf("mail.<domain> is missing: %v", names)
	}
	if containsString(names, "mail.kapali.dev") {
		t.Errorf("a disabled domain must not be requested: %v", names)
	}

	// A hostname a CA cannot issue for must not reach the order.
	m.config.Hostname = "redock.localhost"
	names = m.certificateWantedNames()
	if containsString(names, "redock.localhost") {
		t.Errorf(".localhost must be filtered out: %v", names)
	}
}

func TestCertificateStatusReportsMissingNames(t *testing.T) {
	m := newTestManager(t)
	m.config.Hostname = "mx.redock.dev"

	domain := &EmailDomain{Domain: "acikkaynak.dev", Enabled: true}
	if err := memory.Create(m.db, "email_domains", domain); err != nil {
		t.Fatalf("create domain: %v", err)
	}

	// Bring up the cert manager with a self-signed certificate that only covers
	// the hostname, so mail.<domain> is reported as missing.
	n := m.Native()
	n.mu.Lock()
	n.certs = newCertManager(m.config.Hostname, m.dataPath, m.workDir(), "", "", nil, nil)
	n.mu.Unlock()

	status := m.CertificateStatus()

	if status.Source != "self-signed" {
		t.Errorf("expected a self-signed certificate, got %q", status.Source)
	}
	if !status.SelfSigned {
		t.Error("a locally generated certificate must be flagged as self-signed")
	}
	if !containsString(status.Wanted, "mail.acikkaynak.dev") {
		t.Errorf("the wanted set is wrong: %v", status.Wanted)
	}
	if !containsString(status.Missing, "mail.acikkaynak.dev") {
		t.Errorf("a name the certificate does not cover must be reported: %+v", status)
	}
	if containsString(status.Missing, "mx.redock.dev") {
		t.Errorf("the hostname is covered and must not be listed as missing: %+v", status)
	}
	if status.DaysLeft <= 0 {
		t.Errorf("a freshly generated certificate should have time left, got %d", status.DaysLeft)
	}
}

func TestRequestCertificateRefusesWithoutAGateway(t *testing.T) {
	m := newTestManager(t)
	m.config.Hostname = "mx.redock.dev"

	// No API Gateway is running in the test binary, so the request must be
	// refused with an explanation rather than attempted.
	if _, err := m.RequestLetsEncryptCertificate(); err == nil {
		t.Fatal("requesting a certificate without a running gateway must fail")
	}
}

func TestRequestCertificateRefusesUnissuableHostname(t *testing.T) {
	m := newTestManager(t)
	m.config.Hostname = "redock.localhost"

	_, err := m.RequestLetsEncryptCertificate()
	if err == nil {
		t.Fatal("a .localhost hostname must not produce an ACME order")
	}
}
