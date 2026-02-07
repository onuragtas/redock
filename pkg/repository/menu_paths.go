package repository

// MenuItem menü öğesi (path, name, icon key frontend'de @mdi/js ile eşleşir).
type MenuItem struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Icon string `json:"icon"`
}

// AllMenuItems tüm menü tanımları. Tek kaynak: backend.
var AllMenuItems = []MenuItem{
	{Path: "/", Name: "Dashboard", Icon: "mdiHome"},
	{Path: "/setup_environment", Name: "Setup Environment", Icon: "mdiWrench"},
	{Path: "/deployment", Name: "Deployment", Icon: "mdiRocket"},
	{Path: "/exec", Name: "Terminal", Icon: "mdiConsole"},
	{Path: "/virtual-hosts", Name: "Virtual Hosts", Icon: "mdiWeb"},
	{Path: "/tunnel-proxy-client", Name: "Tunnel Proxy Client", Icon: "mdiLanConnect"},
	{Path: "/local-proxy", Name: "Local Proxy", Icon: "mdiLan"},
	{Path: "/saved-commands", Name: "Saved Commands", Icon: "mdiScriptText"},
	{Path: "/devenv", Name: "Dev Environment", Icon: "mdiLaptop"},
	{Path: "/container_settings", Name: "Container Settings", Icon: "mdiDocker"},
	{Path: "/php-xdebug-adapter", Name: "PHP XDebug", Icon: "mdiBugCheck"},
	{Path: "/api-gateway", Name: "API Gateway", Icon: "mdiNetworkOutline"},
	{Path: "/dns-server", Name: "DNS Server", Icon: "mdiDns"},
	{Path: "/vpn-server", Name: "VPN Server", Icon: "mdiServerNetwork"},
	{Path: "/email-server", Name: "Email Server", Icon: "mdiEmail"},
	{Path: "/cloudflare", Name: "Cloudflare", Icon: "mdiCloud"},
	{Path: "/users", Name: "Kullanıcılar", Icon: "mdiAccountGroup"},
	{Path: "/tunnel-proxy-server", Name: "Tunnel Proxy Server", Icon: "mdiServerNetwork"},
	{Path: "/updates", Name: "Updates", Icon: "mdiDownload"},
}

// AllMenuPaths path listesi (mevcut kullanım için).
var AllMenuPaths = []string{
	"/", "/deployment", "/setup_environment", "/devenv", "/container_settings",
	"/api-gateway", "/dns-server", "/vpn-server", "/email-server", "/cloudflare",
	"/local-proxy", "/exec", "/tunnel-proxy-server", "/tunnel-proxy-client",
	"/virtual-hosts", "/saved-commands", "/php-xdebug-adapter", "/updates",
	"/users",
}

// DefaultUserMenuPaths user rolü için varsayılan menüler (AllowedMenus boşsa).
var DefaultUserMenuPaths = []string{
	"/", "/deployment", "/devenv", "/container_settings", "/exec",
	"/saved-commands", "/virtual-hosts", "/updates",
}

// GetMenuItemsForUser kullanıcının görebileceği menü öğelerini döner.
func GetMenuItemsForUser(role string, allowedPaths []string) []MenuItem {
	var paths []string
	if role == AdminRoleName {
		paths = AllMenuPaths
	} else {
		paths = allowedPaths
		if len(paths) == 0 {
			paths = DefaultUserMenuPaths
		}
	}
	pathSet := make(map[string]bool)
	for _, p := range paths {
		pathSet[p] = true
	}
	var out []MenuItem
	for _, item := range AllMenuItems {
		if pathSet[item.Path] {
			out = append(out, item)
		}
	}
	return out
}
