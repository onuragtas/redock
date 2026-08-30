package email_server

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"redock/pkg/security"

	"blitiri.com.ar/go/spf"
	"github.com/emersion/go-msgauth/dkim"
	"github.com/emersion/go-msgauth/dmarc"
)

// signMessage DKIM-signs an outgoing message with the sending domain's key.
// The key already lives on EmailDomain (the dashboard generates it), so this
// replaces what OpenDKIM did inside the container.
func (m *EmailManager) signMessage(domainName string, raw []byte) ([]byte, error) {
	domain := m.LookupDomain(domainName)
	if domain == nil || domain.DKIMPrivateKey == "" {
		return raw, nil // nothing to sign with; not an error
	}

	key, err := m.domainDKIMKey(domain)
	if err != nil {
		return raw, fmt.Errorf("dkim: unusable private key for %s: %w", domainName, err)
	}

	selector := domain.DKIMSelector
	if selector == "" {
		selector = "mail"
	}

	options := &dkim.SignOptions{
		Domain:   domain.Domain,
		Selector: selector,
		Signer:   key,
		Hash:     crypto.SHA256,
		HeaderKeys: []string{
			"From", "To", "Cc", "Subject", "Date", "Message-ID",
			"MIME-Version", "Content-Type", "Content-Transfer-Encoding",
		},
	}

	var signed bytes.Buffer
	if err := dkim.Sign(&signed, bytes.NewReader(raw), options); err != nil {
		return raw, fmt.Errorf("dkim: signing failed for %s: %w", domainName, err)
	}
	return signed.Bytes(), nil
}

// domainDKIMKey returns a domain's signing key. Keys are stored encrypted at
// rest (AES-GCM with the mail master key); a value that is already PEM is
// accepted too, so keys written before encryption existed still work.
func (m *EmailManager) domainDKIMKey(domain *EmailDomain) (*rsa.PrivateKey, error) {
	stored := domain.DKIMPrivateKey

	if len(m.encryptionKey) > 0 {
		if decrypted, err := security.DecryptAES256GCM(stored, m.encryptionKey); err == nil {
			return parseRSAPrivateKey(decrypted)
		}
	}
	return parseRSAPrivateKey(stored)
}

func parseRSAPrivateKey(pemData string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, fmt.Errorf("not a PEM block")
	}

	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA key")
	}
	return key, nil
}

// AuthResults is what the inbound checks concluded about a message. It is
// stored on the log entry and written into an Authentication-Results header,
// so the webmail can show why something looked suspicious.
type AuthResults struct {
	SPF       string `json:"spf"`   // pass, fail, softfail, neutral, none, temperror, permerror
	DKIM      string `json:"dkim"`  // pass, fail, none
	DMARC     string `json:"dmarc"` // pass, fail, none
	SPFDetail string `json:"spf_detail,omitempty"`
	// Reject is set when policy says the message should not be accepted.
	Reject bool   `json:"reject"`
	Reason string `json:"reason,omitempty"`
}

// checkInbound runs SPF, DKIM and DMARC over a received message. Failures are
// reported, not fatal: only an explicit DMARC reject policy sets Reject.
func (m *EmailManager) checkInbound(remoteIP net.IP, heloName, mailFrom string, raw []byte) AuthResults {
	results := AuthResults{SPF: "none", DKIM: "none", DMARC: "none"}

	cfg := m.nativeConfig()
	_, envelopeDomain := splitAddress(normalizeAddress(mailFrom))

	if cfg.CheckSPF && remoteIP != nil && envelopeDomain != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		result, err := spf.CheckHostWithSender(remoteIP, heloName, normalizeAddress(mailFrom), spf.WithContext(ctx))
		cancel()

		results.SPF = string(result)
		if err != nil {
			results.SPFDetail = err.Error()
		}
	}

	dkimDomain := ""
	if cfg.CheckDKIM {
		verifications, err := dkim.Verify(bytes.NewReader(raw))
		switch {
		case err != nil:
			results.DKIM = "permerror"
		case len(verifications) == 0:
			results.DKIM = "none"
		default:
			results.DKIM = "fail"
			for _, v := range verifications {
				if v.Err == nil {
					results.DKIM = "pass"
					dkimDomain = v.Domain
					break
				}
			}
		}
	}

	if cfg.CheckDMARC {
		headerDomain := headerFromDomain(raw)
		if headerDomain == "" {
			headerDomain = envelopeDomain
		}
		results.DMARC, results.Reject, results.Reason = evaluateDMARC(headerDomain, envelopeDomain, dkimDomain, results)
	}

	if !cfg.RejectOnDMARCFail {
		results.Reject = false
	}
	return results
}

// evaluateDMARC applies the sender's published DMARC policy to the SPF/DKIM
// outcome, with relaxed alignment (the common default).
func evaluateDMARC(headerDomain, envelopeDomain, dkimDomain string, results AuthResults) (string, bool, string) {
	if headerDomain == "" {
		return "none", false, ""
	}

	record, err := dmarc.Lookup(headerDomain)
	if err != nil || record == nil {
		return "none", false, ""
	}

	spfAligned := results.SPF == "pass" && domainsAligned(headerDomain, envelopeDomain)
	dkimAligned := results.DKIM == "pass" && domainsAligned(headerDomain, dkimDomain)
	if spfAligned || dkimAligned {
		return "pass", false, ""
	}

	if record.Policy == dmarc.PolicyReject {
		return "fail", true, fmt.Sprintf("DMARC policy for %s is reject and neither SPF nor DKIM aligned", headerDomain)
	}
	return "fail", false, ""
}

// domainsAligned implements relaxed DMARC alignment: equal, or one is an
// organisational-domain suffix of the other.
func domainsAligned(a, b string) bool {
	a, b = strings.ToLower(strings.TrimSuffix(a, ".")), strings.ToLower(strings.TrimSuffix(b, "."))
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	return strings.HasSuffix(a, "."+b) || strings.HasSuffix(b, "."+a)
}

// headerFromDomain extracts the domain of the From: header, which is what
// DMARC is evaluated against.
func headerFromDomain(raw []byte) string {
	header := raw
	if idx := bytes.Index(raw, []byte("\r\n\r\n")); idx >= 0 {
		header = raw[:idx]
	} else if idx := bytes.Index(raw, []byte("\n\n")); idx >= 0 {
		header = raw[:idx]
	}

	for _, line := range strings.Split(string(header), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToLower(trimmed), "from:") {
			continue
		}
		_, domain := splitAddress(normalizeAddress(trimmed[len("from:"):]))
		return domain
	}
	return ""
}

// authResultsHeader renders the Authentication-Results header prepended to
// accepted mail.
func authResultsHeader(hostname string, results AuthResults) string {
	parts := []string{hostname}
	if results.SPF != "" {
		parts = append(parts, "spf="+results.SPF)
	}
	if results.DKIM != "" {
		parts = append(parts, "dkim="+results.DKIM)
	}
	if results.DMARC != "" {
		parts = append(parts, "dmarc="+results.DMARC)
	}
	return "Authentication-Results: " + strings.Join(parts, "; ") + "\r\n"
}

// readAllLimited reads at most limit bytes, reporting when the limit is hit so
// the caller can answer with a proper SMTP size error.
func readAllLimited(r io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("message exceeds the %d byte limit", limit)
	}
	return data, nil
}
