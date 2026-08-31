package cloudflare

import (
	"fmt"
	"strings"
)

// Publishing mail records is not a plain "create or replace". A domain apex
// normally carries several TXT records — SPF next to a Google or Microsoft
// verification token — and several MX records. Matching on type and name alone
// picks whichever happens to come first, so the SPF record can end up
// overwriting an unrelated token while the record that was actually wanted is
// never written. Every desired record therefore identifies its own existing
// counterpart by content, and nothing else in the zone is touched.

// DNSRecordAction is what happened (or would happen) to one record.
const (
	ActionCreated   = "created"
	ActionUpdated   = "updated"
	ActionUnchanged = "unchanged"
	ActionMissing   = "missing" // planning only: it is not there yet
	ActionDiffers   = "differs" // planning only: it is there with other content
	ActionFailed    = "failed"
)

// DNSRecordOutcome reports one record of a mail DNS sync.
type DNSRecordOutcome struct {
	Kind     string `json:"kind"` // MX, SPF, DKIM, DMARC, A
	Type     string `json:"type"`
	Name     string `json:"name"`
	Content  string `json:"content"`
	Action   string `json:"action"`
	Current  string `json:"current,omitempty"` // what the zone holds today
	Detail   string `json:"detail,omitempty"`
	Priority int    `json:"priority,omitempty"`
}

// OK reports whether the record is in the desired state.
func (o DNSRecordOutcome) OK() bool {
	return o.Action == ActionCreated || o.Action == ActionUpdated || o.Action == ActionUnchanged
}

// desiredRecord is one record we want in the zone, plus how to recognise the
// existing record it should replace.
type desiredRecord struct {
	kind   string
	params DNSRecordParams
	// matches identifies the counterpart of this record among the zone's
	// existing records. It must be narrow enough to leave unrelated records of
	// the same type and name alone.
	matches func(*CloudflareDNSRecord) bool
}

// buildEmailRecords turns the mail settings into the records the zone needs.
func buildEmailRecords(domain string, params EmailDNSParams) []desiredRecord {
	applyEmailDefaults(domain, &params)

	priority := 10
	records := []desiredRecord{
		{
			kind: "MX",
			params: DNSRecordParams{
				Type: "MX", Name: domain, Content: params.MXRecord, TTL: 1,
				Priority: &priority, Comment: "Email server MX record",
			},
			// An MX is identified by the host it points at: other exchangers for
			// the same domain are left in place.
			matches: func(r *CloudflareDNSRecord) bool {
				return r.Type == "MX" && equalName(r.Name, domain) && equalName(r.Content, params.MXRecord)
			},
		},
		{
			kind: "SPF",
			params: DNSRecordParams{
				Type: "TXT", Name: domain, Content: strings.TrimSpace(params.SPFRecord), TTL: 1,
				Comment: "SPF record for email authentication",
			},
			// Only the TXT that is an SPF record; verification tokens at the same
			// name are none of our business.
			matches: func(r *CloudflareDNSRecord) bool {
				return r.Type == "TXT" && equalName(r.Name, domain) &&
					strings.HasPrefix(strings.ToLower(unquote(r.Content)), "v=spf1")
			},
		},
		{
			kind: "DMARC",
			params: DNSRecordParams{
				Type: "TXT", Name: "_dmarc." + domain, Content: strings.TrimSpace(params.DMARCRecord), TTL: 1,
				Comment: "DMARC policy for email authentication",
			},
			matches: func(r *CloudflareDNSRecord) bool {
				return r.Type == "TXT" && equalName(r.Name, "_dmarc."+domain) &&
					strings.HasPrefix(strings.ToLower(unquote(r.Content)), "v=dmarc1")
			},
		},
	}

	if params.DKIMRecord != "" {
		dkimName := params.DKIMSelector + "._domainkey." + domain
		records = append(records, desiredRecord{
			kind: "DKIM",
			params: DNSRecordParams{
				Type: "TXT", Name: dkimName, Content: cleanDKIM(params.DKIMRecord), TTL: 1,
				Comment: "DKIM public key for email signing",
			},
			matches: func(r *CloudflareDNSRecord) bool {
				return r.Type == "TXT" && equalName(r.Name, dkimName)
			},
		})
	}

	// The mail host greets other servers under its own name, and that name is
	// an SPF identity of its own. Without a record here a receiver reports
	// "HELO does not publish an SPF record" — a small penalty, and one of the
	// few remaining marks against an otherwise authenticated message.
	heloHost := params.MXRecord
	records = append(records, desiredRecord{
		kind: "SPF-HELO",
		params: DNSRecordParams{
			Type: "TXT", Name: heloHost, Content: "v=spf1 a -all", TTL: 1,
			Comment: "SPF record for the mail host's HELO identity",
		},
		matches: func(r *CloudflareDNSRecord) bool {
			return r.Type == "TXT" && equalName(r.Name, heloHost) &&
				strings.HasPrefix(strings.ToLower(unquote(r.Content)), "v=spf1")
		},
	})

	if params.MailServerIP != "" {
		mailHost := params.MXRecord
		proxied := false
		records = append(records, desiredRecord{
			kind: "A",
			params: DNSRecordParams{
				Type: "A", Name: mailHost, Content: params.MailServerIP, TTL: 1,
				Proxied: &proxied, Comment: "Mail server A record",
			},
			// The mail host must resolve to this server; another A record at the
			// same name would be a second address for the same host, so match on
			// the name and replace the address.
			matches: func(r *CloudflareDNSRecord) bool {
				return r.Type == "A" && equalName(r.Name, mailHost)
			},
		})
	}

	return records
}

