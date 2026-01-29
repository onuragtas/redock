package vpn_server

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	docker_manager "redock/docker-manager"
	"redock/platform/memory"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/curve25519"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

var (
	vpnManagerInstance *WireGuardManager
	vpnManagerOnce     sync.Once
)

// WireGuardServerInstance represents a running WireGuard server
type WireGuardServerInstance struct {
	Server   *VPNServer
	Device   *device.Device
	TUN      tun.Device
	Bind     conn.Bind
	WGClient *wgctrl.Client
	StopChan chan struct{}
	Running  bool
}

// WireGuardManager manages WireGuard VPN servers and users
type WireGuardManager struct {
	db        *memory.Database
	servers   map[uint]*VPNServer
	instances map[uint]*WireGuardServerInstance
	mutex     sync.RWMutex
	running   bool
}

// GetWireGuardManager returns singleton instance
func GetWireGuardManager() *WireGuardManager {
	vpnManagerOnce.Do(func() {
		vpnManagerInstance = &WireGuardManager{
			servers:   make(map[uint]*VPNServer),
			instances: make(map[uint]*WireGuardServerInstance),
		}
	})
	return vpnManagerInstance
}

func (m *WireGuardManager) Init(db *memory.Database) error {
	m.db = db
	m.running = true

	m.cleanupAllAnchors()

	if err := m.loadServers(); err != nil {
		return fmt.Errorf("failed to load servers: %w", err)
	}

	m.autoStartEnabledServers()
	go m.collectStats()

	return nil
}

// loadServers loads all VPN servers from database
func (m *WireGuardManager) loadServers() error {
	servers := memory.FindAll[*VPNServer](m.db, "vpn_servers")

	m.mutex.Lock()
	defer m.mutex.Unlock()

	for _, server := range servers {
		m.servers[server.ID] = server
	}

	return nil
}

func (m *WireGuardManager) autoStartEnabledServers() {
	m.mutex.RLock()
	servers := make([]*VPNServer, 0)
	for _, server := range m.servers {
		if server.Enabled {
			servers = append(servers, server)
		}
	}
	m.mutex.RUnlock()

	if len(servers) == 0 {
		return
	}

	for _, server := range servers {
		if m.IsServerRunning(server.ID) {
			continue
		}

		go func(srv *VPNServer) {
			if err := m.StartServer(srv.ID); err != nil {
				log.Printf("⚠️  Failed to start %s: %v", srv.Name, err)
			}
		}(server)
	}
}

// IsServerRunning checks if server is running
func (m *WireGuardManager) IsServerRunning(serverID uint) bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	instance, exists := m.instances[serverID]
	return exists && instance != nil && instance.Running
}

// generateKeyPair generates WireGuard key pair
func generateKeyPair() (privateKey, publicKey string, err error) {
	privateKeyBytes := make([]byte, 32)
	if _, err := rand.Read(privateKeyBytes); err != nil {
		return "", "", err
	}

	// Clamp private key
	privateKeyBytes[0] &= 248
	privateKeyBytes[31] &= 127
	privateKeyBytes[31] |= 64

	// Generate public key
	var publicKeyBytes [32]byte
	curve25519.ScalarBaseMult(&publicKeyBytes, (*[32]byte)(privateKeyBytes))

	privateKey = base64.StdEncoding.EncodeToString(privateKeyBytes)
	publicKey = base64.StdEncoding.EncodeToString(publicKeyBytes[:])

	return privateKey, publicKey, nil
}

