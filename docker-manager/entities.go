package docker_manager

import "redock/platform/memory"

// ServiceSettingsEntity memory DB'de tek satır (container name prefix + overrides).
type ServiceSettingsEntity struct {
	memory.BaseEntity
	ContainerNamePrefix string                     `json:"container_name_prefix"`
	Overrides           map[string]*ServiceOverride `json:"overrides"`
}

// StarredVHostEntity yıldızlı vhost path (her biri bir satır).
type StarredVHostEntity struct {
	memory.BaseEntity
	Path string `json:"path"`
}
