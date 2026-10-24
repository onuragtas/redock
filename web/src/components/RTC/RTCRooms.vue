<script setup>
import BaseButton from "@/components/BaseButton.vue";
import CardBox from "@/components/CardBox.vue";
import CardBoxModal from "@/components/CardBoxModal.vue";
import FormCheckRadio from "@/components/FormCheckRadio.vue";
import FormControl from "@/components/FormControl.vue";
import FormField from "@/components/FormField.vue";
import ApiService from "@/services/ApiService";
import RTCModerate from "@/components/RTC/RTCModerate.vue";
import { mdiAccountPlus, mdiContentCopy, mdiDelete, mdiOpenInNew, mdiPencil, mdiPlus, mdiShieldAccount } from '@mdi/js';
import { onMounted, onUnmounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useToast } from "vue-toastification";

defineProps({ running: { type: Boolean, default: false } })

const { t } = useI18n();
const toast = useToast();

const rooms = ref([])
const roomModal = ref(false)
const editing = ref(false)
const form = ref({})
const inviteModal = ref(false)
const inviteForm = ref({})
const invite = ref(null)

const errMsg = (e) => e?.response?.data?.msg || e?.message || t('rtc.actionFailed')
const call = async (fn) => {
  const res = await fn.call(ApiService) // keep `this` for ApiService.get/post
  if (res.data?.error) throw new Error(res.data.msg)
  return res.data?.data
}

const load = async () => {
  try {
    rooms.value = (await call(ApiService.rtcListRooms)) || []
  } catch (e) {
    toast.error(errMsg(e))
  }
}

const openNew = () => {
  editing.value = false
  form.value = { name: '', display_name: '', description: '', max_clients: 0, allow_recording: false }
  roomModal.value = true
}
const openEdit = (r) => {
  editing.value = true
  form.value = { name: r.name, display_name: r.display_name, description: r.description, max_clients: r.max_clients, allow_recording: r.allow_recording }
  roomModal.value = true
}
const saveRoom = async () => {
  try {
    await call(() => ApiService.rtcSaveRoom({ ...form.value, max_clients: Number(form.value.max_clients) || 0 }))
    toast.success(t('rtc.roomSaved'))
    roomModal.value = false
    await load()
  } catch (e) {
    toast.error(errMsg(e))
  }
}
const deleteRoom = async (r) => {
  if (!window.confirm(t('rtc.deleteRoomConfirm', { name: r.name }))) return
  try {
    await call(() => ApiService.rtcDeleteRoom(r.name))
    await load()
  } catch (e) {
    toast.error(errMsg(e))
  }
}

const moderateModal = ref(false)
const moderating = ref(null)
const openModerate = (r) => {
  moderating.value = r
  moderateModal.value = true
}
const onModerated = async () => {
  await load()
  moderating.value = rooms.value.find((r) => r.name === moderating.value?.name) || moderating.value
}

const openInvite = (r) => {
  invite.value = null
  inviteForm.value = { room: r.name, role: 'presenter', username: '', ttl_hours: 24 }
  inviteModal.value = true
}
const createInvite = async () => {
  try {
    invite.value = await call(() => ApiService.rtcInvite({ ...inviteForm.value, ttl_hours: Number(inviteForm.value.ttl_hours) || 24 }))
  } catch (e) {
    toast.error(errMsg(e))
  }
}

const copy = async (text) => {
  try {
    await navigator.clipboard.writeText(text)
    toast.success(t('rtc.copied'))
  } catch {
    toast.error(t('rtc.copyFailed'))
  }
}

let timer = null
onMounted(() => {
  load()
  timer = setInterval(load, 10000)
})
onUnmounted(() => clearInterval(timer))
</script>

