package controllers

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"redock/email_server"
	"redock/pkg/utils"
	"redock/tunnel_server"
	"redock/tunnel_server/client"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

// activeTunnels holds running tunnel clients by domain (key = body.Domain).
var activeTunnels sync.Map

// requireTunnelServerBearer returns tunnel user ID if Authorization: Bearer <token> is valid (tunnel_server OAuth2).
func requireTunnelServerBearer(c *fiber.Ctx) (uint, bool) {
	userID, isAdmin, ok := getTunnelServerAuth(c)
	if !ok || isAdmin {
		return 0, false
	}
	return userID, true
}

// getTunnelServerAuth returns (userID, isAdmin, ok). Önce tunnel_server OAuth2 token dene; olmazsa Redock JWT dene (admin).
func getTunnelServerAuth(c *fiber.Ctx) (userID uint, isAdmin bool, ok bool) {
	auth := c.Get("Authorization")
	if auth == "" {
		return 0, false, false
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return 0, false, false
	}
	token := strings.TrimSpace(auth[len(prefix):])
	if token == "" {
		return 0, false, false
	}
	// 1) Tunnel server OAuth2 token
	uid, err := tunnel_server.ValidateTunnelToken(token)
	if err == nil {
		return uid, false, true
	}
	// 2) Redock JWT = admin (Redock'a giriş yapan kullanıcı sunucuyu yönetir)
	meta, err := utils.ExtractTokenMetadata(c)
	if err == nil && meta != nil {
		return 0, true, true
	}
	return 0, false, false
}

// UpdateDockerImages method to create a new user.
// @Description Create a new user.
// @Summary create a new user
// @Tags User
// @Accept json
// @Produce json
// @Param email body string true "Email"
// @Param password body string true "Password"
// @Param user_role body string true "User role"
// @Success 200 {object} models.User
// @Router /v1/docker/env [get]
// CheckUser validates Bearer token (tunnel_server OAuth2) and returns login status.
func CheckUser(c *fiber.Ctx) error {
	userID, _, ok := getTunnelServerAuth(c)
	if !ok {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"error": false,
			"msg":   nil,
			"data":  fiber.Map{"login": false},
		})
	}
	_ = userID
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  fiber.Map{"login": true},
	})
}

// TunnelLogin method to create a new user.
// @Description Create a new user.
// @Summary create a new user
// @Tags User
// @Accept json
// @Produce json
// @Param email body string true "Email"
// @Param password body string true "Password"
// @Param user_role body string true "User role"
// @Success 200 {object} models.User
// @Router /v1/docker/env [get]
func TunnelLogin(c *fiber.Ctx) error {

	type Login struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	model := &Login{}
	if err := c.BodyParser(model); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Geçersiz istek: " + err.Error(),
			"data":  nil,
		})
	}

	// Boş kimlik bilgileri → 400 (401 sadece yanlış kullanıcı/şifre için)
	if model.Username == "" || model.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Kullanıcı adı ve şifre gerekli",
			"data":  nil,
		})
	}

	if !tunnel_server.GetConfig().Enabled {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Tünel sunucusu etkin değil",
			"data":  nil,
		})
	}
	token, err := tunnel_server.LoginTunnelUser(model.Username, model.Password)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "Geçersiz kullanıcı adı veya şifre. Hesabınız yoksa önce kayıt olun.",
			"data":  nil,
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  fiber.Map{"token": token},
	})
}

// TunnelLogin method to create a new user.
// @Description Create a new user.
// @Summary create a new user
// @Tags User
// @Accept json
// @Produce json
// @Param email body string true "Email"
// @Param password body string true "Password"
// @Param user_role body string true "User role"
// @Success 200 {object} models.User
// @Router /v1/docker/env [get]
func TunnelRegister(c *fiber.Ctx) error {

	type Model struct {
		Email    string `json:"email"`
		Username string `json:"username"`
		Password string `json:"password"`
	}

	model := &Model{}
	if err := c.BodyParser(model); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	if !tunnel_server.GetConfig().Enabled {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true,
			"msg":   "Tünel sunucusu etkin değil",
			"data":  nil,
		})
	}
	token, err := tunnel_server.RegisterTunnelUser(model.Username, model.Password)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
			"data":  nil,
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  fiber.Map{"token": token},
	})
}

// TunnelLogin method to create a new user.
// @Description Create a new user.
// @Summary create a new user
// @Tags User
// @Accept json
// @Produce json
// @Param email body string true "Email"
// @Param password body string true "Password"
// @Param user_role body string true "User role"
// @Success 200 {object} models.User
// @Router /v1/docker/env [get]
func TunnelLogout(c *fiber.Ctx) error {
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  fiber.Map{},
	})
}

