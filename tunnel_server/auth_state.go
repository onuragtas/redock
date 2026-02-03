package tunnel_server

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// AuthState holds user_id, server_id and client redirect URL for OAuth callback (TTL 10 min).
type AuthState struct {
	UserID         uint
	ServerID       uint
	ClientRedirect string // frontend URL to redirect after saving token (e.g. https://app.example.com/#/tunnel-proxy-client)
	Created        time.Time
}

const stateTTL = 10 * time.Minute

var (
	authStateMu   sync.RWMutex
	authStateMap  = make(map[string]*AuthState)
	authStateOnce sync.Once
)

func cleanupAuthState() {
	authStateOnce.Do(func() {
		go func() {
			for range time.Tick(2 * time.Minute) {
				authStateMu.Lock()
				now := time.Now()
				for k, v := range authStateMap {
					if now.Sub(v.Created) > stateTTL {
						delete(authStateMap, k)
					}
				}
				authStateMu.Unlock()
			}
		}()
	})
}

// PutAuthState stores state → AuthState; returns state string. clientRedirect is the frontend URL to redirect after callback.
func PutAuthState(userID, serverID uint, clientRedirect string) string {
	cleanupAuthState()
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	state := hex.EncodeToString(b)
	authStateMu.Lock()
	authStateMap[state] = &AuthState{UserID: userID, ServerID: serverID, ClientRedirect: clientRedirect, Created: time.Now()}
	authStateMu.Unlock()
	return state
}

// GetAuthState returns AuthState and removes it (one-time use). Returns nil if not found or expired.
func GetAuthState(state string) *AuthState {
	authStateMu.Lock()
	defer authStateMu.Unlock()
	v, ok := authStateMap[state]
	if !ok || v == nil || time.Since(v.Created) > stateTTL {
		return nil
	}
	delete(authStateMap, state)
	return v
}
