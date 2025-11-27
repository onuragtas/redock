<script setup>
import BaseIcon from '@/components/BaseIcon.vue'
import ApiService from '@/services/ApiService'
import { useTerminalStore } from '@/stores/terminalStore'
import {
  mdiAccount,
  mdiBell,
  mdiChevronDown,
  mdiClose,
  mdiConsole,
  mdiContentDuplicate,
  mdiDocker,
  mdiHome,
  mdiLogout,
  mdiMagnify,
  mdiMenu,
  mdiMinus,
  mdiNetworkOutline,
  mdiPlaylistEdit,
  mdiRocket,
  mdiTunnel,
  mdiWeb,
  mdiWrench,
  mdiLaptop,
  mdiLan,
  mdiLanConnect,
  mdiBugCheck,
  mdiWindowMaximize
} from '@mdi/js'
import { FitAddon } from '@xterm/addon-fit'
import { Terminal } from '@xterm/xterm'
import '@xterm/xterm/css/xterm.css'
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'

const router = useRouter()
const terminalStore = useTerminalStore()

// State
const sidebarOpen = ref(false)
const userMenuOpen = ref(false)
const terminalContainerRef = ref(null)
const activeTerminalElement = ref(null)
const isResizing = ref(false)
const terminalMinimized = ref(false)
const savedCommands = ref([])
const selectedCommandIndex = ref(-1)
const filteredCommands = ref([])
const commandFilter = ref('')
const windowWidth = ref(typeof window !== 'undefined' ? window.innerWidth : 1024)
const userInfo = ref({ username: '-' }) // Default to '-' if API fails

// Computed properties
const activeTab = computed(() => terminalStore.getActiveTab)

// Navigation items
const navigationItems = [
  { name: 'Dashboard', path: '/', icon: mdiHome },
  { name: 'Deployment', path: '/deployment', icon: mdiRocket },
  { name: 'Setup Environment', path: '/setup_environment', icon: mdiWrench },
  { name: 'Dev Environment', path: '/devenv', icon: mdiLaptop },
  { name: 'Container Settings', path: '/container_settings', icon: mdiDocker },
  { name: 'API Gateway', path: '/api-gateway', icon: mdiNetworkOutline },
  { name: 'Local Proxy', path: '/local-proxy', icon: mdiLan },
  { name: 'Terminal', path: '/exec', icon: mdiConsole },
  { name: 'Tunnel Proxy', path: '/tunnel-proxy', icon: mdiLanConnect },
  { name: 'Virtual Hosts', path: '/virtual-hosts', icon: mdiWeb },
  { name: 'Saved Commands', path: '/saved-commands', icon: mdiPlaylistEdit },
  { name: 'PHP XDebug', path: '/php-xdebug-adapter', icon: mdiBugCheck }
]

// Methods
const toggleSidebar = () => {
  sidebarOpen.value = !sidebarOpen.value
}

const toggleUserMenu = () => {
  userMenuOpen.value = !userMenuOpen.value
}

const logout = async () => {
  try {
    await ApiService.tunnelLogout()
    router.push('/login')
  } catch (error) {
    console.error('Logout failed:', error)
    router.push('/login')
  }
}

// Get user info
const getUserInfo = async () => {
  try {
    const response = await ApiService.get('/api/v1/tunnel/user_info')
    if (response.data && response.data.data && response.data.data.username) {
      userInfo.value = response.data.data
    }
  } catch (error) {
    // Keep default 'Admin' if API fails
    console.warn('Failed to fetch user info, using default:', error)
  }
}

// Close menus when clicking outside
const closeMenus = () => {
  userMenuOpen.value = false
}

// Terminal Functions
let isCreatingTerminal = false
const createNewTerminal = async (containerId, name = null) => {
  // Prevent duplicate creation
  if (isCreatingTerminal) {
    return
  }

  // Container ID yoksa da terminal oluşturabilir, ama uyarı göster
  if (!containerId) {
    console.warn('No container ID provided, creating terminal without container connection')
  }

  isCreatingTerminal = true

  try {
    const tab = terminalStore.addTab(containerId, name)

    // Ensure terminal is visible and persistent
    terminalStore.showTerminal()
    terminalStore.setKeepTerminalOpen(true)

    // Fast terminal initialization
    await nextTick()
    requestAnimationFrame(() => {
      initializeTerminal(tab)
    })
  } finally {
    // Reset flag after 1 second to allow new terminals if needed
    setTimeout(() => {
      isCreatingTerminal = false
    }, 1000)
  }
}

