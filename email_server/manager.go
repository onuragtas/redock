package email_server

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"redock/pkg/security"
	"redock/platform/memory"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

var (
	managerInstance *EmailManager
	managerOnce     sync.Once
)

type EmailManager struct {
	db               *memory.Database
	dockerClient     *client.Client
	config           *EmailServerConfig
	dataPath         string
	mutex            sync.RWMutex
	passwordCache    map[string]string
	passwordCacheMux sync.RWMutex
	encryptionKey    []byte
}

func GetManager() *EmailManager {
	managerOnce.Do(func() {
		managerInstance = &EmailManager{}
	})
	return managerInstance
}

func (m *EmailManager) Init(db *memory.Database, dataPath string) error {
	m.db = db
	m.dataPath = filepath.Join(dataPath, "email")
	m.passwordCache = make(map[string]string)
	
	dirs := []string{
		filepath.Join(m.dataPath, "mail"),
		filepath.Join(m.dataPath, "config"),
		filepath.Join(m.dataPath, "logs"),
		filepath.Join(m.dataPath, "state"),
	}
	
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0777); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
		os.Chmod(dir, 0777)
	}
	
	// Create Dovecot custom config for index workaround
	dovecotConfigPath := filepath.Join(m.dataPath, "config", "dovecot-custom.conf")
	dovecotConfig := `# Docker mount permission workaround
# Use in-memory indexes to avoid permission issues
# Override mail_home to use /var/mail/%d/%n directly (not /var/mail/%d/%n/home/)
mail_home = /var/mail/%d/%n/
mail_location = maildir:~/:INDEX=MEMORY

# Disable index locking that fails on mounted volumes
mail_fsync = never
maildir_stat_dirs = yes
mail_nfs_storage = no
mail_nfs_index = no

# Performance tuning for in-memory indexes
mailbox_list_index = no
`
	if err := os.WriteFile(dovecotConfigPath, []byte(dovecotConfig), 0644); err != nil {
		log.Printf("⚠️  Failed to create Dovecot config: %v", err)
	}
	
	// Fix existing DKIM key permissions
	m.fixDKIMPermissions()
	
	keyPath := filepath.Join(m.dataPath, ".encryption.key")
	var encKeyErr error
	m.encryptionKey, encKeyErr = security.GetOrCreateMasterKey(keyPath)
	if encKeyErr != nil {
		return fmt.Errorf("failed to initialize encryption key: %w", encKeyErr)
	}
	
	var err2 error
	if os.Getenv("DOCKER_HOST") == "" {
		colimaSocket := filepath.Join(os.Getenv("HOME"), ".colima/default/docker.sock")
		if _, err := os.Stat(colimaSocket); err == nil {
			os.Setenv("DOCKER_HOST", "unix://"+colimaSocket)
		}
	}
	
	m.dockerClient, err2 = client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err2 != nil {
		m.dockerClient = nil
	}
	
	configs := memory.FindAll[*EmailServerConfig](db, "email_server_configs")
	if len(configs) == 0 {
		m.config = m.createDefaultConfig()
		memory.Create[*EmailServerConfig](db, "email_server_configs", m.config)
	} else {
		m.config = configs[0]
	}
	
	m.restorePasswordCache()
	
	if m.config.IPAddress == "" {
		go m.autoDetectPublicIP()
	}
	
	if m.dockerClient != nil && m.config != nil {
		go m.autoFixOpenDKIMConfig()
	}
	
	return nil
}

func (m *EmailManager) createDefaultConfig() *EmailServerConfig {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "mail.localhost"
	} else {
		parts := strings.Split(hostname, ".")
		if len(parts) == 1 {
			hostname = hostname + ".localhost"
		}
	}
	
	return &EmailServerConfig{
		Name:           "Email Server",
		Hostname:       hostname,
		IPAddress:      "",
		SMTPPort:       25,
		SMTPSPort:      465,
		SubmissionPort: 587,
		IMAPPort:       143,
		IMAPsPort:      993,
		POP3Port:       110,
		POP3sPort:      995,
		ContainerName:  "redock-mailserver",
		ImageName:      "docker.io/mailserver/docker-mailserver:latest",
		MaxMessageSize: 50,
		MaxRecipients:  50,
		RateLimit:      100,
		SPAMEnabled:    true,
		VirusEnabled:   true,
		DKIMEnabled:    true,
		DataPath:       filepath.Join(m.dataPath, "mail"),
		ConfigPath:     filepath.Join(m.dataPath, "config"),
		LogPath:        filepath.Join(m.dataPath, "logs"),
		IsRunning:      false,
	}
}

