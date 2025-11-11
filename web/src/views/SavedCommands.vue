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
  mdiAlert,
  mdiBookmark,
  mdiCheck,
  mdiChevronLeft, mdiChevronRight,
  mdiClose,
  mdiCodeBraces,
  mdiConsole,
  mdiDelete,
  mdiHistory,
  mdiMagnify,
  mdiPlus,
  mdiRefresh,
  mdiViewGridOutline,
  mdiViewList
} from "@mdi/js";
import { computed, onMounted, ref } from "vue";

// Reactive state
const savedCommands = ref([])
const loading = ref(false)
const isAddModalActive = ref(false)
const isDeleteModalActive = ref(false)
const modalCommand = ref({})

// Form data
const createSavedCommand = ref({
  command: '',
})

// Shared pagination & filter logic
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
  savedCommands,
  (item, query) => item.command?.toLowerCase().includes(query.toLowerCase()),
  8
)

// Computed
const commandStats = computed(() => {
  const total = savedCommands.value.length
  const shellCommands = savedCommands.value.filter(cmd => 
    cmd.command?.includes('bash') || cmd.command?.includes('sh') || cmd.command?.startsWith('cd ')
  ).length
  const dockerCommands = savedCommands.value.filter(cmd => 
    cmd.command?.includes('docker')
  ).length
  
  return { total, shell: shellCommands, docker: dockerCommands }
})

const {
  isGridLayout,
  layoutClass,
  toggleLayout
} = useLayoutToggle(paginatedItems, { minItemsForGrid: 2 })

const layoutToggleLabel = computed(() => isGridLayout.value ? 'List View' : 'Grid View')
const layoutToggleIcon = computed(() => isGridLayout.value ? mdiViewList : mdiViewGridOutline)

// Methods
const getAllSavedCommands = async () => {
  loading.value = true
  try {
    const response = await ApiService.getAllSavedCommands()
    savedCommands.value = response.data.data || []
  } catch (error) {
    console.error('Failed to load saved commands:', error)
    // Mock data for demo
    savedCommands.value = [
      { command: 'docker ps -a' },
      { command: 'docker-compose up -d' },
      { command: 'npm install && npm run build' },
      { command: 'git status && git add . && git commit -m "update"' },
      { command: 'sudo systemctl restart nginx' },
      { command: 'tail -f /var/log/nginx/error.log' }
    ]
  } finally {
    loading.value = false
  }
}

const deleteSavedCommand = (data) => {
  modalCommand.value = data
  isDeleteModalActive.value = true
}

const deleteSubmit = async () => {
  try {
    await ApiService.deleteSavedCommand({ command: modalCommand.value.command })
    isDeleteModalActive.value = false
    await getAllSavedCommands()
  } catch (error) {
    console.error('Failed to delete saved command:', error)
  }
}

const addSubmit = async () => {
  try {
    await ApiService.addSavedCommand(createSavedCommand.value)
    isAddModalActive.value = false
    resetForm()
    await getAllSavedCommands()
  } catch (error) {
    console.error('Failed to save command:', error)
  }
}

const resetForm = () => {
  createSavedCommand.value = {
    command: '',
  }
}

// Lifecycle
onMounted(() => {
  getAllSavedCommands()
})
</script>