func (m *WireGuardManager) CreateServer(name, address, endpoint, dns string) (*VPNServer, error) {
	privateKey, publicKey, err := generateKeyPair()
	if err != nil {
		return nil, fmt.Errorf("failed to generate keys: %w", err)
	}

	// Use default DNS if not provided
	if dns == "" {
		dns = "1.1.1.1,8.8.8.8"
	}

	server := &VPNServer{
		Name:                name,
		Interface:           "",
		PublicKey:           publicKey,
		PrivateKey:          privateKey,
		ListenPort:          51820,
		Address:             address,
		Endpoint:            endpoint,
		DNS:                 dns,
		AllowedIPs:          "0.0.0.0/0",
		MTU:                 1420,
		PersistentKeepalive: 25,
		Enabled:             true,
	}

	if err := memory.Create[*VPNServer](m.db, "vpn_servers", server); err != nil {
		return nil, fmt.Errorf("failed to create server: %w", err)
	}

	m.mutex.Lock()
	m.servers[server.ID] = server
	m.mutex.Unlock()

	if server.Enabled {
		if err := m.StartServer(server.ID); err != nil {
			log.Printf("⚠️  Failed to start server: %v", err)
		}
	}

	return server, nil
}

// StartServer starts a WireGuard server
func (m *WireGuardManager) StartServer(serverID uint) error {
	server, err := memory.FindByID[*VPNServer](m.db, "vpn_servers", serverID)
	if err != nil {
		return fmt.Errorf("server not found: %w", err)
	}

	m.mutex.RLock()
	if instance, exists := m.instances[serverID]; exists && instance.Running {
		m.mutex.RUnlock()
		return nil
	}
	m.mutex.RUnlock()

	return m.startServerInstance(server)
}

func (m *WireGuardManager) startServerInstance(server *VPNServer) error {
	// Check if private key is empty (old servers or corrupted data)
	if server.PrivateKey == "" {
		log.Printf("⚠️  Server %s has no private key, regenerating...", server.Name)
		privateKeyStr, publicKeyStr, err := generateKeyPair()
		if err != nil {
			return fmt.Errorf("failed to generate keys: %w", err)
		}
		server.PrivateKey = privateKeyStr
		server.PublicKey = publicKeyStr
		if err := memory.Update[*VPNServer](m.db, "vpn_servers", server); err != nil {
			return fmt.Errorf("failed to update server keys: %w", err)
		}
	}

	privateKey, err := wgtypes.ParseKey(server.PrivateKey)
	if err != nil {
		return fmt.Errorf("invalid private key: %w", err)
	}

	serverIP, serverNet, err := net.ParseCIDR(server.Address)
	if err != nil {
		return fmt.Errorf("invalid address: %w", err)
	}

	tunName := fmt.Sprintf("utun%d", server.ID)
	tunDevice, err := tun.CreateTUN(tunName, server.MTU)
	if err != nil && strings.Contains(err.Error(), "busy") {
		for offset := uint(100); offset < 200; offset += 10 {
			tunName = fmt.Sprintf("utun%d", server.ID+offset)
			tunDevice, err = tun.CreateTUN(tunName, server.MTU)
			if err == nil {
				break
			}
		}
	}
	if err != nil {
		return fmt.Errorf("failed to create TUN: %w", err)
	}

	actualName, err := tunDevice.Name()
	if err != nil {
		tunDevice.Close()
		return fmt.Errorf("failed to get interface name: %w", err)
	}

	server.Interface = actualName
	memory.Update[*VPNServer](m.db, "vpn_servers", server)

	udpBind := conn.NewDefaultBind()
	logger := &device.Logger{
		Verbosef: func(format string, args ...interface{}) {},
		Errorf:   func(format string, args ...interface{}) {},
	}

	wgDevice := device.NewDevice(tunDevice, udpBind, logger)

	config := fmt.Sprintf("private_key=%s\nlisten_port=%d\n",
		hex.EncodeToString(privateKey[:]),
		server.ListenPort)

	// Get enabled users for this server
	users := memory.Filter[*VPNUser](m.db, "vpn_users", func(u *VPNUser) bool {
		return u.ServerID == server.ID && u.Enabled
	})

	for _, user := range users {
		pubKeyBytes, err := base64.StdEncoding.DecodeString(user.PublicKey)
		if err != nil {
			continue
		}
		pubKeyHex := hex.EncodeToString(pubKeyBytes)
		config += fmt.Sprintf("public_key=%s\nallowed_ip=%s\n", pubKeyHex, user.Address)
		if server.PersistentKeepalive > 0 {
			config += fmt.Sprintf("persistent_keepalive_interval=%d\n", server.PersistentKeepalive)
		}
	}

	if err := wgDevice.IpcSet(config); err != nil {
		tunDevice.Close()
		return fmt.Errorf("failed to configure device: %w", err)
	}

	if err := wgDevice.Up(); err != nil {
		tunDevice.Close()
		return fmt.Errorf("failed to start device: %w", err)
	}

	if err := m.configureNetwork(actualName, serverIP, serverNet); err != nil {
		log.Printf("⚠️  Network configuration warning: %v", err)
	}

	wgClient, _ := wgctrl.New()

	instance := &WireGuardServerInstance{
		Server:   server,
		Device:   wgDevice,
		TUN:      tunDevice,
		Bind:     udpBind,
		WGClient: wgClient,
		StopChan: make(chan struct{}),
		Running:  true,
	}

	m.mutex.Lock()
	m.instances[server.ID] = instance
	m.mutex.Unlock()

	log.Printf("VPN Server Started: %s:%d", actualName, server.ListenPort)
	return nil
}

