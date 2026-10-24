<script setup>
import BaseButton from "@/components/BaseButton.vue";
import BaseIcon from "@/components/BaseIcon.vue";
import CardBox from "@/components/CardBox.vue";
import CardBoxModal from "@/components/CardBoxModal.vue";
import FormCheckRadio from "@/components/FormCheckRadio.vue";
import TURNSettings from "@/components/RTC/TURNSettings.vue";
import FormControl from "@/components/FormControl.vue";
import FormField from "@/components/FormField.vue";
import ApiService from "@/services/ApiService";
import {
  mdiAlertCircleOutline,
  mdiCancel,
  mdiContentCopy,
  mdiDelete,
  mdiKeyChange,
  mdiPencil,
  mdiPlay,
  mdiPlus,
  mdiRefresh,
  mdiStop,
  mdiTicketConfirmation,
  mdiTransitConnectionVariant
} from '@mdi/js';
import { computed, onMounted, onUnmounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useToast } from "vue-toastification";

const { t } = useI18n();
const toast = useToast();

const status = ref(null)
const apps = ref([])
const sessions = ref([])
const blocked = ref([])
const events = ref([])
const busy = ref(false)

const running = computed(() => status.value?.running === true)
const errMsg = (e) => e?.response?.data?.msg || e?.message || t('turn.actionFailed')

const call = async (fn) => {
  const res = await fn.call(ApiService) // keep `this` for ApiService.get/post
  if (res.data?.error) throw new Error(res.data.msg)
  return res.data?.data
}

const loadStatus = async () => { status.value = await call(ApiService.turnStatus) }
const loadApps = async () => { apps.value = (await call(ApiService.turnListApps)) || [] }
const loadLive = async () => {
  const [s, b, e] = await Promise.all([
    call(ApiService.turnSessions), call(ApiService.turnBlocked), call(ApiService.turnEvents),
  ])
  sessions.value = s || []
  blocked.value = b || []
  events.value = e || []
}

const refreshAll = async () => {
  try {
    await Promise.all([loadStatus(), loadApps(), loadLive()])
  } catch (e) {
    toast.error(errMsg(e))
  }
}

const toggleRunning = async () => {
  busy.value = true
  try {
    status.value = await call(running.value ? ApiService.turnStop : ApiService.turnStart)
  } catch (e) {
    toast.error(errMsg(e))
  } finally {
    busy.value = false
  }
}

// --- apps ---
const appModal = ref(false)
const appEditing = ref(false)
const appForm = ref({ name: '', enabled: true, max_sessions: 0, credential_ttl: 86400, note: '' })
const secretModal = ref(false)
const shownSecret = ref({ name: '', secret: '' })
const credsModal = ref(false)
const issued = ref(null)

const openNewApp = () => {
  appEditing.value = false
  appForm.value = { name: '', enabled: true, max_sessions: 0, credential_ttl: 86400, note: '' }
  appModal.value = true
}
const openEditApp = (a) => {
  appEditing.value = true
  appForm.value = { name: a.name, enabled: a.enabled, max_sessions: a.max_sessions, credential_ttl: a.credential_ttl, note: a.note }
  appModal.value = true
}
const submitApp = async () => {
  try {
    const body = { ...appForm.value, max_sessions: Number(appForm.value.max_sessions) || 0, credential_ttl: Number(appForm.value.credential_ttl) || 0 }
    if (appEditing.value) {
      await call(() => ApiService.turnUpdateApp(body))
      toast.success(t('turn.appUpdated'))
    } else {
      const created = await call(() => ApiService.turnCreateApp(body))
      shownSecret.value = { name: created.name, secret: created.secret }
      secretModal.value = true
    }
    appModal.value = false
    await loadApps()
  } catch (e) {
    toast.error(errMsg(e))
  }
}
const rotateSecret = async (a) => {
  if (!window.confirm(t('turn.rotateConfirm', { name: a.name }))) return
  try {
    const res = await call(() => ApiService.turnRotateSecret(a.name))
    shownSecret.value = { name: res.name, secret: res.secret }
    secretModal.value = true
  } catch (e) {
    toast.error(errMsg(e))
  }
}
const deleteApp = async (a) => {
  if (!window.confirm(t('turn.deleteConfirm', { name: a.name }))) return
  try {
    await call(() => ApiService.turnDeleteApp(a.name))
    await loadApps()
  } catch (e) {
    toast.error(errMsg(e))
  }
}
const issueCreds = async (a) => {
  try {
    issued.value = { app: a.name, ...(await call(() => ApiService.turnIssueCredentials(a.name, 'test'))) }
    credsModal.value = true
  } catch (e) {
    toast.error(errMsg(e))
  }
}