// applyEmailDefaults fills in the values the caller did not set.
func applyEmailDefaults(domain string, params *EmailDNSParams) {
	if params.DKIMSelector == "" {
		params.DKIMSelector = "mail"
	}
	if params.MXRecord == "" {
		params.MXRecord = "mail." + domain
	}
	if params.SPFRecord == "" {
		if params.MailServerIP != "" {
			params.SPFRecord = fmt.Sprintf("v=spf1 a mx ip4:%s -all", params.MailServerIP)
		} else {
			params.SPFRecord = "v=spf1 a mx ~all"
		}
	}
	if params.DMARCRecord == "" {
		params.DMARCRecord = fmt.Sprintf("v=DMARC1; p=none; rua=mailto:postmaster@%s", domain)
	}
}

// PlanEmailDNSRecords reports what a sync would do, changing nothing. This is
// the safe way to see which records are already correct, which differ and which
// are missing.
func (m *CloudflareManager) PlanEmailDNSRecords(zoneID string, params EmailDNSParams) ([]DNSRecordOutcome, error) {
	zone, err := m.GetZone(zoneID)
	if err != nil {
		return nil, err
	}

	existing, err := m.ListDNSRecords(zone.ZoneID)
	if err != nil {
		return nil, fmt.Errorf("failed to read the zone: %w", err)
	}

	outcomes := make([]DNSRecordOutcome, 0, 5)
	for _, desired := range buildEmailRecords(zone.Name, params) {
		outcome := DNSRecordOutcome{
			Kind: desired.kind, Type: desired.params.Type,
			Name: desired.params.Name, Content: desired.params.Content,
		}
		if desired.params.Priority != nil {
			outcome.Priority = *desired.params.Priority
		}

		match := findMatch(existing, desired)
		switch {
		case match == nil:
			outcome.Action = ActionMissing
		case sameContent(match, desired.params):
			outcome.Action = ActionUnchanged
			outcome.Current = unquote(match.Content)
		default:
			outcome.Action = ActionDiffers
			outcome.Current = unquote(match.Content)
		}
		outcomes = append(outcomes, outcome)
	}
	return outcomes, nil
}

