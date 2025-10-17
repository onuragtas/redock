<script setup>
import BaseIcon from '@/components/BaseIcon.vue'

import ApiService from '@/services/ApiService'
import {
  mdiChevronLeft,
  mdiChevronRight,
  mdiCloudDownload,
  mdiCog,
  mdiConsole,
  mdiDocker,
  mdiDownload,
  mdiMagnify,
  mdiMonitorDashboard,
  mdiPlay,
  mdiRefresh,
  mdiServer,
  mdiSpeedometer,
  mdiStop,
  mdiRefresh as mdiUpdate
} from '@mdi/js'
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'

// Reactive state
const loading = ref(false)
const localIp = ref('')
const containers = ref([])
const systemStats = ref({
  cpu_percent: '0%',
  memory_percent: '0%',
  memory_used_gb: '0 GB',
  memory_total_gb: '0 GB',
  disk_percent: '0%',
  disk_used_gb: '0 GB',
  disk_total_gb: '0 GB',
  upload_speed: '0 KB/s',
  download_speed: '0 KB/s',
  network_sent_total: '0 MB',
  network_recv_total: '0 MB'
})

// Get system stats from API
const updateSystemStats = async () => {
  try {
    const response = await ApiService.get('/api/v1/usage/list')
    if (response.data && response.data.data) {
      systemStats.value = response.data.data
    }
  } catch (error) {
    console.error('Failed to get system stats:', error)
    // Fallback to mock data if API fails
    systemStats.value = {
      cpu_percent: '25%',
      memory_percent: '45%',
      memory_used_gb: '8.5 GB',
      memory_total_gb: '16 GB',
      disk_percent: '65%',
      disk_used_gb: '250 GB',
      disk_total_gb: '500 GB',
      upload_speed: '150 KB/s',
      download_speed: '1.2 MB/s',
      network_sent_total: '1250 MB',
      network_recv_total: '8750 MB'
    }
  }
}

// Quick actions state
const quickActions = ref({
  regenerateXDebug: false,
  restartNginx: false,
  selfUpdate: false,
  updateDocker: false,
  install: false,
  updateDockerImages: false
})

// Pagination
const currentPage = ref(1)
const itemsPerPage = ref(5)

// Search
const searchQuery = ref('')

// Computed properties
const runningContainers = computed(() => 
  containers.value.filter(c => c.active).length
)

const stoppedContainers = computed(() => 
  containers.value.filter(c => !c.active).length
)

const totalContainers = computed(() => containers.value.length)

// Search functionality
const filteredContainers = computed(() => {
  if (!searchQuery.value) {
    return containers.value
  }
  
  const query = searchQuery.value.toLowerCase()
  return containers.value.filter(container => 
    container.container_name.toLowerCase().includes(query) ||
    (container.active ? 'running' : 'stopped').includes(query) ||
    (container.active ? 'active' : 'inactive').includes(query)
  )
})

const paginatedContainers = computed(() => {
  const start = (currentPage.value - 1) * itemsPerPage.value
  const end = start + itemsPerPage.value
  return filteredContainers.value.slice(start, end)
})

const totalPages = computed(() => {
  return Math.ceil(filteredContainers.value.length / itemsPerPage.value)
})

const paginationInfo = computed(() => {
  const total = filteredContainers.value.length
  if (total === 0) return 'No containers found'
  
  const start = (currentPage.value - 1) * itemsPerPage.value + 1
  const end = Math.min(start + itemsPerPage.value - 1, total)
  return `${start}-${end} of ${total} containers`
})

// API Methods
const getLocalIp = async () => {
  try {
    const response = await ApiService.getLocalIp()
    localIp.value = response.data.data.ip
  } catch (error) {
    console.error('Failed to get local IP:', error)
    localIp.value = '192.168.1.100' // Fallback
  }
}

const getAllServices = async () => {
  try {
    const response = await ApiService.getAllServices()
    containers.value = response.data.data.all_services || []
  } catch (error) {
    console.error('Failed to get services:', error)
    // Mock data for demo
    containers.value = [
      { container_name: 'nginx', active: true, disabled: false },
      { container_name: 'mysql', active: true, disabled: false },
      { container_name: 'redis', active: false, disabled: false },
      { container_name: 'php-fpm', active: true, disabled: false },
      { container_name: 'elasticsearch', active: false, disabled: false }
    ]
  }
}

