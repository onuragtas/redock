package stacks

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"

	"redock/platform/memory"
)

// DevEnvSettings holds the configurable parameters for personal development
// containers — the stacks, settings-driven replacement for serviceip.sh.
type DevEnvSettings struct {
	SitesPath           string   `json:"sites_path"`            // host dir holding each user's workspace
	Image               string   `json:"image"`                 // dev container image
	MemoryBytes         int64    `json:"memory_bytes"`          // hard memory limit (0 = none)
	MemorySwapBytes     int64    `json:"memory_swap_bytes"`     // memory+swap limit (0 = none)
	NanoCPUs            int64    `json:"nano_cpus"`             // CPU limit in 1e9 units (0 = none)
	Privileged          bool     `json:"privileged"`            // run privileged
	SSHContainerPort    int      `json:"ssh_container_port"`    // container SSH port (usually 22)
	RedockContainerPort int      `json:"redock_container_port"` // container redock port (usually 6001)
	SSHPortBase         int      `json:"ssh_port_base"`         // host SSH port auto-assignment starts here
	HostSuffix          string   `json:"host_suffix"`           // e.g. "ept-dev.net" for generated /etc/hosts entries
	HostServices        []string `json:"host_services"`         // e.g. ["admin","fe"] → <svc>.<user>.<suffix>
	InstallNvm          bool     `json:"install_nvm"`           // run nvm install on create
}

// DefaultDevEnvSettings returns sane defaults matching the legacy serviceip.sh.
func DefaultDevEnvSettings() DevEnvSettings {
	return DevEnvSettings{
		SitesPath:           "/sites",
		Image:               "hakanbaysal/devenv:latest",
		MemoryBytes:         8 * 1024 * 1024 * 1024,
		MemorySwapBytes:     10 * 1024 * 1024 * 1024,
		NanoCPUs:            1_500_000_000,
		Privileged:          true,
		SSHContainerPort:    22,
		RedockContainerPort: 6001,
		SSHPortBase:         100,
		HostSuffix:          "",
		HostServices:        nil,
		InstallNvm:          false,
	}
}

// GetDevEnvSettings loads the persisted settings, or the defaults.
func (m *Manager) GetDevEnvSettings() DevEnvSettings {
	if m.db != nil {
		if list := memory.FindAll[*DevEnvSettingsEntity](m.db, TableDevEnvSettings); len(list) > 0 {
			s := list[0].Settings
			if s.SitesPath == "" {
				s.SitesPath = "/sites"
			}
			if s.Image == "" {
				s.Image = DefaultDevEnvSettings().Image
			}
			if s.SSHContainerPort == 0 {
				s.SSHContainerPort = 22
			}
			if s.RedockContainerPort == 0 {
				s.RedockContainerPort = 6001
			}
			if s.SSHPortBase == 0 {
				s.SSHPortBase = 100
			}
			return s
		}
	}
	return DefaultDevEnvSettings()
}

// SaveDevEnvSettings persists the settings (single row).
func (m *Manager) SaveDevEnvSettings(s DevEnvSettings) error {
	if m.db == nil {
		return fmt.Errorf("no database")
	}
	list := memory.FindAll[*DevEnvSettingsEntity](m.db, TableDevEnvSettings)
	if len(list) == 0 {
		return memory.Create(m.db, TableDevEnvSettings, &DevEnvSettingsEntity{Settings: s})
	}
	list[0].Settings = s
	return memory.Update(m.db, TableDevEnvSettings, list[0])
}

