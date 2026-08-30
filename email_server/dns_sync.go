package email_server

import (
	"fmt"
	"log"
	"strings"
	"time"

	"redock/cloudflare"
	"redock/platform/memory"
)

// DNSSyncResult reports what publishing a domain's mail records did.
type DNSSyncResult struct {
	Domain  string `json:"domain"`
	Synced  bool   `json:"synced"`
	ZoneID  string `json:"zone_id,omitempty"`
	Message string `json:"message"`
}

// SyncDomainDNS publishes a domain's MX, SPF, DKIM and DMARC records to
// Cloudflare when the domain is a zone on a connected account. This is the
// automation the container-based setup had, kept intact: adding a domain still
// wires its DNS up by itself, and the dashboard can re-run it on demand.
func (m *EmailManager) SyncDomainDNS(domain *EmailDomain) DNSSyncResult {
	result := DNSSyncResult{Domain: domain.Domain}

	cfManager := cloudflare.GetManager()
	if cfManager == nil {
		result.Message = "Cloudflare is not configured"
		return result
	}

	zone := m.findCloudflareZone(domain.Domain)
	if zone == nil {
		result.Message = fmt.Sprintf("%s is not a zone on any connected Cloudflare account", domain.Domain)
		return result
	}
	result.ZoneID = zone.ZoneID

	// Make sure the domain has a key before its public half is published.
	if err := m.ensureDomainDKIM(domain); err != nil {
		result.Message = "could not prepare the DKIM key: " + err.Error()
		return result
	}

	cfg := m.nativeConfig()
	params := cloudflare.EmailDNSParams{
		MXRecord:     m.mxHostFor(domain),
		SPFRecord:    domain.SPFRecord,
		DKIMSelector: domain.DKIMSelector,
		DKIMRecord:   domain.DKIMPublicKey,
		DMARCRecord:  domain.DMARCRecord,
		MailServerIP: cfg.IPAddress,
	}

	if err := cfManager.CreateEmailDNSRecords(zone.ZoneID, params); err != nil {
		result.Message = "Cloudflare rejected the records: " + err.Error()
		return result
	}

	domain.DNSConfigured = true
	domain.MXRecord = params.MXRecord
	domain.LastSync = timePtr(time.Now())
	if err := memory.Update(m.db, "email_domains", domain); err != nil {
		log.Printf("email_server: could not record the DNS sync for %s: %v", domain.Domain, err)
	}

	result.Synced = true
	result.Message = "MX, SPF, DKIM and DMARC records published"
	return result
}

// SyncAllDomainsDNS publishes the records of every enabled domain.
func (m *EmailManager) SyncAllDomainsDNS() []DNSSyncResult {
	domains := memory.FindAll[*EmailDomain](m.db, "email_domains")
	results := make([]DNSSyncResult, 0, len(domains))

	for _, domain := range domains {
		if domain == nil || domain.IsDeleted() || !domain.Enabled {
			continue
		}
		results = append(results, m.SyncDomainDNS(domain))
	}
	return results
}

// SyncDomainDNSAsync runs the sync in the background, which is what domain
// creation and updates want: DNS propagation must never block the response.
func (m *EmailManager) SyncDomainDNSAsync(domain *EmailDomain) {
	go func() {
		result := m.SyncDomainDNS(domain)
		if result.Synced {
			log.Printf("email_server: Cloudflare DNS for %s updated", result.Domain)
			return
		}
		if result.Message != "" {
			log.Printf("email_server: Cloudflare DNS for %s skipped: %s", result.Domain, result.Message)
		}
	}()
}

// findCloudflareZone locates the Cloudflare zone that owns a domain. A mail
// domain may be a subdomain of the zone, so parent zones are considered too.
func (m *EmailManager) findCloudflareZone(domainName string) *cloudflare.CloudflareZone {
	if m.db == nil {
		return nil
	}

	domainName = strings.ToLower(strings.TrimSuffix(domainName, "."))
	zones := memory.FindAll[*cloudflare.CloudflareZone](m.db, "cloudflare_zones")

	var best *cloudflare.CloudflareZone
	for _, zone := range zones {
		if zone == nil {
			continue
		}
		name := strings.ToLower(strings.TrimSuffix(zone.Name, "."))
		if name == "" {
			continue
		}
		if name != domainName && !strings.HasSuffix(domainName, "."+name) {
			continue
		}
		// Prefer the most specific matching zone.
		if best == nil || len(name) > len(best.Name) {
			best = zone
		}
	}
	return best
}

// mxHostFor is the hostname a domain's MX record should point at: the server's
// own hostname when it is set to a real name, otherwise mail.<domain>.
func (m *EmailManager) mxHostFor(domain *EmailDomain) string {
	if domain.MXRecord != "" {
		return domain.MXRecord
	}

	hostname := m.nativeConfig().Hostname
	if hostname != "" && strings.Contains(hostname, ".") && !strings.HasSuffix(hostname, ".localhost") {
		return hostname
	}
	return "mail." + domain.Domain
}
