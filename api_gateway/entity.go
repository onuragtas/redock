package api_gateway

import (
	"redock/platform/memory"
	"time"
)

// ApiGatewayConfigEntity tek satır: tüm gateway konfigü JSON olarak.
const tableApiGatewayConfig = "api_gateway_config"

type ApiGatewayConfigEntity struct {
	memory.BaseEntity
	ConfigJSON string `json:"config_json"`
}

// ApiGatewayBlockEntity engellenen bir IP (block list).
const tableApiGatewayBlocks = "api_gateway_blocks"

type ApiGatewayBlockEntity struct {
	memory.BaseEntity
	IP           string    `json:"ip"`
	Manual       bool      `json:"manual"`
	BlockedAt    time.Time `json:"blocked_at"`
	BlockedUntil time.Time `json:"blocked_until"`
	Reason       string    `json:"reason"`
}
