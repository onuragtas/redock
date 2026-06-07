<script setup>
import BaseButton from "@/components/BaseButton.vue";
import BaseIcon from "@/components/BaseIcon.vue";
import CardBox from "@/components/CardBox.vue";
import CardBoxModal from "@/components/CardBoxModal.vue";
import FormControl from "@/components/FormControl.vue";
import FormField from "@/components/FormField.vue";
import SectionTitleLineWithButton from "@/components/SectionTitleLineWithButton.vue";

import ApiService from "@/services/ApiService";
import {
  mdiAlertCircleOutline,
  mdiCheckCircleOutline,
  mdiContentCopy,
  mdiDelete,
  mdiIncognito,
  mdiPencil,
  mdiPlus,
  mdiRefresh,
  mdiRouter
} from '@mdi/js';
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";

const { t } = useI18n();

// State
const status = ref(null)
const list = ref([])
const routes = ref([])
const loading = ref(false)
const submitting = ref(false)
const isAddModalActive = ref(false)
const isEditModalActive = ref(false)
const isDeleteModalActive = ref(false)
const selected = ref(null)
const copyToast = ref('')
const lastError = ref('')

// Form (create + edit ortak kullanır). editingId null ise create modu.
const editingId = ref(null)
const form = ref({
  name: '',
  route_id: '',
  enabled: true,
})

const torInstalled = computed(() => status.value?.installed === true)
const torRunning = computed(() => status.value?.tor_running === true)
const systemServiceConflict = computed(() => status.value?.system_service_conflict === true)
const noRoutes = computed(() => routes.value.length === 0)

// Loaders
const loadStatus = async () => {
  try {
    const res = await ApiService.onionStatus()
    status.value = res.data.data
  } catch (e) {
    lastError.value = e?.response?.data?.msg || e.message
  }
}

const loadList = async () => {
  try {
    const res = await ApiService.onionList()
    list.value = res.data.data || []
  } catch (e) {
    lastError.value = e?.response?.data?.msg || e.message
  }
}

const loadRoutes = async () => {
  try {
    const res = await ApiService.apiGatewayGetConfig()
    routes.value = res.data?.data?.routes || []
  } catch {
    routes.value = []
  }
}

const refreshAll = async () => {
  loading.value = true
  try {
    await Promise.all([loadStatus(), loadList(), loadRoutes()])
  } finally {
    loading.value = false
  }
}

// Create
const openCreate = () => {
  editingId.value = null
  form.value = {
    name: '',
    route_id: routes.value[0]?.id || '',
    enabled: true,
  }
  lastError.value = ''
  isAddModalActive.value = true
}

const submitCreate = async () => {
  submitting.value = true
  lastError.value = ''
  try {
    await ApiService.onionCreate({
      name: form.value.name,
      route_id: form.value.route_id,
    })
    isAddModalActive.value = false
    await refreshAll()
  } catch (e) {
    lastError.value = e?.response?.data?.msg || e.message || 'Create failed'
  } finally {
    submitting.value = false
  }
}

// Edit
const openEdit = (item) => {
  editingId.value = item.id
  form.value = {
    name: item.name || '',
    route_id: item.route_id || '',
    enabled: item.enabled,
  }
  lastError.value = ''
  isEditModalActive.value = true
}

const submitEdit = async () => {
  submitting.value = true
  lastError.value = ''
  try {
    await ApiService.onionUpdate(editingId.value, {
      name: form.value.name,
      route_id: form.value.route_id,
      enabled: form.value.enabled,
    })
    isEditModalActive.value = false
    await refreshAll()
  } catch (e) {
    lastError.value = e?.response?.data?.msg || e.message || 'Update failed'
  } finally {
    submitting.value = false
  }
}

// Inline enable toggle (her item'da switch)
const toggleEnabled = async (item) => {
  try {
    await ApiService.onionUpdate(item.id, { enabled: !item.enabled })
    await loadList()
  } catch (e) {
    lastError.value = e?.response?.data?.msg || e.message
  }
}

// Delete
const openDelete = (item) => {
  selected.value = item
  isDeleteModalActive.value = true
}

const confirmDelete = async () => {
  if (!selected.value) return
  try {
    await ApiService.onionDelete(selected.value.id)
    isDeleteModalActive.value = false
    selected.value = null
    await refreshAll()
  } catch (e) {
    lastError.value = e?.response?.data?.msg || e.message
  }
}