// TunnelUserInfo returns tunnel user info for Bearer token (tunnel_server OAuth2).
func TunnelUserInfo(c *fiber.Ctx) error {
	userID, isAdmin, ok := getTunnelServerAuth(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "Authorization: Bearer <token> required (tunnel token or Redock JWT)",
		})
	}
	if isAdmin {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"error": false,
			"data":  fiber.Map{"user": fiber.Map{"id": 0, "username": "admin"}},
		})
	}
	u, err := tunnel_server.FindTunnelUserByID(userID)
	if err != nil || u == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "user not found",
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"data":  fiber.Map{"user": fiber.Map{"id": u.ID, "username": u.Username}},
	})
}

// daemonAddr returns the address to connect to the local tunnel daemon (127.0.0.1:port).
func daemonAddr() string {
	cfg := tunnel_server.GetConfig()
	if cfg == nil {
		return "127.0.0.1:8443"
	}
	addr := cfg.TunnelListenAddr
	if addr == "" {
		return "127.0.0.1:8443"
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "127.0.0.1:8443"
	}
	return net.JoinHostPort("127.0.0.1", port)
}

// daemonAddrForBaseURL parses tunnel server base URL (e.g. https://tunnel.example.com) and returns host:8443 for the daemon connection.
func daemonAddrForBaseURL(baseURL string) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return ""
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	host := u.Hostname()
	if host == "" {
		return ""
	}
	return net.JoinHostPort(host, "8443")
}

// bearerTokenForDaemon returns the token to use for the daemon. If request is from Redock admin (JWT), generates a tunnel token for user 0.
func bearerTokenForDaemon(c *fiber.Ctx, isAdmin bool) (string, error) {
	if isAdmin {
		return tunnel_server.GenerateTunnelToken(0)
	}
	auth := c.Get("Authorization")
	const prefix = "Bearer "
	if strings.HasPrefix(auth, prefix) {
		return strings.TrimSpace(auth[len(prefix):]), nil
	}
	return "", fmt.Errorf("missing bearer token")
}

// TunnelStart accepts POST /tunnel/start. Body: DomainId, Domain, LocalIp, DestinationIp, LocalPort (tunnel-client format).
// Starts a tunnel client in the background that connects to the daemon and forwards TCP (and optionally UDP) to the local address.
func TunnelStart(c *fiber.Ctx) error {
	_, isAdmin, ok := getTunnelServerAuth(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "Authorization: Bearer <token> required (tunnel token or Redock JWT)",
		})
	}
	var body struct {
		DomainId      uint   `json:"DomainId"`
		Domain        string `json:"Domain"`
		LocalIp       string `json:"LocalIp"`
		DestinationIp string `json:"DestinationIp"`
		LocalPort     int    `json:"LocalPort"`
		LocalUdpIp    string `json:"LocalUdpIp"`   // optional: UDP forward target IP
		LocalUdpPort  int    `json:"LocalUdpPort"` // optional: UDP forward target port
		HostRewrite   string `json:"HostRewrite"` // optional: set route Host header override (HTTP/HTTPS); empty clears
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	if body.Domain == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "Domain required"})
	}
	token, err := bearerTokenForDaemon(c, isAdmin)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	localTCP := ""
	if body.LocalIp != "" && body.LocalPort > 0 {
		localTCP = net.JoinHostPort(body.LocalIp, strconv.Itoa(body.LocalPort))
	}
	localUDP := ""
	if body.LocalUdpIp != "" && body.LocalUdpPort > 0 {
		localUDP = net.JoinHostPort(body.LocalUdpIp, strconv.Itoa(body.LocalUdpPort))
	}
	if localTCP == "" && localUDP == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "LocalIp+LocalPort or LocalUdpIp+LocalUdpPort required"})
	}
	// Stop existing tunnel for this domain if any
	if existing, ok := activeTunnels.Load(body.Domain); ok {
		if cl, _ := existing.(*client.Client); cl != nil {
			_ = cl.Close()
		}
		activeTunnels.Delete(body.Domain)
	}
	cfg := client.Config{
		ServerAddr:   daemonAddr(),
		Token:        token,
		Domain:       body.Domain,
		LocalTCPAddr: localTCP,
		LocalUDPAddr: localUDP,
		HostRewrite:  strings.TrimSpace(body.HostRewrite),
	}
	cl, err := client.ConnectOnce(cfg)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "tunnel connect: " + err.Error(),
		})
	}
	activeTunnels.Store(body.Domain, cl)
	go func() {
		_ = cl.Run()
		activeTunnels.Delete(body.Domain)
	}()
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  []interface{}{},
	})
}

// TunnelStop accepts POST /tunnel/stop. Body: DomainId, Domain. Closes the tunnel client for that domain.
func TunnelStop(c *fiber.Ctx) error {
	if _, _, ok := getTunnelServerAuth(c); !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "Authorization: Bearer <token> required (tunnel token or Redock JWT)",
		})
	}
	var body struct {
		DomainId uint   `json:"DomainId"`
		Domain   string `json:"Domain"`
	}
	_ = c.BodyParser(&body)
	if body.Domain != "" {
		if existing, ok := activeTunnels.Load(body.Domain); ok {
			if cl, _ := existing.(*client.Client); cl != nil {
				_ = cl.Close()
			}
			activeTunnels.Delete(body.Domain)
		}
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  fiber.Map{},
	})
}