const initializeTerminal = async (tab) => {
  // Fast terminal initialization with RAF
  const findAndInit = () => {
    const terminalElement = document.getElementById(`terminal-${tab.id}`)

    if (terminalElement) {
      initializeTerminalInstance(tab, terminalElement)
      return
    }

    // Use RAF for smoother retries
    requestAnimationFrame(findAndInit)
  }

  // Start immediately
  requestAnimationFrame(findAndInit)
}

const initializeTerminalInstance = async (tab, terminalElement) => {
  try {
    const terminal = new Terminal({
      cursorBlink: true,
      fontSize: 14,
      fontFamily: 'Monaco, Menlo, "Ubuntu Mono", monospace',
      theme: {
        background: '#1f2937',
        foreground: '#f3f4f6',
        cursor: '#60a5fa',
        selection: '#374151'
      }
    })

    const fitAddon = new FitAddon()
    terminal.loadAddon(fitAddon)

    terminal.open(terminalElement)

    // Fit with delay and retries
    setTimeout(() => {
      try {
        fitAddon.fit()
        terminal.scrollToBottom()
      } catch (error) {
        console.warn('Fit error (attempt 1):', error)
        setTimeout(() => {
          try {
            fitAddon.fit()
          } catch (error2) {
            console.warn('Fit error (attempt 2):', error2)
          }
        }, 200)
      }
    }, 100)

    // WebSocket connection with auto-reconnect
    const connectWebSocket = () => {
      let wsUrl = window.location.hostname + (window.location.port == '5173' ? ':6001' : (window.location.port !== '' ? ':' + window.location.port : ''))
      wsUrl = 'ws://' + wsUrl + '/ws/' + tab.containerId

      const socket = new WebSocket(wsUrl)

      socket.onopen = () => {
        terminalStore.updateTabConnection(tab.id, true)
        terminal.write('\r\n\x1b[32mConnected to terminal\x1b[0m\r\n')

        // Send initial window size (backend expects this first)
        setTimeout(() => {
          if (socket.readyState === WebSocket.OPEN) {
            const windowSize = { high: terminal.rows, width: terminal.cols }
            const blob = new Blob([JSON.stringify(windowSize)], { type: 'application/json' })
            socket.send(blob)

            if (tab.containerId) {
              socket.send('docker exec -it ' + tab.containerId + ' bash\n');
            }
          }
        }, 100)
      }

      socket.onerror = (error) => {
        console.error('Terminal WebSocket error:', error)
        terminal.write('\r\n\x1b[31mWebSocket connection error\x1b[0m\r\n')
        terminalStore.updateTabConnection(tab.id, false)
      }

      socket.onclose = (event) => {
        terminalStore.updateTabConnection(tab.id, false)

        // Only attempt reconnect for unexpected closures (not manual closes)
        if (event.code !== 1000 && event.code !== 1001) {
          terminal.write('\r\n\x1b[33mConnection lost. Reconnecting...\x1b[0m\r\n')
          setTimeout(() => {
            if (terminalStore.getTab(tab.id)) { // Only reconnect if tab still exists
              connectWebSocket()
            }
          }, 2000)
        } else {
          terminal.write('\r\n\x1b[33mConnection closed\x1b[0m\r\n')
        }
      }

      // Handle WebSocket messages manually
      socket.onmessage = (event) => {
        terminal.write(event.data)
      }

      // Handle terminal input
      terminal.onData((data) => {
        if (socket.readyState === WebSocket.OPEN) {
          socket.send(data)
        }
      })

      // Handle terminal resize
      terminal.onResize(({ cols, rows }) => {
        if (socket.readyState === WebSocket.OPEN) {
          const windowSize = { high: rows, width: cols }
          const blob = new Blob([JSON.stringify(windowSize)], { type: 'application/json' })
          socket.send(blob)
        }
      })

      // Store terminal references
      terminalStore.setTabTerminal(tab.id, terminal, socket, fitAddon)

      return socket
    }

    // Initialize WebSocket connection
    connectWebSocket()

  } catch (error) {
    console.error('Terminal initialization error:', error)
  }
}

