<script setup>
import { ref, onMounted } from 'vue'
import { mdiArchive, mdiBackupRestore, mdiContentSave, mdiDelete, mdiDownload, mdiRefresh, mdiUpload } from '@mdi/js'
import SectionMain from '@/components/SectionMain.vue'
import SectionTitleLineWithButton from '@/components/SectionTitleLineWithButton.vue'
import CardBox from '@/components/CardBox.vue'
import CardBoxModal from '@/components/CardBoxModal.vue'
import BaseButton from '@/components/BaseButton.vue'
import BaseButtons from '@/components/BaseButtons.vue'
import BaseIcon from '@/components/BaseIcon.vue'
import FormControl from '@/components/FormControl.vue'
import FormField from '@/components/FormField.vue'
import ApiService from '@/services/ApiService'
import { useToast } from 'vue-toastification'

const toast = useToast()

const loading = ref(false)
const creating = ref(false)
const restoring = ref(false)
const uploading = ref(false)
const backups = ref([])

const restoreTarget = ref(null)
const deleteTarget = ref(null)
const showRestoreModal = ref(false)
const showDeleteModal = ref(false)

const fileInput = ref(null)
const config = ref({ max_backups: 10 })
const savingConfig = ref(false)

const formatBytes = (n) => {
  if (!n || n < 0) return '-'
  const units = ['B', 'KB', 'MB', 'GB']
  let v = n
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(v >= 100 || i === 0 ? 0 : 1)} ${units[i]}`
}

const formatDate = (s) => {
  if (!s) return '-'
  const d = new Date(s)
  if (Number.isNaN(d.getTime())) return s
  return d.toLocaleString()
}

const reasonClass = (r) => {
  switch (r) {
    case 'manual':       return 'bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-400'
    case 'pre-update':   return 'bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400'
    case 'pre-restore':  return 'bg-purple-100 dark:bg-purple-900/30 text-purple-700 dark:text-purple-400'
    case 'unreadable':   return 'bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-400'
    default:             return 'bg-slate-200 dark:bg-slate-700 text-slate-600 dark:text-slate-300'
  }
}

const fetchBackups = async () => {
  loading.value = true
  try {
    const [listRes, cfgRes] = await Promise.all([
      ApiService.listBackups(),
      ApiService.getBackupConfig().catch(() => null)
    ])
    if (!listRes.data.error) {
      backups.value = listRes.data.data || []
    } else {
      toast.error(listRes.data.msg || 'Failed to load backups')
    }
    if (cfgRes && !cfgRes.data.error && cfgRes.data.data) {
      config.value = { max_backups: cfgRes.data.data.max_backups || 10 }
    }
  } catch (err) {
    toast.error('Failed to load backups: ' + (err.message || ''))
  } finally {
    loading.value = false
  }
}

const saveConfig = async () => {
  const n = Number(config.value.max_backups)
  if (!Number.isFinite(n) || n < 1) {
    toast.error('Keep last must be at least 1')
    return
  }
  savingConfig.value = true
  try {
    const res = await ApiService.updateBackupConfig({ max_backups: Math.round(n) })
    if (!res.data.error) {
      toast.success('Settings saved')
      await fetchBackups()
    } else {
      toast.error(res.data.msg || 'Failed to save settings')
    }
  } catch (err) {
    toast.error('Failed to save settings: ' + (err.response?.data?.msg || err.message || ''))
  } finally {
    savingConfig.value = false
  }
}

const createBackup = async () => {
  creating.value = true
  try {
    const res = await ApiService.createBackup('manual')
    if (!res.data.error) {
      toast.success('Backup created')
      await fetchBackups()
    } else {
      toast.error(res.data.msg || 'Failed to create backup')
    }
  } catch (err) {
    toast.error('Failed to create backup: ' + (err.message || ''))
  } finally {
    creating.value = false
  }
}

const askRestore = (b) => {
  restoreTarget.value = b
  showRestoreModal.value = true
}

const confirmRestore = async () => {
  const b = restoreTarget.value
  showRestoreModal.value = false
  if (!b) return
  restoring.value = true
  try {
    const res = await ApiService.restoreBackup(b.id)
    if (!res.data.error) {
      toast.info('Restore started — server will restart in a few seconds. You will need to re-login.')
    } else {
      toast.error(res.data.msg || 'Restore failed')
      restoring.value = false
    }
  } catch (err) {
    toast.error('Restore failed: ' + (err.message || ''))
    restoring.value = false
  }
}

const downloadBackup = async (b) => {
  try {
    const res = await ApiService.downloadBackup(b.id)
    const blob = new Blob([res.data], { type: 'application/gzip' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = b.id + '.tar.gz'
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
  } catch (err) {
    toast.error('Download failed: ' + (err.message || ''))
  }
}

const triggerUpload = () => {
  fileInput.value?.click()
}

const onFileSelected = async (event) => {
  const file = event.target.files?.[0]
  event.target.value = '' // reset so re-selecting the same file fires change
  if (!file) return
  uploading.value = true
  try {
    const res = await ApiService.uploadBackup(file)
    if (!res.data.error) {
      toast.success('Backup uploaded')
      await fetchBackups()
    } else {
      toast.error(res.data.msg || 'Upload failed')
    }
  } catch (err) {
    const msg = err.response?.data?.msg || err.message || 'Upload failed'
    toast.error('Upload failed: ' + msg)
  } finally {
    uploading.value = false
  }
}

const askDelete = (b) => {
  deleteTarget.value = b
  showDeleteModal.value = true
}

const confirmDelete = async () => {
  const b = deleteTarget.value
  showDeleteModal.value = false
  if (!b) return
  try {
    const res = await ApiService.deleteBackup(b.id)
    if (!res.data.error) {
      toast.success('Backup deleted')
      await fetchBackups()
    } else {
      toast.error(res.data.msg || 'Failed to delete backup')
    }
  } catch (err) {
    toast.error('Failed to delete backup: ' + (err.message || ''))
  }
}

onMounted(fetchBackups)
</script>

<template>
  <SectionMain>
    <CardBoxModal
      v-model="showRestoreModal"
      title="Restore backup?"
      button="warning"
      button-label="Restore & Restart"
      has-cancel
      @confirm="confirmRestore"
    >
      <p class="mb-2">
        This replaces the current data directory with the contents of
        <strong>{{ restoreTarget?.id }}</strong> and restarts the server.
      </p>
      <p class="text-sm text-slate-500">
        The current state will be discarded. If you want to keep it, take a manual backup first.
        You will need to log in again after the restart.
      </p>
    </CardBoxModal>

    <CardBoxModal
      v-model="showDeleteModal"
      title="Delete backup?"
      button="danger"
      button-label="Delete"
      has-cancel
      @confirm="confirmDelete"
    >
      Permanently delete <strong>{{ deleteTarget?.id }}</strong>?
    </CardBoxModal>

    <input
      ref="fileInput"
      type="file"
      accept=".tar.gz,application/gzip,application/x-gzip,application/x-tar"
      class="hidden"
      @change="onFileSelected"
    />

    <SectionTitleLineWithButton :icon="mdiArchive" title="Backups" main>
      <BaseButtons>
        <BaseButton
          :icon="mdiRefresh"
          color="info"
          small
          :disabled="loading"
          @click="fetchBackups"
          label="Refresh"
        />
        <BaseButton
          :icon="mdiUpload"
          color="info"
          small
          :disabled="uploading"
          @click="triggerUpload"
          :label="uploading ? 'Uploading…' : 'Upload'"
        />
        <BaseButton
          :icon="mdiContentSave"
          color="success"
          small
          :disabled="creating || restoring"
          @click="createBackup"
          :label="creating ? 'Creating…' : 'Backup Now'"
        />
      </BaseButtons>
    </SectionTitleLineWithButton>

    <CardBox class="mb-4">
      <div class="flex flex-wrap items-end gap-4">
        <FormField label="Keep last N backups" class="!mb-0">
          <FormControl
            v-model.number="config.max_backups"
            type="number"
            min="1"
            placeholder="10"
            class="w-32"
          />
        </FormField>
        <BaseButton
          color="info"
          small
          :disabled="savingConfig"
          @click="saveConfig"
          :label="savingConfig ? 'Saving…' : 'Save Settings'"
        />
        <p class="text-xs text-slate-500 flex-1 min-w-[200px]">
          After every Create or Upload, the oldest backups beyond this limit are pruned automatically.
        </p>
      </div>
    </CardBox>

    <CardBox>
      <p class="text-sm text-slate-500 mb-3">
        Snapshots of <code>~/.docker-environment/data/</code> stored at <code>~/redock_backup/</code>.
        A backup is taken automatically before every binary update.
      </p>

      <div v-if="loading && backups.length === 0" class="text-center py-12 text-slate-500">Loading…</div>

      <div v-else-if="backups.length === 0" class="text-center py-12">
        <BaseIcon :path="mdiArchive" size="64" class="mx-auto text-slate-300 dark:text-slate-600 mb-4" />
        <p class="text-slate-500 mb-4">No backups yet</p>
        <BaseButtons class="justify-center">
          <BaseButton :icon="mdiContentSave" color="success" label="Create First Backup" @click="createBackup" :disabled="creating" />
          <BaseButton :icon="mdiUpload" color="info" label="Upload Backup" @click="triggerUpload" :disabled="uploading" />
        </BaseButtons>
      </div>

      <div v-else class="overflow-x-auto">
        <table class="min-w-full text-sm">
          <thead>
            <tr class="text-left text-slate-500 border-b border-slate-200 dark:border-slate-700">
              <th class="py-2 pr-4">ID</th>
              <th class="py-2 pr-4">Created</th>
              <th class="py-2 pr-4">Reason</th>
              <th class="py-2 pr-4">Files</th>
              <th class="py-2 pr-4">Skipped</th>
              <th class="py-2 pr-4">Size (compressed)</th>
              <th class="py-2 pr-4">Size (uncompressed)</th>
              <th class="py-2 pr-4">Version</th>
              <th class="py-2 pr-4 text-right">Actions</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="b in backups"
              :key="b.id"
              class="border-b border-slate-100 dark:border-slate-800 hover:bg-slate-50 dark:hover:bg-slate-800/50"
            >
              <td class="py-2 pr-4 font-mono text-xs">{{ b.id }}</td>
              <td class="py-2 pr-4">{{ formatDate(b.created_at) }}</td>
              <td class="py-2 pr-4">
                <span class="px-2 py-0.5 rounded text-xs" :class="reasonClass(b.trigger_reason)">
                  {{ b.trigger_reason || 'unknown' }}
                </span>
              </td>
              <td class="py-2 pr-4">{{ b.file_count || '-' }}</td>
              <td class="py-2 pr-4">
                <span
                  v-if="b.skipped_count"
                  class="px-2 py-0.5 rounded text-xs bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400"
                  :title="'Files skipped due to permission or read errors. See server logs for paths.'"
                >
                  {{ b.skipped_count }}
                </span>
                <span v-else class="text-slate-400">0</span>
              </td>
              <td class="py-2 pr-4">{{ formatBytes(b.size_bytes) }}</td>
              <td class="py-2 pr-4">{{ formatBytes(b.uncompressed_bytes) }}</td>
              <td class="py-2 pr-4 text-xs text-slate-500">{{ b.redock_version || '-' }}</td>
              <td class="py-2 pr-4">
                <div class="flex justify-end gap-2">
                  <BaseButton
                    :icon="mdiDownload"
                    color="info"
                    small
                    @click="downloadBackup(b)"
                  />
                  <BaseButton
                    :icon="mdiBackupRestore"
                    color="warning"
                    small
                    :disabled="restoring"
                    @click="askRestore(b)"
                  />
                  <BaseButton
                    :icon="mdiDelete"
                    color="danger"
                    small
                    @click="askDelete(b)"
                  />
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </CardBox>
  </SectionMain>
</template>