// TunnelRenew accepts POST /tunnel/renew. Body: id, domain. Domain yenileme (DNS/Cloudflare tarafında ek işlem yok; uyumluluk için kabul).
func TunnelRenew(c *fiber.Ctx) error {
	if _, _, ok := getTunnelServerAuth(c); !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "Authorization: Bearer <token> required (tunnel token or Redock JWT)",
		})
	}
	var body struct {
		ID     uint   `json:"id"`
		Domain string `json:"domain"`
	}
	_ = c.BodyParser(&body)
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
	})
}

// TunnelServerGetConfig returns tunnel server config. Requires Redock JWT (route is under JWTProtected(); admin-only if desired).
func TunnelServerGetConfig(c *fiber.Ctx) error {
	_, _, ok := getTunnelServerAuth(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "Authorization required (Redock JWT)",
		})
	}
	cfg := tunnel_server.GetConfig()
	if cfg == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "tunnel server config not loaded",
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"data":  cfg,
	})
}

// TunnelServerUpdateConfig updates tunnel server config (Redock JWT). Body: enabled, cloudflare_zone_id, domain_suffix, unused_domain_ttl_days.
func TunnelServerUpdateConfig(c *fiber.Ctx) error {
	_, _, ok := getTunnelServerAuth(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "Authorization required (Redock JWT)",
		})
	}
	var body struct {
		Enabled             *bool  `json:"enabled"`
		CloudflareZoneID    string `json:"cloudflare_zone_id"`
		ServerPublicIP      string `json:"server_public_ip"`
		DomainSuffix        string `json:"domain_suffix"`
		UnusedDomainTTLDays *int   `json:"unused_domain_ttl_days"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	cfg := tunnel_server.GetConfig()
	if cfg == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "tunnel server config not loaded",
		})
	}
	if body.Enabled != nil {
		cfg.Enabled = *body.Enabled
	}
	if body.CloudflareZoneID != "" {
		cfg.CloudflareZoneID = strings.TrimSpace(body.CloudflareZoneID)
	}
	if body.ServerPublicIP != "" {
		cfg.ServerPublicIP = strings.TrimSpace(body.ServerPublicIP)
	}
	if body.DomainSuffix != "" {
		cfg.DomainSuffix = strings.TrimSpace(body.DomainSuffix)
	}
	if body.UnusedDomainTTLDays != nil && *body.UnusedDomainTTLDays >= 0 {
		cfg.UnusedDomainTTLDays = *body.UnusedDomainTTLDays
	}
	if err := tunnel_server.UpdateConfig(cfg); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"data":  tunnel_server.GetConfig(),
	})
}

// isTunnelStarted returns true if a tunnel client is running for the given domain (keyed by full_domain or subdomain).
func isTunnelStarted(domain string) bool {
	_, ok := activeTunnels.Load(domain)
	return ok
}

// TunnelServerListDomains returns domains for the tunnel user (OAuth2 Bearer) or all domains when Redock JWT (admin).
// Each item includes "started": true/false from the in-memory tunnel client state.
func TunnelServerListDomains(c *fiber.Ctx) error {
	userID, isAdmin, ok := getTunnelServerAuth(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "Authorization: Bearer <token> required (tunnel token or Redock JWT)",
		})
	}
	var domains []*tunnel_server.TunnelDomain
	if isAdmin {
		domains = tunnel_server.AllDomains()
	} else {
		domains = tunnel_server.FindDomainsByUserID(userID)
	}
	list := make([]fiber.Map, 0, len(domains))
	for _, d := range domains {
		started := isTunnelStarted(d.FullDomain) || isTunnelStarted(d.Subdomain)
		item := fiber.Map{
			"id":          d.ID,
			"subdomain":   d.Subdomain,
			"full_domain": d.FullDomain,
			"port":        d.Port,
			"protocol":    d.Protocol,
			"created_at":  d.CreatedAt,
			"started":     started,
		}
		if d.LastUsedAt != nil {
			item["last_used_at"] = d.LastUsedAt
		}
		list = append(list, item)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"data":  list,
	})
}

// TunnelServerCreateDomain creates a domain (OAuth2 Bearer = tunnel user; Redock JWT = admin, UserID 0).
func TunnelServerCreateDomain(c *fiber.Ctx) error {
	userID, isAdmin, ok := getTunnelServerAuth(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "Authorization: Bearer <token> required (tunnel token or Redock JWT)",
		})
	}
	domainUserID := userID
	if isAdmin {
		domainUserID = 0
	}
	type Body struct {
		Domain   string `json:"domain"`   // subdomain
		Protocol string `json:"protocol"` // optional; default "all" (HTTP+HTTPS+TCP+UDP)
	}
	var body Body
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	subdomain := strings.TrimSpace(body.Domain)
	if subdomain == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "domain required",
		})
	}
	cfg := tunnel_server.GetConfig()
	if cfg == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "tunnel server config not loaded",
		})
	}
	if tunnel_server.FindDomainBySubdomain(subdomain) != nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"error": true,
			"msg":   "domain already exists",
		})
	}
	port, err := tunnel_server.NextPortForDomain()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	protocol := strings.TrimSpace(body.Protocol)
	if protocol == "" {
		protocol = "all"
	}
	fullDomain := tunnel_server.FullDomainFor(subdomain, cfg.DomainSuffix)
	d := &tunnel_server.TunnelDomain{
		UserID:     domainUserID,
		Subdomain:  subdomain,
		FullDomain: fullDomain,
		Port:       port,
		Protocol:   protocol,
	}
	if err := tunnel_server.CreateDomain(d); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// API Gateway: Route + Service (HTTP) ve gerekirse UDPRoute + Service (UDP)
	if err := tunnel_server.AddTunnelDomainToGateway(d); err != nil {
		_ = tunnel_server.DeleteDomainByID(d.ID)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "API Gateway: " + err.Error(),
		})
	}

	// Cloudflare: A record for full_domain when zone is set; public IP from tunnel config, else email config, else auto-detect (same as email)
	if cfg.CloudflareZoneID != "" {
		serverIP := cfg.ServerPublicIP
		if serverIP == "" {
			if mgr := email_server.GetManager(); mgr != nil {
				serverIP = mgr.GetConfig().IPAddress
			}
		}
		if serverIP == "" {
			serverIP = tunnel_server.DetectPublicIP()
		}
		if serverIP != "" {
			recordID, err := tunnel_server.CreateTunnelDNSRecord(cfg.CloudflareZoneID, d.FullDomain, serverIP)
			if err != nil {
				_ = tunnel_server.RemoveTunnelDomainFromGateway(d)
				_ = tunnel_server.DeleteDomainByID(d.ID)
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": true,
					"msg":   "Cloudflare: " + err.Error(),
				})
			}
			d.CloudflareRecordID = recordID
		}
	}

	if err := tunnel_server.UpdateDomain(d); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"data": fiber.Map{
			"id":          d.ID,
			"subdomain":   d.Subdomain,
			"full_domain": d.FullDomain,
			"port":        d.Port,
			"protocol":    d.Protocol,
			"created_at":  d.CreatedAt,
		},
	})
}

// TunnelServerDeleteDomain deletes a domain by ID (OAuth2 Bearer = own domain; Redock JWT = any domain).
func TunnelServerDeleteDomain(c *fiber.Ctx) error {
	userID, isAdmin, ok := getTunnelServerAuth(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "Authorization: Bearer <token> required (tunnel token or Redock JWT)",
		})
	}
	idStr := c.Params("id")
	if idStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "id required",
		})
	}
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "invalid id",
		})
	}
	d, err := tunnel_server.FindDomainByID(uint(id))
	if err != nil || d == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "domain not found",
		})
	}
	if !isAdmin && d.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": true,
			"msg":   "forbidden",
		})
	}

	cfg := tunnel_server.GetConfig()
	if cfg != nil {
		// API Gateway: Route, Service(s), UDPRoute kaldır
		_ = tunnel_server.RemoveTunnelDomainFromGateway(d)
		// Cloudflare: A record sil
		if d.CloudflareRecordID != "" && cfg.CloudflareZoneID != "" {
			_ = tunnel_server.DeleteTunnelDNSRecord(cfg.CloudflareZoneID, d.CloudflareRecordID)
		}
	}

	if err := tunnel_server.DeleteDomainByID(uint(id)); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "deleted",
	})
}

// requireRedockJWT returns Redock user ID if Authorization Bearer is valid Redock JWT.
func requireRedockJWT(c *fiber.Ctx) (uint, bool) {
	meta, err := utils.ExtractTokenMetadata(c)
	if err != nil || meta == nil {
		return 0, false
	}
	return uint(meta.UserID), true
}

// writeProxyResult ProxyResult'ı Fiber response olarak yazar.
func writeProxyResult(c *fiber.Ctx, res *tunnel_server.ProxyResult) error {
	c.Set("Content-Type", res.ContentType)
	return c.Status(res.StatusCode).Send(res.Body)
}

// proxyHandler JWT + serverID alır, tunnel_server.ProxyToExternal çağırır, sonucu yazar.
func proxyHandler(c *fiber.Ctx, serverID uint, method, path string, body []byte) error {
	userID, ok := requireRedockJWT(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "Authorization required (Redock JWT)",
		})
	}
	res, err := tunnel_server.ProxyToExternal(userID, serverID, method, path, body)
	if err != nil {
		if err == tunnel_server.ErrInvalidServerID || err == tunnel_server.ErrServerNoBaseURL {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	return writeProxyResult(c, res)
}

// TunnelProxyDomainsList: internal proxy GET /tunnel/proxy/domains?server_id=
func TunnelProxyDomainsList(c *fiber.Ctx) error {
	serverIDStr := c.Query("server_id")
	if serverIDStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "server_id required"})
	}
	serverID, err := strconv.ParseUint(serverIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "invalid server_id"})
	}
	return proxyHandler(c, uint(serverID), http.MethodGet, "/api/v1/tunnel/domains", nil)
}

// TunnelProxyDomainCreate: internal proxy POST /tunnel/domains (body: server_id, domain, protocol)
func TunnelProxyDomainCreate(c *fiber.Ctx) error {
	var body struct {
		ServerID uint   `json:"server_id"`
		Domain   string `json:"domain"`
		Protocol string `json:"protocol"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	if body.ServerID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "server_id required"})
	}
	payload := fiber.Map{"domain": body.Domain, "protocol": body.Protocol}
	if body.Protocol == "" {
		payload["protocol"] = "http"
	}
	raw, _ := json.Marshal(payload)
	return proxyHandler(c, body.ServerID, http.MethodPost, "/api/v1/tunnel/domains", raw)
}

