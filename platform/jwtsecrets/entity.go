package jwtsecrets

import "redock/platform/memory"

const TableName = "jwt_secrets"

// JWTSecretsEntity stores JWT signing secret and refresh salt in memory DB (single row).
type JWTSecretsEntity struct {
	memory.BaseEntity
	SecretKey   string `json:"secret_key"`
	RefreshSalt string `json:"refresh_salt"`
}