// CreateDevEnv launches a personal SSH development container for a user via the
// Docker SDK, driven entirely by DevEnvSettings (stacks serviceip.sh).
func (m *Manager) CreateDevEnv(ctx context.Context, username, password string, sshPort, redockPort int) error {
	if username == "" || sshPort == 0 {
		return fmt.Errorf("devenv requires a username and SSH port")
	}
	s := m.GetDevEnvSettings()

	if err := m.Engine.PullImage(ctx, s.Image, "", os.Stdout); err != nil {
		return fmt.Errorf("pull devenv image: %w", err)
	}
	if err := m.Engine.EnsureNetwork(ctx); err != nil {
		return err
	}

	// Per-user host directories (bind-mounted into the container).
	userDir := filepath.Join(s.SitesPath, username)
	hostsFile := filepath.Join(userDir, ".hosts", "hosts")
	for _, d := range []string{
		userDir,
		filepath.Join(userDir, ".hosts"),
		filepath.Join(userDir, "cron.d"),
		filepath.Join(userDir, ".nvm"),
		filepath.Join(userDir, ".configs"),
		filepath.Join(userDir, ".docker-environment"),
		filepath.Join(m.VHostNginxDir(), username),
		filepath.Join(m.VHostHttpdDir(), username),
	} {
		_ = os.MkdirAll(d, 0o755)
	}
	// Seed the hosts file (configurable extra domains → local IP).
	writeDevEnvHosts(hostsFile, username, s)

	// Recreate semantics.
	_ = m.Engine.Remove(ctx, username)

	cli := m.Engine.Client()

	config := &container.Config{
		Image:    s.Image,
		Hostname: username,
		Env:      []string{"PASSWORD=" + password},
	}
	bindings := nat.PortMap{
		nat.Port(fmt.Sprintf("%d/tcp", s.SSHContainerPort)): []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: fmt.Sprintf("%d", sshPort)}},
	}
	exposed := nat.PortSet{nat.Port(fmt.Sprintf("%d/tcp", s.SSHContainerPort)): struct{}{}}
	if redockPort != 0 {
		rp := nat.Port(fmt.Sprintf("%d/tcp", s.RedockContainerPort))
		bindings[rp] = []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: fmt.Sprintf("%d", redockPort)}}
		exposed[rp] = struct{}{}
	}
	config.ExposedPorts = exposed

	hostConfig := &container.HostConfig{
		Privileged:    s.Privileged,
		RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
		PortBindings:  bindings,
		Mounts: []mount.Mount{
			{Type: mount.TypeBind, Source: userDir, Target: "/sites"},
			{Type: mount.TypeBind, Source: filepath.Join(userDir, ".nvm"), Target: "/root/.nvm"},
			{Type: mount.TypeBind, Source: filepath.Join(userDir, ".docker-environment"), Target: "/root/.docker-environment"},
			{Type: mount.TypeBind, Source: filepath.Join(userDir, "cron.d"), Target: "/etc/cron.d"},
			{Type: mount.TypeBind, Source: hostsFile, Target: "/etc/hosts"},
			{Type: mount.TypeBind, Source: filepath.Join(userDir, ".configs"), Target: "/root/.configs"},
			{Type: mount.TypeBind, Source: filepath.Join(m.VHostNginxDir(), username), Target: "/usr/local/nginx"},
			{Type: mount.TypeBind, Source: filepath.Join(m.VHostHttpdDir(), username), Target: "/usr/local/httpd"},
		},
	}
	hostConfig.Resources.Memory = s.MemoryBytes
	hostConfig.Resources.MemorySwap = s.MemorySwapBytes
	hostConfig.Resources.NanoCPUs = s.NanoCPUs

	netConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			m.Engine.Network(): {Aliases: []string{username}},
		},
	}

	resp, err := cli.ContainerCreate(ctx, config, hostConfig, netConfig, nil, username)
	if err != nil {
		return fmt.Errorf("create devenv %s: %w", username, err)
	}
	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("start devenv %s: %w", username, err)
	}

	// Post-setup (best-effort): username marker, git credential helper, nvm.
	_, _ = m.Engine.Exec(ctx, username, []string{"sh", "-c", "echo " + username + " > /root/.username"})
	_, _ = m.Engine.Exec(ctx, username, []string{"sh", "-c",
		`printf '[credential]\n\thelper = store --file /root/.configs/.git-credential\n' > /root/.gitconfig`})
	if s.InstallNvm {
		_, _ = m.Engine.Exec(ctx, username, []string{"sh", "-c",
			"curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.3/install.sh | bash"})
	}

	// Regenerate the nginx service-IP map and reload the web servers.
	m.regenerateServiceIPMap(ctx)
	_ = m.Engine.Restart(ctx, "nginx")
	_ = m.Engine.Restart(ctx, "httpd")
	return nil
}

