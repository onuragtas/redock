import {
    mdiBookSearch,
    mdiMonitorDashboard
} from '@mdi/js'

export default [
  {
    to: '/dashboard',
    icon: mdiMonitorDashboard,
    label: 'Dashboard'
  },
  {
    to: '/setup_environment',
    icon: mdiMonitorDashboard,
    label: 'Setup Environment',
  },
  {
    to: '/virtual_hosts',
    icon: mdiBookSearch,
    label: 'Virtual Hosts',
  },
  {
    to: '/saved_commands',
    icon: mdiBookSearch,
    label: 'Saved Commands',
  },
  {
    to: '/devenv',
    icon: mdiBookSearch,
    label: 'Personal Containers',
  },
  {
    to: '/tunnel_proxy',
    icon: mdiBookSearch,
    label: 'Tunnel Proxy',
  },
  {
    to: '/local_proxy',
    icon: mdiBookSearch,
    label: 'Local Proxy',
  },
  {
    to: '/php_xdebug_adapter',
    icon: mdiBookSearch,
    label: 'PHP XDebug Adapter',
  },
  {
    to: '/deployment',
    icon: mdiBookSearch,
    label: 'Deployment',
  },
  {
    to: '/logs',
    icon: mdiBookSearch,
    label: 'Logs',
  }
]
