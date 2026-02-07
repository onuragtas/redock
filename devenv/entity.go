package devenv

import "redock/platform/memory"

// DevEnvEntity memory DB'de saklanan dev environment kaydı.
// API ve docker_manager tarafında docker_manager.DevEnv (DTO) kullanılır.
type DevEnvEntity struct {
	memory.BaseEntity
	Username   string `json:"username"`
	Password   string `json:"password"`
	Port       int    `json:"port"`
	RedockPort int    `json:"redockPort"`
}
