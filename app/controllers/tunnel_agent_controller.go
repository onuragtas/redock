package controllers

import (
	"strconv"
	"strings"
	"time"

	"redock/tunnel_server"

	"github.com/gofiber/fiber/v2"
)

// agentOnlineWindow: an agent seen within this window is considered "online".
const agentOnlineWindow = 90 * time.Second

// agentFromClientID resolves the agent from the X-Agent-Id header (keyless identity).
func agentFromClientID(c *fiber.Ctx) *tunnel_server.TunnelAgent {
	clientID := strings.TrimSpace(c.Get("X-Agent-Id"))
	if clientID == "" {
		return nil
	}
	return tunnel_server.FindAgentByClientID(clientID)
}

// ---------------------------------------------------------------------------
// Agent-facing endpoints (called by remote redock clients) — PUBLIC, keyless.
// A client self-registers with a stable client_id + OS hostname/user; the server
// returns a fresh daemon token. No login or pre-shared key is required.
// ---------------------------------------------------------------------------

// TunnelAgentRegister handles POST /tunnel/agent/register (public, no auth).
// Body: { client_id, hostname, os_user, version }. Upserts the agent and returns
// a fresh daemon token. Called on first boot and on every reconnect (token
// refresh), so it beats the 24h token expiry without any stored secret.
func TunnelAgentRegister(c *fiber.Ctx) error {
	var body struct {
		ClientID string `json:"client_id"`
		Hostname string `json:"hostname"`
		OSUser   string `json:"os_user"`
		Version  string `json:"version"`
	}
	_ = c.BodyParser(&body)
	if strings.TrimSpace(body.ClientID) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "client_id required"})
	}
	agent, err := tunnel_server.RegisterAgent(
		strings.TrimSpace(body.ClientID),
		strings.TrimSpace(body.Hostname),
		strings.TrimSpace(body.OSUser),
		strings.TrimSpace(body.Version),
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	// Admin-scoped daemon token: lets the agent bind admin-created domains
	// (UserID 0) — exactly the ones the admin assigns — and nothing user-owned.
	token, err := tunnel_server.GenerateTunnelToken(0)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"data": fiber.Map{
			"agent_id":     agent.ID,
			"daemon_token": token,
		},
	})
}

// TunnelAgentAssignments handles GET /tunnel/agent/assignments (auth: X-Agent-Id).
// Returns the agent's enabled assignments so the client can reconcile its tunnels.
func TunnelAgentAssignments(c *fiber.Ctx) error {
	agent := agentFromClientID(c)
	if agent == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": true, "msg": "valid X-Agent-Id required"})
	}
	tunnel_server.TouchAgent(agent)
	out := make([]*tunnel_server.TunnelAgentAssignment, 0)
	for _, a := range tunnel_server.AssignmentsByAgentID(agent.ID) {
		if a.Enabled {
			out = append(out, a)
		}
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"error": false, "data": out})
}

// ---------------------------------------------------------------------------
// Admin-facing endpoints (manage agents and their assignments)
// ---------------------------------------------------------------------------

func agentToMap(a *tunnel_server.TunnelAgent) fiber.Map {
	return fiber.Map{
		"id":           a.ID,
		"client_id":    a.ClientID,
		"hostname":     a.Hostname,
		"os_user":      a.OSUser,
		"label":        a.Label(),
		"version":      a.Version,
		"last_seen_at": a.LastSeenAt,
		"online":       time.Since(a.LastSeenAt) <= agentOnlineWindow,
		"created_at":   a.CreatedAt,
	}
}

// TunnelListAgents handles GET /tunnel/agents.
func TunnelListAgents(c *fiber.Ctx) error {
	if _, _, ok := getTunnelServerAuth(c); !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": true, "msg": "auth required"})
	}
	agents := tunnel_server.AllAgents()
	data := make([]fiber.Map, 0, len(agents))
	for _, a := range agents {
		data = append(data, agentToMap(a))
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"error": false, "data": data})
}

// TunnelDeleteAgent handles DELETE /tunnel/agents/:id.
func TunnelDeleteAgent(c *fiber.Ctx) error {
	if _, _, ok := getTunnelServerAuth(c); !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": true, "msg": "auth required"})
	}
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "invalid id"})
	}
	// Tear down every auto-generated domain bound to this agent (gateway route +
	// Cloudflare DNS + domain record) before removing the agent + assignments.
	for _, a := range tunnel_server.AssignmentsByAgentID(uint(id)) {
		if a.Domain != "" {
			deleteManagedTunnelDomainByFull(a.Domain)
		}
	}
	if err := tunnel_server.DeleteAgentByID(uint(id)); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"error": false, "msg": "deleted"})
}

// TunnelListAgentAssignments handles GET /tunnel/agents/:id/assignments.
func TunnelListAgentAssignments(c *fiber.Ctx) error {
	if _, _, ok := getTunnelServerAuth(c); !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": true, "msg": "auth required"})
	}
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "invalid id"})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"error": false, "data": tunnel_server.AssignmentsByAgentID(uint(id))})
}

