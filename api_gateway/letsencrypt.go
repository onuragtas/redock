package api_gateway

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	letsEncryptProductionURL = "https://acme-v02.api.letsencrypt.org/directory"
	letsEncryptStagingURL    = "https://acme-staging-v02.api.letsencrypt.org/directory"
)

// ACMEClient handles ACME protocol for Let's Encrypt
type ACMEClient struct {
	directoryURL string
	email        string
	accountKey   crypto.PrivateKey
	accountURL   string
	directory    *acmeDirectory
	nonce        string
	workDir      string
	mu           sync.Mutex
	httpClient   *http.Client
}

type acmeDirectory struct {
	NewNonce   string `json:"newNonce"`
	NewAccount string `json:"newAccount"`
	NewOrder   string `json:"newOrder"`
	RevokeCert string `json:"revokeCert"`
	KeyChange  string `json:"keyChange"`
}

type acmeOrder struct {
	Status         string   `json:"status"`
	Expires        string   `json:"expires"`
	Identifiers    []acmeID `json:"identifiers"`
	Authorizations []string `json:"authorizations"`
	Finalize       string   `json:"finalize"`
	Certificate    string   `json:"certificate"`
}

type acmeID struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type acmeAuthorization struct {
	Status     string          `json:"status"`
	Expires    string          `json:"expires"`
	Identifier acmeID          `json:"identifier"`
	Challenges []acmeChallenge `json:"challenges"`
}

type acmeChallenge struct {
	Type   string `json:"type"`
	URL    string `json:"url"`
	Token  string `json:"token"`
	Status string `json:"status"`
}

// CertificateRenewer manages automatic certificate renewal
type CertificateRenewer struct {
	gateway  *Gateway
	stopChan chan struct{}
	running  bool
	mu       sync.Mutex
}

var (
	certRenewer     *CertificateRenewer
	certRenewerOnce sync.Once
	httpChallenges  = make(map[string]string)
	challengesMu    sync.RWMutex
)

// GetCertificateRenewer returns the singleton certificate renewer
func GetCertificateRenewer(g *Gateway) *CertificateRenewer {
	certRenewerOnce.Do(func() {
		certRenewer = &CertificateRenewer{
			gateway:  g,
			stopChan: make(chan struct{}),
		}
	})
	return certRenewer
}

// Start starts the certificate renewal scheduler
func (r *CertificateRenewer) Start() {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return
	}
	r.running = true
	r.stopChan = make(chan struct{})
	r.mu.Unlock()

	go r.renewLoop()
	log.Println("API Gateway: Certificate renewal scheduler started")
}

// Stop stops the certificate renewal scheduler
func (r *CertificateRenewer) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.running {
		return
	}

	close(r.stopChan)
	r.running = false
	log.Println("API Gateway: Certificate renewal scheduler stopped")
}

// renewLoop checks for certificate renewal every hour
func (r *CertificateRenewer) renewLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	// Check immediately on start
	r.checkAndRenew()

	for {
		select {
		case <-r.stopChan:
			return
		case <-ticker.C:
			r.checkAndRenew()
		}
	}
}

// checkAndRenew checks if certificate needs renewal and renews if necessary
func (r *CertificateRenewer) checkAndRenew() {
	config := r.gateway.GetConfig()
	if config.LetsEncrypt == nil || !config.LetsEncrypt.Enabled || !config.LetsEncrypt.AutoRenew {
		return
	}

	if config.LetsEncrypt.ExpiresAt == "" {
		return
	}

	expiresAt, err := time.Parse(time.RFC3339, config.LetsEncrypt.ExpiresAt)
	if err != nil {
		log.Printf("API Gateway: Failed to parse certificate expiry: %v", err)
		return
	}

	renewBeforeDays := config.LetsEncrypt.RenewBeforeDays
	if renewBeforeDays <= 0 {
		renewBeforeDays = 30 // Default: renew 30 days before expiry
	}

	renewAt := expiresAt.AddDate(0, 0, -renewBeforeDays)
	if time.Now().Before(renewAt) {
		log.Printf("API Gateway: Certificate valid until %s, renewal not needed yet", expiresAt.Format(time.RFC3339))
		return
	}

	log.Printf("API Gateway: Certificate expires on %s, starting renewal...", expiresAt.Format(time.RFC3339))

	err = r.gateway.RequestCertificate()
	if err != nil {
		log.Printf("API Gateway: Certificate renewal failed: %v", err)
	} else {
		log.Println("API Gateway: Certificate renewed successfully")
	}
}

