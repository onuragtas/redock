package cloudflare

import (
	"strings"
	"testing"
)

// The reported failure: a zone whose apex already carries an unrelated TXT
// record. Matching on type and name alone would overwrite that token and never
// publish SPF at all.
func zoneWithVerificationToken() []*CloudflareDNSRecord {
	return []*CloudflareDNSRecord{
		{RecordID: "1", Type: "TXT", Name: "example.com", Content: "google-site-verification=abc123"},
		{RecordID: "2", Type: "A", Name: "example.com", Content: "203.0.113.10"},
	}
}

func planFor(t *testing.T, existing []*CloudflareDNSRecord, params EmailDNSParams) map[string]DNSRecordOutcome {
	t.Helper()

	out := map[string]DNSRecordOutcome{}
	for _, desired := range buildEmailRecords("example.com", params) {
		outcome := DNSRecordOutcome{Kind: desired.kind, Content: desired.params.Content}
		match := findMatch(existing, desired)
		switch {
		case match == nil:
			outcome.Action = ActionMissing
		case sameContent(match, desired.params):
			outcome.Action = ActionUnchanged
			outcome.Current = match.Content
		default:
			outcome.Action = ActionDiffers
			outcome.Current = match.Content
		}
		out[desired.kind] = outcome
	}
	return out
}

func TestUnrelatedTXTRecordIsNotMistakenForSPF(t *testing.T) {
	plan := planFor(t, zoneWithVerificationToken(), EmailDNSParams{
		MXRecord: "mail.example.com", SPFRecord: "v=spf1 a mx -all", MailServerIP: "203.0.113.10",
	})

	spf := plan["SPF"]
	if spf.Action != ActionMissing {
		t.Fatalf("the verification token was mistaken for SPF: %+v", spf)
	}
	if spf.Current != "" {
		t.Errorf("nothing existing should have been matched: %+v", spf)
	}
}

func TestExistingCorrectRecordsAreLeftAlone(t *testing.T) {
	existing := append(zoneWithVerificationToken(),
		&CloudflareDNSRecord{RecordID: "3", Type: "TXT", Name: "example.com", Content: "v=spf1 a mx -all"},
		&CloudflareDNSRecord{RecordID: "4", Type: "MX", Name: "example.com", Content: "mail.example.com", Priority: 10},
	)

	plan := planFor(t, existing, EmailDNSParams{
		MXRecord: "mail.example.com", SPFRecord: "v=spf1 a mx -all",
	})

	if plan["SPF"].Action != ActionUnchanged {
		t.Errorf("an already-correct SPF record should be left alone: %+v", plan["SPF"])
	}
	if plan["MX"].Action != ActionUnchanged {
		t.Errorf("an already-correct MX record should be left alone: %+v", plan["MX"])
	}
	// The records that are genuinely missing must still be reported.
	if plan["DMARC"].Action != ActionMissing {
		t.Errorf("the missing DMARC record was not reported: %+v", plan["DMARC"])
	}
}

func TestChangedContentIsReportedAsDiffering(t *testing.T) {
	existing := []*CloudflareDNSRecord{
		{RecordID: "1", Type: "TXT", Name: "example.com", Content: "v=spf1 include:old-provider.example.net ~all"},
	}

	plan := planFor(t, existing, EmailDNSParams{SPFRecord: "v=spf1 a mx -all"})

	spf := plan["SPF"]
	if spf.Action != ActionDiffers {
		t.Fatalf("an outdated SPF record should be reported as differing: %+v", spf)
	}
	if spf.Current == "" {
		t.Error("the plan should show what the zone holds today")
	}
}

func TestAdditionalMXRecordsAreNotDisturbed(t *testing.T) {
	// A backup exchanger belonging to someone else must survive.
	existing := []*CloudflareDNSRecord{
		{RecordID: "1", Type: "MX", Name: "example.com", Content: "backup-mx.example.net", Priority: 20},
	}

	plan := planFor(t, existing, EmailDNSParams{MXRecord: "mail.example.com"})

	if plan["MX"].Action != ActionMissing {
		t.Fatalf("our MX is not there yet and must be created, not matched to the backup: %+v", plan["MX"])
	}
	if plan["MX"].Current != "" {
		t.Errorf("the backup exchanger must not be treated as ours: %+v", plan["MX"])
	}
}

