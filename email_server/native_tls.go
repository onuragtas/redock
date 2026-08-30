package email_server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// certManager supplies TLS certificates to every mail listener. It resolves a
// certificate lazily on each handshake so a renewal (by the API Gateway's
// Let's Encrypt flow, or a manual replacement) is picked up without a restart.
//
// Resolution order:
//  1. explicitly configured cert/key files in the mail server config
//  2. the API Gateway's Let's Encrypt pair at <workDir>/data/tls.{crt,key}
//  3. a self-signed certificate generated once under <dataPath>/tls/
type certManager struct {
	mu sync.RWMutex

	certFile string
	keyFile  string
	leCert   string
	leKey    string
	selfDir  string
	hostname string
	// names / ips are every identity the certificate must cover: the mail
	// hostname, mail.<domain> for each served domain, and every address this
	// machine answers on. A client that connects by IP (203.0.113.5, say) fails
	// verification unless that IP is in the SAN list.
	names []string
	ips   []net.IP

	cached     *tls.Certificate
	cachedFrom string
	loadedAt   time.Time
	modTime    time.Time
}

// certReloadInterval is how often the files are re-stat'ed during handshakes.
const certReloadInterval = 60 * time.Second

func newCertManager(hostname, dataPath, workDir, certFile, keyFile string, names []string, ips []net.IP) *certManager {
	return &certManager{
		certFile: certFile,
		keyFile:  keyFile,
		leCert:   filepath.Join(workDir, "data", "tls.crt"),
		leKey:    filepath.Join(workDir, "data", "tls.key"),
		selfDir:  filepath.Join(dataPath, "tls"),
		hostname: hostname,
		names:    names,
		ips:      ips,
	}
}

// localAddresses collects every non-loopback address of this host, so a client
// reaching the server on a LAN or container address still gets a certificate
// that matches what it dialled.
func localAddresses() []net.IP {
	ips := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ips
	}
	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			continue
		}
		ips = append(ips, ip)
	}
	return ips
}

// certificateCovers reports whether a certificate already vouches for every
// name and address we need, which is what decides if it has to be re-issued.
func certificateCovers(cert *x509.Certificate, names []string, ips []net.IP) bool {
	for _, name := range names {
		if name == "" {
			continue
		}
		if cert.VerifyHostname(name) != nil {
			return false
		}
	}
	for _, ip := range ips {
		if ip == nil {
			continue
		}
		found := false
		for _, certIP := range cert.IPAddresses {
			if certIP.Equal(ip) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// TLSConfig returns a config usable for both STARTTLS and implicit-TLS
// listeners.
func (c *certManager) TLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion:     tls.VersionTLS12,
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return c.certificate() },
		NextProtos:     []string{"smtp", "imap"},
	}
}

// Source describes where the active certificate came from, for the dashboard.
func (c *certManager) Source() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cachedFrom
}

// Expiry returns when the active certificate expires (zero if unknown).
func (c *certManager) Expiry() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.cached == nil || len(c.cached.Certificate) == 0 {
		return time.Time{}
	}
	leaf, err := x509.ParseCertificate(c.cached.Certificate[0])
	if err != nil {
		return time.Time{}
	}
	return leaf.NotAfter
}

func (c *certManager) certificate() (*tls.Certificate, error) {
	c.mu.RLock()
	cached, loadedAt := c.cached, c.loadedAt
	c.mu.RUnlock()

	if cached != nil && time.Since(loadedAt) < certReloadInterval {
		return cached, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Another goroutine may have refreshed while we waited for the lock.
	if c.cached != nil && time.Since(c.loadedAt) < certReloadInterval {
		return c.cached, nil
	}

	candidates := []struct {
		cert, key, source string
	}{
		{c.certFile, c.keyFile, "configured"},
		{c.leCert, c.leKey, "letsencrypt"},
	}

	for _, candidate := range candidates {
		if candidate.cert == "" || candidate.key == "" {
			continue
		}
		info, err := os.Stat(candidate.cert)
		if err != nil {
			continue
		}
		// Reuse the parsed certificate when the file has not changed.
		if c.cached != nil && c.cachedFrom == candidate.source && info.ModTime().Equal(c.modTime) {
			c.loadedAt = time.Now()
			return c.cached, nil
		}
		pair, err := tls.LoadX509KeyPair(candidate.cert, candidate.key)
		if err != nil {
			log.Printf("mail: could not load %s certificate: %v", candidate.source, err)
			continue
		}
		c.cached = &pair
		c.cachedFrom = candidate.source
		c.modTime = info.ModTime()
		c.loadedAt = time.Now()
		return c.cached, nil
	}

	pair, err := c.selfSigned()
	if err != nil {
		return nil, err
	}
	c.cached = pair
	c.cachedFrom = "self-signed"
	c.loadedAt = time.Now()
	return c.cached, nil
}

// selfSigned loads (or creates once) a self-signed certificate. It keeps
// STARTTLS working out of the box; clients that verify names will complain,
// which is why the dashboard shows the source.
func (c *certManager) selfSigned() (*tls.Certificate, error) {
	certPath := filepath.Join(c.selfDir, "self.crt")
	keyPath := filepath.Join(c.selfDir, "self.key")

	hostname := c.hostname
	if hostname == "" {
		hostname = "localhost"
	}

	names := dedupeStrings(append([]string{hostname, "localhost"}, c.names...))
	ips := dedupeIPs(append(localAddresses(), c.ips...))

	// Reuse the existing certificate only while it is valid *and* still covers
	// everything: a new interface address or a new mail domain has to trigger a
	// re-issue, otherwise clients keep failing verification.
	if pair, err := tls.LoadX509KeyPair(certPath, keyPath); err == nil && len(pair.Certificate) > 0 {
		if leaf, err := x509.ParseCertificate(pair.Certificate[0]); err == nil {
			if time.Now().Before(leaf.NotAfter) && certificateCovers(leaf, names, ips) {
				return &pair, nil
			}
			log.Printf("mail: re-issuing the self-signed certificate to cover %v / %v", names, ips)
		}
	}

	if err := os.MkdirAll(c.selfDir, 0700); err != nil {
		return nil, err
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}

	template := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: hostname, Organization: []string{"Redock Mail"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(5, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              names,
		IPAddresses:           ips,
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	if err := os.WriteFile(certPath, certPEM, 0600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return nil, err
	}

	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to build self-signed certificate: %w", err)
	}
	log.Printf("mail: generated a self-signed TLS certificate for %v (%v)", names, ips)
	return &pair, nil
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func dedupeIPs(values []net.IP) []net.IP {
	seen := make(map[string]struct{}, len(values))
	out := make([]net.IP, 0, len(values))
	for _, ip := range values {
		if ip == nil {
			continue
		}
		key := ip.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, ip)
	}
	return out
}