const switchToTerminalTab = (tabId) => {
  terminalStore.setActiveTab(tabId)
  terminalStore.showTerminal()
  terminalStore.setKeepTerminalOpen(true)

  // Fast fit on tab switch using store method
  requestAnimationFrame(() => {
    terminalStore.fitActiveTerminal()
  })
}

const closeTerminalTab = (tabId) => {
  terminalStore.removeTab(tabId)
}

const toggleTerminal = () => {
  terminalStore.toggleTerminal()
  terminalMinimized.value = !terminalStore.isTerminalVisible

  // Terminal açıldığında persistent mode'u aç
  if (terminalStore.isTerminalVisible && terminalStore.hasAnyTabs) {
    terminalStore.setKeepTerminalOpen(true)
  }
}

const minimizeTerminal = () => {
  terminalStore.hideTerminal()
  terminalMinimized.value = true
  // Kullanıcı manuel olarak minimize ettiğinde persistent mode'u kapat
  terminalStore.setKeepTerminalOpen(false)
}

const maximizeTerminal = () => {
  // Calculate dynamic maximum height based on screen size
  // Use 70% of viewport height, but cap at 800px minimum and respect the resize handle limits
  const maxHeight = Math.min(800, Math.max(600, Math.floor(window.innerHeight * 0.9)))

  terminalStore.setTerminalHeight(maxHeight)
  terminalStore.showTerminal()
  terminalMinimized.value = false
  terminalStore.setKeepTerminalOpen(true)

  // Fit terminal after resize
  setTimeout(() => {
    terminalStore.fitActiveTerminal()
  }, 100)
}

  const duplicateActiveTerminal = () => {
    const activeTerminal = terminalStore.terminals.find(t => t.id === terminalStore.activeTerminalId)
    if (activeTerminal) {
      createNewTerminal(activeTerminal.containerId, `${activeTerminal.name} (Copy)`)
    }
  }

// Handle terminal resizing - Ultra optimized
const startResize = (event) => {
  isResizing.value = true
  const startY = event.clientY
  const startHeight = terminalStore.terminalHeight

  let animationId = null
  let lastFitTime = 0
  const FIT_THROTTLE = 50 // Reduced for faster response

  const fitTerminals = () => {
    const now = performance.now()
    if (now - lastFitTime < FIT_THROTTLE) return

    lastFitTime = now
    // Use store method for consistent fit logic
    terminalStore.fitActiveTerminal()
  }

  const handleMouseMove = (e) => {
    // Cancel previous frame
    if (animationId) cancelAnimationFrame(animationId)

    // Ultra smooth RAF updates
    animationId = requestAnimationFrame(() => {
      const deltaY = startY - e.clientY
      const dynamicMaxHeight = Math.min(800, Math.max(600, Math.floor(window.innerHeight * 0.7)))
      const newHeight = Math.max(200, Math.min(dynamicMaxHeight, startHeight + deltaY))
      terminalStore.setTerminalHeight(newHeight)
      fitTerminals()
    })
  }

  const handleMouseUp = () => {
    isResizing.value = false
    if (animationId) cancelAnimationFrame(animationId)

    // Only fit active terminal on resize end
    requestAnimationFrame(() => {
      terminalStore.fitActiveTerminal()
    })

    document.removeEventListener('mousemove', handleMouseMove)
    document.removeEventListener('mouseup', handleMouseUp)
  }

  document.addEventListener('mousemove', handleMouseMove, { passive: true })
  document.addEventListener('mouseup', handleMouseUp, { passive: true })
}

// Saved Commands
const getAllSavedCommands = async () => {
  try {
    const response = await ApiService.getAllSavedCommands()
    savedCommands.value = response.data.data || []
    filteredCommands.value = savedCommands.value
  } catch (error) {
    console.error('Failed to load saved commands:', error)
    savedCommands.value = []
    filteredCommands.value = []
  }
}