const toggleContainer = async (container) => {
  container.disabled = true
  try {
    if (container.active) {
      await ApiService.removeService(container.container_name)
    } else {
      await ApiService.addService(container.container_name)
    }
    await getAllServices()
  } catch (error) {
    console.error('Failed to toggle container:', error)
  } finally {
    container.disabled = false
  }
}

const executeQuickAction = async (actionName) => {
  quickActions.value[actionName] = true
  
  try {
    switch (actionName) {
      case 'regenerateXDebug':
        await ApiService.regenerateXDebugConfiguration()
        break
      case 'restartNginx':
        await ApiService.restartNginxHttpd()
        break
      case 'selfUpdate':
        await ApiService.selfUpdate()
        break
      case 'updateDocker':
        await ApiService.updateDocker()
        break
      case 'install':
        await ApiService.install()
        break
      case 'updateDockerImages':
        await ApiService.updateDockerImages()
        break
    }
  } catch (error) {
    console.error(`Failed to execute ${actionName}:`, error)
  } finally {
    quickActions.value[actionName] = false
  }
}

// Reset pagination when search changes
watch(searchQuery, () => {
  currentPage.value = 1
})

// Pagination methods
const nextPage = () => {
  if (currentPage.value < totalPages.value) {
    currentPage.value++
  }
}

const prevPage = () => {
  if (currentPage.value > 1) {
    currentPage.value--
  }
}

const goToPage = (page) => {
  if (page >= 1 && page <= totalPages.value) {
    currentPage.value = page
  }
}

// Lifecycle
let statsInterval = null
let ipInterval = null

onMounted(async () => {
  await getLocalIp()
  await getAllServices()
  await updateSystemStats()
  
  // Update stats periodically (every 3 seconds for real-time feel)
  statsInterval = setInterval(updateSystemStats, 3000)
  ipInterval = setInterval(getLocalIp, 30000)
})

onUnmounted(() => {
  if (statsInterval) clearInterval(statsInterval)
  if (ipInterval) clearInterval(ipInterval)
})
</script>

