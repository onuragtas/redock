<script setup>
import BaseButton from "@/components/BaseButton.vue";
import BaseIcon from "@/components/BaseIcon.vue";
import CardBox from "@/components/CardBox.vue";
import CardBoxModal from "@/components/CardBoxModal.vue";
import FormControl from "@/components/FormControl.vue";
import FormField from "@/components/FormField.vue";
import SectionTitleLineWithButton from "@/components/SectionTitleLineWithButton.vue";
import { useLayoutToggle } from "@/composables/useLayoutToggle";
import { usePaginationFilter } from "@/composables/usePaginationFilter";

import ApiService from "@/services/ApiService";
import {
  mdiAccount,
  mdiAccountPlus,
  mdiCheckCircle,
  mdiChevronLeft, mdiChevronRight,
  mdiCloseCircle,
  mdiConnection,
  mdiDelete,
  mdiEarth,
  mdiEmail,
  mdiEthernet,
  mdiLan,
  mdiLock,
  mdiLogin, mdiLogout,
  mdiMagnify,
  mdiPlay,
  mdiPlus,
  mdiRefresh,
  mdiServer,
  mdiStop,
  mdiTunnel,
  mdiViewGridOutline,
  mdiViewList
} from "@mdi/js";
import { computed, onMounted, ref } from "vue";

// Reactive state
const login = ref(false)
const proxies = ref([])
const isAddModalActive = ref(false)
const isStartModalActive = ref(false)
const isRegisterModalActive = ref(false)
const isDeleteModalActive = ref(false)
const loading = ref(false)
const addLoading = ref(false)
const selectedTunnel = ref(null)
const startDomain = ref({})

// Form data
const credentials = ref({
  username: '',
  password: '',
  email: ''
})

const create = ref({
  keep_alive: 0
})

const start = ref({
  localIp: '127.0.0.1',
  destinationIp: '127.0.0.1',
  localPort: 80,
})

// Computed
const tunnelStats = computed(() => {
  const total = proxies.value.length
  const active = proxies.value.filter(proxy => proxy.started).length
  
  return { total, active, inactive: total - active }
})

const {
  searchQuery,
  filteredItems,
  paginatedItems,
  currentPage,
  totalPages,
  paginationInfo,
  pages,
  nextPage,
  prevPage,
  goToPage
} = usePaginationFilter(proxies, undefined, 8)

// Methods
const checkLogin = async () => {
  try {
    const response = await ApiService.checkLogin()
    login.value = response.data.data.login
    if (login.value) {
      await tunnelList()
    }
  } catch (error) {
    console.error('Login check failed:', error)
    login.value = false
  }
}

const loginSubmit = async () => {
  try {
    const response = await ApiService.tunnelLogin(credentials.value.username, credentials.value.password)
    login.value = response.data.data.login
    if (login.value) {
      await tunnelList()
    }
  } catch (error) {
    console.error('Login failed:', error)
  }
}

const registerSubmit = async () => {
  try {
    const response = await ApiService.tunnelRegister(credentials.value.email, credentials.value.username, credentials.value.password)
    login.value = response.data.data.login
    isRegisterModalActive.value = false
    if (login.value) {
      await tunnelList()
    }
  } catch (error) {
    console.error('Registration failed:', error)
  }
}

const logoutSubmit = async () => {
  try {
    await ApiService.tunnelLogout()
    login.value = false
    proxies.value = []
    credentials.value = { username: '', password: '', email: '' }
    create.value = { keep_alive: 0 }
    start.value = { localIp: '127.0.0.1', destinationIp: '127.0.0.1', localPort: 80 }
    selectedTunnel.value = null
    isAddModalActive.value = false
    isStartModalActive.value = false
    isDeleteModalActive.value = false
  } catch (error) {
    console.error('Logout failed:', error)
  }
}

const tunnelList = async () => {
  
 loading.value = true
  try {
    const response = await ApiService.tunnelList()
    proxies.value = response.data.data || []
  } catch (error) {
    console.error('Failed to load tunnel list:', error)
  } finally {
    loading.value = false
  }
      
}

const deleteModal = (tunnel) => {
  selectedTunnel.value = tunnel
  isDeleteModalActive.value = true
}

