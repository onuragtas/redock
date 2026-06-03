package controllers

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/user"
	"strconv"
	"strings"
	"time"

	"redock/tunnel_server"
	"redock/tunnel_server/client"
)

const (
	agentPollInterval  = 20 * time.Second
	agentHTTPTimeout   = 20 * time.Second
	agentVersion       = "redock"
	agentDefaultServer = "https://redock.tnpx.org"
	// agentMaxKeepalive caps the tunnel keepalive for agents (which usually sit
	// behind NAT/firewall) so conntrack never drops the idle return path.
	agentMaxKeepalive = 5 * time.Second
)

var agentHTTPClient = &http.Client{Timeout: agentHTTPTimeout}

// StartTunnelAgents launches the autonomous agent loop. The agent always talks
// to the central server (agentDefaultServer); it needs no login or key and
// self-registers at the server's public endpoint using a stable client id + its
// OS hostname/user, then opens whatever tunnels the remote admin assigned to it.
// REDOCK_TUNNEL_AGENT_SERVERS may override the server(s) for dev/testing.
func StartTunnelAgents() {
	clientID := ensureAgentClientID()
	seen := map[string]bool{}
	add := func(baseURL string) {
		baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
		if baseURL == "" || seen[baseURL] {
			return
		}
		seen[baseURL] = true
		go agentLoop(baseURL, clientID)
	}
	if env := strings.TrimSpace(os.Getenv("REDOCK_TUNNEL_AGENT_SERVERS")); env != "" {
		for _, u := range strings.Split(env, ",") {
			add(u)
		}
		return
	}
	add(agentDefaultServer)
}

// ensureAgentClientID returns this instance's stable agent client id, generating
// and persisting it on first use.
func ensureAgentClientID() string {
	cfg := tunnel_server.GetConfig()
	if cfg.AgentClientID == "" {
		cfg.AgentClientID = randomClientID()
		_ = tunnel_server.UpdateConfig(cfg)
	}
	return cfg.AgentClientID
}

func agentIdentity() (hostname, osUser string) {
	hostname, _ = os.Hostname()
	if u, err := user.Current(); err == nil {
		osUser = u.Username
	}
	return hostname, osUser
}

// agentLoop keeps a single remote server's assignments reconciled into tunnels.
func agentLoop(baseURL, clientID string) {
	running := map[string]string{} // domain -> signature of the running tunnel
	keyOf := func(domain string) string { return "agent:" + baseURL + ":" + domain }

	for {
		// Register / heartbeat (also confirms the server is reachable).
		if _, err := agentRegister(baseURL, clientID); err != nil {
			log.Printf("tunnel agent: register %s: %v (retry)", baseURL, err)
			time.Sleep(agentPollInterval)
			continue
		}

		assignments, err := agentFetchAssignments(baseURL, clientID)
		if err != nil {
			log.Printf("tunnel agent: assignments %s: %v", baseURL, err)
			time.Sleep(agentPollInterval)
			continue
		}

		desired := map[string]string{}
		for _, a := range assignments {
			if !a.Enabled || !a.AutoConnect || strings.TrimSpace(a.Domain) == "" {
				continue
			}
			cfg, ok := assignmentToConfig(baseURL, clientID, a)
			if !ok {
				continue
			}
			sig := assignmentSignature(a)
			desired[a.Domain] = sig
			if running[a.Domain] != sig || !tunnelRunning(keyOf(a.Domain)) {
				log.Printf("tunnel agent: opening %s -> %s", a.Domain, baseURL)
				runTunnelClientKeyed(keyOf(a.Domain), cfg)
				running[a.Domain] = sig
			}
		}

		for d := range running {
			if _, ok := desired[d]; !ok {
				log.Printf("tunnel agent: closing %s (unassigned)", d)
				stopTunnelByKey(keyOf(d))
				delete(running, d)
			}
		}

		time.Sleep(agentPollInterval)
	}
}

