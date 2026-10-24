<script setup>
import BaseButton from "@/components/BaseButton.vue";
import BaseIcon from "@/components/BaseIcon.vue";
import CardBox from "@/components/CardBox.vue";
import RTCAPIKeys from "@/components/RTC/RTCAPIKeys.vue";
import RTCAudit from "@/components/RTC/RTCAudit.vue";
import RTCRecordings from "@/components/RTC/RTCRecordings.vue";
import RTCPortsTable from "@/components/RTC/RTCPortsTable.vue";
import RTCRooms from "@/components/RTC/RTCRooms.vue";
import RTCSettings from "@/components/RTC/RTCSettings.vue";
import ApiService from "@/services/ApiService";
import {
  mdiAlertCircleOutline,
  mdiContentCopy,
  mdiOpenInNew,
  mdiPlay,
  mdiRefresh,
  mdiStop,
  mdiVideo
} from '@mdi/js';
import { computed, onMounted, onUnmounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useToast } from "vue-toastification";

const { t } = useI18n();
const toast = useToast();

const status = ref(null)
const busy = ref(false)
// The saved configuration (what the server runs with), reported by RTCSettings.
const config = ref({})

const running = computed(() => status.value?.running === true)
const errMsg = (e) => e?.response?.data?.msg || e?.message || t('rtc.actionFailed')

const loadStatus = async () => {
  try {
    const res = await ApiService.rtcStatus()
    status.value = res.data.data
  } catch (e) {
    toast.error(errMsg(e))
  }
}

const toggleRunning = async () => {
  busy.value = true
  try {
    const res = running.value ? await ApiService.rtcStop() : await ApiService.rtcStart()
    if (res.data?.error) throw new Error(res.data.msg)
    status.value = res.data.data
  } catch (e) {
    toast.error(errMsg(e))
  } finally {
    busy.value = false
  }
}

const portsTable = ref(null)
const onSaved = (st) => {
  if (st) status.value = st
  portsTable.value?.load() // ports follow the saved settings
}

// The test room URL: through the gateway when a public host is set,
// otherwise the local listener (camera access needs HTTPS except on localhost).
const testRoomURL = computed(() => {
  const c = config.value
  if (!c.test_room_name) return ''
  if (c.public_host) return `https://${c.public_host}/group/${c.test_room_name}/`
  return `http://${c.listen_addr}/group/${c.test_room_name}/`
})


const copy = async (text) => {
  try {
    await navigator.clipboard.writeText(text)
    toast.success(t('rtc.copied'))
  } catch {
    toast.error(t('rtc.copyFailed'))
  }
}

const formatDate = (v) => (v ? new Date(v).toLocaleString() : '—')

let timer = null
onMounted(async () => {
  await loadStatus()
  timer = setInterval(loadStatus, 10000)
})
onUnmounted(() => clearInterval(timer))
</script>

<template>
  <div class="space-y-8">
    <!-- Header -->
    <div class="bg-gradient-to-r from-emerald-700 via-teal-700 to-cyan-700 rounded-2xl p-8 text-white shadow-lg">
      <div class="flex flex-col lg:flex-row lg:items-center lg:justify-between">
        <div>
          <h1 class="text-3xl lg:text-4xl font-bold mb-2 flex items-center">
            <BaseIcon :path="mdiVideo" size="40" class="mr-4" />
            {{ t('rtc.title') }}
          </h1>
          <p class="text-teal-100 text-lg">{{ t('rtc.subtitle') }}</p>
        </div>
        <div class="mt-6 lg:mt-0 flex items-center gap-3">
          <span
            :class="['px-3 py-1 rounded-full text-sm font-semibold', running ? 'bg-green-500/30 text-white' : 'bg-white/20 text-teal-50']"
          >● {{ running ? t('rtc.running') : t('rtc.stopped') }}</span>
          <BaseButton :icon="mdiRefresh" color="white" outline :disabled="busy" @click="loadStatus" />
          <BaseButton
            :icon="running ? mdiStop : mdiPlay"
            :label="running ? t('rtc.stop') : t('rtc.start')"
            color="white"
            :disabled="busy"
            @click="toggleRunning"
          />
        </div>
      </div>
    </div>

    <CardBox v-if="status?.last_error" class="border-l-4 border-red-500">
      <div class="flex items-start gap-3">
        <BaseIcon :path="mdiAlertCircleOutline" size="24" class="text-red-500 flex-shrink-0" />
        <div>
          <p class="font-semibold">{{ t('rtc.lastError') }}</p>
          <p class="text-sm font-mono text-slate-600 dark:text-slate-300">{{ status.last_error }}</p>
        </div>
      </div>
    </CardBox>

    <!-- At a glance -->
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
      <CardBox>
        <h3 class="text-lg font-semibold mb-4">{{ t('rtc.status') }}</h3>
        <dl class="grid grid-cols-2 gap-y-2 text-sm">
          <dt class="text-slate-500">{{ t('rtc.startedAt') }}</dt>
          <dd>{{ formatDate(status?.started_at) }}</dd>
          <dt class="text-slate-500">{{ t('rtc.activeRooms') }}</dt>
          <dd>{{ status?.rooms?.length || 0 }}</dd>
          <dt class="text-slate-500">{{ t('rtc.version') }}</dt>
          <dd class="font-mono text-xs break-all">{{ status?.version }}</dd>
        </dl>
      </CardBox>

      <CardBox>
        <h3 class="text-lg font-semibold mb-2">{{ t('rtc.testRoom') }}</h3>
        <p class="text-sm text-slate-500 mb-4">{{ t('rtc.testRoomHelp') }}</p>
        <div class="space-y-2 text-sm">
          <div class="flex items-center gap-2 min-w-0">
            <span class="font-mono text-xs break-all flex-1 min-w-0">{{ testRoomURL }}</span>
            <BaseButton :icon="mdiContentCopy" small color="lightDark" outline :title="t('rtc.link')" @click="copy(testRoomURL)" />
            <BaseButton :icon="mdiOpenInNew" small color="info" :href="testRoomURL" target="_blank" :disabled="!running" />
          </div>
          <div class="flex items-center gap-2">
            <span class="text-slate-500">{{ t('rtc.password') }}</span>
            <span class="font-mono flex-1">{{ config.test_room_password }}</span>
            <BaseButton :icon="mdiContentCopy" small color="lightDark" outline @click="copy(config.test_room_password)" />
          </div>
        </div>
      </CardBox>

    </div>

    <RTCSettings :running="running" @loaded="config = $event" @saved="onSaved" />
    <RTCPortsTable ref="portsTable" />
    <RTCRooms :running="running" />
    <RTCRecordings :retention-days="config.retention_days || 0" />
    <RTCAPIKeys />
    <RTCAudit />
  </div>
</template>