// TunnelProxyDomainDelete: internal proxy DELETE /tunnel/proxy/domains/:id?server_id=
func TunnelProxyDomainDelete(c *fiber.Ctx) error {
	serverIDStr := c.Query("server_id")
	idStr := c.Params("id")
	if serverIDStr == "" || idStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "server_id and id required"})
	}
	serverID, err := strconv.ParseUint(serverIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "invalid server_id"})
	}
	return proxyHandler(c, uint(serverID), http.MethodDelete, "/api/v1/tunnel/domains/"+idStr, nil)
}

// TunnelProxyList: internal proxy GET /tunnel/domains; enriches "started" from locally-running proxy clients.
func TunnelProxyList(c *fiber.Ctx) error {
	serverIDStr := c.Query("server_id")
	if serverIDStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "server_id required"})
	}
	serverID, err := strconv.ParseUint(serverIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "invalid server_id"})
	}
	userID, ok := requireRedockJWT(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": true, "msg": "Authorization required (Redock JWT)"})
	}
	res, err := tunnel_server.ProxyToExternal(userID, uint(serverID), http.MethodGet, "/api/v1/tunnel/domains", nil)
	if err != nil {
		if err == tunnel_server.ErrInvalidServerID || err == tunnel_server.ErrServerNoBaseURL {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	if res.StatusCode != 200 {
		c.Set("Content-Type", res.ContentType)
		return c.Status(res.StatusCode).Send(res.Body)
	}
	// Enrich "started" from local proxy clients
	var out struct {
		Error bool          `json:"error"`
		Data  []fiber.Map   `json:"data"`
	}
	if err := json.Unmarshal(res.Body, &out); err != nil {
		c.Set("Content-Type", res.ContentType)
		return c.Status(res.StatusCode).Send(res.Body)
	}
	for _, item := range out.Data {
		fullDomain, _ := item["full_domain"].(string)
		subdomain, _ := item["subdomain"].(string)
		_, startedFull := activeTunnels.Load(proxyTunnelKey(uint(serverID), fullDomain))
		_, startedSub := activeTunnels.Load(proxyTunnelKey(uint(serverID), subdomain))
		item["started"] = startedFull || startedSub
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"error": out.Error, "data": out.Data})
}

