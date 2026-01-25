package vpn_server

import (
	"time"

	"gorm.io/gorm"
)

// VPNServer represents a WireGuard VPN server
type VPNServer struct {
	ID                  uint           `gorm:"primarykey" json:"id"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	DeletedAt           gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	Name                string         `gorm:"not null" json:"name"`
	Interface           string         `gorm:"default:wg0" json:"interface"` // wg0, wg1, etc.
	PublicKey           string         `gorm:"not null;uniqueIndex" json:"public_key"`
	PrivateKey          string         `gorm:"not null" json:"-"` // encrypted, not returned in JSON
	ListenPort          int            `gorm:"default:51820" json:"listen_port"`
	Address             string         `gorm:"not null" json:"address"` // 10.0.0.1/24
	Endpoint            string         `json:"endpoint"`                  // server.example.com:51820
	DNS                 string         `gorm:"default:1.1.1.1,8.8.8.8" json:"dns"`
	AllowedIPs          string         `gorm:"default:0.0.0.0/0" json:"allowed_ips"`
	MTU                 int            `gorm:"default:1420" json:"mtu"`
	PersistentKeepalive int            `gorm:"default:25" json:"persistent_keepalive"`
	Enabled             bool           `gorm:"default:true" json:"enabled"`
	Description         string         `gorm:"type:text" json:"description,omitempty"`
}

// VPNUser represents a VPN user/client
type VPNUser struct {
	ID                  uint           `gorm:"primarykey" json:"id"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	DeletedAt           gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	ServerID            uint           `gorm:"not null;index" json:"server_id"`
	Username            string         `gorm:"not null;index" json:"username"`
	Email               string         `json:"email,omitempty"`
	FullName            string         `json:"full_name,omitempty"`
	PublicKey           string         `gorm:"not null;uniqueIndex" json:"public_key"`
	PrivateKey          string         `gorm:"not null" json:"-"` // encrypted, not returned in JSON
	Address             string         `gorm:"not null" json:"address"` // 10.0.0.2/32
	AllowedIPs          string         `gorm:"default:0.0.0.0/0" json:"allowed_ips"`
	DNS                 string         `json:"dns,omitempty"` // Custom DNS (optional)
	Enabled             bool           `gorm:"default:true" json:"enabled"`
	Quota               int64         `gorm:"default:0" json:"quota"` // bytes (0 = unlimited)
	UsedQuota           int64         `gorm:"default:0" json:"used_quota"`
	ExpiresAt           *time.Time    `json:"expires_at,omitempty"`
	LastConnectedAt     *time.Time    `json:"last_connected_at,omitempty"`
	LastDisconnectedAt  *time.Time    `json:"last_disconnected_at,omitempty"`
	TotalConnections    int           `gorm:"default:0" json:"total_connections"`
	TotalDuration       int64         `gorm:"default:0" json:"total_duration"` // seconds
	TotalBytesReceived  int64         `gorm:"default:0" json:"total_bytes_received"`
	TotalBytesSent      int64         `gorm:"default:0" json:"total_bytes_sent"`
	Notes               string         `gorm:"type:text" json:"notes,omitempty"`
}

// VPNConnection represents an active or historical connection
type VPNConnection struct {
	ID              uint      `gorm:"primarykey" json:"id"`
	CreatedAt       time.Time `gorm:"index" json:"created_at"`
	UserID          uint      `gorm:"not null;index" json:"user_id"`
	ServerID        uint      `gorm:"not null;index" json:"server_id"`
	PublicKey       string    `gorm:"not null" json:"public_key"`
	RemoteIP        string    `json:"remote_ip,omitempty"`
	RemotePort      int       `json:"remote_port,omitempty"`
	BytesReceived   int64     `gorm:"default:0" json:"bytes_received"`
	BytesSent       int64     `gorm:"default:0" json:"bytes_sent"`
	LastHandshake   *time.Time `json:"last_handshake,omitempty"`
	ConnectedAt     time.Time `json:"connected_at"`
	DisconnectedAt  *time.Time `json:"disconnected_at,omitempty"`
	Duration        int64     `gorm:"default:0" json:"duration"` // seconds
	Status          string    `gorm:"default:disconnected" json:"status"` // connected, disconnected
}

// VPNConnectionLog represents connection events
type VPNConnectionLog struct {
	ID              uint      `gorm:"primarykey" json:"id"`
	CreatedAt       time.Time `gorm:"index" json:"created_at"`
	UserID          uint      `gorm:"not null;index" json:"user_id"`
	ServerID        uint      `gorm:"not null;index" json:"server_id"`
	Event           string    `gorm:"not null" json:"event"` // connect, disconnect, handshake
	RemoteIP        string    `json:"remote_ip,omitempty"`
	BytesReceived   int64     `gorm:"default:0" json:"bytes_received"`
	BytesSent       int64     `gorm:"default:0" json:"bytes_sent"`
	Duration        int64     `gorm:"default:0" json:"duration"`
	Error           string    `gorm:"type:text" json:"error,omitempty"`
}

// VPNBandwidthStat represents hourly/daily bandwidth statistics
type VPNBandwidthStat struct {
	ID              uint      `gorm:"primarykey" json:"id"`
	CreatedAt       time.Time `gorm:"index" json:"created_at"`
	UserID          uint      `gorm:"not null;index" json:"user_id"`
	ServerID        uint      `gorm:"not null;index" json:"server_id"`
	Date            time.Time `gorm:"index" json:"date"`
	Hour            int       `gorm:"index" json:"hour"` // 0-23
	BytesReceived   int64     `gorm:"default:0" json:"bytes_received"`
	BytesSent       int64     `gorm:"default:0" json:"bytes_sent"`
	ConnectionCount int       `gorm:"default:0" json:"connection_count"`
	AvgDuration     int64     `gorm:"default:0" json:"avg_duration"` // seconds
}

// VPNUserGroup represents a group of VPN users
type VPNUserGroup struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	Name        string         `gorm:"not null;uniqueIndex" json:"name"`
	Description string         `gorm:"type:text" json:"description,omitempty"`
	AllowedIPs  string         `json:"allowed_ips,omitempty"`
	DNS         string         `json:"dns,omitempty"`
	Quota       int64         `gorm:"default:0" json:"quota"` // bytes (0 = unlimited)
	RateLimit   int           `gorm:"default:0" json:"rate_limit"` // Mbps (0 = unlimited)
	Enabled     bool           `gorm:"default:true" json:"enabled"`
}

// VPNUserGroupMember represents user-group membership
type VPNUserGroupMember struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UserID    uint      `gorm:"not null;index" json:"user_id"`
	GroupID   uint      `gorm:"not null;index" json:"group_id"`
}

// VPNSecurityRule represents security rules (whitelist/blacklist)
type VPNSecurityRule struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	ServerID    uint           `gorm:"not null;index" json:"server_id"`
	Type        string         `gorm:"not null" json:"type"` // whitelist, blacklist, geo-block
	IP          string         `json:"ip,omitempty"`
	CIDR        string         `json:"cidr,omitempty"`
	Country     string         `json:"country,omitempty"`
	Action      string         `gorm:"not null" json:"action"` // allow, block
	Description string         `gorm:"type:text" json:"description,omitempty"`
	Enabled     bool           `gorm:"default:true" json:"enabled"`
}
