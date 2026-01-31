<script setup>
import { ref, computed, onMounted } from 'vue'
import { mdiDownload, mdiRocketLaunchOutline, mdiTestTube, mdiCheckCircle, mdiAlertCircle, mdiRefresh } from '@mdi/js'
import SectionMain from '@/components/SectionMain.vue'
import CardBox from '@/components/CardBox.vue'
import BaseLevel from '@/components/BaseLevel.vue'
import BaseButton from '@/components/BaseButton.vue'
import BaseIcon from '@/components/BaseIcon.vue'
import ModalConfirm from '@/components/ModalConfirm.vue'
import ApiService from '@/services/ApiService'
import { useToast } from 'vue-toastification'

const toast = useToast()

const loading = ref(false)
const updating = ref(false)
const currentVersion = ref(null)
const availableUpdates = ref([])
const countdown = ref(0)
const countdownInterval = ref(null)

// Modal state
const showConfirmModal = ref(false)
const selectedUpdate = ref(null)

// Fetch current version and available updates
const fetchUpdates = async () => {
  loading.value = true
  try {
    const response = await ApiService.getAvailableUpdates()
    if (!response.data.error) {
      currentVersion.value = response.data.data
      availableUpdates.value = response.data.data.updates || []
    } else {
      toast.error(response.data.msg)
    }
  } catch (error) {
    toast.error('Failed to fetch updates: ' + error.message)
  } finally {
    loading.value = false
  }
}

// Show confirmation modal
const confirmUpdate = (version) => {
  selectedUpdate.value = version
  showConfirmModal.value = true
}

// Apply update
const applyUpdate = async () => {
  const version = selectedUpdate.value
  showConfirmModal.value = false
  
  updating.value = true
  countdown.value = 30

  try {
    const response = await ApiService.applyUpdate(version)
    if (!response.data.error) {
      toast.success(response.data.msg)
      
      // Start countdown
      countdownInterval.value = setInterval(() => {
        countdown.value--
        if (countdown.value <= 0) {
          clearInterval(countdownInterval.value)
          // Refresh page after restart
          setTimeout(() => {
            window.location.reload()
          }, 5000)
        }
      }, 1000)
    } else {
      toast.error(response.data.msg)
      updating.value = false
    }
  } catch (error) {
    toast.error('Failed to apply update: ' + error.message)
    updating.value = false
  }
}

// Get modal config based on update type
const getModalConfig = computed(() => {
  if (!selectedUpdate.value) return {}
  
  const update = availableUpdates.value.find(u => u.tag === selectedUpdate.value)
  if (!update) return {}
  
  const isBeta = update.type === 'beta'
  const isRecommended = update.recommended
  
  return {
    type: isBeta ? 'warning' : isRecommended ? 'success' : 'info',
    title: isBeta ? '🧪 Beta Update' : isRecommended ? '⭐ Recommended Update' : '📦 Update Confirmation',
    message: `Are you sure you want to update to ${selectedUpdate.value}?\n\n${
      isBeta 
        ? '⚠️ This is a beta version and may contain bugs.\n' 
        : ''
    }The server will restart automatically (30 seconds downtime).`,
    confirmText: isBeta ? 'Try Beta' : 'Update Now'
  }
})

// Format date
const formatDate = (dateString) => {
  const date = new Date(dateString)
  return date.toLocaleDateString() + ' ' + date.toLocaleTimeString()
}

// Get version badge color
const getVersionBadge = (update) => {
  if (update.recommended) {
    return 'bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-300'
  }
  if (update.type === 'beta') {
    return 'bg-orange-100 dark:bg-orange-900/30 text-orange-700 dark:text-orange-300'
  }
  return 'bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300'
}

// Current version badge
const currentVersionBadge = computed(() => {
  if (!currentVersion.value) return ''
  if (currentVersion.value.is_beta) {
    return 'bg-orange-100 dark:bg-orange-900/30 text-orange-700 dark:text-orange-300 border-orange-300 dark:border-orange-700'
  }
  return 'bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-300 border-green-300 dark:border-green-700'
})