func (m *EmailManager) StartServer() error {
	if m.dockerClient == nil {
		return fmt.Errorf("Docker is not available. Please start Docker daemon first")
	}
	
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	ctx := context.Background()
	
	// Ensure named volume exists for mail data
	volumeName := "redock-mail-data"
	if err := m.ensureVolumeExists(ctx, volumeName); err != nil {
		return fmt.Errorf("failed to ensure volume exists: %w", err)
	}
	
	containers, err := m.dockerClient.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}
	
	var existingContainer string
	for _, c := range containers {
		for _, name := range c.Names {
			if name == "/"+m.config.ContainerName {
				existingContainer = c.ID
				break
			}
		}
	}
	
	if existingContainer != "" {
		if err := m.dockerClient.ContainerStart(ctx, existingContainer, container.StartOptions{}); err != nil {
			return fmt.Errorf("failed to start container: %w", err)
		}
		m.config.ContainerID = existingContainer
		m.config.IsRunning = true
		memory.Update[*EmailServerConfig](m.db, "email_server_configs", m.config)
		
		// Wait for container to be ready
		time.Sleep(3 * time.Second)
		
		// Wait for container to be fully ready
		go func() {
			if err := m.waitForContainerReady(30); err != nil {
				log.Printf("⚠️  %v", err)
			}
		}()
		
		return nil
	}
	
	reader, err := m.dockerClient.ImagePull(ctx, m.config.ImageName, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}
	defer reader.Close()
	
	_, err = io.Copy(io.Discard, reader)
	if err != nil {
		return fmt.Errorf("failed to read pull response: %w", err)
	}
	
	portBindings := map[string][]string{
		"25/tcp":   {fmt.Sprintf("%d", m.config.SMTPPort)},
		"465/tcp":  {fmt.Sprintf("%d", m.config.SMTPSPort)},
		"587/tcp":  {fmt.Sprintf("%d", m.config.SubmissionPort)},
		"143/tcp":  {fmt.Sprintf("%d", m.config.IMAPPort)},
		"993/tcp":  {fmt.Sprintf("%d", m.config.IMAPsPort)},
		"110/tcp":  {fmt.Sprintf("%d", m.config.POP3Port)},
		"995/tcp":  {fmt.Sprintf("%d", m.config.POP3sPort)},
	}
	
	envVars := []string{
		"OVERRIDE_HOSTNAME=" + m.config.Hostname,
		"ACCOUNT_PROVISIONER=FILE",
		"ENABLE_SPAMASSASSIN=1",
		"SA_TAG=2.0",
		"SA_TAG2=6.0",
		"SA_KILL=10.0",
		"ENABLE_CLAMAV=0",
		"ENABLE_OPENDKIM=1",
		"ENABLE_OPENDMARC=0",
		"ENABLE_POLICYD_SPF=1",
		"ENABLE_POSTGREY=0",
		"ENABLE_FAIL2BAN=1",
		"FAIL2BAN_BLOCKTYPE=drop",
		"PERMIT_DOCKER=network",
		"TZ=Europe/Istanbul",
		"ONE_DIR=1",
		"DMS_DEBUG=0",
	}
	
	// SSL disabled (use Let's Encrypt proxy for production)
	envVars = append(envVars, "SSL_TYPE=")
	
	config := &container.Config{
		Image: m.config.ImageName,
		Hostname: m.config.Hostname,
		Env: envVars,
	}
	
	// Use Docker named volume for mail data - no permission issues!
	mounts := []mount.Mount{
		{
			Type:   mount.TypeVolume,
			Source: "redock-mail-data",
			Target: "/var/mail",
		},
		{
			Type:   mount.TypeBind,
			Source: m.config.ConfigPath,
			Target: "/tmp/docker-mailserver",
		},
		{
			Type:   mount.TypeBind,
			Source: m.config.LogPath,
			Target: "/var/log/mail",
		},
		{
			Type:     mount.TypeBind,
			Source:   filepath.Join(m.dataPath, "config", "dovecot-custom.conf"),
			Target:   "/etc/dovecot/conf.d/99-custom.conf",
			ReadOnly: true,
		},
	}
	
	// SSL mounts removed - use reverse proxy for TLS termination
	
	hostConfig := &container.HostConfig{
		Mounts: mounts,
		RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
	}
	
	hostConfig.PortBindings = make(nat.PortMap)
	for containerPort, hostPorts := range portBindings {
		natPort := nat.Port(containerPort)
		for _, hostPort := range hostPorts {
			hostConfig.PortBindings[natPort] = append(
				hostConfig.PortBindings[natPort],
				nat.PortBinding{HostIP: "0.0.0.0", HostPort: hostPort},
			)
		}
	}
	
	resp, err := m.dockerClient.ContainerCreate(ctx, config, hostConfig, nil, nil, m.config.ContainerName)
	if err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}
	
	if err := m.dockerClient.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}
	
	m.config.ContainerID = resp.ID
	m.config.IsRunning = true
	memory.Update[*EmailServerConfig](m.db, "email_server_configs", m.config)
	
	time.Sleep(10 * time.Second)
	m.createPostmasterAccountInContainer()
	
	// Wait for container to be fully ready
	go func() {
		if err := m.waitForContainerReady(30); err != nil {
			log.Printf("⚠️  %v", err)
		}
	}()
	
	return nil
}

