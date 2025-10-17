<script setup>
import BaseButton from "@/components/BaseButton.vue";
import BaseIcon from "@/components/BaseIcon.vue";
import CardBox from "@/components/CardBox.vue";
import CardBoxModal from "@/components/CardBoxModal.vue";
import FormControl from "@/components/FormControl.vue";
import FormField from "@/components/FormField.vue";

import ApiService from "@/services/ApiService";
import {
  mdiAlert, mdiCalendar,
  mdiCheckCircle,
  mdiChevronLeft,
  mdiChevronRight,
  mdiCloseCircle,
  mdiCloudUpload,
  mdiCog,
  mdiDelete,
  mdiGit,
  mdiHistory,
  mdiPencil,
  mdiPlus,
  mdiRefresh,
  mdiServer
} from "@mdi/js";
import { computed, onMounted, ref } from "vue";
// Reactive state
const deployments = ref([])
const loading = ref(false)
const isAddModalActive = ref(false)
const isEditModalActive = ref(false)
const isSettingsModalActive = ref(false)
const isDeleteModalActive = ref(false)
const selectedDeployment = ref(null)

// Pagination
const currentPage = ref(1)
const itemsPerPage = ref(10)

// Form data
const credentials = ref({
  username: '',
  token: '',
  checkTime: 60
})

const create = ref({
  name: '',
  path: '',
  url: '',
  branch: '',
  check: '',
  script: ''
})

const edit = ref({})

// Computed
const deploymentStats = computed(() => {
  const total = deployments.value.length
  const recent = deployments.value.filter(dep => {
    if (!dep.last_deployed) return false
    const lastDeployed = new Date(dep.last_deployed)
    const oneDayAgo = new Date(Date.now() - 24 * 60 * 60 * 1000)
    return lastDeployed > oneDayAgo
  }).length
  
  return { total, recent }
})

const paginatedDeployments = computed(() => {
  const start = (currentPage.value - 1) * itemsPerPage.value
  const end = start + itemsPerPage.value
  return deployments.value.slice(start, end)
})

const totalPages = computed(() => {
  return Math.ceil(deployments.value.length / itemsPerPage.value)
})

const paginationInfo = computed(() => {
  const start = (currentPage.value - 1) * itemsPerPage.value + 1
  const end = Math.min(start + itemsPerPage.value - 1, deployments.value.length)
  return `${start}-${end} of ${deployments.value.length}`
})

// Methods
const getList = async () => {
  loading.value = true
  try {
    const response = await ApiService.deploymentList()
    deployments.value = response.data.data || []
  } catch (error) {
    console.error('Error fetching deployments:', error)
  } finally {
    loading.value = false
  }
}

const deleteConfirm = (deployment) => {
  selectedDeployment.value = deployment
  modalDeleteActive.value = true
}

const confirmDelete = async () => {
  if (!selectedDeployment.value) return
  
  try {
    await ApiService.deploymentDelete({ path: selectedDeployment.value.path })
    await getList()
    isDeleteModalActive.value = false
    selectedDeployment.value = null
  } catch (error) {
    console.error('Error deleting deployment:', error)
  }
}

const addSubmit = async () => {
  try {
    await ApiService.deploymentAdd(create.value)
    isAddModalActive.value = false
    create.value = {
      name: '',
      path: '',
      url: '',
      branch: '',
      check: '',
      script: ''
    }
    await getList()
  } catch (error) {
    console.error('Error adding deployment:', error)
  }
}

const editModal = (deployment) => {
  edit.value = { ...deployment }
  isEditModalActive.value = true
}

const editSubmit = async () => {
  try {
    await ApiService.deploymentUpdate(edit.value)
    isEditModalActive.value = false
    await getList()
  } catch (error) {
    console.error('Error updating deployment:', error)
  }
}

