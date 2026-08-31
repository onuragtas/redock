package email_server

import (
	"fmt"
	"strings"

	"redock/pkg/security"
	"redock/platform/memory"
)

// UpdateNativeSettings persists listener/policy changes and restarts the
// native listeners when they are running, so a port or TLS change takes effect
// without the operator restarting Redock.
func (m *EmailManager) UpdateNativeSettings(updated EmailServerConfig) (*EmailServerConfig, error) {
	m.mutex.Lock()
	if m.config == nil {
		m.mutex.Unlock()
		return nil, fmt.Errorf("no configuration")
	}

	// Only the fields the native engine owns may be changed here; identity and
	// container fields keep their stored values.
	current := m.config
	current.Hostname = orDefault(updated.Hostname, current.Hostname)
	current.IPAddress = orDefault(updated.IPAddress, current.IPAddress)
	current.SMTPPort = updated.SMTPPort
	current.SMTPSPort = updated.SMTPSPort
	current.SubmissionPort = updated.SubmissionPort
	current.IMAPPort = updated.IMAPPort
	current.IMAPsPort = updated.IMAPsPort
	current.POP3Port = updated.POP3Port
	current.POP3sPort = updated.POP3sPort
	current.SSLEnabled = updated.SSLEnabled
	current.SSLCertPath = updated.SSLCertPath
	current.SSLKeyPath = updated.SSLKeyPath
	current.STARTTLSRequired = updated.STARTTLSRequired
	current.SMTPSEnabled = updated.SMTPSEnabled
	current.IMAPEnabled = updated.IMAPEnabled
	current.IMAPsEnabled = updated.IMAPsEnabled
	current.POP3Enabled = updated.POP3Enabled
	current.POP3sEnabled = updated.POP3sEnabled
	current.CheckSPF = updated.CheckSPF
	current.CheckDKIM = updated.CheckDKIM
	current.CheckDMARC = updated.CheckDMARC
	current.RejectOnDMARCFail = updated.RejectOnDMARCFail
	current.LogConnections = updated.LogConnections
	current.GuardEnabled = updated.GuardEnabled
	current.MaxAuthFailures = updated.MaxAuthFailures
	current.MaxConnectionsPerMinute = updated.MaxConnectionsPerMinute
	current.BlockMinutes = updated.BlockMinutes
	current.MaxMessageSize = updated.MaxMessageSize
	current.MaxRecipients = updated.MaxRecipients
	current.QueueMaxAttempts = updated.QueueMaxAttempts
	current.QueueRetryMinutes = updated.QueueRetryMinutes
	current.OutboundHELO = updated.OutboundHELO
	applyNativeDefaults(current)

	if err := memory.Update[*EmailServerConfig](m.db, "email_server_configs", current); err != nil {
		m.mutex.Unlock()
		return nil, fmt.Errorf("failed to persist settings: %w", err)
	}
	m.mutex.Unlock()

	if m.Native().IsRunning() {
		m.Native().Stop()
		if err := m.Native().Start(); err != nil {
			return m.GetConfig(), fmt.Errorf("settings saved but the server could not restart: %w", err)
		}
	}

	return m.GetConfig(), nil
}

// QueueItems exposes the outbound queue to the API.
func (m *EmailManager) QueueItems() []QueueItem { return m.queue().List() }

// FlushQueue retries every queued message now.
func (m *EmailManager) FlushQueue() int { return m.queue().Flush() }

// DeleteQueueItem drops one queued message.
func (m *EmailManager) DeleteQueueItem(id string) error { return m.queue().Delete(id) }

// DNSRecord is one record the operator has to publish for a domain.
type DNSRecord struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	Value    string `json:"value"`
	Priority int    `json:"priority,omitempty"`
	Note     string `json:"note,omitempty"`
}