onMounted(() => {
  fetchUpdates()
})
</script>

<template>
  <SectionMain>
    <div class="mb-6">
      <h1 class="text-3xl font-bold mb-2">System Updates</h1>
      <p class="text-gray-600 dark:text-gray-400">Manage software updates and switch between stable and beta versions</p>
    </div>

    <!-- Current Version Card -->
    <CardBox class="mb-6" v-if="currentVersion">
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-4">
          <div class="p-3 rounded-lg" :class="currentVersionBadge">
            <BaseIcon :path="mdiCheckCircle" w="w-8" h="h-8" />
          </div>
          <div>
            <div class="text-sm text-gray-600 dark:text-gray-400">Current Version</div>
            <div class="text-2xl font-bold">{{ currentVersion.current_version }}</div>
            <div class="flex items-center gap-2 mt-1">
              <span
                class="px-2 py-0.5 text-xs rounded-full"
                :class="currentVersionBadge"
              >
                {{ currentVersion.is_beta ? 'Beta' : 'Stable' }}
              </span>
            </div>
          </div>
        </div>
        <BaseButton
          :icon="mdiRefresh"
          color="info"
          label="Check for Updates"
          @click="fetchUpdates"
          :disabled="loading || updating"
          rounded-full
        />
      </div>
    </CardBox>

    <!-- Updating Status -->
    <CardBox v-if="updating" class="mb-6 bg-yellow-50 dark:bg-yellow-900/20 border-yellow-300 dark:border-yellow-700">
      <div class="flex items-center gap-4">
        <div class="animate-spin">
          <BaseIcon :path="mdiRefresh" class="text-yellow-600" w="w-8" h="h-8" />
        </div>
        <div class="flex-1">
          <div class="font-semibold text-yellow-900 dark:text-yellow-100">Update in Progress</div>
          <div class="text-sm text-yellow-700 dark:text-yellow-300">
            Server will restart in {{ countdown }} seconds...
          </div>
          <div class="mt-2 h-2 bg-yellow-200 dark:bg-yellow-800 rounded-full overflow-hidden">
            <div
              class="h-full bg-yellow-600 transition-all duration-1000"
              :style="{ width: `${(countdown / 30) * 100}%` }"
            ></div>
          </div>
        </div>
      </div>
    </CardBox>

    <!-- Available Updates -->
    <div v-if="availableUpdates.length > 0">
      <h2 class="text-xl font-semibold mb-4">Available Updates ({{ availableUpdates.length }})</h2>
      
      <div class="space-y-4">
        <CardBox
          v-for="update in availableUpdates"
          :key="update.version"
          class="hover:shadow-lg transition-shadow"
        >
          <div class="flex items-start justify-between gap-4">
            <!-- Update Info -->
            <div class="flex-1">
              <div class="flex items-center gap-3 mb-2">
                <BaseIcon
                  :path="update.type === 'beta' ? mdiTestTube : mdiRocketLaunchOutline"
                  :class="update.type === 'beta' ? 'text-orange-600' : 'text-blue-600'"
                  w="w-6"
                  h="h-6"
                />
                <div>
                  <div class="flex items-center gap-2">
                    <span class="text-lg font-bold">{{ update.version }}</span>
                    <span
                      class="px-2 py-0.5 text-xs rounded-full"
                      :class="getVersionBadge(update)"
                    >
                      {{ update.type === 'beta' ? '🧪 Beta' : '✅ Stable' }}
                    </span>
                    <span
                      v-if="update.recommended"
                      class="px-2 py-0.5 text-xs rounded-full bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-300 font-semibold"
                    >
                      ⭐ Recommended
                    </span>
                  </div>
                  <div class="text-sm text-gray-600 dark:text-gray-400 mt-1">
                    {{ update.name }}
                  </div>
                  <div class="text-xs text-gray-500 dark:text-gray-500 mt-1">
                    Released: {{ formatDate(update.published_at) }}
                  </div>
                </div>
              </div>

              <!-- Description -->
              <div
                v-if="update.description"
                class="mt-3 p-3 bg-gray-50 dark:bg-gray-900/50 rounded-lg text-sm text-gray-700 dark:text-gray-300 max-h-32 overflow-y-auto"
              >
                <div class="prose dark:prose-invert max-w-none text-sm" v-html="update.description"></div>
              </div>
            </div>

            <!-- Actions -->
            <div class="flex flex-col gap-2">
              <BaseButton
                :icon="mdiDownload"
                :color="update.recommended ? 'success' : update.type === 'beta' ? 'warning' : 'info'"
                :label="update.recommended ? 'Update Now' : update.type === 'beta' ? 'Try Beta' : 'Update'"
                @click="confirmUpdate(update.tag)"
                :disabled="updating || loading"
                rounded-full
              />
            </div>
          </div>

          <!-- Beta Warning -->
          <div
            v-if="update.type === 'beta'"
            class="mt-4 p-3 bg-orange-50 dark:bg-orange-900/20 border border-orange-200 dark:border-orange-800 rounded-lg flex items-start gap-2"
          >
            <BaseIcon :path="mdiAlertCircle" class="text-orange-600 flex-shrink-0" w="w-5" h="h-5" />
            <div class="text-xs text-orange-800 dark:text-orange-200">
              <strong>Beta Warning:</strong> This is a pre-release version that may contain bugs. 
              Not recommended for production use. You can always switch back to stable versions.
            </div>
          </div>
        </CardBox>
      </div>
    </div>

    <!-- No Updates Available -->
    <CardBox v-else-if="!loading && currentVersion" class="text-center py-12">
      <BaseIcon :path="mdiCheckCircle" class="text-green-600 mx-auto mb-4" w="w-16" h="w-16" />
      <h3 class="text-xl font-semibold mb-2">You're Up to Date!</h3>
      <p class="text-gray-600 dark:text-gray-400">
        No updates available for your current version
      </p>
    </CardBox>

    <!-- Loading -->
    <CardBox v-else-if="loading" class="text-center py-12">
      <div class="animate-spin mx-auto mb-4">
        <BaseIcon :path="mdiRefresh" class="text-blue-600" w="w-12" h="w-12" />
      </div>
      <p class="text-gray-600 dark:text-gray-400">Checking for updates...</p>
    </CardBox>

    <!-- Info Card -->
    <CardBox class="mt-6 bg-blue-50 dark:bg-blue-900/20 border-blue-200 dark:border-blue-800">
      <div class="flex items-start gap-3">
        <BaseIcon :path="mdiAlertCircle" class="text-blue-600 flex-shrink-0" w="w-5" h="h-5" />
        <div class="text-sm text-blue-800 dark:text-blue-200">
          <strong>About Updates:</strong>
          <ul class="mt-2 space-y-1 list-disc list-inside">
            <li><strong>Stable versions</strong> are thoroughly tested and recommended for production</li>
            <li><strong>Beta versions</strong> include new features but may have bugs</li>
            <li>The server will restart automatically after update (30 seconds downtime)</li>
            <li>You can switch between beta and stable versions anytime</li>
            <li>Updates are applied using graceful restart to minimize downtime</li>
          </ul>
        </div>
      </div>
    </CardBox>

    <!-- Confirmation Modal -->
    <ModalConfirm
      v-model="showConfirmModal"
      :type="getModalConfig.type"
      :title="getModalConfig.title"
      :message="getModalConfig.message"
      :confirm-text="getModalConfig.confirmText"
      cancel-text="Cancel"
      @confirm="applyUpdate"
    />
  </SectionMain>
</template>