const openSettingsModal = async () => {
  try {
    const res = await ApiService.deploymentGetSettings()
    if (res.data && res.data.data) {
      credentials.value = {
        username: res.data.data.username || '',
        token: res.data.data.token || '',
        checkTime: res.data.data.checkTime || 60
      }
    }
    isSettingsModalActive.value = true
  } catch (error) {
    console.error('Error fetching settings:', error)
  }
}

const saveCredentials = async () => {
  try {
    const data = {
      username: credentials.value.username,
      token: credentials.value.token,
      checkTime: parseInt(credentials.value.checkTime)
    }
    await ApiService.deploymentSetCredentials(data)
    isSettingsModalActive.value = false
  } catch (error) {
    console.error('Error saving credentials:', error)
  }
}

const formatDate = (dateStr) => {
  if (!dateStr) return "-"
  const date = new Date(dateStr)
  if (isNaN(date)) return dateStr
  return date.toLocaleString('tr-TR', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit'
  })
}

const getStatusBadgeClass = (deployment) => {
  if (!deployment.last_deployed) return 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300'
  
  const lastDeployed = new Date(deployment.last_deployed)
  const oneDayAgo = new Date(Date.now() - 24 * 60 * 60 * 1000)
  
  if (lastDeployed > oneDayAgo) {
    return 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-300'
  } else {
    return 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-300'
  }
}

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
  getList()
})
</script>

