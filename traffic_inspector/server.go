package traffic_inspector

import (
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"redock/platform/memory"
	"redock/vpn_server"
)

// publishConnEvent surfaces a connection that carried no decrypted content
// (a blocked/pinned handshake, a QUIC attempt, an upstream error) to the
// dashboard so it still shows up in the live-traffic table. The FlowID is
// deliberately *stable* per (status, user, protocol, sni/host, port) rather
// than unique-per-attempt: a pinned client retries the same target dozens of
// times a second, and a stable ID collapses that storm into one row that the
// frontend bumps in place instead of flooding the list.
func (m *Manager) publishConnEvent(protocol, sni, host string, port int, userID uint, status, reason string) {
	if m == nil || m.Hub == nil {
		return
	}
	key := sni
	if key == "" {
		key = host
	}
	m.Hub.Publish(FlowEvent{
		Timestamp: time.Now().Unix(),
		UserID:    userID,
		FlowID:    fmt.Sprintf("%s|%d|%s|%s|%d", status, userID, protocol, key, port),
		Protocol:  protocol,
		SNI:       sni,
		Host:      host,
		Port:      port,
		Status:    status,
		Error:     reason,
		Closed:    true,
	})
}

// parseTLSClientHelloSNI extracts the server_name (SNI) from a raw TLS
// ClientHello record, without terminating the handshake — so a connection the
// client later refuses (cert pinning) can still be labelled with its target
// hostname in the dashboard. Returns "" if the bytes aren't a ClientHello or
// carry no SNI. Defensive on every length so a truncated/garbage peek can't
// panic or read out of bounds.
func parseTLSClientHelloSNI(b []byte) string {
	// TLS record header (5) + handshake header (4) minimum.
	if len(b) < 43 || b[0] != 0x16 || b[5] != 0x01 {
		return ""
	}
	// Skip: record hdr(5) + handshake hdr(4) + client_version(2) + random(32).
	pos := 43
	// session_id
	if pos >= len(b) {
		return ""
	}
	pos += 1 + int(b[pos])
	// cipher_suites
	if pos+2 > len(b) {
		return ""
	}
	csLen := int(b[pos])<<8 | int(b[pos+1])
	pos += 2 + csLen
	// compression_methods
	if pos+1 > len(b) {
		return ""
	}
	pos += 1 + int(b[pos])
	// extensions
	if pos+2 > len(b) {
		return ""
	}
	extTotal := int(b[pos])<<8 | int(b[pos+1])
	pos += 2
	end := pos + extTotal
	if end > len(b) {
		end = len(b)
	}
	for pos+4 <= end {
		extType := int(b[pos])<<8 | int(b[pos+1])
		extLen := int(b[pos+2])<<8 | int(b[pos+3])
		pos += 4
		if pos+extLen > end {
			return ""
		}
		if extType == 0x0000 { // server_name
			sni := parseSNIExtension(b[pos : pos+extLen])
			if sni != "" {
				return sni
			}
		}
		pos += extLen
	}
	return ""
}

// parseSNIExtension reads the first host_name entry from a server_name
// extension body (ServerNameList).
func parseSNIExtension(ext []byte) string {
	// server_name_list length(2) + name_type(1) + host_name length(2).
	if len(ext) < 5 {
		return ""
	}
	// ext[0:2] = list length; ext[2] = name_type (0 = host_name).
	if ext[2] != 0x00 {
		return ""
	}
	nameLen := int(ext[3])<<8 | int(ext[4])
	if 5+nameLen > len(ext) {
		return ""
	}
	return string(ext[5 : 5+nameLen])
}

// tcpProxyPortBase + serverID gives a deterministic loopback port for each
// VPN server's TCP MITM listener, so redirect rules can be constructed
// without any runtime coordination.
const tcpProxyPortBase = 20000

// serverInterceptor is the per-VPN-server TCP MITM listener plus the set of
// currently-redirected (inspected) user IPs for that server.
type serverInterceptor struct {
	serverID     uint
	tunIface     string
	uplinkIface  string
	tcpProxyPort int
	listener     net.Listener
	quic         *quicInterceptor // nil if QUIC listener failed to start; TCP MITM still works

	mu            sync.Mutex
	redirectedIPs map[string]bool
}

// onServerNATConfigured is registered as vpn_server.OnNATConfigured; it
// starts (or updates) this server's TCP MITM listener and re-applies
// redirect rules for any already-inspection-enabled users.
func (m *Manager) onServerNATConfigured(ev vpn_server.NATEvent) {
	m.mu.Lock()
	si, exists := m.servers[ev.ServerID]
	m.mu.Unlock()

	if exists {
		si.tunIface = ev.TunInterface
		si.uplinkIface = ev.UplinkInterface
	} else {
		var err error
		si, err = m.startServerInterceptor(ev.ServerID, ev.TunInterface, ev.UplinkInterface)
		if err != nil {
			logWarn("traffic_inspector: failed to start interceptor for server %d: %v", ev.ServerID, err)
			return
		}
		m.mu.Lock()
		m.servers[ev.ServerID] = si
		m.mu.Unlock()
	}

	m.resyncServerUsers(ev.ServerID)
}

// onServerNATCleanup is registered as vpn_server.OnNATCleanup; it tears down
// this server's TCP MITM listener and removes any redirect rules.
func (m *Manager) onServerNATCleanup(serverID uint, tunIface string) {
	m.mu.Lock()
	si, exists := m.servers[serverID]
	delete(m.servers, serverID)
	m.mu.Unlock()

	if !exists {
		return
	}
	si.stop()
}