func (m *WireGuardManager) configureNetwork(ifName string, serverIP net.IP, serverNet *net.IPNet) error {
	if err := m.assignIP(ifName, serverIP, serverNet); err != nil {
		return fmt.Errorf("IP assignment failed: %w", err)
	}

	if err := m.addRoute(ifName, serverNet); err != nil {
		return fmt.Errorf("route addition failed: %w", err)
	}

	if err := m.enableForwarding(); err != nil {
		log.Printf("⚠️  IP forwarding: %v", err)
	}

	uplinkIface, err := m.getUplinkInterface()
	if err != nil {
		return fmt.Errorf("uplink detection failed: %w", err)
	}

	if err := m.setupNAT(ifName, serverNet, uplinkIface); err != nil {
		return fmt.Errorf("NAT setup failed: %w", err)
	}

	return nil
}

func (m *WireGuardManager) assignIP(ifName string, ip net.IP, ipNet *net.IPNet) error {
	switch runtime.GOOS {
	case "darwin":
		networkAddr := ipNet.IP.String()
		mask := ipNet.Mask
		maskStr := fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])

		cmd := exec.Command("ifconfig", ifName, "inet", ip.String(), networkAddr, "netmask", maskStr, "up")
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("ifconfig failed: %v, output: %s", err, string(output))
		}
		return nil

	case "linux":
		addr := ipNet.String()
		cmd := exec.Command("ip", "addr", "add", addr, "dev", ifName)
		if output, err := cmd.CombinedOutput(); err != nil {
			if !strings.Contains(string(output), "exists") {
				return fmt.Errorf("ip addr failed: %v, output: %s", err, string(output))
			}
		}
		cmd = exec.Command("ip", "link", "set", "dev", ifName, "up")
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("ip link up failed: %v, output: %s", err, string(output))
		}
		return nil

	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func (m *WireGuardManager) addRoute(ifName string, network *net.IPNet) error {
	networkStr := network.String()

	switch runtime.GOOS {
	case "darwin":
		cmd := exec.Command("route", "add", "-net", networkStr, "-interface", ifName)
		output, err := cmd.CombinedOutput()
		if err != nil {
			if strings.Contains(string(output), "exists") || strings.Contains(string(output), "File exists") {
				return nil
			}
			return fmt.Errorf("route add failed: %v, output: %s", err, string(output))
		}
		return nil

	case "linux":
		cmd := exec.Command("ip", "route", "add", networkStr, "dev", ifName)
		output, err := cmd.CombinedOutput()
		if err != nil {
			if strings.Contains(string(output), "exists") {
				return nil
			}
			return fmt.Errorf("ip route add failed: %v, output: %s", err, string(output))
		}
		return nil

	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func (m *WireGuardManager) enableForwarding() error {
	switch runtime.GOOS {
	case "darwin":
		cmd := exec.Command("sysctl", "-w", "net.inet.ip.forwarding=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("sysctl failed: %v, output: %s", err, string(output))
		}
		return nil

	case "linux":
		cmd := exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("sysctl failed: %v, output: %s", err, string(output))
		}
		return nil

	default:
		return nil
	}
}

func (m *WireGuardManager) getUplinkInterface() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	localIP := localAddr.IP

	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	for _, iface := range ifaces {
		if strings.HasPrefix(iface.Name, "utun") ||
			strings.HasPrefix(iface.Name, "tun") ||
			strings.HasPrefix(iface.Name, "wg") ||
			strings.HasPrefix(iface.Name, "lo") {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok {
				if ipNet.IP.Equal(localIP) {
					return iface.Name, nil
				}
			}
		}
	}

	for _, name := range []string{"en0", "eth0", "wlan0"} {
		if _, err := net.InterfaceByName(name); err == nil {
			return name, nil
		}
	}

	return "", fmt.Errorf("no uplink interface found")
}

