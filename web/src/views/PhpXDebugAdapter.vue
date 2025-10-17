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
  mdiBug,
  mdiCheck,
  mdiChevronLeft, mdiChevronRight,
  mdiClose,
  mdiCog,
  mdiDelete,
  mdiFileCode,
  mdiFolder,
  mdiLink,
  mdiMagnify,
  mdiMap,
  mdiNetwork,
  mdiPlay,
  mdiPlus,
  mdiRefresh,
  mdiStop,
  mdiViewGridOutline,
  mdiViewList,
  mdiWeb
} from "@mdi/js";
import { computed, onMounted, ref } from "vue";

// Reactive state
const settings = ref({
  listen: '',
  mappings: []
})
const loading = ref(false)
const isAddModalActive = ref(false)
const isConfigurationModalActive = ref(false)
const isRunning = ref(false)

// Form data
const create = ref({
  name: '',
  path: '',
  url: '',
})

// Computed
const debugStats = computed(() => {
  const total = settings.value.mappings?.length || 0
  const hasConfiguration = settings.value.listen ? 1 : 0
  
  return { total, configured: hasConfiguration }
})

const mappingsRef = computed(() => settings.value.mappings || [])

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
  mappingsRef,
  (mapping, query) => {
    const q = query.toLowerCase()
    return (
      mapping.name?.toLowerCase().includes(q) ||
      mapping.path?.toLowerCase().includes(q) ||
      mapping.url?.toLowerCase().includes(q)
    )
  },
  5
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
    const response = await ApiService.getXDebugAdapterSettings()
    settings.value = response.data.data || { listen: '', mappings: [] }
  } catch (error) {
    console.error('Failed to load XDebug adapter settings:', error)
    // Mock data for demo
    settings.value = {
      listen: '0.0.0.0:9003',
      mappings: [
        {
          name: 'Laravel Project',
          path: '/var/www/html/laravel-app',
          url: '127.0.0.1:8000'
        },
        {
          name: 'WordPress Site',
          path: '/var/www/html/wordpress',
          url: '127.0.0.1:8080'
        },
        {
          name: 'Symfony API',
          path: '/var/www/html/symfony-api',
          url: '127.0.0.1:9000'
        }
      ]
    }
  } finally {
    loading.value = false
  }
}

const deleteModal = async (data) => {
  if (!confirm(`Are you sure you want to delete mapping "${data.name}"?`)) {
    return
  }
  
  try {
    await ApiService.removeXDebugAdapterSettings(data)
    await getList()
  } catch (error) {
    console.error('Failed to delete mapping:', error)
  }
}

const addSubmit = async () => {
  try {
    await ApiService.addXDebugAdapterSettings(create.value)
    isAddModalActive.value = false
    resetForm()
    await getList()
  } catch (error) {
    console.error('Failed to add mapping:', error)
  }
}

const saveConfiguration = async () => {
  try {
    await ApiService.updateXDebugAdapterSettings(settings.value)
    isConfigurationModalActive.value = false
    await getList()
  } catch (error) {
    console.error('Failed to update configuration:', error)
  }
}

const start = async () => {
  try {
    await ApiService.startXDebugAdapter()
    isRunning.value = true
  } catch (error) {
    console.error('Failed to start XDebug adapter:', error)
  }
}

const stop = async () => {
  try {
    await ApiService.stopXDebugAdapter()
    isRunning.value = false
  } catch (error) {
    console.error('Failed to stop XDebug adapter:', error)
  }
}

const resetForm = () => {
  create.value = {
    name: '',
    path: '',
    url: '',
  }
}

const getStatusColor = () => {
  return isRunning.value 
    ? 'text-green-600 bg-green-100 dark:text-green-400 dark:bg-green-900/30'
    : 'text-gray-600 bg-gray-100 dark:text-gray-400 dark:bg-gray-900/30'
}

