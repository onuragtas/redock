package cloudflare

import (
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
