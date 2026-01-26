package main

import (
	"log"
	"os"
	"path/filepath"
	"redock/api_gateway"
	"redock/deployment"
	"redock/devenv"
	"redock/dns_server"
	localproxy "redock/local_proxy"
	"redock/php_debug_adapter"
	"redock/platform/database"
	"redock/platform/memory"
	"redock/saved_commands"
	"redock/tunnel_proxy"
	"redock/vpn_server"
	"time"

	dockermanager "redock/docker-manager"
)

type Process struct {
	Name string
	Func func()
}

var devEnv bool
var globalDB *memory.Database

func initialize() {
	checkSelfUpdate()

	go func() {
		for range time.Tick(time.Minute * 2) {
			checkSelfUpdate()
		}
	}()

	if len(os.Args) > 1 && os.Args[1] == "--devenv" {
		devEnv = true
	}

	log.Println("initialize....")
	dockerEnvironmentManager := dockermanager.GetDockerManager()

	// Data directory
	dataDir := filepath.Join(dockerEnvironmentManager.GetWorkDir(), "data")

	// Auto-migrate from SQLite if needed
	if err := database.AutoMigrate(dataDir); err != nil {
		log.Fatalf("Failed to auto-migrate: %v", err)
	}

	// Initialize generic in-memory database
	db, err := database.InitMemoryDB(dataDir)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	globalDB = db

	// Register all entity types
	if err := registerEntities(db); err != nil {
		log.Fatalf("Failed to register entities: %v", err)
	}

	log.Println("✅ Generic in-memory database initialized")

	go dockerEnvironmentManager.UpdateDocker()

	dockerEnvironmentManager.Init()
	if !devEnv {
		go dockerEnvironmentManager.CheckLocalIpAndRegenerate()
	}
	devenv.Init(dockerEnvironmentManager)
	tunnel_proxy.Init(dockerEnvironmentManager)
	localproxy.Init(dockerEnvironmentManager)
	php_debug_adapter.Init(dockerEnvironmentManager)
	saved_commands.Init(dockerEnvironmentManager)
	deployment.Init(dockerEnvironmentManager)
	api_gateway.Init(dockerEnvironmentManager)
	dns_server.Init(dockerEnvironmentManager)
	vpn_server.Init()
	go deployment.GetDeployment().Run()
	localproxy.GetLocalProxyManager().StartAll()
	api_gateway.GetGateway().StartAll()
}

// registerEntities registers all entity types with the database
func registerEntities(db *memory.Database) error {
	// DNS entities
	entities := []struct {
		name  string
		register func() error
	}{
		{"dns_config", func() error { return memory.Register[*dns_server.DNSConfig](db, "dns_config") }},
		{"dns_blocklists", func() error { return memory.Register[*dns_server.DNSBlocklist](db, "dns_blocklists") }},
		{"dns_custom_filters", func() error { return memory.Register[*dns_server.DNSCustomFilter](db, "dns_custom_filters") }},
		{"dns_client_settings", func() error { return memory.Register[*dns_server.DNSClientSettings](db, "dns_client_settings") }},
		{"dns_client_rules", func() error { return memory.Register[*dns_server.DNSClientDomainRule](db, "dns_client_rules") }},
		{"dns_rewrites", func() error { return memory.Register[*dns_server.DNSRewrite](db, "dns_rewrites") }},
		
		// VPN entities
		{"vpn_servers", func() error { return memory.Register[*vpn_server.VPNServer](db, "vpn_servers") }},
		{"vpn_users", func() error { return memory.Register[*vpn_server.VPNUser](db, "vpn_users") }},
		{"vpn_groups", func() error { return memory.Register[*vpn_server.VPNUserGroup](db, "vpn_groups") }},
		{"vpn_security_rules", func() error { return memory.Register[*vpn_server.VPNSecurityRule](db, "vpn_security_rules") }},
		{"vpn_connections", func() error { return memory.Register[*vpn_server.VPNConnection](db, "vpn_connections") }},
		{"vpn_connection_logs", func() error { return memory.Register[*vpn_server.VPNConnectionLog](db, "vpn_connection_logs") }},
		{"vpn_bandwidth_stats", func() error { return memory.Register[*vpn_server.VPNBandwidthStat](db, "vpn_bandwidth_stats") }},
		
		// Other entities
		{"saved_commands", func() error { return memory.Register[*database.SavedCommand](db, "saved_commands") }},
	}

	for _, entity := range entities {
		if err := entity.register(); err != nil {
			return err
		}
	}

	return nil
}