func (m *EmailManager) StopServer() error {
	if m.dockerClient == nil {
		return fmt.Errorf("Docker is not available. Please start Docker daemon first")
	}
	
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	if m.config.ContainerID == "" {
		return fmt.Errorf("no container running")
	}
	
	ctx := context.Background()
	timeout := 30
	
	if err := m.dockerClient.ContainerStop(ctx, m.config.ContainerID, container.StopOptions{Timeout: &timeout}); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}
	
	m.config.IsRunning = false
	memory.Update[*EmailServerConfig](m.db, "email_server_configs", m.config)
	
	return nil
}

func (m *EmailManager) RestartServer() error {
	if err := m.StopServer(); err != nil {
		return err
	}
	return m.StartServer()
}

func (m *EmailManager) GetServerStatus() (*EmailServerConfig, error) {
	if m.config.ContainerID == "" || m.dockerClient == nil {
		m.config.IsRunning = false
		return m.config, nil
	}
	
	ctx := context.Background()
	inspect, err := m.dockerClient.ContainerInspect(ctx, m.config.ContainerID)
	if err != nil {
		m.config.IsRunning = false
	} else {
		m.config.IsRunning = inspect.State.Running
	}
	
	return m.config, nil
}

func (m *EmailManager) GetDB() *memory.Database {
	return m.db
}

func (m *EmailManager) restorePasswordCache() {
	mailboxes := memory.FindAll[*EmailMailbox](m.db, "email_mailboxes")
	
	m.passwordCacheMux.Lock()
	defer m.passwordCacheMux.Unlock()
	
	for _, mailbox := range mailboxes {
		if mailbox.PlainPassword != "" {
			decryptedPassword, err := security.DecryptAES256GCM(mailbox.PlainPassword, m.encryptionKey)
			if err != nil {
				continue
			}
			m.passwordCache[mailbox.Email] = decryptedPassword
		}
	}
}

func (m *EmailManager) GetConfig() *EmailServerConfig {
	return m.config
}

func (m *EmailManager) getContainerID() string {
	if m.config == nil {
		return ""
	}
	return m.config.ContainerID
}

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
		return "", fmt.Errorf("password not set for %s - use PUT /api/mailboxes/%d/password to set it", email, mailbox.ID)
	}
	
	decryptedPassword, err := security.DecryptAES256GCM(mailbox.PlainPassword, m.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt password for %s: %w", email, err)
	}
	
	m.passwordCacheMux.Lock()
	m.passwordCache[email] = decryptedPassword
	m.passwordCacheMux.Unlock()
	
	return decryptedPassword, nil
}

func (m *EmailManager) createPostmasterAccountInContainer() error {
	password, err := security.GenerateSecurePassword(32)
	if err != nil {
		return err
	}
	email := fmt.Sprintf("postmaster@%s", m.config.Hostname)
	
	cmd := exec.Command("docker", "exec", m.config.ContainerName,
		"setup", "email", "add", email, password)
	
	if err := cmd.Run(); err != nil {
		return err
	}
	
	m.passwordCacheMux.Lock()
	m.passwordCache[email] = password
	m.passwordCacheMux.Unlock()
	
	return nil
}