// TunnelProxyAdd: internal proxy POST /tunnel/add (body: server_id + data)
func TunnelProxyAdd(c *fiber.Ctx) error {
	var body struct {
		ServerID uint        `json:"server_id"`
		Data     interface{} `json:"data"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	if body.ServerID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "server_id required"})
	}
	raw, _ := json.Marshal(body.Data)
	if len(raw) == 0 || string(raw) == "null" {
		raw = []byte("{}")
	}
	return proxyHandler(c, body.ServerID, http.MethodPost, "/api/v1/tunnel/add", raw)
}

// TunnelProxyDelete: internal proxy POST /tunnel/delete (body: server_id + data)
func TunnelProxyDelete(c *fiber.Ctx) error {
	var body struct {
		ServerID uint        `json:"server_id"`
		Data     interface{} `json:"data"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	if body.ServerID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "server_id required"})
	}
	raw, _ := json.Marshal(body.Data)
	if len(raw) == 0 || string(raw) == "null" {
		raw = []byte("{}")
	}
	return proxyHandler(c, body.ServerID, http.MethodPost, "/api/v1/tunnel/delete", raw)
}

// proxyTunnelKey returns the activeTunnels key for a proxy-started client.
func proxyTunnelKey(serverID uint, domain string) string {
	return fmt.Sprintf("proxy:%d:%s", serverID, domain)
}