func (m *WireGuardManager) setupNAT(tunIface string, vpnNet *net.IPNet, uplinkIface string) error {
	vpnCIDR := vpnNet.String()

	switch runtime.GOOS {
	case "darwin":
		anchorName := fmt.Sprintf("redock.vpn.%s", tunIface)
		anchorFile := fmt.Sprintf("/etc/pf.anchors/%s", anchorName)
		mainRuleFile := "/etc/pf.conf"

		os.MkdirAll("/etc/pf.anchors", 0755)

		anchorRules := fmt.Sprintf(`# Redock VPN rules for %s
nat on %s inet from %s to any -> (%s)

pass in quick on %s inet from %s to any keep state
pass out quick on %s inet from %s to any keep state
`, tunIface, uplinkIface, vpnCIDR, uplinkIface, tunIface, vpnCIDR, uplinkIface, vpnCIDR)

		if err := os.WriteFile(anchorFile, []byte(anchorRules), 0644); err != nil {
			return fmt.Errorf("write anchor file failed: %w", err)
		}

		mainContent := ""
		if data, err := os.ReadFile(mainRuleFile); err == nil {
			mainContent = string(data)
		}

		anchorLoadLine := fmt.Sprintf("load anchor %s from \"%s\"", anchorName, anchorFile)
		anchorNATLine := fmt.Sprintf("nat-anchor \"%s\"", anchorName)
		anchorRDRLine := fmt.Sprintf("rdr-anchor \"%s\"", anchorName)
		anchorFilterLine := fmt.Sprintf("anchor \"%s\"", anchorName)

		needsUpdate := false
		if !strings.Contains(mainContent, anchorLoadLine) {
			needsUpdate = true
		}

		if needsUpdate {
			backupFile := "/etc/pf.conf.redock.backup"
			if _, err := os.Stat(backupFile); os.IsNotExist(err) {
				exec.Command("cp", mainRuleFile, backupFile).Run()
			}

			var newConfig strings.Builder
			newConfig.WriteString(anchorLoadLine + "\n\n")
			newConfig.WriteString("# Scrub (normalization)\n")
			newConfig.WriteString("scrub-anchor \"com.apple/*\" all fragment reassemble\n\n")
			newConfig.WriteString("# Translation (NAT/RDR)\n")
			newConfig.WriteString(anchorNATLine + "\n")
			newConfig.WriteString(anchorRDRLine + "\n\n")
			newConfig.WriteString("# Filtering\n")
			newConfig.WriteString("anchor \"com.apple/*\" all\n")
			newConfig.WriteString(anchorFilterLine + "\n")

			if err := os.WriteFile(mainRuleFile, []byte(newConfig.String()), 0644); err != nil {
				return fmt.Errorf("write pf.conf failed: %w", err)
			}
		}

		reloadCmd := exec.Command("pfctl", "-f", mainRuleFile)
		if output, err := reloadCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("pfctl reload failed: %v, output: %s", err, string(output))
		}

		exec.Command("pfctl", "-e").Run()
		return nil

	case "linux":
		checkCmd := exec.Command("iptables", "-t", "nat", "-C", "POSTROUTING", "-s", vpnCIDR, "-o", uplinkIface, "-j", "MASQUERADE")
		if checkCmd.Run() == nil {
			return nil
		}

		cmd := exec.Command("iptables", "-t", "nat", "-A", "POSTROUTING", "-s", vpnCIDR, "-o", uplinkIface, "-j", "MASQUERADE")
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("iptables NAT failed: %v, output: %s", err, string(output))
		}

		exec.Command("iptables", "-I", "FORWARD", "1", "-i", tunIface, "-j", "ACCEPT").Run()
		exec.Command("iptables", "-I", "FORWARD", "1", "-o", tunIface, "-j", "ACCEPT").Run()
		return nil

	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// StopServer stops a WireGuard server
