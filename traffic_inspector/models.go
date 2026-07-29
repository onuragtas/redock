package traffic_inspector

import "redock/platform/memory"

// CATableName is the memory DB table used to persist the root CA so it
// survives restarts (same single-row pattern as platform/jwtsecrets).
const CATableName = "vpn_ca"

// CAEntity stores the root CA certificate/key used to sign per-host leaf
// certificates for TLS/QUIC interception.
type CAEntity struct {
	memory.BaseEntity
	CertPEM string `json:"cert_pem"`
	KeyPEM  string `json:"key_pem"`
}

// FlowEvent describes one chunk of decrypted (or already-plaintext) traffic
// tapped from an inspected connection, pushed to dashboard clients over WS
// and kept in the in-memory ring buffer for the /flows REST endpoint.
type FlowEvent struct {
	Timestamp int64  `json:"timestamp"`
	UserID    uint   `json:"user_id"`
	FlowID    string `json:"flow_id"`
	Protocol  string `json:"protocol"` // tcp-tls, tcp-plain, quic
	SNI       string `json:"sni,omitempty"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Direction string `json:"direction"` // client->server, server->client
	Data      []byte `json:"data"`      // base64-encoded automatically by encoding/json
	Closed    bool   `json:"closed,omitempty"`
	Error     string `json:"error,omitempty"`
	// Status classifies a connection that produced no decrypted content so it
	// can still be surfaced in the dashboard: "" for normal data-carrying
	// flows, "attempt" for a QUIC handshake we saw but cannot confirm
	// succeeded, "blocked" for a handshake the client refused (e.g. cert
	// pinning), "failed" for an upstream/lookup error on our side.
	Status string `json:"status,omitempty"`
}