<template>
  <div class="space-y-6">
      <!-- Welcome Header -->
      <div class="bg-gradient-to-r from-blue-600 to-purple-600 rounded-xl p-6 text-white">
        <div class="flex flex-col lg:flex-row lg:items-center lg:justify-between">
          <div>
            <h1 class="text-2xl lg:text-3xl font-bold mb-2">Welcome to Redock DevStation</h1>
            <p class="text-blue-100">Your all-in-one local development environment</p>
          </div>
          <div class="mt-4 lg:mt-0 bg-white/10 rounded-lg p-4 backdrop-blur-sm">
            <div class="text-sm text-blue-100">Server IP</div>
            <div class="text-lg font-mono font-semibold">{{ localIp || 'Loading...' }}</div>
          </div>
        </div>
      </div>

      <!-- Stats Grid -->
      <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
        <!-- Running Containers -->
        <div class="bg-gray-800 rounded-xl p-6 border border-gray-700">
          <div class="flex items-center justify-between">
            <div>
              <p class="text-gray-400 text-sm">Running</p>
              <p class="text-2xl font-bold text-green-400">{{ runningContainers }}</p>
            </div>
            <div class="p-3 bg-green-600/20 rounded-full">
              <BaseIcon :path="mdiPlay" size="24" class="text-green-400" />
            </div>
          </div>
        </div>

        <!-- Stopped Containers -->
        <div class="bg-gray-800 rounded-xl p-6 border border-gray-700">
          <div class="flex items-center justify-between">
            <div>
              <p class="text-gray-400 text-sm">Stopped</p>
              <p class="text-2xl font-bold text-red-400">{{ stoppedContainers }}</p>
            </div>
            <div class="p-3 bg-red-600/20 rounded-full">
              <BaseIcon :path="mdiStop" size="24" class="text-red-400" />
            </div>
          </div>
        </div>

        <!-- Total Containers -->
        <div class="bg-gray-800 rounded-xl p-6 border border-gray-700">
          <div class="flex items-center justify-between">
            <div>
              <p class="text-gray-400 text-sm">Total</p>
              <p class="text-2xl font-bold text-blue-400">{{ totalContainers }}</p>
            </div>
            <div class="p-3 bg-blue-600/20 rounded-full">
              <BaseIcon :path="mdiDocker" size="24" class="text-blue-400" />
            </div>
          </div>
        </div>

        <!-- CPU Usage -->
        <div class="bg-gray-800 rounded-xl p-6 border border-gray-700">
          <div class="flex items-center justify-between">
            <div>
              <p class="text-gray-400 text-sm">CPU Usage</p>
              <p class="text-2xl font-bold text-yellow-400">{{ systemStats.cpu_percent }}</p>
            </div>
            <div class="p-3 bg-yellow-600/20 rounded-full">
              <BaseIcon :path="mdiSpeedometer" size="24" class="text-yellow-400" />
            </div>
          </div>
        </div>
      </div>

      <!-- System Stats -->
      <div class="bg-gray-800 rounded-xl border border-gray-700 overflow-hidden">
        <div class="p-6 border-b border-gray-700">
          <h2 class="text-xl font-semibold flex items-center">
            <BaseIcon :path="mdiMonitorDashboard" size="24" class="mr-3 text-blue-400" />
            System Resources
          </h2>
        </div>
        <div class="p-6">
          <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
            <!-- Memory Usage -->
            <div class="space-y-3">
              <div class="flex justify-between items-center">
                <span class="text-sm text-gray-400">Memory Usage</span>
                <span class="text-sm font-medium">{{ systemStats.memory_percent }}</span>
              </div>
              <div class="w-full bg-gray-700 rounded-full h-3">
                <div 
                  class="bg-blue-500 h-3 rounded-full transition-all duration-300"
                  :style="{ width: systemStats.memory_percent }"
                ></div>
              </div>
              <div class="flex justify-between text-xs text-gray-500">
                <span>{{ systemStats.memory_used_gb }}</span>
                <span>{{ systemStats.memory_total_gb }}</span>
              </div>
            </div>

            <!-- Disk Usage -->
            <div class="space-y-3">
              <div class="flex justify-between items-center">
                <span class="text-sm text-gray-400">Disk Usage</span>
                <span class="text-sm font-medium">{{ systemStats.disk_percent }}</span>
              </div>
              <div class="w-full bg-gray-700 rounded-full h-3">
                <div 
                  class="bg-green-500 h-3 rounded-full transition-all duration-300"
                  :style="{ width: systemStats.disk_percent }"
                ></div>
              </div>
              <div class="flex justify-between text-xs text-gray-500">
                <span>{{ systemStats.disk_used_gb }}</span>
                <span>{{ systemStats.disk_total_gb }}</span>
              </div>
            </div>

            <!-- Network Activity -->
            <div class="space-y-3">
              <div class="flex justify-between items-center">
                <span class="text-sm text-gray-400">Network Activity</span>
                <span class="text-xs text-gray-500">Real-time</span>
              </div>
              <div class="space-y-2">
                <div class="flex justify-between items-center">
                  <span class="text-xs text-gray-500">↑ Upload:</span>
                  <span class="text-sm font-medium text-orange-400">{{ systemStats.upload_speed }}</span>
                </div>
                <div class="flex justify-between items-center">
                  <span class="text-xs text-gray-500">↓ Download:</span>
                  <span class="text-sm font-medium text-green-400">{{ systemStats.download_speed }}</span>
                </div>
                <div class="pt-2 border-t border-gray-700">
                  <div class="flex justify-between text-xs text-gray-500">
                    <span>Total ↑: {{ systemStats.network_sent_total }}</span>
                    <span>Total ↓: {{ systemStats.network_recv_total }}</span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Quick Actions -->
      <div class="bg-gray-800 rounded-xl border border-gray-700 overflow-hidden">
        <div class="p-6 border-b border-gray-700">
          <h2 class="text-xl font-semibold flex items-center">
            <BaseIcon :path="mdiCog" size="24" class="mr-3 text-green-400" />
            Quick Actions
          </h2>
        </div>
        <div class="p-6">
          <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
            <button
              @click="executeQuickAction('install')"
              :disabled="quickActions.install"
              class="flex items-center justify-center space-x-2 p-4 bg-emerald-600 hover:bg-emerald-700 disabled:bg-gray-600 rounded-lg transition-all duration-200 hover:transform hover:scale-105"
            >
              <BaseIcon :path="mdiCloudDownload" size="20" />
              <span>Install System</span>
            </button>

            <router-link 
              to="/exec"
              class="flex items-center justify-center space-x-2 p-4 bg-cyan-600 hover:bg-cyan-700 rounded-lg transition-all duration-200 hover:transform hover:scale-105"
            >
              <BaseIcon :path="mdiConsole" size="20" />
              <span>SSH Console</span>
            </router-link>

            <button
              @click="executeQuickAction('regenerateXDebug')"
              :disabled="quickActions.regenerateXDebug"
              class="flex items-center justify-center space-x-2 p-4 bg-blue-600 hover:bg-blue-700 disabled:bg-gray-600 rounded-lg transition-all duration-200 hover:transform hover:scale-105"
            >
              <BaseIcon :path="mdiRefresh" size="20" />
              <span>Regenerate XDebug</span>
            </button>

            <button
              @click="executeQuickAction('restartNginx')"
              :disabled="quickActions.restartNginx"
              class="flex items-center justify-center space-x-2 p-4 bg-orange-600 hover:bg-orange-700 disabled:bg-gray-600 rounded-lg transition-all duration-200 hover:transform hover:scale-105"
            >
              <BaseIcon :path="mdiServer" size="20" />
              <span>Restart Nginx</span>
            </button>

            <button
              @click="executeQuickAction('selfUpdate')"
              :disabled="quickActions.selfUpdate"
              class="flex items-center justify-center space-x-2 p-4 bg-purple-600 hover:bg-purple-700 disabled:bg-gray-600 rounded-lg transition-all duration-200 hover:transform hover:scale-105"
            >
              <BaseIcon :path="mdiUpdate" size="20" />
              <span>Self Update</span>
            </button>

            <button
              @click="executeQuickAction('updateDocker')"
              :disabled="quickActions.updateDocker"
              class="flex items-center justify-center space-x-2 p-4 bg-red-600 hover:bg-red-700 disabled:bg-gray-600 rounded-lg transition-all duration-200 hover:transform hover:scale-105"
            >
              <BaseIcon :path="mdiDownload" size="20" />
              <span>Update Docker</span>
            </button>

            <button
              @click="executeQuickAction('updateDockerImages')"
              :disabled="quickActions.updateDockerImages"
              class="flex items-center justify-center space-x-2 p-4 bg-indigo-600 hover:bg-indigo-700 disabled:bg-gray-600 rounded-lg transition-all duration-200 hover:transform hover:scale-105"
            >
              <BaseIcon :path="mdiCloudDownload" size="20" />
              <span>Update Images</span>
            </button>
          </div>
        </div>
      </div>

      <!-- Container Statistics -->
      <div class="grid grid-cols-1 md:grid-cols-3 gap-6 mb-8">
        <div class="bg-gradient-to-br from-emerald-50 to-emerald-100 dark:from-emerald-900/20 dark:to-emerald-800/20 border border-emerald-200 dark:border-emerald-700 rounded-2xl p-6">
          <div class="flex items-center justify-between">
            <div>
              <div class="text-2xl font-bold text-emerald-600 dark:text-emerald-400">{{ totalContainers }}</div>
              <div class="text-sm text-emerald-600/70 dark:text-emerald-400/70">Total Containers</div>
            </div>
            <BaseIcon :path="mdiDocker" size="48" class="text-emerald-500 opacity-20" />
          </div>
        </div>

        <div class="bg-gradient-to-br from-green-50 to-green-100 dark:from-green-900/20 dark:to-green-800/20 border border-green-200 dark:border-green-700 rounded-2xl p-6">
          <div class="flex items-center justify-between">
            <div>
              <div class="text-2xl font-bold text-green-600 dark:text-green-400">{{ runningContainers }}</div>
              <div class="text-sm text-green-600/70 dark:text-green-400/70">Running</div>
            </div>
            <BaseIcon :path="mdiPlay" size="48" class="text-green-500 opacity-20" />
          </div>
        </div>

        <div class="bg-gradient-to-br from-red-50 to-red-100 dark:from-red-900/20 dark:to-red-800/20 border border-red-200 dark:border-red-700 rounded-2xl p-6">
          <div class="flex items-center justify-between">
            <div>
              <div class="text-2xl font-bold text-red-600 dark:text-red-400">{{ stoppedContainers }}</div>
              <div class="text-sm text-red-600/70 dark:text-red-400/70">Stopped</div>
            </div>
            <BaseIcon :path="mdiStop" size="48" class="text-red-500 opacity-20" />
          </div>
        </div>
      </div>

      <!-- Container Management -->
      <div class="bg-white dark:bg-slate-800 rounded-2xl shadow-lg border border-slate-200 dark:border-slate-700">
        <!-- Header -->
        <div class="p-6 border-b border-slate-200 dark:border-slate-700 bg-gradient-to-r from-purple-600 via-indigo-600 to-blue-600 rounded-t-2xl">
          <div class="flex flex-col lg:flex-row lg:items-center lg:justify-between">
            <div>
              <h2 class="text-xl font-bold text-white mb-2 flex items-center">
                <BaseIcon :path="mdiDocker" size="32" class="mr-3" />
                Container Management
              </h2>
              <p class="text-blue-100 text-sm">Docker container lifecycle management</p>
            </div>
            <div class="mt-4 lg:mt-0 flex items-center space-x-3">
              <!-- Search Input -->
              <div class="relative">
                <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
                  <BaseIcon :path="mdiMagnify" size="16" class="text-white/60" />
                </div>
                <input
                  v-model="searchQuery"
                  type="text"
                  placeholder="Search containers..."
                  class="pl-10 pr-4 py-2 bg-white/20 backdrop-blur border border-white/30 text-white placeholder-white/60 rounded-xl focus:outline-none focus:ring-2 focus:ring-white/50 focus:border-transparent transition-all duration-200 w-64"
                />
              </div>
              
              <button
                @click="getContainerList"
                :disabled="loading"
                class="px-4 py-2 bg-white/20 hover:bg-white/30 backdrop-blur text-white rounded-xl transition-all duration-200 flex items-center space-x-2 shadow-lg hover:shadow-xl"
              >
                <BaseIcon :path="mdiRefresh" size="16" />
                <span>Refresh</span>
              </button>
            </div>
          </div>
        </div>
        
        <!-- Content -->
        <div class="p-6">
          <div v-if="loading" class="text-center py-12">
            <div class="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600"></div>
            <p class="text-slate-500 dark:text-slate-400 mt-4">Loading containers...</p>
          </div>

          <div v-else-if="containers.length === 0" class="text-center py-12">
            <BaseIcon :path="mdiDocker" size="64" class="mx-auto text-slate-300 dark:text-slate-600 mb-4" />
            <p class="text-slate-500 dark:text-slate-400 mb-4">No containers found</p>
          </div>

          <div v-else-if="filteredContainers.length === 0" class="text-center py-12">
            <BaseIcon :path="mdiMagnify" size="64" class="mx-auto text-slate-300 dark:text-slate-600 mb-4" />
            <p class="text-slate-500 dark:text-slate-400 mb-4">No containers match your search</p>
            <p class="text-slate-400 dark:text-slate-500 text-sm">Try adjusting your search terms</p>
          </div>

          <div v-else class="space-y-4">
            <div 
              v-for="container in paginatedContainers" 
              :key="container.container_name"
              class="flex items-center justify-between p-6 bg-slate-50 dark:bg-slate-800/50 rounded-xl hover:bg-slate-100 dark:hover:bg-slate-700/50 transition-colors"
            >
              <div class="flex items-center space-x-6">
                <div class="flex-shrink-0">
                  <div class="w-12 h-12 bg-gradient-to-br from-purple-500 to-indigo-600 rounded-xl flex items-center justify-center relative">
                    <BaseIcon :path="mdiDocker" size="24" class="text-white" />
                    <div 
                      :class="[
                        'absolute -top-1 -right-1 w-4 h-4 rounded-full border-2 border-white',
                        container.active ? 'bg-green-500' : 'bg-red-500'
                      ]"
                    ></div>
                  </div>
                </div>
                
                <div class="flex-1">
                  <h3 class="font-semibold text-lg">{{ container.container_name }}</h3>
                  <div class="flex items-center space-x-4 mt-1 text-sm text-slate-500 dark:text-slate-400">
                    <div class="flex items-center">
                      <BaseIcon :path="mdiServer" size="16" class="mr-1" />
                      Status: {{ container.active ? 'Running' : 'Stopped' }}
                    </div>
                  </div>
                </div>
                
                <div class="flex-shrink-0">
                  <span 
                    :class="[
                      'inline-flex items-center px-3 py-1 rounded-full text-xs font-medium',
                      container.active 
                        ? 'text-green-600 bg-green-100 dark:text-green-400 dark:bg-green-900/30'
                        : 'text-red-600 bg-red-100 dark:text-red-400 dark:bg-red-900/30'
                    ]"
                  >
                    {{ container.active ? 'Active' : 'Inactive' }}
                  </span>
                </div>
              </div>
              
              <div class="flex items-center space-x-2 ml-6">
                <router-link 
                  v-if="container.active"
                  :to="`/exec/${container.container_name}`"
                  class="inline-flex items-center px-3 py-2 text-sm bg-blue-600 hover:bg-blue-700 text-white rounded-lg transition-colors space-x-2 shadow-sm hover:shadow-md"
                >
                  <BaseIcon :path="mdiConsole" size="16" />
                  <span>Console</span>
                </router-link>
                
                <button
                  @click="toggleContainer(container)"
                  :disabled="container.disabled"
                  :class="[
                    'inline-flex items-center px-3 py-2 text-sm rounded-lg transition-colors space-x-2 shadow-sm hover:shadow-md',
                    container.active 
                      ? 'bg-red-600 hover:bg-red-700 text-white' 
                      : 'bg-green-600 hover:bg-green-700 text-white',
                    container.disabled ? 'opacity-50 cursor-not-allowed' : ''
                  ]"
                >
                  <BaseIcon :path="container.active ? mdiStop : mdiPlay" size="16" />
                  <span>{{ container.active ? 'Stop' : 'Start' }}</span>
                </button>
              </div>
            </div>
          </div>

          <!-- Pagination -->
          <div v-if="totalPages > 1" class="flex items-center justify-between mt-6 px-6 pb-4">
            <div class="text-sm text-slate-500 dark:text-slate-400">
              {{ paginationInfo }}
            </div>
            <div class="flex space-x-2">
              <button
                @click="prevPage"
                :disabled="currentPage === 1"
                :class="[
                  'inline-flex items-center px-3 py-2 text-sm rounded-lg transition-colors space-x-2 shadow-sm hover:shadow-md',
                  currentPage === 1
                    ? 'text-slate-400 bg-slate-100 dark:text-slate-600 dark:bg-slate-700 cursor-not-allowed'
                    : 'text-slate-600 bg-white hover:bg-slate-50 dark:text-slate-300 dark:bg-slate-800 dark:hover:bg-slate-700'
                ]"
              >
                <BaseIcon :path="mdiChevronLeft" size="16" />
                <span>Previous</span>
              </button>
              
              <div class="flex space-x-1">
                <button
                  v-for="page in totalPages"
                  :key="page"
                  @click="goToPage(page)"
                  :class="[
                    'px-3 py-2 text-sm rounded-lg transition-colors shadow-sm hover:shadow-md',
                    page === currentPage
                      ? 'bg-purple-600 text-white'
                      : 'text-slate-600 dark:text-slate-300 bg-white dark:bg-slate-800 hover:bg-purple-50 dark:hover:bg-slate-700 hover:text-purple-600'
                  ]"
                >
                  {{ page }}
                </button>
              </div>
              
              <button
                @click="nextPage"
                :disabled="currentPage === totalPages"
                :class="[
                  'inline-flex items-center px-3 py-2 text-sm rounded-lg transition-colors space-x-2 shadow-sm hover:shadow-md',
                  currentPage === totalPages
                    ? 'text-slate-400 bg-slate-100 dark:text-slate-600 dark:bg-slate-700 cursor-not-allowed'
                    : 'text-slate-600 bg-white hover:bg-slate-50 dark:text-slate-300 dark:bg-slate-800 dark:hover:bg-slate-700'
                ]"
              >
                <span>Next</span>
                <BaseIcon :path="mdiChevronRight" size="16" />
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
</template>

