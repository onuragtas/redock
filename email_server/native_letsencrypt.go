package email_server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"redock/api_gateway"
	"redock/platform/memory"
)

// A mail server needs a certificate whose name matches what clients dial —
// mail.example.com, not whatever the dashboard happens to serve. Rather than
// running a second ACME implementation, the mail server borrows the API
// Gateway's: same account key, same port-80 responder, its own certificate.

const (
	// mailCertFile / mailKeyFile are where the mail server's own certificate is
	// stored, separate from the gateway's.
	mailCertFile = "mail.crt"
	mailKeyFile  = "mail.key"
	// renewCheckInterval is how often the certificate is examined.
	renewCheckInterval = 12 * time.Hour
	// defaultRenewBeforeDays is used when the gateway has no renewal window set.
	defaultRenewBeforeDays = 30
)

// CertificateStatus describes the mail server's TLS certificate for the
// dashboard, including whether a Let's Encrypt request could succeed now.
type CertificateStatus struct {
	Source     string     `json:"source"` // configured, letsencrypt, self-signed
	Subject    string     `json:"subject,omitempty"`
	Issuer     string     `json:"issuer,omitempty"`
	Names      []string   `json:"names"`
	IPs        []string   `json:"ips"`
	NotAfter   *time.Time `json:"not_after,omitempty"`
	DaysLeft   int        `json:"days_left"`
	SelfSigned bool       `json:"self_signed"`
	// Wanted lists the names the certificate should cover; Missing is what it
	// does not, which is exactly why a client would refuse it.
	Wanted  []string `json:"wanted"`
	Missing []string `json:"missing"`
	// ACMEReady / ACMEReason explain whether "request a certificate" will work.
	ACMEReady  bool   `json:"acme_ready"`
	ACMEReason string `json:"acme_reason,omitempty"`
	CertPath   string `json:"cert_path,omitempty"`
	// NameChecks says, per name, whether Let's Encrypt could validate it.
	NameChecks []NameCheck `json:"name_checks,omitempty"`
}

// mailCertPaths returns where this server's own certificate lives.
func (m *EmailManager) mailCertPaths() (string, string) {
	dir := filepath.Join(m.dataPath, "tls")
	return filepath.Join(dir, mailCertFile), filepath.Join(dir, mailKeyFile)
}

// certificateWantedNames is what a Let's Encrypt certificate has to cover: the
// mail hostname plus mail.<domain> for every served domain. IP addresses cannot
// be included — no public CA issues for them — which is why a client dialling
// by IP will always have to trust the self-signed certificate instead.
func (m *EmailManager) certificateWantedNames() []string {
	cfg := m.nativeConfig()
	hostname := cfg.Hostname

	names := []string{}
	if isPublicName(hostname) {
		names = append(names, hostname)
	}

	if m.db != nil {
		for _, domain := range memory.FindAll[*EmailDomain](m.db, "email_domains") {
			if domain == nil || domain.IsDeleted() || !domain.Enabled {
				continue
			}

			// With a real server hostname every domain's MX points at it, so
			// that single name is all the certificate needs. Only a domain with
			// its own mail host — either an explicit override, or the fallback
			// used when no server hostname is set — adds a name here.
			mxHost := m.mxHostFor(domain)
			if mxHost != hostname && isPublicName(mxHost) {
				names = append(names, mxHost)
			}
		}
	}

	names = dedupeStrings(names)
	sort.Strings(names)
	return names
}

// isPublicName filters out the names a public CA will never issue for.
func isPublicName(name string) bool {
	name = strings.TrimSpace(strings.TrimSuffix(name, "."))
	if name == "" || !strings.Contains(name, ".") {
		return false
	}
	lower := strings.ToLower(name)
	for _, suffix := range []string{".localhost", ".local", ".internal", ".test", ".invalid", ".example"} {
		if strings.HasSuffix(lower, suffix) {
			return false
		}
	}
	return lower != "localhost"
}

