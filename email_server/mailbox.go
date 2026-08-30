package email_server

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"redock/pkg/security"
	"redock/platform/memory"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func (m *EmailManager) AddDomain(domain, description string) (*EmailDomain, error) {
	existing := memory.Filter[*EmailDomain](m.db, "email_domains", func(d *EmailDomain) bool {
		return d.Domain == domain
	})

	if len(existing) > 0 {
		return nil, fmt.Errorf("domain already exists: %s", domain)
	}

	dkimSelector := "mail"
	privateKey, publicKey, err := generateDKIMKeys()
	if err != nil {
		return nil, fmt.Errorf("failed to generate DKIM keys: %w", err)
	}

	encryptedPrivateKey, err := security.EncryptAES256GCM(privateKey, m.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt DKIM private key: %w", err)
	}

	serverIP := m.config.IPAddress
	if serverIP == "" {
		serverIP = "127.0.0.1"
	}

	emailDomain := &EmailDomain{
		Domain:         domain,
		Enabled:        true,
		Description:    description,
		MaxMailboxes:   0,
		MaxQuotaPerBox: 5000,
		TotalQuota:     0,
		UsedQuota:      0,
		DNSConfigured:  false,
		MXRecord:       fmt.Sprintf("mail.%s", domain),
		DKIMSelector:   dkimSelector,
		DKIMPrivateKey: encryptedPrivateKey,
		DKIMPublicKey:  publicKey,
		SPFRecord:      fmt.Sprintf("v=spf1 ip4:%s ~all", serverIP),
		DMARCRecord:    fmt.Sprintf("v=DMARC1; p=quarantine; rua=mailto:dmarc@%s", domain),
		EnableSPAM:     true,
		EnableVirus:    true,
		SMTPOnly:       false,
	}

	if err := memory.Create[*EmailDomain](m.db, "email_domains", emailDomain); err != nil {
		return nil, fmt.Errorf("failed to create domain: %w", err)
	}

	domainPath := filepath.Join(m.config.DataPath, domain)
	if err := os.MkdirAll(domainPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create domain directory: %w", err)
	}

	return emailDomain, nil
}

func (m *EmailManager) DeleteDomain(domainID uint) error {
	domain, err := memory.FindByID[*EmailDomain](m.db, "email_domains", domainID)
	if err != nil {
		return fmt.Errorf("domain not found: %w", err)
	}

	mailboxes := memory.Filter[*EmailMailbox](m.db, "email_mailboxes", func(mb *EmailMailbox) bool {
		return mb.DomainID == domainID
	})

	if len(mailboxes) > 0 {
		return fmt.Errorf("cannot delete domain with existing mailboxes (found %d mailboxes)", len(mailboxes))
	}

	if err := memory.Delete[*EmailDomain](m.db, "email_domains", domainID); err != nil {
		return fmt.Errorf("failed to delete domain: %w", err)
	}

	domainPath := filepath.Join(m.config.DataPath, domain.Domain)
	os.RemoveAll(domainPath)

	return nil
}

