<script setup>
import BaseButton from "@/components/BaseButton.vue";
import CardBox from "@/components/CardBox.vue";
import FormControl from "@/components/FormControl.vue";
import ApiService from "@/services/ApiService";
import { mdiDelete, mdiDownload, mdiRefresh } from '@mdi/js';
import { onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useToast } from "vue-toastification";

defineProps({ retentionDays: { type: Number, default: 0 } })

const { t } = useI18n();
const toast = useToast();

const recordings = ref([])
const usage = ref(0)
const room = ref('')

const errMsg = (e) => e?.response?.data?.msg || e?.message || t('rtc.actionFailed')
const call = async (fn) => {
  const res = await fn()
  if (res.data?.error) throw new Error(res.data.msg)
  return res.data?.data
}

const load = async () => {
  try {
    const data = await call(() => ApiService.rtcRecordings(room.value.trim()))
    recordings.value = data?.recordings || []
    usage.value = data?.usage || 0
  } catch (e) {
    toast.error(errMsg(e))
  }
}

// The server mints a 5-minute signed link, so the browser downloads the
// file directly instead of buffering it through JavaScript.
const download = async (r) => {
  try {
    const data = await call(() => ApiService.rtcRecordingLink({ room: r.room, name: r.name }))
    window.location.href = data.url
  } catch (e) {
    toast.error(errMsg(e))
  }
}

const remove = async (r) => {
  if (!window.confirm(t('rtc.deleteRecordingConfirm', { name: r.name }))) return
  try {
    await call(() => ApiService.rtcDeleteRecording({ room: r.room, name: r.name }))
    await load()
  } catch (e) {
    toast.error(errMsg(e))
  }
}

const fmtBytes = (n) => {
  if (!n) return '0 B'
  const u = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  while (n >= 1024 && i < u.length - 1) { n /= 1024; i++ }
  return `${n.toFixed(i ? 1 : 0)} ${u[i]}`
}

// "2026-10-24T10:00:00.000-alice.webm" -> participant "alice"
const participantOf = (name) => {
  const m = name.match(/^\d{4}-\d{2}-\d{2}T[\d:.-]+?-(.+)\.(webm|ogg|mp4)$/)
  return m ? m[1] : '—'
}

defineExpose({ load })
onMounted(load)
</script>

<template>
  <CardBox>
    <div class="flex items-center justify-between mb-2 gap-3">
      <h3 class="text-lg font-semibold">{{ t('rtc.recordings') }}</h3>
      <div class="flex items-center gap-2">
        <FormControl v-model="room" :placeholder="t('rtc.filterRoom')" class="w-40" @keyup.enter="load" />
        <BaseButton :icon="mdiRefresh" small color="lightDark" outline @click="load" />
      </div>
    </div>
    <p class="text-sm text-slate-500 mb-1">{{ t('rtc.recordingsHelp') }}</p>
    <p class="text-sm text-slate-500 mb-3">
      {{ t('rtc.recordingsUsage', { size: fmtBytes(usage) }) }} ·
      {{ retentionDays > 0 ? t('rtc.retentionInfo', { days: retentionDays }) : t('rtc.retentionForever') }}
    </p>
    <table class="w-full text-sm">
      <thead>
        <tr class="text-left text-slate-500">
          <th class="py-1">{{ t('rtc.room') }}</th>
          <th class="py-1">{{ t('rtc.participant') }}</th>
          <th class="py-1">{{ t('rtc.date') }}</th>
          <th class="py-1">{{ t('rtc.size') }}</th>
          <th class="py-1"></th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="r in recordings" :key="r.room + '/' + r.name" class="border-t border-slate-200 dark:border-slate-700">
          <td class="py-2 font-mono">{{ r.room }}</td>
          <td class="py-2">
            {{ participantOf(r.name) }}
            <span v-if="r.active" class="ml-1 text-xs px-1.5 py-0.5 rounded bg-red-600 text-white">● {{ t('rtc.recordingNow') }}</span>
            <div class="text-xs text-slate-400 font-mono">{{ r.name }}</div>
          </td>
          <td class="py-2 whitespace-nowrap">{{ new Date(r.modified).toLocaleString() }}</td>
          <td class="py-2 whitespace-nowrap">{{ fmtBytes(r.size) }}</td>
          <td class="py-2">
            <div class="flex justify-end gap-1">
              <BaseButton :icon="mdiDownload" small color="info" :disabled="r.active" :title="t('rtc.download')" @click="download(r)" />
              <BaseButton :icon="mdiDelete" small color="danger" outline :disabled="r.active" :title="t('rtc.delete')" @click="remove(r)" />
            </div>
          </td>
        </tr>
        <tr v-if="!recordings.length"><td colspan="5" class="py-3 text-slate-400">{{ t('rtc.noRecordings') }}</td></tr>
      </tbody>
    </table>
  </CardBox>
</template>
