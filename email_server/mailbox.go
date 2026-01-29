package email_server

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"os"
	"os/exec"
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
	
	if err := m.writeDKIMKeys(domain, dkimSelector, privateKey); err != nil {
		log.Printf("⚠️  Failed to write DKIM keys: %v", err)
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
	
	if m.config.IsRunning {
		if err := m.addMailboxToContainer(email, password); err != nil {
			log.Printf("⚠️  Failed to add mailbox to container: %v", err)
		} else {
			if err := m.reloadOpenDKIM(); err != nil {
				log.Printf("⚠️  Failed to sync OpenDKIM: %v", err)
			}
		}
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
	
	if m.config.ContainerName != "" {
		delCmd := exec.Command("docker", "exec", m.config.ContainerName, "setup", "email", "del", mailbox.Email)
		delCmd.Run()
		
		addCmd := exec.Command("docker", "exec", m.config.ContainerName, "setup", "email", "add", mailbox.Email, newPassword)
		if err := addCmd.Run(); err != nil {
			return fmt.Errorf("failed to update password in container: %w", err)
		}
		
		// Re-create folder structure after password update (mailbox was deleted and re-added)
		parts := strings.Split(mailbox.Email, "@")
		if len(parts) == 2 {
			username := parts[0]
			domain, err := memory.FindByID[*EmailDomain](m.db, "email_domains", mailbox.DomainID)
			if err == nil {
				if err := m.ensureMailboxFolderStructure(domain.Domain, username); err != nil {
					log.Printf("⚠️  Failed to recreate folder structure: %v (not critical)", err)
				}
			}
		}
		
		m.reloadOpenDKIM()
	}
	
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
	
	if m.config.IsRunning {
		m.deleteMailboxFromContainer(mailbox.Email)
		m.reloadOpenDKIM()
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

func (m *EmailManager) writeDKIMKeys(domain, selector, privateKey string) error {
	opendkimBase := filepath.Join(m.config.ConfigPath, "opendkim")
	if err := os.MkdirAll(opendkimBase, 0777); err != nil {
		return fmt.Errorf("failed to create opendkim directory: %w", err)
	}
	os.Chmod(opendkimBase, 0777)
	
	dkimPath := filepath.Join(opendkimBase, "keys", domain)
	if err := os.MkdirAll(dkimPath, 0777); err != nil {
		return fmt.Errorf("failed to create DKIM keys directory: %w", err)
	}
	os.Chmod(dkimPath, 0777)
	
	keyFile := filepath.Join(dkimPath, selector+".private")
	if err := os.WriteFile(keyFile, []byte(privateKey), 0644); err != nil {
		return fmt.Errorf("failed to write private key: %w", err)
	}
	os.Chmod(keyFile, 0644)
	
	// Ensure directory permissions are correct
	os.Chmod(filepath.Dir(keyFile), 0755)
	
	if err := m.updateOpenDKIMConfig(domain, selector); err != nil {
		return err
	}
	
	m.reloadOpenDKIM()
	
	return nil
}

func (m *EmailManager) updateOpenDKIMConfig(domain, selector string) error {
	configBase := filepath.Join(m.config.ConfigPath, "opendkim")
	
	signingTable := filepath.Join(configBase, "SigningTable")
	signingEntry := fmt.Sprintf("*@%s %s._domainkey.%s\n", domain, selector, domain)
	appendOrCreateFile(signingTable, signingEntry)
	
	keyTable := filepath.Join(configBase, "KeyTable")
	keyEntry := fmt.Sprintf("%s._domainkey.%s %s:mail:/tmp/docker-mailserver/opendkim/keys/%s/%s.private\n", 
		selector, domain, domain, domain, selector)
	appendOrCreateFile(keyTable, keyEntry)
	
	trustedHosts := filepath.Join(configBase, "TrustedHosts")
	trustedEntry := fmt.Sprintf("%s\n", domain)
	appendOrCreateFile(trustedHosts, trustedEntry)
	
	for _, file := range []string{signingTable, keyTable, trustedHosts} {
		os.Chmod(file, 0644)
	}
	
	return nil
}

func appendOrCreateFile(path, content string) error {
	if data, err := os.ReadFile(path); err == nil {
		if strings.Contains(string(data), strings.TrimSpace(content)) {
			return nil
		}
	}
	
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	
	_, err = f.WriteString(content)
	return err
}

func (m *EmailManager) reloadOpenDKIM() error {
	if m.config.ContainerName == "" {
		return nil
	}
	
	syncCommands := [][]string{
		{"docker", "exec", m.config.ContainerName, "bash", "-c", "mkdir -p /etc/opendkim/keys 2>/dev/null || true"},
		{"docker", "exec", m.config.ContainerName, "bash", "-c", "cp /tmp/docker-mailserver/opendkim/KeyTable /etc/opendkim/KeyTable 2>/dev/null || true"},
		{"docker", "exec", m.config.ContainerName, "bash", "-c", "cp /tmp/docker-mailserver/opendkim/SigningTable /etc/opendkim/SigningTable 2>/dev/null || true"},
		{"docker", "exec", m.config.ContainerName, "bash", "-c", "cp /tmp/docker-mailserver/opendkim/TrustedHosts /etc/opendkim/TrustedHosts 2>/dev/null || true"},
		{"docker", "exec", m.config.ContainerName, "bash", "-c", "cp -r /tmp/docker-mailserver/opendkim/keys/* /etc/opendkim/keys/ 2>/dev/null || true"},
		{"docker", "exec", m.config.ContainerName, "bash", "-c", "chown -R opendkim:opendkim /etc/opendkim/keys 2>/dev/null || true"},
		{"docker", "exec", m.config.ContainerName, "bash", "-c", "chmod 600 /etc/opendkim/keys/*/mail.private 2>/dev/null || true"},
		{"docker", "exec", m.config.ContainerName, "bash", "-c", "sed -i 's|/tmp/docker-mailserver/opendkim/keys|/etc/opendkim/keys|g' /etc/opendkim/KeyTable 2>/dev/null || true"},
	}
	
	for _, cmdArgs := range syncCommands {
		cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
		cmd.Run()
	}
	
	restartCmd := exec.Command("docker", "exec", m.config.ContainerName, "supervisorctl", "restart", "opendkim")
	restartCmd.Run()
	
	return nil
}

// ensureMailboxFolderStructure creates the Maildir folder structure for a mailbox
// Creates INBOX and special folders under home/ directory (docker-mailserver uses mail_home=/var/mail/%d/%n/home/)
func (m *EmailManager) ensureMailboxFolderStructure(domain, username string) error {
	// Wait for user directory to be created by setup email add (max 5 seconds)
	userDir := fmt.Sprintf("/var/mail/%s/%s", domain, username)
	for i := 0; i < 10; i++ {
		checkCmd := exec.Command("docker", "exec", m.config.ContainerName, "test", "-d", userDir)
		if checkCmd.Run() == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	
	createCmd := exec.Command("docker", "exec", "-u", "docker", m.config.ContainerName, "bash", "-c",
		fmt.Sprintf(`
			cd /var/mail/%s/%s && \
			mkdir -p home/{cur,new,tmp} home/.Sent/{cur,new,tmp} home/.Drafts/{cur,new,tmp} home/.Trash/{cur,new,tmp} home/.Junk/{cur,new,tmp}
		`, domain, username))
	
	output, err := createCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create folder structure: %v (output: %s)", err, string(output))
	}
	
	return nil
}

func (m *EmailManager) addMailboxToContainer(email, password string) error {
	// Add mailbox to container using docker-mailserver's setup command
	cmd := exec.Command("docker", "exec", m.config.ContainerName, "setup", "email", "add", email, password)
	if err := cmd.Run(); err != nil {
		return err
	}
	
	// Create maildir folder structure for special folders and INBOX
	// With INDEX=MEMORY and mail_home=/var/mail/%d/%n/, we need to create physical folders
	parts := strings.Split(email, "@")
	if len(parts) == 2 {
		username := parts[0]
		domain := parts[1]
		
		if err := m.ensureMailboxFolderStructure(domain, username); err != nil {
			log.Printf("⚠️  %v (not critical)", err)
		}
	}
	
	return nil
}

func (m *EmailManager) deleteMailboxFromContainer(email string) error {
	cmd := exec.Command("docker", "exec", m.config.ContainerName, "setup", "email", "del", email)
	return cmd.Run()
}

// fixAllMailboxPermissions removed - not needed with Docker named volumes
// Named volumes handle permissions automatically inside container

func timePtr(t time.Time) *time.Time {
	return &t
}
