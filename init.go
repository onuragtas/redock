package main

import (
	"log"
	"os"
	"path/filepath"
	"redock/api_gateway"
	"redock/app/cache_models"
	"redock/app/controllers"
	"redock/app/models"
	"redock/cloudflare"
	"redock/deployment"
	"redock/devenv"
	"redock/dns_server"
	"redock/email_server"
	localproxy "redock/local_proxy"
	"redock/onion_proxy"
	"redock/php_debug_adapter"
	"redock/pkg/network"
	"redock/platform/database"
	"redock/platform/jwtsecrets"
	"redock/platform/memguard"
	"redock/platform/memory"
	"redock/platform/migrations"
	"redock/saved_commands"
	"redock/traffic_inspector"
	"redock/tunnel_server"
	"redock/vpn_server"
	"time"

	dockermanager "redock/docker-manager"
	"redock/docker-manager/stacks"
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

	// Cap append-only log tables and start the memory guard before any service
	// allocates: GOMEMLIMIT and OOM protection must be in place from boot.
	applyLogTableLimits(db)
	memguard.Init(db)
	registerMemoryDBReliever(db)

	// Load or create JWT secret and refresh salt in memory DB (persisted across restarts)
	jwtsecrets.Ensure(db)

	// Run memory DB migrations (one-time data migrations)
	if err := database.RunMemoryMigrations(db, dataDir, migrations.MemoryMigrations()); err != nil {
		log.Fatalf("Failed to run memory migrations: %v", err)
	}

	// The stack comes entirely from the stacks repository system (fetched/cached
	// on demand); the legacy clone of github.com/onuragtas/docker is gone.

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
	onion_proxy.Init(dockerEnvironmentManager)
	dns_server.Init(dockerEnvironmentManager)
	vpn_server.Init()
	traffic_inspector.Init(db)
	cloudflare.Init()
	email_server.Init(dockerEnvironmentManager)
	go deployment.GetDeployment().Run()
	localproxy.GetLocalProxyManager().StartAll()
	api_gateway.GetGateway().StartAll()
	network.ApplyPersistedAliases(globalDB)

	// Auto-start persisted tunnel clients flagged AutoStart, once the tunnel
	// daemon and gateway routes are up. Backgrounded so boot is not blocked.
	go func() {
		time.Sleep(3 * time.Second)
		controllers.StartPersistedAutoTunnels()
		// Remote-managed agents: register with their servers and open assigned
		// tunnels. Runs without a logged-in user (uses stored credentials/secret).
		controllers.StartTunnelAgents()
	}()
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
		{"dns_query_logs", func() error {
			return memory.RegisterWithLimit[*dns_server.DNSQueryLog](db, "dns_query_logs", logTableCap("dns_query_logs"))
		}},

		// VPN entities
		{"vpn_servers", func() error { return memory.Register[*vpn_server.VPNServer](db, "vpn_servers") }},
		{"vpn_users", func() error { return memory.Register[*vpn_server.VPNUser](db, "vpn_users") }},
		{"vpn_groups", func() error { return memory.Register[*vpn_server.VPNUserGroup](db, "vpn_groups") }},
		{"vpn_security_rules", func() error { return memory.Register[*vpn_server.VPNSecurityRule](db, "vpn_security_rules") }},
		{"vpn_connections", func() error { return memory.Register[*vpn_server.VPNConnection](db, "vpn_connections") }},
		{"vpn_connection_logs", func() error {
			return memory.RegisterWithLimit[*vpn_server.VPNConnectionLog](db, "vpn_connection_logs", logTableCap("vpn_connection_logs"))
		}},
		{"vpn_bandwidth_stats", func() error {
			return memory.RegisterWithLimit[*vpn_server.VPNBandwidthStat](db, "vpn_bandwidth_stats", logTableCap("vpn_bandwidth_stats"))
		}},

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
		{"cloudflare_events", func() error {
			return memory.RegisterWithLimit[*cloudflare.CloudflareEvent](db, "cloudflare_events", logTableCap("cloudflare_events"))
		}},

		// Email entities
		{"email_domains", func() error { return memory.Register[*email_server.EmailDomain](db, "email_domains") }},
		{"email_mailboxes", func() error { return memory.Register[*email_server.EmailMailbox](db, "email_mailboxes") }},
		{"email_aliases", func() error { return memory.Register[*email_server.EmailAlias](db, "email_aliases") }},
		{"email_folders", func() error { return memory.Register[*email_server.EmailFolder](db, "email_folders") }},
		{"email_messages", func() error { return memory.Register[*email_server.Email](db, "email_messages") }},
		{"email_attachments", func() error { return memory.Register[*email_server.EmailAttachment](db, "email_attachments") }},
		{"email_filters", func() error { return memory.Register[*email_server.EmailFilter](db, "email_filters") }},
		{"email_logs", func() error {
			return memory.RegisterWithLimit[*email_server.EmailLog](db, "email_logs", logTableCap("email_logs"))
		}},
		{"email_server_configs", func() error { return memory.Register[*email_server.EmailServerConfig](db, "email_server_configs") }},

		// Other entities
		{"users", func() error { return memory.Register[*models.User](db, "users") }},
		{"saved_commands", func() error { return memory.Register[*database.SavedCommand](db, "saved_commands") }},
		{"release_cache", func() error { return memory.Register[*cache_models.ReleaseCache](db, "release_cache") }},
		{"local_proxy_items", func() error { return memory.Register[*localproxy.LocalProxyItem](db, "local_proxy_items") }},
		{"dev_envs", func() error { return memory.Register[*devenv.DevEnvEntity](db, "dev_envs") }},
		{"deployment_settings", func() error { return memory.Register[*deployment.DeploymentSettingsEntity](db, "deployment_settings") }},
		{"deployment_projects", func() error { return memory.Register[*deployment.DeploymentProjectEntity](db, "deployment_projects") }},
		{"service_settings", func() error { return memory.Register[*dockermanager.ServiceSettingsEntity](db, "service_settings") }},
		{"starred_vhosts", func() error { return memory.Register[*dockermanager.StarredVHostEntity](db, "starred_vhosts") }},
		{"php_xdebug_settings", func() error {
			return memory.Register[*php_debug_adapter.PhpXDebugSettingsEntity](db, "php_xdebug_settings")
		}},
		{"php_xdebug_mappings", func() error {
			return memory.Register[*php_debug_adapter.PhpXDebugMappingEntity](db, "php_xdebug_mappings")
		}},
		{"stacks_repositories", func() error { return memory.Register[*stacks.RepositoryEntity](db, stacks.TableRepositories) }},
		{"stacks_custom_services", func() error { return memory.Register[*stacks.CustomServiceEntity](db, stacks.TableCustomServices) }},
		{"stacks_active_services", func() error { return memory.Register[*stacks.ActiveServiceEntity](db, stacks.TableActiveServices) }},
		{"stacks_devenv_settings", func() error { return memory.Register[*stacks.DevEnvSettingsEntity](db, stacks.TableDevEnvSettings) }},
		{"stacks_meta", func() error { return memory.Register[*stacks.MetaEntity](db, stacks.TableMeta) }},
		{"api_gateway_config", func() error { return memory.Register[*api_gateway.ApiGatewayConfigEntity](db, "api_gateway_config") }},
		{"api_gateway_blocks", func() error { return memory.Register[*api_gateway.ApiGatewayBlockEntity](db, "api_gateway_blocks") }},
		{"onion_services", func() error {
			return memory.Register[*onion_proxy.OnionServiceEntity](db, onion_proxy.TableOnionServices)
		}},
		{"jwt_secrets", func() error { return memory.Register[*jwtsecrets.JWTSecretsEntity](db, jwtsecrets.TableName) }},
		{"memguard_config", func() error { return memory.Register[*memguard.ConfigEntity](db, memguard.TableName) }},
		{"vpn_ca", func() error { return memory.Register[*traffic_inspector.CAEntity](db, traffic_inspector.CATableName) }},
		// Tunnel server
		{"tunnel_server_config", func() error { return memory.Register[*tunnel_server.TunnelServerConfig](db, "tunnel_server_config") }},
		{"tunnel_domains", func() error { return memory.Register[*tunnel_server.TunnelDomain](db, "tunnel_domains") }},
		{"tunnel_users", func() error { return memory.Register[*tunnel_server.TunnelUser](db, "tunnel_users") }},
		{"tunnel_server_credentials", func() error {
			return memory.Register[*tunnel_server.TunnelServerCredential](db, "tunnel_server_credentials")
		}},
		{"tunnel_servers", func() error { return memory.Register[*tunnel_server.TunnelServer](db, "tunnel_servers") }},
		{"tunnel_client_configs", func() error {
			return memory.Register[*tunnel_server.TunnelClientConfig](db, tunnel_server.TableTunnelClientConfigs)
		}},
		{"tunnel_agents", func() error { return memory.Register[*tunnel_server.TunnelAgent](db, tunnel_server.TableTunnelAgents) }},
		{"tunnel_agent_assignments", func() error {
			return memory.Register[*tunnel_server.TunnelAgentAssignment](db, tunnel_server.TableTunnelAgentAssignments)
		}},
		{"network_ip_aliases", func() error { return memory.Register[*network.PersistedIPAlias](db, network.TableIPAliases) }},
	}

	for _, entity := range entities {
		if err := entity.register(); err != nil {
			return err
		}
	}

	return nil
}