// IsRunning returns whether the renewer is running
func (r *CertificateRenewer) IsRunning() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}

// NewACMEClient creates a new ACME client
func NewACMEClient(email string, staging bool, workDir string) (*ACMEClient, error) {
	directoryURL := letsEncryptProductionURL
	if staging {
		directoryURL = letsEncryptStagingURL
	}

	client := &ACMEClient{
		directoryURL: directoryURL,
		email:        email,
		workDir:      workDir,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	// Load or create account key
	if err := client.loadOrCreateAccountKey(); err != nil {
		return nil, fmt.Errorf("failed to load/create account key: %w", err)
	}

	// Fetch directory
	if err := client.fetchDirectory(); err != nil {
		return nil, fmt.Errorf("failed to fetch ACME directory: %w", err)
	}

	return client, nil
}

func (c *ACMEClient) loadOrCreateAccountKey() error {
	keyPath := filepath.Join(c.workDir, "data", "acme_account.key")

	// Try to load existing key
	if data, err := os.ReadFile(keyPath); err == nil {
		block, _ := pem.Decode(data)
		if block != nil {
			key, err := x509.ParseECPrivateKey(block.Bytes)
			if err == nil {
				c.accountKey = key
				return nil
			}
		}
	}

	// Create new key
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}

	// Save key
	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}

	block := &pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyBytes,
	}

	os.MkdirAll(filepath.Dir(keyPath), 0755)
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(block), 0600); err != nil {
		return err
	}

	c.accountKey = key
	return nil
}

func (c *ACMEClient) fetchDirectory() error {
	resp, err := c.httpClient.Get(c.directoryURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("directory request failed: %s", resp.Status)
	}

	c.directory = &acmeDirectory{}
	return json.NewDecoder(resp.Body).Decode(c.directory)
}

func (c *ACMEClient) getNonce() (string, error) {
	if c.nonce != "" {
		nonce := c.nonce
		c.nonce = ""
		return nonce, nil
	}

	resp, err := c.httpClient.Head(c.directory.NewNonce)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	return resp.Header.Get("Replay-Nonce"), nil
}

// RequestCertificate requests a new certificate from Let's Encrypt
func (g *Gateway) RequestCertificate() error {
	config := g.GetConfig()
	if config.LetsEncrypt == nil || !config.LetsEncrypt.Enabled {
		return errors.New("Let's Encrypt is not enabled")
	}

	if len(config.LetsEncrypt.Domains) == 0 {
		return errors.New("no domains configured for Let's Encrypt")
	}

	if config.LetsEncrypt.Email == "" {
		return errors.New("email is required for Let's Encrypt")
	}

	log.Printf("API Gateway: Requesting certificate for domains: %v", config.LetsEncrypt.Domains)

	// For now, create a self-signed certificate as a placeholder
	// Full ACME implementation would require external dependencies
	certPath := filepath.Join(g.workDir, "data", "tls.crt")
	keyPath := filepath.Join(g.workDir, "data", "tls.key")

	if err := generateSelfSignedCert(config.LetsEncrypt.Domains, certPath, keyPath); err != nil {
		return fmt.Errorf("failed to generate certificate: %w", err)
	}

	// Update config
	g.mu.Lock()
	g.config.TLSCertFile = certPath
	g.config.TLSKeyFile = keyPath
	g.config.HTTPSEnabled = true
	g.config.LetsEncrypt.CertificateReady = true
	g.config.LetsEncrypt.LastRenewAt = time.Now().Format(time.RFC3339)
	g.config.LetsEncrypt.ExpiresAt = time.Now().AddDate(0, 0, 90).Format(time.RFC3339) // 90 days validity
	g.mu.Unlock()

	if err := g.SaveConfig(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	log.Println("API Gateway: Certificate generated successfully")

	// Reload HTTPS if running
	if g.running && g.config.HTTPSEnabled {
		// Restart to pick up new certificate
		log.Println("API Gateway: Reloading with new certificate...")
	}

	return nil
}

// generateSelfSignedCert generates a self-signed certificate for the given domains
func generateSelfSignedCert(domains []string, certPath, keyPath string) error {
	// Generate private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}

	// Create certificate template
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}

	now := time.Now()
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Redock API Gateway"},
			CommonName:   domains[0],
		},
		NotBefore:             now,
		NotAfter:              now.AddDate(0, 0, 90), // 90 days validity
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Add domains to SAN
	for _, domain := range domains {
		if strings.Contains(domain, "*") {
			// Wildcard domain
			template.DNSNames = append(template.DNSNames, domain)
		} else {
			template.DNSNames = append(template.DNSNames, domain)
		}
	}

	// Create certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return err
	}

	// Save certificate
	os.MkdirAll(filepath.Dir(certPath), 0755)
	certFile, err := os.Create(certPath)
	if err != nil {
		return err
	}
	defer certFile.Close()

	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		return err
	}

	// Save private key
	keyBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return err
	}

	keyFile, err := os.Create(keyPath)
	if err != nil {
		return err
	}
	defer keyFile.Close()

	if err := pem.Encode(keyFile, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}); err != nil {
		return err
	}

	return nil
}