const extractProjectName = (path) => {
  const parts = path.split('/')
  return parts[parts.length - 1] || 'Unknown'
}

// Lifecycle
onMounted(() => {
  getList()
})
</script>

<template>
  <div class="space-y-8">
      <!-- Header -->
      <div class="bg-gradient-to-r from-purple-600 via-pink-600 to-red-600 rounded-2xl p-8 text-white shadow-lg">
        <div class="flex flex-col lg:flex-row lg:items-center lg:justify-between">
          <div>
            <h1 class="text-3xl lg:text-4xl font-bold mb-2 flex items-center">
              <BaseIcon :path="mdiFileCode" size="40" class="mr-4" />
              PHP XDebug Adapter
            </h1>
            <p class="text-purple-100 text-lg">Debug PHP applications with IDE integration</p>
          </div>
          <div class="mt-6 lg:mt-0 flex flex-wrap gap-3">
            <BaseButton
              label="Configuration"
              :icon="mdiCog"
              color="white"
              outline
              @click="isConfigurationModalActive = true"
              class="shadow-lg hover:shadow-xl transform hover:scale-105 transition-all duration-200"
            />
            <BaseButton
              label="Refresh"
              :icon="mdiRefresh"
              color="white"
              outline
              @click="getList"
              :disabled="loading"
              class="shadow-lg hover:shadow-xl transform hover:scale-105 transition-all duration-200"
            />
            <BaseButton
              label="Add Mapping"
              :icon="mdiPlus"
              color="white"
              @click="isAddModalActive = true"
              class="shadow-lg hover:shadow-xl transform hover:scale-105 transition-all duration-200"
            />
          </div>
        </div>
      </div>

      <!-- Control Panel -->
      <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <!-- Adapter Status -->
        <CardBox class="lg:col-span-1">
          <div class="text-center">
            <div class="w-16 h-16 mx-auto mb-4 bg-gradient-to-br from-purple-500 to-pink-600 rounded-2xl flex items-center justify-center">
              <BaseIcon :path="mdiBug" size="32" class="text-white" />
            </div>
            <h3 class="text-lg font-semibold mb-2">Debug Adapter</h3>
            <span 
              :class="[
                'inline-flex items-center px-3 py-1 rounded-full text-sm font-medium mb-4',
                getStatusColor()
              ]"
            >
              {{ isRunning ? 'Running' : 'Stopped' }}
            </span>
            <div class="flex space-x-2">
              <BaseButton
                label="Start Adapter"
                :icon="mdiPlay"
                color="success"
                @click="start()"
                class="flex-1"
              />
              <BaseButton
                label="Stop Adapter"
                :icon="mdiStop"
                color="danger"
                @click="stop()"
                class="flex-1"
              />
            </div>
          </div>
        </CardBox>

        <!-- Statistics -->
        <div class="lg:col-span-2 grid grid-cols-1 md:grid-cols-2 gap-6">
          <CardBox class="bg-gradient-to-br from-purple-50 to-purple-100 dark:from-purple-900/20 dark:to-purple-800/20 border-purple-200 dark:border-purple-700">
            <div class="flex items-center justify-between">
              <div>
                <div class="text-2xl font-bold text-purple-600 dark:text-purple-400">{{ debugStats.total }}</div>
                <div class="text-sm text-purple-600/70 dark:text-purple-400/70">Debug Mappings</div>
              </div>
              <BaseIcon :path="mdiMap" size="48" class="text-purple-500 opacity-20" />
            </div>
          </CardBox>

          <CardBox class="bg-gradient-to-br from-blue-50 to-blue-100 dark:from-blue-900/20 dark:to-blue-800/20 border-blue-200 dark:border-blue-700">
            <div class="flex items-center justify-between">
              <div>
                <div class="text-2xl font-bold text-blue-600 dark:text-blue-400">{{ debugStats.configured }}</div>
                <div class="text-sm text-blue-600/70 dark:text-blue-400/70">Configuration</div>
              </div>
              <BaseIcon :path="mdiCog" size="48" class="text-blue-500 opacity-20" />
            </div>
          </CardBox>
        </div>
      </div>

      <!-- Debug Mappings -->
      <CardBox>
        <SectionTitleLineWithButton :icon="mdiMap" title="Debug Path Mappings" main>
          <div class="flex flex-col gap-3 md:flex-row md:items-center">
            <div class="w-full md:w-64">
              <FormControl
                v-model="searchQuery"
                :icon="mdiMagnify"
                placeholder="Search mappings"
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
          </div>
        </SectionTitleLineWithButton>

        <div v-if="loading" class="text-center py-12">
          <div class="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-purple-600"></div>
          <p class="text-slate-500 dark:text-slate-400 mt-4">Loading debug mappings...</p>
        </div>

        <div v-else-if="filteredItems.length === 0" class="text-center py-12">
          <BaseIcon :path="mdiFileCode" size="64" class="mx-auto text-slate-300 dark:text-slate-600 mb-4" />
          <p class="text-slate-500 dark:text-slate-400 mb-4">
            {{ searchQuery ? 'No mappings match your search.' : 'No debug mappings configured.' }}
          </p>
          <BaseButton
            v-if="!searchQuery"
            label="Create Your First Mapping"
            :icon="mdiPlus"
            color="info"
            @click="isAddModalActive = true"
          />
        </div>

  <div v-else :class="layoutClass">
          <div 
            v-for="mapping in paginatedItems" 
            :key="mapping.name"
            class="p-6 bg-slate-50 dark:bg-slate-800/50 rounded-xl hover:bg-slate-100 dark:hover:bg-slate-700/50 transition-colors flex flex-col h-full"
          >
            <div class="flex items-start gap-4 flex-1">
              <div class="flex-shrink-0">
                <div class="w-12 h-12 bg-gradient-to-br from-purple-500 to-pink-600 rounded-xl flex items-center justify-center">
                  <BaseIcon :path="mdiFileCode" size="24" class="text-white" />
                </div>
              </div>
              
              <div class="space-y-3 flex-1">
                <h3 class="font-semibold text-lg">{{ mapping.name }}</h3>
                <div class="space-y-2 text-sm text-slate-500 dark:text-slate-400">
                  <div class="flex items-center">
                    <BaseIcon :path="mdiFolder" size="16" class="mr-2" />
                    <span class="font-mono break-words">{{ mapping.path }}</span>
                  </div>
                  <div class="flex items-center">
                    <BaseIcon :path="mdiLink" size="16" class="mr-2" />
                    <span class="font-mono break-words">{{ mapping.url }}</span>
                  </div>
                </div>
              </div>
            </div>
            
            <div class="mt-6 flex items-center justify-end gap-2">
              <BaseButton 
                :icon="mdiDelete" 
                color="danger"
                size="small"
                @click="deleteModal(mapping)"
                title="Delete Mapping"
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

      <!-- Add Mapping Modal -->
      <CardBoxModal 
        v-model="isAddModalActive" 
        title="Add Debug Mapping" 
        button="success" 
        buttonLabel="Add Mapping"
        has-cancel
        @confirm="addSubmit"
      >
        <form class="space-y-6">
          <div class="bg-blue-50 dark:bg-blue-900/20 p-4 rounded-lg mb-6">
            <h4 class="font-semibold text-blue-800 dark:text-blue-200 mb-2 flex items-center">
              <BaseIcon :path="mdiBug" size="20" class="mr-2" />
              Debug Path Mapping
            </h4>
            <p class="text-sm text-blue-600 dark:text-blue-300">
              Map local project paths to debug URLs for IDE integration.
            </p>
          </div>

          <FormField label="Project Name" help="A friendly name for this project">
            <div class="relative">
              <div class="absolute inset-y-0 left-0 pl-3 flex items-center">
                <BaseIcon :path="mdiFileCode" size="20" class="text-slate-400" />
              </div>
              <FormControl
                v-model="create.name"
                placeholder="Laravel Project"
                required
                class="pl-10"
              />
            </div>
          </FormField>

          <FormField label="Project Path" help="Local filesystem path to the project">
            <div class="relative">
              <div class="absolute inset-y-0 left-0 pl-3 flex items-center">
                <BaseIcon :path="mdiFolder" size="20" class="text-slate-400" />
              </div>
              <FormControl
                v-model="create.path"
                placeholder="/var/www/html/PROJECT"
                required
                class="pl-10 font-mono"
              />
            </div>
          </FormField>

          <FormField label="Debug URL" help="URL where the debug server is accessible">
            <div class="relative">
              <div class="absolute inset-y-0 left-0 pl-3 flex items-center">
                <BaseIcon :path="mdiWeb" size="20" class="text-slate-400" />
              </div>
              <FormControl
                v-model="create.url"
                placeholder="127.0.0.1:9981"
                required
                class="pl-10 font-mono"
              />
            </div>
          </FormField>
        </form>

        <template #footer>
          <div class="flex justify-end space-x-3">
            <BaseButton
              :icon="mdiClose"
              label="Cancel"
              color="lightDark"
              @click="isAddModalActive = false"
            />
            <BaseButton
              :icon="mdiCheck"
              label="Save Mapping"
              color="success"
              @click="addSubmit"
            />
          </div>
        </template>
      </CardBoxModal>

      <!-- Configuration Modal -->
      <CardBoxModal 
        v-model="isConfigurationModalActive" 
        title="XDebug Adapter Configuration" 
        button="success" 
        buttonLabel="Save Configuration"
        has-cancel
        @confirm="saveConfiguration"
      >
        <form class="space-y-6">
          <div class="bg-yellow-50 dark:bg-yellow-900/20 p-4 rounded-lg mb-6">
            <h4 class="font-semibold text-yellow-800 dark:text-yellow-200 mb-2 flex items-center">
              <BaseIcon :path="mdiCog" size="20" class="mr-2" />
              Debug Adapter Settings
            </h4>
            <p class="text-sm text-yellow-600 dark:text-yellow-300">
              Configure the XDebug adapter listen address and port.
            </p>
          </div>

          <FormField label="Listen Address" help="IP address and port for XDebug connections">
            <div class="relative">
              <div class="absolute inset-y-0 left-0 pl-3 flex items-center">
                <BaseIcon :path="mdiNetwork" size="20" class="text-slate-400" />
              </div>
              <FormControl
                v-model="settings.listen"
                placeholder="0.0.0.0:9003"
                required
                class="pl-10 font-mono"
              />
            </div>
          </FormField>

          <div class="bg-gray-50 dark:bg-gray-800 p-4 rounded-lg">
            <h5 class="font-medium text-gray-800 dark:text-gray-200 mb-2">Common Settings:</h5>
            <div class="space-y-2 text-sm text-gray-600 dark:text-gray-400 font-mono">
              <div>• 0.0.0.0:9003 (XDebug 3.x default)</div>
              <div>• 0.0.0.0:9000 (XDebug 2.x default)</div>
              <div>• 127.0.0.1:9003 (Local only)</div>
            </div>
          </div>
        </form>

        <template #footer>
          <div class="flex justify-end space-x-3">
            <BaseButton
              :icon="mdiClose"
              label="Cancel"
              color="lightDark"
              @click="isConfigurationModalActive = false"
            />
            <BaseButton
              :icon="mdiCheck"
              label="Save Configuration"
              color="success"
              @click="saveConfiguration"
            />
          </div>
        </template>
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

/* Monospace font */
.font-mono {
  font-family: 'SF Mono', 'Monaco', 'Inconsolata', 'Fira Code', 'Droid Sans Mono', 'Source Code Pro', monospace;
}
</style>