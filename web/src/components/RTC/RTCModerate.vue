<script setup>
// Live moderation of one room from the dashboard: who is in it, remove
// people, lock, clear chat, end the session, or join visibly as moderator.
// Every action here is written to the audit log by the backend.
import BaseButton from "@/components/BaseButton.vue";
import CardBoxModal from "@/components/CardBoxModal.vue";
import FormControl from "@/components/FormControl.vue";
import ApiService from "@/services/ApiService";
import { mdiAccountRemove, mdiAccountTie, mdiChatRemove, mdiLock, mdiLockOpen, mdiPhoneHangup, mdiRecord, mdiRefresh, mdiStop } from '@mdi/js';
import { computed, onUnmounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { useToast } from "vue-toastification";

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  room: { type: Object, default: null },
})
const emit = defineEmits(['update:modelValue', 'changed'])

const { t } = useI18n();
const toast = useToast();

const open = computed({ get: () => props.modelValue, set: (v) => emit('update:modelValue', v) })
const participants = ref([])
const lockMessage = ref('')
const busy = ref(false)

const errMsg = (e) => e?.response?.data?.msg || e?.message || t('rtc.actionFailed')
const call = async (fn) => {
  const res = await fn()
  if (res.data?.error) throw new Error(res.data.msg)
  return res.data?.data
}

const load = async () => {
  if (!props.room) return
  try {
    participants.value = (await call(() => ApiService.rtcParticipants(props.room.name))) || []
  } catch (e) {
    toast.error(errMsg(e))
  }
}

const act = async (fn, okText) => {
  busy.value = true
  try {
    await call(fn)
    if (okText) toast.success(okText)
    emit('changed')
    await load()
  } catch (e) {
    toast.error(errMsg(e))
  } finally {
    busy.value = false
  }
}

const kick = (p) => {
  const reason = window.prompt(t('rtc.kickReason', { name: p.username }), '')
  if (reason === null) return
  act(() => ApiService.rtcKick({ room: props.room.name, id: p.id, reason }), t('rtc.kicked'))
}
const toggleLock = () => act(
  () => ApiService.rtcLock({ room: props.room.name, locked: !props.room.locked, message: lockMessage.value }),
  props.room.locked ? t('rtc.unlocked') : t('rtc.lockedOk'),
)
const toggleRecording = () => {
  const start = !props.room.recording
  if (start && !window.confirm(t('rtc.startRecordingConfirm'))) return
  act(() => ApiService.rtcRecord({ room: props.room.name, start }), start ? t('rtc.recordingStarted') : t('rtc.recordingStopped'))
}
const clearChat = () => {
  if (!window.confirm(t('rtc.clearChatConfirm'))) return
  act(() => ApiService.rtcClearChat({ room: props.room.name }), t('rtc.chatCleared'))
}
const endSession = () => {
  if (!window.confirm(t('rtc.endSessionConfirm', { name: props.room.name }))) return
  act(() => ApiService.rtcEndSession({ room: props.room.name, reason: '' }), t('rtc.sessionEnded'))
}
const joinAsModerator = async () => {
  try {
    const inv = await call(() => ApiService.rtcModeratorJoin({ room: props.room.name }))
    window.open(inv.join_url, '_blank', 'noopener')
  } catch (e) {
    toast.error(errMsg(e))
  }
}

const roleLabel = (r) => ({
  moderator: t('rtc.roleModeratorShort'), presenter: t('rtc.rolePresenterShort'),
  viewer: t('rtc.roleViewerShort'), observer: t('rtc.roleObserverShort'),
}[r] || r)

let timer = null
watch(open, (v) => {
  clearInterval(timer)
  if (v) {
    load()
    timer = setInterval(load, 5000)
  }
})
onUnmounted(() => clearInterval(timer))
</script>

<template>
  <CardBoxModal v-model="open" :title="t('rtc.moderateTitle', { name: room?.display_name || room?.name })" :button-label="t('rtc.done')">
    <div class="flex flex-wrap gap-2 mb-4">
      <BaseButton :icon="mdiAccountTie" :label="t('rtc.joinAsModerator')" color="info" small @click="joinAsModerator" />
      <BaseButton :icon="room?.locked ? mdiLockOpen : mdiLock" :label="room?.locked ? t('rtc.unlock') : t('rtc.lock')" color="warning" small :disabled="busy" @click="toggleLock" />
      <BaseButton
        v-if="room?.allow_recording"
        :icon="room?.recording ? mdiStop : mdiRecord"
        :label="room?.recording ? t('rtc.stopRecording') : t('rtc.startRecording')"
        color="danger"
        small
        :outline="!room?.recording"
        :disabled="busy || (!room?.recording && !participants.length)"
        @click="toggleRecording"
      />
      <BaseButton :icon="mdiChatRemove" :label="t('rtc.clearChat')" color="lightDark" small outline :disabled="busy" @click="clearChat" />
      <BaseButton :icon="mdiPhoneHangup" :label="t('rtc.endSession')" color="danger" small :disabled="busy || !participants.length" @click="endSession" />
      <BaseButton :icon="mdiRefresh" small color="lightDark" outline @click="load" />
    </div>
    <FormControl v-if="!room?.locked" v-model="lockMessage" :placeholder="t('rtc.lockMessage')" class="mb-4" />
    <p class="text-xs text-slate-500 mb-3">{{ t('rtc.joinAsModeratorHelp') }}</p>

    <h4 class="font-semibold mb-2">{{ t('rtc.liveParticipants') }} ({{ participants.length }})</h4>
    <p v-if="!participants.length" class="text-sm text-slate-400">{{ t('rtc.nobodyInRoom') }}</p>
    <table v-else class="w-full text-sm">
      <thead>
        <tr class="text-left text-slate-500">
          <th class="py-1">{{ t('rtc.name') }}</th>
          <th class="py-1">{{ t('rtc.role') }}</th>
          <th class="py-1">IP</th>
          <th class="py-1"></th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="p in participants" :key="p.id" class="border-t border-slate-200 dark:border-slate-700">
          <td class="py-2">{{ p.system ? t('rtc.recorder') : p.username }}</td>
          <td class="py-2">{{ p.system ? '—' : roleLabel(p.role) }}</td>
          <td class="py-2 font-mono text-xs">{{ p.ip || '—' }}</td>
          <td class="py-2 text-right">
            <BaseButton v-if="!p.system" :icon="mdiAccountRemove" small color="danger" outline :title="t('rtc.kick')" :disabled="busy" @click="kick(p)" />
          </td>
        </tr>
      </tbody>
    </table>
  </CardBoxModal>
</template>