// Misc
const copy = async (text) => {
  try {
    await navigator.clipboard.writeText(text)
    copyToast.value = t('common.copied')
  } catch {
    copyToast.value = t('common.copyFailed')
  }
  setTimeout(() => (copyToast.value = ''), 1500)
}

const routeLabel = (routeId) => {
  const r = routes.value.find(x => x.id === routeId)
  if (!r) return routeId
  return r.name ? `${r.name} (${r.id})` : r.id
}

// Install polling
let installPollHandle = null
const startInstallPoll = () => {
  if (installPollHandle) return
  installPollHandle = setInterval(loadStatus, 5000)
}
const stopInstallPoll = () => {
  if (!installPollHandle) return
  clearInterval(installPollHandle)
  installPollHandle = null
}

watch(torInstalled, (installed) => {
  if (installed) {
    stopInstallPoll()
    loadList()
    loadRoutes()
  } else if (status.value) {
    startInstallPoll()
  }
})

onMounted(refreshAll)
onUnmounted(stopInstallPoll)
</script>

<template>
  <div class="space-y-8">
    <!-- Header -->
    <div class="bg-gradient-to-r from-purple-700 via-indigo-700 to-blue-700 rounded-2xl p-8 text-white shadow-lg">
      <div class="flex flex-col lg:flex-row lg:items-center lg:justify-between">
        <div>
          <h1 class="text-3xl lg:text-4xl font-bold mb-2 flex items-center">
            <BaseIcon :path="mdiIncognito" size="40" class="mr-4" />
            {{ t('os.title') }}
          </h1>
          <p class="text-indigo-100 text-lg">
            {{ t('os.subtitle') }}
          </p>
        </div>
        <div class="mt-6 lg:mt-0 flex space-x-3">
          <BaseButton :icon="mdiRefresh" color="white" outline :disabled="loading" @click="refreshAll" />
          <BaseButton
            :label="t('os.addOnion')"
            :icon="mdiPlus"
            color="white"
            :disabled="!torInstalled || noRoutes || systemServiceConflict"
            @click="openCreate"
          />
        </div>
      </div>
    </div>

    <!-- Tor not installed -->
    <CardBox v-if="status && !torInstalled" class="border-l-4 border-amber-500">
      <div class="flex items-start gap-4">
        <BaseIcon :path="mdiAlertCircleOutline" size="32" class="text-amber-500 flex-shrink-0" />
        <div class="flex-1">
          <h3 class="font-semibold text-lg mb-2">{{ t('os.torNotInstalled') }}</h3>
          <p class="text-slate-600 dark:text-slate-300 mb-4" v-html="t('os.torNeedsDaemon')"></p>
          <div v-if="status.install_hint" class="bg-slate-900 text-slate-100 rounded-lg p-4 font-mono text-sm whitespace-pre overflow-x-auto">{{ status.install_hint.command }}</div>
          <div v-if="status.install_hint?.url" class="mt-3">
            <a
