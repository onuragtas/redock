package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"redock/app/models"
	"redock/pkg/middleware"
	"redock/pkg/repository"
	"redock/pkg/utils"
	"redock/platform/database"
	"redock/platform/jwtsecrets"
	"redock/platform/memory"
	"redock/tunnel_server"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

// The 2026-11-21 intrusion: a self-registered tunnel account's token was accepted by the panel, and most
// server-management routes and the /ws terminal checked nothing but the signature. These tests pin that
// only panel tokens of existing users pass JWTProtected, and only admins pass AdminOnly and /ws.

type fixture struct {
	app   *fiber.App
	admin uint
	user  uint
}

func setup(t *testing.T) fixture {
	t.Helper()
	db, err := memory.NewDatabase(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatalf("NewDatabase: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := memory.Register[*models.User](db, "users"); err != nil {
		t.Fatalf("register users: %v", err)
	}
	jwtsecrets.Ensure(nil) // random secret for the test process
	database.SetGlobalDB(db)

	admin := &models.User{Email: "admin@test", UserRole: repository.AdminRoleName, UserStatus: 1}
	user := &models.User{Email: "user@test", UserRole: repository.UserRoleName, UserStatus: 1}
	for _, u := range []*models.User{admin, user} {
		if err := memory.Create[*models.User](db, "users", u); err != nil {
			t.Fatalf("create user: %v", err)
		}
	}

	ok := func(c *fiber.Ctx) error { return c.SendStatus(http.StatusOK) }
	app := fiber.New()
	api := app.Group("/api/v1", middleware.JWTProtected())
	api.Get("/auth/me", ok)
	sys := app.Group("/api/v1/system", middleware.JWTProtected(), middleware.AdminOnly())
	sys.Get("/info", ok)
	app.Get("/ws/:id?", middleware.WebSocketAccessToken(), ok)
	return fixture{app: app, admin: admin.ID, user: user.ID}
}

func panelToken(t *testing.T, id uint) string {
	t.Helper()
	tok, err := utils.GenerateNewTokens(int(id), nil)
	if err != nil {
		t.Fatalf("GenerateNewTokens: %v", err)
	}
	return tok.Access
}

// signed with the panel key but not issued by the panel — what a tunnel token used to be.
func foreignToken(t *testing.T, iss string, id uint) string {
	t.Helper()
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss": iss, "id": strconv.Itoa(int(id)), "exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString(utils.GetJWTSecretKey())
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return s
}

func status(t *testing.T, app *fiber.App, path, token string, inQuery bool) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if inQuery {
		req = httptest.NewRequest(http.MethodGet, path+"?token="+token, nil)
	} else if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("request %s: %v", path, err)
	}
	return res.StatusCode
}

func TestPanelAccess(t *testing.T) {
	f := setup(t)
	tunnelTok, err := tunnel_server.GenerateTunnelToken(f.admin) // same id as the panel admin
	if err != nil {
		t.Fatalf("GenerateTunnelToken: %v", err)
	}

	cases := []struct {
		name               string
		token              string
		me, system, socket int
	}{
		{"admin", panelToken(t, f.admin), 200, 200, 200},
		{"user", panelToken(t, f.user), 200, 403, 403},
		{"tunnel token", tunnelTok, 401, 401, 401},
		{"panel-key token, iss tunnel", foreignToken(t, "tunnel", f.admin), 401, 401, 401},
		{"panel-key token, no iss", foreignToken(t, "", f.admin), 401, 401, 401},
		{"deleted user", panelToken(t, 9999), 401, 401, 403},
		{"no token", "", 401, 401, 401},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := status(t, f.app, "/api/v1/auth/me", c.token, false); got != c.me {
				t.Errorf("/auth/me = %d, want %d", got, c.me)
			}
			if got := status(t, f.app, "/api/v1/system/info", c.token, false); got != c.system {
				t.Errorf("/system/info = %d, want %d", got, c.system)
			}
			if got := status(t, f.app, "/ws/", c.token, true); got != c.socket {
				t.Errorf("/ws = %d, want %d", got, c.socket)
			}
		})
	}
}

func TestTunnelTokensAreTheirOwnKind(t *testing.T) {
	f := setup(t)
	tok, err := tunnel_server.GenerateTunnelToken(7)
	if err != nil {
		t.Fatalf("GenerateTunnelToken: %v", err)
	}
	if id, err := tunnel_server.ValidateTunnelToken(tok); err != nil || id != 7 {
		t.Fatalf("ValidateTunnelToken(tunnel token) = %d, %v; want 7, nil", id, err)
	}
	if _, err := tunnel_server.ValidateTunnelToken(panelToken(t, f.admin)); err == nil {
		t.Error("a panel token validated as a tunnel token")
	}
	if _, err := utils.VerifyAccessTokenString(tok); err == nil {
		t.Error("a tunnel token verified as a panel token")
	}
}