func (m *EmailManager) AddMailbox(domainID uint, username, password, name string) (*EmailMailbox, error) {
	domain, err := memory.FindByID[*EmailDomain](m.db, "email_domains", domainID)
	if err != nil {
		return nil, fmt.Errorf("domain not found: %w", err)
	}

	email := fmt.Sprintf("%s@%s", username, domain.Domain)

	existing := memory.Filter[*EmailMailbox](m.db, "email_mailboxes", func(mb *EmailMailbox) bool {
		return mb.Email == email
	})

	if len(existing) > 0 {
		return nil, fmt.Errorf("mailbox already exists: %s", email)
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	encryptedPassword, err := security.EncryptAES256GCM(password, m.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt password: %w", err)
	}

	mailbox := &EmailMailbox{
		DomainID:      domainID,
		Username:      username,
		Email:         email,
		Password:      string(hashedPassword),
		PlainPassword: encryptedPassword,
		Name:          name,
		Quota:         domain.MaxQuotaPerBox,
		UsedQuota:     0,
		MessageCount:  0,
		Enabled:       true,
		ForwardTo:     "",
		KeepCopy:      true,
		AutoReply:     false,
		IMAPEnabled:   true,
		POP3Enabled:   true,
		SMTPEnabled:   true,
		LoginCount:    0,
	}

	if err := memory.Create[*EmailMailbox](m.db, "email_mailboxes", mailbox); err != nil {
		return nil, fmt.Errorf("failed to create mailbox: %w", err)
	}

	m.passwordCacheMux.Lock()
	m.passwordCache[email] = password
	m.passwordCacheMux.Unlock()

	mailboxPath := filepath.Join(m.config.DataPath, domain.Domain, username)

	baseDirs := []string{"cur", "new", "tmp"}
	for _, dir := range baseDirs {
		path := filepath.Join(mailboxPath, dir)
		if err := os.MkdirAll(path, 0777); err != nil {
			return nil, fmt.Errorf("failed to create mailbox directory: %w", err)
		}
		os.Chmod(path, 0777)
	}

	specialFolders := []string{".Sent", ".Drafts", ".Trash", ".Spam", ".Archive"}
	for _, folder := range specialFolders {
		for _, subDir := range []string{"cur", "new", "tmp"} {
			path := filepath.Join(mailboxPath, folder, subDir)
			if err := os.MkdirAll(path, 0777); err != nil {
				return nil, fmt.Errorf("failed to create folder directory: %w", err)
			}
			os.Chmod(path, 0777)
		}
	}

	os.Chmod(mailboxPath, 0777)
	domainPath := filepath.Join(m.config.DataPath, domain.Domain)
	os.Chmod(domainPath, 0777)

	m.createDefaultFolders(mailbox.ID)

	// The account lives in the memory DB and the Maildir is ours to create.
	if err := m.store().EnsureMailbox(domain.Domain, mailbox.Username); err != nil {
		log.Printf("⚠️  Failed to create maildir for %s: %v", email, err)
	}

	return mailbox, nil
}

func (m *EmailManager) createDefaultFolders(mailboxID uint) {
	folders := []struct {
		name   string
		path   string
		icon   string
		system bool
	}{
		{"Inbox", "INBOX", "📥", true},
		{"Sent", "Sent", "📤", true},
		{"Drafts", "Drafts", "📝", true},
		{"Trash", "Trash", "🗑️", true},
		{"Spam", "Spam", "🚫", true},
		{"Archive", "Archive", "📦", true},
	}

	for _, f := range folders {
		folder := &EmailFolder{
			MailboxID:    mailboxID,
			Name:         f.name,
			Path:         f.path,
			IsSystem:     f.system,
			Icon:         f.icon,
			MessageCount: 0,
			UnreadCount:  0,
		}
		memory.Create[*EmailFolder](m.db, "email_folders", folder)
	}
}

func (m *EmailManager) UpdateMailboxPassword(mailboxID uint, newPassword string) error {
	mailbox, err := memory.FindByID[*EmailMailbox](m.db, "email_mailboxes", mailboxID)
	if err != nil {
		return fmt.Errorf("mailbox not found: %w", err)
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	mailbox.Password = string(hashedPassword)

	encryptedPassword, err := security.EncryptAES256GCM(newPassword, m.encryptionKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt password: %w", err)
	}
	mailbox.PlainPassword = encryptedPassword

	memory.Update[*EmailMailbox](m.db, "email_mailboxes", mailbox)

	m.passwordCacheMux.Lock()
	m.passwordCache[mailbox.Email] = newPassword
	m.passwordCacheMux.Unlock()

	// Authentication uses the stored bcrypt hash, so there is nothing else to
	// synchronise.
	return nil
}

func (m *EmailManager) DeleteMailbox(mailboxID uint) error {
	mailbox, err := memory.FindByID[*EmailMailbox](m.db, "email_mailboxes", mailboxID)
	if err != nil {
		return fmt.Errorf("mailbox not found: %w", err)
	}

	m.passwordCacheMux.Lock()
	delete(m.passwordCache, mailbox.Email)
	m.passwordCacheMux.Unlock()

	if err := memory.Delete[*EmailMailbox](m.db, "email_mailboxes", mailboxID); err != nil {
		return fmt.Errorf("failed to delete mailbox: %w", err)
	}

	folders := memory.Filter[*EmailFolder](m.db, "email_folders", func(f *EmailFolder) bool {
		return f.MailboxID == mailboxID
	})
	for _, folder := range folders {
		memory.Delete[*EmailFolder](m.db, "email_folders", folder.ID)
	}

	domain, _ := memory.FindByID[*EmailDomain](m.db, "email_domains", mailbox.DomainID)
	if domain != nil {
		mailboxPath := filepath.Join(m.config.DataPath, domain.Domain, mailbox.Username)
		os.RemoveAll(mailboxPath)
	}

	return nil
}

func generateDKIMKeys() (privateKeyPEM, publicKeyTXT string, err error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}

	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	privateKeyPEM = string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	}))

	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return "", "", err
	}
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	publicKeyStr := string(publicKeyPEM)
	publicKeyStr = strings.ReplaceAll(publicKeyStr, "-----BEGIN PUBLIC KEY-----", "")
	publicKeyStr = strings.ReplaceAll(publicKeyStr, "-----END PUBLIC KEY-----", "")
	publicKeyStr = strings.ReplaceAll(publicKeyStr, "\n", "")
	publicKeyStr = strings.ReplaceAll(publicKeyStr, "\r", "")
	publicKeyStr = strings.ReplaceAll(publicKeyStr, " ", "")
	publicKeyStr = strings.TrimSpace(publicKeyStr)

	publicKeyTXT = fmt.Sprintf("v=DKIM1; k=rsa; p=%s", publicKeyStr)

	return privateKeyPEM, publicKeyTXT, nil
}

// fixAllMailboxPermissions removed - not needed with Docker named volumes
// Named volumes handle permissions automatically inside container

func timePtr(t time.Time) *time.Time {
	return &t
}
