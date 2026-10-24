package turn_server

import (
	"crypto/hmac"
	cryptorand "crypto/rand"
	"crypto/sha1" //nolint:gosec // TURN REST API mandates HMAC-SHA1
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Credentials is a short-lived TURN username/password pair.
type Credentials struct {
	Username  string    `json:"username"`
	Password  string    `json:"password"`
	ExpiresAt time.Time `json:"expires_at"`
}

// sign computes the TURN REST password for username.
func sign(secret, username string) string {
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write([]byte(username))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// GenerateCredentials issues credentials for app (and optional user) valid
// for ttl. Platforms can do the same themselves with the app secret.
func GenerateCredentials(app, secret, user string, ttl time.Duration) Credentials {
	exp := time.Now().Add(ttl)
	username := strconv.FormatInt(exp.Unix(), 10) + ":" + app
	if user != "" {
		username += ":" + user
	}
	return Credentials{Username: username, Password: sign(secret, username), ExpiresAt: exp}
}

// parsedUsername is a TURN REST username split into its parts.
type parsedUsername struct {
	expiry time.Time
	app    string
	user   string
}

// parseUsername accepts "<expiry>:<app>[:<user>]". The user part may itself
// contain colons (it is the remainder).
func parseUsername(username string) (parsedUsername, error) {
	parts := strings.SplitN(username, ":", 3)
	if len(parts) < 2 || parts[1] == "" {
		return parsedUsername{}, fmt.Errorf("username must be <expiry>:<app>[:<user>]")
	}
	ts, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return parsedUsername{}, fmt.Errorf("bad expiry in username")
	}
	p := parsedUsername{expiry: time.Unix(ts, 0), app: parts[1]}
	if len(parts) == 3 {
		p.user = parts[2]
	}
	return p, nil
}

// randomToken returns n random bytes, URL-safe base64 encoded.
func randomToken(n int) string {
	b := make([]byte, n)
	_, _ = cryptorand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