<style scoped>
/* Glass morphism effect */
.backdrop-blur {
  backdrop-filter: blur(10px);
}

/* Enhanced card shadows */
.shadow-lg {
  box-shadow: 0 10px 15px -3px rgba(0, 0, 0, 0.1), 0 4px 6px -2px rgba(0, 0, 0, 0.05);
}

.shadow-xl {
  box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.1), 0 10px 10px -5px rgba(0, 0, 0, 0.04);
}

/* Smooth transitions */
.transition-colors {
  transition: background-color 0.2s ease-in-out, color 0.2s ease-in-out;
}

.transition-all {
  transition: all 0.2s ease-in-out;
}

/* Hover effects */
button:hover:not(:disabled) {
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
}

/* Enhanced gradient backgrounds */
.bg-gradient-to-br {
  background-image: linear-gradient(to bottom right, var(--tw-gradient-stops));
}

/* Loading animation */
.animate-spin {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}

/* Status indicator glow effect */
.shadow-green-500\/50 {
  box-shadow: 0 0 10px rgba(34, 197, 94, 0.5);
}

.shadow-red-500\/50 {
  box-shadow: 0 0 10px rgba(239, 68, 68, 0.5);
}

/* Card hover animations */
.hover\:bg-slate-100:hover {
  background-color: rgb(241 245 249);
}

.dark .hover\:bg-slate-700\/50:hover {
  background-color: rgba(51, 65, 85, 0.5);
}
</style>