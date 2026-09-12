package email_server

import "strings"

// A DNS block list answers for the connecting machine; this list answers for
// the envelope sender. The two catch different things: a domain that has never
// sent from the same address twice is invisible to a reputation list, and an
// operator who simply wants nothing from a particular domain should not have
// to write the same rule into every mailbox's filters.
//
// The check runs at MAIL FROM, so a refused sender never transfers a body.

// senderRule is one parsed entry of the blocked-sender list.
type senderRule struct {
	// value is the domain, or the full address when address is true.
	value string
	// address means the entry named one mailbox rather than a whole domain.
	address bool
}

// parseBlockedSenders reads the configured list. Entries are separated by
// commas, semicolons or newlines, the same way the DNSBL zones are.
//
// An entry is one of:
//
//	example.com     — the domain and every subdomain of it
//	*.example.com   — the same thing written the way a wildcard usually is
//	bad@example.com — that one address only
func parseBlockedSenders(configured string) []senderRule {
	if strings.TrimSpace(configured) == "" {
		return nil
	}

	rules := make([]senderRule, 0, 8)
	seen := make(map[string]bool)

	for _, field := range strings.FieldsFunc(configured, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == ' ' || r == '\t'
	}) {
		entry := strings.ToLower(strings.TrimSpace(field))
		entry = strings.Trim(entry, ".")
		// "@example.com" and "*.example.com" are both how people write a domain
		// rule by hand; neither is a separate meaning.
		entry = strings.TrimPrefix(entry, "*.")
		if strings.HasPrefix(entry, "@") {
			entry = strings.TrimPrefix(entry, "@")
		}
		if entry == "" {
			continue
		}

		rule := senderRule{value: entry, address: strings.Contains(entry, "@")}
		// A bare "@" or an address with nothing either side of it blocks
		// everything; that is never what the operator meant.
		if rule.address {
			local, domain, _ := strings.Cut(entry, "@")
			if local == "" || domain == "" {
				continue
			}
		}
		if seen[entry] {
			continue
		}
		seen[entry] = true
		rules = append(rules, rule)
	}
	return rules
}

// blockedSender reports which entry, if any, refuses this envelope sender.
// The empty envelope sender — the null sender a bounce carries — matches
// nothing: refusing it would break delivery-status notifications.
func blockedSender(from string, rules []senderRule) (string, bool) {
	address := strings.ToLower(strings.TrimSpace(from))
	if address == "" || len(rules) == 0 {
		return "", false
	}

	_, domain, ok := strings.Cut(address, "@")
	if !ok || domain == "" {
		return "", false
	}
	domain = strings.Trim(domain, ".")

	for _, rule := range rules {
		if rule.address {
			if rule.value == address {
				return rule.value, true
			}
			continue
		}
		// A domain rule covers the domain itself and everything under it, so a
		// sender rotating through subdomains is blocked by one entry.
		if domain == rule.value || strings.HasSuffix(domain, "."+rule.value) {
			return rule.value, true
		}
	}
	return "", false
}

// blockedSenderRules returns the parsed list for the current configuration.
func (m *EmailManager) blockedSenderRules(cfg EmailServerConfig) []senderRule {
	return parseBlockedSenders(cfg.BlockedSenderDomains)
}

// normalizeBlockedSenders tidies what the dashboard sent before it is stored:
// one entry per line, lower-cased, duplicates and stray punctuation gone. The
// operator gets back a list they can read, and the stored value only changes
// when the set of rules actually changes.
func normalizeBlockedSenders(configured string) string {
	rules := parseBlockedSenders(configured)
	if len(rules) == 0 {
		return ""
	}

	entries := make([]string, 0, len(rules))
	for _, rule := range rules {
		entries = append(entries, rule.value)
	}
	return strings.Join(entries, "\n")
}
