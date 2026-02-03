package tunnel_server

import (
	"log"

	dockermanager "redock/docker-manager"
)

// Init initializes the tunnel server module.
// Config and entity tables must be registered in registerEntities before using GetConfig/repo.
// If tunnel server is enabled, starts the daemon listener on TunnelListenAddr and the unused-domain cleanup loop.
func Init(dm *dockermanager.DockerEnvironmentManager) {
	_ = dm
	cfg := GetConfig()
	log.Printf("Tunnel server: config loaded (enabled=%v)", cfg.Enabled)
	if cfg.Enabled {
		StartDaemon()
		go RunCleanupLoop()
	}
}
