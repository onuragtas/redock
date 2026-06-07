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
  mdiAccountBox,
  mdiChevronLeft,
  mdiChevronRight,
  mdiCloudOutline,
  mdiConsole,
  mdiDelete,
  mdiDeveloperBoard,
  mdiDocker,
  mdiEthernet,
  mdiKey,
  mdiMagnify,
  mdiPencil,
  mdiPlus,
  mdiRefresh,
  mdiServer,
  mdiViewGridOutline,
  mdiViewList
} from "@mdi/js";
import { computed, onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { useI18n } from "vue-i18n";

const { t } = useI18n();
const router = useRouter();

// Reactive state
const personalContainers = ref([]);
const loading = ref(false);
const isAddModalActive = ref(false);
const isEditModalActive = ref(false);
const isDeleteModalActive = ref(false);
const selectedContainer = ref(null);

const create = ref({
  username: '',
  password: '',
  port: 0,
  redockPort: 0,
})

const modalPath = ref({
  username: '',
  password: '',
  port: '',
  redockPort: ''
});

// Computed
const containerStats = computed(() => {
  const total = personalContainers.value.length
  const withRedockPort = personalContainers.value.filter(container => container.redockPort).length
  
  return { total, withRedockPort }
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
  personalContainers,
  (container, query) => {
    const q = query.toLowerCase()
    return (
      container.username?.toLowerCase().includes(q) ||
      container.port?.toString().includes(q) ||
      container.redockPort?.toString().includes(q)
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

const layoutToggleLabel = computed(() => isGridLayout.value ? t('de.listView') : t('de.gridView'))
const layoutToggleIcon = computed(() => isGridLayout.value ? mdiViewList : mdiViewGridOutline)

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
    personalContainers.value = []
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
              <BaseIcon :path="mdiDocker" size="40" class="mr-4" />
              {{ t('de.title') }}
            </h1>
            <p class="text-emerald-100 text-lg">{{ t('de.subtitle') }}</p>
          </div>
          <div class="mt-6 lg:mt-0 flex flex-wrap gap-3">
            <BaseButton
              :label="t('common.refresh')"
              :icon="mdiRefresh"
              color="white"
              outline
              :disabled="loading"
              class="shadow-lg hover:shadow-xl transform hover:scale-105 transition-all duration-200"
              @click="getPersonalContainers"
            />
            <BaseButton
              :label="t('de.addContainer')"
              :icon="mdiPlus"
              color="white"
              class="shadow-lg hover:shadow-xl transform hover:scale-105 transition-all duration-200"
              @click="isAddModalActive = true"
            />
          </div>
        </div>
      </div>

      <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
        <CardBox class="bg-gradient-to-br from-emerald-50 to-emerald-100 dark:from-emerald-900/20 dark:to-emerald-800/20 border-emerald-200 dark:border-emerald-700">
          <div class="flex items-center justify-between">
            <div>
              <div class="text-2xl font-bold text-emerald-600 dark:text-emerald-400">{{ containerStats.total }}</div>
              <div class="text-sm text-emerald-600/70 dark:text-emerald-400/70">{{ t('de.totalContainers') }}</div>
            </div>
            <BaseIcon :path="mdiDocker" size="48" class="text-emerald-500 opacity-20" />
          </div>
        </CardBox>

        <CardBox class="bg-gradient-to-br from-teal-50 to-teal-100 dark:from-teal-900/20 dark:to-teal-800/20 border-teal-200 dark:border-teal-700">
          <div class="flex items-center justify-between">
            <div>
              <div class="text-2xl font-bold text-teal-600 dark:text-teal-400">{{ containerStats.withRedockPort }}</div>
              <div class="text-sm text-teal-600/70 dark:text-teal-400/70">{{ t('de.withRedockPort') }}</div>
            </div>
            <BaseIcon :path="mdiCloudOutline" size="48" class="text-teal-500 opacity-20" />
          </div>
        </CardBox>
      </div>

      <!-- Containers Table -->
      <CardBox>
        <SectionTitleLineWithButton :icon="mdiServer" :title="t('de.title')" main>
          <div class="flex flex-col gap-3 md:flex-row md:items-center">
            <div class="w-full md:w-64">
              <FormControl
                v-model="searchQuery"
                :icon="mdiMagnify"
                :placeholder="t('de.searchContainers')"
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
              @click="getPersonalContainers"
            />
          </div>
        </SectionTitleLineWithButton>

        <div v-if="loading" class="text-center py-12">
          <div class="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-emerald-600"></div>
          <p class="text-slate-500 dark:text-slate-400 mt-4">{{ t('de.loadingContainers') }}</p>
        </div>

        <div v-else-if="filteredItems.length === 0" class="text-center py-12">
          <BaseIcon :path="mdiDeveloperBoard" size="64" class="mx-auto text-slate-300 dark:text-slate-600 mb-4" />
          <p class="text-slate-500 dark:text-slate-400 mb-4">
            {{ searchQuery ? t('de.noMatch') : t('de.noContainers') }}
          </p>
          <BaseButton
            v-if="!searchQuery"
            :label="t('de.createFirst')"
            :icon="mdiPlus"
            color="success"
            @click="isAddModalActive = true"
          />
        </div>

  <div v-else :class="layoutClass">
          <div 
            v-for="container in paginatedItems" 
            :key="container.username"
            class="p-6 bg-slate-50 dark:bg-slate-800/50 rounded-xl hover:bg-slate-100 dark:hover:bg-slate-700/50 transition-colors flex flex-col h-full"
          >
            <div class="flex flex-col gap-6 lg:flex-row lg:items-start lg:justify-between">
              <div class="flex items-start gap-4 flex-1">
                <div class="flex-shrink-0">
                  <div class="w-12 h-12 bg-gradient-to-br from-emerald-500 to-teal-600 rounded-xl flex items-center justify-center">
                    <BaseIcon :path="mdiAccountBox" size="24" class="text-white" />
                  </div>
                </div>
                
                <div class="space-y-2 flex-1">
                  <h3 class="font-semibold text-lg">{{ container.username }}</h3>
                  <div class="flex flex-wrap items-center gap-x-4 gap-y-2 text-sm text-slate-500 dark:text-slate-400">
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
              </div>
              <div class="flex items-start lg:flex-none justify-start lg:justify-end">
                <span 
                  :class="[
                    'inline-flex items-center px-3 py-1 rounded-full text-xs font-medium',
                    container.redockPort 
                      ? 'text-emerald-600 bg-emerald-100 dark:text-emerald-400 dark:bg-emerald-900/30'
                      : 'text-yellow-600 bg-yellow-100 dark:text-yellow-400 dark:bg-yellow-900/30'
                  ]"
                >
                  {{ container.redockPort ? t('de.ready') : t('de.sshOnly') }}
                </span>
              </div>
            </div>
            
            <div class="mt-6 flex flex-wrap items-center justify-end gap-2">
              <BaseButton 
                :icon="mdiConsole" 
                color="info"
                small
                :title="t('de.sshAccess')"
                @click="openTerminal(container)"
              />
              
              <BaseButton 
                :icon="mdiPencil" 
                color="warning"
                small
                :title="t('common.edit')"
                @click="editModal(container)"
              />
              
              <BaseButton 
                :icon="mdiDelete" 
                color="danger"
                small
                :title="t('common.delete')"
                @click="deleteModal(container)"
              />
            </div>
          </div>
        </div>

        <!-- Pagination -->
        <div v-if="filteredItems.length > 0" class="flex flex-col gap-4 md:flex-row md:items-center md:justify-between mt-6 pt-6 border-t border-slate-200 dark:border-slate-700">
          <div class="text-sm text-slate-700 dark:text-slate-300">
            {{ t('de.showing', { info: paginationInfo }) }}
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
                :active="currentPage === page"
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

      <!-- Add Container Modal -->
      <CardBoxModal 
        v-model="isAddModalActive" 
        :title="t('de.addModalTitle')" 
        button="success" 
        :button-label="t('de.createContainer')"
        has-cancel
        @confirm="addSubmit"
      >
        <form class="space-y-6">
          <FormField :label="t('de.username')" :help="t('de.usernameHelp')">
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

          <FormField :label="t('de.password')" :help="t('de.passwordHelp')">
            <div class="relative">
              <div class="absolute inset-y-0 left-0 pl-3 flex items-center">
                <BaseIcon :path="mdiKey" size="20" class="text-slate-400" />
              </div>
              <FormControl
                v-model="create.password"
                type="password"
                :placeholder="t('de.enterPassword')"
                required
                class="pl-10 pr-12"
              />
              <button
                type="button"
                class="absolute inset-y-0 right-0 pr-3 flex items-center text-blue-600 hover:text-blue-800"
                :title="t('de.generatePassword')"
                @click="generatePassword"
              >
                <BaseIcon :path="mdiRefresh" size="20" />
              </button>
            </div>
          </FormField>

          <FormField :label="t('de.sshPort')" :help="t('de.sshPortHelp')">
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

          <FormField :label="t('de.redockPortOptional')" :help="t('de.redockPortHelp')">
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
        :title="t('de.editModalTitle')" 
        button="success" 
        :button-label="t('de.updateContainer')"
        has-cancel
        @confirm="editSubmit"
      >
        <form class="space-y-6">
          <FormField :label="t('de.username')">
            <div class="relative">
              <div class="absolute inset-y-0 left-0 pl-3 flex items-center">
                <BaseIcon :path="mdiAccountBox" size="20" class="text-slate-400" />
              </div>
              <FormControl v-model="modalPath.username" required class="pl-10" />
            </div>
          </FormField>

          <FormField :label="t('de.password')">
            <div class="relative">
              <div class="absolute inset-y-0 left-0 pl-3 flex items-center">
                <BaseIcon :path="mdiKey" size="20" class="text-slate-400" />
              </div>
              <FormControl v-model="modalPath.password" type="password" required class="pl-10" />
            </div>
          </FormField>

          <FormField :label="t('de.sshPort')">
            <div class="relative">
              <div class="absolute inset-y-0 left-0 pl-3 flex items-center">
                <BaseIcon :path="mdiEthernet" size="20" class="text-slate-400" />
              </div>
              <FormControl v-model="modalPath.port" type="text" required class="pl-10" />
            </div>
          </FormField>

          <FormField :label="t('de.redockPort')">
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
        :title="t('de.deleteModalTitle')" 
        button="danger" 
        :button-label="t('de.deleteContainer')"
        has-cancel
        @confirm="deleteSubmit"
      >
        <div v-if="selectedContainer" class="space-y-4">
          <div class="bg-red-50 dark:bg-red-900/20 p-4 rounded-lg">
            <h4 class="font-semibold text-red-800 dark:text-red-200">{{ selectedContainer.username }}</h4>
            <p class="text-sm text-red-600 dark:text-red-300 mt-1">{{ t('de.sshPortLabel', { port: selectedContainer.port }) }}</p>
          </div>
          
          <p class="text-slate-600 dark:text-slate-400">{{ t('de.deleteWarn') }}</p>
          
          <div class="bg-yellow-50 dark:bg-yellow-900/20 p-4 rounded-lg">
            <div class="flex items-start">
              <BaseIcon :path="mdiDelete" size="20" class="text-yellow-600 dark:text-yellow-400 mt-0.5 mr-2 flex-shrink-0" />
              <p class="text-sm text-yellow-800 dark:text-yellow-200" v-html="t('de.deleteWarnBold')"></p>
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