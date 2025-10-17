<script setup>
import BaseButton from "@/components/BaseButton.vue";
import BaseIcon from "@/components/BaseIcon.vue";
import CardBox from "@/components/CardBox.vue";
import CardBoxModal from "@/components/CardBoxModal.vue";
import FormControl from "@/components/FormControl.vue";
import FormField from "@/components/FormField.vue";

import ApiService from "@/services/ApiService";
import {
  mdiAccountBox,
  mdiCloudOutline,
  mdiConsole,
  mdiDelete,
  mdiDeveloperBoard,
  mdiDocker,
  mdiEthernet,
  mdiKey,
  mdiPencil, mdiPlus,
  mdiRefresh,
  mdiServer
} from "@mdi/js";
import { computed, onMounted, ref } from "vue";
import { useRouter } from "vue-router";

// Router
const router = useRouter()

// Reactive state
const personalContainers = ref([])
const isEditModalActive = ref(false)
const isAddModalActive = ref(false)
const isDeleteModalActive = ref(false)
const loading = ref(false)
const selectedContainer = ref(null)

// Pagination
const currentPage = ref(1)
const itemsPerPage = ref(6)

// Form data
const modalPath = ref({
  username: '',
  password: '',
  port: '',
  redockPort: ''
})

const create = ref({
  username: '',
  password: '',
  port: 0,
  redockPort: 0,
})

// Computed
const containerStats = computed(() => {
  const total = personalContainers.value.length
  const withRedockPort = personalContainers.value.filter(container => container.redockPort).length
  
  return { total, withRedockPort }
})

const paginatedContainers = computed(() => {
  const start = (currentPage.value - 1) * itemsPerPage.value
  const end = start + itemsPerPage.value
  return personalContainers.value.slice(start, end)
})

const totalPages = computed(() => {
  return Math.ceil(personalContainers.value.length / itemsPerPage.value)
})

const paginationInfo = computed(() => {
  const start = (currentPage.value - 1) * itemsPerPage.value + 1
  const end = Math.min(start + itemsPerPage.value - 1, personalContainers.value.length)
  return `${start}-${end} of ${personalContainers.value.length} containers`
})

// Methods
const getPersonalContainers = async () => {
  loading.value = true
  try {
    const response = await ApiService.getPersonalContainers()
    personalContainers.value = response.data.data.map(container => ({
      username: container.username,
      password: container.password,
      port: container.port,
      redockPort: container.redockPort
    }))
  } catch (error) {
    console.error('Failed to load personal containers:', error)
    // Mock data for demo
    personalContainers.value = [
      { username: 'developer1', password: '****', port: 2222, redockPort: 8080 },
      { username: 'developer2', password: '****', port: 2223, redockPort: 8081 },
      { username: 'tester', password: '****', port: 2224, redockPort: null }
    ]
  } finally {
    loading.value = false
  }
}

const editModal = (container) => {
  modalPath.value = {
    username: container.username,
    password: container.password,
    port: container.port.toString(),
    redockPort: container.redockPort ? container.redockPort.toString() : ''
  }
  isEditModalActive.value = true
}

const deleteModal = (container) => {
  selectedContainer.value = container
  isDeleteModalActive.value = true
}

const addSubmit = async () => {
  try {
    // Convert ports to strings before sending
    const containerData = {
      ...create.value,
      port: String(create.value.port),
      redockPort: String(create.value.redockPort)
    }
    await ApiService.addPersonalContainer(containerData)
    isAddModalActive.value = false
    resetCreateForm()
    await getPersonalContainers()
  } catch (error) {
    console.error('Failed to add container:', error)
  }
}

const editSubmit = async () => {
  try {
    // Convert ports to strings before sending
    const containerData = {
      ...modalPath.value,
      port: String(modalPath.value.port),
      redockPort: String(modalPath.value.redockPort)
    }
    await ApiService.updatePersonalContainer(containerData)
    isEditModalActive.value = false
    await getPersonalContainers()
  } catch (error) {
    console.error('Failed to update container:', error)
  }
}

const deleteSubmit = async () => {
  if (!selectedContainer.value) return
  
  try {
    await ApiService.deletePersonalContainer({
      username: selectedContainer.value.username,
      password: selectedContainer.value.password,
      port: String(selectedContainer.value.port),
      redockPort: String(selectedContainer.value.redockPort)
    })
    isDeleteModalActive.value = false
    selectedContainer.value = null
    await getPersonalContainers()
  } catch (error) {
    console.error('Failed to delete container:', error)
  }
}

