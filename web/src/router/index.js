import Home from '@/views/HomeView.vue'
import { createRouter, createWebHashHistory } from 'vue-router'

const routes = [
  {
    meta: {
      title: 'Welcome'
    },
    path: '/',
    name: 'welcome',
    component: () => import('@/views/LandingView.vue')
  },
  {
    // Document title tag
    // We combine it with defaultDocumentTitle set in `src/main.js` on router.afterEach hook
    meta: {
      title: 'Dashboard'
    },
    path: '/dashboard',
    name: 'dashboard',
    component: Home
  },
  {
    meta: {
      title: 'Setup Environment',
    },
    path: '/setup_environment',
    name: 'setup_environment',
    component: () => import('@/views/SetupEnvironment.vue')
  },
  {
    meta: {
      title: 'Virtual Hosts',
    },
    path: '/virtual_hosts',
    name: 'virtual_hosts',
    component: () => import('@/views/VirtualHosts.vue')
  },
  {
    meta: {
      title: 'Saved Commands',
    },
    path: '/saved_commands',
    name: 'saved_commands',
    component: () => import('@/views/SavedCommands.vue')
  },
  {
    meta: {
      title: 'Logs',
    },
    path: '/logs',
    name: 'logs',
    component: () => import('@/views/TerminalView.vue')
  },
  {
    meta: {
      title: 'Personal Containers',
    },
    path: '/devenv',
    name: 'devenv',
    component: () => import('@/views/DevEnv.vue')
  },
  {
    meta: {
      title: 'Tunnel Proxy',
    },
    path: '/tunnel_proxy',
    name: 'tunnel_proxy',
    component: () => import('@/views/TunnelProxy.vue')
  },
  {
    meta: {
      title: 'Local Proxy',
    },
    path: '/local_proxy',
    name: 'local_proxy',
    component: () => import('@/views/LocalProxy.vue')
  },
  {
    meta: {
      title: 'PHP XDebug Adapter',
    },
    path: '/php_xdebug_adapter',
    name: 'php_xdebug_adapter',
    component: () => import('@/views/PhpXDebugAdapter.vue')
  },
  {
    meta: {
      title: 'Exec',
    },
    path: '/exec/:id?',
    name: 'exec',
    component: () => import('@/views/TerminalView.vue')
  },
  {
    meta: {
      title: 'SSH Client',
    },
    path: '/ssh_client',
    name: 'ssh_client',
    component: () => import('@/views/AttachSshUi.vue')
  },
  {
    meta: {
      title: 'Tables'
    },
    path: '/tables',
    name: 'tables',
    component: () => import('@/views/TablesView.vue')
  },
  {
    meta: {
      title: 'Forms'
    },
    path: '/forms',
    name: 'forms',
    component: () => import('@/views/FormsView.vue')
  },
  {
    meta: {
      title: 'Profile'
    },
    path: '/profile',
    name: 'profile',
    component: () => import('@/views/ProfileView.vue')
  },
  {
    meta: {
      title: 'Ui'
    },
    path: '/ui',
    name: 'ui',
    component: () => import('@/views/UiView.vue')
  },
  {
    meta: {
      title: 'Responsive layout'
    },
    path: '/responsive',
    name: 'responsive',
    component: () => import('@/views/ResponsiveView.vue')
  },
  {
    meta: {
      title: 'Login'
    },
    path: '/login',
    name: 'login',
    component: () => import('@/views/LoginView.vue')
  },
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
    return savedPosition || { top: 0 }
  }
})

export default router