func (m *WireGuardManager) StopServer(serverID uint) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	instance, exists := m.instances[serverID]
	if !exists || !instance.Running {
		return fmt.Errorf("server not running")
	}

	// Cleanup NAT
	m.cleanupNAT(instance.Server.Interface, instance.Server.Address)

	// Stop device
	instance.Device.Down()
	instance.Device.Close()
	instance.TUN.Close()
	if instance.Bind != nil {
		instance.Bind.Close()
	}

	close(instance.StopChan)
	instance.Running = false
	delete(m.instances, serverID)

	return nil
}

func (m *WireGuardManager) cleanupNAT(tunIface, address string) {
	_, vpnNet, _ := net.ParseCIDR(address)
	if vpnNet == nil {
		return
	}

	switch runtime.GOOS {
	case "darwin":
		anchorName := fmt.Sprintf("redock.vpn.%s", tunIface)
		anchorFile := fmt.Sprintf("/etc/pf.anchors/%s", anchorName)
		os.Remove(anchorFile)

		mainRuleFile := "/etc/pf.conf"
		data, err := os.ReadFile(mainRuleFile)
		if err != nil {
			return
		}

		lines := strings.Split(string(data), "\n")
		var newLines []string

		for _, line := range lines {
			if strings.Contains(line, anchorName) {
				continue
			}
			newLines = append(newLines, line)
		}

		newContent := strings.Join(newLines, "\n")
		os.WriteFile(mainRuleFile, []byte(newContent), 0644)
		exec.Command("pfctl", "-f", mainRuleFile).Run()

	case "linux":
		vpnCIDR := vpnNet.String()
		uplinkIface, _ := m.getUplinkInterface()
		if uplinkIface != "" {
			exec.Command("iptables", "-t", "nat", "-D", "POSTROUTING", "-s", vpnCIDR, "-o", uplinkIface, "-j", "MASQUERADE").Run()
		}
		exec.Command("iptables", "-D", "FORWARD", "-i", tunIface, "-j", "ACCEPT").Run()
		exec.Command("iptables", "-D", "FORWARD", "-o", tunIface, "-j", "ACCEPT").Run()
	}
}

func (m *WireGuardManager) cleanupAllAnchors() {
	if runtime.GOOS != "darwin" {
		return
	}

	anchors, _ := os.ReadDir("/etc/pf.anchors")
	for _, anchor := range anchors {
		if strings.HasPrefix(anchor.Name(), "redock.vpn.") {
			os.Remove(fmt.Sprintf("/etc/pf.anchors/%s", anchor.Name()))
		}
	}

	mainRuleFile := "/etc/pf.conf"
	data, err := os.ReadFile(mainRuleFile)
	if err != nil {
		return
	}

	lines := strings.Split(string(data), "\n")
	var cleanLines []string

	for _, line := range lines {
		if strings.Contains(line, "redock.vpn.") {
			continue
		}
		cleanLines = append(cleanLines, line)
	}

	cleanContent := strings.Join(cleanLines, "\n")
	os.WriteFile(mainRuleFile, []byte(cleanContent), 0644)
	exec.Command("pfctl", "-f", mainRuleFile).Run()
}

