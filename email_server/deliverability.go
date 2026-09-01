package email_server

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"blitiri.com.ar/go/spf"
	"redock/platform/memory"
)

// Whether a message lands in the inbox or in spam is decided almost entirely by
// things outside the message: does the sending IP have matching reverse DNS,
// does SPF authorise it, is the DKIM key actually published, does the sender
// domain look routable. Those are all checkable, so the server checks them
// rather than leaving the operator to guess.

// CheckLevel grades a finding.
const (
	CheckOK   = "ok"
	CheckWarn = "warning"
	CheckFail = "fail"
	CheckUnkn = "unknown"
)

// DeliverabilityCheck is one verdict.
type DeliverabilityCheck struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Level  string `json:"level"`  // ok | warning | fail | unknown
	Detail string `json:"detail"` // what was found
	Advice string `json:"advice,omitempty"`
	Domain string `json:"domain,omitempty"`
}

// DeliverabilityReport groups the host-level and per-domain findings.
type DeliverabilityReport struct {
	CheckedAt time.Time             `json:"checked_at"`
	Hostname  string                `json:"hostname"`
	IPAddress string                `json:"ip_address"`
	Checks    []DeliverabilityCheck `json:"checks"`
	// Score is how many checks passed out of how many ran, for a quick glance.
	Passed int `json:"passed"`
	Total  int `json:"total"`
}

const lookupTimeout = 8 * time.Second

// CheckDeliverability inspects the public DNS and reverse DNS this server
// depends on, and reports what a receiving system like Gmail would see.
func (m *EmailManager) CheckDeliverability() DeliverabilityReport {
	cfg := m.nativeConfig()

	report := DeliverabilityReport{
		CheckedAt: time.Now(),
		Hostname:  cfg.Hostname,
		IPAddress: cfg.IPAddress,
	}

	report.Checks = append(report.Checks, m.checkHostname(cfg)...)

	domains := memory.FindAll[*EmailDomain](m.db, "email_domains")
	sort.Slice(domains, func(i, j int) bool { return domains[i].Domain < domains[j].Domain })
	for _, domain := range domains {
		if domain == nil || domain.IsDeleted() || !domain.Enabled {
			continue
		}
		report.Checks = append(report.Checks, m.checkDomain(cfg, domain)...)
	}

	for _, check := range report.Checks {
		report.Total++
		if check.Level == CheckOK {
			report.Passed++
		}
	}
	return report
}