// --- sessions / blocks ---
const killSession = async (s, blockIP) => {
  if (!window.confirm(t(blockIP ? 'turn.killBlockConfirm' : 'turn.killConfirm', { client: s.client_addr }))) return
  try {
    await call(() => ApiService.turnKillSession(s.id, blockIP))
    toast.success(t('turn.killed'))
    await loadLive()
  } catch (e) {
    toast.error(errMsg(e))
  }
}
const unblock = async (ip) => {
  try {
    await call(() => ApiService.turnUnblock(ip))
    await loadLive()
  } catch (e) {
    toast.error(errMsg(e))
  }
}

const copy = async (text) => {
  try {
    await navigator.clipboard.writeText(text)
    toast.success(t('turn.copied'))
  } catch {
    toast.error(t('turn.copyFailed'))
  }
}

const fmtBytes = (n) => {
  if (!n) return '0 B'
  const u = ['B', 'KB', 'MB', 'GB']
  let i = 0
  while (n >= 1024 && i < u.length - 1) { n /= 1024; i++ }
  return `${n.toFixed(i ? 1 : 0)} ${u[i]}`
}
const fmtTime = (v) => (v ? new Date(v).toLocaleString() : '—')
const since = (v) => {
  const s = Math.max(0, Math.floor((Date.now() - new Date(v)) / 1000))
  return s < 60 ? `${s}s` : s < 3600 ? `${Math.floor(s / 60)}m` : `${Math.floor(s / 3600)}h ${Math.floor((s % 3600) / 60)}m`
}

const credSnippet = computed(() => `// Node.js — sign short-lived TURN credentials on your backend
const crypto = require('crypto')
const app = '${shownSecret.value.name}', secret = process.env.TURN_SECRET
const username = \`\${Math.floor(Date.now() / 1000) + 3600}:\${app}:\${userId}\`
const credential = crypto.createHmac('sha1', secret).update(username).digest('base64')
// iceServers: [{ urls: [...], username, credential }]`)

let timer = null
onMounted(async () => {
  await refreshAll()
  timer = setInterval(() => { loadStatus().catch(() => {}); loadLive().catch(() => {}) }, 5000)
})
onUnmounted(() => clearInterval(timer))
</script>