:href="status.install_hint.url" target="_blank" rel="noopener"
               class="text-indigo-600 dark:text-indigo-400 hover:underline text-sm">
              {{ t('os.documentation') }} {{ status.install_hint.url }}
            </a>
          </div>
          <div class="mt-4 text-xs text-slate-500">
            {{ t('os.detectedOs') }} <code>{{ status.install_hint?.os || '?' }}</code> · {{ t('os.arch') }}
            <code>{{ status.install_hint?.arch || '?' }}</code>
          </div>
        </div>
      </div>
    </CardBox>

    <!-- System tor service conflict -->
    <CardBox v-if="torInstalled && systemServiceConflict" class="border-l-4 border-red-500">
      <div class="flex items-start gap-3">
        <BaseIcon :path="mdiAlertCircleOutline" size="24" class="text-red-500 flex-shrink-0" />
        <div class="flex-1">
          <h3 class="font-semibold">{{ t('os.conflictTitle') }}</h3>
          <p class="text-sm text-slate-600 dark:text-slate-300 mt-1 mb-3" v-html="t('os.conflictText')"></p>
          <div class="bg-slate-900 text-slate-100 rounded-lg p-3 font-mono text-sm">sudo systemctl disable --now tor</div>
        </div>
      </div>
    </CardBox>

    <!-- No routes warning -->
    <CardBox v-if="torInstalled && noRoutes" class="border-l-4 border-amber-500">
      <div class="flex items-start gap-3">
        <BaseIcon :path="mdiAlertCircleOutline" size="24" class="text-amber-500 flex-shrink-0" />
        <div>
          <h3 class="font-semibold">{{ t('os.noRoutesTitle') }}</h3>
          <p class="text-sm text-slate-600 dark:text-slate-300 mt-1">
            {{ t('os.noRoutesText') }}
          </p>
        </div>
      </div>
    </CardBox>

    <!-- Status pills -->
    <div v-if="torInstalled" class="grid grid-cols-1 md:grid-cols-3 gap-4">
      <CardBox class="bg-gradient-to-br from-emerald-50 to-emerald-100 dark:from-emerald-900/20 dark:to-emerald-800/20 border-emerald-200 dark:border-emerald-700">
        <div class="flex items-center gap-3">
          <BaseIcon :path="mdiCheckCircleOutline" size="32" class="text-emerald-500" />
          <div>
            <div class="font-semibold">{{ t('os.torInstalled') }}</div>
            <div class="text-xs text-slate-500 truncate" :title="status.binary_path">{{ status.binary_path }}</div>
            <div class="text-xs text-slate-400">{{ status.version }}</div>
          </div>
        </div>
      </CardBox>
      <CardBox>
        <div class="text-sm text-slate-500">{{ t('os.torDaemon') }}</div>
        <div class="text-xl font-semibold mt-1">
          <span v-if="torRunning" class="text-emerald-600">{{ t('os.running') }}</span>
          <span v-else class="text-slate-500">{{ t('os.idleStarts') }}</span>
        </div>
      </CardBox>
      <CardBox>
        <div class="text-sm text-slate-500">{{ t('os.configuredServices') }}</div>
        <div class="text-2xl font-bold mt-1">{{ status.onion_count }}</div>
      </CardBox>
    </div>

    <!-- List -->
    <CardBox v-if="torInstalled">
      <SectionTitleLineWithButton :icon="mdiIncognito" :title="t('os.hiddenServices')" main />

      <div v-if="loading" class="text-center py-12">
        <div class="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-indigo-600"></div>
        <p class="text-slate-500 mt-4">{{ t('common.loading') }}</p>
      </div>

      <div v-else-if="list.length === 0" class="text-center py-12">
        <BaseIcon :path="mdiIncognito" size="64" class="mx-auto text-slate-300 dark:text-slate-600 mb-4" />
        <p class="text-slate-500 mb-4">{{ t('os.noOnion') }}</p>
        <BaseButton
          :label="t('os.createFirst')"
          :icon="mdiPlus"
          color="info"
          :disabled="noRoutes"
          @click="openCreate"
        />
      </div>

      <div v-else class="space-y-3">
        <div
          v-for="item in list"
          :key="item.id"
          class="p-5 bg-slate-50 dark:bg-slate-800/50 rounded-xl flex flex-col lg:flex-row lg:items-center lg:justify-between gap-4"
        >
          <div class="flex-1 min-w-0">
            <div class="flex items-center gap-2 mb-1">
              <span class="font-semibold text-lg">{{ item.name }}</span>
              <span
                v-if="item.enabled"
                class="text-xs px-2 py-0.5 rounded-full bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400"
              >{{ t('common.enabled') }}</span>
              <span
                v-else
                class="text-xs px-2 py-0.5 rounded-full bg-slate-200 text-slate-600 dark:bg-slate-700 dark:text-slate-300"
              >{{ t('common.disabled') }}</span>
            </div>
            <div class="flex items-center gap-2 text-sm font-mono break-all">
              <span class="text-indigo-700 dark:text-indigo-400">{{ item.onion_address || t('os.notPublished') }}</span>
              <button
                v-if="item.onion_address"
                class="text-slate-400 hover:text-indigo-500"
                :title="t('common.copy')"
                @click="copy(item.onion_address)"
              >
                <BaseIcon :path="mdiContentCopy" size="16" />
              </button>
            </div>
            <div class="text-xs text-slate-500 mt-2 flex flex-wrap gap-x-4 gap-y-1">
              <span v-if="item.route_id">
                <BaseIcon :path="mdiRouter" size="12" class="inline mr-1" />
                {{ t('os.gatewayRoutePrefix') }} <code>{{ routeLabel(item.route_id) }}</code>
              </span>
              <span v-else>
                {{ t('os.direct') }} <code>{{ item.target_host }}:{{ item.target_port }}</code>
              </span>
              <span>{{ t('os.virtualPort') }} <code>{{ item.virtual_port }}</code></span>
            </div>
          </div>

          <div class="flex items-center gap-3">
            <!-- Enable/disable switch -->
            <label class="inline-flex items-center cursor-pointer" :title="item.enabled ? t('os.disable') : t('os.enable')">
              <input
                type="checkbox"
                class="sr-only peer"
                :checked="item.enabled"
                @change="toggleEnabled(item)"
              />
              <div class="w-10 h-5 bg-slate-300 dark:bg-slate-600 rounded-full peer peer-checked:bg-emerald-500 transition-colors relative">
                <div class="absolute top-0.5 left-0.5 bg-white w-4 h-4 rounded-full transition-transform peer-checked:translate-x-5"></div>
              </div>
            </label>
            <BaseButton :icon="mdiPencil" color="info" small :title="t('common.edit')" @click="openEdit(item)" />
            <BaseButton :icon="mdiDelete" color="danger" small :title="t('common.delete')" @click="openDelete(item)" />
          </div>
        </div>
      </div>
    </CardBox>

    <div
