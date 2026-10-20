package cloudflare

import "testing"

func kinds(records []desiredRecord) map[string]bool {
	out := map[string]bool{}
	for _, r := range records {
		out[r.kind] = true
	}
	return out
}

// A shared mail host in another zone must not be written into the mail
// domain's zone (Cloudflare would store mail.first.com as
// mail.first.com.second.com and every later sync would fail with 81058).
func TestMailHostRecordsOnlyInOwningZone(t *testing.T) {
	params := EmailDNSParams{MXRecord: "mail.first.com", MailServerIP: "203.0.113.10"}

	second := kinds(recordsFor("second.com", params, false))
	if second["A"] || second["SPF-HELO"] {
		t.Fatalf("mail host records published into the wrong zone: %v", second)
	}
	if !second["MX"] || !second["SPF"] || !second["DMARC"] {
		t.Fatalf("domain records missing: %v", second)
	}

	first := kinds(recordsFor("first.com", params, true))
	if !first["A"] || !first["SPF-HELO"] || first["MX"] {
		t.Fatalf("host-only sync for the owning zone = %v", first)
	}

	if MailHostInZone("second.com", "second.com", params) || !MailHostInZone("first.com", "second.com", params) {
		t.Fatal("MailHostInZone wrong")
	}
}