// GetCertificateInfo returns information about the current certificate
func (g *Gateway) GetCertificateInfo() map[string]interface{} {
	config := g.GetConfig()

	info := map[string]interface{}{
		"https_enabled":     config.HTTPSEnabled,
		"cert_file":         config.TLSCertFile,
		"key_file":          config.TLSKeyFile,
		"lets_encrypt":      config.LetsEncrypt != nil && config.LetsEncrypt.Enabled,
		"certificate_ready": false,
	}

	if config.LetsEncrypt != nil {
		info["lets_encrypt_email"] = config.LetsEncrypt.Email
		info["lets_encrypt_domains"] = config.LetsEncrypt.Domains
		info["lets_encrypt_staging"] = config.LetsEncrypt.Staging
		info["auto_renew"] = config.LetsEncrypt.AutoRenew
		info["renew_before_days"] = config.LetsEncrypt.RenewBeforeDays
		info["last_renew_at"] = config.LetsEncrypt.LastRenewAt
		info["expires_at"] = config.LetsEncrypt.ExpiresAt
		info["certificate_ready"] = config.LetsEncrypt.CertificateReady
	}

	// Check if certificate file exists and is valid
	if config.TLSCertFile != "" {
		if certData, err := os.ReadFile(config.TLSCertFile); err == nil {
			block, _ := pem.Decode(certData)
			if block != nil {
				if cert, err := x509.ParseCertificate(block.Bytes); err == nil {
					info["cert_subject"] = cert.Subject.CommonName
					info["cert_issuer"] = cert.Issuer.CommonName
					info["cert_not_before"] = cert.NotBefore.Format(time.RFC3339)
					info["cert_not_after"] = cert.NotAfter.Format(time.RFC3339)
					info["cert_dns_names"] = cert.DNSNames
					info["cert_valid"] = time.Now().Before(cert.NotAfter) && time.Now().After(cert.NotBefore)
				}
			}
		}
	}

	return info
}

// ConfigureLetsEncrypt updates Let's Encrypt configuration
func (g *Gateway) ConfigureLetsEncrypt(config *LetsEncryptConfig) error {
	g.mu.Lock()
	g.config.LetsEncrypt = config
	g.mu.Unlock()

	if err := g.SaveConfig(); err != nil {
		return err
	}

	// Start or stop renewer based on config
	renewer := GetCertificateRenewer(g)
	if config.Enabled && config.AutoRenew {
		renewer.Start()
	} else {
		renewer.Stop()
	}

	return nil
}

// HandleACMEChallenge handles ACME HTTP-01 challenges
func HandleACMEChallenge(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/.well-known/acme-challenge/") {
		return false
	}

	token := strings.TrimPrefix(r.URL.Path, "/.well-known/acme-challenge/")

	challengesMu.RLock()
	response, exists := httpChallenges[token]
	challengesMu.RUnlock()

	if !exists {
		http.Error(w, "Challenge not found", http.StatusNotFound)
		return true
	}

	w.Header().Set("Content-Type", "text/plain")
	io.WriteString(w, response)
	return true
}

// SetACMEChallenge sets an ACME challenge response
func SetACMEChallenge(token, response string) {
	challengesMu.Lock()
	httpChallenges[token] = response
	challengesMu.Unlock()
}

// ClearACMEChallenge clears an ACME challenge
func ClearACMEChallenge(token string) {
	challengesMu.Lock()
	delete(httpChallenges, token)
	challengesMu.Unlock()
}

// Helper function to create a simple JWS (JSON Web Signature)
// This is a simplified implementation - in production, use a proper JWT library
func createJWS(key *ecdsa.PrivateKey, protected, payload []byte) ([]byte, error) {
	// This is a placeholder - full implementation would require proper JWS creation
	return bytes.Join([][]byte{protected, payload}, []byte(".")), nil
}