// NameCheck is the verdict on one name before an ACME order is placed.
type NameCheck struct {
	Name       string   `json:"name"`
	Resolves   bool     `json:"resolves"`
	PointsAtUs bool     `json:"points_at_us"`
	Addresses  []string `json:"addresses,omitempty"`
	Reason     string   `json:"reason,omitempty"`
}

// checkNames resolves each wanted name and reports whether Let's Encrypt would
// be able to validate it. An ACME order fails as a whole if any single
// authorisation fails, so a domain whose DNS is not ready yet must be left out
// rather than allowed to block the certificate for every other domain.
func (m *EmailManager) checkNames(names []string) []NameCheck {
	ours := make(map[string]struct{})
	for _, ip := range m.certificateIPs(m.nativeConfig()) {
		ours[ip.String()] = struct{}{}
	}

	resolver := net.DefaultResolver
	checks := make([]NameCheck, 0, len(names))

	for _, name := range names {
		check := NameCheck{Name: name}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		addrs, err := resolver.LookupHost(ctx, name)
		cancel()

		if err != nil || len(addrs) == 0 {
			check.Reason = "does not resolve"
			checks = append(checks, check)
			continue
		}

		check.Resolves = true
		check.Addresses = addrs
		for _, addr := range addrs {
			if _, ok := ours[addr]; ok {
				check.PointsAtUs = true
				break
			}
		}
		if !check.PointsAtUs {
			check.Reason = "resolves to " + strings.Join(addrs, ", ") + ", which is not this server"
		}
		checks = append(checks, check)
	}
	return checks
}

// CertificateStatus reports on the active certificate.
func (m *EmailManager) CertificateStatus() CertificateStatus {
	cfg := m.nativeConfig()
	wanted := m.certificateWantedNames()

	status := CertificateStatus{
		Source: "self-signed",
		Wanted: wanted,
	}
	status.ACMEReady, status.ACMEReason = api_gateway.ACMEReady()

	n := m.Native()
	n.mu.Lock()
	certs := n.certs
	n.mu.Unlock()

	var pair *tls.Certificate
	if certs != nil {
		status.Source = certs.Source()
		if loaded, err := certs.certificate(); err == nil {
			pair = loaded
			status.Source = certs.Source()
		}
	}

	if pair == nil || len(pair.Certificate) == 0 {
		status.Missing = wanted
		return status
	}

	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		status.Missing = wanted
		return status
	}

	status.Subject = leaf.Subject.CommonName
	status.Issuer = leaf.Issuer.CommonName
	status.Names = leaf.DNSNames
	for _, ip := range leaf.IPAddresses {
		status.IPs = append(status.IPs, ip.String())
	}
	notAfter := leaf.NotAfter
	status.NotAfter = &notAfter
	status.DaysLeft = int(time.Until(notAfter).Hours() / 24)
	status.SelfSigned = leaf.Issuer.CommonName == leaf.Subject.CommonName

	for _, name := range wanted {
		if leaf.VerifyHostname(name) != nil {
			status.Missing = append(status.Missing, name)
		}
	}

	if cfg.SSLCertPath != "" {
		status.CertPath = cfg.SSLCertPath
	}
	if status.ACMEReady {
		status.NameChecks = m.checkNames(wanted)
	}
	return status
}

