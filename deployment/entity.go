package deployment

import (
	"redock/platform/memory"
	"time"
)

// DeploymentSettingsEntity global deployment ayarları (tek satır).
type DeploymentSettingsEntity struct {
	memory.BaseEntity
	Username  string `json:"username"`
	Token     string `json:"token"`
	CheckTime int    `json:"check_time"`
}

// DeploymentProjectEntity tek bir deployment projesi (API ve Run'da da kullanılır).
type DeploymentProjectEntity struct {
	memory.BaseEntity
	Url          string    `json:"url"`
	Path         string    `json:"path"`
	Branch       string    `json:"branch"`
	Check        string    `json:"check"`
	Script       string    `json:"script"`
	Username     string    `json:"username,omitempty"`
	Token        string    `json:"token,omitempty"`
	LastDeployed time.Time `json:"last_deployed"`
	LastChecked  time.Time `json:"last_checked"`
	Enabled      bool      `json:"enabled"`
}