const filterCommands = (event) => {
  const filter = event.target.value.toLowerCase()
  commandFilter.value = filter

  if (!filter) {
    filteredCommands.value = savedCommands.value
  } else {
    filteredCommands.value = savedCommands.value.filter(command =>
      command.command.toLowerCase().includes(filter) ||
      (command.description && command.description.toLowerCase().includes(filter)) ||
      (command.category && command.category.toLowerCase().includes(filter))
    )
  }

  // Reset selection when filtering
  selectedCommandIndex.value = -1
}

const selectCommand = (index) => {
  selectedCommandIndex.value = index
}

const executeSelectedCommand = () => {
  if (selectedCommandIndex.value >= 0 && selectedCommandIndex.value < filteredCommands.value.length) {
    const command = filteredCommands.value[selectedCommandIndex.value]

    if (activeTab.value && activeTab.value.socket && activeTab.value.socket.readyState === WebSocket.OPEN) {
      activeTab.value.socket.send(command.command + '\n')
    } else {
      console.warn('No active terminal or terminal not connected')
    }
  }
}

// Lifecycle
onMounted(() => {
  // Load user info
  getUserInfo()

  // Load saved commands
  getAllSavedCommands()

  // Window resize listener for responsive design
  const updateWindowWidth = () => {
    windowWidth.value = window.innerWidth
  }

  window.addEventListener('resize', updateWindowWidth)

  // Handle terminal creation from TerminalView
  const handleCreateTerminal = (event) => {
    const { containerId, name } = event.detail
    createNewTerminal(containerId, name)
  }

  window.addEventListener('create-terminal', handleCreateTerminal)

  // Handle window resize
  const handleResize = () => {
    terminalStore.tabs.forEach(tab => {
      if (tab.fitAddon && tab.terminal) {
        setTimeout(() => {
          try {
            tab.fitAddon.fit()
          } catch (error) {
            console.warn('Fit error on window resize:', error)
          }
        }, 100)
      }
    })
  }

  window.addEventListener('resize', handleResize)

  onUnmounted(() => {
    window.removeEventListener('resize', updateWindowWidth)
    window.removeEventListener('create-terminal', handleCreateTerminal)
    window.removeEventListener('resize', handleResize)

    // Clean up terminals only on real page unload
    // terminalStore.tabs.forEach(tab => {
    //   if (tab.socket) {
    //     tab.socket.close(1000) // Normal closure
    //   }
    //   if (tab.terminal) {
    //     tab.terminal.dispose()
    //   }
    // })
  })
})
</script>