// SyncUserRedirect applies (or removes) the redirect rule for a single
// user's current InspectionEnabled state. Called by the controller layer
// right after a VPNUser update. No-op if the user's server isn't running —
// the desired state is re-read from the DB and applied on next server start
// via resyncServerUsers.
func (m *Manager) SyncUserRedirect(user *vpn_server.VPNUser) {
	m.mu.Lock()
	si, exists := m.servers[user.ServerID]
	m.mu.Unlock()
	if !exists {
		return
	}
	si.setUserRedirect(user.Address, user.InspectionEnabled)
}

func (m *Manager) resyncServerUsers(serverID uint) {
	if m.db == nil {
		return
	}

	m.mu.Lock()
	si, exists := m.servers[serverID]
	m.mu.Unlock()
	if !exists {
		return
	}

	users := memory.Filter[*vpn_server.VPNUser](m.db, "vpn_users", func(u *vpn_server.VPNUser) bool {
		return u.ServerID == serverID && u.Enabled
	})
	for _, u := range users {
		si.setUserRedirect(u.Address, u.InspectionEnabled)
	}
}

// lookupUserByIP finds the VPNUser whose tunnel address matches the given
// bare IP (used to tag captured flows with a UserID).
func (m *Manager) lookupUserByIP(ip string) *vpn_server.VPNUser {
	if m.db == nil {
		return nil
	}
	matches := memory.Filter[*vpn_server.VPNUser](m.db, "vpn_users", func(u *vpn_server.VPNUser) bool {
		return stripCIDR(u.Address) == ip
	})
	if len(matches) == 0 {
		return nil
	}
	return matches[0]
}

func (m *Manager) startServerInterceptor(serverID uint, tunIface, uplinkIface string) (*serverInterceptor, error) {
	port := tcpProxyPortBase + int(serverID)
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, fmt.Errorf("listen on proxy port %d: %w", port, err)
	}

	si := &serverInterceptor{
		serverID:      serverID,
		tunIface:      tunIface,
		uplinkIface:   uplinkIface,
		tcpProxyPort:  port,
		listener:      ln,
		redirectedIPs: make(map[string]bool),
	}

	go si.acceptLoop(m)

	log.Printf("Traffic Inspector: TCP MITM listener for VPN server %d on 127.0.0.1:%d (tun=%s)", serverID, port, tunIface)

	if qi, qerr := m.startQUICInterceptor(serverID, tunIface); qerr != nil {
		logWarn("traffic_inspector: QUIC MITM listener failed to start for server %d (TCP interception still active): %v", serverID, qerr)
	} else {
		si.quic = qi
	}

	return si, nil
}

func (si *serverInterceptor) acceptLoop(m *Manager) {
	for {
		conn, err := si.listener.Accept()
		if err != nil {
			return // listener closed (server stopped)
		}
		go handleInterceptedConn(m, conn)
	}
}

func (si *serverInterceptor) stop() {
	si.listener.Close()
	if si.quic != nil {
		si.quic.stop()
	}

	si.mu.Lock()
	ips := make([]string, 0, len(si.redirectedIPs))
	for ip := range si.redirectedIPs {
		ips = append(ips, ip)
	}
	si.mu.Unlock()

	for _, ip := range ips {
		if err := RemoveUserRedirect(si.tunIface, ip, si.tcpProxyPort); err != nil {
			logWarn("traffic_inspector: failed to remove TCP redirect rule for %s: %v", ip, err)
		}
		if si.quic != nil {
			if err := RemoveUserRedirectQUIC(si.tunIface, ip, si.quic.udpProxyPort); err != nil {
				logWarn("traffic_inspector: failed to remove QUIC redirect rule for %s: %v", ip, err)
			}
		}
	}
}

func (si *serverInterceptor) setUserRedirect(userAddress string, enabled bool) {
	ip := stripCIDR(userAddress)
	if ip == "" {
		return
	}

	si.mu.Lock()
	already := si.redirectedIPs[ip]
	si.mu.Unlock()

	if enabled == already {
		return
	}

	var err error
	if enabled {
		err = AddUserRedirect(si.tunIface, ip, si.tcpProxyPort)
	} else {
		err = RemoveUserRedirect(si.tunIface, ip, si.tcpProxyPort)
	}
	if err != nil {
		logWarn("traffic_inspector: TCP redirect rule sync failed for %s (enabled=%v): %v", ip, enabled, err)
		return
	}
	log.Printf("traffic_inspector: TCP redirect %s for %s -> 127.0.0.1:%d on %s", map[bool]string{true: "ENABLED", false: "disabled"}[enabled], ip, si.tcpProxyPort, si.tunIface)

	if si.quic != nil {
		var qerr error
		if enabled {
			qerr = AddUserRedirectQUIC(si.tunIface, ip, si.quic.udpProxyPort)
		} else {
			qerr = RemoveUserRedirectQUIC(si.tunIface, ip, si.quic.udpProxyPort)
		}
		if qerr != nil {
			logWarn("traffic_inspector: QUIC redirect rule sync failed for %s (enabled=%v): %v", ip, enabled, qerr)
		}
	}

	si.mu.Lock()
	if enabled {
		si.redirectedIPs[ip] = true
	} else {
		delete(si.redirectedIPs, ip)
	}
	si.mu.Unlock()
}

// stripCIDR turns "10.0.0.2/32" into "10.0.0.2"; returns the input unchanged
// if it isn't in CIDR form.
func stripCIDR(address string) string {
	ip, _, _ := strings.Cut(address, "/")
	return ip
}