const deleteSubmit = async () => {
  if (!selectedTunnel.value) return
  
  try {
    await ApiService.tunnelDelete(selectedTunnel.value)
    isDeleteModalActive.value = false
    selectedTunnel.value = null
    await tunnelList()
  } catch (error) {
    console.error('Failed to delete tunnel:', error)
  }
}

const addSubmit = async () => {
  if (addLoading.value) return // Prevent multiple clicks
  
  addLoading.value = true
  try {
    await ApiService.tunnelCreate(create.value)
    // Success - now we can close the modal
    await tunnelList()
    resetCreateForm()
    isAddModalActive.value = false
  } catch (error) {
    console.error('Failed to create tunnel:', error)
    // Keep modal open on error
  } finally {
    addLoading.value = false
  }
}

const startModal = (data) => {
  startDomain.value = data
  isStartModalActive.value = true
}

const startSubmit = async () => {
  try {
    const data = {
      DomainId: startDomain.value.id,
      Domain: startDomain.value.domain,
      LocalIp: start.value.localIp,
      DestinationIp: start.value.destinationIp,
      LocalPort: parseInt(start.value.localPort)
    }
    
    await ApiService.tunnelStart(data)
    isStartModalActive.value = false
    setTimeout(() => {
      tunnelList()
    }, 2000)
  } catch (error) {
    console.error('Failed to start tunnel:', error)
  }
}

const stopModal = async (item) => {
  try {
    const data = {
      DomainId: item.id,
      Domain: item.domain,
    }
    
    await ApiService.tunnelStop(data)
    setTimeout(() => {
      tunnelList()
    }, 2000)
  } catch (error) {
    console.error('Failed to stop tunnel:', error)
  }
}

const resetCreateForm = () => {
  create.value = {
    keep_alive: 0
  }
}

const getStatusColor = (started) => {
  return started 
    ? 'text-green-600 bg-green-100 dark:text-green-400 dark:bg-green-900/30'
    : 'text-gray-600 bg-gray-100 dark:text-gray-400 dark:bg-gray-900/30'
}

const formatDate = (dateString) => {
  return new Date(dateString).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit'
  })
}

const GRID_MIN_ITEMS = 2

const {
  isGridLayout,
  layoutClass,
  toggleLayout
} = useLayoutToggle(paginatedItems, { minItemsForGrid: GRID_MIN_ITEMS })

const layoutToggleLabel = computed(() => isGridLayout.value ? 'List View' : 'Grid View')
const layoutToggleIcon = computed(() => isGridLayout.value ? mdiViewList : mdiViewGridOutline)

// Pagination methods
// Lifecycle
onMounted(() => {
  checkLogin()
})
</script>