const resetCreateForm = () => {
  create.value = {
    username: '',
    password: '',
    port: 0,
    redockPort: 0,
  }
}

const openTerminal = (container) => {
  // Terminal sayfasına exec/name formatında router push
  router.push(`/exec/${container.username}`)
}

const generatePassword = () => {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789!@#$%^&*'
  let password = ''
  for (let i = 0; i < 12; i++) {
    password += chars.charAt(Math.floor(Math.random() * chars.length))
  }
  create.value.password = password
}

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
onMounted(() => {
  getPersonalContainers()
})
</script>

<template>
  <div class="space-y-8">
      <!-- Header -->
      <div class="bg-gradient-to-r from-emerald-600 via-teal-600 to-cyan-600 rounded-2xl p-8 text-white shadow-lg">
        <div class="flex flex-col lg:flex-row lg:items-center lg:justify-between">
          <div>
            <h1 class="text-3xl lg:text-4xl font-bold mb-2 flex items-center">
              <BaseIcon :path="mdiDeveloperBoard" size="40" class="mr-4" />
              Development Environment
            </h1>
            <p class="text-emerald-100 text-lg">Manage personal development containers and environments</p>
          </div>
          <div class="mt-6 lg:mt-0">
            <BaseButton
              label="Add Container"
              :icon="mdiPlus"
              color="white"
              @click="isAddModalActive = true"
              class="shadow-lg hover:shadow-xl transform hover:scale-105 transition-all duration-200"
            />
          </div>
        </div>
      </div>

      <!-- Statistics -->
      <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
        <CardBox class="bg-gradient-to-br from-emerald-50 to-emerald-100 dark:from-emerald-900/20 dark:to-emerald-800/20 border-emerald-200 dark:border-emerald-700">
          <div class="flex items-center justify-between">
            <div>
              <div class="text-2xl font-bold text-emerald-600 dark:text-emerald-400">{{ containerStats.total }}</div>
              <div class="text-sm text-emerald-600/70 dark:text-emerald-400/70">Total Containers</div>
            </div>
            <BaseIcon :path="mdiDocker" size="48" class="text-emerald-500 opacity-20" />
          </div>
        </CardBox>

        <CardBox class="bg-gradient-to-br from-teal-50 to-teal-100 dark:from-teal-900/20 dark:to-teal-800/20 border-teal-200 dark:border-teal-700">
          <div class="flex items-center justify-between">
            <div>
              <div class="text-2xl font-bold text-teal-600 dark:text-teal-400">{{ containerStats.withRedockPort }}</div>
              <div class="text-sm text-teal-600/70 dark:text-teal-400/70">With Redock Port</div>
            </div>
            <BaseIcon :path="mdiCloudOutline" size="48" class="text-teal-500 opacity-20" />
          </div>
        </CardBox>
      </div>

      <!-- Containers Table -->
      <CardBox>
        <div class="bg-gradient-to-r from-slate-50 to-slate-100 dark:from-slate-800 dark:to-slate-700 p-6 -m-6 mb-6">
          <div class="flex items-center justify-between">
            <div>
              <h2 class="text-xl font-bold flex items-center">
                <BaseIcon :path="mdiServer" size="24" class="mr-3 text-emerald-600 dark:text-emerald-400" />
                Personal Development Containers
              </h2>
              <p class="text-slate-600 dark:text-slate-400 mt-1">Manage isolated development environments</p>
            </div>
            <BaseButton
              :icon="mdiRefresh"
              color="info"
              rounded-full
              @click="getPersonalContainers"
              :disabled="loading"
              class="shadow-sm hover:shadow-md"
            />
          </div>
        </div>

        <div v-if="loading" class="text-center py-12">
          <div class="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-emerald-600"></div>
          <p class="text-slate-500 dark:text-slate-400 mt-4">Loading containers...</p>
        </div>

        <div v-else-if="personalContainers.length === 0" class="text-center py-12">
          <BaseIcon :path="mdiDeveloperBoard" size="64" class="mx-auto text-slate-300 dark:text-slate-600 mb-4" />
          <p class="text-slate-500 dark:text-slate-400 mb-4">No development containers found</p>
          <BaseButton
            label="Create Your First Container"
            :icon="mdiPlus"
            color="success"
            @click="isAddModalActive = true"
          />
        </div>

        <div v-else class="space-y-4">
          <div 
            v-for="container in paginatedContainers" 
            :key="container.username"
            class="flex items-center justify-between p-6 bg-slate-50 dark:bg-slate-800/50 rounded-xl hover:bg-slate-100 dark:hover:bg-slate-700/50 transition-colors"
          >
            <div class="flex items-center space-x-6">
              <div class="flex-shrink-0">
                <div class="w-12 h-12 bg-gradient-to-br from-emerald-500 to-teal-600 rounded-xl flex items-center justify-center">
                  <BaseIcon :path="mdiAccountBox" size="24" class="text-white" />
                </div>
              </div>
              
              <div class="flex-1">
                <h3 class="font-semibold text-lg">{{ container.username }}</h3>
                <div class="flex items-center space-x-4 mt-1 text-sm text-slate-500 dark:text-slate-400">
                  <div class="flex items-center">
                    <BaseIcon :path="mdiEthernet" size="16" class="mr-1" />
                    SSH: {{ container.port }}
                  </div>
                  <div v-if="container.redockPort" class="flex items-center">
                    <BaseIcon :path="mdiCloudOutline" size="16" class="mr-1" />
                    Redock: {{ container.redockPort }}
                  </div>
                </div>
              </div>
              
              <div class="flex-shrink-0">
                <span 
                  :class="[
                    'inline-flex items-center px-3 py-1 rounded-full text-xs font-medium',
                    container.redockPort 
                      ? 'text-emerald-600 bg-emerald-100 dark:text-emerald-400 dark:bg-emerald-900/30'
                      : 'text-yellow-600 bg-yellow-100 dark:text-yellow-400 dark:bg-yellow-900/30'
                  ]"
                >
                  {{ container.redockPort ? 'Ready' : 'SSH Only' }}
                </span>
              </div>
            </div>
            
            <div class="flex items-center space-x-2 ml-6">
              <BaseButton 
                :icon="mdiConsole" 
                color="info"
                size="small"
                @click="openTerminal(container)"
                title="SSH Access"
              />
              
              <BaseButton 
                :icon="mdiPencil" 
                color="warning"
                size="small"
                @click="editModal(container)"
                title="Edit"
              />
              
              <BaseButton 
                :icon="mdiDelete" 
                color="danger"
                size="small"
                @click="deleteModal(container)"
                title="Delete"
              />
            </div>
          </div>
        </div>

        <!-- Pagination -->
        <div v-if="totalPages > 1" class="flex items-center justify-between mt-6 px-6 pb-4">
          <div class="text-sm text-slate-500 dark:text-slate-400">
            {{ paginationInfo }}
          </div>
          <div class="flex space-x-2">
            <BaseButton
              :icon="mdiChevronLeft"
              label="Previous"
              :disabled="currentPage === 1"
              color="light"
              size="small"
              @click="prevPage"
            />
            <div class="flex space-x-1">
              <button
                v-for="page in totalPages"
                :key="page"
                @click="goToPage(page)"
                :class="[
                  'px-3 py-2 text-sm rounded-lg transition-colors',
                  page === currentPage
                    ? 'bg-emerald-600 text-white shadow-md'
                    : 'text-slate-600 dark:text-slate-300 hover:text-emerald-600 hover:bg-emerald-50 dark:hover:bg-slate-700'
                ]"
              >
                {{ page }}
              </button>
            </div>
            <BaseButton
              :icon="mdiChevronRight"
              label="Next"
              :disabled="currentPage === totalPages"
              color="light"
              size="small"
              @click="nextPage"
            />
          </div>
        </div>
      </CardBox>

      <!-- Add Container Modal -->
      <CardBoxModal 
        v-model="isAddModalActive" 
        title="Add Development Container" 
        button="success" 
        buttonLabel="Create Container"
        has-cancel
        @confirm="addSubmit"
      >
        <form class="space-y-6">
          <FormField label="Username" help="Container username for SSH access">
            <div class="relative">
              <div class="absolute inset-y-0 left-0 pl-3 flex items-center">
                <BaseIcon :path="mdiAccountBox" size="20" class="text-slate-400" />
              </div>
              <FormControl
                v-model="create.username"
                placeholder="developer"
                required
                class="pl-10"
              />
            </div>
          </FormField>

          <FormField label="Password" help="SSH password for the container">
            <div class="relative">
              <div class="absolute inset-y-0 left-0 pl-3 flex items-center">
                <BaseIcon :path="mdiKey" size="20" class="text-slate-400" />
              </div>
              <FormControl
                v-model="create.password"
                type="password"
                placeholder="Enter password"
                required
                class="pl-10 pr-12"
              />
              <button
                type="button"
                @click="generatePassword"
                class="absolute inset-y-0 right-0 pr-3 flex items-center text-blue-600 hover:text-blue-800"
                title="Generate Password"
              >
                <BaseIcon :path="mdiRefresh" size="20" />
              </button>
            </div>
          </FormField>

          <FormField label="SSH Port" help="Port for SSH access">
            <div class="relative">
              <div class="absolute inset-y-0 left-0 pl-3 flex items-center">
                <BaseIcon :path="mdiEthernet" size="20" class="text-slate-400" />
              </div>
              <FormControl
                v-model="create.port"
                type="text"
                placeholder="2222"
                required
                class="pl-10"
              />
            </div>
          </FormField>

          <FormField label="Redock Port (Optional)" help="Port for Redock web interface">
            <div class="relative">
              <div class="absolute inset-y-0 left-0 pl-3 flex items-center">
                <BaseIcon :path="mdiCloudOutline" size="20" class="text-slate-400" />
              </div>
              <FormControl
                v-model="create.redockPort"
                type="text"
                placeholder="8080"
                class="pl-10"
              />
            </div>
          </FormField>
        </form>
      </CardBoxModal>

      <!-- Edit Container Modal -->
      <CardBoxModal 
        v-model="isEditModalActive" 
        title="Edit Development Container" 
        button="success" 
        buttonLabel="Update Container"
        has-cancel
        @confirm="editSubmit"
      >
        <form class="space-y-6">
          <FormField label="Username">
            <div class="relative">
              <div class="absolute inset-y-0 left-0 pl-3 flex items-center">
                <BaseIcon :path="mdiAccountBox" size="20" class="text-slate-400" />
              </div>
              <FormControl v-model="modalPath.username" required class="pl-10" />
            </div>
          </FormField>

          <FormField label="Password">
            <div class="relative">
              <div class="absolute inset-y-0 left-0 pl-3 flex items-center">
                <BaseIcon :path="mdiKey" size="20" class="text-slate-400" />
              </div>
              <FormControl v-model="modalPath.password" type="password" required class="pl-10" />
            </div>
          </FormField>

          <FormField label="SSH Port">
            <div class="relative">
              <div class="absolute inset-y-0 left-0 pl-3 flex items-center">
                <BaseIcon :path="mdiEthernet" size="20" class="text-slate-400" />
              </div>
              <FormControl v-model="modalPath.port" type="text" required class="pl-10" />
            </div>
          </FormField>

          <FormField label="Redock Port">
            <div class="relative">
              <div class="absolute inset-y-0 left-0 pl-3 flex items-center">
                <BaseIcon :path="mdiCloudOutline" size="20" class="text-slate-400" />
              </div>
              <FormControl v-model="modalPath.redockPort" type="text" class="pl-10" />
            </div>
          </FormField>
        </form>
      </CardBoxModal>

      <!-- Delete Confirmation Modal -->
      <CardBoxModal 
        v-model="isDeleteModalActive" 
        title="Delete Container" 
        button="danger" 
        buttonLabel="Delete Container"
        has-cancel
        @confirm="deleteSubmit"
      >
        <div v-if="selectedContainer" class="space-y-4">
          <div class="bg-red-50 dark:bg-red-900/20 p-4 rounded-lg">
            <h4 class="font-semibold text-red-800 dark:text-red-200">{{ selectedContainer.username }}</h4>
            <p class="text-sm text-red-600 dark:text-red-300 mt-1">SSH Port: {{ selectedContainer.port }}</p>
          </div>
          
          <p class="text-slate-600 dark:text-slate-400">
            This will permanently delete the development container and all its data.
          </p>
          
          <div class="bg-yellow-50 dark:bg-yellow-900/20 p-4 rounded-lg">
            <div class="flex items-start">
              <BaseIcon :path="mdiDelete" size="20" class="text-yellow-600 dark:text-yellow-400 mt-0.5 mr-2 flex-shrink-0" />
              <p class="text-sm text-yellow-800 dark:text-yellow-200">
                <strong>Warning:</strong> This action cannot be undone. Make sure to backup any important data.
              </p>
            </div>
          </div>
        </div>
      </CardBoxModal>
    </div>
</template>

<style scoped>
/* Custom hover effects */
.hover\:scale-105:hover {
  transform: scale(1.05);
}

/* Loading spinner */
.animate-spin {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}
</style>