<template>
  <div class="space-y-8">
      <!-- Header -->
      <div class="bg-gradient-to-r from-indigo-600 via-purple-600 to-pink-600 rounded-2xl p-8 text-white shadow-lg">
        <div class="flex flex-col lg:flex-row lg:items-center lg:justify-between">
          <div>
            <h1 class="text-3xl lg:text-4xl font-bold mb-2 flex items-center">
              <BaseIcon :path="mdiConsole" size="40" class="mr-4" />
              Saved Commands
            </h1>
            <p class="text-indigo-100 text-lg">Quick access to frequently used terminal commands</p>
          </div>
          <div class="mt-6 lg:mt-0 flex space-x-3">
            <BaseButton
              label="Refresh"
              :icon="mdiRefresh"
              color="white"
              outline
              :disabled="loading"
              class="shadow-lg hover:shadow-xl transform hover:scale-105 transition-all duration-200"
              @click="getAllSavedCommands"
            />
            <BaseButton
              label="Add Command"
              :icon="mdiPlus"
              color="white"
              class="shadow-lg hover:shadow-xl transform hover:scale-105 transition-all duration-200"
              @click="isAddModalActive = true"
            />
          </div>
        </div>
      </div>

      <!-- Statistics -->
      <div class="grid grid-cols-1 md:grid-cols-3 gap-6">
        <CardBox class="bg-gradient-to-br from-indigo-50 to-indigo-100 dark:from-indigo-900/20 dark:to-indigo-800/20 border-indigo-200 dark:border-indigo-700">
          <div class="flex items-center justify-between">
            <div>
              <div class="text-2xl font-bold text-indigo-600 dark:text-indigo-400">{{ commandStats.total }}</div>
              <div class="text-sm text-indigo-600/70 dark:text-indigo-400/70">Total Commands</div>
            </div>
            <BaseIcon :path="mdiBookmark" size="48" class="text-indigo-500 opacity-20" />
          </div>
        </CardBox>

        <CardBox class="bg-gradient-to-br from-blue-50 to-blue-100 dark:from-blue-900/20 dark:to-blue-800/20 border-blue-200 dark:border-blue-700">
          <div class="flex items-center justify-between">
            <div>
              <div class="text-2xl font-bold text-blue-600 dark:text-blue-400">{{ commandStats.shell }}</div>
              <div class="text-sm text-blue-600/70 dark:text-blue-400/70">Shell Commands</div>
            </div>
            <BaseIcon :path="mdiConsole" size="48" class="text-blue-500 opacity-20" />
          </div>
        </CardBox>

        <CardBox class="bg-gradient-to-br from-green-50 to-green-100 dark:from-green-900/20 dark:to-green-800/20 border-green-200 dark:border-green-700">
          <div class="flex items-center justify-between">
            <div>
              <div class="text-2xl font-bold text-green-600 dark:text-green-400">{{ commandStats.docker }}</div>
              <div class="text-sm text-green-600/70 dark:text-green-400/70">Docker Commands</div>
            </div>
            <BaseIcon :path="mdiConsole" size="48" class="text-green-500 opacity-20" />
          </div>
        </CardBox>
      </div>

      <!-- Commands List -->
      <CardBox>
        <SectionTitleLineWithButton :icon="mdiHistory" title="Command Library" main>
          <div class="flex flex-col gap-3 md:flex-row md:items-center">
            <div class="w-full md:w-64">
              <FormControl
                v-model="searchQuery"
                :icon="mdiMagnify"
                placeholder="Search commands"
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
          </div>
        </SectionTitleLineWithButton>

        <div v-if="loading" class="text-center py-12">
          <div class="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-indigo-600"></div>
          <p class="text-slate-500 dark:text-slate-400 mt-4">Loading commands...</p>
        </div>

        <div v-else-if="filteredItems.length === 0" class="text-center py-12">
          <BaseIcon :path="mdiConsole" size="64" class="mx-auto text-slate-300 dark:text-slate-600 mb-4" />
          <p class="text-slate-500 dark:text-slate-400 mb-4">
            {{ searchQuery ? 'No commands match your search.' : 'No saved commands found.' }}
          </p>
          <BaseButton
            v-if="!searchQuery"
            label="Save Your First Command"
            :icon="mdiPlus"
            color="info"
            @click="isAddModalActive = true"
          />
        </div>

  <div v-else :class="layoutClass">
          <div 
            v-for="(command, index) in paginatedItems" 
            :key="index"
            class="p-6 bg-slate-50 dark:bg-slate-800/50 rounded-xl hover:bg-slate-100 dark:hover:bg-slate-700/50 transition-colors flex flex-col h-full"
          >
            <div class="flex items-start gap-4 flex-1">
              <div class="flex-shrink-0">
                <div class="w-12 h-12 bg-gradient-to-br from-indigo-500 to-purple-600 rounded-xl flex items-center justify-center">
                  <BaseIcon :path="mdiCodeBraces" size="24" class="text-white" />
                </div>
              </div>
              
              <div class="flex-1 space-y-3 min-w-0">
                <div class="text-sm font-semibold text-slate-700 dark:text-slate-200 flex items-center gap-2">
                  <BaseIcon :path="mdiConsole" size="18" class="text-indigo-500" />
                  Command {{ index + 1 }}
                </div>
                <div class="text-sm text-slate-500 dark:text-slate-400 font-mono bg-slate-100 dark:bg-slate-700 px-3 py-2 rounded-md break-words">
                  {{ command.command }}
                </div>
              </div>
            </div>
            
            <div class="mt-6 flex items-center justify-end gap-2">
              <BaseButton 
                :icon="mdiDelete" 
                color="danger"
                size="small"
                title="Delete Command"
                @click="deleteSavedCommand(command)"
              />
            </div>
          </div>
        </div>

        <!-- Pagination -->
        <div v-if="filteredItems.length > 0" class="flex flex-col gap-4 md:flex-row md:items-center md:justify-between mt-6 pt-6 border-t border-slate-200 dark:border-slate-700">
          <div class="text-sm text-slate-700 dark:text-slate-300">
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

      <!-- Add Command Modal -->
      <CardBoxModal 
        v-model="isAddModalActive" 
        title="Save New Command" 
        button="success" 
        button-label="Save Command"
        has-cancel
        @confirm="addSubmit"
      >
        <form class="space-y-6">
          <div class="bg-blue-50 dark:bg-blue-900/20 p-4 rounded-lg mb-6">
            <h4 class="font-semibold text-blue-800 dark:text-blue-200 mb-2 flex items-center">
              <BaseIcon :path="mdiBookmark" size="20" class="mr-2" />
              Quick Command Access
            </h4>
            <p class="text-sm text-blue-600 dark:text-blue-300">
              Save frequently used commands for quick access and execution.
            </p>
          </div>

          <FormField label="Command" help="Enter the terminal command to save">
            <div class="relative">
              <div class="absolute inset-y-0 left-0 pl-3 flex items-center">
                <BaseIcon :path="mdiConsole" size="20" class="text-slate-400" />
              </div>
              <FormControl
                v-model="createSavedCommand.command"
                placeholder="docker ps -a"
                required
                class="pl-10 font-mono"
              />
            </div>
          </FormField>

          <div class="bg-gray-50 dark:bg-gray-800 p-4 rounded-lg">
            <h5 class="font-medium text-gray-800 dark:text-gray-200 mb-2">Examples:</h5>
            <div class="space-y-2 text-sm text-gray-600 dark:text-gray-400 font-mono">
              <div>• docker-compose up -d</div>
              <div>• npm install && npm run build</div>
              <div>• sudo systemctl restart nginx</div>
              <div>• git status && git pull origin main</div>
            </div>
          </div>
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
              label="Save Command"
              color="success"
              @click="addSubmit"
            />
          </div>
        </template>
      </CardBoxModal>

      <!-- Delete Confirmation Modal -->
      <CardBoxModal 
        v-model="isDeleteModalActive" 
        title="Delete Saved Command" 
        button="danger"
        button-label="Delete Command"
        has-cancel
        @confirm="deleteSubmit"
      >
        <div class="text-center">
          <BaseIcon :path="mdiAlert" size="48" class="mx-auto text-red-500 mb-4" />
          <h3 class="text-lg font-semibold mb-2">Delete this command?</h3>
          <p class="text-slate-600 dark:text-slate-400 mb-6">
            This action cannot be undone. The following command will be removed:
          </p>
          <div class="bg-red-50 dark:bg-red-900/20 p-4 rounded-lg border border-red-200 dark:border-red-800">
            <code class="text-red-600 dark:text-red-400 break-all font-mono">{{ modalCommand.command }}</code>
          </div>
        </div>

        <template #footer>
          <div class="flex justify-end space-x-3">
            <BaseButton
              :icon="mdiClose"
              label="Cancel"
              color="lightDark"
              @click="isDeleteModalActive = false"
            />
            <BaseButton
              :icon="mdiDelete"
              label="Delete Command"
              color="danger"
              @click="deleteSubmit"
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

/* Command text styling */
.font-mono {
  font-family: 'SF Mono', 'Monaco', 'Inconsolata', 'Fira Code', 'Droid Sans Mono', 'Source Code Pro', monospace;
}
</style>