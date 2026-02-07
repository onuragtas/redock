package main

import (
	"log"
	"os"
	"path/filepath"
	"redock/api_gateway"
	"redock/app/cache_models"
	"redock/app/models"
	"redock/cloudflare"
	"redock/deployment"
	"redock/devenv"
	"redock/dns_server"
	"redock/email_server"
	localproxy "redock/local_proxy"
	"redock/php_debug_adapter"
	"redock/pkg/network"
	"redock/platform/database"
	"redock/platform/memory"
	"redock/platform/migrations"
	"redock/saved_commands"
	"redock/tunnel_server"
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

	// Run memory DB migrations (one-time data migrations)
	if err := database.RunMemoryMigrations(db, dataDir, migrations.MemoryMigrations()); err != nil {
		log.Fatalf("Failed to run memory migrations: %v", err)
	}

	go dockerEnvironmentManager.UpdateDocker()

	dockerEnvironmentManager.Init()
	if !devEnv {
		go dockerEnvironmentManager.CheckLocalIpAndRegenerate()
	}
	devenv.Init(dockerEnvironmentManager)
	tunnel_server.Init(dockerEnvironmentManager)
	localproxy.Init(dockerEnvironmentManager)
	php_debug_adapter.Init(dockerEnvironmentManager)
	saved_commands.Init(dockerEnvironmentManager)
	deployment.Init(dockerEnvironmentManager)
	api_gateway.Init(dockerEnvironmentManager)
	dns_server.Init(dockerEnvironmentManager)
	vpn_server.Init()
	cloudflare.Init()
	email_server.Init(dockerEnvironmentManager)
	go deployment.GetDeployment().Run()
	localproxy.GetLocalProxyManager().StartAll()
	api_gateway.GetGateway().StartAll()
	network.ApplyPersistedAliases(globalDB)
}

// registerEntities registers all entity types with the database
func registerEntities(db *memory.Database) error {
	// DNS entities
	entities := []struct {
		name     string
		register func() error
	}{
		{"dns_config", func() error { return memory.Register[*dns_server.DNSConfig](db, "dns_config") }},
		{"dns_blocklists", func() error { return memory.Register[*dns_server.DNSBlocklist](db, "dns_blocklists") }},
		{"dns_custom_filters", func() error { return memory.Register[*dns_server.DNSCustomFilter](db, "dns_custom_filters") }},
		{"dns_client_settings", func() error { return memory.Register[*dns_server.DNSClientSettings](db, "dns_client_settings") }},
		{"dns_client_rules", func() error { return memory.Register[*dns_server.DNSClientDomainRule](db, "dns_client_rules") }},
		{"dns_rewrites", func() error { return memory.Register[*dns_server.DNSRewrite](db, "dns_rewrites") }},
		{"dns_query_logs", func() error { return memory.Register[*dns_server.DNSQueryLog](db, "dns_query_logs") }},

		// VPN entities
		{"vpn_servers", func() error { return memory.Register[*vpn_server.VPNServer](db, "vpn_servers") }},
		{"vpn_users", func() error { return memory.Register[*vpn_server.VPNUser](db, "vpn_users") }},
		{"vpn_groups", func() error { return memory.Register[*vpn_server.VPNUserGroup](db, "vpn_groups") }},
		{"vpn_security_rules", func() error { return memory.Register[*vpn_server.VPNSecurityRule](db, "vpn_security_rules") }},
		{"vpn_connections", func() error { return memory.Register[*vpn_server.VPNConnection](db, "vpn_connections") }},
		{"vpn_connection_logs", func() error { return memory.Register[*vpn_server.VPNConnectionLog](db, "vpn_connection_logs") }},
		{"vpn_bandwidth_stats", func() error { return memory.Register[*vpn_server.VPNBandwidthStat](db, "vpn_bandwidth_stats") }},

		// Cloudflare entities
		{"cloudflare_accounts", func() error { return memory.Register[*cloudflare.CloudflareAccount](db, "cloudflare_accounts") }},
		{"cloudflare_zones", func() error { return memory.Register[*cloudflare.CloudflareZone](db, "cloudflare_zones") }},
		{"cloudflare_dns_records", func() error { return memory.Register[*cloudflare.CloudflareDNSRecord](db, "cloudflare_dns_records") }},
		{"cloudflare_firewall_rules", func() error {
			return memory.Register[*cloudflare.CloudflareFirewallRule](db, "cloudflare_firewall_rules")
		}},
		{"cloudflare_page_rules", func() error { return memory.Register[*cloudflare.CloudflarePageRule](db, "cloudflare_page_rules") }},
		{"cloudflare_zone_settings", func() error {
			return memory.Register[*cloudflare.CloudflareZoneSettings](db, "cloudflare_zone_settings")
		}},
		{"cloudflare_events", func() error { return memory.Register[*cloudflare.CloudflareEvent](db, "cloudflare_events") }},

		// Email entities
		{"email_domains", func() error { return memory.Register[*email_server.EmailDomain](db, "email_domains") }},
		{"email_mailboxes", func() error { return memory.Register[*email_server.EmailMailbox](db, "email_mailboxes") }},
		{"email_aliases", func() error { return memory.Register[*email_server.EmailAlias](db, "email_aliases") }},
		{"email_folders", func() error { return memory.Register[*email_server.EmailFolder](db, "email_folders") }},
		{"email_messages", func() error { return memory.Register[*email_server.Email](db, "email_messages") }},
		{"email_attachments", func() error { return memory.Register[*email_server.EmailAttachment](db, "email_attachments") }},
		{"email_filters", func() error { return memory.Register[*email_server.EmailFilter](db, "email_filters") }},
		{"email_logs", func() error { return memory.Register[*email_server.EmailLog](db, "email_logs") }},
		{"email_server_configs", func() error { return memory.Register[*email_server.EmailServerConfig](db, "email_server_configs") }},

		// Other entities
		{"users", func() error { return memory.Register[*models.User](db, "users") }},
		{"saved_commands", func() error { return memory.Register[*database.SavedCommand](db, "saved_commands") }},
		{"release_cache", func() error { return memory.Register[*cache_models.ReleaseCache](db, "release_cache") }},
		{"local_proxy_items", func() error { return memory.Register[*localproxy.LocalProxyItem](db, "local_proxy_items") }},
		{"dev_envs", func() error { return memory.Register[*devenv.DevEnvEntity](db, "dev_envs") }},
		{"deployment_settings", func() error { return memory.Register[*deployment.DeploymentSettingsEntity](db, "deployment_settings") }},
		{"deployment_projects", func() error { return memory.Register[*deployment.DeploymentProjectEntity](db, "deployment_projects") }},
		// Tunnel server
		{"tunnel_server_config", func() error { return memory.Register[*tunnel_server.TunnelServerConfig](db, "tunnel_server_config") }},
		{"tunnel_domains", func() error { return memory.Register[*tunnel_server.TunnelDomain](db, "tunnel_domains") }},
		{"tunnel_users", func() error { return memory.Register[*tunnel_server.TunnelUser](db, "tunnel_users") }},
		{"tunnel_server_credentials", func() error {
			return memory.Register[*tunnel_server.TunnelServerCredential](db, "tunnel_server_credentials")
		}},
		{"tunnel_servers", func() error { return memory.Register[*tunnel_server.TunnelServer](db, "tunnel_servers") }},
		{"network_ip_aliases", func() error { return memory.Register[*network.PersistedIPAlias](db, network.TableIPAliases) }},
	}

	for _, entity := range entities {
		if err := entity.register(); err != nil {
			return err
		}
	}

	return nil
}