// TunnelProxyStart: start tunnel client locally; client connects to the external tunnel server's host:8443 (daemon).
func TunnelProxyStart(c *fiber.Ctx) error {
	userID, ok := requireRedockJWT(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "Authorization required (Redock JWT)",
		})
	}
	var body struct {
		ServerID uint        `json:"server_id"`
		Data     interface{} `json:"data"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	if body.ServerID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "server_id required"})
	}
	server, err := tunnel_server.FindTunnelServerByID(body.ServerID)
	if err != nil || server == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "invalid server_id"})
	}
	baseURL := strings.TrimSpace(server.BaseURL)
	if baseURL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "server has no base_url"})
	}
	cred := tunnel_server.CredentialByBaseURLAndUser(baseURL, userID)
	if cred == nil || cred.AccessToken == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "no credential for this tunnel server; connect via OAuth2 first",
		})
	}
	// Parse data (same shape as TunnelStart)
	var data struct {
		DomainId      uint   `json:"DomainId"`
		Domain        string `json:"Domain"`
		LocalIp       string `json:"LocalIp"`
		DestinationIp string `json:"DestinationIp"`
		LocalPort     int    `json:"LocalPort"`
		LocalUdpIp    string `json:"LocalUdpIp"`
		LocalUdpPort  int    `json:"LocalUdpPort"`
		HostRewrite   string `json:"HostRewrite"`
	}
	raw, _ := json.Marshal(body.Data)
	if len(raw) == 0 || string(raw) == "null" {
		raw = []byte("{}")
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "invalid data"})
	}
	if data.Domain == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "domain required"})
	}
	localTCP := ""
	if data.LocalIp != "" && data.LocalPort > 0 {
		localTCP = net.JoinHostPort(data.LocalIp, strconv.Itoa(data.LocalPort))
	}
	localUDP := ""
	if data.LocalUdpIp != "" && data.LocalUdpPort > 0 {
		localUDP = net.JoinHostPort(data.LocalUdpIp, strconv.Itoa(data.LocalUdpPort))
	}
	if localTCP == "" && localUDP == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "LocalIp+LocalPort or LocalUdpIp+LocalUdpPort required"})
	}
	serverDaemonAddr := daemonAddrForBaseURL(baseURL)
	if serverDaemonAddr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "could not parse server base_url"})
	}
	key := proxyTunnelKey(body.ServerID, data.Domain)
	if existing, ok := activeTunnels.Load(key); ok {
		if cl, _ := existing.(*client.Client); cl != nil {
			_ = cl.Close()
		}
		activeTunnels.Delete(key)
	}
	cfg := client.Config{
		ServerAddr:   serverDaemonAddr,
		Token:        cred.AccessToken,
		Domain:       data.Domain,
		LocalTCPAddr: localTCP,
		LocalUDPAddr: localUDP,
		HostRewrite:  strings.TrimSpace(data.HostRewrite),
	}
	cl, err := client.ConnectOnce(cfg)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "tunnel connect: " + err.Error(),
		})
	}
	activeTunnels.Store(key, cl)
	go func() {
		_ = cl.Run()
		activeTunnels.Delete(key)
	}()
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  []interface{}{},
	})
}

// TunnelProxyStop: stop the locally-running tunnel client for the given server and domain.
func TunnelProxyStop(c *fiber.Ctx) error {
	if _, ok := requireRedockJWT(c); !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "Authorization required (Redock JWT)",
		})
	}
	var body struct {
		ServerID uint        `json:"server_id"`
		Data     interface{} `json:"data"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	if body.ServerID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "server_id required"})
	}
	var data struct {
		Domain string `json:"Domain"`
	}
	raw, _ := json.Marshal(body.Data)
	if len(raw) > 0 && string(raw) != "null" {
		_ = json.Unmarshal(raw, &data)
	}
	if data.Domain == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "domain required in data"})
	}
	key := proxyTunnelKey(body.ServerID, data.Domain)
	if existing, ok := activeTunnels.Load(key); ok {
		if cl, _ := existing.(*client.Client); cl != nil {
			_ = cl.Close()
		}
		activeTunnels.Delete(key)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"data":  fiber.Map{},
	})
}

