import LayoutAuthenticated from '@/layouts/LayoutAuthenticated.vue'
import Home from '@/views/HomeView.vue'
import { createRouter, createWebHashHistory } from 'vue-router'

const routes = [
  // Login page - outside main layout
  {
    meta: {
      title: 'Login'
    },
    path: '/login',
    name: 'login',
    component: () => import('@/views/LoginView.vue')
  },
  // Tunnel server sign-in/register — called with redirect_uri; after auth, token is sent to redirect
  {
    meta: {
      title: 'Sign in to tunnel server'
    },
    path: '/tunnel-auth',
    name: 'tunnel-auth',
    component: () => import('@/views/TunnelAuthView.vue')
  },
  // Main layout - all authenticated pages
  {
    path: '/',
    component: LayoutAuthenticated,
    children: [
      {
        // Document title tag
        // We combine it with defaultDocumentTitle set in `src/main.js` on router.afterEach hook
        meta: {
          title: 'Dashboard'
        },
        path: '',
        name: 'dashboard',
        component: Home
      },
      {
        meta: {
          title: 'Dashboard'
        },
        path: 'dashboard',
        redirect: '/'
      },
      {
        meta: {
          title: 'Setup Environment',
        },
        path: 'setup_environment',
        name: 'setup_environment',
        component: () => import('@/views/SetupEnvironment.vue')
      },
      {
        meta: {
          title: 'Container Settings',
        },
        path: 'container_settings',
        name: 'container_settings',
        component: () => import('@/views/ContainerSettings.vue')
      },
      {
        meta: {
          title: 'Container Settings',
        },
        path: 'container-settings',
        name: 'container-settings',
        component: () => import('@/views/ContainerSettings.vue')
      },
      {
        meta: {
          title: 'Virtual Hosts',
        },
        path: 'virtual_hosts',
        name: 'virtual_hosts',
        component: () => import('@/views/VirtualHosts.vue')
      },
      {
        meta: {
          title: 'Virtual Hosts',
        },
        path: 'virtual-hosts',
        name: 'virtual-hosts',
        component: () => import('@/views/VirtualHosts.vue')
      },
      {
        meta: {
          title: 'Saved Commands',
        },
        path: 'saved_commands',
        name: 'saved_commands',
        component: () => import('@/views/SavedCommands.vue')
      },
      {
        meta: {
          title: 'Saved Commands',
        },
        path: 'saved-commands',
        name: 'saved-commands',
        component: () => import('@/views/SavedCommands.vue')
      },
      {
        meta: {
          title: 'Logs',
        },
        path: 'logs',
        name: 'logs',
        component: () => import('@/views/TerminalView.vue')
      },
      {
        meta: {
          title: 'Personal Containers',
        },
        path: 'devenv',
        name: 'devenv',
        component: () => import('@/views/DevEnv.vue')
      },
      {
        meta: {
          title: 'Tunnel Proxy Server',
        },
        path: 'tunnel_proxy_server',
        name: 'tunnel_proxy_server',
        component: () => import('@/views/TunnelProxyServer.vue')
      },
      {
        meta: {
          title: 'Tunnel Proxy Server',
        },
        path: 'tunnel-proxy-server',
        name: 'tunnel-proxy-server',
        component: () => import('@/views/TunnelProxyServer.vue')
      },
      {
        meta: {
          title: 'Tunnel Proxy Client',
        },
        path: 'tunnel_proxy_client',
        name: 'tunnel_proxy_client',
        component: () => import('@/views/TunnelProxyClient.vue')
      },
      {
        meta: {
          title: 'Tunnel Proxy Client',
        },
        path: 'tunnel-proxy-client',
        name: 'tunnel-proxy-client',
        component: () => import('@/views/TunnelProxyClient.vue')
      },
      {
        meta: {
          title: 'Tunnel Proxy Client',
        },
        path: 'tunnel_proxy',
        name: 'tunnel_proxy',
        redirect: '/tunnel-proxy-client'
      },
      {
        meta: {
          title: 'Tunnel Proxy Client',
        },
        path: 'tunnel-proxy',
        name: 'tunnel-proxy',
        redirect: '/tunnel-proxy-client'
      },
      {
        meta: {
          title: 'Local Proxy',
        },
        path: 'local_proxy',
        name: 'local_proxy',
        component: () => import('@/views/LocalProxy.vue')
      },
      {
        meta: {
          title: 'Local Proxy',
        },
        path: 'local-proxy',
        name: 'local-proxy',
        component: () => import('@/views/LocalProxy.vue')
      },
      {
        meta: {
          title: 'PHP XDebug Adapter',
        },
        path: 'php_xdebug_adapter',
        name: 'php_xdebug_adapter',
        component: () => import('@/views/PhpXDebugAdapter.vue')
      },
      {
        meta: {
          title: 'PHP XDebug Adapter',
        },
        path: 'php-xdebug-adapter',
        name: 'php-xdebug-adapter',
        component: () => import('@/views/PhpXDebugAdapter.vue')
      },
      {
        meta: {
          title: 'Exec',
        },
        path: 'exec/:id?',
        name: 'exec',
        component: () => import('@/views/TerminalView.vue')
      },
      {
        meta: {
          title: 'SSH Client',
        },
        path: 'ssh_client',
        name: 'ssh_client',
        component: () => import('@/views/AttachSshUi.vue')
      },
      {
        meta: {
          title: 'Deployment'
        },
        path: 'deployment',
        name: 'Deployment',
        component: () => import('@/views/Deployment.vue')
      },
      {
        meta: {
          title: 'API Gateway'
        },
        path: 'api_gateway',
        name: 'api_gateway',
        component: () => import('@/views/ApiGateway.vue')
      },
      {
        meta: {
          title: 'API Gateway'
        },
        path: 'api-gateway',
        name: 'api-gateway',
        component: () => import('@/views/ApiGateway.vue')
      },
      {
        meta: {
          title: 'DNS Server'
        },
        path: 'dns_server',
        name: 'dns_server',
        component: () => import('@/views/DNSServer.vue')
      },
      {
        meta: {
          title: 'DNS Server'
        },
        path: 'dns-server',
        name: 'dns-server',
        component: () => import('@/views/DNSServer.vue')
      },
      {
        meta: {
          title: 'IP Alias'
        },
        path: 'ip_alias',
        name: 'ip_alias',
        component: () => import('@/views/IPAlias.vue')
      },
      {
        meta: {
          title: 'IP Alias'
        },
        path: 'ip-alias',
        name: 'ip-alias',
        component: () => import('@/views/IPAlias.vue')
      },
      {
        meta: {
          title: 'VPN Server'
        },
        path: 'vpn_server',
        name: 'vpn_server',
        component: () => import('@/views/VPNServer.vue')
      },
      {
        meta: {
          title: 'VPN Server'
        },
        path: 'vpn-server',
        name: 'vpn-server',
        component: () => import('@/views/VPNServer.vue')
      },
      {
        meta: {
          title: 'Email Server'
        },
        path: 'email_server',
        name: 'email_server',
        component: () => import('@/views/Email/EmailServer.vue')
      },
      {
        meta: {
          title: 'Email Server'
        },
        path: 'email-server',
        name: 'email-server',
        component: () => import('@/views/Email/EmailServer.vue')
      },
      {
        meta: {
          title: 'Cloudflare'
        },
        path: 'cloudflare',
        name: 'cloudflare',
        component: () => import('@/views/Cloudflare/CloudflareManager.vue')
      },
      {
        meta: {
          title: 'Updates'
        },
        path: 'updates',
        name: 'updates',
        component: () => import('@/views/Updates.vue')
      },
      {
        meta: {
          title: 'Users',
          requiresAdmin: true
        },
        path: 'users',
        name: 'users',
        component: () => import('@/views/UsersView.vue')
      },
      {
        meta: {
          title: 'Forms'
        },
        path: 'forms',
        name: 'forms',
        component: () => import('@/views/FormsView.vue')
      },
      {
        meta: {
          title: 'Profile'
        },
        path: 'profile',
        name: 'profile',
        component: () => import('@/views/ProfileView.vue')
      },
      {
        meta: {
          title: 'Ui'
        },
        path: 'ui',
        name: 'ui',
        component: () => import('@/views/UiView.vue')
      },
      {
        meta: {
          title: 'Responsive layout'
        },
        path: 'responsive',
        name: 'responsive',
        component: () => import('@/views/ResponsiveView.vue')
      }
    ]
  },
  // Error sayfası - layout dışında
  {
    meta: {
      title: 'Error'
    },
    path: '/error',
    name: 'error',
    component: () => import('@/views/ErrorView.vue')
  }
]

const router = createRouter({
  history: createWebHashHistory(),
  routes,
  scrollBehavior(to, from, savedPosition) {
    return savedPosition || {top: 0}
  }
})

export default router
