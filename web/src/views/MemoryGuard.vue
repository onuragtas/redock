<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { mdiBroom, mdiContentSave, mdiMemory, mdiRefresh } from '@mdi/js'
import SectionMain from '@/components/SectionMain.vue'
import SectionTitleLineWithButton from '@/components/SectionTitleLineWithButton.vue'
import CardBox from '@/components/CardBox.vue'
import BaseButton from '@/components/BaseButton.vue'
import BaseButtons from '@/components/BaseButtons.vue'
import ApiService from '@/services/ApiService'
import { useToast } from 'vue-toastification'
import { useI18n } from 'vue-i18n'

const toast = useToast()
const { t } = useI18n()

const loading = ref(false)
const saving = ref(false)
const releasing = ref(false)

const status = ref(null)
const tables = ref([])
const history = ref([])
const events = ref([])
const form = ref(null)

// Manual limit is edited in MB; 0 keeps the automatic budget.
const limitMb = ref(0)

let timer = null

const formatBytes = (n) => {
  if (n === null || n === undefined) return '-'
  if (n <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = n
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

const formatTime = (unix) => {
  if (!unix) return '-'
  return new Date(unix * 1000).toLocaleTimeString()
}

const levelMeta = computed(() => {
  switch (status.value?.level) {
    case 'emergency':
      return { label: t('mem.levelEmergency'), badge: 'bg-red-500/20 text-red-400 border-red-500/40', bar: 'bg-red-500' }
    case 'critical':
      return { label: t('mem.levelCritical'), badge: 'bg-orange-500/20 text-orange-400 border-orange-500/40', bar: 'bg-orange-500' }
    case 'warning':
      return { label: t('mem.levelWarning'), badge: 'bg-amber-500/20 text-amber-400 border-amber-500/40', bar: 'bg-amber-500' }
    default:
      return { label: t('mem.levelNormal'), badge: 'bg-emerald-500/20 text-emerald-400 border-emerald-500/40', bar: 'bg-emerald-500' }
  }
})

const usedPercent = computed(() => Math.min(100, Math.max(0, status.value?.used_percent || 0)))

// Threshold markers drawn on the usage bar.
const markers = computed(() => {
  const cfg = status.value?.config
  if (!cfg) return []
  return [
    { pct: cfg.warning_percent, color: 'bg-amber-400' },
    { pct: cfg.critical_percent, color: 'bg-orange-400' },
    { pct: cfg.emergency_percent, color: 'bg-red-400' }
  ]
})

// History as an SVG polyline; no chart library, one dependency less.
const chart = computed(() => {
  const points = history.value
  if (!points.length) return { line: '', area: '', max: 0 }

  const width = 600
  const height = 120
  const max = Math.max(...points.map((p) => p.used_bytes), status.value?.limit_bytes || 0) || 1
  const step = points.length > 1 ? width / (points.length - 1) : width

  const coords = points.map((p, i) => {
    const x = (i * step).toFixed(1)
    const y = (height - (p.used_bytes / max) * height).toFixed(1)
    return `${x},${y}`
  })

  return {
    line: coords.join(' '),
    area: `0,${height} ${coords.join(' ')} ${width},${height}`,
    max
  }
})

const limitLineY = computed(() => {
  const limit = status.value?.limit_bytes || 0
  if (!limit || !chart.value.max) return null
  return (120 - (limit / chart.value.max) * 120).toFixed(1)
})

const fetchAll = async () => {
  try {
    const [statusRes, historyRes, eventsRes] = await Promise.all([
      ApiService.getMemoryStatus(),
      ApiService.getMemoryHistory(),
      ApiService.getMemoryEvents()
    ])

    status.value = statusRes.data?.data?.status || null
    tables.value = statusRes.data?.data?.tables || []
    history.value = historyRes.data?.data || []
    events.value = (eventsRes.data?.data || []).slice(-40).reverse()

    if (!form.value && status.value?.config) {
      form.value = { ...status.value.config }
      limitMb.value = Math.round((status.value.config.limit_bytes || 0) / (1024 * 1024))
    }
  } catch (err) {
    toast.error(t('mem.loadFailed') + (err.message || ''))
  }
}

const refresh = async () => {
  loading.value = true
  await fetchAll()
  loading.value = false
}

const saveConfig = async () => {
  if (!form.value) return
  saving.value = true
  try {
    const payload = { ...form.value, limit_bytes: Math.max(0, Number(limitMb.value) || 0) * 1024 * 1024 }
    const res = await ApiService.updateMemoryConfig(payload)
    if (!res.data.error) {
      form.value = { ...res.data.data }
      limitMb.value = Math.round((res.data.data.limit_bytes || 0) / (1024 * 1024))
      toast.success(t('mem.configSaved'))
      await fetchAll()
    } else {
      toast.error(res.data.msg || t('mem.configFailed'))
    }
  } catch (err) {
    toast.error(t('mem.configFailed') + ' ' + (err.message || ''))
  } finally {
    saving.value = false
  }
}

const release = async (level) => {
  releasing.value = true
  try {
    const res = await ApiService.releaseMemory(level)
    if (!res.data.error) {
      const freed = (res.data.data?.results || []).reduce((sum, r) => sum + (r.freed_bytes || 0), 0)
      toast.success(t('mem.released', { size: formatBytes(freed) }))
      await fetchAll()
    } else {
      toast.error(res.data.msg || t('mem.releaseFailed'))
    }
  } catch (err) {
    toast.error(t('mem.releaseFailed') + ' ' + (err.message || ''))
  } finally {
    releasing.value = false
  }
}

onMounted(async () => {
  await refresh()
  timer = setInterval(fetchAll, 5000)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
})
</script>

<template>
  <SectionMain>
    <SectionTitleLineWithButton :icon="mdiMemory" :title="t('mem.title')" main>
      <BaseButtons>
        <BaseButton
          :icon="mdiBroom"
          :label="t('mem.releaseNow')"
          color="warning"
          small
          :disabled="releasing"
          @click="release('critical')"
        />
        <BaseButton :icon="mdiRefresh" :label="t('mem.refresh')" color="info" small :disabled="loading" @click="refresh" />
      </BaseButtons>
    </SectionTitleLineWithButton>

    <!-- Usage summary -->
    <CardBox class="mb-6">
      <div class="flex flex-wrap items-center justify-between gap-3 mb-4">
        <div class="flex items-center gap-3">
          <span class="px-3 py-1 text-xs font-semibold rounded-full border" :class="levelMeta.badge">
            {{ levelMeta.label }}
          </span>
          <span class="text-sm text-slate-500">
            {{ t('mem.budgetSource', { source: status?.limit_source || '-' }) }}
          </span>
        </div>
        <div class="text-right">
          <div class="text-2xl font-semibold">
            {{ formatBytes(status?.used_bytes) }}
            <span class="text-slate-500 text-base">/ {{ formatBytes(status?.limit_bytes) }}</span>
          </div>
          <div class="text-xs text-slate-500">{{ usedPercent.toFixed(1) }}%</div>
        </div>
      </div>

      <div class="relative h-3 rounded-full bg-slate-200 dark:bg-slate-800 overflow-hidden mb-1">
        <div class="h-full transition-all duration-500" :class="levelMeta.bar" :style="{ width: usedPercent + '%' }"></div>
        <div
          v-for="(m, i) in markers"
          :key="i"
          class="absolute top-0 h-full w-px opacity-70"
          :class="m.color"
          :style="{ left: m.pct + '%' }"
        ></div>
      </div>
      <div class="flex justify-between text-xs text-slate-500 mb-5">
        <span>0</span>
        <span>{{ t('mem.thresholdLegend') }}</span>
      </div>

      <div class="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
        <div>
          <div class="text-slate-500 text-xs">{{ t('mem.rss') }}</div>
          <div class="font-medium">{{ formatBytes(status?.rss_bytes) }}</div>
        </div>
        <div>
          <div class="text-slate-500 text-xs">{{ t('mem.heap') }}</div>
          <div class="font-medium">{{ formatBytes(status?.heap_bytes) }}</div>
        </div>
        <div>
          <div class="text-slate-500 text-xs">{{ t('mem.goSys') }}</div>
          <div class="font-medium">{{ formatBytes(status?.go_sys_bytes) }}</div>
        </div>
        <div>
          <div class="text-slate-500 text-xs">{{ t('mem.goroutines') }}</div>
          <div class="font-medium">{{ status?.goroutines ?? '-' }}</div>
        </div>
        <div>
          <div class="text-slate-500 text-xs">{{ t('mem.goMemLimit') }}</div>
          <div class="font-medium">{{ formatBytes(status?.go_mem_limit) }}</div>
        </div>
        <div>
          <div class="text-slate-500 text-xs">{{ t('mem.gcPercent') }}</div>
          <div class="font-medium">{{ status?.gc_percent ?? '-' }}</div>
        </div>
        <div>
          <div class="text-slate-500 text-xs">{{ t('mem.gcRuns') }}</div>
          <div class="font-medium">{{ status?.num_gc ?? '-' }}</div>
        </div>
        <div>
          <div class="text-slate-500 text-xs">{{ t('mem.totalFreed') }}</div>
          <div class="font-medium">{{ formatBytes(status?.total_freed_bytes) }}</div>
        </div>
      </div>

      <p v-if="status?.oom_protection" class="mt-4 text-xs text-slate-500">
        {{ t('mem.oomProtection') }}: {{ status.oom_protection }}
      </p>
    </CardBox>

    <!-- Usage over time -->
    <CardBox class="mb-6">
      <h2 class="text-lg font-semibold mb-3">{{ t('mem.chartTitle') }}</h2>
      <svg v-if="history.length" viewBox="0 0 600 120" class="w-full h-32" preserveAspectRatio="none">
        <polygon :points="chart.area" class="fill-blue-500/20" />
        <polyline :points="chart.line" class="stroke-blue-500 fill-none" stroke-width="2" />
        <line
          v-if="limitLineY !== null"
          x1="0"
          x2="600"
          :y1="limitLineY"
          :y2="limitLineY"
          class="stroke-red-500"
          stroke-width="1"
          stroke-dasharray="4 4"
        />
      </svg>
      <p v-else class="text-sm text-slate-500">{{ t('mem.noSamples') }}</p>
    </CardBox>

    <!-- Biggest in-memory tables -->
    <CardBox class="mb-6">
      <h2 class="text-lg font-semibold mb-1">{{ t('mem.tablesTitle') }}</h2>
      <p class="text-sm text-slate-500 mb-3">{{ t('mem.tablesHint') }}</p>
      <div class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead class="text-slate-500 text-left">
            <tr>
              <th class="py-2">{{ t('mem.table') }}</th>
              <th class="py-2">{{ t('mem.rows') }}</th>
              <th class="py-2">{{ t('mem.rowLimit') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="tbl in tables.slice(0, 12)" :key="tbl.name" class="border-t border-slate-200 dark:border-slate-800">
              <td class="py-2 font-mono text-xs">{{ tbl.name }}</td>
              <td class="py-2">{{ tbl.rows.toLocaleString() }}</td>
              <td class="py-2 text-slate-500">{{ tbl.max_rows > 0 ? tbl.max_rows.toLocaleString() : t('mem.unlimited') }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </CardBox>

    <!-- Configuration -->
    <CardBox v-if="form" class="mb-6">
      <h2 class="text-lg font-semibold mb-4">{{ t('mem.configTitle') }}</h2>

      <div class="grid grid-cols-1 md:grid-cols-3 gap-4 mb-4">
        <label class="block">
          <span class="text-xs text-slate-500">{{ t('mem.limitMb') }}</span>
          <input
            v-model.number="limitMb"
            type="number"
            min="0"
            class="w-full mt-1 px-3 py-2 rounded-lg bg-white dark:bg-slate-800 border border-slate-300 dark:border-slate-700"
          />
          <span class="text-xs text-slate-500">{{ t('mem.limitMbHint') }}</span>
        </label>
        <label class="block">
          <span class="text-xs text-slate-500">{{ t('mem.autoPercent') }}</span>
          <input
            v-model.number="form.auto_limit_percent"
            type="number"
            min="10"
            max="95"
            class="w-full mt-1 px-3 py-2 rounded-lg bg-white dark:bg-slate-800 border border-slate-300 dark:border-slate-700"
          />
          <span class="text-xs text-slate-500">
            {{ t('mem.systemTotal', { size: formatBytes(status?.system_bytes) }) }}
          </span>
        </label>
        <label class="block">
          <span class="text-xs text-slate-500">{{ t('mem.interval') }}</span>
          <input
            v-model.number="form.interval_seconds"
            type="number"
            min="1"
            max="300"
            class="w-full mt-1 px-3 py-2 rounded-lg bg-white dark:bg-slate-800 border border-slate-300 dark:border-slate-700"
          />
        </label>
      </div>

      <div class="grid grid-cols-1 md:grid-cols-3 gap-4 mb-4">
        <label class="block">
          <span class="text-xs text-slate-500">{{ t('mem.warningPercent') }}</span>
          <input
            v-model.number="form.warning_percent"
            type="number"
            min="1"
            max="99"
            class="w-full mt-1 px-3 py-2 rounded-lg bg-white dark:bg-slate-800 border border-slate-300 dark:border-slate-700"
          />
        </label>
        <label class="block">
          <span class="text-xs text-slate-500">{{ t('mem.criticalPercent') }}</span>
          <input
            v-model.number="form.critical_percent"
            type="number"
            min="1"
            max="99"
            class="w-full mt-1 px-3 py-2 rounded-lg bg-white dark:bg-slate-800 border border-slate-300 dark:border-slate-700"
          />
        </label>
        <label class="block">
          <span class="text-xs text-slate-500">{{ t('mem.emergencyPercent') }}</span>
          <input
            v-model.number="form.emergency_percent"
            type="number"
            min="1"
            max="99"
            class="w-full mt-1 px-3 py-2 rounded-lg bg-white dark:bg-slate-800 border border-slate-300 dark:border-slate-700"
          />
        </label>
      </div>

      <div class="grid grid-cols-1 md:grid-cols-2 gap-3 mb-5">
        <label class="flex items-start gap-2 text-sm">
          <input v-model="form.enabled" type="checkbox" class="mt-1" />
          <span>
            <span class="font-medium">{{ t('mem.optEnabled') }}</span>
            <span class="block text-xs text-slate-500">{{ t('mem.optEnabledHint') }}</span>
          </span>
        </label>
        <label class="flex items-start gap-2 text-sm">
          <input v-model="form.shed_load" type="checkbox" class="mt-1" />
          <span>
            <span class="font-medium">{{ t('mem.optShedLoad') }}</span>
            <span class="block text-xs text-slate-500">{{ t('mem.optShedLoadHint') }}</span>
          </span>
        </label>
        <label class="flex items-start gap-2 text-sm">
          <input v-model="form.apply_go_mem_limit" type="checkbox" class="mt-1" />
          <span>
            <span class="font-medium">{{ t('mem.optGoMemLimit') }}</span>
            <span class="block text-xs text-slate-500">{{ t('mem.optGoMemLimitHint') }}</span>
          </span>
        </label>
        <label class="flex items-start gap-2 text-sm">
          <input v-model="form.adaptive_gc" type="checkbox" class="mt-1" />
          <span>
            <span class="font-medium">{{ t('mem.optAdaptiveGc') }}</span>
            <span class="block text-xs text-slate-500">{{ t('mem.optAdaptiveGcHint') }}</span>
          </span>
        </label>
        <label class="flex items-start gap-2 text-sm">
          <input v-model="form.return_memory_to_os" type="checkbox" class="mt-1" />
          <span>
            <span class="font-medium">{{ t('mem.optFreeOs') }}</span>
            <span class="block text-xs text-slate-500">{{ t('mem.optFreeOsHint') }}</span>
          </span>
        </label>
        <label class="flex items-start gap-2 text-sm">
          <input v-model="form.protect_from_oom_killer" type="checkbox" class="mt-1" />
          <span>
            <span class="font-medium">{{ t('mem.optOomProtect') }}</span>
            <span class="block text-xs text-slate-500">{{ t('mem.optOomProtectHint') }}</span>
          </span>
        </label>
      </div>

      <BaseButtons>
        <BaseButton :icon="mdiContentSave" :label="t('mem.save')" color="success" :disabled="saving" @click="saveConfig" />
        <BaseButton :label="t('mem.releaseEmergency')" color="danger" outline :disabled="releasing" @click="release('emergency')" />
      </BaseButtons>
    </CardBox>

    <!-- Relievers -->
    <CardBox class="mb-6">
      <h2 class="text-lg font-semibold mb-1">{{ t('mem.relieversTitle') }}</h2>
      <p class="text-sm text-slate-500 mb-3">{{ t('mem.relieversHint') }}</p>
      <div class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead class="text-slate-500 text-left">
            <tr>
              <th class="py-2">{{ t('mem.reliever') }}</th>
              <th class="py-2">{{ t('mem.triggersAt') }}</th>
              <th class="py-2">{{ t('mem.runs') }}</th>
              <th class="py-2">{{ t('mem.freed') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="r in status?.relievers || []" :key="r.name" class="border-t border-slate-200 dark:border-slate-800">
              <td class="py-2">
                <div class="font-mono text-xs">{{ r.name }}</div>
                <div class="text-xs text-slate-500">{{ r.description }}</div>
              </td>
              <td class="py-2">{{ r.min_level }}</td>
              <td class="py-2">{{ r.runs }}</td>
              <td class="py-2">{{ formatBytes(r.freed_bytes) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </CardBox>

    <!-- Activity -->
    <CardBox>
      <h2 class="text-lg font-semibold mb-3">{{ t('mem.eventsTitle') }}</h2>
      <div v-if="events.length" class="space-y-1 max-h-80 overflow-y-auto text-sm">
        <div v-for="(ev, i) in events" :key="i" class="flex gap-3 py-1 border-b border-slate-100 dark:border-slate-800">
          <span class="text-xs text-slate-500 w-20 shrink-0">{{ formatTime(ev.timestamp) }}</span>
          <span class="text-xs font-mono w-40 shrink-0">{{ ev.action }}</span>
          <span class="text-xs text-slate-500 flex-1">{{ ev.detail }}</span>
        </div>
      </div>
      <p v-else class="text-sm text-slate-500">{{ t('mem.noEvents') }}</p>
    </CardBox>
  </SectionMain>
</template>