<template>
  <div class="space-y-8">
    <!-- Header -->
    <div class="bg-gradient-to-r from-slate-700 via-blue-800 to-indigo-800 rounded-2xl p-8 text-white shadow-lg">
      <div class="flex flex-col lg:flex-row lg:items-center lg:justify-between">
        <div>
          <h1 class="text-3xl lg:text-4xl font-bold mb-2 flex items-center">
            <BaseIcon :path="mdiTransitConnectionVariant" size="40" class="mr-4" />
            {{ t('turn.title') }}
          </h1>
          <p class="text-blue-100 text-lg">{{ t('turn.subtitle') }}</p>
        </div>
        <div class="mt-6 lg:mt-0 flex items-center gap-3">
          <span :class="['px-3 py-1 rounded-full text-sm font-semibold', running ? 'bg-green-500/30' : 'bg-white/20']">
            ● {{ running ? t('turn.running') : t('turn.stopped') }}<template v-if="running && status?.tls"> · TLS</template>
          </span>
          <BaseButton :icon="mdiRefresh" color="white" outline :disabled="busy" @click="refreshAll" />
          <BaseButton
            :icon="running ? mdiStop : mdiPlay"
            :label="running ? t('turn.stop') : t('turn.start')"
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
        <p class="text-sm font-mono">{{ status.last_error }}</p>
      </div>
    </CardBox>

    <CardBox v-if="status?.urls?.length">
      <h3 class="font-semibold mb-2">{{ t('turn.iceUrls') }}</h3>
      <div v-for="u in status.urls" :key="u" class="flex items-center gap-2 text-sm font-mono">
        <span class="flex-1 break-all">{{ u }}</span>
        <BaseButton :icon="mdiContentCopy" small color="lightDark" outline @click="copy(u)" />
      </div>
    </CardBox>

    <TURNSettings :running="running" @saved="(st) => { if (st) status = st; refreshAll() }" />

    <!-- Apps -->
    <CardBox>
      <div class="flex items-center justify-between mb-4">
        <h3 class="text-lg font-semibold">{{ t('turn.apps') }}</h3>
        <BaseButton :icon="mdiPlus" :label="t('turn.newApp')" color="info" small @click="openNewApp" />
      </div>
      <p class="text-sm text-slate-500 mb-3">{{ t('turn.appsHelp') }}</p>
      <table class="w-full text-sm">
        <thead>
          <tr class="text-left text-slate-500">
            <th class="py-1">{{ t('turn.appName') }}</th>
            <th class="py-1">{{ t('turn.activeSessions') }}</th>
            <th class="py-1">{{ t('turn.maxSessions') }}</th>
            <th class="py-1">TTL</th>
            <th class="py-1">{{ t('turn.note') }}</th>
            <th class="py-1"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="a in apps" :key="a.name" class="border-t border-slate-200 dark:border-slate-700">
            <td class="py-2 font-mono">
              {{ a.name }}
              <span v-if="a.system" class="ml-1 text-xs px-1.5 py-0.5 rounded bg-slate-100 dark:bg-slate-700">system</span>
              <span v-if="!a.enabled" class="ml-1 text-xs px-1.5 py-0.5 rounded bg-red-100 text-red-700">{{ t('turn.disabled') }}</span>
            </td>
            <td class="py-2">{{ a.active_sessions }}</td>
            <td class="py-2">{{ a.max_sessions || t('turn.default') }}</td>
            <td class="py-2">{{ Math.round(a.credential_ttl / 3600) }}h</td>
            <td class="py-2 text-slate-500">{{ a.note }}</td>
            <td class="py-2">
              <div class="flex justify-end gap-1">
                <BaseButton :icon="mdiTicketConfirmation" small color="lightDark" outline :title="t('turn.testCreds')" @click="issueCreds(a)" />
                <BaseButton :icon="mdiPencil" small color="lightDark" outline :title="t('turn.edit')" @click="openEditApp(a)" />
                <BaseButton :icon="mdiKeyChange" small color="warning" outline :title="t('turn.rotate')" @click="rotateSecret(a)" />
                <BaseButton v-if="!a.system" :icon="mdiDelete" small color="danger" outline :title="t('turn.delete')" @click="deleteApp(a)" />
              </div>
            </td>
          </tr>
          <tr v-if="!apps.length"><td colspan="6" class="py-3 text-slate-400">{{ t('turn.noApps') }}</td></tr>
        </tbody>
      </table>
    </CardBox>

    <!-- Sessions -->
    <CardBox>
      <h3 class="text-lg font-semibold mb-4">{{ t('turn.sessions') }} ({{ sessions.length }})</h3>
      <p v-if="!sessions.length" class="text-sm text-slate-400">{{ t('turn.noSessions') }}</p>
      <div v-else class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead>
            <tr class="text-left text-slate-500">
              <th class="py-1">{{ t('turn.appUser') }}</th>
              <th class="py-1">{{ t('turn.client') }}</th>
              <th class="py-1">{{ t('turn.peers') }}</th>
              <th class="py-1">{{ t('turn.duration') }}</th>
              <th class="py-1">↓ / ↑</th>
              <th class="py-1"></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="s in sessions" :key="s.id" class="border-t border-slate-200 dark:border-slate-700">
              <td class="py-2 font-mono">{{ s.app }}<template v-if="s.user">:{{ s.user }}</template></td>
              <td class="py-2 font-mono">{{ s.client_addr }} <span class="text-xs text-slate-400">{{ s.protocol }}</span></td>
              <td class="py-2 font-mono text-xs">{{ (s.peers || []).join(', ') || '—' }}</td>
              <td class="py-2">{{ since(s.started_at) }}</td>
              <td class="py-2 whitespace-nowrap">{{ fmtBytes(s.bytes_in) }} / {{ fmtBytes(s.bytes_out) }}</td>
              <td class="py-2">
                <div class="flex justify-end gap-1">
                  <BaseButton :icon="mdiStop" small color="warning" :title="t('turn.kill')" @click="killSession(s, false)" />
                  <BaseButton :icon="mdiCancel" small color="danger" :title="t('turn.killBlock')" @click="killSession(s, true)" />
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </CardBox>

    <div class="grid grid-cols-1 xl:grid-cols-2 gap-6">
      <!-- Blocked -->
      <CardBox>
        <h3 class="text-lg font-semibold mb-4">{{ t('turn.blocked') }}</h3>
        <p v-if="!blocked.length" class="text-sm text-slate-400">{{ t('turn.noBlocked') }}</p>
        <div v-for="b in blocked" :key="b.ip" class="flex items-center gap-3 text-sm border-t border-slate-200 dark:border-slate-700 py-2">
          <span class="font-mono flex-1">{{ b.ip }}</span>
          <span class="text-slate-500">{{ b.reason }}</span>
          <span class="text-slate-500 whitespace-nowrap">→ {{ fmtTime(b.until) }}</span>
          <BaseButton :label="t('turn.unblock')" small color="lightDark" outline @click="unblock(b.ip)" />
        </div>
      </CardBox>

      <!-- Events -->
      <CardBox>
        <h3 class="text-lg font-semibold mb-1">{{ t('turn.events') }}</h3>
        <p class="text-xs text-slate-500 mb-3">{{ t('turn.eventsHelp') }}</p>
        <div class="max-h-96 overflow-y-auto text-xs font-mono space-y-1">
          <div v-for="(e, i) in events" :key="i" class="flex gap-2">
            <span class="text-slate-400 whitespace-nowrap">{{ new Date(e.time).toLocaleTimeString() }}</span>
            <span
              :class="['whitespace-nowrap', /failed|denied|blocked|killed|exceeded/.test(e.kind) ? 'text-red-500' : 'text-emerald-600']"
            >{{ e.kind }}</span>
            <span class="text-slate-600 dark:text-slate-300 break-all">
              <template v-if="e.app">{{ e.app }}<template v-if="e.user">:{{ e.user }}</template> </template>
              <template v-if="e.client">{{ e.client }} </template>{{ e.detail }}
            </span>
          </div>
          <p v-if="!events.length" class="text-slate-400">{{ t('turn.noEvents') }}</p>
        </div>
      </CardBox>
    </div>

    <!-- App modal -->
    <CardBoxModal
      v-model="appModal"
      :title="appEditing ? t('turn.editApp') : t('turn.newApp')"
      :button-label="t('turn.save')"
      has-cancel
      @confirm="submitApp"
    >
      <FormField :label="t('turn.appName')" :help="t('turn.appNameHelp')">
        <FormControl v-model="appForm.name" :disabled="appEditing" placeholder="my-platform" />
      </FormField>
      <FormField v-if="appEditing">
        <FormCheckRadio v-model="appForm.enabled" :input-value="true" name="app_enabled" :label="t('turn.appEnabled')" />
      </FormField>
      <FormField :label="t('turn.maxSessions')" :help="t('turn.maxSessionsHelp')">
        <FormControl v-model.number="appForm.max_sessions" type="number" min="0" />
      </FormField>
      <FormField :label="t('turn.credentialTTL')" :help="t('turn.credentialTTLHelp')">
        <FormControl v-model.number="appForm.credential_ttl" type="number" min="60" />
      </FormField>
      <FormField :label="t('turn.note')">
        <FormControl v-model="appForm.note" />
      </FormField>
    </CardBoxModal>

    <!-- Secret modal (shown once) -->
    <CardBoxModal v-model="secretModal" :title="t('turn.secretTitle', { name: shownSecret.name })" :button-label="t('turn.done')">
      <p class="text-sm text-amber-600 mb-3">{{ t('turn.secretOnce') }}</p>
      <div class="flex items-center gap-2 mb-4">
        <code class="flex-1 break-all text-sm bg-slate-100 dark:bg-slate-800 rounded p-2">{{ shownSecret.secret }}</code>
        <BaseButton :icon="mdiContentCopy" small color="info" @click="copy(shownSecret.secret)" />
      </div>
      <p class="text-sm text-slate-500 mb-2">{{ t('turn.howToSign') }}</p>
      <pre class="text-xs bg-slate-900 text-slate-100 rounded p-3 overflow-x-auto">{{ credSnippet }}</pre>
    </CardBoxModal>

    <!-- Test credentials -->
    <CardBoxModal v-model="credsModal" :title="t('turn.testCredsTitle', { name: issued?.app })" :button-label="t('turn.done')">
      <template v-if="issued">
        <div class="text-sm space-y-2">
          <div><span class="text-slate-500">username:</span> <code class="break-all">{{ issued.credentials.username }}</code></div>
          <div><span class="text-slate-500">credential:</span> <code class="break-all">{{ issued.credentials.password }}</code></div>
          <div><span class="text-slate-500">{{ t('turn.expires') }}:</span> {{ fmtTime(issued.credentials.expires_at) }}</div>
          <div class="text-slate-500">{{ t('turn.testCredsHelp') }}</div>
        </div>
        <BaseButton
          class="mt-3"
          :icon="mdiContentCopy"
          small
          color="info"
          :label="t('turn.copyIceJson')"
          @click="copy(JSON.stringify([{ urls: issued.urls, username: issued.credentials.username, credential: issued.credentials.password }], null, 2))"
        />
      </template>
    </CardBoxModal>
  </div>
</template>
