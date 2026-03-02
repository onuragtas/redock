package utils

import (
	"fmt"
	"strconv"
	"time"

	"redock/platform/jwtsecrets"

	"github.com/golang-jwt/jwt/v5"
)

// GetJWTSecretKey returns the JWT secret from memory DB (persisted in jwt_secrets table).
func GetJWTSecretKey() []byte {
	return jwtsecrets.GetJWTSecretKey()
}

// Tokens struct to describe tokens object.
type Tokens struct {
	Access  string
	Refresh string
}

// GenerateNewTokens func for generate a new Access & Refresh tokens.
func GenerateNewTokens(iId int, credentials []string) (*Tokens, error) {
	id := strconv.Itoa(iId)
	// Generate JWT Access token.
	accessToken, err := generateNewAccessToken(id, credentials)
	if err != nil {
		return nil, err
	}

	// Generate JWT Refresh token (user_id + exp ile imzalı; renew sadece buna bakar).
	refreshToken, err := generateNewRefreshToken(id)
	if err != nil {
		return nil, err
	}

	return &Tokens{
		Access:  accessToken,
		Refresh: refreshToken,
	}, nil
}

func generateNewAccessToken(id string, credentials []string) (string, error) {
	const accessTokenExpire = 60 * time.Minute // 1 saat

	claims := jwt.MapClaims{}
	claims["iss"] = "redock"
	claims["id"] = id
	claims["exp"] = time.Now().Add(accessTokenExpire).Unix()
	for _, credential := range credentials {
		claims[credential] = true
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	t, err := token.SignedString(jwtsecrets.GetJWTSecretKey())
	if err != nil {
		return "", err
	}
	return t, nil
}

func generateNewRefreshToken(userID string) (string, error) {
	const refreshTokenExpire = 24 * time.Hour // 1 gün

	claims := jwt.MapClaims{}
	claims["id"] = userID
	claims["exp"] = time.Now().Add(refreshTokenExpire).Unix()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtsecrets.GetRefreshSalt())
}

// ParseRefreshToken verifies refresh token JWT (signature + exp), returns user ID.
// Önemli olan refresh token'ın expire olmaması ve kullanıcıya bağlı olması; renew sadece buna bakar.
func ParseRefreshToken(refreshToken string) (userID int, err error) {
	token, err := jwt.Parse(refreshToken, func(t *jwt.Token) (interface{}, error) {
		return jwtsecrets.GetRefreshSalt(), nil
	})
	if err != nil {
		return 0, err
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return 0, fmt.Errorf("invalid refresh token")
	}
	id, ok := claims["id"].(string)
	if !ok || id == "" {
		return 0, fmt.Errorf("invalid refresh token claims")
	}
	var uid int
	uid, err = strconv.Atoi(id)
	if err != nil {
		return 0, err
	}
	return uid, nil
}
