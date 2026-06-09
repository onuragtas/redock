package stacks

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// xdebug.ini templates (%s host, %d port).
const tplXdebug = `zend_extension=xdebug.so
xdebug.remote_enable=1
xdebug.remote_autostart=1
xdebug.remote_connect_back=
xdebug.remote_handler = dbgp
xdebug.remote_mode = req
xdebug.remote_host=%s
xdebug.remote_port=%d`

const tplXdebug8 = `
zend_extension=xdebug.so
xdebug.mode=debug
xdebug.client_host=%s
xdebug.start_with_request=yes
xdebug.client_port=%d
xdebug.discover_client_host=true
`

// VHostNginxDir / VHostHttpdDir are the config directories bind-mounted into the
// nginx/httpd containers. They deliberately match the legacy docker_manager
// paths (etc/nginx, httpd/sites-enabled) so the existing Virtual Hosts page and
// the stacks engine share a single source of truth: the page writes config
// files there and stacks nginx/httpd serve them.
func (m *Manager) VHostNginxDir() string { return filepath.Join(m.WorkDir, "etc", "nginx") }
func (m *Manager) VHostHttpdDir() string { return filepath.Join(m.WorkDir, "httpd", "sites-enabled") }

// webSeedFiles are base web-server config files that live in a directory mount
// (so they are not fetched as individual bind files during repo sync). They are
// seeded into the shared, writable dirs so nginx/httpd have a working base
// config even without the legacy git clone.
var webSeedFiles = []string{"etc/nginx/default", "etc/nginx/default.conf"}

// SeedWebConfig ensures the shared nginx/httpd config dirs exist and contain the
// base config files, fetching any missing ones from the default repository
// (repo cache first, then HTTP). Best-effort and idempotent — existing files
// (e.g. the user's virtual hosts) are never overwritten.
func (m *Manager) SeedWebConfig() {
	_ = os.MkdirAll(m.VHostNginxDir(), 0o755)
	_ = os.MkdirAll(m.VHostHttpdDir(), 0o755)

	var baseURL string
	for _, r := range m.Registry.Repos {
		if r.Kind == RepoComposeURL && r.Enabled {
			if i := strings.LastIndex(r.Location, "/"); i >= 0 {
				baseURL = r.Location[:i+1]
			}
			break
		}
	}

	for _, rel := range webSeedFiles {
		dst := filepath.Join(m.WorkDir, filepath.FromSlash(rel))
		if pathExists(dst) {
			continue
		}
		// 1) repo cache, 2) HTTP from the default repo base.
		cacheSrc := filepath.Join(m.WorkDir, "repositories", DefaultRepoName, filepath.FromSlash(rel))
		if data, err := os.ReadFile(cacheSrc); err == nil {
			_ = os.WriteFile(dst, data, 0o644)
			continue
		}
		if baseURL != "" && m.Registry.Get != nil {
			if data, err := m.Registry.Get(baseURL + rel); err == nil {
				_ = os.WriteFile(dst, data, 0o644)
			}
		}
	}
}

// vhostBindOverride redirects a service's nginx/httpd config bind mounts to the
// shared vhost dirs above. Called from materialize.
func (m *Manager) vhostBindOverride(target string) (string, bool) {
	switch strings.TrimRight(target, "/") {
	case "/etc/nginx/conf.d":
		_ = os.MkdirAll(m.VHostNginxDir(), 0o755)
		return m.VHostNginxDir(), true
	case "/etc/apache2/sites-enabled":
		_ = os.MkdirAll(m.VHostHttpdDir(), 0o755)
		return m.VHostHttpdDir(), true
	}
	return "", false
}

// RegenerateXDebugINI writes a fresh xdebug.ini (pointing at the current host
// IP) into every active *_xdebug PHP container via the SDK and restarts them.
// It is invoked by docker_manager.RegenerateXDebugConf when stacks mode is on,
// so the existing dashboard "Regenerate XDebug" action keeps working.
func (m *Manager) RegenerateXDebugINI(ctx context.Context) error {
	host := localIPv4()
	const port = 10000
	for _, name := range m.Active() {
		if !strings.Contains(name, "_xdebug") {
			continue
		}
		conf := fmt.Sprintf(tplXdebug, host, port)
		if strings.Contains(name, "81") || strings.Contains(name, "84") {
			conf = fmt.Sprintf(tplXdebug8, host, port)
		}
		cn := name
		if s, ok := m.catalogMap()[name]; ok && s.ContainerName != "" {
			cn = s.ContainerName
		}
		if err := m.Engine.CopyToContainer(ctx, cn, "/usr/local/etc/php/conf.d/xdebug.ini", []byte(conf)); err != nil {
			return fmt.Errorf("copy xdebug.ini to %s: %w", cn, err)
		}
		_ = m.Engine.Restart(ctx, cn)
	}
	return nil
}

// localIPv4 returns the first non-loopback IPv4 address of the host.
func localIPv4() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if v4 := ipnet.IP.To4(); v4 != nil {
				return v4.String()
			}
		}
	}
	return "127.0.0.1"
}