// getNextIP gets next available IP for user
func (m *WireGuardManager) getNextIP(serverID uint) string {
	server, err := memory.FindByID[*VPNServer](m.db, "vpn_servers", serverID)
	if err != nil {
		return "10.0.0.2/32"
	}

	_, ipNet, err := net.ParseCIDR(server.Address)
	if err != nil {
		return "10.0.0.2/32"
	}

	// Get all users for this server
	users := memory.Filter[*VPNUser](m.db, "vpn_users", func(u *VPNUser) bool {
		return u.ServerID == serverID
	})

	usedIPs := make(map[string]bool)
	for _, user := range users {
		ip := strings.Split(user.Address, "/")[0]
		usedIPs[ip] = true
	}

	ip := ipNet.IP
	for i := 2; i < 254; i++ {
		ip[3] = byte(i)
		ipStr := ip.String()
		if !usedIPs[ipStr] {
			return fmt.Sprintf("%s/32", ipStr)
		}
	}

	return "10.0.0.2/32"
}

func (m *WireGuardManager) AddUser(serverID uint, username, email, fullName, dns string) (*VPNUser, error) {
	privateKey, publicKey, err := generateKeyPair()
	if err != nil {
		return nil, fmt.Errorf("failed to generate keys: %w", err)
	}

	server, err := memory.FindByID[*VPNServer](m.db, "vpn_servers", serverID)
	if err != nil {
		return nil, fmt.Errorf("server not found: %w", err)
	}

	address := m.getNextIP(serverID)

	// Use custom DNS if provided, otherwise use server's DNS
	if dns == "" {
		dns = server.DNS
	}

	user := &VPNUser{
		ServerID:   serverID,
		Username:   username,
		Email:      email,
		FullName:   fullName,
		PublicKey:  publicKey,
		PrivateKey: privateKey,
		Address:    address,
		AllowedIPs: "0.0.0.0/0",
		DNS:        dns,
		Enabled:    true,
	}

	if err := memory.Create[*VPNUser](m.db, "vpn_users", user); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	m.mutex.RLock()
	instance, exists := m.instances[serverID]
	m.mutex.RUnlock()

	if exists && instance.Running {
		pubKeyBytes, _ := base64.StdEncoding.DecodeString(user.PublicKey)
		pubKeyHex := hex.EncodeToString(pubKeyBytes)
		peerConfig := fmt.Sprintf("public_key=%s\nallowed_ip=%s\n", pubKeyHex, user.Address)
		if server.PersistentKeepalive > 0 {
			peerConfig += fmt.Sprintf("persistent_keepalive_interval=%d\n", server.PersistentKeepalive)
		}
		instance.Device.IpcSet(peerConfig)
	}

	return user, nil
}

// GetUserConfig generates client configuration
func (m *WireGuardManager) GetUserConfig(userID uint) (string, error) {
	user, err := memory.FindByID[*VPNUser](m.db, "vpn_users", userID)
	if err != nil {
		return "", fmt.Errorf("user not found: %w", err)
	}

	// Check if user has private key (old users or corrupted data)
	if user.PrivateKey == "" {
		log.Printf("⚠️  User %s has no private key, regenerating...", user.Username)
		privateKeyStr, publicKeyStr, err := generateKeyPair()
		if err != nil {
			return "", fmt.Errorf("failed to generate keys: %w", err)
		}
		user.PrivateKey = privateKeyStr
		user.PublicKey = publicKeyStr
		if err := memory.Update[*VPNUser](m.db, "vpn_users", user); err != nil {
			return "", fmt.Errorf("failed to update user keys: %w", err)
		}
	}

	server, err := memory.FindByID[*VPNServer](m.db, "vpn_servers", user.ServerID)
	if err != nil {
		return "", fmt.Errorf("server not found: %w", err)
	}

	var config strings.Builder
	config.WriteString("[Interface]\n")
	config.WriteString(fmt.Sprintf("PrivateKey = %s\n", user.PrivateKey))
	config.WriteString(fmt.Sprintf("Address = %s\n", user.Address))
	if user.DNS != "" {
		config.WriteString(fmt.Sprintf("DNS = %s\n", user.DNS))
	}

	config.WriteString("\n[Peer]\n")
	config.WriteString(fmt.Sprintf("PublicKey = %s\n", server.PublicKey))

	endpoint := server.Endpoint
	if endpoint == "" {
		localIP := docker_manager.GetDockerManager().GetLocalIP()
		if localIP != "" {
			endpoint = fmt.Sprintf("%s:%d", localIP, server.ListenPort)
		}
	}
	if endpoint != "" {
		config.WriteString(fmt.Sprintf("Endpoint = %s\n", endpoint))
	}

	config.WriteString(fmt.Sprintf("AllowedIPs = %s\n", user.AllowedIPs))
	if server.PersistentKeepalive > 0 {
		config.WriteString(fmt.Sprintf("PersistentKeepalive = %d\n", server.PersistentKeepalive))
	}

	return config.String(), nil
}

