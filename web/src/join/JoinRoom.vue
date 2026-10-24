<script setup>
// Redock's room-join page. It speaks Galène's protocol through protocol.js
// (window.ServerConnection), served by the SFU this page is hosted on.
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import VideoTile from './VideoTile.vue'
import { t } from './messages'

const phase = ref('loading') // loading | prejoin | joining | call | ended | error
const errorText = ref('')
const endedText = ref('')
const notices = ref([])

const room = ref('')
let token = ''
const claims = ref({})
const roomTitle = ref('')
let endpoint = ''

const username = ref('')
const permissions = ref([])
const locked = ref(false)

const local = reactive({ stream: null, mic: true, cam: true })
const screen = reactive({ stream: null, up: null })
let cameraUp = null
let sc = null

const remotes = ref([]) // { id, username, label, stream }
const users = ref([]) // { id, username, permissions }
const chat = ref([])
const chatInput = ref('')
const panel = ref('') // '' | 'chat' | 'people'
const chatBox = ref(null)

const canPresent = computed(() => permissions.value.includes('present'))
const isOp = computed(() => permissions.value.includes('op'))
const canRecord = computed(() => permissions.value.includes('record'))
const recording = computed(() => users.value.some((u) => u.username === 'RECORDING'))
const others = computed(() => users.value.filter((u) => u.id !== sc?.id && u.username !== 'RECORDING'))
const namePinned = computed(() => !!claims.value.sub)
const canShareScreen = !!navigator.mediaDevices?.getDisplayMedia
const invitePerms = computed(() => claims.value.permissions || [])

const notify = (text, kind = 'info') => {
  const n = { id: Math.random(), text, kind }
  notices.value.push(n)
  setTimeout(() => { notices.value = notices.value.filter((x) => x.id !== n.id) }, 5000)
}

// --- link & room ----------------------------------------------------------

const decodeClaims = (jwt) => {
  try {
    const part = jwt.split('.')[1].replace(/-/g, '+').replace(/_/g, '/')
    const bytes = Uint8Array.from(atob(part), (ch) => ch.charCodeAt(0))
    return JSON.parse(new TextDecoder().decode(bytes))
  } catch {
    return null
  }
}