<template>
  <div class="min-h-screen bg-gray-950" @click="closeMenus">
    <!-- Sidebar -->
    <div
      :class="[
        'fixed inset-y-0 left-0 z-50 w-64 bg-gray-900/95 backdrop-blur-xl transform transition-transform duration-300 ease-in-out border-r border-gray-700/50',
        sidebarOpen ? 'translate-x-0' : '-translate-x-full lg:translate-x-0'
      ]"
      :style="{
        bottom: terminalStore.isTerminalVisible ? `${terminalStore.terminalHeight}px` : '0px'
      }"
    >
      <!-- Sidebar Header -->
      <div class="flex items-center justify-between h-16 px-6 border-b border-gray-700/50">
        <div class="flex items-center space-x-3">
          <div class="w-8 h-8 bg-gradient-to-br from-blue-500 to-purple-600 rounded-lg flex items-center justify-center">
            <BaseIcon :path="mdiDocker" size="20" class="text-white" />
          </div>
          <div>
            <h1 class="text-lg font-bold text-white">Redock</h1>
            <p class="text-xs text-gray-400">DevStation</p>
          </div>
        </div>

        <button
          class="lg:hidden p-1 text-gray-400 hover:text-white rounded"
          @click="toggleSidebar"
        >
          <BaseIcon :path="mdiClose" size="20" />
        </button>
      </div>

      <!-- Navigation -->
      <nav
        class="flex-1 px-4 py-6 space-y-2 overflow-y-auto"
        :style="{
          maxHeight: terminalStore.isTerminalVisible
            ? `calc(100vh - ${terminalStore.terminalHeight}px - 12rem)`
            : 'calc(100vh - 12rem)'
        }"
      >
        <router-link
          v-for="item in navigationItems"
          :key="item.path"
          :to="item.path"
          :class="[
            'group flex items-center px-3 py-2.5 text-sm font-medium rounded-xl transition-all duration-200',
            $route.path === item.path
              ? 'bg-gradient-to-r from-blue-600 to-purple-600 text-white shadow-lg'
              : 'text-gray-300 hover:text-white hover:bg-gray-700/50'
          ]"
        >
          <BaseIcon :path="item.icon" size="20" class="mr-3" />
          <span class="flex-1">{{ item.name }}</span>
        </router-link>
      </nav>

      <!-- Sidebar Footer -->
      <div class="absolute bottom-0 left-0 right-0 p-4 border-t border-gray-700/50 bg-gray-900/95">
        <button
          class="w-full flex items-center px-3 py-2.5 text-sm font-medium text-red-400 hover:text-white hover:bg-red-600/20 rounded-xl transition-all duration-200 border border-red-600/20"
          @click="logout"
        >
          <BaseIcon :path="mdiLogout" size="20" class="mr-3" />
          <span>Logout</span>
        </button>
      </div>
    </div>

    <!-- Main Content -->
    <div class="lg:ml-64">
      <!-- Top Navigation -->
      <nav class="bg-gray-900/80 backdrop-blur-xl border-b border-gray-700/50 px-6 py-4 flex items-center justify-between h-16 sticky top-0 z-40">
        <!-- Mobile menu button -->
        <button
          class="lg:hidden p-2 text-gray-400 hover:text-white hover:bg-gray-700/50 rounded-lg transition-colors"
          @click="toggleSidebar"
        >
          <BaseIcon :path="mdiMenu" size="20" />
        </button>

        <!-- Search -->
        <div class="flex-1 max-w-md mx-auto lg:mx-4">
          <div class="relative">
            <BaseIcon
              :path="mdiMagnify"
              size="20"
              class="absolute left-3 top-1/2 transform -translate-y-1/2 text-gray-400"
            />
            <input
              type="text"
              placeholder="Search containers, environments, commands..."
              class="w-full pl-10 pr-4 py-2 bg-gray-800/50 border border-gray-600/50 rounded-xl text-white placeholder-gray-400 focus:ring-2 focus:ring-blue-500 focus:border-blue-500 backdrop-blur-sm transition-all"
            />
          </div>
        </div>

        <!-- Right side controls -->
        <div class="flex items-center space-x-4">
          <!-- Notifications -->
          <button class="relative p-2 text-gray-400 hover:text-white hover:bg-gray-700/50 rounded-lg transition-colors">
            <BaseIcon :path="mdiBell" size="20" />
            <span class="absolute -top-1 -right-1 w-3 h-3 bg-red-500 rounded-full border-2 border-gray-900"></span>
          </button>

          <!-- User menu -->
          <div class="relative" @click.stop>
            <button
              class="flex items-center space-x-2 p-2 text-gray-400 hover:text-white hover:bg-gray-700/50 rounded-lg transition-colors"
              @click="toggleUserMenu"
            >
              <div class="w-8 h-8 bg-gradient-to-br from-blue-500 to-purple-600 rounded-full flex items-center justify-center">
                <BaseIcon :path="mdiAccount" size="16" class="text-white" />
              </div>
              <span class="hidden md:block text-sm font-medium text-white">{{ userInfo.username }}</span>
              <BaseIcon :path="mdiChevronDown" size="16" :class="{ 'transform rotate-180': userMenuOpen }" class="transition-transform" />
            </button>

            <!-- User dropdown -->
            <transition
              enter-active-class="transition duration-200 ease-out"
              enter-from-class="transform scale-95 opacity-0"
              enter-to-class="transform scale-100 opacity-100"
              leave-active-class="transition duration-75 ease-in"
              leave-from-class="transform scale-100 opacity-100"
              leave-to-class="transform scale-95 opacity-0"
            >
              <div
                v-show="userMenuOpen"
                class="absolute right-0 mt-2 w-48 bg-gray-800/95 backdrop-blur-xl border border-gray-700/50 rounded-xl shadow-2xl z-50 overflow-hidden"
              >
                <div class="py-2">
                  <button
                    class="w-full flex items-center px-4 py-3 text-red-400 hover:bg-red-600/10 hover:text-red-300 transition-colors"
                    @click="logout"
                  >
                    <BaseIcon :path="mdiLogout" size="16" class="mr-3" />
                    <span>Sign out</span>
                  </button>
                </div>
              </div>
            </transition>
          </div>
        </div>
      </nav>

      <!-- Page Content -->
      <main
        :style="{
          height: terminalStore.isTerminalVisible
            ? `calc(100vh - ${terminalStore.terminalHeight}px - 4rem)`
            : 'calc(100vh - 4rem)',
          maxHeight: terminalStore.isTerminalVisible
            ? `calc(100vh - ${terminalStore.terminalHeight}px - 4rem)`
            : 'calc(100vh - 4rem)',
          paddingBottom: terminalStore.isTerminalVisible ? '0' : '1.5rem'
        }"
        class="p-6 bg-gray-950 transition-all duration-300 overflow-y-auto"
      >
        <router-view />
      </main>

      <!-- Saved Commands Panel (Left Side of Terminal) -->
      <div
        v-if="terminalStore.hasAnyTabs && terminalStore.isTerminalVisible && savedCommands.length > 0"
        :class="[
          'fixed left-0 bottom-0 w-80 bg-gray-800/95 backdrop-blur-xl border-r border-t border-gray-700/50 transition-all duration-300 z-40'
        ]"
        :style="{
          height: `${terminalStore.terminalHeight}px`
        }"
      >
        <!-- Header -->
        <div class="flex items-center justify-between p-3 border-b border-gray-700/50 bg-gray-800/80">
          <h4 class="text-sm font-semibold text-white">Saved Commands</h4>
          <span class="text-xs text-gray-400 bg-gray-700/50 px-2 py-1 rounded-full">
            {{ filteredCommands.length }}/{{ savedCommands.length }}
          </span>
        </div>

        <!-- Search/Filter -->
        <div class="p-2 border-b border-gray-700/50">
          <input
            v-model="commandFilter"
            type="text"
            placeholder="Search commands..."
            class="w-full px-3 py-1.5 text-sm bg-gray-700/50 border border-gray-600/50 rounded-lg text-white placeholder-gray-400 focus:ring-1 focus:ring-blue-500 focus:border-blue-500"
            @input="filterCommands"
          />
        </div>

        <!-- Commands List -->
        <div class="h-full overflow-y-auto pb-20">
          <div class="p-2 space-y-1">
            <div
              v-for="(command, index) in filteredCommands"
              :key="command.id"
              :class="[
                'p-3 rounded-lg cursor-pointer transition-all duration-200 group border',
                selectedCommandIndex === index
                  ? 'bg-blue-600/20 border-blue-500/40 shadow-lg'
                  : 'bg-gray-700/20 hover:bg-gray-600/30 border-gray-600/20 hover:border-gray-500/40'
              ]"
              @click="selectedCommandIndex = index"
              @dblclick="executeSelectedCommand"
            >
              <div class="space-y-2">
                <!-- Command -->
                <div class="flex items-start justify-between">
                  <code class="text-sm text-green-400 font-mono leading-relaxed break-all">
                    {{ command.command }}
                  </code>
                  <button
                    v-if="selectedCommandIndex === index"
                    class="ml-2 flex-shrink-0 px-2 py-1 text-xs bg-green-600 hover:bg-green-700 text-white rounded transition-colors"
                    @click.stop="executeSelectedCommand"
                  >
                    ▶ Run
                  </button>
                </div>

                <!-- Description -->
                <p class="text-xs text-gray-400 leading-relaxed">
                  {{ command.description || 'No description available' }}
                </p>

                <!-- Tags/Category (if available) -->
                <div v-if="command.category" class="flex items-center space-x-2">
                  <span class="text-xs text-blue-400 bg-blue-500/10 px-2 py-0.5 rounded-full">
                    {{ command.category }}
                  </span>
                </div>
              </div>
            </div>

            <!-- No results message -->
            <div v-if="filteredCommands.length === 0 && commandFilter" class="text-center py-8">
              <p class="text-gray-400 text-sm">No commands found</p>
              <p class="text-gray-500 text-xs mt-1">Try a different search term</p>
            </div>
          </div>
        </div>
      </div>

      <!-- Terminal Container (Full Width) -->
      <div
        v-if="terminalStore.hasAnyTabs"
        :class="[
          'fixed bottom-0 left-0 right-0 bg-gray-900/95 backdrop-blur-xl border-t border-gray-700/50 transition-all duration-300 z-30',
          terminalStore.isTerminalVisible ? 'translate-y-0' : 'translate-y-full'
        ]"
        :style="{
          height: terminalStore.isTerminalVisible ? `${terminalStore.terminalHeight}px` : '0px'
        }"
      >
        <!-- Terminal Header -->
        <div
          class="flex items-center px-4 py-2 bg-gray-800/80 border-b border-gray-700/50"
          :style="{ marginLeft: savedCommands.length > 0 ? '320px' : '0px' }"
        >
          <!-- Terminal Tabs with Horizontal Scroll -->
          <div class="flex-1 overflow-hidden mr-4">
            <div class="flex space-x-1 overflow-x-auto pb-1 scrollbar-thin scrollbar-thumb-gray-600 scrollbar-track-transparent">
              <div
                v-for="tab in terminalStore.tabs"
                :key="tab.id"
                :class="[
                  'flex items-center space-x-2 px-3 py-1.5 text-sm rounded-lg transition-all duration-200 cursor-pointer whitespace-nowrap flex-shrink-0',
                  tab.active
                    ? 'bg-gray-700 text-white border border-gray-600'
                    : 'text-gray-400 hover:text-white hover:bg-gray-700/50'
                ]"
                @click="switchToTerminalTab(tab.id)"
              >
                <div