<template>
  <CardBox>
    <div class="flex items-center justify-between mb-2">
      <h3 class="text-lg font-semibold">{{ t('rtc.rooms') }}</h3>
      <BaseButton :icon="mdiPlus" :label="t('rtc.newRoom')" color="info" small @click="openNew" />
    </div>
    <p class="text-sm text-slate-500 mb-4">{{ t('rtc.roomsHelp') }}</p>
    <table class="w-full text-sm">
      <thead>
        <tr class="text-left text-slate-500">
          <th class="py-1">{{ t('rtc.room') }}</th>
          <th class="py-1">{{ t('rtc.participants') }}</th>
          <th class="py-1">{{ t('rtc.maxClients') }}</th>
          <th class="py-1">{{ t('rtc.recording') }}</th>
          <th class="py-1"></th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="r in rooms" :key="r.name" class="border-t border-slate-200 dark:border-slate-700">
          <td class="py-2">
            <div class="font-medium">{{ r.display_name || r.name }}</div>
            <div class="text-xs font-mono text-slate-500">
              {{ r.name }}<template v-if="r.locked"> · 🔒</template>
              <span v-if="r.recording" class="ml-1 px-1.5 rounded bg-red-600 text-white">● REC</span>
            </div>
          </td>
          <td class="py-2">{{ r.participants }}</td>
          <td class="py-2">{{ r.max_clients || '∞' }}</td>
          <td class="py-2">{{ r.allow_recording ? t('rtc.allowed') : '—' }}</td>
          <td class="py-2">
            <div class="flex justify-end gap-1">
              <BaseButton :icon="mdiShieldAccount" small color="warning" :title="t('rtc.moderate')" :disabled="!running" @click="openModerate(r)" />
              <BaseButton :icon="mdiAccountPlus" small color="info" :title="t('rtc.invite')" :disabled="!running" @click="openInvite(r)" />
              <BaseButton :icon="mdiPencil" small color="lightDark" outline :title="t('rtc.edit')" @click="openEdit(r)" />
              <BaseButton :icon="mdiDelete" small color="danger" outline :title="t('rtc.delete')" @click="deleteRoom(r)" />
            </div>
          </td>
        </tr>
        <tr v-if="!rooms.length"><td colspan="5" class="py-3 text-slate-400">{{ t('rtc.noConfiguredRooms') }}</td></tr>
      </tbody>
    </table>
  </CardBox>

  <CardBoxModal v-model="roomModal" :title="editing ? t('rtc.editRoom') : t('rtc.newRoom')" :button-label="t('rtc.save')" has-cancel @confirm="saveRoom">
    <FormField :label="t('rtc.roomName')" :help="t('rtc.roomNameHelp')">
      <FormControl v-model="form.name" :disabled="editing" placeholder="team-weekly" />
    </FormField>
    <FormField :label="t('rtc.displayName')">
      <FormControl v-model="form.display_name" placeholder="Haftalık toplantı" />
    </FormField>
    <FormField :label="t('rtc.description')">
      <FormControl v-model="form.description" />
    </FormField>
    <FormField :label="t('rtc.maxClients')" :help="t('rtc.maxClientsHelp')">
      <FormControl v-model.number="form.max_clients" type="number" min="0" />
    </FormField>
    <FormField>
      <FormCheckRadio v-model="form.allow_recording" :input-value="true" name="allow_recording" :label="t('rtc.allowRecording')" />
    </FormField>
    <p class="text-xs text-slate-500">{{ t('rtc.allowRecordingHelp') }}</p>
  </CardBoxModal>

  <RTCModerate v-model="moderateModal" :room="moderating" @changed="onModerated" />

  <CardBoxModal v-model="inviteModal" :title="t('rtc.inviteTitle', { name: inviteForm.room })" :button-label="t('rtc.done')">
    <template v-if="!invite">
      <FormField :label="t('rtc.role')">
        <select v-model="inviteForm.role" class="w-full rounded-lg border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-800 px-3 py-2 text-sm">
          <option value="presenter">{{ t('rtc.rolePresenter') }}</option>
          <option value="viewer">{{ t('rtc.roleViewer') }}</option>
          <option value="observer">{{ t('rtc.roleObserver') }}</option>
          <option value="moderator">{{ t('rtc.roleModerator') }}</option>
        </select>
      </FormField>
      <FormField :label="t('rtc.inviteName')" :help="t('rtc.inviteNameHelp')">
        <FormControl v-model="inviteForm.username" />
      </FormField>
      <FormField :label="t('rtc.validityHours')" :help="t('rtc.validityHelp')">
        <FormControl v-model.number="inviteForm.ttl_hours" type="number" min="1" max="720" />
      </FormField>
      <BaseButton :label="t('rtc.createLink')" color="info" @click="createInvite" />
    </template>
    <template v-else>
      <p class="text-sm text-slate-500 mb-2">{{ t('rtc.inviteReady', { expires: new Date(invite.expires_at).toLocaleString() }) }}</p>
      <code class="block break-all text-xs bg-slate-100 dark:bg-slate-800 rounded p-2 mb-3">{{ invite.join_url }}</code>
      <div class="flex gap-2">
        <BaseButton :icon="mdiContentCopy" :label="t('rtc.copyLink')" color="info" small @click="copy(invite.join_url)" />
        <BaseButton :icon="mdiOpenInNew" :label="t('rtc.open')" color="lightDark" small outline :href="invite.join_url" target="_blank" />
      </div>
      <p class="text-xs text-amber-600 mt-3">{{ t('rtc.inviteSecret') }}</p>
    </template>
  </CardBoxModal>
</template>
