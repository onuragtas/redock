package jwtsecrets

import (
	"crypto/rand"
	"encoding/hex"
	"sync"

	"redock/platform/memory"
)

var (
	secretKey   string
	refreshSalt string
	once        sync.Once
)

// Ensure loads secret and refresh salt from memory DB, or generates and persists them once.
// Must be called at startup with the same db used in registerEntities (e.g. from init.go).
func Ensure(db *memory.Database) {
	once.Do(func() {
		if db != nil {
			list := memory.FindAll[*JWTSecretsEntity](db, TableName)
			if len(list) > 0 && list[0].SecretKey != "" && list[0].RefreshSalt != "" {
				secretKey = list[0].SecretKey
				refreshSalt = list[0].RefreshSalt
				return
			}
		}
		// Generate new secrets
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			panic("jwtsecrets: failed to generate secret: " + err.Error())
		}
		secretKey = hex.EncodeToString(b)
		b = make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			panic("jwtsecrets: failed to generate refresh salt: " + err.Error())
		}
		refreshSalt = hex.EncodeToString(b)
		if db != nil {
			_ = memory.Create(db, TableName, &JWTSecretsEntity{
				SecretKey:   secretKey,
				RefreshSalt: refreshSalt,
			})
		}
	})
}

// GetJWTSecretKey returns the JWT secret. Ensure(db) must have been called at startup.
func GetJWTSecretKey() []byte {
	if secretKey == "" {
		panic("jwtsecrets: Ensure(db) must be called at startup before using JWT")
	}
	return []byte(secretKey)
}

// GetRefreshSalt returns the refresh token salt. Ensure(db) must have been called at startup.
func GetRefreshSalt() []byte {
	if refreshSalt == "" {
		panic("jwtsecrets: Ensure(db) must be called at startup before using JWT")
	}
	return []byte(refreshSalt)
}