:class="[
                  'w-2 h-2 rounded-full flex-shrink-0',
                  tab.connected ? 'bg-green-400' : 'bg-red-400'
                ]"></div>
                <span class="max-w-24 truncate">{{ tab.name }}</span>
                <button
                  class="p-0.5 text-gray-500 hover:text-red-400 transition-colors flex-shrink-0"
                  @click.stop="closeTerminalTab(tab.id)"
                >
                  <BaseIcon :path="mdiClose" size="12" />
                </button>
              </div>
            </div>
          </div>

          <!-- Terminal Controls (Always Visible) -->
          <div class="flex items-center space-x-2 flex-shrink-0">
            <!-- Duplicate Button -->
            <button
              class="p-1.5 text-gray-400 hover:text-white hover:bg-gray-700/50 rounded transition-colors"
              title="Duplicate Terminal"
              @click="duplicateActiveTerminal"
            >
              <BaseIcon :path="mdiContentDuplicate" size="16" />
            </button>
            <button
              class="p-1.5 text-gray-400 hover:text-white hover:bg-gray-700/50 rounded transition-colors"
              title="Minimize Terminal"
              @click="minimizeTerminal"
            >
              <BaseIcon :path="mdiMinus" size="16" />
            </button>
            <button
              class="p-1.5 text-gray-400 hover:text-white hover:bg-gray-700/50 rounded transition-colors"
              title="Maximize Terminal"
              @click="maximizeTerminal"
            >
              <BaseIcon :path="mdiWindowMaximize" size="16" />
            </button>
          </div>
        </div>

        <!-- Resize Handle -->
        <div
          class="absolute top-0 right-0 h-1 cursor-ns-resize bg-gradient-to-r from-transparent via-gray-600/50 to-transparent hover:via-blue-500/50 transition-colors"
          :style="{
            left: savedCommands.length > 0 ? '320px' : '0px'
          }"
          @mousedown="startResize"
        ></div>

        <!-- Terminal Content -->
        <div
          ref="terminalContainerRef"
          class="h-full overflow-hidden"
          :style="{
            height: 'calc(100% - 40px)',
            marginLeft: savedCommands.length > 0 ? '320px' : '0px'
          }"
        >
          <div
            v-for="tab in terminalStore.tabs"
            v-show="tab.active"
            :id="`terminal-${tab.id}`"
            :key="tab.id"
            class="w-full h-full bg-black border border-gray-700 rounded"
            :data-tab-id="tab.id"
            style="min-height: 300px;"
          ></div>
        </div>
      </div>
    </div>

    <!-- Mobile sidebar overlay -->
    <div
      v-if="sidebarOpen"
      class="fixed inset-0 z-40 bg-black/50 backdrop-blur-sm lg:hidden"
      @click="toggleSidebar"
    ></div>

    <!-- Minimized Terminal Indicator -->
    <div
      v-if="terminalStore.hasAnyTabs && !terminalStore.isTerminalVisible"
      class="fixed bottom-4 right-4 z-50 lg:right-6"
    >
      <button
        :class="[
          'flex items-center space-x-2 px-4 py-2 bg-gray-800/90 backdrop-blur-xl border border-gray-700/50 rounded-xl shadow-2xl transition-all duration-200',
          'hover:bg-gray-700/90 hover:border-gray-600/50 hover:shadow-xl'
        ]"
        @click="maximizeTerminal"
      >
        <BaseIcon :path="mdiConsole" size="20" class="text-green-400" />
        <span class="text-white text-sm font-medium">
          {{ terminalStore.tabs.length }} Terminal{{ terminalStore.tabs.length > 1 ? 's' : '' }}
        </span>
        <div class="flex space-x-1">
          <div
            v-for="tab in terminalStore.tabs.slice(0, 3)"
            :key="tab.id"
            :class="[
              'w-2 h-2 rounded-full',
              tab.connected ? 'bg-green-400' : 'bg-red-400'
            ]"
          ></div>
          <span v-if="terminalStore.tabs.length > 3" class="text-gray-400 text-xs">
            +{{ terminalStore.tabs.length - 3 }}
          </span>
        </div>
      </button>
    </div>
  </div>