// RequiredDNSRecords lists the MX/SPF/DKIM/DMARC records a domain needs. With
// the native engine there is no container to ask, so the values come from the
// server config and the domain's stored DKIM key.
func (m *EmailManager) RequiredDNSRecords(domain *EmailDomain) []DNSRecord {
	cfg := m.nativeConfig()

	selector := domain.DKIMSelector
	if selector == "" {
		selector = "mail"
	}

	records := []DNSRecord{
		{
			Type:     "MX",
			Name:     domain.Domain,
			Value:    cfg.Hostname,
			Priority: 10,
			Note:     "points mail for this domain at this server",
		},
		{
			Type:  "A",
			Name:  cfg.Hostname,
			Value: cfg.IPAddress,
			Note:  "the mail host must resolve, and its PTR should match",
		},
		{
			Type:  "TXT",
			Name:  domain.Domain,
			Value: orDefault(domain.SPFRecord, fmt.Sprintf("v=spf1 a:%s -all", cfg.Hostname)),
			Note:  "SPF: who may send for this domain",
		},
	}

	if domain.DKIMPublicKey != "" {
		records = append(records, DNSRecord{
			Type:  "TXT",
			Name:  fmt.Sprintf("%s._domainkey.%s", selector, domain.Domain),
			Value: domain.DKIMPublicKey,
			Note:  "DKIM public key; outgoing mail is signed with the matching private key",
		})
	}

	dmarc := domain.DMARCRecord
	if dmarc == "" {
		dmarc = fmt.Sprintf("v=DMARC1; p=none; rua=mailto:postmaster@%s", domain.Domain)
	}
	records = append(records, DNSRecord{
		Type:  "TXT",
		Name:  "_dmarc." + domain.Domain,
		Value: dmarc,
		Note:  "DMARC policy; start at p=none and tighten once SPF/DKIM pass",
	})

	return records
}

// NativeSelfTest performs a quick, read-only sanity check of the native setup
// and returns human-readable findings for the dashboard.
func (m *EmailManager) NativeSelfTest() []string {
	cfg := m.nativeConfig()
	findings := make([]string, 0, 8)

	if cfg.Hostname == "" || cfg.Hostname == "localhost" {
		findings = append(findings, "hostname is not set to a real FQDN; receiving servers will distrust the HELO name")
	}

	status := m.Native().Status()
	if !status.Running {
		findings = append(findings, "the mail server is not running")
	}
	if status.CertSource == "self-signed" {
		findings = append(findings, "TLS is using a self-signed certificate; clients will warn until a real certificate is issued")
	}

	domains := memory.FindAll[*EmailDomain](m.db, "email_domains")
	for _, domain := range domains {
		if domain == nil || domain.IsDeleted() || !domain.Enabled {
			continue
		}
		if domain.DKIMPrivateKey == "" {
			findings = append(findings, fmt.Sprintf("%s has no DKIM key; outgoing mail will not be signed", domain.Domain))
		}
	}

	if len(findings) == 0 {
		findings = append(findings, "no problems found")
	}
	return findings
}

// ensureDomainDKIM generates a DKIM key for a domain that has none, so mail
// leaving the native engine is always signed.
func (m *EmailManager) ensureDomainDKIM(domain *EmailDomain) error {
	if domain.DKIMPrivateKey != "" {
		return nil
	}

	privateKey, publicTXT, err := generateDKIMKeys()
	if err != nil {
		return err
	}

	// Stored encrypted at rest, matching AddDomain.
	stored := privateKey
	if len(m.encryptionKey) > 0 {
		encrypted, err := security.EncryptAES256GCM(privateKey, m.encryptionKey)
		if err != nil {
			return fmt.Errorf("failed to encrypt DKIM key: %w", err)
		}
		stored = encrypted
	}
	domain.DKIMPrivateKey = stored
	domain.DKIMPublicKey = publicTXT
	if domain.DKIMSelector == "" {
		domain.DKIMSelector = "mail"
	}
	if domain.SPFRecord == "" {
		domain.SPFRecord = fmt.Sprintf("v=spf1 a:%s -all", m.nativeConfig().Hostname)
	}
	if domain.DMARCRecord == "" {
		domain.DMARCRecord = fmt.Sprintf("v=DMARC1; p=none; rua=mailto:postmaster@%s", domain.Domain)
	}

	return memory.Update(m.db, "email_domains", domain)
}

// NativeSummaryLine is a compact one-line description of the running engine,
// used in logs and the dashboard header.
func (m *EmailManager) NativeSummaryLine() string {
	status := m.Native().Status()
	if !status.Running {
		return "native engine stopped"
	}

	parts := make([]string, 0, len(status.Listeners))
	for _, l := range status.Listeners {
		parts = append(parts, fmt.Sprintf("%s:%d", l.Name, l.Port))
	}
	return fmt.Sprintf("native engine on %s (TLS: %s)", strings.Join(parts, " "), status.CertSource)
}

// ShouldAutoStart reports whether the mail listeners should come up at boot:
// either the operator had them running, or at least one mail domain exists.
func (m *EmailManager) ShouldAutoStart() bool {
	if m.config != nil && m.config.IsRunning {
		return true
	}

	domains := memory.FindAll[*EmailDomain](m.db, "email_domains")
	for _, domain := range domains {
		if domain != nil && !domain.IsDeleted() && domain.Enabled {
			return true
		}
	}
	return false
}
