package utils

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

// TokenMetadata struct to describe metadata in JWT.
type TokenMetadata struct {
	UserID      int
	Credentials map[string]bool
	Expires     int64
}

// getClaimUserID extracts user ID from JWT claims (handles string, float64, int).
func getClaimUserID(claims jwt.MapClaims) (int, bool) {
	if id, ok := claims["id"]; ok && id != nil {
		switch v := id.(type) {
		case float64:
			return int(v), true
		case int:
			return v, true
		case string:
			n, err := strconv.Atoi(v)
			if err == nil {
				return n, true
			}
		}
	}
	return 0, false
}

// ExtractTokenMetadata func to extract metadata from JWT (token must be valid and not expired).
func ExtractTokenMetadata(c *fiber.Ctx) (*TokenMetadata, error) {
	token, err := verifyToken(c)
	if err != nil {
		return nil, err
	}

	// Setting and checking token and credentials.
	claims, ok := token.Claims.(jwt.MapClaims)
	if ok && token.Valid {
		userID, okID := getClaimUserID(claims)
		if !okID {
			return nil, err
		}

		// Expires time.
		exp, _ := claims["exp"].(float64)
		expires := int64(exp)

		return &TokenMetadata{
			UserID:      userID,
			Credentials: map[string]bool{},
			Expires:     expires,
		}, nil
	}

	return nil, err
}

// ExtractTokenMetadataIgnoringExpiry extracts metadata from JWT without validating expiry.
// Used by token renew endpoint so expired access token can still provide user ID.
func ExtractTokenMetadataIgnoringExpiry(c *fiber.Ctx) (*TokenMetadata, error) {
	token, err := verifyToken(c)
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, err
	}

	userID, okID := getClaimUserID(claims)
	if !okID {
		return nil, err
	}

	exp, _ := claims["exp"].(float64)
	expires := int64(exp)

	return &TokenMetadata{
		UserID:      userID,
		Credentials: map[string]bool{},
		Expires:     expires,
	}, nil
}

func extractToken(c *fiber.Ctx) string {
	bearToken := c.Get("Authorization")

	// Normally Authorization HTTP header.
	onlyToken := strings.Split(bearToken, " ")
	if len(onlyToken) == 2 {
		return onlyToken[1]
	}

	return ""
}

func verifyToken(c *fiber.Ctx) (*jwt.Token, error) {
	tokenString := extractToken(c)

	token, err := jwt.Parse(tokenString, jwtKeyFunc)
	if err != nil {
		return nil, err
	}

	return token, nil
}

func jwtKeyFunc(token *jwt.Token) (interface{}, error) {
	return GetJWTSecretKey(), nil
}

// VerifyAccessTokenString parses and validates an access token string (e.g. from query param).
// Returns the JWT and nil if valid and not expired; otherwise error.
func VerifyAccessTokenString(tokenString string) (*jwt.Token, error) {
	if tokenString == "" {
		return nil, fmt.Errorf("empty token")
	}
	token, err := jwt.Parse(tokenString, jwtKeyFunc)
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return token, nil
}