const readLink = () => {
  const params = new URLSearchParams(location.hash.replace(/^#/, ''))
  room.value = params.get('room') || ''
  token = params.get('token') || ''
  // Keep the token out of the address bar, history and screenshots.
  if (location.hash) history.replaceState(null, '', location.pathname)
  const c = room.value && token ? decodeClaims(token) : null
  if (!c) return t('badLink')
  if (c.exp && c.exp * 1000 < Date.now()) return t('expired')
  claims.value = c
  username.value = c.sub || localStorage.getItem('redock-meet-name') || ''
  return ''
}

const loadRoom = async () => {
  const res = await fetch(`/group/${encodeURIComponent(room.value)}/.status`)
  if (!res.ok) throw new Error(t('roomUnavailable'))
  const status = await res.json()
  endpoint = status.endpoint
  roomTitle.value = status.displayName || room.value
  locked.value = !!status.locked
  if (!endpoint) throw new Error(t('roomUnavailable'))
}

// --- local media ----------------------------------------------------------

const stopStream = (s) => s?.getTracks().forEach((tr) => tr.stop())

const startPreview = async () => {
  if (!invitePerms.value.includes('present')) return
  if (!window.isSecureContext) {
    notify(t('insecure'), 'warning')
    return
  }
  try {
    local.stream = await navigator.mediaDevices.getUserMedia({ audio: true, video: { width: 1280, height: 720 } })
  } catch {
    try {
      local.stream = await navigator.mediaDevices.getUserMedia({ audio: true })
      local.cam = false
    } catch {
      notify(t('permissionDenied'), 'warning')
    }
  }
}

const toggleMic = () => {
  local.mic = !local.mic
  local.stream?.getAudioTracks().forEach((tr) => { tr.enabled = local.mic })
}

const toggleCam = () => {
  local.cam = !local.cam
  local.stream?.getVideoTracks().forEach((tr) => { tr.enabled = local.cam })
}

// --- publishing -----------------------------------------------------------

const publish = (stream, label) => {
  const c = sc.newUpStream()
  c.label = label
  for (const track of stream.getTracks()) {
    c.pc.addTransceiver(track, { direction: 'sendonly', streams: [stream] })
  }
  c.setStream(stream)
  return c
}

const publishCamera = () => {
  if (!canPresent.value || !local.stream || cameraUp) return
  cameraUp = publish(local.stream, 'camera')
  cameraUp.onclose = () => { cameraUp = null }
}

const toggleScreen = async () => {
  if (screen.up) {
    screen.up.close()
    stopStream(screen.stream)
    screen.up = null
    screen.stream = null
    return
  }
  try {
    const s = await navigator.mediaDevices.getDisplayMedia({ video: true, audio: true })
    screen.stream = s
    screen.up = publish(s, 'screenshare')
    screen.up.onclose = () => { screen.up = null; stopStream(screen.stream); screen.stream = null }
    s.getVideoTracks()[0].onended = () => { if (screen.up) toggleScreen() }
  } catch {
    /* cancelled */
  }
}

// --- connection -----------------------------------------------------------

const refreshUsers = () => {
  users.value = Object.entries(sc.users || {}).map(([id, u]) => ({
    id, username: u.username, permissions: u.permissions || [],
  }))
}

const join = async () => {
  if (!namePinned.value && !username.value.trim()) return
  if (!namePinned.value) localStorage.setItem('redock-meet-name', username.value.trim())
  phase.value = 'joining'

  sc = new window.ServerConnection()
  sc.onconnected = async () => {
    try {
      await sc.join(room.value, namePinned.value ? null : username.value.trim(), { type: 'token', token })
    } catch (e) {
      fail(e.message || String(e))
    }
  }
  sc.onjoined = (kind, _group, perms, status, _data, error, message) => {
    permissions.value = perms || []
    if (status && 'locked' in status) locked.value = !!status.locked
    switch (kind) {
      case 'join':
        phase.value = 'call'
        username.value = sc.username || username.value
        sc.request({ '': ['audio', 'video'] })
        publishCamera()
        break
      case 'change':
        if (!canPresent.value && cameraUp) cameraUp.close()
        break
      case 'fail':
        fail(error || message || t('roomUnavailable'))
        break
      case 'leave':
        end(t('left'))
        break
    }
  }
  sc.onuser = () => refreshUsers()
  sc.ondownstream = (c) => {
    const tile = { id: c.id, username: c.username, label: c.label, stream: c.stream }
    remotes.value = [...remotes.value.filter((r) => r.id !== c.id), tile]
    const update = () => {
      remotes.value = remotes.value.map((r) => (r.id === c.id ? { ...r, stream: c.stream, username: c.username } : r))
    }
    c.ondowntrack = update
    c.onnegotiationcompleted = update
    c.onclose = () => { remotes.value = remotes.value.filter((r) => r.id !== c.id) }
  }
  sc.onchat = async (id, _source, _dest, name, time, _privileged, isHistory, kind, message) => {
    if (kind && kind !== 'me') return
    chat.value.push({ id: Math.random(), name, time: new Date(time), text: message, mine: id === sc.id, kind })
    if (!isHistory && panel.value !== 'chat' && id !== sc.id) notify(`${name}: ${message}`)
    await nextTick()
    if (chatBox.value) chatBox.value.scrollTop = chatBox.value.scrollHeight
  }
  sc.onusermessage = (_id, _dest, _name, _time, privileged, kind, error, message) => {
    if (kind === 'mute' && privileged) {
      local.mic = true
      toggleMic()
      notify(t('mutedByModerator'), 'warning')
    } else if (kind === 'kicked') {
      end(t('kicked'))
    } else if (kind === 'error' || kind === 'warning' || kind === 'info') {
      notify(String(message || error), kind)
    }
  }
  sc.onerror = (e) => notify(String(e?.message || e), 'error')
  sc.onclose = () => { if (phase.value === 'call' || phase.value === 'joining') end(t('disconnected')) }

  try {
    await sc.connect(endpoint)
  } catch (e) {
    fail(e.message || String(e))
  }
}

const fail = (text) => {
  errorText.value = text
  phase.value = 'error'
  sc?.close()
}

const end = (text) => {
  if (phase.value === 'ended') return
  endedText.value = text
  phase.value = 'ended'
  if (screen.up) toggleScreen()
  remotes.value = []
  sc?.close()
}

const leave = () => end(t('left'))

const rejoin = async () => {
  remotes.value = []
  chat.value = []
  if (!local.stream) await startPreview()
  join()
}

// --- chat & moderation ----------------------------------------------------

const sendChat = () => {
  const text = chatInput.value.trim()
  if (!text) return
  sc.chat('', '', text)
  chatInput.value = ''
}

const kick = (u) => sc.userAction('kick', u.id, 'removed by a moderator')
const askMute = (u) => sc.userMessage('mute', u.id, null)
const toggleLock = () => sc.groupAction(locked.value ? 'unlock' : 'lock', '')
const toggleRecord = () => sc.groupAction(recording.value ? 'unrecord' : 'record')

// --- lifecycle --------------------------------------------------------------

onMounted(async () => {
  if (typeof window.ServerConnection !== 'function') {
    errorText.value = t('roomUnavailable')
    phase.value = 'error'
    return
  }
  const linkError = readLink()
  if (linkError) {
    errorText.value = linkError
    phase.value = 'error'
    return
  }
  try {
    await loadRoom()
  } catch (e) {
    errorText.value = e.message
    phase.value = 'error'
    return
  }
  await startPreview()
  phase.value = 'prejoin'
})

onBeforeUnmount(() => {
  sc?.close()
  stopStream(local.stream)
})
window.addEventListener('beforeunload', () => sc?.close())

const gridClass = computed(() => {
  const n = remotes.value.length + (local.stream && canPresent.value ? 1 : 0)
  if (n <= 1) return 'grid-cols-1 max-w-4xl'
  if (n <= 4) return 'grid-cols-1 sm:grid-cols-2 max-w-6xl'
  return 'grid-cols-2 lg:grid-cols-3 xl:grid-cols-4'
})
</script>

<template>
  <div class="min-h-screen bg-slate-950 text-slate-100 flex flex-col">
    <!-- notices -->
    <div class="fixed top-4 right-4 z-50 space-y-2 w-80">
      <div
        v-for="n in notices"
        :key="n.id"
        :class="['rounded-lg px-4 py-2 text-sm shadow-lg', n.kind === 'error' ? 'bg-red-600' : n.kind === 'warning' ? 'bg-amber-600' : 'bg-slate-700']"
      >{{ n.text }}</div>
    </div>

    <!-- loading / error / ended -->
    <div v-if="phase === 'loading'" class="flex-1 flex items-center justify-center text-slate-400">{{ t('loading') }}</div>

    <div v-else-if="phase === 'error' || phase === 'ended'" class="flex-1 flex items-center justify-center p-6">
      <div class="max-w-md w-full bg-slate-900 rounded-2xl p-8 text-center space-y-4">
        <h1 class="text-xl font-semibold">{{ roomTitle || 'Redock Meet' }}</h1>
        <p class="text-slate-300">{{ phase === 'error' ? errorText : endedText }}</p>
        <button
          v-if="phase === 'ended' && endedText !== t('kicked')"
          class="px-5 py-2 rounded-lg bg-emerald-600 hover:bg-emerald-500"
          @click="rejoin"
        >{{ t('rejoin') }}</button>
      </div>
    </div>

    <!-- pre-join -->
    <div v-else-if="phase === 'prejoin' || phase === 'joining'" class="flex-1 flex items-center justify-center p-6">
      <div class="max-w-3xl w-full grid md:grid-cols-2 gap-6 items-center">
        <div>
          <VideoTile v-if="local.stream" :stream="local.stream" :name="username || t('you')" muted mirror />
          <div v-else class="rounded-xl bg-slate-800 aspect-video flex items-center justify-center text-slate-400 text-sm px-6 text-center">
            {{ invitePerms.includes('present') ? t('noCamera') : t('watchOnly') }}
          </div>
          <div v-if="local.stream" class="flex justify-center gap-3 mt-3">
            <button :class="['px-3 py-2 rounded-lg text-sm', local.mic ? 'bg-slate-700' : 'bg-red-600']" @click="toggleMic">
              🎙 {{ local.mic ? t('mute') : t('unmute') }}
            </button>
            <button :class="['px-3 py-2 rounded-lg text-sm', local.cam ? 'bg-slate-700' : 'bg-red-600']" @click="toggleCam">
              📷 {{ local.cam ? t('camOff') : t('camOn') }}
            </button>
          </div>
        </div>
        <div class="space-y-4">
          <h1 class="text-2xl font-semibold">{{ roomTitle }}</h1>
          <p v-if="locked" class="text-amber-400 text-sm">🔒 {{ t('locked') }}</p>
          <div v-if="!namePinned">
            <label class="block text-sm text-slate-400 mb-1">{{ t('yourName') }}</label>
            <input
              v-model="username"
              maxlength="64"
              :placeholder="t('namePlaceholder')"
              class="w-full rounded-lg bg-slate-800 border border-slate-700 px-3 py-2 focus:outline-none focus:border-emerald-500"
              @keyup.enter="join"
            />
          </div>
          <p v-else class="text-slate-300">{{ username }}</p>
          <button
            class="w-full px-5 py-3 rounded-lg bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 font-semibold"
            :disabled="phase === 'joining' || (!namePinned && !username.trim())"
            @click="join"
          >{{ phase === 'joining' ? t('joining') : t('join') }}</button>
        </div>
      </div>
    </div>

    <!-- call -->
    <template v-else-if="phase === 'call'">
      <header class="flex items-center gap-3 px-4 py-3 border-b border-slate-800">
        <h1 class="font-semibold truncate">{{ roomTitle }}</h1>
        <span v-if="recording" class="flex items-center gap-1 text-xs px-2 py-1 rounded bg-red-600 animate-pulse">● {{ t('recording') }}</span>
        <span v-if="locked" class="text-xs px-2 py-1 rounded bg-amber-700">🔒 {{ t('locked') }}</span>
        <span v-if="isOp" class="text-xs px-2 py-1 rounded bg-emerald-700">{{ t('moderator') }}</span>
      </header>

      <div class="flex-1 flex min-h-0">
        <main class="flex-1 p-4 overflow-y-auto">
          <div :class="['grid gap-4 mx-auto', gridClass]">
            <VideoTile
              v-if="local.stream && canPresent"
              :stream="local.stream"
              :name="`${username} (${t('you')})`"
              muted
              mirror
            />
            <VideoTile v-if="screen.stream" :stream="screen.stream" :name="`${username} — ${t('screen')}`" muted />
            <VideoTile
              v-for="r in remotes"
              :key="r.id"
              :stream="r.stream"
              :name="r.label === 'screenshare' ? `${r.username} — ${t('screen')}` : r.username"
            />
          </div>
          <p v-if="!remotes.length" class="text-center text-slate-500 mt-8">{{ t('nobody') }}</p>
        </main>

        <aside v-if="panel" class="w-80 border-l border-slate-800 flex flex-col">
          <template v-if="panel === 'chat'">
            <div ref="chatBox" class="flex-1 overflow-y-auto p-3 space-y-2 text-sm">
              <div v-for="m in chat" :key="m.id">
                <span :class="['font-semibold', m.mine ? 'text-emerald-400' : 'text-sky-400']">{{ m.name }}</span>
                <span class="text-slate-500 text-xs ml-1">{{ m.time.toLocaleTimeString().slice(0, 5) }}</span>
                <p class="text-slate-200 break-words whitespace-pre-wrap">{{ m.text }}</p>
              </div>
            </div>
            <form v-if="permissions.includes('message')" class="p-3 border-t border-slate-800 flex gap-2" @submit.prevent="sendChat">
              <input
                v-model="chatInput"
                maxlength="2000"
                :placeholder="t('chatPlaceholder')"
                class="flex-1 rounded-lg bg-slate-800 border border-slate-700 px-3 py-2 text-sm focus:outline-none"
              />
              <button class="px-3 py-2 rounded-lg bg-emerald-600 text-sm">{{ t('send') }}</button>
            </form>
          </template>
          <template v-else>
            <div class="p-3 space-y-2 text-sm overflow-y-auto">
              <div class="flex items-center justify-between">
                <span class="font-semibold">{{ username }} ({{ t('you') }})</span>
              </div>
              <div v-for="u in others" :key="u.id" class="flex items-center gap-2">
                <span class="flex-1 truncate">{{ u.username }}</span>
                <span v-if="u.permissions.includes('op')" class="text-xs px-1.5 rounded bg-emerald-800">{{ t('moderator') }}</span>
                <template v-if="isOp">
                  <button class="text-xs px-2 py-1 rounded bg-slate-700" :title="t('askMute')" @click="askMute(u)">🔇</button>
                  <button class="text-xs px-2 py-1 rounded bg-red-700" :title="t('kick')" @click="kick(u)">✕</button>
                </template>
              </div>
              <div v-if="isOp" class="pt-3 mt-3 border-t border-slate-800 space-y-2">
                <button class="w-full px-3 py-2 rounded-lg bg-slate-700 text-sm" @click="toggleLock">
                  {{ locked ? t('unlock') : t('lock') }}
                </button>
                <button v-if="canRecord" class="w-full px-3 py-2 rounded-lg bg-red-700 text-sm" @click="toggleRecord">
                  {{ recording ? t('stopRecord') : t('record') }}
                </button>
              </div>
            </div>
          </template>
        </aside>
      </div>

      <footer class="flex items-center justify-center gap-3 px-4 py-3 border-t border-slate-800">
        <template v-if="canPresent && local.stream">
          <button :class="['px-4 py-2 rounded-full text-sm', local.mic ? 'bg-slate-700' : 'bg-red-600']" :title="local.mic ? t('mute') : t('unmute')" @click="toggleMic">🎙</button>
          <button :class="['px-4 py-2 rounded-full text-sm', local.cam ? 'bg-slate-700' : 'bg-red-600']" :title="local.cam ? t('camOff') : t('camOn')" @click="toggleCam">📷</button>
        </template>
        <button
          v-if="canPresent && canShareScreen"
          :class="['px-4 py-2 rounded-full text-sm', screen.up ? 'bg-emerald-600' : 'bg-slate-700']"
          :title="screen.up ? t('stopShare') : t('share')"
          @click="toggleScreen"
        >🖥</button>
        <button :class="['px-4 py-2 rounded-full text-sm', panel === 'chat' ? 'bg-emerald-600' : 'bg-slate-700']" :title="t('chat')" @click="panel = panel === 'chat' ? '' : 'chat'">💬</button>
        <button :class="['px-4 py-2 rounded-full text-sm', panel === 'people' ? 'bg-emerald-600' : 'bg-slate-700']" :title="t('people')" @click="panel = panel === 'people' ? '' : 'people'">
          👥 {{ others.length + 1 }}
        </button>
        <button class="px-5 py-2 rounded-full text-sm bg-red-600 hover:bg-red-500" @click="leave">{{ t('leave') }}</button>
      </footer>
    </template>
  </div>
</template>
