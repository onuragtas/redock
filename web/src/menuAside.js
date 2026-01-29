import {
  mdiBugCheck,
  mdiCloud,
  mdiConsole,
  mdiDns,
  mdiDocker,
  mdiEmail,
  mdiHome,
  mdiLan,
  mdiLanConnect,
  mdiLaptop,
  mdiNetworkOutline,
  mdiPlaylistEdit,
  mdiRocket,
  mdiServerNetwork,
  mdiWeb,
  mdiWrench
} from '@mdi/js'

export default [
  { name: 'Dashboard', path: '/', icon: mdiHome },
  { name: 'Deployment', path: '/deployment', icon: mdiRocket },
  { name: 'Setup Environment', path: '/setup_environment', icon: mdiWrench },
  { name: 'Dev Environment', path: '/devenv', icon: mdiLaptop },
  { name: 'Container Settings', path: '/container_settings', icon: mdiDocker },
  { name: 'API Gateway', path: '/api-gateway', icon: mdiNetworkOutline },
  { name: 'DNS Server', path: '/dns-server', icon: mdiDns },
  { name: 'VPN Server', path: '/vpn-server', icon: mdiServerNetwork },
  { name: 'Email Server', path: '/email-server', icon: mdiEmail },
  { name: 'Cloudflare', path: '/cloudflare', icon: mdiCloud },
  { name: 'Local Proxy', path: '/local-proxy', icon: mdiLan },
  { name: 'Terminal', path: '/exec', icon: mdiConsole },
  { name: 'Tunnel Proxy', path: '/tunnel-proxy', icon: mdiLanConnect },
  { name: 'Virtual Hosts', path: '/virtual-hosts', icon: mdiWeb },
  { name: 'Saved Commands', path: '/saved-commands', icon: mdiPlaylistEdit },
  { name: 'PHP XDebug', path: '/php-xdebug-adapter', icon: mdiBugCheck }
]