// SyncEmailDNSRecords brings the zone's mail records to the desired state,
// creating what is missing and updating what differs. Records it did not put
// there are never deleted.
func (m *CloudflareManager) SyncEmailDNSRecords(zoneID string, params EmailDNSParams) ([]DNSRecordOutcome, error) {
	zone, err := m.GetZone(zoneID)
	if err != nil {
		return nil, err
	}

	existing, err := m.ListDNSRecords(zone.ZoneID)
	if err != nil {
		return nil, fmt.Errorf("failed to read the zone: %w", err)
	}

	outcomes := make([]DNSRecordOutcome, 0, 5)
	for _, desired := range buildEmailRecords(zone.Name, params) {
		outcome := DNSRecordOutcome{
			Kind: desired.kind, Type: desired.params.Type,
			Name: desired.params.Name, Content: desired.params.Content,
		}
		if desired.params.Priority != nil {
			outcome.Priority = *desired.params.Priority
		}

		match := findMatch(existing, desired)
		switch {
		case match == nil:
			if _, err := m.CreateDNSRecord(zone.ZoneID, desired.params); err != nil {
				outcome.Action = ActionFailed
				outcome.Detail = err.Error()
			} else {
				outcome.Action = ActionCreated
			}

		case sameContent(match, desired.params):
			outcome.Action = ActionUnchanged
			outcome.Current = unquote(match.Content)

		default:
			outcome.Current = unquote(match.Content)
			if _, err := m.UpdateDNSRecord(zone.ZoneID, match.RecordID, desired.params); err != nil {
				outcome.Action = ActionFailed
				outcome.Detail = err.Error()
			} else {
				outcome.Action = ActionUpdated
			}
		}
		outcomes = append(outcomes, outcome)
	}
	return outcomes, nil
}

// findMatch returns the existing record a desired one should replace.
func findMatch(existing []*CloudflareDNSRecord, desired desiredRecord) *CloudflareDNSRecord {
	for _, record := range existing {
		if record == nil {
			continue
		}
		if desired.matches(record) {
			return record
		}
	}
	return nil
}

// sameContent reports whether an existing record already says what we want.
//
// How to compare depends on the type: a hostname is case-insensitive and may
// carry a trailing dot, while TXT content must match exactly — a DKIM key is
// base64, where "A" and "a" are different keys.
func sameContent(record *CloudflareDNSRecord, params DNSRecordParams) bool {
	current := strings.TrimSpace(unquote(record.Content))
	desired := strings.TrimSpace(unquote(params.Content))

	switch strings.ToUpper(params.Type) {
	case "MX", "CNAME", "NS", "PTR":
		if !equalName(current, desired) {
			return false
		}
	default:
		if current != desired {
			return false
		}
	}

	if params.Priority != nil && record.Priority != *params.Priority {
		return false
	}
	return true
}

// unquote strips the quoting a resolver or API may wrap TXT content in, and
// joins the pieces of a long record split across strings.
func unquote(value string) string {
	value = strings.TrimSpace(value)
	if !strings.Contains(value, "\"") {
		return value
	}

	var b strings.Builder
	inQuotes := false
	for _, r := range value {
		if r == '"' {
			inQuotes = !inQuotes
			continue
		}
		if inQuotes {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return value
	}
	return b.String()
}

// cleanDKIM squeezes a DKIM record onto one line: resolvers and the API split
// long values differently, and stray whitespace breaks verification.
func cleanDKIM(record string) string {
	replacer := strings.NewReplacer("\n", "", "\r", "", "\t", "")
	return strings.TrimSpace(replacer.Replace(record))
}

// equalName compares DNS names ignoring case and any trailing dot.
func equalName(a, b string) bool {
	return strings.EqualFold(strings.TrimSuffix(strings.TrimSpace(a), "."),
		strings.TrimSuffix(strings.TrimSpace(b), "."))
}
