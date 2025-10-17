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
  mdiArrowRight,
  mdiChevronLeft, mdiChevronRight,
  mdiConnection,
  mdiDelete,
  mdiEarth,
  mdiEthernet,
  mdiMagnify,
  mdiNetwork,
  mdiPencil,
  mdiPlay,
  mdiPlus,
  mdiRefresh,
  mdiServer,
  mdiStop,
  mdiTimer,
  mdiViewGridOutline,
  mdiViewList
} from '@mdi/js';
import { computed, onMounted, ref } from "vue";

// Reactive state
const list = ref([])
const isAddModalActive = ref(false)
const isDeleteModalActive = ref(false)
const loading = ref(false)
const selectedProxy = ref(null)

// Form data
const create = ref({
  name: '',
  local_port: '',
  host: '',
  remote_port: '',
  timeout: 30,
})

// Computed
const proxyStats = computed(() => {
  const total = list.value.length
  const active = list.value.filter(proxy => proxy.status === 'active').length
  
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
} = usePaginationFilter(
  list,
  (proxy, query) => {
    const q = query.toLowerCase()
    return (
      proxy.name?.toLowerCase().includes(q) ||
      proxy.host?.toLowerCase().includes(q) ||
      proxy.remote_port?.toString().includes(q) ||
      proxy.local_port?.toString().includes(q) ||
      proxy.status?.toLowerCase().includes(q)
    )
  },
  6
)

const GRID_MIN_ITEMS = 2

const {
  isGridLayout,
  layoutClass,
  toggleLayout
} = useLayoutToggle(paginatedItems, { minItemsForGrid: GRID_MIN_ITEMS })

const layoutToggleLabel = computed(() => isGridLayout.value ? 'List View' : 'Grid View')
const layoutToggleIcon = computed(() => isGridLayout.value ? mdiViewList : mdiViewGridOutline)

// Methods
const getList = async () => {
  loading.value = true
  try {
    const response = await ApiService.localProxyList()
    list.value = response.data.data || []
  } catch (error) {
    console.error('Failed to load proxy list:', error)
    // Mock data for demo
    list.value = [
      {
        name: 'Web Server Proxy',
        local_port: 8080,
        host: '192.168.1.100',
        remote_port: 80,
        timeout: 30,
        status: 'active'
      },
      {
        name: 'Database Proxy',
        local_port: 3306,
        host: '10.0.0.50',
        remote_port: 3306,
        timeout: 60,
        status: 'inactive'
      },
      {
        name: 'API Gateway',
        local_port: 9000,
        host: 'api.example.com',
        remote_port: 443,
        timeout: 30,
        status: 'active'
      }
    ]
  } finally {
    loading.value = false
  }
}

const deleteModal = (proxy) => {
  selectedProxy.value = proxy
  isDeleteModalActive.value = true
}

const deleteSubmit = async () => {
  if (!selectedProxy.value) return
  
  try {
    await ApiService.localProxyDelete(selectedProxy.value)
    isDeleteModalActive.value = false
    selectedProxy.value = null
    await getList()
  } catch (error) {
    console.error('Failed to delete proxy:', error)
  }
}

const addSubmit = async () => {
  try {
    const data = {
      name: create.value.name,
      local_port: parseInt(create.value.local_port),
      host: create.value.host,
      remote_port: parseInt(create.value.remote_port),
      timeout: parseInt(create.value.timeout)
    }
    await ApiService.localProxyCreate(data)
    isAddModalActive.value = false
    resetCreateForm()
    await getList()
  } catch (error) {
    console.error('Failed to add proxy:', error)
  }
}

const startAllProxies = async () => {
  try {
    await ApiService.localProxyStartAll()
    await getList()
  } catch (error) {
    console.error('Failed to start all proxies:', error)
  }
}

const toggleProxyStatus = async (proxy) => {
  try {
    if (proxy.status === 'active') {
      await ApiService.stopLocalProxy(proxy)
    } else {
      await ApiService.startLocalProxyById(proxy)
    }
    await getList()
  } catch (error) {
    console.error('Failed to toggle proxy status:', error)
  }
}

const resetCreateForm = () => {
  create.value = {
    name: '',
    local_port: '',
    host: '',
    remote_port: '',
    timeout: 30,
  }
}

const getStatusColor = (status) => {
  switch (status) {
    case 'active': return 'text-green-600 bg-green-100 dark:text-green-400 dark:bg-green-900/30'
    case 'inactive': return 'text-gray-600 bg-gray-100 dark:text-gray-400 dark:bg-gray-900/30'
    default: return 'text-gray-600 bg-gray-100 dark:text-gray-400 dark:bg-gray-900/30'
  }
}

// Lifecycle
onMounted(() => {
  getList()
})
</script>

<template>
  <div class="space-y-8">
      <!-- Header -->
      <div class="bg-gradient-to-r from-blue-600 via-indigo-600 to-purple-600 rounded-2xl p-8 text-white shadow-lg">
        <div class="flex flex-col lg:flex-row lg:items-center lg:justify-between">
          <div>
            <h1 class="text-3xl lg:text-4xl font-bold mb-2 flex items-center">
              <BaseIcon :path="mdiNetwork" size="40" class="mr-4" />
              Local Proxy Manager
            </h1>
            <p class="text-blue-100 text-lg">Network traffic forwarding and proxy management</p>
          </div>
          <div class="mt-6 lg:mt-0 flex space-x-3">
            <BaseButton
              label="Start All"
              :icon="mdiPlay"
              color="white"
              outline
              @click="startAllProxies"
              class="shadow-lg hover:shadow-xl transform hover:scale-105 transition-all duration-200"
            />
            <BaseButton
              label="Add Proxy"
              :icon="mdiPlus"
              color="white"
              @click="isAddModalActive = true"
              class="shadow-lg hover:shadow-xl transform hover:scale-105 transition-all duration-200"
            />
          </div>
        </div>
      </div>

      <!-- Statistics -->
      <div class="grid grid-cols-1 md:grid-cols-3 gap-6">
        <CardBox class="bg-gradient-to-br from-blue-50 to-blue-100 dark:from-blue-900/20 dark:to-blue-800/20 border-blue-200 dark:border-blue-700">
          <div class="flex items-center justify-between">
            <div>
              <div class="text-2xl font-bold text-blue-600 dark:text-blue-400">{{ proxyStats.total }}</div>
              <div class="text-sm text-blue-600/70 dark:text-blue-400/70">Total Proxies</div>
            </div>
            <BaseIcon :path="mdiServer" size="48" class="text-blue-500 opacity-20" />
          </div>
        </CardBox>

        <CardBox class="bg-gradient-to-br from-green-50 to-green-100 dark:from-green-900/20 dark:to-green-800/20 border-green-200 dark:border-green-700">
          <div class="flex items-center justify-between">
            <div>
              <div class="text-2xl font-bold text-green-600 dark:text-green-400">{{ proxyStats.active }}</div>
              <div class="text-sm text-green-600/70 dark:text-green-400/70">Active Proxies</div>
            </div>
            <BaseIcon :path="mdiPlay" size="48" class="text-green-500 opacity-20" />
          </div>
        </CardBox>

        <CardBox class="bg-gradient-to-br from-gray-50 to-gray-100 dark:from-gray-900/20 dark:to-gray-800/20 border-gray-200 dark:border-gray-700">
          <div class="flex items-center justify-between">
            <div>
              <div class="text-2xl font-bold text-gray-600 dark:text-gray-400">{{ proxyStats.inactive }}</div>
              <div class="text-sm text-gray-600/70 dark:text-gray-400/70">Inactive Proxies</div>
            </div>
            <BaseIcon :path="mdiStop" size="48" class="text-gray-500 opacity-20" />
          </div>
        </CardBox>
      </div>

      <!-- Proxy List -->
      <CardBox>
        <SectionTitleLineWithButton :icon="mdiConnection" title="Active Proxy Connections" main>
          <div class="flex flex-col gap-3 md:flex-row md:items-center">
            <div class="w-full md:w-64">
              <FormControl
                v-model="searchQuery"
                :icon="mdiMagnify"
                placeholder="Search proxies"
              />
            </div>
            <BaseButton
              :icon="layoutToggleIcon"
              :label="layoutToggleLabel"
              color="lightDark"
              outline
              @click="toggleLayout"
              class="shrink-0"
            />
            <BaseButton
              :icon="mdiRefresh"
              color="info"
              rounded-full
              @click="getList"
              :disabled="loading"
              class="shadow-sm hover:shadow-md"
            />
          </div>
        </SectionTitleLineWithButton>

        <div v-if="loading" class="text-center py-12">
          <div class="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600"></div>
          <p class="text-slate-500 dark:text-slate-400 mt-4">Loading proxies...</p>
        </div>

        <div v-else-if="filteredItems.length === 0" class="text-center py-12">
          <BaseIcon :path="mdiNetwork" size="64" class="mx-auto text-slate-300 dark:text-slate-600 mb-4" />
          <p class="text-slate-500 dark:text-slate-400 mb-4">
            {{ searchQuery ? 'No proxies match your search.' : 'No proxy configurations found.' }}
          </p>
          <BaseButton
            v-if="!searchQuery"
            label="Create Your First Proxy"
            :icon="mdiPlus"
            color="info"
            @click="isAddModalActive = true"
          />
        </div>

  <div v-else :class="layoutClass">
          <div 
            v-for="proxy in paginatedItems" 
            :key="proxy.name"
            class="p-6 bg-slate-50 dark:bg-slate-800/50 rounded-xl hover:bg-slate-100 dark:hover:bg-slate-700/50 transition-colors flex flex-col h-full"
          >
            <div class="flex flex-col gap-6 lg:flex-row lg:items-start lg:justify-between">
              <div class="flex items-start gap-4 flex-1">
                <div class="flex-shrink-0">
                  <div class="w-12 h-12 bg-gradient-to-br from-blue-500 to-indigo-600 rounded-xl flex items-center justify-center">
                    <BaseIcon :path="mdiServer" size="24" class="text-white" />
                  </div>
                </div>
                
                <div class="space-y-2 flex-1">
                  <h3 class="font-semibold text-lg">{{ proxy.name }}</h3>
                  <div class="flex flex-wrap items-center gap-x-4 gap-y-2 text-sm text-slate-500 dark:text-slate-400">
                    <div class="flex items-center">
                      <BaseIcon :path="mdiEthernet" size="16" class="mr-1" />
                      {{ proxy.local_port }}
                    </div>
                    <div class="flex items-center gap-2">
                      <BaseIcon :path="mdiArrowRight" size="16" class="text-slate-400" />
                      <span class="flex items-center">
                        <BaseIcon :path="mdiEarth" size="16" class="mr-1" />
                        {{ proxy.host }}:{{ proxy.remote_port }}
                      </span>
                    </div>
                    <div class="flex items-center">
                      <BaseIcon :path="mdiTimer" size="16" class="mr-1" />
                      {{ proxy.timeout }}s
                    </div>
                  </div>
                </div>
              </div>
              <div class="flex items-start lg:flex-none justify-start lg:justify-end">
                <span 
                  :class="[
                    'inline-flex items-center px-3 py-1 rounded-full text-xs font-medium',
                    getStatusColor(proxy.status)
                  ]"
                >
                  {{ proxy.status === 'active' ? 'Active' : 'Inactive' }}
                </span>
              </div>
            </div>
            
            <div class="mt-6 flex flex-wrap items-center justify-end gap-2">
              <BaseButton 
                :icon="proxy.status === 'active' ? mdiStop : mdiPlay" 
                :color="proxy.status === 'active' ? 'danger' : 'success'"
                small
                @click="toggleProxyStatus(proxy)"
                :title="proxy.status === 'active' ? 'Stop Proxy' : 'Start Proxy'"
              />
              
              <BaseButton 
                :icon="mdiPencil" 
                color="info"
                small
                title="Edit"
              />
              
              <BaseButton 
                :icon="mdiDelete" 
                color="danger"
                small
                @click="deleteModal(proxy)"
                title="Delete"
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

      <!-- Add Proxy Modal -->
      <CardBoxModal 
        v-model="isAddModalActive" 
        title="Add New Proxy" 
        button="success" 
        buttonLabel="Create Proxy"
        has-cancel
        @confirm="addSubmit"
      >
        <form class="space-y-6">
          <FormField label="Proxy Name" help="A friendly name for this proxy">
            <div class="relative">
              <div class="absolute inset-y-0 left-0 pl-3 flex items-center">
                <BaseIcon :path="mdiServer" size="20" class="text-slate-400" />
              </div>
              <FormControl
                v-model="create.name"
                placeholder="Web Server Proxy"
                required
                class="pl-10"
              />
            </div>
          </FormField>

          <FormField label="Local Port" help="Port on local machine">
            <div class="relative">
              <div class="absolute inset-y-0 left-0 pl-3 flex items-center">
                <BaseIcon :path="mdiEthernet" size="20" class="text-slate-400" />
              </div>
              <FormControl
                v-model="create.local_port"
                type="number"
                placeholder="8080"
                required
                class="pl-10"
              />
            </div>
          </FormField>

          <FormField label="Remote Host" help="Target hostname or IP address">
            <div class="relative">
              <div class="absolute inset-y-0 left-0 pl-3 flex items-center">
                <BaseIcon :path="mdiEarth" size="20" class="text-slate-400" />
              </div>
              <FormControl
                v-model="create.host"
                placeholder="192.168.1.100 or example.com"
                required
                class="pl-10"
              />
            </div>
          </FormField>

          <FormField label="Remote Port" help="Port on remote host">
            <div class="relative">
              <div class="absolute inset-y-0 left-0 pl-3 flex items-center">
                <BaseIcon :path="mdiEthernet" size="20" class="text-slate-400" />
              </div>
              <FormControl
                v-model="create.remote_port"
                type="number"
                placeholder="80"
                required
                class="pl-10"
              />
            </div>
          </FormField>

          <FormField label="Timeout (seconds)" help="Connection timeout in seconds">
            <div class="relative">
              <div class="absolute inset-y-0 left-0 pl-3 flex items-center">
                <BaseIcon :path="mdiTimer" size="20" class="text-slate-400" />
              </div>
              <FormControl
                v-model="create.timeout"
                type="number"
                placeholder="30"
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
        title="Delete Proxy" 
        button="danger" 
        buttonLabel="Delete Proxy"
        has-cancel
        @confirm="deleteSubmit"
      >
        <div v-if="selectedProxy" class="space-y-4">
          <div class="bg-red-50 dark:bg-red-900/20 p-4 rounded-lg">
            <h4 class="font-semibold text-red-800 dark:text-red-200">{{ selectedProxy.name }}</h4>
            <p class="text-sm text-red-600 dark:text-red-300 mt-1">
              {{ selectedProxy.local_port }} → {{ selectedProxy.host }}:{{ selectedProxy.remote_port }}
            </p>
          </div>
          
          <p class="text-slate-600 dark:text-slate-400">
            This will permanently delete the proxy configuration. This action cannot be undone.
          </p>
          
          <div class="bg-yellow-50 dark:bg-yellow-900/20 p-4 rounded-lg">
            <div class="flex items-start">
              <BaseIcon :path="mdiDelete" size="20" class="text-yellow-600 dark:text-yellow-400 mt-0.5 mr-2 flex-shrink-0" />
              <p class="text-sm text-yellow-800 dark:text-yellow-200">
                <strong>Warning:</strong> If this proxy is currently active, it will be stopped and removed.
              </p>
            </div>
          </div>
        </div>
      </CardBoxModal>
    </div>
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