// collectStats collects statistics periodically
func (m *WireGuardManager) collectStats() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if !m.running {
			return
		}

		m.mutex.RLock()
		instances := make([]*WireGuardServerInstance, 0)
		for _, inst := range m.instances {
			if inst.Running {
				instances = append(instances, inst)
			}
		}
		m.mutex.RUnlock()

		for _, instance := range instances {
			m.updateServerStats(instance)
		}
	}
}

func (m *WireGuardManager) updateServerStats(instance *WireGuardServerInstance) {
	if instance.Device == nil {
		return
	}

	ipcResp, err := instance.Device.IpcGet()
	if err != nil {
		return
	}

	lines := strings.Split(ipcResp, "\n")
	var currentPubKey string
	peers := make(map[string]map[string]string)
	currentPeer := make(map[string]string)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key, value := parts[0], parts[1]

		if key == "public_key" {
			if currentPubKey != "" {
				peers[currentPubKey] = currentPeer
			}
			currentPubKey = value
			currentPeer = make(map[string]string)
		} else if currentPubKey != "" {
			currentPeer[key] = value
		}
	}
	if currentPubKey != "" {
		peers[currentPubKey] = currentPeer
	}

	for pubKeyHex, peerData := range peers {
		pubKeyBytes, _ := hex.DecodeString(pubKeyHex)
		pubKeyBase64 := base64.StdEncoding.EncodeToString(pubKeyBytes)

		// Find user by public key and server ID
		users := memory.Filter[*VPNUser](m.db, "vpn_users", func(u *VPNUser) bool {
			return u.PublicKey == pubKeyBase64 && u.ServerID == instance.Server.ID
		})
		if len(users) == 0 {
			continue
		}
		user := users[0]

		var rxBytes, txBytes int64
		var lastHandshake *time.Time

		if rx, ok := peerData["transfer_rx_bytes"]; ok {
			fmt.Sscanf(rx, "%d", &rxBytes)
		}
		if tx, ok := peerData["transfer_tx_bytes"]; ok {
			fmt.Sscanf(tx, "%d", &txBytes)
		}
		if hs, ok := peerData["last_handshake_time_sec"]; ok {
			var sec int64
			if n, _ := fmt.Sscanf(hs, "%d", &sec); n == 1 && sec > 0 {
				t := time.Unix(sec, 0)
				lastHandshake = &t
			}
		}

		// Update user stats
		user.TotalBytesReceived = rxBytes
		user.TotalBytesSent = txBytes
		if lastHandshake != nil {
			user.LastConnectedAt = lastHandshake
		}
		memory.Update[*VPNUser](m.db, "vpn_users", user)
	}
}

// GetDB returns database connection
func (m *WireGuardManager) GetDB() (*memory.Database, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return m.db, nil
}

func (m *WireGuardManager) Stop() {
	m.running = false
}