</template>

<style>
/* Custom scrollbar */
::-webkit-scrollbar {
  width: 4px;
  height: 4px;
}

::-webkit-scrollbar-track {
  background: transparent;
}

::-webkit-scrollbar-thumb {
  background: rgba(107, 114, 128, 0.3);
  border-radius: 2px;
}

::-webkit-scrollbar-thumb:hover {
  background: rgba(107, 114, 128, 0.5);
}

/* Horizontal scrollbar for terminal tabs */
.scrollbar-thin::-webkit-scrollbar {
  height: 3px;
}

.scrollbar-thumb-gray-600::-webkit-scrollbar-thumb {
  background: rgba(75, 85, 99, 0.6);
  border-radius: 2px;
}

.scrollbar-thumb-gray-600::-webkit-scrollbar-thumb:hover {
  background: rgba(75, 85, 99, 0.8);
}

.scrollbar-track-transparent::-webkit-scrollbar-track {
  background: transparent;
}

/* Ultra-fast transitions */
.transition-all {
  transition-property: transform, opacity, background-color;
  transition-timing-function: cubic-bezier(0.25, 0.46, 0.45, 0.94);
  transition-duration: 150ms;
}

/* Hardware acceleration */
.transform-gpu {
  transform: translate3d(0, 0, 0);
  will-change: transform;
}

/* Fast sidebar animations */
.sidebar-enter-active,
.sidebar-leave-active {
  transition: transform 200ms cubic-bezier(0.25, 0.46, 0.45, 0.94);
}

.sidebar-enter-from {
  transform: translateX(-100%);
}

.sidebar-leave-to {
  transform: translateX(-100%);
}

/* Optimized scrolling */
.scroll-smooth {
  scroll-behavior: smooth;
  -webkit-overflow-scrolling: touch;
}

/* Active link glow effect */
.router-link-exact-active {
  position: relative;
}

.router-link-exact-active::before {
  content: '';
  position: absolute;
  inset: -2px;
  background: linear-gradient(45deg, #3b82f6, #8b5cf6);
  border-radius: 12px;
  z-index: -1;
  filter: blur(4px);
  opacity: 0.6;
}

/* Terminal specific styles */
.xterm {
  height: 100% !important;
  padding-bottom: 32px;
}

.xterm .xterm-viewport {
  overflow-y: auto;
}

.xterm-cursor-layer {
  z-index: 2;
}

.xterm-selection-layer {
  z-index: 1;
}
</style>