<template>
  <div class="space-y-6">
    <!-- Header -->
    <div class="bg-gradient-to-r from-blue-600 to-purple-600 rounded-xl p-6 text-white">
      <div class="flex flex-col lg:flex-row lg:items-center lg:justify-between">
        <div class="flex items-center space-x-4">
          <div class="w-12 h-12 bg-white/20 rounded-xl flex items-center justify-center backdrop-blur-sm">
            <BaseIcon :path="mdiGit" size="24" class="text-white" />
          </div>
          <div>
            <h1 class="text-2xl lg:text-3xl font-bold mb-2">Git Deployments</h1>
            <p class="text-blue-100">Manage your automated deployments</p>
          </div>
        </div>
        <div class="flex space-x-3 mt-4 lg:mt-0">
          <BaseButton
            :icon="mdiRefresh"
            label="Refresh"
            color="lightDark"
            @click="getList"
            :disabled="loading"
          />
          <BaseButton
            :icon="mdiCog"
            label="Settings"
            color="warning"
            @click="openSettingsModal"
          />
          <BaseButton
            :icon="mdiPlus"
            label="New Deployment"
            color="success"
            @click="isAddModalActive = true"
          />
        </div>
      </div>
    </div>

    <!-- Stats Cards -->
    <div class="grid grid-cols-1 md:grid-cols-3 gap-6">
      <!-- Total Deployments -->
      <CardBox class="bg-gradient-to-br from-blue-50 to-blue-100 dark:from-blue-900/20 dark:to-blue-800/20 border-blue-200 dark:border-blue-700">
        <div class="flex items-center justify-between">
              <div>
                <div class="text-2xl font-bold text-blue-600 dark:text-blue-400">{{ deploymentStats.total }}</div>
                <div class="text-sm text-blue-600/70 dark:text-blue-400/70">Total Deployments</div>
              </div>
              <BaseIcon :path="mdiServer" size="48" class="text-blue-500 opacity-20" />
            </div>
          </CardBox>

          <!-- Recent Deployments -->
          <CardBox class="bg-gradient-to-br from-green-50 to-green-100 dark:from-green-900/20 dark:to-green-800/20 border-green-200 dark:border-green-700">
            <div class="flex items-center justify-between">
              <div>
                <div class="text-2xl font-bold text-green-600 dark:text-green-400">{{ deploymentStats.recent }}</div>
                <div class="text-sm text-green-600/70 dark:text-green-400/70">Recent (24h)</div>
              </div>
              <BaseIcon :path="mdiHistory" size="48" class="text-green-500 opacity-20" />
            </div>
          </CardBox>

          <!-- Status -->
          <CardBox class="bg-gradient-to-br from-purple-50 to-purple-100 dark:from-purple-900/20 dark:to-purple-800/20 border-purple-200 dark:border-purple-700">
            <div class="flex items-center justify-between">
              <div>
                <div class="text-2xl font-bold text-purple-600 dark:text-purple-400">Active</div>
                <div class="text-sm text-purple-600/70 dark:text-purple-400/70">System Status</div>
              </div>
              <BaseIcon :path="mdiCloudUpload" size="48" class="text-purple-500 opacity-20" />
            </div>
          </CardBox>
        </div>

        <!-- Deployments List -->
        <CardBox>
          <div class="bg-gradient-to-r from-slate-50 to-slate-100 dark:from-slate-800 dark:to-slate-700 p-6 -m-6 mb-6">
            <div class="flex items-center justify-between">
              <div class="flex items-center space-x-3">
                <BaseIcon :path="mdiServer" size="24" class="text-slate-600 dark:text-slate-400" />
                <h3 class="text-lg font-semibold text-slate-900 dark:text-white">Deployment List</h3>
              </div>
            </div>
          </div>

          <div v-if="loading" class="text-center py-8">
            <div class="inline-flex items-center space-x-2 text-slate-600 dark:text-slate-400">
              <div class="animate-spin rounded-full h-5 w-5 border-b-2 border-blue-600"></div>
              <span>Loading deployments...</span>
            </div>
          </div>

          <div v-else-if="deployments.length === 0" class="text-center py-12">
            <BaseIcon :path="mdiServer" size="64" class="mx-auto text-slate-300 dark:text-slate-600 mb-4" />
            <h3 class="text-lg font-medium text-slate-900 dark:text-white mb-2">No deployments found</h3>
            <p class="text-slate-600 dark:text-slate-400 mb-6">Get started by creating your first deployment</p>
            <BaseButton
              :icon="mdiPlus"
              label="Create Deployment"
              color="success"
              @click="isAddModalActive = true"
            />
          </div>

          <div v-else class="space-y-4">
            <div 
              v-for="deployment in paginatedDeployments" 
              :key="deployment.path"
              class="bg-white dark:bg-slate-800 rounded-xl border border-slate-200 dark:border-slate-700 p-6 hover:shadow-lg transition-all duration-200"
            >
              <div class="flex items-start justify-between">
                <div class="flex-1">
                  <div class="flex items-center space-x-3 mb-3">
                    <div class="w-10 h-10 bg-gradient-to-br from-blue-500 to-purple-600 rounded-xl flex items-center justify-center">
                      <BaseIcon :path="mdiGit" size="20" class="text-white" />
                    </div>
                    <div>
                      <h4 class="text-lg font-semibold text-slate-900 dark:text-white">{{ deployment.path }}</h4>
                      <span 
                        :class="['inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium', getStatusBadgeClass(deployment)]"
                      >
                        <BaseIcon 
                          :path="deployment.last_deployed ? mdiCheckCircle : mdiCloseCircle" 
                          size="12" 
                          class="mr-1" 
                        />
                        {{ deployment.last_deployed ? 'Deployed' : 'Not Deployed' }}
                      </span>
                    </div>
                  </div>
                  
                  <div class="grid grid-cols-1 md:grid-cols-3 gap-4 text-sm">
                    <div class="flex items-center space-x-2">
                      <BaseIcon :path="mdiGit" size="16" class="text-slate-400" />
                      <span class="text-slate-600 dark:text-slate-400">Branch:</span>
                      <span class="font-medium text-slate-900 dark:text-white">{{ deployment.branch || '-' }}</span>
                    </div>
                    <div class="flex items-center space-x-2">
                      <BaseIcon :path="mdiCalendar" size="16" class="text-slate-400" />
                      <span class="text-slate-600 dark:text-slate-400">Last Deployed:</span>
                      <span class="font-medium text-slate-900 dark:text-white">{{ formatDate(deployment.last_deployed) }}</span>
                    </div>
                    <div class="flex items-center space-x-2">
                      <BaseIcon :path="mdiHistory" size="16" class="text-slate-400" />
                      <span class="text-slate-600 dark:text-slate-400">Last Checked:</span>
                      <span class="font-medium text-slate-900 dark:text-white">{{ formatDate(deployment.last_checked) }}</span>
                    </div>
                  </div>
                </div>
                
                <div class="flex space-x-2 ml-6">
                  <BaseButton
                    :icon="mdiPencil"
                    label="Edit"
                    color="info"
                    size="small"
                    @click="editModal(deployment)"
                  />
                  <BaseButton
                    :icon="mdiDelete"
                    label="Delete"
                    color="danger"
                    size="small"
                    @click="deleteDeployment(deployment)"
                  />
                </div>
              </div>
            </div>

            <!-- Pagination -->
            <div v-if="totalPages > 1" class="flex items-center justify-between mt-6 pt-6 border-t border-slate-200 dark:border-slate-700">
              <div class="text-sm text-slate-700 dark:text-slate-300">
                Showing {{ paginationInfo }}
              </div>
              
              <div class="flex items-center space-x-2">
                <BaseButton
                  :icon="mdiChevronLeft"
                  color="lightDark"
                  size="small"
                  @click="prevPage"
                  :disabled="currentPage === 1"
                />
                
                <div class="flex space-x-1">
                  <button
                    v-for="page in Math.min(totalPages, 5)"
                    :key="page"
                    @click="goToPage(page)"
                    :class="[
                      'px-3 py-2 text-sm font-medium rounded-lg transition-colors',
                      currentPage === page
                        ? 'bg-blue-600 text-white'
                        : 'text-slate-700 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800'
                    ]"
                  >
                    {{ page }}
                  </button>
                </div>
                
                <BaseButton
                  :icon="mdiChevronRight"
                  color="lightDark"
                  size="small"
                  @click="nextPage"
                  :disabled="currentPage === totalPages"
                />
              </div>
            </div>
          </div>
        </CardBox>

    <!-- Add Modal -->
    <CardBoxModal 
      v-model="isAddModalActive" 
      title="Create New Deployment"
      button="success"
      buttonLabel="Create Deployment"
      has-cancel
      @confirm="addSubmit"
    >
      <form class="space-y-6">
        <div class="bg-blue-50 dark:bg-blue-900/20 p-4 rounded-lg mb-6">
          <h4 class="font-semibold text-blue-800 dark:text-blue-200 mb-2 flex items-center">
            <BaseIcon :path="mdiGit" size="20" class="mr-2" />
            Deployment Configuration
          </h4>
          <p class="text-sm text-blue-600 dark:text-blue-300">
            Configure your Git repository and deployment settings.
          </p>
        </div>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
          <FormField label="Deployment Path">
            <FormControl 
              v-model="create.path" 
              type="input" 
              placeholder="/var/www/html/PROJECT"
              required
            />
          </FormField>
          
          <FormField label="Git Branch">
            <FormControl 
              v-model="create.branch" 
              type="input" 
              placeholder="main"
              required
            />
          </FormField>
        </div>

        <FormField label="Git Repository URL">
          <FormControl 
            v-model="create.url" 
            type="input" 
            placeholder="https://github.com/user/repo.git"
            required
          />
        </FormField>

        <FormField 
          label="Pre-deployment Check" 
          help="Command to run before deployment. Output must contain 'start_deployment' to proceed."
        >
          <FormControl 
            v-model="create.check" 
            type="textarea" 
            placeholder="Optional check command"
            :rows="3"
          />
        </FormField>

        <FormField label="Deployment Script">
          <FormControl 
            v-model="create.script" 
            type="textarea" 
            placeholder="#!/bin/bash&#10;git pull origin main&#10;npm install&#10;npm run build"
            :rows="5"
            required
          />
        </FormField>
      </form>
    </CardBoxModal>

    <!-- Edit Modal -->
    <CardBoxModal 
      v-model="isEditModalActive" 
      title="Edit Deployment"
      button="success"
      buttonLabel="Update Deployment"
      has-cancel
      @confirm="editSubmit"
    >
      <form class="space-y-6">
        <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
          <FormField label="Deployment Path">
            <FormControl 
              v-model="edit.path" 
              type="input" 
              placeholder="/var/www/html/PROJECT"
              required
            />
          </FormField>
          
          <FormField label="Git Branch">
            <FormControl 
              v-model="edit.branch" 
              type="input" 
              placeholder="main"
              required
            />
          </FormField>
        </div>

        <FormField label="Git Repository URL">
          <FormControl 
            v-model="edit.url" 
            type="input" 
            placeholder="https://github.com/user/repo.git"
            required
          />
        </FormField>

        <FormField 
          label="Pre-deployment Check" 
          help="Command to run before deployment. Output must contain 'start_deployment' to proceed."
        >
          <FormControl 
            v-model="edit.check" 
            type="textarea" 
            placeholder="Optional check command"
            :rows="3"
          />
        </FormField>

        <FormField label="Deployment Script">
          <FormControl 
            v-model="edit.script" 
            type="textarea" 
            placeholder="Deployment commands"
            :rows="5"
            required
          />
        </FormField>
      </form>
    </CardBoxModal>

    <!-- Settings Modal -->
    <CardBoxModal 
      v-model="isSettingsModalActive" 
      title="Deployment Settings"
      button="success"
      buttonLabel="Save Settings"
      has-cancel
      @confirm="saveCredentials"
    >
      <form class="space-y-6">
        <div class="bg-yellow-50 dark:bg-yellow-900/20 p-4 rounded-lg mb-6">
          <h4 class="font-semibold text-yellow-800 dark:text-yellow-200 mb-2 flex items-center">
            <BaseIcon :path="mdiCog" size="20" class="mr-2" />
            Git Credentials & Settings
          </h4>
          <p class="text-sm text-yellow-600 dark:text-yellow-300">
            Configure your Git credentials and deployment check interval.
          </p>
        </div>

        <FormField label="Username">
          <FormControl 
            v-model="credentials.username" 
            type="input" 
            placeholder="Git username"
          />
        </FormField>
        
        <FormField label="Access Token">
          <FormControl 
            v-model="credentials.token" 
            type="password" 
            placeholder="Personal access token"
          />
        </FormField>
        
        <FormField label="Check Interval (seconds)">
          <FormControl 
            v-model="credentials.checkTime" 
            type="number" 
            placeholder="60"
            min="30"
          />
        </FormField>
      </form>
    </CardBoxModal>

    <!-- Delete Confirmation Modal -->
    <CardBoxModal 
      v-model="isDeleteModalActive" 
      title="Delete Deployment"
      button="danger"
      buttonLabel="Delete"
      has-cancel
      @confirm="confirmDelete"
    >
      <div class="space-y-4">
        <div class="bg-red-50 dark:bg-red-900/20 p-4 rounded-lg">
          <div class="flex items-center space-x-3">
            <BaseIcon :path="mdiAlert" size="24" class="text-red-600 dark:text-red-400" />
            <div>
              <h4 class="font-semibold text-red-800 dark:text-red-200">Confirm Deletion</h4>
              <p class="text-sm text-red-600 dark:text-red-300">
                Are you sure you want to delete this deployment? This action cannot be undone.
              </p>
            </div>
          </div>
        </div>
        
        <div v-if="selectedDeployment" class="p-4 bg-slate-50 dark:bg-slate-800 rounded-lg">
          <p class="text-sm text-slate-600 dark:text-slate-400">Deployment Path:</p>
          <p class="font-medium text-slate-900 dark:text-white">{{ selectedDeployment.path }}</p>
        </div>
      </div>
    </CardBoxModal>
  </div>
</template>