v-if="copyToast"
         class="fixed bottom-6 right-6 bg-slate-900 text-white px-4 py-2 rounded-lg shadow-lg text-sm z-50">
      {{ copyToast }}
    </div>

    <!-- Add modal -->
    <CardBoxModal
      v-model="isAddModalActive"
      :title="t('os.addOnionTitle')"
      :button-label="t('common.create')"
      button="info"
      :button-disabled="submitting"
      has-cancel
      @confirm="submitCreate"
    >
      <FormField :label="t('common.name')">
        <FormControl v-model="form.name" placeholder="my-app" />
      </FormField>

      <FormField :label="t('os.gatewayRoute')">
        <select v-model="form.route_id" class="w-full px-3 py-2 rounded border bg-white dark:bg-slate-900 dark:border-slate-700">
          <option v-for="r in routes" :key="r.id" :value="r.id">
            {{ r.name || r.id }} {{ r.hosts && r.hosts.length ? '— ' + r.hosts.join(', ') : '' }}
          </option>
        </select>
        <p class="text-xs text-slate-500 mt-1" v-html="t('os.routeAppendHint')"></p>
      </FormField>

      <div v-if="lastError" class="text-sm text-red-600 dark:text-red-400 mt-2">{{ lastError }}</div>
      <p class="text-xs text-slate-500 mt-2">
        {{ t('os.bootstrapHint') }}
      </p>
    </CardBoxModal>

    <!-- Edit modal -->
    <CardBoxModal
      v-model="isEditModalActive"
      :title="t('os.editOnionTitle')"
      :button-label="t('common.save')"
      button="info"
      :button-disabled="submitting"
      has-cancel
      @confirm="submitEdit"
    >
      <FormField :label="t('common.name')">
        <FormControl v-model="form.name" />
      </FormField>

      <FormField :label="t('os.gatewayRoute')">
        <select v-model="form.route_id" class="w-full px-3 py-2 rounded border bg-white dark:bg-slate-900 dark:border-slate-700">
          <option v-for="r in routes" :key="r.id" :value="r.id">
            {{ r.name || r.id }} {{ r.hosts && r.hosts.length ? '— ' + r.hosts.join(', ') : '' }}
          </option>
        </select>
      </FormField>

      <FormField :label="t('common.status')">
        <label class="inline-flex items-center cursor-pointer mt-1">
          <input v-model="form.enabled" type="checkbox" class="sr-only peer" />
          <div class="w-10 h-5 bg-slate-300 dark:bg-slate-600 rounded-full peer peer-checked:bg-emerald-500 transition-colors relative">
            <div class="absolute top-0.5 left-0.5 bg-white w-4 h-4 rounded-full transition-transform peer-checked:translate-x-5"></div>
          </div>
          <span class="ml-3 text-sm">{{ form.enabled ? t('common.enabled') : t('common.disabled') }}</span>
        </label>
      </FormField>

      <div v-if="lastError" class="text-sm text-red-600 dark:text-red-400 mt-2">{{ lastError }}</div>
      <p class="text-xs text-slate-500 mt-2">
        {{ t('os.editPreserveHint') }}
      </p>
    </CardBoxModal>

    <!-- Delete modal -->
    <CardBoxModal
      v-model="isDeleteModalActive"
      :title="t('os.deleteOnionTitle')"
      :button-label="t('common.delete')"
      button="danger"
      has-cancel
      @confirm="confirmDelete"
    >
      <p v-html="t('os.deleteConfirm', { name: selected?.name, addr: selected?.onion_address })"></p>
    </CardBoxModal>
  </div>
</template>