func (m *EmailManager) autoFixOpenDKIMConfig() {
	time.Sleep(5 * time.Second)
	
	cmd := exec.Command("docker", "ps", "--filter", fmt.Sprintf("name=%s", m.config.ContainerName), "--format", "{{.Names}}")
	output, err := cmd.CombinedOutput()
	if err != nil || len(output) == 0 {
		return
	}
	
	checkCmd := exec.Command("docker", "exec", m.config.ContainerName, "test", "-f", "/tmp/docker-mailserver/opendkim/SigningTable")
	if err := checkCmd.Run(); err != nil {
		return
	}
	
	syncCommands := [][]string{
		{"docker", "exec", m.config.ContainerName, "bash", "-c", "mkdir -p /etc/opendkim/keys"},
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
}

func (m *EmailManager) autoDetectPublicIP() {
	services := []string{
		"https://ifconfig.me/ip",
		"https://api.ipify.org",
		"https://icanhazip.com",
	}
	
	var detectedIP string
	for _, service := range services {
		ip, err := detectIPFromService(service)
		if err == nil && ip != "" {
			detectedIP = ip
			break
		}
	}
	
	if detectedIP == "" {
		return
	}
	
	m.config.IPAddress = detectedIP
	memory.Update[*EmailServerConfig](m.db, "email_server_configs", m.config)
}

func detectIPFromService(serviceURL string) (string, error) {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	
	resp, err := client.Get(serviceURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("service returned status %d", resp.StatusCode)
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	
	ip := strings.TrimSpace(string(body))
	if len(ip) < 7 || len(ip) > 15 || !strings.Contains(ip, ".") {
		return "", fmt.Errorf("invalid IP format")
	}
	return ip, nil
}

func (m *EmailManager) UpdateServerIP(newIP string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	if newIP == "" {
		return fmt.Errorf("IP address cannot be empty")
	}
	
	parts := strings.Split(newIP, ".")
	if len(parts) != 4 {
		return fmt.Errorf("invalid IP address format")
	}
	
	m.config.IPAddress = newIP
	memory.Update[*EmailServerConfig](m.db, "email_server_configs", m.config)
	return nil
}

func (m *EmailManager) fixDKIMPermissions() {
	// Fix permissions for all DKIM keys
	dkimKeysPath := filepath.Join(m.dataPath, "config", "opendkim", "keys")
	
	// Walk through all domain directories
	filepath.Walk(dkimKeysPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		
		if !info.IsDir() {
			// Fix file permissions (make readable by all)
			if strings.HasSuffix(path, ".private") || strings.HasSuffix(path, ".txt") {
				os.Chmod(path, 0644)
			}
		} else {
			// Fix directory permissions
			os.Chmod(path, 0755)
		}
		
		return nil
	})
}

func (m *EmailManager) isContainerReady() bool {
	// Check SMTP port (587)
	smtpAddr := fmt.Sprintf("localhost:%d", m.config.SubmissionPort)
	if conn, err := net.DialTimeout("tcp", smtpAddr, 1*time.Second); err == nil {
		conn.Close()
		
		// Check IMAP port (143)
		imapAddr := fmt.Sprintf("localhost:%d", m.config.IMAPPort)
		if conn, err := net.DialTimeout("tcp", imapAddr, 1*time.Second); err == nil {
			conn.Close()
			return true
		}
	}
	
	return false
}

func (m *EmailManager) waitForContainerReady(maxWaitSeconds int) error {
	for i := 0; i < maxWaitSeconds; i++ {
		if m.isContainerReady() {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	
	return fmt.Errorf("container did not become ready in %d seconds", maxWaitSeconds)
}

func (m *EmailManager) ensureVolumeExists(ctx context.Context, volumeName string) error {
	// Check if volume already exists
	volumes, err := m.dockerClient.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list volumes: %w", err)
	}
	
	for _, vol := range volumes.Volumes {
		if vol.Name == volumeName {
			return nil
		}
	}
	
	// Create volume if it doesn't exist
	_, err = m.dockerClient.VolumeCreate(ctx, volume.CreateOptions{
		Name:   volumeName,
		Driver: "local",
		Labels: map[string]string{
			"app": "redock",
			"type": "mail-data",
		},
	})
	
	if err != nil {
		return fmt.Errorf("failed to create volume: %w", err)
	}
	
	return nil
}

// fixMailPermissions removed - not needed with Docker named volumes
// Named volumes handle permissions automatically inside container