// TunnelProxyRenew: internal proxy POST /tunnel/renew (body: server_id + data)
func TunnelProxyRenew(c *fiber.Ctx) error {
	var body struct {
		ServerID uint        `json:"server_id"`
		Data     interface{} `json:"data"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	if body.ServerID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "server_id required"})
	}
	raw, _ := json.Marshal(body.Data)
	if len(raw) == 0 || string(raw) == "null" {
		raw = []byte("{}")
	}
	return proxyHandler(c, body.ServerID, http.MethodPost, "/api/v1/tunnel/renew", raw)
}

// TunnelServerListServers returns the federation tunnel server list (Redock JWT).
func TunnelServerListServers(c *fiber.Ctx) error {
	if _, ok := requireRedockJWT(c); !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "Authorization required (Redock JWT)",
		})
	}
	list := tunnel_server.AllTunnelServers()
	data := make([]fiber.Map, 0, len(list))
	for _, s := range list {
		data = append(data, fiber.Map{
			"id":         s.ID,
			"name":       s.Name,
			"base_url":   s.BaseURL,
			"is_default": s.IsDefault,
			"order":      s.Order,
			"created_at": s.CreatedAt,
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"data":  data,
	})
}

// TunnelServerCreateServer adds a tunnel server to the list (Redock JWT). Body: name, base_url, is_default?, order?.
func TunnelServerCreateServer(c *fiber.Ctx) error {
	if _, ok := requireRedockJWT(c); !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "Authorization required (Redock JWT)",
		})
	}
	var body struct {
		Name      string `json:"name"`
		BaseURL   string `json:"base_url"`
		IsDefault *bool  `json:"is_default"`
		Order     *int   `json:"order"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = "Tunnel Server"
	}
	baseURL := strings.TrimSpace(body.BaseURL)
	s := &tunnel_server.TunnelServer{
		Name:      name,
		BaseURL:   baseURL,
		IsDefault: false,
		Order:     0,
	}
	if body.IsDefault != nil {
		s.IsDefault = *body.IsDefault
	}
	if body.Order != nil {
		s.Order = *body.Order
	}
	all := tunnel_server.AllTunnelServers()
	if len(all) == 0 {
		s.IsDefault = true
	}
	if err := tunnel_server.CreateTunnelServer(s); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	if s.IsDefault {
		_ = tunnel_server.SetDefaultTunnelServer(s.ID)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"data": fiber.Map{
			"id":         s.ID,
			"name":       s.Name,
			"base_url":   s.BaseURL,
			"is_default": s.IsDefault,
			"order":      s.Order,
			"created_at": s.CreatedAt,
		},
	})
}

// TunnelServerUpdateServer updates a tunnel server (Redock JWT). Body: name?, is_default?.
func TunnelServerUpdateServer(c *fiber.Ctx) error {
	if _, ok := requireRedockJWT(c); !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "Authorization required (Redock JWT)",
		})
	}
	idStr := c.Params("id")
	if idStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "id required",
		})
	}
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "invalid id",
		})
	}
	s, err := tunnel_server.FindTunnelServerByID(uint(id))
	if err != nil || s == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "server not found",
		})
	}
	var body struct {
		Name      *string `json:"name"`
		IsDefault *bool   `json:"is_default"`
		Order     *int    `json:"order"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	if body.Name != nil {
		s.Name = strings.TrimSpace(*body.Name)
		if s.Name == "" {
			s.Name = "Tunnel Server"
		}
	}
	if body.IsDefault != nil && *body.IsDefault {
		_ = tunnel_server.SetDefaultTunnelServer(s.ID)
		s.IsDefault = true
	}
	if body.Order != nil {
		s.Order = *body.Order
	}
	if err := tunnel_server.UpdateTunnelServer(s); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"data": fiber.Map{
			"id":         s.ID,
			"name":       s.Name,
			"base_url":   s.BaseURL,
			"is_default": s.IsDefault,
			"order":      s.Order,
			"created_at": s.CreatedAt,
		},
	})
}

// TunnelServerDeleteServer deletes a tunnel server from the list (Redock JWT).
func TunnelServerDeleteServer(c *fiber.Ctx) error {
	if _, ok := requireRedockJWT(c); !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "Authorization required (Redock JWT)",
		})
	}
	idStr := c.Params("id")
	if idStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "id required",
		})
	}
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "invalid id",
		})
	}
	if err := tunnel_server.DeleteTunnelServerByID(uint(id)); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "deleted",
	})
}

// TunnelCredentialList returns credentials for the current Redock user (has_token per base_url; token dönmez).
func TunnelCredentialList(c *fiber.Ctx) error {
	userID, ok := requireRedockJWT(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "Authorization required (Redock JWT)",
		})
	}
	baseURL := c.Query("base_url")
	if baseURL != "" {
		cred := tunnel_server.CredentialByBaseURLAndUser(baseURL, userID)
		if cred == nil || cred.AccessToken == "" {
			return c.Status(fiber.StatusOK).JSON(fiber.Map{
				"error": false,
				"data":  fiber.Map{"base_url": baseURL, "has_token": false},
			})
		}
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"error": false,
			"data": fiber.Map{
				"base_url":     baseURL,
				"has_token":    true,
				"access_token": cred.AccessToken,
			},
		})
	}
	all := tunnel_server.AllTunnelServers()
	type credItem struct {
		BaseURL  string `json:"base_url"`
		HasToken bool   `json:"has_token"`
	}
	out := make([]credItem, 0, len(all))
	for _, s := range all {
		cred := tunnel_server.CredentialByBaseURLAndUser(s.BaseURL, userID)
		out = append(out, credItem{
			BaseURL:  s.BaseURL,
			HasToken: cred != nil && cred.AccessToken != "",
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"data":  out,
	})
}

// TunnelAuthPrepare (JWT) creates a one-time state and returns callback_url for OAuth redirect. Body: server_id, client_redirect (frontend URL to redirect after callback).
func TunnelAuthPrepare(c *fiber.Ctx) error {
	userID, ok := requireRedockJWT(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "Authorization required (Redock JWT)",
		})
	}
	var body struct {
		ServerID       uint   `json:"server_id"`
		ClientRedirect string `json:"client_redirect"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	if body.ServerID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "server_id required"})
	}
	_, err := tunnel_server.FindTunnelServerByID(body.ServerID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": "invalid server_id"})
	}
	state := tunnel_server.PutAuthState(userID, body.ServerID, strings.TrimSpace(body.ClientRedirect))
	callbackURL := c.Protocol() + "://" + c.Hostname() + "/api/v1/tunnel/auth/callback?state=" + state
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"data":  fiber.Map{"callback_url": callbackURL, "state": state},
	})
}