func TestQuotedAndSplitTXTContentCompareEqual(t *testing.T) {
	// Cloudflare and resolvers quote and split long TXT values differently.
	existing := []*CloudflareDNSRecord{
		{RecordID: "1", Type: "TXT", Name: "mail._domainkey.example.com",
			Content: "\"v=DKIM1; k=rsa; \" \"p=MIIBIjANBgkq\""},
	}

	plan := planFor(t, existing, EmailDNSParams{
		DKIMSelector: "mail",
		DKIMRecord:   "v=DKIM1; k=rsa; p=MIIBIjANBgkq",
	})

	if plan["DKIM"].Action != ActionUnchanged {
		t.Fatalf("quoting differences must not look like a change: %+v", plan["DKIM"])
	}
}

func TestPriorityDifferenceCountsAsAChange(t *testing.T) {
	existing := []*CloudflareDNSRecord{
		{RecordID: "1", Type: "MX", Name: "example.com", Content: "mail.example.com", Priority: 50},
	}

	plan := planFor(t, existing, EmailDNSParams{MXRecord: "mail.example.com"})
	if plan["MX"].Action != ActionDiffers {
		t.Fatalf("a wrong priority should be corrected: %+v", plan["MX"])
	}
}

func TestNamesCompareWithoutCaseOrTrailingDot(t *testing.T) {
	existing := []*CloudflareDNSRecord{
		{RecordID: "1", Type: "MX", Name: "Example.COM.", Content: "Mail.Example.com.", Priority: 10},
	}

	plan := planFor(t, existing, EmailDNSParams{MXRecord: "mail.example.com"})
	if plan["MX"].Action != ActionUnchanged {
		t.Fatalf("case and trailing dots must not look like a different record: %+v", plan["MX"])
	}
}

func TestDefaultsFillInMissingValues(t *testing.T) {
	records := buildEmailRecords("example.com", EmailDNSParams{MailServerIP: "203.0.113.10"})

	byKind := map[string]desiredRecord{}
	for _, record := range records {
		byKind[record.kind] = record
	}

	if got := byKind["MX"].params.Content; got != "mail.example.com" {
		t.Errorf("the MX target should default to mail.<domain>, got %q", got)
	}
	if got := byKind["SPF"].params.Content; got != "v=spf1 a mx ip4:203.0.113.10 -all" {
		t.Errorf("the SPF default should authorise the server address, got %q", got)
	}
	if _, ok := byKind["A"]; !ok {
		t.Error("an A record for the mail host should be planned when the address is known")
	}
	if _, ok := byKind["DKIM"]; ok {
		t.Error("no DKIM record should be planned when there is no key to publish")
	}
}

func TestDKIMKeyIsComparedCaseSensitively(t *testing.T) {
	// Base64 is case-sensitive: two keys differing only in case are two keys.
	existing := []*CloudflareDNSRecord{
		{RecordID: "1", Type: "TXT", Name: "mail._domainkey.example.com",
			Content: "v=DKIM1; k=rsa; p=miibijanbgkq"},
	}

	plan := planFor(t, existing, EmailDNSParams{
		DKIMSelector: "mail",
		DKIMRecord:   "v=DKIM1; k=rsa; p=MIIBIjANBgkq",
	})

	if plan["DKIM"].Action != ActionDiffers {
		t.Fatalf("a key differing only in case must be republished: %+v", plan["DKIM"])
	}
}

// The mail host greets other servers under its own name, which is a separate
// SPF identity from the domain in the envelope sender. Receivers report
// "SPF: HELO does not publish an SPF record" when it is missing.
func TestPlanPublishesSPFForTheMailHost(t *testing.T) {
	records := buildEmailRecords("example.com", EmailDNSParams{MailServerIP: "203.0.113.10"})

	var helo *desiredRecord
	for i := range records {
		if records[i].kind == "SPF-HELO" {
			helo = &records[i]
		}
	}
	if helo == nil {
		t.Fatal("no SPF record is planned for the mail host")
	}

	if helo.params.Name != "mail.example.com" {
		t.Errorf("name = %q, want the mail host", helo.params.Name)
	}
	if helo.params.Type != "TXT" {
		t.Errorf("type = %q, want TXT", helo.params.Type)
	}
	if !strings.HasPrefix(helo.params.Content, "v=spf1") {
		t.Errorf("content = %q, want an SPF record", helo.params.Content)
	}

	// It must recognise its own counterpart and nothing else at that name: an
	// A record or a verification token there is none of its business.
	if !helo.matches(&CloudflareDNSRecord{Type: "TXT", Name: "mail.example.com", Content: "v=spf1 a -all"}) {
		t.Error("an existing HELO SPF record is not recognised")
	}
	if helo.matches(&CloudflareDNSRecord{Type: "TXT", Name: "mail.example.com", Content: "google-site-verification=abc"}) {
		t.Error("an unrelated TXT record at the mail host would be overwritten")
	}
	if helo.matches(&CloudflareDNSRecord{Type: "A", Name: "mail.example.com", Content: "203.0.113.10"}) {
		t.Error("the mail host's A record would be overwritten by its SPF record")
	}
}

