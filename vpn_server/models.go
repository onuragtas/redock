package vpn_server

import (
	"redock/platform/memory"
	"time"
)

// VPNServer represents a WireGuard VPN server
type VPNServer struct {
	memory.SoftDeleteEntity
	Name                string  `json:"name"`
	Interface           string  `json:"interface"` // wg0, wg1, etc.
	PublicKey           string  `json:"public_key"`
	PrivateKey          string  `json:"private_key"` // persisted to disk; strip before API response
	ListenPort          int     `json:"listen_port"`
	Address             string  `json:"address"` // 10.0.0.1/24
	Endpoint            string  `json:"endpoint"` // server.example.com:51820
	DNS                 string  `json:"dns"`
	AllowedIPs          string  `json:"allowed_ips"`
	MTU                 int     `json:"mtu"`
	PersistentKeepalive int     `json:"persistent_keepalive"`
	Enabled             bool    `json:"enabled"`
	Description         string  `json:"description,omitempty"`
}

// VPNUser represents a VPN user/client
type VPNUser struct {
	memory.SoftDeleteEntity
	ServerID           uint       `json:"server_id"`
	Username           string     `json:"username"`
	Email              string     `json:"email,omitempty"`
	FullName           string     `json:"full_name,omitempty"`
	PublicKey          string     `json:"public_key"`
	PrivateKey         string     `json:"private_key"` // persisted to disk; strip before API response
	Address            string     `json:"address"` // 10.0.0.2/32
	AllowedIPs         string     `json:"allowed_ips"`
	DNS                string     `json:"dns,omitempty"` // Custom DNS (optional)
	Enabled            bool       `json:"enabled"`
	Quota              int64      `json:"quota"` // bytes (0 = unlimited)
	UsedQuota          int64      `json:"used_quota"`
	ExpiresAt          *time.Time `json:"expires_at,omitempty"`
	LastConnectedAt    *time.Time `json:"last_connected_at,omitempty"`
	LastDisconnectedAt *time.Time `json:"last_disconnected_at,omitempty"`
	TotalConnections   int        `json:"total_connections"`
	TotalDuration      int64      `json:"total_duration"` // seconds
	TotalBytesReceived int64      `json:"total_bytes_received"`
	TotalBytesSent     int64      `json:"total_bytes_sent"`
	Notes              string     `json:"notes,omitempty"`
}

// VPNConnection represents an active or historical connection
type VPNConnection struct {
	memory.BaseEntity
	UserID         uint       `json:"user_id"`
	ServerID       uint       `json:"server_id"`
	PublicKey      string     `json:"public_key"`
	RemoteIP       string     `json:"remote_ip,omitempty"`
	RemotePort     int        `json:"remote_port,omitempty"`
	BytesReceived  int64      `json:"bytes_received"`
	BytesSent      int64      `json:"bytes_sent"`
	LastHandshake  *time.Time `json:"last_handshake,omitempty"`
	ConnectedAt    time.Time  `json:"connected_at"`
	DisconnectedAt *time.Time `json:"disconnected_at,omitempty"`
	Duration       int64      `json:"duration"` // seconds
	Status         string     `json:"status"`   // connected, disconnected
}

// VPNConnectionLog represents connection events
type VPNConnectionLog struct {
	memory.BaseEntity
	UserID        uint   `json:"user_id"`
	ServerID      uint   `json:"server_id"`
	Event         string `json:"event"` // connect, disconnect, handshake
	RemoteIP      string `json:"remote_ip,omitempty"`
	BytesReceived int64  `json:"bytes_received"`
	BytesSent     int64  `json:"bytes_sent"`
	Duration      int64  `json:"duration"`
	Error         string `json:"error,omitempty"`
}

// VPNBandwidthStat represents hourly/daily bandwidth statistics
type VPNBandwidthStat struct {
	memory.BaseEntity
	UserID          uint      `json:"user_id"`
	ServerID        uint      `json:"server_id"`
	Date            time.Time `json:"date"`
	Hour            int       `json:"hour"` // 0-23
	BytesReceived   int64     `json:"bytes_received"`
	BytesSent       int64     `json:"bytes_sent"`
	ConnectionCount int       `json:"connection_count"`
	AvgDuration     int64     `json:"avg_duration"` // seconds
}

// VPNUserGroup represents a group of VPN users
type VPNUserGroup struct {
	memory.SoftDeleteEntity
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	AllowedIPs  string `json:"allowed_ips,omitempty"`
	DNS         string `json:"dns,omitempty"`
	Quota       int64  `json:"quota"`      // bytes (0 = unlimited)
	RateLimit   int    `json:"rate_limit"` // Mbps (0 = unlimited)
	Enabled     bool   `json:"enabled"`
}

// VPNUserGroupMember represents user-group membership
type VPNUserGroupMember struct {
	memory.BaseEntity
	UserID  uint `json:"user_id"`
	GroupID uint `json:"group_id"`
}

// VPNSecurityRule represents security rules (whitelist/blacklist)
type VPNSecurityRule struct {
	memory.SoftDeleteEntity
	ServerID    uint   `json:"server_id"`
	Type        string `json:"type"` // whitelist, blacklist, geo-block
	IP          string `json:"ip,omitempty"`
	CIDR        string `json:"cidr,omitempty"`
	Country     string `json:"country,omitempty"`
	Action      string `json:"action"` // allow, block
	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled"`
}
