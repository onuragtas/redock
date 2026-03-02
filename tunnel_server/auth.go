package tunnel_server

import (
	"fmt"
	"strconv"
	"time"

	"redock/pkg/utils"

	"github.com/golang-jwt/jwt/v5"
)

// getTunnelJWTSecret returns the general JWT secret (pkg/utils/jwt_generator); tunnel token imzası aynı secret ile.
func getTunnelJWTSecret() []byte {
	return utils.GetJWTSecretKey()
}

const accessTokenExpire = 24 * time.Hour

// GenerateTunnelToken generates a JWT access token for a tunnel user (ID).
func GenerateTunnelToken(tunnelUserID uint) (string, error) {
	claims := jwt.MapClaims{
		"iss": "tunnel",
		"id":  strconv.FormatUint(uint64(tunnelUserID), 10),
		"exp": time.Now().Add(accessTokenExpire).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(getTunnelJWTSecret())
}

// ValidateTunnelToken parses and validates the JWT; returns tunnel user ID.
// Used by daemon to accept tunnel connections and by API to authorize create_domain etc.
func ValidateTunnelToken(accessToken string) (tunnelUserID uint, err error) {
	token, err := jwt.Parse(accessToken, func(t *jwt.Token) (interface{}, error) {
		return getTunnelJWTSecret(), nil
	})
	if err != nil {
		return 0, fmt.Errorf("invalid token: %w", err)
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return 0, fmt.Errorf("invalid token claims")
	}
	// Redock JWT uses same secret and has "id"; reject so API can treat as admin.
	if iss, _ := claims["iss"].(string); iss == "redock" {
		return 0, fmt.Errorf("not a tunnel token")
	}
	idStr, ok := claims["id"].(string)
	if !ok || idStr == "" {
		return 0, fmt.Errorf("missing id in token")
	}
	var id uint64
	_, err = fmt.Sscanf(idStr, "%d", &id)
	if err != nil {
		return 0, fmt.Errorf("invalid id in token: %w", err)
	}
	return uint(id), nil
}

// RegisterTunnelUser creates a new tunnel user and returns an access token.
func RegisterTunnelUser(email, password string) (accessToken string, err error) {
	if email == "" || password == "" {
		return "", fmt.Errorf("email and password required")
	}
	if FindTunnelUserByEmail(email) != nil {
		return "", fmt.Errorf("email already registered")
	}
	u := &TunnelUser{
		Email:        email,
		PasswordHash: utils.GeneratePassword(password),
	}
	if err := CreateTunnelUser(u); err != nil {
		return "", fmt.Errorf("create tunnel user: %w", err)
	}
	return GenerateTunnelToken(u.ID)
}

// LoginTunnelUser verifies email/password and returns an access token.
func LoginTunnelUser(email, password string) (accessToken string, err error) {
	if email == "" || password == "" {
		return "", fmt.Errorf("email and password required")
	}
	u := FindTunnelUserByEmail(email)
	if u == nil {
		return "", fmt.Errorf("invalid credentials")
	}
	if !utils.ComparePasswords(u.PasswordHash, password) {
		return "", fmt.Errorf("invalid credentials")
	}
	return GenerateTunnelToken(u.ID)
}