// assignmentToConfig builds a client.Config for an assignment. The daemon token
// is minted fresh on every reconnect via the public register endpoint.
func assignmentToConfig(baseURL, clientID string, a *tunnel_server.TunnelAgentAssignment) (client.Config, bool) {
	localHTTP := joinHostPortIf(a.LocalHttpIp, a.LocalHttpPort)
	localTCP := joinHostPortIf(a.LocalTcpIp, a.LocalTcpPort)
	localUDP := joinHostPortIf(a.LocalUdpIp, a.LocalUdpPort)
	if localHTTP == "" && localTCP == "" && localUDP == "" {
		return client.Config{}, false
	}
	serverDaemon := daemonAddrForBaseURL(baseURL)
	if serverDaemon == "" {
		return client.Config{}, false
	}
	serverHost, _, _ := net.SplitHostPort(serverDaemon)
	// Agent tunnels usually traverse NAT/firewall; force an aggressive keepalive
	// (<= agentMaxKeepalive) so the idle return path is never dropped.
	keepalive := time.Duration(a.KeepaliveIntervalSeconds) * time.Second
	if keepalive <= 0 || keepalive > agentMaxKeepalive {
		keepalive = agentMaxKeepalive
	}
	return client.Config{
		ServerAddr: serverDaemon,
		TokenProvider: func() (string, error) {
			return agentRegister(baseURL, clientID)
		},
		Domain:            a.Domain,
		LocalHttpAddr:     localHTTP,
		LocalTCPAddr:      localTCP,
		LocalUDPAddr:      localUDP,
		SourceBindIP:      strings.TrimSpace(a.SourceBindIp),
		HostRewrite:       strings.TrimSpace(a.HostRewrite),
		KeepaliveInterval: keepalive,
		// TLS so the connection looks like HTTPS and traverses firewalls/DPI.
		UseTLS:        true,
		TLSServerName: serverHost,
	}, true
}

func joinHostPortIf(ip string, port int) string {
	ip = strings.TrimSpace(ip)
	if ip == "" || port <= 0 {
		return ""
	}
	return net.JoinHostPort(ip, strconv.Itoa(port))
}

func assignmentSignature(a *tunnel_server.TunnelAgentAssignment) string {
	return fmt.Sprintf("%s|%s:%d|%s:%d|%s:%d|%s|%s|%d",
		a.Domain,
		a.LocalHttpIp, a.LocalHttpPort,
		a.LocalTcpIp, a.LocalTcpPort,
		a.LocalUdpIp, a.LocalUdpPort,
		a.SourceBindIp, a.HostRewrite, a.KeepaliveIntervalSeconds)
}

// agentRegister calls POST /tunnel/agent/register (public) and returns a fresh
// daemon token. Used both for heartbeat and as the per-reconnect TokenProvider.
func agentRegister(baseURL, clientID string) (string, error) {
	hostname, osUser := agentIdentity()
	out, status, err := agentHTTP(
		"POST",
		strings.TrimSuffix(baseURL, "/")+"/api/v1/tunnel/agent/register",
		clientID,
		map[string]interface{}{
			"client_id": clientID,
			"hostname":  hostname,
			"os_user":   osUser,
			"version":   agentVersion,
		},
	)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("register status %d", status)
	}
	data, _ := out["data"].(map[string]interface{})
	token, _ := data["daemon_token"].(string)
	if token == "" {
		return "", fmt.Errorf("no daemon_token in response")
	}
	return token, nil
}

// agentFetchAssignments calls GET /tunnel/agent/assignments with the agent id.
func agentFetchAssignments(baseURL, clientID string) ([]*tunnel_server.TunnelAgentAssignment, error) {
	out, status, err := agentHTTP(
		"GET",
		strings.TrimSuffix(baseURL, "/")+"/api/v1/tunnel/agent/assignments",
		clientID, nil,
	)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("assignments status %d", status)
	}
	raw, _ := json.Marshal(out["data"])
	var list []*tunnel_server.TunnelAgentAssignment
	_ = json.Unmarshal(raw, &list)
	return list, nil
}

func agentHTTP(method, url, clientID string, body interface{}) (map[string]interface{}, int, error) {
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if clientID != "" {
		req.Header.Set("X-Agent-Id", clientID)
	}
	resp, err := agentHTTPClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]interface{}
	_ = json.Unmarshal(raw, &out)
	return out, resp.StatusCode, nil
}

func randomClientID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("client-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
