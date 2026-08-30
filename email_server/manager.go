package email_server

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"redock/pkg/pathutil"
	"redock/pkg/security"
	"redock/platform/memory"
	"strings"
	"sync"
	"time"
)

var (
	managerInstance *EmailManager
	managerOnce     sync.Once
)

// EmailManager owns the mail server's configuration and state. Mail itself is
// served by the native SMTP/IMAP/POP3 engine in this package; there is no
// container involved.
type EmailManager struct {
	db               *memory.Database
	config           *EmailServerConfig
	dataPath         string
	mutex            sync.RWMutex
	passwordCache    map[string]string
	passwordCacheMux sync.RWMutex
	encryptionKey    []byte

	native     *NativeServer
	nativeOnce sync.Once
}

func GetManager() *EmailManager {
	managerOnce.Do(func() {
		managerInstance = &EmailManager{}
	})
	return managerInstance
}

// Init prepares the data directories, loads (or creates) the configuration and
// the encryption key. Starting the listeners is the caller's job.
func (m *EmailManager) Init(db *memory.Database, dataPath string) error {
	m.db = db
	m.dataPath = filepath.Join(dataPath, "email")
	m.passwordCache = make(map[string]string)

	dirs := []string{
		filepath.Join(m.dataPath, "mail"),
		filepath.Join(m.dataPath, "queue"),
		filepath.Join(m.dataPath, "tls"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	keyPath := filepath.Join(m.dataPath, ".encryption.key")
	encryptionKey, err := security.GetOrCreateMasterKey(keyPath)
	if err != nil {
		return fmt.Errorf("failed to initialize encryption key: %w", err)
	}
	m.encryptionKey = encryptionKey

	configs := memory.FindAll[*EmailServerConfig](db, "email_server_configs")
	if len(configs) == 0 {
		m.config = m.createDefaultConfig()
		if err := memory.Create[*EmailServerConfig](db, "email_server_configs", m.config); err != nil {
			return fmt.Errorf("failed to store default configuration: %w", err)
		}
	} else {
		m.config = configs[0]
		// Cross-machine restore: persisted absolute paths still reference the
		// source host's home. Rewrite them against the running user's workDir
		// so data and certificate files resolve. m.dataPath is
		// "<workDir>/data/email", so the workDir is two levels up.
		workDir := filepath.Dir(filepath.Dir(m.dataPath))
		changed := m.normalizeConfigPaths(workDir)
		if applyDefaultsIfMissing(m.config) {
			changed = true
		}
		if changed {
			if err := memory.Update[*EmailServerConfig](db, "email_server_configs", m.config); err != nil {
				log.Printf("email_server: failed to persist normalized config: %v", err)
			}
		}
	}

	m.restorePasswordCache()

	if m.config.IPAddress == "" {
		go m.autoDetectPublicIP()
	}

	// Every domain needs a DKIM key: the engine signs outgoing mail itself.
	go m.ensureAllDomainKeys()

	return nil
}

// applyDefaultsIfMissing brings an older stored configuration up to date and
// reports whether anything changed.
func applyDefaultsIfMissing(cfg *EmailServerConfig) bool {
	before := *cfg
	applyNativeDefaults(cfg)
	return before != *cfg
}

// normalizeConfigPaths rewrites absolute paths in the loaded config so they
// match the running user's workDir. Returns true if any field changed;
// idempotent on already-correct paths.
func (m *EmailManager) normalizeConfigPaths(workDir string) bool {
	if m.config == nil || workDir == "" {
		return false
	}

	changed := false
	rewrite := func(p *string) {
		if v := pathutil.NormalizeWorkDirPath(*p, workDir); v != *p {
			*p = v
			changed = true
		}
	}
	rewrite(&m.config.SSLCertPath)
	rewrite(&m.config.SSLKeyPath)
	rewrite(&m.config.DataPath)
	return changed
}

func (m *EmailManager) createDefaultConfig() *EmailServerConfig {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "mail.localhost"
	} else if !strings.Contains(hostname, ".") {
		hostname += ".localhost"
	}

	config := &EmailServerConfig{
		Name:      "Email Server",
		Hostname:  hostname,
		IPAddress: "",
		DataPath:  filepath.Join(m.dataPath, "mail"),
	}
	EnableAllNativeServices(config)
	return config
}

// StartServer brings the mail listeners up.
func (m *EmailManager) StartServer() error {
	if err := m.Native().Start(); err != nil {
		return err
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.config.IsRunning = true
	m.config.LastStarted = timePtr(time.Now())
	if err := memory.Update[*EmailServerConfig](m.db, "email_server_configs", m.config); err != nil {
		log.Printf("email_server: failed to persist running state: %v", err)
	}
	return nil
}

// StopServer closes the listeners; queued mail stays on disk.
func (m *EmailManager) StopServer() error {
	m.Native().Stop()

	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.config.IsRunning = false
	m.config.LastStopped = timePtr(time.Now())
	if err := memory.Update[*EmailServerConfig](m.db, "email_server_configs", m.config); err != nil {
		log.Printf("email_server: failed to persist stopped state: %v", err)
	}
	return nil
}

// RestartServer rebinds every listener, picking up configuration changes.
func (m *EmailManager) RestartServer() error {
	if err := m.StopServer(); err != nil {
		return err
	}
	return m.StartServer()
}

// GetServerStatus returns the configuration with the live running flag.
func (m *EmailManager) GetServerStatus() (*EmailServerConfig, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.config == nil {
		return nil, fmt.Errorf("email server is not initialized")
	}
	m.config.IsRunning = m.Native().IsRunning()
	return m.config, nil
}

func (m *EmailManager) GetDB() *memory.Database { return m.db }

func (m *EmailManager) GetConfig() *EmailServerConfig { return m.config }

// restorePasswordCache decrypts the stored mailbox passwords once at boot, so
// the dashboard can show them without a round-trip per request.
func (m *EmailManager) restorePasswordCache() {
	mailboxes := memory.FindAll[*EmailMailbox](m.db, "email_mailboxes")

	m.passwordCacheMux.Lock()
	defer m.passwordCacheMux.Unlock()

	for _, mailbox := range mailboxes {
		if mailbox.PlainPassword == "" {
			continue
		}
		decrypted, err := security.DecryptAES256GCM(mailbox.PlainPassword, m.encryptionKey)
		if err != nil {
			continue
		}
		m.passwordCache[mailbox.Email] = decrypted
	}
}

// GetMailboxPassword returns a mailbox's stored password. Authentication uses
// the bcrypt hash instead; this exists only for the dashboard's "show
// password" affordance and for client-configuration hints.
func (m *EmailManager) GetMailboxPassword(email string) (string, error) {
	m.passwordCacheMux.RLock()
	password, ok := m.passwordCache[email]
	m.passwordCacheMux.RUnlock()
	if ok {
		return password, nil
	}

	mailboxes := memory.Filter[*EmailMailbox](m.db, "email_mailboxes", func(mb *EmailMailbox) bool {
		return mb.Email == email
	})
	if len(mailboxes) == 0 {
		return "", fmt.Errorf("mailbox not found: %s", email)
	}

	mailbox := mailboxes[0]
	if mailbox.PlainPassword == "" {
		return "", fmt.Errorf("password not set for %s - use PUT /api/email/mailboxes/%d/password to set it", email, mailbox.ID)
	}

	decrypted, err := security.DecryptAES256GCM(mailbox.PlainPassword, m.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt password for %s: %w", email, err)
	}

	m.passwordCacheMux.Lock()
	m.passwordCache[email] = decrypted
	m.passwordCacheMux.Unlock()
	return decrypted, nil
}

// ensureAllDomainKeys gives every domain a DKIM key, so nothing leaves the
// server unsigned.
func (m *EmailManager) ensureAllDomainKeys() {
	domains := memory.FindAll[*EmailDomain](m.db, "email_domains")
	for _, domain := range domains {
		if domain == nil || domain.IsDeleted() {
			continue
		}
		if err := m.ensureDomainDKIM(domain); err != nil {
			log.Printf("email_server: could not prepare DKIM for %s: %v", domain.Domain, err)
		}
	}
}

// autoDetectPublicIP fills in the server's public address, which SPF records
// and the dashboard's DNS help both need.
func (m *EmailManager) autoDetectPublicIP() {
	services := []string{
		"https://ifconfig.me/ip",
		"https://api.ipify.org",
		"https://icanhazip.com",
	}

	var detected string
	for _, service := range services {
		ip, err := detectIPFromService(service)
		if err == nil && ip != "" {
			detected = ip
			break
		}
	}
	if detected == "" {
		return
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.config.IPAddress = detected
	if err := memory.Update[*EmailServerConfig](m.db, "email_server_configs", m.config); err != nil {
		log.Printf("email_server: failed to persist detected IP: %v", err)
	}
}

func detectIPFromService(serviceURL string) (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(serviceURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("service returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64))
	if err != nil {
		return "", err
	}

	ip := strings.TrimSpace(string(body))
	if len(ip) < 7 || len(ip) > 15 || !strings.Contains(ip, ".") {
		return "", fmt.Errorf("invalid IP format")
	}
	return ip, nil
}

// UpdateServerIP sets the advertised public address.
func (m *EmailManager) UpdateServerIP(newIP string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if newIP == "" {
		return fmt.Errorf("IP address cannot be empty")
	}
	if parts := strings.Split(newIP, "."); len(parts) != 4 {
		return fmt.Errorf("invalid IP address format")
	}

	m.config.IPAddress = newIP
	return memory.Update[*EmailServerConfig](m.db, "email_server_configs", m.config)
}
