package turn_server

import (
	"time"

	"redock/platform/memory"
)

const (
	TableTURNConfig = "turn_config"
	TableTURNApps   = "turn_apps"
)

// Defaults for a fresh install. The relay range is kept clear of Galène's
// media range (40000-40199) and the OS ephemeral range on most systems.
const (
	DefaultPort          = 3478
	DefaultTLSPort       = 5349
	DefaultRelayMin      = 49160
	DefaultRelayMax      = 49359
	DefaultRealm         = "redock"
	DefaultSessionsApp   = 200
	DefaultSessionsIP    = 10
	DefaultBitrateKbps   = 8000 // per session; 0 = unlimited
	DefaultCredentialTTL = 24 * 60 * 60
	DefaultAuthFailLimit = 10 // failed auths per source IP per minute before a block
	DefaultBlockMinutes  = 15
)

// TURNConfigEntity is the single-row configuration of the TURN/STUN server.
type TURNConfigEntity struct {
	memory.BaseEntity
	Enabled bool `json:"enabled"`

	Port    int `json:"port"`     // UDP + TCP (TURN/STUN)
	TLSPort int `json:"tls_port"` // TURNS; 0 disables. Uses the gateway's certificate.

	// PublicHost is what clients put in their ICE server URLs (domain or IP).
	// For TURNS it must be a domain covered by the gateway certificate.
	PublicHost string `json:"public_host"`
	// RelayIP is the address advertised for relayed media — the public IP
	// clients can reach (behind NAT: the router's public address).
	RelayIP  string `json:"relay_ip"`
	RelayMin uint16 `json:"relay_min"`
	RelayMax uint16 `json:"relay_max"`
	Realm    string `json:"realm"`

	// Relay target policy. Private, loopback, link-local, CGNAT and
	// multicast destinations are refused unless AllowPrivate is set — an open
	// relay into the LAN is the classic TURN abuse (SSRF).
	AllowPrivate bool     `json:"allow_private"`
	AllowCIDRs   []string `json:"allow_cidrs,omitempty"` // exceptions to the private ban / deny list
	DenyCIDRs    []string `json:"deny_cidrs,omitempty"`  // extra destinations to refuse

	MaxSessionsPerIP  int `json:"max_sessions_per_ip"`
	MaxBitrateKbps    int `json:"max_bitrate_kbps"`    // per session, each direction
	MaxSessionMinutes int `json:"max_session_minutes"` // 0 = no cap (credential TTL still applies)
	AuthFailLimit     int `json:"auth_fail_limit"`     // per source IP per minute; 0 disables blocking
	BlockMinutes      int `json:"block_minutes"`
}

// TURNAppEntity is one consumer of the TURN server (a platform, Galène, ...).
// It signs short-lived credentials with Secret:
//
//	username = "<unix expiry>:<app>[:<user>]"
//	password = base64(HMAC-SHA1(secret, username))
type TURNAppEntity struct {
	memory.BaseEntity
	Name          string    `json:"name"` // the <app> part of usernames
	Secret        string    `json:"secret"`
	Enabled       bool      `json:"enabled"`
	MaxSessions   int       `json:"max_sessions"`   // concurrent; 0 = server default
	CredentialTTL int       `json:"credential_ttl"` // seconds, for credentials Redock issues
	System        bool      `json:"system"`         // managed by Redock (e.g. "galene"); not deletable
	Note          string    `json:"note,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}