// NextFreePort returns the first port >= base that is neither in the used set
// nor currently bound on the host (auto-increment with conflict avoidance).
func NextFreePort(base int, used map[int]bool) int {
	p := base
	if p < 1 {
		p = 1
	}
	for p <= 65535 {
		if !used[p] && portAvailable(p) {
			return p
		}
		p++
	}
	return 0
}

func portAvailable(p int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

// RemoveDevEnv force-removes a personal development container.
func (m *Manager) RemoveDevEnv(ctx context.Context, username string) error {
	err := m.Engine.Remove(ctx, username)
	m.regenerateServiceIPMap(ctx)
	return err
}

// writeDevEnvHosts seeds the per-user /etc/hosts with configurable domains
// (<service>.<user>.<suffix> → local IP), matching the legacy add_hosts.
func writeDevEnvHosts(hostsFile, username string, s DevEnvSettings) {
	if s.HostSuffix == "" || len(s.HostServices) == 0 {
		// still ensure the file exists for the bind mount
		if _, err := os.Stat(hostsFile); err != nil {
			_ = os.WriteFile(hostsFile, []byte("127.0.0.1 localhost\n"), 0o644)
		}
		return
	}
	ip := localIPv4()
	var b strings.Builder
	b.WriteString("127.0.0.1 localhost\n")
	for _, svc := range s.HostServices {
		fmt.Fprintf(&b, "%s %s.%s.%s\n", ip, svc, username, s.HostSuffix)
	}
	_ = os.WriteFile(hostsFile, []byte(b.String()), 0o644)
}

// regenerateServiceIPMap writes etc/nginx/docker_service_ip.conf mapping each
// dev container's name to its IP on the redock network (used by nginx to route
// per-user traffic), replacing the awk/docker-inspect logic in serviceip.sh.
//
// The file is self-contained: it defines $username (extracted from the host as
// <service>.<user>.<suffix>) so nginx never fails with "unknown $username
// variable". When there are no dev containers the file is removed so an empty
// map can't break the nginx config.
func (m *Manager) regenerateServiceIPMap(ctx context.Context) {
	path := filepath.Join(m.VHostNginxDir(), "docker_service_ip.conf")

	cli := m.Engine.Client()
	list, err := cli.ContainerList(ctx, container.ListOptions{All: false})
	if err != nil {
		return
	}
	s := m.GetDevEnvSettings()

	entries := map[string]string{} // container name → IP
	for _, c := range list {
		if c.Image != s.Image {
			continue
		}
		name := ""
		for _, n := range c.Names {
			name = strings.TrimPrefix(n, "/")
			break
		}
		if name == "" {
			continue
		}
		if c.NetworkSettings != nil {
			for _, ep := range c.NetworkSettings.Networks {
				if ep.IPAddress != "" {
					entries[name] = ep.IPAddress
					break
				}
			}
		}
	}

	if len(entries) == 0 {
		_ = os.Remove(path) // no dev containers → don't leave a map in conf.d
		return
	}

	var b strings.Builder
	// Define $username self-sufficiently: second host label of <svc>.<user>.<suffix>.
	b.WriteString("map $host $username {\n    default \"\";\n    ~^[^.]+\\.(?<u>[^.]+)\\. $u;\n}\n")
	b.WriteString("map $username $serviceip {\n    default \"\";\n")
	for name, ip := range entries {
		fmt.Fprintf(&b, "    %s %s;\n", name, ip)
	}
	b.WriteString("}\n")
	_ = os.WriteFile(path, []byte(b.String()), 0o644)
}