// TunnelAuthCallback (no auth) receives redirect from tunnel login page: state, tunnel_token, tunnel_base_url. Saves credential and redirects to client_redirect (proxy client page).
// Token is read from raw query and decoded without converting + to space, so JWT is stored exactly as issued by the external tunnel server.
func TunnelAuthCallback(c *fiber.Ctx) error {
	state := strings.TrimSpace(c.Query("state"))
	// Raw query'den tunnel_token al; Query() + karakterini boşluğa çevirir, JWT bozulmasın diye sadece %xx decode
	tokenRaw := string(c.Context().QueryArgs().Peek("tunnel_token"))
	if tokenRaw != "" {
		if decoded, err := url.PathUnescape(tokenRaw); err == nil {
			tokenRaw = decoded
		}
	}
	token := strings.TrimSpace(tokenRaw)
	baseURL := strings.TrimSpace(c.Query("tunnel_base_url"))
	if state == "" || token == "" || baseURL == "" {
		return c.Status(fiber.StatusBadRequest).SendString("state, tunnel_token ve tunnel_base_url gerekli")
	}
	auth := tunnel_server.GetAuthState(state)
	if auth == nil {
		return c.Status(fiber.StatusBadRequest).SendString("Geçersiz veya süresi dolmuş state. Lütfen Tünel Proxy Client sayfasından tekrar Bağlan ile deneyin.")
	}
	server, err := tunnel_server.FindTunnelServerByID(auth.ServerID)
	if err != nil || server == nil {
		return c.Status(fiber.StatusBadRequest).SendString("Sunucu bulunamadı")
	}
	effectiveBaseURL := strings.TrimSpace(server.BaseURL)
	if effectiveBaseURL == "" {
		effectiveBaseURL = baseURL
	}
	cred := &tunnel_server.TunnelServerCredential{
		BaseURL:     effectiveBaseURL,
		AccessToken: token,
		UserID:      auth.UserID,
	}
	if err := tunnel_server.SaveCredential(cred); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Token kaydedilemedi")
	}
	redirectTo := auth.ClientRedirect
	if redirectTo == "" {
		redirectTo = c.Protocol() + "://" + c.Hostname() + "/#/tunnel-proxy-client?server=" + strconv.FormatUint(uint64(auth.ServerID), 10)
	}
	return c.Redirect(redirectTo, fiber.StatusFound)
}

// TunnelCredentialSave saves a tunnel credential for the current user (Redock JWT). Body: base_url, access_token, refresh_token?, expires_at?.
func TunnelCredentialSave(c *fiber.Ctx) error {
	userID, ok := requireRedockJWT(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "Authorization required (Redock JWT)",
		})
	}
	var body struct {
		BaseURL      string `json:"base_url"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresAt    string `json:"expires_at"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	baseURL := strings.TrimSpace(body.BaseURL)
	if baseURL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "base_url required",
		})
	}
	var expiresAt time.Time
	if body.ExpiresAt != "" {
		if t, err := time.Parse(time.RFC3339, body.ExpiresAt); err == nil {
			expiresAt = t
		}
	}
	cred := &tunnel_server.TunnelServerCredential{
		BaseURL:      baseURL,
		AccessToken:  strings.TrimSpace(body.AccessToken),
		RefreshToken: strings.TrimSpace(body.RefreshToken),
		ExpiresAt:    expiresAt,
		UserID:       userID,
	}
	if err := tunnel_server.SaveCredential(cred); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"error": false,
		"msg":   "saved",
	})
}