// RequestLetsEncryptCertificate obtains a certificate for the mail hostname
// through the API Gateway's ACME account, stores it beside the mail data and
// points the listeners at it.
func (m *EmailManager) RequestLetsEncryptCertificate() (CertificateStatus, error) {
	ready, reason := api_gateway.ACMEReady()
	if !ready {
		return m.CertificateStatus(), fmt.Errorf("%s", reason)
	}

	wanted := m.certificateWantedNames()
	if len(wanted) == 0 {
		return m.CertificateStatus(), fmt.Errorf(
			"no publicly resolvable name to request a certificate for — set the mail hostname to a real FQDN first")
	}

	// Order only the names that can actually pass validation: one unready name
	// would otherwise fail the order for all of them.
	checks := m.checkNames(wanted)
	names := make([]string, 0, len(checks))
	skipped := make([]string, 0, len(checks))
	for _, check := range checks {
		if check.PointsAtUs {
			names = append(names, check.Name)
			continue
		}
		skipped = append(skipped, check.Name+" ("+check.Reason+")")
	}

	if len(names) == 0 {
		return m.CertificateStatus(), fmt.Errorf(
			"none of these names point at this server yet: %s", strings.Join(skipped, "; "))
	}
	if len(skipped) > 0 {
		log.Printf("mail: leaving unready names out of the certificate order: %s", strings.Join(skipped, "; "))
		m.logMailEvent(mailEvent{
			Direction: "system",
			Status:    "certificate",
			Service:   "tls",
			Detail:    "names left out of the certificate request: " + strings.Join(skipped, "; "),
		})
	}

	settings := api_gateway.LetsEncryptSettings()
	workDir := api_gateway.GatewayWorkDir()
	if workDir == "" {
		workDir = m.workDir()
	}

	certPath, keyPath := m.mailCertPaths()
	if err := os.MkdirAll(filepath.Dir(certPath), 0700); err != nil {
		return m.CertificateStatus(), err
	}

	log.Printf("mail: requesting a Let's Encrypt certificate for %v", names)
	if err := api_gateway.ObtainCertificate(workDir, settings, names, certPath, keyPath); err != nil {
		return m.CertificateStatus(), fmt.Errorf("Let's Encrypt: %w", err)
	}

	// Point the mail server at its own certificate and rebind so it takes
	// effect immediately rather than on the next reload.
	m.mutex.Lock()
	m.config.SSLCertPath = certPath
	m.config.SSLKeyPath = keyPath
	m.config.SSLEnabled = true
	if err := memory.Update[*EmailServerConfig](m.db, "email_server_configs", m.config); err != nil {
		m.mutex.Unlock()
		return m.CertificateStatus(), fmt.Errorf("certificate obtained but not persisted: %w", err)
	}
	m.mutex.Unlock()

	if m.Native().IsRunning() {
		m.Native().Stop()
		if err := m.Native().Start(); err != nil {
			return m.CertificateStatus(), fmt.Errorf("certificate obtained but the server could not restart: %w", err)
		}
	}

	m.logMailEvent(mailEvent{
		Direction: "system",
		Status:    "certificate",
		Service:   "tls",
		Detail:    "Let's Encrypt certificate issued for " + strings.Join(names, ", "),
	})

	return m.CertificateStatus(), nil
}

// renewCertificateIfNeeded re-requests the certificate when it is close to
// expiry or no longer covers every name — the same rule the gateway uses.
func (m *EmailManager) renewCertificateIfNeeded() {
	cfg := m.nativeConfig()
	if cfg.SSLCertPath == "" {
		return // still on the self-signed certificate; nothing to renew
	}

	settings := api_gateway.LetsEncryptSettings()
	if settings == nil || !settings.AutoRenew {
		return
	}

	renewBefore := settings.RenewBeforeDays
	if renewBefore <= 0 {
		renewBefore = defaultRenewBeforeDays
	}

	status := m.CertificateStatus()
	needsRenewal := status.DaysLeft <= renewBefore || len(status.Missing) > 0
	if !needsRenewal {
		return
	}

	if ready, reason := api_gateway.ACMEReady(); !ready {
		log.Printf("mail: certificate renewal postponed: %s", reason)
		return
	}

	log.Printf("mail: renewing the certificate (%d days left, %d name(s) missing)", status.DaysLeft, len(status.Missing))
	if _, err := m.RequestLetsEncryptCertificate(); err != nil {
		log.Printf("mail: certificate renewal failed: %v", err)
	}
}

// startCertificateRenewal runs the renewal check on a slow ticker for as long
// as the server is up.
func (n *NativeServer) startCertificateRenewal(stop chan struct{}) {
	ticker := time.NewTicker(renewCheckInterval)
	defer ticker.Stop()

	// One check shortly after boot catches a certificate that expired while the
	// server was down.
	select {
	case <-stop:
		return
	case <-time.After(time.Minute):
		n.manager.renewCertificateIfNeeded()
	}

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			n.manager.renewCertificateIfNeeded()
		}
	}
}