// The apex SPF and the host SPF are different records and must not be
// mistaken for one another.
func TestApexAndHostSPFStaySeparate(t *testing.T) {
	records := buildEmailRecords("example.com", EmailDNSParams{MailServerIP: "203.0.113.10"})

	names := map[string]string{}
	for _, record := range records {
		if strings.HasPrefix(record.kind, "SPF") {
			names[record.kind] = record.params.Name
		}
	}
	if names["SPF"] != "example.com" || names["SPF-HELO"] != "mail.example.com" {
		t.Errorf("SPF records landed on %v", names)
	}
}

func TestDMARCPolicyIsRead(t *testing.T) {
	tests := map[string]string{
		"v=DMARC1; p=quarantine; rua=mailto:a@b.com": "quarantine",
		`"v=DMARC1; p=reject"`:                       "reject",
		"v=DMARC1;p=none;":                           "none",
		"v=DMARC1; rua=mailto:a@b.com":               "",
	}
	for record, want := range tests {
		if got := dmarcPolicy(record); got != want {
			t.Errorf("dmarcPolicy(%q) = %q, want %q", record, got, want)
		}
	}
}

// Publishing must never turn enforcement off. A p=quarantine or p=reject in the
// zone was switched on deliberately, usually after weeks of reading reports;
// replacing it with the p=none default would undo that silently.
func TestSyncKeepsAStricterPublishedDMARCPolicy(t *testing.T) {
	tests := []struct {
		name      string
		published string
		desired   string
		want      string
	}{
		{
			"quarantine is kept over the none default",
			"v=DMARC1; p=quarantine; rua=mailto:dmarc@example.com",
			"v=DMARC1; p=none; rua=mailto:postmaster@example.com",
			"v=DMARC1; p=quarantine; rua=mailto:postmaster@example.com",
		},
		{
			"reject is kept too",
			"v=DMARC1; p=reject; rua=mailto:dmarc@example.com",
			"v=DMARC1; p=none; rua=mailto:postmaster@example.com",
			"v=DMARC1; p=reject; rua=mailto:postmaster@example.com",
		},
		{
			// Tightening is the operator's own change and must go through.
			"a weaker published policy does not hold the new one back",
			"v=DMARC1; p=none; rua=mailto:dmarc@example.com",
			"v=DMARC1; p=reject; rua=mailto:postmaster@example.com",
			"v=DMARC1; p=reject; rua=mailto:postmaster@example.com",
		},
		{
			"an equal policy changes nothing",
			"v=DMARC1; p=quarantine; rua=mailto:old@example.com",
			"v=DMARC1; p=quarantine; rua=mailto:postmaster@example.com",
			"v=DMARC1; p=quarantine; rua=mailto:postmaster@example.com",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := keepStricterDMARCPolicy(&CloudflareDNSRecord{Content: tc.published}, tc.desired)
			if got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

// The reported outcome has to show what will actually be written, or the
// preview would promise one thing and the sync do another.
func TestPlannedContentReflectsTheReconciledPolicy(t *testing.T) {
	records := buildEmailRecords("example.com", EmailDNSParams{})

	var dmarc *desiredRecord
	for i := range records {
		if records[i].kind == "DMARC" {
			dmarc = &records[i]
		}
	}
	if dmarc == nil {
		t.Fatal("no DMARC record is planned")
	}

	outcome := DNSRecordOutcome{Content: dmarc.params.Content}
	applyReconcile(dmarc, &CloudflareDNSRecord{
		Type: "TXT", Name: "_dmarc.example.com",
		Content: "v=DMARC1; p=reject; rua=mailto:dmarc@example.com",
	}, &outcome)

	if !strings.Contains(outcome.Content, "p=reject") {
		t.Errorf("the plan reports %q, which is not what would be written", outcome.Content)
	}
	if outcome.Content != dmarc.params.Content {
		t.Errorf("the plan (%q) and what gets written (%q) disagree", outcome.Content, dmarc.params.Content)
	}
	// And the report address is still corrected to one that exists.
	if !strings.Contains(outcome.Content, "postmaster@example.com") {
		t.Errorf("the report address was not corrected: %q", outcome.Content)
	}
}