// checkHostname covers the sending identity: the name used in HELO, its
// address, and the reverse DNS of that address.
func (m *EmailManager) checkHostname(cfg EmailServerConfig) []DeliverabilityCheck {
	checks := []DeliverabilityCheck{}

	helo := cfg.OutboundHELO
	if helo == "" {
		helo = cfg.Hostname
	}

	if !isPublicName(helo) {
		return append(checks, DeliverabilityCheck{
			ID:     "helo",
			Title:  "HELO name",
			Level:  CheckFail,
			Detail: fmt.Sprintf("%q is not a public hostname", helo),
			Advice: "Set the mail hostname to a real FQDN (mail.<your domain>); receivers distrust a HELO they cannot resolve.",
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), lookupTimeout)
	defer cancel()

	// Does the HELO name resolve, and to which address?
	heloAddrs, err := net.DefaultResolver.LookupHost(ctx, helo)
	switch {
	case err != nil || len(heloAddrs) == 0:
		checks = append(checks, DeliverabilityCheck{
			ID:     "helo",
			Title:  "HELO name resolves",
			Level:  CheckFail,
			Detail: helo + " does not resolve",
			Advice: "Publish an A record for " + helo + " pointing at this server.",
		})
	default:
		checks = append(checks, DeliverabilityCheck{
			ID:     "helo",
			Title:  "HELO name resolves",
			Level:  CheckOK,
			Detail: helo + " → " + strings.Join(heloAddrs, ", "),
		})
	}

	sendingIP := cfg.IPAddress
	if sendingIP == "" {
		return append(checks, DeliverabilityCheck{
			ID:     "ptr",
			Title:  "Reverse DNS (PTR)",
			Level:  CheckUnkn,
			Detail: "the server's public address is not set, so its PTR cannot be checked",
			Advice: "Set the public IP in Listeners & TLS.",
		})
	}

	// PTR: the single strongest signal after SPF/DKIM. A hosting provider's
	// default name (…​.clients.example-hoster.tld) reads as an unconfigured
	// machine and is treated accordingly.
	names, err := net.DefaultResolver.LookupAddr(ctx, sendingIP)
	switch {
	case err != nil || len(names) == 0:
		checks = append(checks, DeliverabilityCheck{
			ID:     "ptr",
			Title:  "Reverse DNS (PTR)",
			Level:  CheckFail,
			Detail: sendingIP + " has no PTR record",
			Advice: "Set the PTR for this address to " + helo + " in your hosting provider's panel.",
		})
	default:
		ptr := strings.TrimSuffix(names[0], ".")
		if strings.EqualFold(ptr, helo) {
			checks = append(checks, DeliverabilityCheck{
				ID:     "ptr",
				Title:  "Reverse DNS (PTR)",
				Level:  CheckOK,
				Detail: sendingIP + " → " + ptr,
			})
		} else {
			checks = append(checks, DeliverabilityCheck{
				ID:     "ptr",
				Title:  "Reverse DNS (PTR)",
				Level:  CheckFail,
				Detail: fmt.Sprintf("%s → %s, which does not match the HELO name %s", sendingIP, ptr, helo),
				Advice: "Set the PTR to " + helo + " in your hosting provider's panel. Large providers weigh this heavily.",
			})
		}

		// Forward-confirmed reverse DNS: the PTR name must resolve back to the
		// same address.
		if forward, err := net.DefaultResolver.LookupHost(ctx, ptr); err == nil {
			confirmed := false
			for _, addr := range forward {
				if addr == sendingIP {
					confirmed = true
					break
				}
			}
			level := CheckOK
			detail := ptr + " resolves back to " + sendingIP
			advice := ""
			if !confirmed {
				level = CheckWarn
				detail = ptr + " does not resolve back to " + sendingIP
				advice = "Forward and reverse DNS must agree; publish an A record for " + ptr + "."
			}
			checks = append(checks, DeliverabilityCheck{
				ID: "fcrdns", Title: "Forward-confirmed reverse DNS", Level: level, Detail: detail, Advice: advice,
			})
		}
	}

	return checks
}

// checkDomain covers what a receiver looks up about the sender's domain.
func (m *EmailManager) checkDomain(cfg EmailServerConfig, domain *EmailDomain) []DeliverabilityCheck {
	checks := []DeliverabilityCheck{}
	name := domain.Domain

	ctx, cancel := context.WithTimeout(context.Background(), lookupTimeout)
	defer cancel()

	// MX. It does not gate outbound mail, but without it the domain cannot
	// receive — so bounces and delivery reports never arrive, and plenty of
	// receivers treat a sender domain with no MX as suspect.
	mxRecords, err := net.DefaultResolver.LookupMX(ctx, name)
	switch {
	case err != nil || len(mxRecords) == 0:
		checks = append(checks, DeliverabilityCheck{
			ID: "mx", Domain: name, Title: "MX record", Level: CheckFail,
			Detail: name + " has no MX record",
			Advice: "Publish MX for " + name + " pointing at " + m.mxHostFor(domain) +
				". Without it this domain cannot receive mail, so bounces and delivery reports are lost.",
		})
	default:
		hosts := make([]string, 0, len(mxRecords))
		for _, mx := range mxRecords {
			hosts = append(hosts, strings.TrimSuffix(mx.Host, "."))
		}
		checks = append(checks, DeliverabilityCheck{
			ID: "mx", Domain: name, Title: "MX record", Level: CheckOK,
			Detail: name + " → " + strings.Join(hosts, ", "),
		})
	}

	// SPF: evaluated for real against this server's address, not just "is a
	// record present".
	if cfg.IPAddress != "" {
		ip := net.ParseIP(cfg.IPAddress)
		sender := "postmaster@" + name
		result, spfErr := spf.CheckHostWithSender(ip, cfg.Hostname, sender, spf.WithContext(ctx))

		check := DeliverabilityCheck{ID: "spf", Domain: name, Title: "SPF authorises this server"}
		switch result {
		case spf.Pass:
			check.Level = CheckOK
			check.Detail = "SPF passes for " + cfg.IPAddress
		case spf.None:
			check.Level = CheckFail
			check.Detail = name + " publishes no SPF record"
			check.Advice = "Publish the SPF record shown in the DNS tab."
		case spf.Fail, spf.SoftFail:
			check.Level = CheckFail
			check.Detail = fmt.Sprintf("SPF returns %s for %s", result, cfg.IPAddress)
			check.Advice = "The SPF record does not authorise this server's address; publish the one shown in the DNS tab."
		default:
			check.Level = CheckWarn
			check.Detail = fmt.Sprintf("SPF result: %s", result)
			if spfErr != nil {
				check.Detail += " (" + spfErr.Error() + ")"
			}
		}
		checks = append(checks, check)
	}

	// DKIM: is the public half actually published, and does it match the key
	// this server signs with?
	selector := domain.DKIMSelector
	if selector == "" {
		selector = "mail"
	}
	dkimName := selector + "._domainkey." + name

	switch records, err := net.DefaultResolver.LookupTXT(ctx, dkimName); {
	case err != nil || len(records) == 0:
		level := CheckFail
		detail := dkimName + " has no TXT record"
		if domain.DKIMPrivateKey == "" {
			detail += " and this domain has no DKIM key yet"
		}
		checks = append(checks, DeliverabilityCheck{
			ID: "dkim", Domain: name, Title: "DKIM key published", Level: level, Detail: detail,
			Advice: "Publish the DKIM record shown in the DNS tab; unsigned mail is far more likely to be filtered.",
		})
	default:
		published := strings.Join(records, "")
		if publicKeyMatches(published, domain.DKIMPublicKey) {
			checks = append(checks, DeliverabilityCheck{
				ID: "dkim", Domain: name, Title: "DKIM key published", Level: CheckOK,
				Detail: "the published key matches the one used for signing",
			})
		} else {
			checks = append(checks, DeliverabilityCheck{
				ID: "dkim", Domain: name, Title: "DKIM key published", Level: CheckFail,
				Detail: "the published key does not match the one this server signs with",
				Advice: "Re-publish the DKIM record from the DNS tab; signatures made with the current key will fail otherwise.",
			})
		}
	}

	// DMARC: ties SPF and DKIM together and is what large receivers read first.
	switch records, err := net.DefaultResolver.LookupTXT(ctx, "_dmarc."+name); {
	case err != nil || len(records) == 0:
		checks = append(checks, DeliverabilityCheck{
			ID: "dmarc", Domain: name, Title: "DMARC policy", Level: CheckWarn,
			Detail: "_dmarc." + name + " has no TXT record",
			Advice: "Publish the DMARC record shown in the DNS tab, starting at p=none.",
		})
	default:
		record := strings.Join(records, " ")
		checks = append(checks, DeliverabilityCheck{
			ID: "dmarc", Domain: name, Title: "DMARC policy", Level: CheckOK,
			Detail: record,
		})
		checks = append(checks, m.checkDMARCReportAddresses(name, record)...)
	}

	return checks
}

// checkDMARCReportAddresses makes sure the reports a DMARC record asks for can
// actually be delivered.
//
// The record names an address; nothing guarantees that address exists. Every
// large receiver then sends its daily report there, this server answers "no
// such recipient", and the reports are silently lost — which is the one part of
// DMARC that tells you who is failing your policy.
func (m *EmailManager) checkDMARCReportAddresses(domain, record string) []DeliverabilityCheck {
	var checks []DeliverabilityCheck

	for _, tag := range []string{"rua", "ruf"} {
		for _, address := range dmarcReportAddresses(record, tag) {
			_, addressDomain := splitAddress(address)

			// An address in somebody else's domain is allowed, but that domain
			// has to say it accepts reports for ours, or receivers will not
			// send them.
			if !strings.EqualFold(addressDomain, domain) {
				checks = append(checks, DeliverabilityCheck{
					ID: "dmarc-" + tag, Domain: domain, Title: "DMARC report address", Level: CheckWarn,
					Detail: address + " is outside " + domain,
					Advice: "An external report address needs a record at " + domain +
						"._report._dmarc." + addressDomain + " or receivers will not send reports there.",
				})
				continue
			}

			if m.LookupAccount(address) == nil {
				checks = append(checks, DeliverabilityCheck{
					ID: "dmarc-" + tag, Domain: domain, Title: "DMARC report address", Level: CheckFail,
					Detail: "reports are addressed to " + address + ", which no mailbox or alias here accepts",
					Advice: "Create a mailbox for " + address + ", or add it as an alias to one that exists. " +
						"Until then every DMARC report for this domain is refused.",
				})
				continue
			}

			checks = append(checks, DeliverabilityCheck{
				ID: "dmarc-" + tag, Domain: domain, Title: "DMARC report address", Level: CheckOK,
				Detail: address + " accepts mail here",
			})
		}
	}

	return checks
}

// dmarcReportAddresses pulls the mailto addresses out of one DMARC tag.
//
// The syntax allows several comma-separated URIs and an optional size limit
// after an exclamation mark, as in "mailto:reports@example.com!10m".
func dmarcReportAddresses(record, tag string) []string {
	var addresses []string

	for _, part := range strings.Split(record, ";") {
		part = strings.TrimSpace(strings.Trim(part, `"`))
		if !strings.HasPrefix(strings.ToLower(part), tag+"=") {
			continue
		}

		for _, uri := range strings.Split(part[len(tag)+1:], ",") {
			uri = strings.TrimSpace(uri)
			if i := strings.Index(uri, "!"); i >= 0 {
				uri = uri[:i]
			}
			if !strings.HasPrefix(strings.ToLower(uri), "mailto:") {
				continue // only mailto is in practice ever used
			}
			if address := normalizeAddress(uri[len("mailto:"):]); address != "" {
				addresses = append(addresses, address)
			}
		}
	}

	return addresses
}

// publicKeyMatches compares the p= value of a published DKIM record with the
// one this server generated, ignoring formatting differences.
func publicKeyMatches(published, expected string) bool {
	extract := func(record string) string {
		for _, part := range strings.Split(record, ";") {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "p=") {
				key := strings.TrimPrefix(part, "p=")
				replacer := strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "", "\"", "")
				return replacer.Replace(key)
			}
		}
		return ""
	}

	publishedKey := extract(published)
	expectedKey := extract(expected)
	return publishedKey != "" && publishedKey == expectedKey
}