<template>
  <!-- Authenticated View -->
  <div v-if="login" class="space-y-8">
      <!-- Header -->
      <div class="bg-gradient-to-r from-purple-600 via-indigo-600 to-blue-600 rounded-2xl p-8 text-white shadow-lg">
        <div class="flex flex-col lg:flex-row lg:items-center lg:justify-between">
          <div>
            <h1 class="text-3xl lg:text-4xl font-bold mb-2 flex items-center">
              <BaseIcon :path="mdiTunnel" size="40" class="mr-4" />
              Tunnel Proxy Manager
            </h1>
            <p class="text-purple-100 text-lg">Secure tunneling and domain management</p>
          </div>
          <div class="mt-6 lg:mt-0 flex space-x-3">
            <BaseButton
              label="Refresh"
              :icon="mdiRefresh"
              color="white"
              outline
              :disabled="loading"
              class="shadow-lg hover:shadow-xl transform hover:scale-105 transition-all duration-200"
              @click="tunnelList"
            />
            <BaseButton
              label="Add Domain"
              :icon="mdiPlus"
              color="white"
              class="shadow-lg hover:shadow-xl transform hover:scale-105 transition-all duration-200"
              @click="isAddModalActive = true"
            />
            <BaseButton
              label="Logout"
              :icon="mdiLogout"
              color="white"
              outline
              class="shadow-lg hover:shadow-xl transform hover:scale-105 transition-all duration-200"
              @click="logoutSubmit"
            />
          </div>
        </div>
      </div>

      <!-- Statistics -->
      <div class="grid grid-cols-1 md:grid-cols-3 gap-6">
        <CardBox class="bg-gradient-to-br from-purple-50 to-purple-100 dark:from-purple-900/20 dark:to-purple-800/20 border-purple-200 dark:border-purple-700">
          <div class="flex items-center justify-between">
            <div>
              <div class="text-2xl font-bold text-purple-600 dark:text-purple-400">{{ tunnelStats.total }}</div>
              <div class="text-sm text-purple-600/70 dark:text-purple-400/70">Total Tunnels</div>
            </div>
            <BaseIcon :path="mdiServer" size="48" class="text-purple-500 opacity-20" />
          </div>
        </CardBox>

        <CardBox class="bg-gradient-to-br from-green-50 to-green-100 dark:from-green-900/20 dark:to-green-800/20 border-green-200 dark:border-green-700">
          <div class="flex items-center justify-between">
            <div>
              <div class="text-2xl font-bold text-green-600 dark:text-green-400">{{ tunnelStats.active }}</div>
              <div class="text-sm text-green-600/70 dark:text-green-400/70">Active Tunnels</div>
            </div>
            <BaseIcon :path="mdiPlay" size="48" class="text-green-500 opacity-20" />
          </div>
        </CardBox>

        <CardBox class="bg-gradient-to-br from-gray-50 to-gray-100 dark:from-gray-900/20 dark:to-gray-800/20 border-gray-200 dark:border-gray-700">
          <div class="flex items-center justify-between">
            <div>
              <div class="text-2xl font-bold text-gray-600 dark:text-gray-400">{{ tunnelStats.inactive }}</div>
              <div class="text-sm text-gray-600/70 dark:text-gray-400/70">Inactive Tunnels</div>
            </div>
            <BaseIcon :path="mdiStop" size="48" class="text-gray-500 opacity-20" />
          </div>
        </CardBox>
      </div>

      <!-- Tunnel List -->
      <CardBox>
        <SectionTitleLineWithButton :icon="mdiConnection" title="Active Tunnel Domains" main>
          <div class="flex flex-col gap-3 md:flex-row md:items-center">
            <div class="w-full md:w-64">
              <FormControl
                v-model="searchQuery"
                :icon="mdiMagnify"
                placeholder="Search tunnels"
              />
            </div>
            <BaseButton
              :icon="layoutToggleIcon"
              :label="layoutToggleLabel"
              color="lightDark"
              outline
              class="shrink-0"
              @click="toggleLayout"
            />
            <BaseButton
              :icon="mdiRefresh"
              color="info"
              rounded-full
              :disabled="loading"
              class="shadow-sm hover:shadow-md"
              @click="tunnelList"
            />
          </div>
        </SectionTitleLineWithButton>

        <div v-if="loading" class="text-center py-12">
          <div class="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-purple-600"></div>
          <p class="text-slate-500 dark:text-slate-400 mt-4">Loading tunnels...</p>
        </div>

        <div v-else-if="filteredItems.length === 0" class="text-center py-12">
          <BaseIcon :path="mdiTunnel" size="64" class="mx-auto text-slate-300 dark:text-slate-600 mb-4" />
          <p class="text-slate-500 dark:text-slate-400 mb-4">
            {{ searchQuery ? 'No tunnels match your search.' : 'No tunnel domains configured.' }}
          </p>
          <BaseButton
            v-if="!searchQuery"
            label="Create Your First Tunnel"
            :icon="mdiPlus"
            color="info"
            @click="isAddModalActive = true"
          />
        </div>

  <div v-else :class="layoutClass">
          <div 
            v-for="tunnel in paginatedItems" 
            :key="tunnel.id"
            class="p-6 bg-slate-50 dark:bg-slate-800/50 rounded-xl hover:bg-slate-100 dark:hover:bg-slate-700/50 transition-colors flex flex-col h-full"
          >
            <div class="flex flex-col gap-6 lg:flex-row lg:items-start lg:justify-between">
              <div class="flex items-start gap-4 flex-1">
                <div class="flex-shrink-0">
                  <div class="w-12 h-12 bg-gradient-to-br from-purple-500 to-indigo-600 rounded-xl flex items-center justify-center">
                    <BaseIcon :path="mdiTunnel" size="24" class="text-white" />
                  </div>
                </div>
                <div class="space-y-2 flex-1">
                  <h3 class="font-semibold text-lg flex items-center">
                    <BaseIcon :path="mdiEarth" size="20" class="mr-2 text-blue-500" />
                    {{ tunnel.domain }}
                  </h3>
                  <div class="flex flex-wrap items-center gap-x-4 gap-y-2 text-sm text-slate-500 dark:text-slate-400">
                    <div class="flex items-center">
                      <BaseIcon :path="mdiEthernet" size="16" class="mr-1" />
                      Port: {{ tunnel.port }}
                    </div>
                    <div class="flex items-center">
                      <BaseIcon 
                        :path="tunnel.keep_alive ? mdiCheckCircle : mdiCloseCircle" 
                        size="16" 
                        class="mr-1"
                        :class="tunnel.keep_alive ? 'text-green-500' : 'text-red-500'"
                      />
                      Keep Alive: {{ tunnel.keep_alive ? 'Yes' : 'No' }}
                    </div>
                    <div>Updated: {{ formatDate(tunnel.UpdatedAt) }}</div>
                  </div>
                </div>
              </div>
              <div class="flex items-start lg:flex-none justify-start lg:justify-end">
                <span 
                  :class="[
                    'inline-flex items-center px-3 py-1 rounded-full text-xs font-medium',
                    getStatusColor(tunnel.started)
                  ]"
                >
                  {{ tunnel.started ? 'Active' : 'Inactive' }}
                </span>
              </div>
            </div>
            <div class="mt-6 flex flex-wrap items-center justify-end gap-2">
              <BaseButton 
                :icon="tunnel.started ? mdiStop : mdiPlay" 
                :color="tunnel.started ? 'danger' : 'success'"
                small
                :title="tunnel.started ? 'Stop Tunnel' : 'Start Tunnel'"
                @click="tunnel.started ? stopModal(tunnel) : startModal(tunnel)"
              />
              <BaseButton 
                :icon="mdiDelete" 
                color="danger"
                small
                title="Delete"
                @click="deleteModal(tunnel)"
              />
            </div>
          </div>
        </div>

        <!-- Pagination -->
        <div v-if="filteredItems.length > 0" class="flex flex-col gap-4 md:flex-row md:items-center md:justify-between mt-6 px-6 pb-4">
          <div class="text-sm text-slate-500 dark:text-slate-400">
            Showing {{ paginationInfo }}
          </div>
          <div class="flex items-center gap-2">
            <BaseButton
              :icon="mdiChevronLeft"
              color="lightDark"
              small
              :disabled="currentPage === 1"
              @click="prevPage"
            />
            <div class="flex flex-wrap gap-1">
              <BaseButton
                v-for="page in pages"
                :key="page"
                :label="page"
                color="lightDark"
                small
                :active="page === currentPage"
                @click="goToPage(page)"
              />
            </div>
            <BaseButton
              :icon="mdiChevronRight"
              color="lightDark"
              small
              :disabled="currentPage === totalPages"
              @click="nextPage"
            />
          </div>
        </div>
      </CardBox>
    </div>

    <!-- Login View -->
    <div v-else-if="!isRegisterModalActive" class="min-h-screen flex items-center justify-center bg-gradient-to-br from-blue-900 via-purple-900 to-indigo-900">
      <div class="w-full max-w-md">
        <CardBox class="shadow-2xl border-0">
          <div class="text-center mb-8">
            <div class="w-20 h-20 bg-gradient-to-br from-purple-500 to-indigo-600 rounded-2xl flex items-center justify-center mx-auto mb-4">
              <BaseIcon :path="mdiTunnel" size="32" class="text-white" />
            </div>
            <h2 class="text-3xl font-bold text-slate-800 dark:text-white mb-2">Tunnel Login</h2>
            <p class="text-slate-600 dark:text-slate-400">Access your secure tunnel dashboard</p>
          </div>

          <form class="space-y-6" @submit.prevent="loginSubmit">
            <FormField label="Username">
              <div class="relative">
                <div class="absolute inset-y-0 left-0 pl-3 flex items-center">
                  <BaseIcon :path="mdiAccount" size="20" class="text-slate-400" />
                </div>
                <FormControl
                  v-model="credentials.username"
                  placeholder="Enter your username"
                  required
                  class="pl-10"
                />
              </div>
            </FormField>

            <FormField label="Password">
              <div class="relative">
                <div class="absolute inset-y-0 left-0 pl-3 flex items-center">
                  <BaseIcon :path="mdiLock" size="20" class="text-slate-400" />
                </div>
                <FormControl
                  v-model="credentials.password"
                  type="password"
                  placeholder="Enter your password"
                  required
                  class="pl-10"
                />
              </div>
            </FormField>

            <div class="space-y-3">
              <BaseButton
                type="submit"
                :icon="mdiLogin"
                label="Sign In"
                color="info"
                class="w-full justify-center py-3 text-lg font-semibold"
              />
              
              <BaseButton
                :icon="mdiAccountPlus"
                label="Create Account"
                color="info"
                outline
                class="w-full justify-center py-2"
                @click="isRegisterModalActive = true"
              />
            </div>
          </form>
        </CardBox>
      </div>
    </div>

    <!-- Register View -->
    <div v-else class="min-h-screen flex items-center justify-center bg-gradient-to-br from-blue-900 via-purple-900 to-indigo-900">
      <div class="w-full max-w-md">
        <CardBox class="shadow-2xl border-0">
          <div class="text-center mb-8">
            <div class="w-20 h-20 bg-gradient-to-br from-purple-500 to-indigo-600 rounded-2xl flex items-center justify-center mx-auto mb-4">
              <BaseIcon :path="mdiAccountPlus" size="32" class="text-white" />
            </div>
            <h2 class="text-3xl font-bold text-slate-800 dark:text-white mb-2">Create Account</h2>
            <p class="text-slate-600 dark:text-slate-400">Join the secure tunnel network</p>
          </div>

          <form class="space-y-6" @submit.prevent="registerSubmit">
            <FormField label="Username">
              <div class="relative">
                <div class="absolute inset-y-0 left-0 pl-3 flex items-center">
                  <BaseIcon :path="mdiAccount" size="20" class="text-slate-400" />
                </div>
                <FormControl
                  v-model="credentials.username"
                  placeholder="Choose a username"
                  required
                  class="pl-10"
                />
              </div>
            </FormField>

            <FormField label="Email">
              <div class="relative">
                <div class="absolute inset-y-0 left-0 pl-3 flex items-center">
                  <BaseIcon :path="mdiEmail" size="20" class="text-slate-400" />
                </div>
                <FormControl
                  v-model="credentials.email"
                  type="email"
                  placeholder="Enter your email"
                  required
                  class="pl-10"
                />
              </div>
            </FormField>

            <FormField label="Password">
              <div class="relative">
                <div class="absolute inset-y-0 left-0 pl-3 flex items-center">
                  <BaseIcon :path="mdiLock" size="20" class="text-slate-400" />
                </div>
                <FormControl
                  v-model="credentials.password"
                  type="password"
                  placeholder="Create a password"
                  required
                  class="pl-10"
                />
              </div>
            </FormField>

            <div class="space-y-3">
              <BaseButton
                type="submit"
                :icon="mdiAccountPlus"
                label="Create Account"
                color="info"
                class="w-full justify-center py-3 text-lg font-semibold"
              />
              
              <BaseButton
                :icon="mdiLogin"
                label="Back to Login"
                color="info"
                outline
                class="w-full justify-center py-2"
                @click="isRegisterModalActive = false"
              />
            </div>
          </form>
        </CardBox>
      </div>
    </div>

    <!-- Add Domain Modal -->
    <CardBoxModal 
      v-model="isAddModalActive"
      title="Add Tunnel Domain" 
      button="success" 
      :button-label="addLoading ? 'Creating...' : 'Add Domain'"
      :button-disabled="addLoading"
      :cancel-disabled="addLoading"
      has-cancel
      @confirm="addSubmit"
    >
      <form class="space-y-6">
        <div class="bg-blue-50 dark:bg-blue-900/20 p-4 rounded-lg mb-6">
          <div class="flex items-start">
            <BaseIcon :path="mdiTunnel" size="20" class="text-blue-600 dark:text-blue-400 mt-0.5 mr-2 flex-shrink-0" />
            <div>
              <h4 class="font-semibold text-blue-800 dark:text-blue-200 mb-1">Random Domain Generation</h4>
              <p class="text-sm text-blue-600 dark:text-blue-300">
                A random domain will be automatically generated for your tunnel connection.
              </p>
            </div>
          </div>
        </div>

      

        <div v-if="addLoading" class="bg-yellow-50 dark:bg-yellow-900/20 p-4 rounded-lg">
          <div class="flex items-center">
            <div class="animate-spin rounded-full h-4 w-4 border-b-2 border-yellow-600 mr-3"></div>
            <p class="text-sm text-yellow-800 dark:text-yellow-200">
              Creating tunnel with random domain...
            </p>
          </div>
        </div>
      </form>
    </CardBoxModal>

    <!-- Start Tunnel Modal -->
    <CardBoxModal 
      v-model="isStartModalActive" 
      title="Start Tunnel" 
      button="success" 
      button-label="Start Tunnel"
      has-cancel
      @confirm="startSubmit"
    >
      <form class="space-y-6">
        <div class="bg-blue-50 dark:bg-blue-900/20 p-4 rounded-lg mb-6">
          <h4 class="font-semibold text-blue-800 dark:text-blue-200 mb-2 flex items-center">
            <BaseIcon :path="mdiTunnel" size="20" class="mr-2" />
            {{ startDomain.domain }}
          </h4>
          <p class="text-sm text-blue-600 dark:text-blue-300">
            Configure tunnel endpoints for secure connection.
          </p>
        </div>

        <FormField label="Local IP" help="Local IP address to bind">
          <div class="relative">
            <div class="absolute inset-y-0 left-0 pl-3 flex items-center">
              <BaseIcon :path="mdiLan" size="20" class="text-slate-400" />
            </div>
            <FormControl
              v-model="start.localIp"
              placeholder="127.0.0.1"
              required
              class="pl-10"
            />
          </div>
        </FormField>

        <FormField label="Destination IP" help="Target server IP address">
          <div class="relative">
            <div class="absolute inset-y-0 left-0 pl-3 flex items-center">
              <BaseIcon :path="mdiServer" size="20" class="text-slate-400" />
            </div>
            <FormControl
              v-model="start.destinationIp"
              placeholder="192.168.1.100"
              required
              class="pl-10"
            />
          </div>
        </FormField>

        <FormField label="Local Port" help="Local port number">
          <div class="relative">
            <div class="absolute inset-y-0 left-0 pl-3 flex items-center">
              <BaseIcon :path="mdiEthernet" size="20" class="text-slate-400" />
            </div>
            <FormControl
              v-model="start.localPort"
              type="number"
              placeholder="80"
              required
              class="pl-10"
            />
          </div>
        </FormField>
      </form>
    </CardBoxModal>

    <!-- Delete Confirmation Modal -->
    <CardBoxModal 
      v-model="isDeleteModalActive" 
      title="Delete Tunnel" 
      button="danger" 
      button-label="Delete Tunnel"
      has-cancel
      @confirm="deleteSubmit"
    >
      <div v-if="selectedTunnel" class="space-y-4">
        <div class="bg-red-50 dark:bg-red-900/20 p-4 rounded-lg">
          <h4 class="font-semibold text-red-800 dark:text-red-200">{{ selectedTunnel.domain }}</h4>
          <p class="text-sm text-red-600 dark:text-red-300 mt-1">Port: {{ selectedTunnel.port }}</p>
        </div>
        
        <p class="text-slate-600 dark:text-slate-400">
          This will permanently delete the tunnel configuration and stop any active connections.
        </p>
        
        <div class="bg-yellow-50 dark:bg-yellow-900/20 p-4 rounded-lg">
          <div class="flex items-start">
            <BaseIcon :path="mdiDelete" size="20" class="text-yellow-600 dark:text-yellow-400 mt-0.5 mr-2 flex-shrink-0" />
            <p class="text-sm text-yellow-800 dark:text-yellow-200">
              <strong>Warning:</strong> This action cannot be undone. Active tunnel connections will be terminated.
            </p>
          </div>
        </div>
      </div>
    </CardBoxModal>
</template>

<style scoped>
/* Loading spinner */
.animate-spin {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}
</style>