// agentAssignmentBody is the create/update payload for an assignment. The domain
// is NOT supplied by the admin — it is auto-generated (random subdomain) when the
// assignment is created.
type agentAssignmentBody struct {
	LocalHttpIp              string `json:"local_http_ip"`
	LocalHttpPort            int    `json:"local_http_port"`
	LocalTcpIp               string `json:"local_tcp_ip"`
	LocalTcpPort             int    `json:"local_tcp_port"`
	LocalUdpIp               string `json:"local_udp_ip"`
	LocalUdpPort             int    `json:"local_udp_port"`
	SourceBindIp             string `json:"source_bind_ip"`
	HostRewrite              string `json:"host_rewrite"`
	KeepaliveIntervalSeconds int    `json:"keepalive_interval_seconds"`
	AutoConnect              *bool  `json:"auto_connect"`
	Enabled                  *bool  `json:"enabled"`
}

// protocolForAssignment derives the tunnel protocol from which local targets are
// set, as a "+"-joined combination (e.g. "http+tcp"). All three → "all".
func protocolForAssignment(b *agentAssignmentBody) string {
	parts := make([]string, 0, 3)
	if b.LocalHttpPort > 0 {
		parts = append(parts, "http")
	}
	if b.LocalTcpPort > 0 {
		parts = append(parts, "tcp")
	}
	if b.LocalUdpPort > 0 {
		parts = append(parts, "udp")
	}
	if len(parts) == 0 || len(parts) == 3 {
		return "all"
	}
	return strings.Join(parts, "+")
}

func applyAssignmentBody(a *tunnel_server.TunnelAgentAssignment, b *agentAssignmentBody) {
	a.LocalHttpIp = strings.TrimSpace(b.LocalHttpIp)
	a.LocalHttpPort = b.LocalHttpPort
	a.LocalTcpIp = strings.TrimSpace(b.LocalTcpIp)
	a.LocalTcpPort = b.LocalTcpPort
	a.LocalUdpIp = strings.TrimSpace(b.LocalUdpIp)
	a.LocalUdpPort = b.LocalUdpPort
	a.SourceBindIp = strings.TrimSpace(b.SourceBindIp)
	a.HostRewrite = strings.TrimSpace(b.HostRewrite)
	a.KeepaliveIntervalSeconds = b.KeepaliveIntervalSeconds
	if b.AutoConnect != nil {
		a.AutoConnect = *b.AutoConnect
	}
	if b.Enabled != nil {
		a.Enabled = *b.Enabled
	}
}

// TunnelCreateAgentAssignment handles POST /tunnel/agents/:id/assignments.
func TunnelCreateAgentAssignment(c *fiber.Ctx) error {
	if _, _, ok := getTunnelServerAuth(c); !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": true, "msg": "auth required"})
	}
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "invalid id"})
	}
	if _, err := tunnel_server.FindAgentByID(uint(id)); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": true, "msg": "agent not found"})
	}
	var body agentAssignmentBody
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	if body.LocalHttpPort <= 0 && body.LocalTcpPort <= 0 && body.LocalUdpPort <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "at least one local target (HTTP/TCP/UDP ip+port) required"})
	}
	// Auto-generate a random domain (admin-owned) with full gateway + DNS setup.
	d, err := createManagedTunnelDomain(0, protocolForAssignment(&body))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	a := &tunnel_server.TunnelAgentAssignment{AgentID: uint(id), Domain: d.FullDomain, Enabled: true}
	applyAssignmentBody(a, &body)
	if err := tunnel_server.CreateAssignment(a); err != nil {
		deleteManagedTunnelDomainByFull(d.FullDomain) // rollback the domain we just created
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"error": false, "data": a})
}

// TunnelUpdateAgentAssignment handles PUT /tunnel/agents/:id/assignments/:asgId.
func TunnelUpdateAgentAssignment(c *fiber.Ctx) error {
	if _, _, ok := getTunnelServerAuth(c); !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": true, "msg": "auth required"})
	}
	asgID, err := strconv.ParseUint(c.Params("asgId"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "invalid assignment id"})
	}
	a, err := tunnel_server.FindAssignmentByID(uint(asgID))
	if err != nil || a == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": true, "msg": "assignment not found"})
	}
	var body agentAssignmentBody
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	applyAssignmentBody(a, &body)
	if err := tunnel_server.UpdateAssignment(a); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	// If now disabled / auto-connect off, drop the live tunnel immediately (the
	// daemon also rejects re-binds, so it stays down without relying on the poll).
	if a.Domain != "" && (!a.Enabled || !a.AutoConnect) {
		tunnel_server.DisconnectDomain(a.Domain)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"error": false, "data": a})
}

// TunnelDeleteAgentAssignment handles DELETE /tunnel/agents/:id/assignments/:asgId.
func TunnelDeleteAgentAssignment(c *fiber.Ctx) error {
	if _, _, ok := getTunnelServerAuth(c); !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": true, "msg": "auth required"})
	}
	asgID, err := strconv.ParseUint(c.Params("asgId"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "invalid assignment id"})
	}
	// Tear down the auto-generated domain (gateway + DNS) along with the assignment,
	// and drop the live tunnel immediately.
	if a, err := tunnel_server.FindAssignmentByID(uint(asgID)); err == nil && a != nil && a.Domain != "" {
		tunnel_server.DisconnectDomain(a.Domain)
		deleteManagedTunnelDomainByFull(a.Domain)
	}
	if err := tunnel_server.DeleteAssignmentByID(uint(asgID)); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"error": false, "msg": "deleted"})
}
