package rtc_server

import (
	"redock/platform/memory"
)

const TableRTCConfig = "rtc_config"

// Defaults for a fresh install.
const (
	DefaultListenAddr = "127.0.0.1:8445"
	DefaultUDPMin     = 40000
	DefaultUDPMax     = 40199
	DefaultTURN       = "auto" // Galène's built-in TURN on UDP/TCP 1194
)

// RTCConfigEntity is the persisted configuration of the embedded Galène SFU.
// There is a single row.
type RTCConfigEntity struct {
	memory.BaseEntity
	Enabled bool `json:"enabled"`

	// ListenAddr is Galène's HTTP listener. It serves plain HTTP and is meant
	// to sit behind the API gateway (TLS); browsers only allow WebRTC on a
	// secure origin, so exposing it directly is only useful on localhost.
	ListenAddr string `json:"listen_addr"`

	// UDPMin/UDPMax bound the media ports (0/0 = any ephemeral port). The
	// range must be reachable from clients (firewall / port forward).
	UDPMin uint16 `json:"udp_min"`
	UDPMax uint16 `json:"udp_max"`

	// TURN selects the relay for Galène: "redock" (Redock's TURN server, with
	// its security controls), "auto" (Galène's built-in TURN on port 1194),
	// an explicit "host:port" for the built-in one, or "" for none.
	TURN string `json:"turn"`

	// PublicHost is the domain the SFU is reached at (e.g. rtc.example.com).
	// When set, a gateway route is created for it (and Let's Encrypt covers it).
	PublicHost string `json:"public_host"`

	// AdminPassword authenticates Redock to Galène's administrative API.
	// Generated once; never sent to the browser.
	AdminPassword string `json:"admin_password"`

	// RetentionDays deletes recordings older than this (0 = keep forever).
	RetentionDays int `json:"retention_days"`

	// TestRoomPassword protects the built-in "redock-test" room.
	TestRoomPassword string `json:"test_room_password"`
}
