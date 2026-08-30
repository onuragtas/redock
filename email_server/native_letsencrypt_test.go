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

func TestCertificateWantedNamesCoversHostAndOverrides(t *testing.T) {
	m := newTestManager(t)
	m.config.Hostname = "mx.redock.dev"

	// A domain on the shared mail host adds no name of its own.
	shared := &EmailDomain{Domain: "acikkaynak.dev", Enabled: true, MXRecord: "mail.acikkaynak.dev"}
	if err := memory.Create(m.db, "email_domains", shared); err != nil {
		t.Fatalf("create domain: %v", err)
	}
	// A domain with its own mail host does.
	custom := &EmailDomain{Domain: "ozel.dev", Enabled: true, MXRecord: "posta.ozel.dev"}
	if err := memory.Create(m.db, "email_domains", custom); err != nil {
		t.Fatalf("create domain: %v", err)
	}
	// A disabled domain is not served, so it is not certified either.
	disabled := &EmailDomain{Domain: "kapali.dev", Enabled: false, MXRecord: "posta.kapali.dev"}
	if err := memory.Create(m.db, "email_domains", disabled); err != nil {
		t.Fatalf("create disabled domain: %v", err)
	}

	names := m.certificateWantedNames()

	if !containsString(names, "mx.redock.dev") {
		t.Errorf("the mail hostname is missing: %v", names)
	}
	if !containsString(names, "posta.ozel.dev") {
		t.Errorf("an overridden mail host must be certified: %v", names)
	}
	if containsString(names, "mail.acikkaynak.dev") {
		t.Errorf("a domain on the shared host needs no name of its own: %v", names)
	}
	if containsString(names, "posta.kapali.dev") {
		t.Errorf("a disabled domain must not be requested: %v", names)
	}

	// A hostname a CA cannot issue for must not reach the order.
	m.config.Hostname = "redock.localhost"
	if names := m.certificateWantedNames(); containsString(names, "redock.localhost") {
		t.Errorf(".localhost must be filtered out: %v", names)
	}
}

func TestCertificateStatusReportsMissingNames(t *testing.T) {
	m := newTestManager(t)
	m.config.Hostname = "mx.redock.dev"

	// This domain has its own mail host, so it needs a name the hostname-only
	// certificate will not cover.
	domain := &EmailDomain{Domain: "acikkaynak.dev", Enabled: true, MXRecord: "posta.acikkaynak.dev"}
	if err := memory.Create(m.db, "email_domains", domain); err != nil {
		t.Fatalf("create domain: %v", err)
	}

	// Bring up the cert manager with a self-signed certificate that only covers
	// the hostname, so the domain's own mail host is reported as missing.
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
	if !containsString(status.Wanted, "posta.acikkaynak.dev") {
		t.Errorf("the wanted set is wrong: %v", status.Wanted)
	}
	if !containsString(status.Missing, "posta.acikkaynak.dev") {
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

func TestMXHostIsSharedAcrossDomains(t *testing.T) {
	m := newTestManager(t)
	m.config.Hostname = "mx.redock.dev"

	// Domains created through the dashboard carry the generated
	// "mail.<domain>" value; that must not override the shared mail host.
	first := &EmailDomain{Domain: "birinci.dev", Enabled: true, MXRecord: "mail.birinci.dev"}
	second := &EmailDomain{Domain: "ikinci.dev", Enabled: true, MXRecord: "mail.ikinci.dev"}
	for _, domain := range []*EmailDomain{first, second} {
		if err := memory.Create(m.db, "email_domains", domain); err != nil {
			t.Fatalf("create domain: %v", err)
		}
	}

	if got := m.mxHostFor(first); got != "mx.redock.dev" {
		t.Errorf("all domains should point at the shared mail host, got %q", got)
	}
	if got := m.mxHostFor(second); got != "mx.redock.dev" {
		t.Errorf("all domains should point at the shared mail host, got %q", got)
	}

	// One certificate name serves every domain.
	names := m.certificateWantedNames()
	if len(names) != 1 || names[0] != "mx.redock.dev" {
		t.Fatalf("expected a single certificate name for all domains, got %v", names)
	}
}

func TestExplicitMXOverrideIsKeptAndCertified(t *testing.T) {
	m := newTestManager(t)
	m.config.Hostname = "mx.redock.dev"

	custom := &EmailDomain{Domain: "ozel.dev", Enabled: true, MXRecord: "posta.ozel.dev"}
	if err := memory.Create(m.db, "email_domains", custom); err != nil {
		t.Fatalf("create domain: %v", err)
	}

	if got := m.mxHostFor(custom); got != "posta.ozel.dev" {
		t.Errorf("an explicit MX host must be respected, got %q", got)
	}

	names := m.certificateWantedNames()
	if !containsString(names, "posta.ozel.dev") {
		t.Errorf("a domain with its own mail host needs its own certificate name: %v", names)
	}
	if !containsString(names, "mx.redock.dev") {
		t.Errorf("the server hostname must still be covered: %v", names)
	}
}

func TestWithoutAServerHostnameEachDomainGetsItsOwnName(t *testing.T) {
	m := newTestManager(t)
	m.config.Hostname = "redock.localhost" // not issuable

	domain := &EmailDomain{Domain: "yedek.dev", Enabled: true, MXRecord: "mail.yedek.dev"}
	if err := memory.Create(m.db, "email_domains", domain); err != nil {
		t.Fatalf("create domain: %v", err)
	}

	if got := m.mxHostFor(domain); got != "mail.yedek.dev" {
		t.Errorf("without a usable server hostname the per-domain host is the fallback, got %q", got)
	}
	if names := m.certificateWantedNames(); !containsString(names, "mail.yedek.dev") {
		t.Errorf("the fallback host must be certified: %v", names)
	}
}

func TestCheckNamesFlagsNamesThatDoNotPointHere(t *testing.T) {
	m := newTestManager(t)

	// "localhost" resolves to a loopback address, which is in our set;
	// a name that cannot resolve must be reported rather than ordered.
	checks := m.checkNames([]string{"localhost", "definitely-not-a-real-name.invalid"})
	if len(checks) != 2 {
		t.Fatalf("expected a verdict per name, got %d", len(checks))
	}

	byName := map[string]NameCheck{}
	for _, check := range checks {
		byName[check.Name] = check
	}

	if !byName["localhost"].PointsAtUs {
		t.Errorf("localhost should resolve to this machine: %+v", byName["localhost"])
	}
	bad := byName["definitely-not-a-real-name.invalid"]
	if bad.PointsAtUs {
		t.Error("an unresolvable name must not be considered ready")
	}
	if bad.Reason == "" {
		t.Error("an unready name must come with a reason")
	}
}
