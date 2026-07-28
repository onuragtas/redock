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
  mdiAccount,
  mdiAccountPlus,
  mdiAlert,
  mdiChartLine,
  mdiChevronDown,
  mdiChevronUp,
  mdiCodeJson,
  mdiCog,
  mdiContentDuplicate,
  mdiDelete,
  mdiDownload,
  mdiFileDocumentOutline,
  mdiFilterOff,
  mdiLock,
  mdiMagnify,
  mdiNetwork,
  mdiPlay,
  mdiPlus,
  mdiQrcode,
  mdiRefresh,
  mdiReplay,
  mdiServer,
  mdiServerNetwork,
  mdiShieldLock,
  mdiSort,
  mdiSortAscending,
  mdiSortDescending,
  mdiStop,
  mdiTimer
} from '@mdi/js';
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { useToast } from 'vue-toastification';
import { useI18n } from 'vue-i18n';

const toast = useToast();
const { t } = useI18n();

// Reactive state
const loading = ref(false)
const statistics = ref({
  total_servers: 0,
  total_users: 0,
  active_connections: 0
})
const bandwidthStats = ref({
  total_received: 0,
  total_sent: 0,
  total_bandwidth: 0,
  top_users: []
})
const connectionStats = ref({
  total_connections: 0,
  total_duration: 0,
  avg_duration: 0,
  active_users_24h: 0
})
const servers = ref([])
const users = ref([])
const connections = ref([])

// Traffic Inspector (Live Traffic tab)
const flows = ref([])
const expandedFlowId = ref(null)
const payloadViewMode = ref('text')
const decodedRequest = ref(null)
const decodedResponse = ref(null)
const MAX_FLOWS = 300

// Traffic Inspector — pipeline log events (kind: "log"): operational warnings/errors from the
// interception pipeline itself (TLS handshake rejections, pf natlookup failures, upstream dial
// failures, …), as opposed to flow events which carry captured connection content. Kept in a
// completely separate reactive array so they never leak into `flows`/`flowIndex`/the transaction
// table — see ingestLogEvent/routing in connectTrafficSocket below.
const trafficLogs = ref([])
const trafficLogsExpanded = ref(false)
const MAX_TRAFFIC_LOGS = 200

// Resend (replay) modal state — see "--- Resend (replay) ---" below for the methods that
// populate/consume it.
const isResendModalActive = ref(false)
const resendRow = ref(null)
const resendRequest = ref({ method: 'GET', url: '', headersText: '', body: '' })
// True when the captured request body is non-text (image/video/audio/pdf/binary) — in that case
// the body textarea just shows an informational note and the ORIGINAL captured bytes are resent
// unmodified (as base64), regardless of whatever ends up in the textarea.
const resendBodyBinary = ref(false)
const resendBinaryBodyBase64 = ref('')
const resendLoading = ref(false)
const resendResult = ref(null) // { status, statusText, durationMs, decoded } | { error } | null
let trafficSocket = null
let flowIndex = new Map() // flow_id -> reactive flow object (kept out of Vue reactivity tracking)

// Ticks once/second while the Live Traffic tab is active so the "Duration" column can keep
// advancing for open (!closed) flows without re-fetching anything — same on/off lifecycle as
// the traffic WebSocket (see the activeTab watcher below). Static once a flow is `closed`.
const trafficNowTick = ref(Date.now())
let trafficTickInterval = null

// Memoizes the cheap per-row HTTP parse (Info/User-Agent/Content-Type columns) keyed by
// flow_id + chunk count, so it isn't redone on every re-render (e.g. while typing in the
// search box) when nothing about the flow's captured bytes has actually changed.
const flowRowInfoCache = new Map()

// Memoizes a flow's split-into-transactions result (see splitHttpMessages/computeFlowTransactions
// below), keyed the same way as flowRowInfoCache — by flow_id + chunk count — so re-parsing only
// happens when new chunks have actually arrived for that flow.
const flowTransactionsCache = new Map()

// Live Traffic quick-filter bar state. Purely client-side/derived — never mutates
// `flows` itself so incoming WS events keep appending correctly regardless of
// whatever filter/sort the user currently has selected.
const TRAFFIC_PROTOCOLS = ['tcp-tls', 'tcp-plain', 'quic']
const trafficSearch = ref('')
const trafficProtocolFilter = ref([]) // multi-select; empty = all protocols
const trafficUserFilter = ref(null) // null = all users
const trafficStatusFilter = ref('all') // all | active | closed
const trafficSortKey = ref('lastSeen') // 'lastSeen' | 'bytes'
const trafficSortDir = ref('desc') // 'asc' | 'desc' — defaults to newest-first, like a live monitor

// Modal states
const activeTab = ref('overview')
const isAddServerModalActive = ref(false)
const isEditServerModalActive = ref(false)
const isAddUserModalActive = ref(false)
const isEditUserModalActive = ref(false)
const isDeleteModalActive = ref(false)
const isQRCodeModalActive = ref(false)
const deleteTarget = ref({ type: '', item: null })
const editingServer = ref(null)
const editingUser = ref(null)
const selectedServer = ref(null)
const qrCodeData = ref({ config: '', qrcode: '', username: '' })

// Form data
const newServer = ref({
  name: '',
  address: '10.0.0.1/24',
  endpoint: '',
  dns: '1.1.1.1,8.8.8.8',
  listen_port: 51820,
  mtu: 1420,
  persistent_keepalive: 25,
  enabled: true,
  description: ''
})

const newUser = ref({
  server_id: null,
  username: '',
  email: '',
  full_name: '',
  allowed_ips: '0.0.0.0/0',
  dns: '',
  quota: 0,
  notes: ''
})

// Auto-refresh interval
let refreshInterval = null

// Computed properties
const activeServers = computed(() => {
  return servers.value.filter(s => s.enabled)
})

const activeUsers = computed(() => {
  return users.value.filter(u => u.enabled)
})

const serverUsers = computed(() => {
  if (!selectedServer.value) return users.value
  return users.value.filter(u => u.server_id === selectedServer.value)
})

// API Methods
const fetchStatistics = async () => {
  try {
    const response = await ApiService.get('/v1/vpn/statistics')
    if (response.data && !response.data.error) {
      statistics.value = response.data.data
    }
  } catch (error) {
    console.error('Failed to fetch VPN statistics:', error)
  }
}

const fetchBandwidthStats = async () => {
  try {
    const response = await ApiService.get('/v1/vpn/statistics/bandwidth')
    if (response.data && !response.data.error) {
      bandwidthStats.value = response.data.data
    }
  } catch (error) {
    console.error('Failed to fetch bandwidth statistics:', error)
  }
}

const fetchConnectionStats = async () => {
  try {
    const response = await ApiService.get('/v1/vpn/statistics/connections')
    if (response.data && !response.data.error) {
      connectionStats.value = response.data.data
    }
  } catch (error) {
    console.error('Failed to fetch connection statistics:', error)
  }
}

const fetchServers = async () => {
  try {
    const response = await ApiService.get('/v1/vpn/servers')
    if (response.data && !response.data.error) {
      servers.value = response.data.data || []
    }
  } catch (error) {
    console.error('Failed to fetch servers:', error)
    toast.error(t('vpn.fetchServersFailed'))
  }
}

const fetchUsers = async (serverId = null) => {
  try {
    const url = serverId 
      ? `/v1/vpn/users?server_id=${serverId}`
      : '/v1/vpn/users'
    const response = await ApiService.get(url)
    if (response.data && !response.data.error) {
      users.value = response.data.data || []
    }
  } catch (error) {
    console.error('Failed to fetch users:', error)
    toast.error(t('vpn.fetchUsersFailed'))
  }
}

const fetchConnections = async () => {
  try {
    const response = await ApiService.get('/v1/vpn/connections')
    if (response.data && !response.data.error) {
      connections.value = response.data.data || []
    }
  } catch (error) {
    console.error('Failed to fetch connections:', error)
  }
}

const createServer = async () => {
  try {
    loading.value = true
    const response = await ApiService.post('/v1/vpn/servers', newServer.value)
    if (response.data && !response.data.error) {
      toast.success(t('vpn.serverCreated'))
      isAddServerModalActive.value = false
      resetServerForm()
      await fetchServers()
      await fetchStatistics()
    } else {
      toast.error(response.data?.msg || t('vpn.createServerFailed'))
    }
  } catch (error) {
    console.error('Failed to create server:', error)
    toast.error(t('vpn.createServerFailed'))
  } finally {
    loading.value = false
  }
}

const updateServer = async () => {
  try {
    loading.value = true
    const response = await ApiService.put(`/v1/vpn/servers/${editingServer.value.id}`, editingServer.value)
    if (response.data && !response.data.error) {
      toast.success(t('vpn.serverUpdated'))
      isEditServerModalActive.value = false
      editingServer.value = null
      await fetchServers()
    } else {
      toast.error(response.data?.msg || t('vpn.updateServerFailed'))
    }
  } catch (error) {
    console.error('Failed to update server:', error)
    toast.error(t('vpn.updateServerFailed'))
  } finally {
    loading.value = false
  }
}

const deleteServer = async () => {
  try {
    loading.value = true
    const response = await ApiService.delete(`/v1/vpn/servers/${deleteTarget.value.item.id}`)
    if (response.data && !response.data.error) {
      toast.success(t('vpn.serverDeleted'))
      isDeleteModalActive.value = false
      deleteTarget.value = { type: '', item: null }
      await fetchServers()
      await fetchStatistics()
    } else {
      toast.error(response.data?.msg || t('vpn.deleteServerFailed'))
    }
  } catch (error) {
    console.error('Failed to delete server:', error)
    toast.error(t('vpn.deleteServerFailed'))
  } finally {
    loading.value = false
  }
}

const startServer = async (serverId) => {
  try {
    const response = await ApiService.post(`/v1/vpn/servers/${serverId}/start`)
    if (response.data && !response.data.error) {
      toast.success(t('vpn.serverStarted'))
      await fetchServers()
    } else {
      toast.error(response.data?.msg || t('vpn.startServerFailed'))
    }
  } catch (error) {
    console.error('Failed to start server:', error)
    toast.error(t('vpn.startServerFailed'))
  }
}

const stopServer = async (serverId) => {
  try {
    const response = await ApiService.post(`/v1/vpn/servers/${serverId}/stop`)
    if (response.data && !response.data.error) {
      toast.success(t('vpn.serverStopped'))
      await fetchServers()
    } else {
      toast.error(response.data?.msg || t('vpn.stopServerFailed'))
    }
  } catch (error) {
    console.error('Failed to stop server:', error)
    toast.error(t('vpn.stopServerFailed'))
  }
}

const createUser = async () => {
  try {
    loading.value = true
    // Extract server_id value if it's an object (from FormControl select)
    const userData = { ...newUser.value }
    if (userData.server_id && typeof userData.server_id === 'object' && userData.server_id.value !== undefined) {
      userData.server_id = userData.server_id.value
    }
    const response = await ApiService.post('/v1/vpn/users', userData)
    if (response.data && !response.data.error) {
      toast.success(t('vpn.userCreated'))
      isAddUserModalActive.value = false
      resetUserForm()
      await fetchUsers()
      await fetchStatistics()
    } else {
      toast.error(response.data?.msg || t('vpn.createUserFailed'))
    }
  } catch (error) {
    console.error('Failed to create user:', error)
    toast.error(t('vpn.createUserFailed'))
  } finally {
    loading.value = false
  }
}

const updateUser = async () => {
  try {
    loading.value = true
    // Extract server_id value if it's an object (from FormControl select)
    const userData = { ...editingUser.value }
    if (userData.server_id && typeof userData.server_id === 'object' && userData.server_id.value !== undefined) {
      userData.server_id = userData.server_id.value
    }
    const response = await ApiService.put(`/v1/vpn/users/${editingUser.value.id}`, userData)
    if (response.data && !response.data.error) {
      toast.success(t('vpn.userUpdated'))
      isEditUserModalActive.value = false
      editingUser.value = null
      await fetchUsers()
    } else {
      toast.error(response.data?.msg || t('vpn.updateUserFailed'))
    }
  } catch (error) {
    console.error('Failed to update user:', error)
    toast.error(t('vpn.updateUserFailed'))
  } finally {
    loading.value = false
  }
}

const deleteUser = async () => {
  try {
    loading.value = true
    const response = await ApiService.delete(`/v1/vpn/users/${deleteTarget.value.item.id}`)
    if (response.data && !response.data.error) {
      toast.success(t('vpn.userDeleted'))
      isDeleteModalActive.value = false
      deleteTarget.value = { type: '', item: null }
      await fetchUsers()
      await fetchStatistics()
    } else {
      toast.error(response.data?.msg || t('vpn.deleteUserFailed'))
    }
  } catch (error) {
    console.error('Failed to delete user:', error)
    toast.error(t('vpn.deleteUserFailed'))
  } finally {
    loading.value = false
  }
}

// Shared Blob + synthetic <a> download pattern, used by the CA cert / config downloads below
// and by the per-flow .txt/.har export actions in the Live Traffic tab.
const triggerDownload = (blob, filename) => {
  const url = window.URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  window.URL.revokeObjectURL(url)
}

const downloadConfig = async (userId) => {
  try {
    const response = await ApiService.get(`/v1/vpn/users/${userId}/config`, {
      responseType: 'blob'
    })
    triggerDownload(new Blob([response.data], { type: 'text/plain' }), `wg-${userId}.conf`)
    toast.success(t('vpn.configDownloaded'))
  } catch (error) {
    console.error('Failed to download config:', error)
    toast.error(t('vpn.downloadConfigFailed'))
  }
}

const getQRCode = async (userId) => {
  try {
    const response = await ApiService.get(`/v1/vpn/users/${userId}/qrcode`)
    if (response.data && !response.data.error) {
      const user = users.value.find(u => u.id === userId)
      qrCodeData.value = {
        config: response.data.data.config,
        qrcode: response.data.data.qrcode,
        username: user?.username || 'User'
      }
      isQRCodeModalActive.value = true
    }
  } catch (error) {
    console.error('Failed to get QR code:', error)
    toast.error(t('vpn.qrFailed'))
  }
}

// --- Traffic Inspector methods ---

const toggleInspection = async (user) => {
  try {
    const response = await ApiService.put(`/v1/vpn/users/${user.id}`, { inspection_enabled: !user.inspection_enabled })
    if (response.data && !response.data.error) {
      await fetchUsers(selectedServer.value)
    } else {
      toast.error(response.data?.msg || t('vpn.updateUserFailed'))
    }
  } catch (error) {
    console.error('Failed to toggle inspection:', error)
    toast.error(t('vpn.updateUserFailed'))
  }
}

const downloadCA = async () => {
  try {
    const response = await ApiService.get('/v1/vpn/ca.pem', {
      options: { responseType: 'blob' }
    })
    triggerDownload(new Blob([response.data], { type: 'application/x-pem-file' }), 'redock-traffic-inspector-ca.pem')
    toast.success(t('vpn.caDownloaded'))
  } catch (error) {
    console.error('Failed to download CA certificate:', error)
    toast.error(t('vpn.caDownloadFailed'))
  }
}

const base64ByteLength = (b64) => {
  if (!b64) return 0
  let len = b64.length
  let padding = 0
  if (b64.endsWith('==')) padding = 2
  else if (b64.endsWith('=')) padding = 1
  return Math.max(0, Math.floor((len * 3) / 4) - padding)
}

const base64ToBytes = (b64) => {
  try {
    const binary = atob(b64)
    const bytes = new Uint8Array(binary.length)
    for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)
    return bytes
  } catch (e) {
    return new Uint8Array(0)
  }
}

const getOrCreateFlow = (evt) => {
  let flow = flowIndex.get(evt.flow_id)
  if (!flow) {
    flow = {
      flow_id: evt.flow_id,
      user_id: evt.user_id,
      protocol: evt.protocol,
      sni: evt.sni || '',
      host: evt.host,
      port: evt.port,
      firstSeen: evt.timestamp, // set once — start of the Duration column, never updated again
      lastSeen: evt.timestamp, // last-updated — bumped on every event, end of Duration once closed
      bytes: 0,
      bytesSent: 0, // client->server (request-side / upload)
      bytesReceived: 0, // server->client (response-side / download)
      closed: false,
      error: '',
      chunks: []
    }
    flowIndex.set(evt.flow_id, flow)
    flows.value.push(flow)

    // Bound the in-memory flow list; evict the oldest flow first.
    if (flows.value.length > MAX_FLOWS) {
      const removed = flows.value.shift()
      if (removed) {
        flowIndex.delete(removed.flow_id)
        flowRowInfoCache.delete(removed.flow_id)
        flowTransactionsCache.delete(removed.flow_id)
      }
    }
  }
  return flow
}

const ingestFlowEvent = (evt) => {
  if (!evt || !evt.flow_id) return
  const flow = getOrCreateFlow(evt)
  flow.lastSeen = evt.timestamp || flow.lastSeen
  if (evt.sni) flow.sni = evt.sni
  if (evt.host) flow.host = evt.host
  if (evt.port) flow.port = evt.port
  if (evt.protocol) flow.protocol = evt.protocol
  if (evt.data) {
    const len = base64ByteLength(evt.data)
    flow.bytes += len
    if (evt.direction === 'client->server') flow.bytesSent += len
    else if (evt.direction === 'server->client') flow.bytesReceived += len
    flow.chunks.push({ direction: evt.direction, data: evt.data, timestamp: evt.timestamp })
  }
  if (evt.closed) flow.closed = true
  if (evt.error) flow.error = evt.error
}

const fetchFlows = async () => {
  try {
    const response = await ApiService.get('/v1/vpn/flows')
    if (response.data && !response.data.error) {
      const backlog = response.data.data || []
      backlog.forEach((evt) => ingestFlowEvent(evt))
    }
  } catch (error) {
    console.error('Failed to fetch traffic flows:', error)
  }
}

// --- Traffic Inspector pipeline logs (separate from flows — see trafficLogs above) ---

// Log events have no stable ID, so de-dup the backlog-vs-live boundary by (timestamp, message).
// Kept out of Vue reactivity, same rationale as flowIndex above.
const trafficLogKeys = new Set()
const trafficLogKey = (evt) => `${evt.timestamp}|${evt.message}`

const ingestLogEvent = (evt) => {
  if (!evt || !evt.message) return
  const key = trafficLogKey(evt)
  if (trafficLogKeys.has(key)) return
  trafficLogKeys.add(key)
  trafficLogs.value.push({ timestamp: evt.timestamp, level: evt.level || 'warning', message: evt.message })

  // Bound the in-memory log list; evict the oldest entry first — same eviction pattern as MAX_FLOWS.
  if (trafficLogs.value.length > MAX_TRAFFIC_LOGS) {
    const removed = trafficLogs.value.shift()
    if (removed) trafficLogKeys.delete(trafficLogKey(removed))
  }
}

const fetchTrafficLogs = async () => {
  try {
    const response = await ApiService.get('/v1/vpn/traffic-logs')
    if (response.data && !response.data.error) {
      const backlog = response.data.data || []
      backlog.forEach((evt) => ingestLogEvent(evt))
    }
  } catch (error) {
    console.error('Failed to fetch traffic logs:', error)
  }
}

// Clears only the locally-displayed log list — there's no backend call to clear the backlog.
const clearTrafficLogs = () => {
  trafficLogs.value = []
  trafficLogKeys.clear()
}

// Most-recent-first, matching the default (desc/lastSeen) ordering convention of filteredFlows.
const sortedTrafficLogs = computed(() => trafficLogs.value.slice().reverse())

const connectTrafficSocket = () => {
  if (trafficSocket) return
  let wsUrl = window.location.hostname + (window.location.port == '5173' ? ':6001' : (window.location.port !== '' ? ':' + window.location.port : ''))
  const scheme = window.location.protocol === 'https:' ? 'wss://' : 'ws://'
  wsUrl = scheme + wsUrl + '/ws/traffic'
  const token = ApiService.getJWT()
  if (token) wsUrl += (wsUrl.includes('?') ? '&' : '?') + 'token=' + encodeURIComponent(token)

  const socket = new WebSocket(wsUrl)

  socket.onmessage = (event) => {
    try {
      const evt = JSON.parse(event.data)
      if (evt && evt.kind === 'log') {
        ingestLogEvent(evt)
      } else {
        ingestFlowEvent(evt)
      }
    } catch (e) {
      console.error('Failed to parse traffic event:', e)
    }
  }

  socket.onerror = (error) => {
    console.error('Traffic inspector WebSocket error:', error)
  }

  socket.onclose = () => {
    if (trafficSocket === socket) trafficSocket = null
  }

  trafficSocket = socket
}

const disconnectTrafficSocket = () => {
  if (trafficSocket) {
    try {
      trafficSocket.close()
    } catch (e) {
      // ignore
    }
    trafficSocket = null
  }
}

// expandedFlowId now holds a compound "flowId:transactionIndex" (or "flowId:fallback") key —
// see rowKey() — so expanding one transaction row doesn't affect any other row from the same
// underlying flow.
const toggleFlow = (row) => {
  const key = rowKey(row)
  expandedFlowId.value = expandedFlowId.value === key ? null : key
}

const usernameForFlow = (userId) => {
  const user = users.value.find((u) => u.id === userId)
  return user ? user.username : String(userId)
}

// Elapsed seconds since the flow's first-seen event. For open flows this advances live off
// `trafficNowTick` (see the activeTab watcher below); once `closed` is set it's a fixed value
// based on the last event actually seen, so it stops ticking exactly when the flow does.
const flowDurationSeconds = (flow) => {
  if (!flow || !flow.firstSeen) return 0
  const endSeconds = flow.closed ? (flow.lastSeen || flow.firstSeen) : (trafficNowTick.value / 1000)
  return Math.max(0, Math.round(endSeconds - flow.firstSeen))
}

// --- Live Traffic filter bar ---

const toggleTrafficProtocolFilter = (proto) => {
  const idx = trafficProtocolFilter.value.indexOf(proto)
  if (idx === -1) trafficProtocolFilter.value.push(proto)
  else trafficProtocolFilter.value.splice(idx, 1)
}

const hasActiveTrafficFilters = computed(() => (
  trafficSearch.value.trim() !== '' ||
  trafficProtocolFilter.value.length > 0 ||
  trafficUserFilter.value !== null ||
  trafficStatusFilter.value !== 'all'
))

const clearTrafficFilters = () => {
  trafficSearch.value = ''
  trafficProtocolFilter.value = []
  trafficUserFilter.value = null
  trafficStatusFilter.value = 'all'
}

// Toggles the sort column; clicking the already-active column flips direction
// instead of resetting it, so re-clicking "Time" bounces between newest/oldest first.
const setTrafficSort = (key) => {
  if (trafficSortKey.value === key) {
    trafficSortDir.value = trafficSortDir.value === 'asc' ? 'desc' : 'asc'
  } else {
    trafficSortKey.value = key
    trafficSortDir.value = 'desc'
  }
}

// Filtered + sorted view over `flows`, driven by the quick-filter bar. Derived only —
// `flows` itself is left untouched so the WebSocket handler can keep appending/evicting
// without any knowledge of the current filter/sort state.
const filteredFlows = computed(() => {
  const q = trafficSearch.value.trim().toLowerCase()
  const protocols = trafficProtocolFilter.value
  const userId = trafficUserFilter.value
  const status = trafficStatusFilter.value
  const key = trafficSortKey.value
  const dir = trafficSortDir.value === 'asc' ? 1 : -1

  const list = flows.value.filter((flow) => {
    if (protocols.length > 0 && !protocols.includes(flow.protocol)) return false
    if (userId !== null && flow.user_id !== userId) return false
    if (status === 'active' && flow.closed) return false
    if (status === 'closed' && !flow.closed) return false
    if (q) {
      const haystack = `${flow.host || ''} ${flow.sni || ''}`.toLowerCase()
      if (!haystack.includes(q)) return false
    }
    return true
  })

  return list.slice().sort((a, b) => {
    const av = key === 'bytes' ? (a.bytes || 0) : (a.lastSeen || 0)
    const bv = key === 'bytes' ? (b.bytes || 0) : (b.lastSeen || 0)
    return (av - bv) * dir
  })
})

const formatTrafficTime = (unixSeconds) => {
  if (!unixSeconds) return '-'
  try {
    return new Date(unixSeconds * 1000).toLocaleTimeString()
  } catch (e) {
    return '-'
  }
}

// Expands `filteredFlows` into a flat list of table rows: one row per HTTP transaction
// (request/response pair) for flows that parse as HTTP, or exactly one whole-flow row for
// flows that don't look like HTTP at all yet (or don't have any complete messages parsed out
// of them yet) — same as today's single-row-per-flow behavior, unchanged for that case.
const flowTransactionRows = computed(() => {
  const rows = []
  for (const flow of filteredFlows.value) {
    const transactions = getFlowTransactions(flow)
    if (transactions.length === 0) {
      rows.push({ flow, transactionIndex: -1, request: null, response: null, isLast: true, isFallback: true })
      continue
    }
    transactions.forEach((tx, idx) => {
      rows.push({
        flow,
        transactionIndex: idx,
        request: tx.request,
        response: tx.response,
        isLast: idx === transactions.length - 1,
        isFallback: false
      })
    })
  }
  return rows
})

const expandedRow = computed(() => flowTransactionRows.value.find((r) => rowKey(r) === expandedFlowId.value) || null)

// Raw (undecoded) bytes for the Text/Hex view tabs: for the non-HTTP fallback row, every chunk
// from both directions in capture order (unchanged legacy whole-connection dump); for a real
// transaction, just that transaction's own request bytes followed by its response bytes.
const expandedRowRawBytes = computed(() => {
  const row = expandedRow.value
  if (!row) return new Uint8Array(0)
  if (row.isFallback) {
    return concatBytes(row.flow.chunks.filter((c) => c.data).map((c) => base64ToBytes(c.data)))
  }
  return concatBytes([bytesForRow(row, 'client->server'), bytesForRow(row, 'server->client')])
})

const expandedFlowText = computed(() => {
  try {
    return new TextDecoder('utf-8', { fatal: false }).decode(expandedRowRawBytes.value)
  } catch (e) {
    return ''
  }
})

// Shared by the expanded flow's Hex view and the per-flow .txt export (for non-HTTP/opaque
// bodies, where a hex+ASCII dump reads more usefully than raw bytes or a wall of base64).
const bytesToHexDump = (bytes) => {
  const lines = []
  for (let i = 0; i < bytes.length; i += 16) {
    const slice = bytes.slice(i, i + 16)
    const hex = Array.from(slice).map((b) => b.toString(16).padStart(2, '0')).join(' ')
    const ascii = Array.from(slice).map((b) => (b >= 32 && b < 127 ? String.fromCharCode(b) : '.')).join('')
    lines.push(i.toString(16).padStart(8, '0') + '  ' + hex.padEnd(16 * 3 - 1, ' ') + '  ' + ascii)
  }
  return lines.join('\n')
}

const expandedFlowHex = computed(() => bytesToHexDump(expandedRowRawBytes.value))

// --- Decoded HTTP view (per-direction) ---

const concatBytes = (byteArrays) => {
  const total = byteArrays.reduce((sum, p) => sum + p.length, 0)
  const out = new Uint8Array(total)
  let offset = 0
  for (const part of byteArrays) {
    out.set(part, offset)
    offset += part.length
  }
  return out
}

const bytesForDirection = (flow, direction) => {
  if (!flow) return new Uint8Array(0)
  return concatBytes(flow.chunks.filter((c) => c.data && c.direction === direction).map((c) => base64ToBytes(c.data)))
}

// The currently-expanded row's own request/response bytes — precisely bounded to that one
// transaction (or the whole direction, for the non-HTTP fallback row) — fed to decodeHttpMessage
// for the Decoded view and reused by the per-row .txt/.har export actions below.
const expandedRowRequestBytes = computed(() => bytesForRow(expandedRow.value, 'client->server'))
const expandedRowResponseBytes = computed(() => bytesForRow(expandedRow.value, 'server->client'))

const HTTP_METHODS = ['GET ', 'POST ', 'PUT ', 'DELETE ', 'HEAD ', 'OPTIONS ', 'PATCH ', 'CONNECT ', 'TRACE ']

const looksLikeHttp = (bytes) => {
  if (!bytes || bytes.length < 4) return false
  const head = new TextDecoder('utf-8', { fatal: false }).decode(bytes.slice(0, 16))
  return head.startsWith('HTTP/1.') || HTTP_METHODS.some((m) => head.startsWith(m))
}

const findSubarrayFrom = (bytes, needle, fromIndex) => {
  outer: for (let i = fromIndex; i <= bytes.length - needle.length; i++) {
    for (let j = 0; j < needle.length; j++) {
      if (bytes[i + j] !== needle[j]) continue outer
    }
    return i
  }
  return -1
}

const findSubarray = (bytes, needle) => findSubarrayFrom(bytes, needle, 0)

const splitHttpHeaders = (bytes) => {
  let idx = findSubarray(bytes, [13, 10, 13, 10])
  let sepLen = 4
  if (idx === -1) {
    idx = findSubarray(bytes, [10, 10])
    sepLen = 2
  }
  if (idx === -1) {
    return { headerText: new TextDecoder('utf-8', { fatal: false }).decode(bytes), bodyBytes: new Uint8Array(0) }
  }
  return {
    headerText: new TextDecoder('utf-8', { fatal: false }).decode(bytes.slice(0, idx)),
    bodyBytes: bytes.slice(idx + sepLen)
  }
}

const getHeaderValue = (headerText, name) => {
  const match = headerText.match(new RegExp('^' + name + '\\s*:\\s*(.*)$', 'im'))
  return match ? match[1].trim() : ''
}

// --- Lightweight per-row HTTP line/header parsing (table row Info/User-Agent/Content-Type
// columns) ---
//
// Deliberately separate from decodeHttpMessage: it never dechunks or decompresses the body,
// and only scans a bounded prefix of each direction's bytes rather than the full accumulated
// payload. Header blocks are always plaintext HTTP/1.1 text regardless of whether the body is
// chunked/gzipped/brotli'd, so this stays cheap enough to run for every visible row on every
// render — including while a multi-MB response body is still streaming in.
const ROW_HEADER_SCAN_CAP = 32 * 1024 // headers are always far smaller than this; bounds cost even against huge bodies

const boundedBytesForDirection = (flow, direction, cap = ROW_HEADER_SCAN_CAP) => {
  if (!flow) return new Uint8Array(0)
  const parts = []
  let total = 0
  for (const c of flow.chunks) {
    if (!c.data || c.direction !== direction) continue
    const bytes = base64ToBytes(c.data)
    parts.push(bytes)
    total += bytes.length
    if (total >= cap) break
  }
  return concatBytes(parts)
}

const firstLineText = (bytes) => {
  if (!bytes || bytes.length === 0) return ''
  const idx = findSubarray(bytes, [13, 10])
  const end = idx === -1 ? bytes.length : idx
  return new TextDecoder('utf-8', { fatal: false }).decode(bytes.slice(0, end)).trim()
}

// Header block only, up to (not including) the blank-line terminator — no dechunk/decompress.
const headerBlockText = (bytes) => {
  if (!bytes || bytes.length === 0) return ''
  let idx = findSubarray(bytes, [13, 10, 13, 10])
  if (idx === -1) idx = findSubarray(bytes, [10, 10])
  const end = idx === -1 ? bytes.length : idx
  return new TextDecoder('utf-8', { fatal: false }).decode(bytes.slice(0, end))
}

const REQUEST_LINE_RE = /^([A-Z]{2,10})\s+(\S+)\s+HTTP\/\d(?:\.\d)?$/
const STATUS_LINE_RE = /^HTTP\/\d(?:\.\d)?\s+(\d{3})(?:\s+(.*))?$/

// Same request-line/status-line match, but against an already-isolated single message's
// header text (as produced by splitHttpMessages) rather than a bounded scan of raw bytes —
// used for per-transaction Info/User-Agent/Content-Type once a message has been precisely
// bounded, so there's no need to re-derive the first line from scratch.
const requestLineFromHeaderText = (headerText) => (headerText || '').split(/\r\n|\n/)[0].trim().match(REQUEST_LINE_RE)
const statusLineFromHeaderText = (headerText) => (headerText || '').split(/\r\n|\n/)[0].trim().match(STATUS_LINE_RE)

// Computes the cheap fields for one flow's table row. Not reactive itself — callers go through
// getFlowRowInfo() below, which memoizes this by chunk count so it's not redone every render.
const computeFlowRowInfo = (flow) => {
  const reqBytes = boundedBytesForDirection(flow, 'client->server')
  const resBytes = boundedBytesForDirection(flow, 'server->client')

  const reqLineMatch = firstLineText(reqBytes).match(REQUEST_LINE_RE)
  const resLineMatch = firstLineText(resBytes).match(STATUS_LINE_RE)

  let info = ''
  if (reqLineMatch) {
    info = `${reqLineMatch[1]} ${reqLineMatch[2]}`
    if (resLineMatch) info += ` → ${resLineMatch[1]}`
  } else if (resLineMatch) {
    info = `→ ${resLineMatch[1]}`
  }

  const userAgent = reqLineMatch ? getHeaderValue(headerBlockText(reqBytes), 'User-Agent') : ''
  const contentType = resLineMatch ? getHeaderValue(headerBlockText(resBytes), 'Content-Type') : ''

  return {
    info,
    method: reqLineMatch ? reqLineMatch[1] : '',
    path: reqLineMatch ? reqLineMatch[2] : '',
    status: resLineMatch ? resLineMatch[1] : '',
    userAgent,
    contentType,
    isHttpRequest: !!reqLineMatch
  }
}

const getFlowRowInfo = (flow) => {
  const cached = flowRowInfoCache.get(flow.flow_id)
  const chunkCount = flow.chunks.length
  if (cached && cached.chunkCount === chunkCount) return cached.info
  const info = computeFlowRowInfo(flow)
  flowRowInfoCache.set(flow.flow_id, { chunkCount, info })
  return info
}

// A row only qualifies for HAR export once it looks like a real HTTP request+response pair.
// For the non-HTTP fallback row this is the old flow-wide check; for a real transaction row,
// both a request and a (paired) response message must actually exist and the request must
// parse as an HTTP request-line.
const canExportHar = (row) => {
  if (row.isFallback) return row.flow.bytesSent > 0 && row.flow.bytesReceived > 0 && getFlowRowInfo(row.flow).isHttpRequest
  return !!(row.request && row.response && requestLineFromHeaderText(row.request.headerText))
}

// Reassembles an HTTP/1.1 "Transfer-Encoding: chunked" body into its real bytes:
// <hex-size>\r\n<size bytes>\r\n ... 0\r\n\r\n
const dechunkBody = (bytes) => {
  const parts = []
  let pos = 0
  while (pos < bytes.length) {
    let lineEnd = -1
    for (let i = pos; i < bytes.length - 1; i++) {
      if (bytes[i] === 13 && bytes[i + 1] === 10) { lineEnd = i; break }
    }
    if (lineEnd === -1) break
    const sizeLine = new TextDecoder('ascii', { fatal: false }).decode(bytes.slice(pos, lineEnd)).split(';')[0].trim()
    const size = parseInt(sizeLine, 16)
    if (isNaN(size) || size === 0) break
    const chunkStart = lineEnd + 2
    const chunkEnd = Math.min(chunkStart + size, bytes.length)
    parts.push(bytes.slice(chunkStart, chunkEnd))
    pos = chunkEnd + 2 // skip the CRLF that follows each chunk's data
  }
  return concatBytes(parts)
}

// Scans a "Transfer-Encoding: chunked" body starting at byte offset `pos` and returns the
// offset immediately *after* the terminating 0\r\n\r\n, or -1 if the terminator hasn't
// arrived yet (the body is still streaming in). Walks chunk-by-chunk like dechunkBody, but
// tracks exact offsets instead of reassembling the body — splitHttpMessages needs to know
// precisely where this message ends so it can resume scanning for the next one right after it.
const findChunkedTerminator = (bytes, pos) => {
  while (pos < bytes.length) {
    let lineEnd = -1
    for (let i = pos; i < bytes.length - 1; i++) {
      if (bytes[i] === 13 && bytes[i + 1] === 10) { lineEnd = i; break }
    }
    if (lineEnd === -1) return -1 // chunk-size line hasn't fully arrived yet
    const sizeLine = new TextDecoder('ascii', { fatal: false }).decode(bytes.slice(pos, lineEnd)).split(';')[0].trim()
    const size = parseInt(sizeLine, 16)
    if (isNaN(size)) return -1 // malformed or still-incomplete size line — wait for more bytes
    if (size === 0) {
      // Terminating chunk: "0\r\n" then an (almost always empty) trailer block, then a final
      // blank line. That blank line's CRLFCRLF starts at `lineEnd` when there are no trailers,
      // or later if there are — either way the next \r\n\r\n from here on is the true terminator.
      const idx = findSubarrayFrom(bytes, [13, 10, 13, 10], lineEnd)
      return idx === -1 ? -1 : idx + 4
    }
    const chunkStart = lineEnd + 2
    const chunkEnd = chunkStart + size
    if (chunkEnd + 2 > bytes.length) return -1 // this chunk's data (+ trailing CRLF) hasn't fully arrived
    pos = chunkEnd + 2
  }
  return -1
}

// Parses a growing byte stream (one direction's accumulated flow chunks) into a sequence of
// discrete, correctly-bounded HTTP/1.1 messages. Safe to re-run on a longer version of the
// same bytes as more chunks stream in — always produces a superset of the previously-found
// complete messages, and marks a still-in-progress trailing message `complete: false` rather
// than guessing at where it ends. This is what fixes the keep-alive bug: previously the whole
// accumulated stream was treated as a single HTTP message, so pipelined/sequential requests on
// one TCP+TLS connection collapsed into one (wrong) decode.
const splitHttpMessages = (bytes) => {
  const messages = []
  let offset = 0
  while (offset < bytes.length) {
    const sepIdx = findSubarrayFrom(bytes, [13, 10, 13, 10], offset)
    if (sepIdx === -1) break // no complete header block yet for a next message — stop here
    const headerText = new TextDecoder('utf-8', { fatal: false }).decode(bytes.slice(offset, sepIdx))
    const bodyStart = sepIdx + 4
    const transferEncoding = getHeaderValue(headerText, 'Transfer-Encoding')
    const contentLengthHeader = getHeaderValue(headerText, 'Content-Length')

    if (/chunked/i.test(transferEncoding)) {
      const term = findChunkedTerminator(bytes, bodyStart)
      if (term === -1) {
        // Still streaming in — this is the last (incomplete) message for now.
        messages.push({ startOffset: offset, endOffset: bytes.length, headerText, bodyBytes: dechunkBody(bytes.slice(bodyStart)), complete: false })
        break
      }
      messages.push({ startOffset: offset, endOffset: term, headerText, bodyBytes: dechunkBody(bytes.slice(bodyStart, term)), complete: true })
      offset = term
      continue
    }

    if (contentLengthHeader !== '' && /^\d+$/.test(contentLengthHeader.trim())) {
      const contentLength = parseInt(contentLengthHeader, 10)
      const bodyEnd = bodyStart + contentLength
      if (bodyEnd > bytes.length) {
        // Fewer than Content-Length bytes available so far — incomplete, stop here.
        messages.push({ startOffset: offset, endOffset: bytes.length, headerText, bodyBytes: bytes.slice(bodyStart), complete: false })
        break
      }
      messages.push({ startOffset: offset, endOffset: bodyEnd, headerText, bodyBytes: bytes.slice(bodyStart, bodyEnd), complete: true })
      offset = bodyEnd
      continue
    }

    // Neither chunked nor Content-Length present. Per RFC 7230 §3.3.2/§3.3.3, this means two
    // different things depending on which side of the connection we're parsing:
    //  - A REQUEST with no body-length header has a body of length ZERO — this is the
    //    overwhelmingly common case (GET/HEAD/DELETE etc. carry no body at all). Treating it as
    //    "read until connection close" would swallow every subsequent pipelined request on the
    //    same keep-alive connection into this one's "body" — i.e. reintroducing the exact bug
    //    this parser exists to fix, just on the request side instead of the response side.
    //  - A RESPONSE with no body-length header genuinely does fall back to body-until-close
    //    (the server is relying on the connection closing to signal the end of the body), and
    //    there cannot reliably be a subsequent message after that on the same connection.
    const firstLine = (headerText.split(/\r\n|\n/)[0] || '').trim()
    if (REQUEST_LINE_RE.test(firstLine)) {
      messages.push({ startOffset: offset, endOffset: bodyStart, headerText, bodyBytes: new Uint8Array(0), complete: true })
      offset = bodyStart
      continue
    }
    messages.push({ startOffset: offset, endOffset: bytes.length, headerText, bodyBytes: bytes.slice(bodyStart), complete: true })
    break
  }
  return messages
}

// --- HTTP transaction pairing (fixes the keep-alive/pipelined-requests bug) ---
//
// A single TCP+TLS (or QUIC) flow can carry many sequential HTTP/1.1 request/response pairs
// when the client reuses the connection (Connection: keep-alive). Each direction's bytes are
// split independently via splitHttpMessages, then paired up index-for-index: the i-th request
// is assumed to be answered by the i-th response. This assumes strictly sequential
// (non-pipelined) request/response ordering — the overwhelmingly common case for HTTP/1.1
// clients, which is what was verified in the live bug report (requests and responses appeared
// in matching sequential order). Full HTTP pipelining reorder support (where a client fires
// several requests before any response arrives, and responses aren't guaranteed to come back
// in the same order for some non-compliant servers) is out of scope here.
const computeFlowTransactions = (flow) => {
  const reqBytes = bytesForDirection(flow, 'client->server')
  const resBytes = bytesForDirection(flow, 'server->client')
  const reqIsHttp = looksLikeHttp(reqBytes)
  const resIsHttp = looksLikeHttp(resBytes)
  if (!reqIsHttp && !resIsHttp) return [] // non-HTTP-parseable (e.g. Noise handshake, QUIC/H3 framing) — caller falls back to one whole-flow row

  const reqMessages = reqIsHttp ? splitHttpMessages(reqBytes) : []
  const resMessages = resIsHttp ? splitHttpMessages(resBytes) : []
  const count = Math.max(reqMessages.length, resMessages.length)
  const transactions = []
  for (let i = 0; i < count; i++) {
    transactions.push({ request: reqMessages[i] || null, response: resMessages[i] || null })
  }
  return transactions
}

// Memoized by chunk count, same pattern as getFlowRowInfo, so re-splitting only happens once
// new bytes have actually arrived for that flow (not on every re-render).
const getFlowTransactions = (flow) => {
  const cached = flowTransactionsCache.get(flow.flow_id)
  const chunkCount = flow.chunks.length
  if (cached && cached.chunkCount === chunkCount) return cached.transactions
  const transactions = computeFlowTransactions(flow)
  flowTransactionsCache.set(flow.flow_id, { chunkCount, transactions })
  return transactions
}

// Per-transaction equivalent of computeFlowRowInfo — much simpler since request/response are
// already precisely bounded single messages, so there's no need for the 32KB-bounded-scan
// trick (that was there to keep a cheap scan cheap against a whole multi-transaction stream).
const computeTransactionInfo = (request, response) => {
  const reqLineMatch = request ? requestLineFromHeaderText(request.headerText) : null
  const resLineMatch = response ? statusLineFromHeaderText(response.headerText) : null

  let info = ''
  if (reqLineMatch) {
    info = `${reqLineMatch[1]} ${reqLineMatch[2]}`
    if (resLineMatch) info += ` → ${resLineMatch[1]}`
  } else if (resLineMatch) {
    info = `→ ${resLineMatch[1]}`
  }

  const userAgent = reqLineMatch ? getHeaderValue(request.headerText, 'User-Agent') : ''
  const contentType = resLineMatch ? getHeaderValue(response.headerText, 'Content-Type') : ''

  return {
    info,
    method: reqLineMatch ? reqLineMatch[1] : '',
    path: reqLineMatch ? reqLineMatch[2] : '',
    status: resLineMatch ? resLineMatch[1] : '',
    userAgent,
    contentType,
    isHttpRequest: !!reqLineMatch
  }
}

// Resolves the Info/User-Agent/Content-Type fields for one table row, whichever shape it is.
const transactionInfoForRow = (row) => (
  row.isFallback ? getFlowRowInfo(row.flow) : computeTransactionInfo(row.request, row.response)
)

// Extracts one row's request or response bytes precisely: for a real transaction, the exact
// wire-slice for that message (header+body, as captured); for the non-HTTP fallback row, the
// whole direction's bytes (unchanged legacy behavior).
const bytesForRow = (row, direction) => {
  if (!row) return new Uint8Array(0)
  if (row.isFallback) return bytesForDirection(row.flow, direction)
  const msg = direction === 'client->server' ? row.request : row.response
  if (!msg) return new Uint8Array(0)
  return bytesForDirection(row.flow, direction).slice(msg.startOffset, msg.endOffset)
}

// Finds the timestamp of whichever raw chunk (in the given direction) covers a given byte
// offset within that direction's concatenated stream — used to derive a per-transaction
// timestamp/duration from the flow's per-chunk timestamps, since messages themselves don't
// carry one directly.
const chunkTimestampAtByte = (flow, direction, byteIndex) => {
  let pos = 0
  for (const c of flow.chunks) {
    if (!c.data || c.direction !== direction) continue
    const len = base64ByteLength(c.data)
    if (byteIndex < pos + len) return c.timestamp
    pos += len
  }
  return null
}

// Best-guess "when did this row happen" timestamp: the request's start for a normal
// transaction (matching how a DevTools Network tab timestamps by request initiation), or the
// response's start if there was no request captured. Fallback rows keep the old flow.lastSeen.
const rowTimestamp = (row) => {
  if (row.isFallback) return row.flow.lastSeen
  if (row.request) return chunkTimestampAtByte(row.flow, 'client->server', row.request.startOffset) ?? row.flow.firstSeen
  if (row.response) return chunkTimestampAtByte(row.flow, 'server->client', row.response.startOffset) ?? row.flow.firstSeen
  return row.flow.firstSeen
}

// Per-transaction duration: from the request's first byte to the response's last byte. While
// the response hasn't completed yet, keeps ticking live off trafficNowTick (same as the
// flow-level flowDurationSeconds), so a still-in-flight transaction's timer visibly advances
// instead of looking frozen.
const rowDurationSeconds = (row) => {
  if (row.isFallback) return flowDurationSeconds(row.flow)
  const flow = row.flow
  const startTs = row.request
    ? (chunkTimestampAtByte(flow, 'client->server', row.request.startOffset) ?? flow.firstSeen)
    : (row.response ? (chunkTimestampAtByte(flow, 'server->client', row.response.startOffset) ?? flow.firstSeen) : flow.firstSeen)
  let endTs
  if (row.response && row.response.complete) {
    endTs = chunkTimestampAtByte(flow, 'server->client', Math.max(0, row.response.endOffset - 1)) ?? flow.lastSeen
  } else {
    endTs = flow.closed ? flow.lastSeen : (trafficNowTick.value / 1000)
  }
  return Math.max(0, Math.round(endTs - startTs))
}

// Per-transaction byte counts, computed from each message's own precise byte offsets rather
// than repeating the whole connection's totals on every row — matches "one row = one HTTP
// call" semantics much better than a flow-wide total that's the same on every row.
const rowByteCounts = (row) => {
  if (row.isFallback) return { sent: row.flow.bytesSent, received: row.flow.bytesReceived }
  return {
    sent: row.request ? (row.request.endOffset - row.request.startOffset) : 0,
    received: row.response ? (row.response.endOffset - row.response.startOffset) : 0
  }
}

// A row's transaction status: 'closed' once the underlying connection has closed, 'pending'
// while a request has arrived but its paired response hasn't (or hasn't finished streaming)
// yet — so a live monitor doesn't have to wait for full completion before showing the row —
// or 'active' otherwise. Fallback (non-HTTP) rows keep the old closed/active-only distinction.
const rowStatusLabel = (row) => {
  if (row.flow.closed) return 'closed'
  if (!row.isFallback && (!row.response || !row.response.complete)) return 'pending'
  return 'active'
}

// A stable, unique key per rendered row: real transactions are keyed by flow + index, the
// non-HTTP fallback row by flow + a fixed suffix (so it can never collide with transaction 0
// if a later re-parse suddenly finds HTTP messages, e.g. once enough bytes have streamed in).
const rowKey = (row) => `${row.flow.flow_id}:${row.isFallback ? 'fallback' : row.transactionIndex}`

// Brotli has no native DecompressionStream support in browsers, so we lazily load a small
// WASM decoder (brotli-dec-wasm) on first use and cache the init promise module-wide, so the
// ~200KB WASM chunk is neither bundled eagerly nor re-fetched/re-instantiated on every flow open.
let brotliModulePromise = null
const getBrotliModule = () => {
  if (!brotliModulePromise) {
    brotliModulePromise = import('brotli-dec-wasm')
      .then((mod) => mod.default)
      .catch((e) => {
        // Allow a later retry (e.g. transient network failure fetching the .wasm asset)
        // instead of permanently caching a rejected promise.
        brotliModulePromise = null
        throw e
      })
  }
  return brotliModulePromise
}

const decompressBody = async (bytes, contentEncoding) => {
  const enc = contentEncoding.toLowerCase()
  if (enc.includes('br')) {
    let brotli
    try {
      brotli = await getBrotliModule()
    } catch (e) {
      // WASM module failed to load/init (e.g. blocked by CSP or a bundling issue) —
      // distinct from a corrupt/truncated stream, which is handled below.
      return { bytes, note: t('vpn.decodeModuleUnavailable', { encoding: contentEncoding }) }
    }
    try {
      return { bytes: brotli.decompress(bytes), note: '' }
    } catch (e) {
      return { bytes, note: t('vpn.decodeFailed', { encoding: contentEncoding }) }
    }
  }
  const format = enc.includes('gzip') ? 'gzip' : (enc.includes('deflate') ? 'deflate' : null)
  if (!format) {
    return { bytes, note: t('vpn.decodeUnavailable', { encoding: contentEncoding }) }
  }
  if (typeof DecompressionStream === 'undefined') {
    return { bytes, note: t('vpn.decodeUnavailable', { encoding: contentEncoding }) }
  }
  try {
    const stream = new Blob([bytes]).stream().pipeThrough(new DecompressionStream(format))
    const buf = await new Response(stream).arrayBuffer()
    return { bytes: new Uint8Array(buf), note: '' }
  } catch (e) {
    return { bytes, note: t('vpn.decodeFailed', { encoding: contentEncoding }) }
  }
}

// Uint8Array -> base64, chunked to avoid a call-stack overflow from
// String.fromCharCode.apply on large (e.g. multi-MB image) arrays.
const bytesToBase64 = (bytes) => {
  let binary = ''
  const chunkSize = 8192
  for (let i = 0; i < bytes.length; i += chunkSize) {
    binary += String.fromCharCode.apply(null, bytes.subarray(i, i + chunkSize))
  }
  return btoa(binary)
}

// Maps a Content-Type to a rendering strategy for the Decoded view.
// 'image'/'video'/'audio'/'pdf' get an inline preview via a data URL;
// 'binary' (anything else non-text) gets a download link instead of an
// attempt to force-decode it as text; 'json'/'text' render as text, with
// 'json' additionally pretty-printed.
const classifyContentType = (contentType) => {
  const ct = (contentType || '').toLowerCase()
  if (ct.startsWith('image/')) return 'image'
  if (ct.startsWith('video/')) return 'video'
  if (ct.startsWith('audio/')) return 'audio'
  if (ct.includes('application/pdf')) return 'pdf'
  if (ct.includes('json')) return 'json'
  if (ct.startsWith('text/') || ct.includes('xml') || ct.includes('javascript') || ct.includes('svg')) return 'text'
  if (!ct) return 'text' // no Content-Type at all — fall back to the old lenient text/JSON-sniffing behavior
  return 'binary'
}

const decodeHttpMessage = async (bytes) => {
  if (!looksLikeHttp(bytes)) return null
  const { headerText, bodyBytes } = splitHttpHeaders(bytes)
  const transferEncoding = getHeaderValue(headerText, 'Transfer-Encoding')
  const contentEncoding = getHeaderValue(headerText, 'Content-Encoding')
  const contentType = getHeaderValue(headerText, 'Content-Type')

  let body = /chunked/i.test(transferEncoding) ? dechunkBody(bodyBytes) : bodyBytes
  let note = ''
  if (contentEncoding) {
    const result = await decompressBody(body, contentEncoding)
    body = result.bytes
    note = result.note
  }

  const kind = classifyContentType(contentType)
  const mime = (contentType.split(';')[0].trim()) || 'application/octet-stream'
  const base = { headerText, contentType, byteLength: body.length, note }

  if (body.length === 0) {
    return { ...base, kind: 'empty', bodyText: '', dataUrl: null }
  }

  if (kind === 'image' || kind === 'video' || kind === 'audio' || kind === 'pdf') {
    return { ...base, kind, bodyText: '', dataUrl: `data:${mime};base64,${bytesToBase64(body)}` }
  }

  if (kind === 'binary') {
    return { ...base, kind, bodyText: '', dataUrl: `data:${mime};base64,${bytesToBase64(body)}` }
  }

  // 'json' or 'text' (or no Content-Type at all)
  let bodyText = new TextDecoder('utf-8', { fatal: false }).decode(body)
  const trimmed = bodyText.trim()
  const looksJson = kind === 'json' || trimmed.startsWith('{') || trimmed.startsWith('[')
  if (looksJson && trimmed) {
    try {
      bodyText = JSON.stringify(JSON.parse(trimmed), null, 2)
    } catch (e) {
      // not valid JSON after all — keep the raw decoded text
    }
  }

  return { ...base, kind: looksJson ? 'json' : 'text', bodyText, dataUrl: null }
}

// --- Per-flow export (.txt / .har) ---

const EXPORT_HEX_DUMP_CAP = 8192 // bodies at/below this render as a hex+ASCII dump; larger ones as base64 to keep the .txt file manageable

// Renders one direction's raw bytes for the .txt export: bodies below the cap get a readable
// hex+ASCII dump (matching the expanded flow's own Hex view), larger ones fall back to base64
// so the export stays a reasonable size.
const renderExportBody = (bytes) => {
  if (!bytes || bytes.length === 0) return '(empty)'
  if (bytes.length <= EXPORT_HEX_DUMP_CAP) return bytesToHexDump(bytes)
  return `[${bytes.length} bytes — larger than ${EXPORT_HEX_DUMP_CAP}, showing as base64]\n${bytesToBase64(bytes)}`
}

// Renders one direction (request or response) of a flow for the .txt export. Reuses
// decodeHttpMessage's dechunk/decompress logic when the direction looks like HTTP; for
// non-HTTP/opaque traffic (e.g. a Noise-protocol handshake) it just dumps the raw bytes, so
// nothing is silently unexportable.
const renderExportSection = async (bytes, label) => {
  const lines = [`--- ${label} ---`]
  if (!bytes || bytes.length === 0) {
    lines.push('(no data captured)')
    return lines.join('\n')
  }
  if (looksLikeHttp(bytes)) {
    const decoded = await decodeHttpMessage(bytes)
    lines.push(decoded.headerText, '')
    if (decoded.kind === 'text' || decoded.kind === 'json') {
      lines.push(decoded.bodyText || '(empty body)')
    } else if (decoded.kind === 'empty') {
      lines.push('(empty body)')
    } else {
      // image/video/audio/pdf/binary — decoded.dataUrl already holds the post-decompression
      // body as base64; decode it back to bytes so the export reflects the real body, not the
      // still-compressed wire bytes.
      const b64 = decoded.dataUrl ? decoded.dataUrl.split(',')[1] : ''
      const bodyBytes = b64 ? base64ToBytes(b64) : new Uint8Array(0)
      lines.push(`[${decoded.contentType || 'unknown content-type'}, ${decoded.byteLength} bytes]`)
      lines.push(renderExportBody(bodyBytes))
    }
    if (decoded.note) lines.push('', `Note: ${decoded.note}`)
  } else {
    lines.push('(raw, non-HTTP)', renderExportBody(bytes))
  }
  return lines.join('\n')
}

// Exports the SPECIFIC transaction being viewed (or, for the non-HTTP fallback row, the whole
// connection — unchanged legacy behavior) rather than the whole connection's concatenated
// bytes, so a keep-alive flow with many requests exports just the one the user is looking at.
const exportFlowAsText = async (row) => {
  try {
    const flow = row.flow
    const reqBytes = bytesForRow(row, 'client->server')
    const resBytes = bytesForRow(row, 'server->client')
    const counts = rowByteCounts(row)

    const meta = [
      `Flow ID: ${flow.flow_id}`,
      row.isFallback ? null : `Transaction: ${row.transactionIndex + 1}`,
      `Time: ${formatTrafficTime(rowTimestamp(row))} (${formatDate(rowTimestamp(row) * 1000)})`,
      `User: ${usernameForFlow(flow.user_id)}`,
      `Protocol: ${flow.protocol}`,
      `Host: ${flow.host}:${flow.port}`,
      `SNI: ${flow.sni || '-'}`,
      `Duration: ${formatDuration(rowDurationSeconds(row))}`,
      `Bytes sent: ${formatBytes(counts.sent)}`,
      `Bytes received: ${formatBytes(counts.received)}`,
      `Status: ${rowStatusLabel(row)}`,
      flow.error ? `Error: ${flow.error}` : null
    ].filter((line) => line !== null).join('\n')

    const [reqSection, resSection] = await Promise.all([
      renderExportSection(reqBytes, 'REQUEST'),
      renderExportSection(resBytes, 'RESPONSE')
    ])

    const text = [meta, '', reqSection, '', resSection].join('\n')
    const suffix = row.isFallback ? '' : `-tx${row.transactionIndex + 1}`
    triggerDownload(new Blob([text], { type: 'text/plain' }), `flow-${flow.flow_id}${suffix}.txt`)
    toast.success(t('vpn.flowExported'))
  } catch (error) {
    console.error('Failed to export flow as text:', error)
    toast.error(t('vpn.exportFailed'))
  }
}

// Splits a header block's lines (after the request-line/status-line) into {name, value} pairs
// for the HAR "headers" array.
const parseHeaderLines = (headerText) => {
  const headers = []
  const lines = (headerText || '').split(/\r\n|\n/).slice(1)
  for (const line of lines) {
    const idx = line.indexOf(':')
    if (idx === -1) continue
    headers.push({ name: line.slice(0, idx).trim(), value: line.slice(idx + 1).trim() })
  }
  return headers
}

// Maps a decodeHttpMessage() result to a HAR postData/content body: text bodies stay as plain
// text, everything else (image/video/audio/pdf/binary) is carried as base64 — valid per the
// HAR 1.2 spec's `encoding: "base64"` on postData/content.
const buildHarBodyContent = (decoded) => {
  if (!decoded || decoded.kind === 'empty') {
    return { mimeType: (decoded && decoded.contentType) || 'application/octet-stream', size: 0, text: '' }
  }
  if (decoded.kind === 'text' || decoded.kind === 'json') {
    return {
      mimeType: decoded.contentType || (decoded.kind === 'json' ? 'application/json' : 'text/plain'),
      size: decoded.byteLength,
      text: decoded.bodyText
    }
  }
  const base64 = decoded.dataUrl ? decoded.dataUrl.split(',')[1] : ''
  return { mimeType: decoded.contentType || 'application/octet-stream', size: decoded.byteLength, text: base64, encoding: 'base64' }
}

// Builds a minimal but valid single-entry HAR 1.2 document for one flow. Not exhaustively
// spec-perfect (no cookies, no separate queryString array, headersSize/timings are estimates/
// unknowns per spec's -1/0 conventions) — just enough for Chrome DevTools' "Import HAR" or an
// online viewer to render request/response headers and bodies for sharing/diffing.
const exportFlowAsHar = async (row) => {
  try {
    if (!canExportHar(row)) {
      toast.error(t('vpn.exportHarUnavailable'))
      return
    }
    const flow = row.flow
    const reqBytes = bytesForRow(row, 'client->server')
    const resBytes = bytesForRow(row, 'server->client')
    const [reqDecoded, resDecoded] = await Promise.all([
      decodeHttpMessage(reqBytes),
      decodeHttpMessage(resBytes)
    ])
    if (!reqDecoded) {
      toast.error(t('vpn.exportHarUnavailable'))
      return
    }

    const info = transactionInfoForRow(row)
    const scheme = flow.protocol === 'tcp-plain' ? 'http' : 'https'
    const path = info.path && info.path !== '*' ? info.path : '/'
    const url = `${scheme}://${flow.host || flow.sni}${path}`

    const resStatusMatch = resDecoded ? firstLineText(resBytes).match(STATUS_LINE_RE) : null
    const reqBody = buildHarBodyContent(reqDecoded)
    const resBody = buildHarBodyContent(resDecoded)

    const har = {
      log: {
        version: '1.2',
        creator: { name: 'Redock DevStation Traffic Inspector', version: '1.0' },
        entries: [{
          startedDateTime: new Date(rowTimestamp(row) * 1000).toISOString(),
          time: rowDurationSeconds(row) * 1000,
          request: {
            method: info.method || 'GET',
            url,
            httpVersion: 'HTTP/1.1',
            cookies: [],
            headers: parseHeaderLines(reqDecoded.headerText),
            queryString: [],
            headersSize: -1,
            bodySize: reqDecoded.byteLength || 0,
            postData: reqDecoded.byteLength > 0 ? {
              mimeType: reqBody.mimeType,
              text: reqBody.text,
              ...(reqBody.encoding ? { encoding: reqBody.encoding } : {})
            } : undefined
          },
          response: {
            status: resStatusMatch ? parseInt(resStatusMatch[1], 10) : 0,
            statusText: resStatusMatch ? (resStatusMatch[2] || '') : '',
            httpVersion: 'HTTP/1.1',
            cookies: [],
            headers: resDecoded ? parseHeaderLines(resDecoded.headerText) : [],
            content: resBody,
            redirectURL: '',
            headersSize: -1,
            bodySize: resDecoded ? (resDecoded.byteLength || 0) : 0
          },
          cache: {},
          timings: { send: 0, wait: 0, receive: 0 }
        }]
      }
    }

    const suffix = row.isFallback ? '' : `-tx${row.transactionIndex + 1}`
    triggerDownload(new Blob([JSON.stringify(har, null, 2)], { type: 'application/json' }), `flow-${flow.flow_id}${suffix}.har`)
    toast.success(t('vpn.flowExported'))
  } catch (error) {
    console.error('Failed to export flow as HAR:', error)
    toast.error(t('vpn.exportFailed'))
  }
}

// --- Resend (replay) ---

// Only real, parseable HTTP requests can be resent — the non-HTTP fallback row (e.g. opaque
// Noise-protocol/WhatsApp traffic) has no coherent request to replay.
const canResendRow = (row) => !row.isFallback && !!(row.request && requestLineFromHeaderText(row.request.headerText))

// Builds the full absolute URL the replay endpoint needs from the row's flow (host/port/protocol)
// and the captured request's path. Scheme is 'http' only for tcp-plain flows; both tcp-tls and
// quic (HTTP/3) replay over https. The port is omitted when it's the scheme's default, purely for
// a cleaner pre-filled URL — it's still fully editable before sending.
const buildResendUrl = (row) => {
  const flow = row.flow
  const scheme = flow.protocol === 'tcp-plain' ? 'http' : 'https'
  const host = flow.host || flow.sni || ''
  const port = Number(flow.port)
  const defaultPort = scheme === 'http' ? 80 : 443
  const hostPort = port && port !== defaultPort ? `${host}:${port}` : host
  const reqLineMatch = requestLineFromHeaderText(row.request.headerText)
  const path = reqLineMatch && reqLineMatch[2] && reqLineMatch[2] !== '*' ? reqLineMatch[2] : '/'
  return `${scheme}://${hostPort}${path}`
}

// Opens the Resend modal pre-filled from `row`'s captured request: method/URL derived from the
// flow + request-line, headers parsed from the header block (minus the request-line itself, via
// parseHeaderLines — already used by the HAR export), and body pre-filled from
// decodeHttpMessage's own bodyText so JSON gets the exact same pretty-print the Decoded view
// already shows. A binary request body (rare) is kept verbatim rather than shoved into a
// textarea — see resendBodyBinary above.
const openResendModal = async (row) => {
  const reqLineMatch = requestLineFromHeaderText(row.request.headerText)
  resendRow.value = row
  resendResult.value = null
  const reqBytes = bytesForRow(row, 'client->server')
  const decoded = await decodeHttpMessage(reqBytes)
  const headersText = parseHeaderLines(row.request.headerText)
    .map((h) => `${h.name}: ${h.value}`)
    .join('\n')

  resendBodyBinary.value = !!decoded && ['image', 'video', 'audio', 'pdf', 'binary'].includes(decoded.kind)
  resendBinaryBodyBase64.value = resendBodyBinary.value && decoded.dataUrl ? decoded.dataUrl.split(',')[1] : ''

  resendRequest.value = {
    method: reqLineMatch ? reqLineMatch[1] : 'GET',
    url: buildResendUrl(row),
    headersText,
    body: resendBodyBinary.value
      ? t('vpn.resendBinaryBodyNote', { bytes: formatBytes(decoded ? decoded.byteLength : 0) })
      : ((decoded && decoded.bodyText) || '')
  }
  isResendModalActive.value = true
}

const closeResendModal = () => {
  isResendModalActive.value = false
}

// Parses the edited headers textarea (one "Name: value" per line) back into the flat object the
// replay endpoint expects. Blank lines and lines without a ':' are silently skipped.
const parseResendHeadersText = (text) => {
  const headers = {}
  for (const rawLine of (text || '').split(/\r?\n/)) {
    const line = rawLine.trim()
    if (!line) continue
    const idx = line.indexOf(':')
    if (idx === -1) continue
    const name = line.slice(0, idx).trim()
    if (!name) continue
    headers[name] = line.slice(idx + 1).trim()
  }
  return headers
}

// Sends the (possibly user-edited) request through POST /v1/vpn/flows/replay, then reconstructs a
// synthetic raw HTTP response from the JSON result and feeds it straight through
// decodeHttpMessage — reusing 100% of the existing JSON-pretty-print/image/video/pdf/binary
// rendering already used by the Decoded view, with zero new decode logic. Renders the result (or
// an error) inline in the modal rather than closing it, per how a resend/replay tool should work.
const sendResendRequest = async () => {
  if (!resendRow.value) return
  resendLoading.value = true
  resendResult.value = null
  try {
    const payload = {
      method: (resendRequest.value.method || 'GET').trim().toUpperCase() || 'GET',
      url: resendRequest.value.url,
      headers: parseResendHeadersText(resendRequest.value.headersText)
    }
    if (resendBodyBinary.value) {
      if (resendBinaryBodyBase64.value) {
        payload.body = resendBinaryBodyBase64.value
        payload.body_base64 = true
      }
    } else if (resendRequest.value.body) {
      payload.body = resendRequest.value.body
      payload.body_base64 = false
    }

    const response = await ApiService.post('/v1/vpn/flows/replay', payload)
    const envelope = response.data
    if (!envelope || envelope.error) {
      const msg = (envelope && envelope.msg) || t('vpn.resendFailed')
      resendResult.value = { error: msg }
      toast.error(msg)
      return
    }

    const data = envelope.data || {}
    if (data.error) {
      resendResult.value = { error: data.error }
      toast.error(t('vpn.resendNetworkError', { error: data.error }))
      return
    }

    const bodyBytes = data.body_base64 ? base64ToBytes(data.body_base64) : new Uint8Array(0)
    const headerLines = Object.entries(data.headers || {}).map(([name, value]) => `${name}: ${value}\r\n`).join('')
    const responseText = `HTTP/1.1 ${data.status} ${data.status_text || ''}\r\n${headerLines}Content-Length: ${bodyBytes.length}\r\n\r\n`
    const syntheticBytes = concatBytes([new TextEncoder().encode(responseText), bodyBytes])
    const decoded = await decodeHttpMessage(syntheticBytes)

    resendResult.value = {
      status: data.status,
      statusText: data.status_text || '',
      durationMs: data.duration_ms,
      decoded
    }
    toast.success(t('vpn.resendSuccess'))
  } catch (error) {
    console.error('Failed to resend request:', error)
    const msg = error?.response?.data?.msg || t('vpn.resendFailed')
    resendResult.value = { error: msg }
    toast.error(msg)
  } finally {
    resendLoading.value = false
  }
}

const updateDecodedView = async () => {
  if (payloadViewMode.value !== 'decoded' || !expandedRow.value) {
    decodedRequest.value = null
    decodedResponse.value = null
    return
  }
  const rowKeyAtStart = expandedFlowId.value
  const [req, res] = await Promise.all([
    decodeHttpMessage(expandedRowRequestBytes.value),
    decodeHttpMessage(expandedRowResponseBytes.value)
  ])
  // Discard the result if the user switched row/view while decoding was in flight.
  if (expandedFlowId.value !== rowKeyAtStart || payloadViewMode.value !== 'decoded') return
  decodedRequest.value = req
  decodedResponse.value = res
}

watch([expandedFlowId, payloadViewMode], () => {
  updateDecodedView()
})

const formatBytes = (bytes) => {
  if (!bytes || bytes === 0 || isNaN(bytes)) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i]
}

const formatDate = (date) => {
  if (!date || date === null || date === undefined) return 'Never'
  try {
    return new Date(date).toLocaleString()
  } catch (e) {
    return 'Invalid Date'
  }
}

const formatDuration = (seconds) => {
  if (!seconds) return '0s'
  const hours = Math.floor(seconds / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  const secs = seconds % 60
  if (hours > 0) return `${hours}h ${minutes}m ${secs}s`
  if (minutes > 0) return `${minutes}m ${secs}s`
  return `${secs}s`
}

const resetServerForm = () => {
  newServer.value = {
    name: '',
    address: '10.0.0.1/24',
    endpoint: '',
    dns: '1.1.1.1,8.8.8.8',
    listen_port: 51820,
    mtu: 1420,
    persistent_keepalive: 25,
    enabled: true,
    description: ''
  }
}

const resetUserForm = () => {
  newUser.value = {
    server_id: selectedServer.value || null,
    username: '',
    email: '',
    full_name: '',
    allowed_ips: '0.0.0.0/0',
    dns: '',
    quota: 0,
    notes: ''
  }
}

const openEditServer = (server) => {
  editingServer.value = { ...server }
  isEditServerModalActive.value = true
}

const openEditUser = (user) => {
  editingUser.value = { ...user }
  isEditUserModalActive.value = true
}

const openDeleteModal = (type, item) => {
  deleteTarget.value = { type, item }
  isDeleteModalActive.value = true
}

const confirmDelete = () => {
  if (deleteTarget.value.type === 'server') {
    deleteServer()
  } else if (deleteTarget.value.type === 'user') {
    deleteUser()
  }
}

const selectServer = (serverId) => {
  selectedServer.value = serverId
  fetchUsers(serverId)
}

// Lifecycle
onMounted(async () => {
  await fetchStatistics()
  await fetchServers()
  await fetchUsers()
  await fetchConnections()
  await fetchBandwidthStats()
  await fetchConnectionStats()
  
  // Auto-refresh every 30 seconds
  refreshInterval = setInterval(async () => {
    await fetchStatistics()
    await fetchConnections()
    await fetchBandwidthStats()
    await fetchConnectionStats()
  }, 30000)
})

onUnmounted(() => {
  if (refreshInterval) {
    clearInterval(refreshInterval)
  }
  disconnectTrafficSocket()
  if (trafficTickInterval) {
    clearInterval(trafficTickInterval)
    trafficTickInterval = null
  }
})

// Only keep the traffic websocket (and the once/second duration tick) running while the
// Live Traffic tab is active.
watch(activeTab, (tab) => {
  if (tab === 'traffic') {
    fetchFlows()
    fetchTrafficLogs()
    connectTrafficSocket()
    if (!trafficTickInterval) {
      trafficNowTick.value = Date.now()
      trafficTickInterval = setInterval(() => { trafficNowTick.value = Date.now() }, 1000)
    }
  } else {
    disconnectTrafficSocket()
    if (trafficTickInterval) {
      clearInterval(trafficTickInterval)
      trafficTickInterval = null
    }
  }
})
</script>

<template>
  <div>
    <!-- Header -->
    <SectionTitleLineWithButton :icon="mdiServerNetwork" :title="t('vpn.title')" main>
      <BaseButton
        :icon="mdiRefresh"
        :label="t('common.refresh')"
        color="info"
        @click="fetchStatistics(); fetchServers(); fetchUsers(); fetchConnections()"
      />
    </SectionTitleLineWithButton>

    <!-- Tabs (responsive: horizontal scroll on small screens) -->
    <div class="mb-6 overflow-x-auto pb-px -mx-1 px-1">
      <div class="flex flex-nowrap gap-2 border-b border-slate-200 dark:border-slate-700">
        <button
          v-for="tab in ['overview', 'servers', 'users', 'statistics', 'traffic']"
          :key="tab"
          :class="[
            'shrink-0 whitespace-nowrap px-4 py-2 font-medium text-sm transition-colors',
            activeTab === tab
              ? 'border-b-2 border-blue-500 text-blue-600 dark:text-blue-400'
              : 'text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200'
          ]"
          @click="activeTab = tab"
        >
          {{ t('vpn.tabs.' + tab) }}
        </button>
      </div>
    </div>

    <!-- Overview Tab -->
    <div v-if="activeTab === 'overview'" class="space-y-6">
      <!-- Statistics Cards -->
      <div class="grid grid-cols-1 md:grid-cols-3 gap-6">
        <CardBox>
          <div class="flex items-center justify-between">
            <div>
              <p class="text-sm text-slate-500 dark:text-slate-400">{{ t('vpn.totalServers') }}</p>
              <p class="text-3xl font-bold text-slate-900 dark:text-slate-100">
                {{ statistics.total_servers || 0 }}
              </p>
            </div>
            <BaseIcon :path="mdiServer" size="48" class="text-blue-500" />
          </div>
        </CardBox>

        <CardBox>
          <div class="flex items-center justify-between">
            <div>
              <p class="text-sm text-slate-500 dark:text-slate-400">{{ t('vpn.totalUsers') }}</p>
              <p class="text-3xl font-bold text-slate-900 dark:text-slate-100">
                {{ statistics.total_users || 0 }}
              </p>
            </div>
            <BaseIcon :path="mdiAccount" size="48" class="text-green-500" />
          </div>
        </CardBox>

        <CardBox>
          <div class="flex items-center justify-between">
            <div>
              <p class="text-sm text-slate-500 dark:text-slate-400">{{ t('vpn.activeConnections') }}</p>
              <p class="text-3xl font-bold text-slate-900 dark:text-slate-100">
                {{ statistics.active_connections || 0 }}
              </p>
            </div>
            <BaseIcon :path="mdiNetwork" size="48" class="text-purple-500" />
          </div>
        </CardBox>
      </div>

      <!-- Active Connections -->
      <CardBox>
        <SectionTitleLineWithButton :icon="mdiNetwork" :title="t('vpn.activeConnections')">
          <BaseButton
            :icon="mdiRefresh"
            :label="t('common.refresh')"
            color="info"
            small
            @click="fetchConnections()"
          />
        </SectionTitleLineWithButton>

        <div v-if="connections.length === 0" class="text-center py-8 text-slate-500">
          {{ t('vpn.noActiveConnections') }}
        </div>

        <div v-else class="overflow-x-auto">
          <table class="w-full">
            <thead>
              <tr class="border-b border-slate-200 dark:border-slate-700">
                <th class="px-4 py-3 text-left text-xs font-semibold text-slate-500 dark:text-slate-400">{{ t('vpn.user') }}</th>
                <th class="px-4 py-3 text-left text-xs font-semibold text-slate-500 dark:text-slate-400">{{ t('vpn.remoteIp') }}</th>
                <th class="px-4 py-3 text-left text-xs font-semibold text-slate-500 dark:text-slate-400">{{ t('vpn.received') }}</th>
                <th class="px-4 py-3 text-left text-xs font-semibold text-slate-500 dark:text-slate-400">{{ t('vpn.sent') }}</th>
                <th class="px-4 py-3 text-left text-xs font-semibold text-slate-500 dark:text-slate-400">{{ t('vpn.lastHandshake') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="conn in connections"
                :key="conn.id"
                class="border-b border-slate-100 dark:border-slate-800 hover:bg-slate-50 dark:hover:bg-slate-800/50"
              >
                <td class="px-4 py-3 text-sm">{{ conn.user_id }}</td>
                <td class="px-4 py-3 text-sm font-mono">{{ conn.remote_ip || 'N/A' }}</td>
                <td class="px-4 py-3 text-sm">{{ formatBytes(conn.bytes_received) }}</td>
                <td class="px-4 py-3 text-sm">{{ formatBytes(conn.bytes_sent) }}</td>
                <td class="px-4 py-3 text-sm">{{ formatDate(conn.last_handshake) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </CardBox>
    </div>

    <!-- Servers Tab -->
    <div v-if="activeTab === 'servers'" class="space-y-6">
      <CardBox>
        <SectionTitleLineWithButton :icon="mdiServer" :title="t('vpn.vpnServers')">
          <BaseButton
            :icon="mdiPlus"
            :label="t('vpn.addServer')"
            color="info"
            @click="isAddServerModalActive = true"
          />
        </SectionTitleLineWithButton>

        <div v-if="servers.length === 0" class="text-center py-8 text-slate-500">
          {{ t('vpn.noServers') }}
        </div>

        <div v-else class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 mt-6">
          <div
            v-for="server in servers"
            :key="server.id"
            class="p-4 bg-slate-50 dark:bg-slate-800/50 rounded-xl border border-slate-200 dark:border-slate-700"
          >
            <div class="flex items-start justify-between mb-3">
              <div>
                <h3 class="font-semibold text-lg">{{ server.name }}</h3>
                <p class="text-xs text-slate-500 mt-1">{{ server.interface }}</p>
              </div>
              <span
                v-if="server.running"
                class="px-2 py-1 text-xs rounded bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300"
              >
                {{ t('common.running') }}
              </span>
              <span
                v-else
                class="px-2 py-1 text-xs rounded bg-gray-100 text-gray-700 dark:bg-gray-900/30 dark:text-gray-300"
              >
                {{ t('common.stopped') }}
              </span>
            </div>

            <div class="space-y-2 text-sm">
              <div class="flex justify-between">
                <span class="text-slate-500">{{ t('vpn.address') }}:</span>
                <span class="font-mono">{{ server.address }}</span>
              </div>
              <div class="flex justify-between">
                <span class="text-slate-500">{{ t('vpn.port') }}:</span>
                <span>{{ server.listen_port }}</span>
              </div>
              <div v-if="server.endpoint" class="flex justify-between">
                <span class="text-slate-500">{{ t('vpn.endpoint') }}:</span>
                <span class="font-mono text-xs">{{ server.endpoint }}</span>
              </div>
            </div>

            <div class="flex space-x-2 mt-4">
              <BaseButton
                :icon="server.running ? mdiStop : mdiPlay"
                :label="server.running ? t('common.stop') : t('common.start')"
                :color="server.running ? 'danger' : 'success'"
                small
                @click="server.running ? stopServer(server.id) : startServer(server.id)"
              />
              <BaseButton
                :icon="mdiPencil"
                :label="t('common.edit')"
                color="info"
                small
                @click="openEditServer(server)"
              />
              <BaseButton
                :icon="mdiDelete"
                :label="t('common.delete')"
                color="danger"
                small
                @click="openDeleteModal('server', server)"
              />
            </div>
          </div>
        </div>
      </CardBox>
    </div>

    <!-- Users Tab -->
    <div v-if="activeTab === 'users'" class="space-y-6">
      <CardBox>
        <SectionTitleLineWithButton :icon="mdiAccount" :title="t('vpn.vpnUsers')">
          <div class="flex space-x-2">
          <BaseButton
            :icon="mdiAccountPlus"
            :label="t('vpn.addUser')"
            color="info"
            @click="isAddUserModalActive = true"
          />
            <BaseButton
              :icon="mdiRefresh"
              :label="t('common.refresh')"
              color="info"
              @click="fetchUsers()"
            />
          </div>
        </SectionTitleLineWithButton>

        <!-- Server Selection -->
        <div v-if="servers.length > 0" class="mb-4">
          <FormField :label="t('vpn.filterByServer')">
            <FormControl
              v-model="selectedServer"
              :options="[{ value: null, label: t('vpn.allServers') }, ...servers.map(s => ({ value: s.id, label: s.name }))]"
              :placeholder="t('vpn.allServers')"
            />
          </FormField>
        </div>

        <div v-if="users.length === 0" class="text-center py-8 text-slate-500">
          {{ t('vpn.noUsers') }}
        </div>

        <div v-else class="overflow-x-auto mt-6">
          <table class="w-full">
            <thead>
              <tr class="border-b border-slate-200 dark:border-slate-700">
                <th class="px-4 py-3 text-left text-xs font-semibold text-slate-500 dark:text-slate-400">{{ t('vpn.username') }}</th>
                <th class="px-4 py-3 text-left text-xs font-semibold text-slate-500 dark:text-slate-400">{{ t('common.email') }}</th>
                <th class="px-4 py-3 text-left text-xs font-semibold text-slate-500 dark:text-slate-400">{{ t('vpn.address') }}</th>
                <th class="px-4 py-3 text-left text-xs font-semibold text-slate-500 dark:text-slate-400">{{ t('vpn.bandwidth') }}</th>
                <th class="px-4 py-3 text-left text-xs font-semibold text-slate-500 dark:text-slate-400">{{ t('vpn.lastConnected') }}</th>
                <th class="px-4 py-3 text-left text-xs font-semibold text-slate-500 dark:text-slate-400">{{ t('vpn.inspection') }}</th>
                <th class="px-4 py-3 text-left text-xs font-semibold text-slate-500 dark:text-slate-400">{{ t('common.actions') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="user in users"
                :key="user.id"
                class="border-b border-slate-100 dark:border-slate-800 hover:bg-slate-50 dark:hover:bg-slate-800/50"
              >
                <td class="px-4 py-3 text-sm font-semibold">{{ user.username }}</td>
                <td class="px-4 py-3 text-sm">{{ user.email || '-' }}</td>
                <td class="px-4 py-3 text-sm font-mono">{{ user.address }}</td>
                <td class="px-4 py-3 text-sm">
                  {{ formatBytes((user.total_bytes_received || 0) + (user.total_bytes_sent || 0)) }}
                </td>
                <td class="px-4 py-3 text-sm">{{ formatDate(user.last_connected_at) }}</td>
                <td class="px-4 py-3">
                  <label class="inline-flex items-center cursor-pointer" :title="user.inspection_enabled ? t('vpn.disableInspection') : t('vpn.enableInspection')">
                    <input
                      type="checkbox"
                      class="sr-only peer"
                      :checked="user.inspection_enabled"
                      @change="toggleInspection(user)"
                    />
                    <div class="w-10 h-5 bg-slate-300 dark:bg-slate-600 rounded-full peer peer-checked:bg-emerald-500 transition-colors relative">
                      <div class="absolute top-0.5 left-0.5 bg-white w-4 h-4 rounded-full transition-transform peer-checked:translate-x-5"></div>
                    </div>
                  </label>
                </td>
                <td class="px-4 py-3">
                  <div class="flex space-x-2">
                    <BaseButton
                      :icon="mdiDownload"
                      :label="t('vpn.config')"
                      color="info"
                      small
                      @click="downloadConfig(user.id)"
                    />
                    <BaseButton
                      :icon="mdiQrcode"
                      :label="t('vpn.qr')"
                      color="info"
                      small
                      @click="getQRCode(user.id)"
                    />
                    <BaseButton
                      :icon="mdiPencil"
                      :label="t('common.edit')"
                      color="info"
                      small
                      @click="openEditUser(user)"
                    />
                    <BaseButton
                      :icon="mdiDelete"
                      :label="t('common.delete')"
                      color="danger"
                      small
                      @click="openDeleteModal('user', user)"
                    />
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </CardBox>
    </div>

    <!-- Statistics Tab -->
    <div v-if="activeTab === 'statistics'" class="space-y-6">
      <CardBox>
        <SectionTitleLineWithButton :icon="mdiChartLine" :title="t('vpn.statistics')">
          <BaseButton
            :icon="mdiRefresh"
            :label="t('common.refresh')"
            color="info"
            @click="fetchStatistics(); fetchBandwidthStats(); fetchConnectionStats()"
          />
        </SectionTitleLineWithButton>

        <!-- Bandwidth Statistics -->
        <div class="mt-6 space-y-6">
          <div>
            <h3 class="text-lg font-semibold mb-4">{{ t('vpn.bandwidthStatistics') }}</h3>
            <div class="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
              <div class="p-4 bg-slate-50 dark:bg-slate-800/50 rounded-lg">
                <p class="text-sm text-slate-500 dark:text-slate-400">{{ t('vpn.totalReceived') }}</p>
                <p class="text-2xl font-bold">{{ formatBytes(bandwidthStats.total_received || 0) }}</p>
              </div>
              <div class="p-4 bg-slate-50 dark:bg-slate-800/50 rounded-lg">
                <p class="text-sm text-slate-500 dark:text-slate-400">{{ t('vpn.totalSent') }}</p>
                <p class="text-2xl font-bold">{{ formatBytes(bandwidthStats.total_sent || 0) }}</p>
              </div>
              <div class="p-4 bg-slate-50 dark:bg-slate-800/50 rounded-lg">
                <p class="text-sm text-slate-500 dark:text-slate-400">{{ t('vpn.totalBandwidth') }}</p>
                <p class="text-2xl font-bold">{{ formatBytes(bandwidthStats.total_bandwidth || 0) }}</p>
              </div>
            </div>

            <div v-if="bandwidthStats.top_users && bandwidthStats.top_users.length > 0">
              <h4 class="text-md font-semibold mb-3">{{ t('vpn.top10Users') }}</h4>
              <div class="overflow-x-auto">
                <table class="w-full">
                  <thead>
                    <tr class="border-b border-slate-200 dark:border-slate-700">
                      <th class="px-4 py-2 text-left text-xs font-semibold text-slate-500 dark:text-slate-400">{{ t('vpn.username') }}</th>
                      <th class="px-4 py-2 text-left text-xs font-semibold text-slate-500 dark:text-slate-400">{{ t('vpn.received') }}</th>
                      <th class="px-4 py-2 text-left text-xs font-semibold text-slate-500 dark:text-slate-400">{{ t('vpn.sent') }}</th>
                      <th class="px-4 py-2 text-left text-xs font-semibold text-slate-500 dark:text-slate-400">{{ t('vpn.total') }}</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr
                      v-for="(user, index) in bandwidthStats.top_users"
                      :key="index"
                      class="border-b border-slate-100 dark:border-slate-800"
                    >
                      <td class="px-4 py-2 text-sm font-semibold">{{ user.username }}</td>
                      <td class="px-4 py-2 text-sm">{{ formatBytes(user.received) }}</td>
                      <td class="px-4 py-2 text-sm">{{ formatBytes(user.sent) }}</td>
                      <td class="px-4 py-2 text-sm font-semibold">{{ formatBytes(user.total) }}</td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>
          </div>

          <!-- Connection Statistics -->
          <div class="pt-6 border-t border-slate-200 dark:border-slate-700">
            <h3 class="text-lg font-semibold mb-4">{{ t('vpn.connectionStatistics') }}</h3>
            <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
              <div class="p-4 bg-slate-50 dark:bg-slate-800/50 rounded-lg">
                <p class="text-sm text-slate-500 dark:text-slate-400">{{ t('vpn.totalConnections') }}</p>
                <p class="text-2xl font-bold">{{ connectionStats.total_connections || 0 }}</p>
              </div>
              <div class="p-4 bg-slate-50 dark:bg-slate-800/50 rounded-lg">
                <p class="text-sm text-slate-500 dark:text-slate-400">{{ t('vpn.totalDuration') }}</p>
                <p class="text-2xl font-bold">{{ formatDuration(connectionStats.total_duration || 0) }}</p>
              </div>
              <div class="p-4 bg-slate-50 dark:bg-slate-800/50 rounded-lg">
                <p class="text-sm text-slate-500 dark:text-slate-400">{{ t('vpn.avgDuration') }}</p>
                <p class="text-2xl font-bold">{{ formatDuration(Math.round(connectionStats.avg_duration || 0)) }}</p>
              </div>
              <div class="p-4 bg-slate-50 dark:bg-slate-800/50 rounded-lg">
                <p class="text-sm text-slate-500 dark:text-slate-400">{{ t('vpn.activeUsers24h') }}</p>
                <p class="text-2xl font-bold">{{ connectionStats.active_users_24h || 0 }}</p>
              </div>
            </div>
          </div>
        </div>
      </CardBox>
    </div>

    <!-- Live Traffic Tab -->
    <div v-if="activeTab === 'traffic'" class="space-y-6">
      <CardBox>
        <SectionTitleLineWithButton :icon="mdiShieldLock" :title="t('vpn.liveTraffic')">
          <BaseButton
            :icon="mdiDownload"
            :label="t('vpn.downloadCa')"
            color="info"
            @click="downloadCA"
          />
        </SectionTitleLineWithButton>

        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">
          {{ t('vpn.caInstallNote') }}
        </p>

        <!-- Quick-filter bar: purely client-side over the already-accumulated `flows` list -->
        <div v-if="flows.length > 0" class="mt-4 pt-4 border-t border-slate-200 dark:border-slate-700 flex flex-wrap gap-3 items-center">
          <div class="flex-1 min-w-[220px]">
            <FormControl
              v-model="trafficSearch"
              :placeholder="t('vpn.trafficSearchPlaceholder')"
              :icon="mdiMagnify"
            />
          </div>

          <div class="flex flex-wrap items-center gap-1.5">
            <span class="text-xs text-slate-500 dark:text-slate-400">{{ t('vpn.protocol') }}:</span>
            <button
              v-for="proto in TRAFFIC_PROTOCOLS"
              :key="proto"
              type="button"
              class="px-2.5 py-1 text-xs rounded-full font-mono border transition-colors"
              :class="trafficProtocolFilter.includes(proto)
                ? 'bg-blue-500 border-blue-500 text-white'
                : 'bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700'"
              @click="toggleTrafficProtocolFilter(proto)"
            >
              {{ proto }}
            </button>
          </div>

          <select
            v-model="trafficUserFilter"
            class="h-12 min-w-[140px] px-3 py-2 text-sm rounded border border-gray-700 bg-white dark:bg-slate-800 text-slate-700 dark:text-slate-200 focus:ring focus:outline-none"
          >
            <option :value="null">{{ t('vpn.allUsers') }}</option>
            <option v-for="user in users" :key="user.id" :value="user.id">{{ user.username }}</option>
          </select>

          <div class="inline-flex rounded-lg overflow-hidden border border-slate-200 dark:border-slate-700 text-sm">
            <button
              v-for="opt in [
                { value: 'all',    label: t('vpn.all') },
                { value: 'active', label: t('vpn.flowActive') },
                { value: 'closed', label: t('vpn.flowClosed') }
              ]"
              :key="opt.value"
              type="button"
              class="px-3 py-1.5 transition-colors"
              :class="trafficStatusFilter === opt.value
                ? 'bg-blue-500 text-white'
                : 'bg-white dark:bg-slate-800 text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700'"
              @click="trafficStatusFilter = opt.value"
            >
              {{ opt.label }}
            </button>
          </div>

          <span class="text-xs text-slate-500 dark:text-slate-400">
            {{ t('vpn.flowsMatchingCount', { count: filteredFlows.length, total: flows.length }) }}
          </span>

          <BaseButton
            v-if="hasActiveTrafficFilters"
            :icon="mdiFilterOff"
            :label="t('vpn.clearFilters')"
            color="lightDark"
            small
            outline
            @click="clearTrafficFilters"
          />
        </div>

        <div v-if="flows.length === 0" class="text-center py-8 text-slate-500 mt-4">
          {{ t('vpn.noFlows') }}
        </div>

        <div v-else-if="filteredFlows.length === 0" class="text-center py-8 text-slate-500 mt-4">
          {{ t('vpn.noFlowsMatch') }}
        </div>

        <div v-else class="overflow-x-auto mt-4">
          <table class="w-full">
            <thead>
              <tr class="border-b border-slate-200 dark:border-slate-700">
                <th
                  class="px-4 py-3 text-left text-xs font-semibold text-slate-500 dark:text-slate-400 cursor-pointer select-none hover:text-slate-700 dark:hover:text-slate-200"
                  :title="t('vpn.clickToSort')"
                  @click="setTrafficSort('lastSeen')"
                >
                  <span class="inline-flex items-center gap-1">
                    {{ t('vpn.time') }}
                    <BaseIcon
                      :path="trafficSortKey === 'lastSeen' ? (trafficSortDir === 'asc' ? mdiSortAscending : mdiSortDescending) : mdiSort"
                      size="14"
                      :class="trafficSortKey === 'lastSeen' ? 'text-blue-500' : 'text-slate-300 dark:text-slate-600'"
                    />
                  </span>
                </th>
                <th class="px-4 py-3 text-left text-xs font-semibold text-slate-500 dark:text-slate-400">{{ t('vpn.user') }}</th>
                <th class="px-4 py-3 text-left text-xs font-semibold text-slate-500 dark:text-slate-400">{{ t('vpn.protocol') }}</th>
                <th class="px-4 py-3 text-left text-xs font-semibold text-slate-500 dark:text-slate-400">{{ t('vpn.info') }}</th>
                <th class="px-4 py-3 text-left text-xs font-semibold text-slate-500 dark:text-slate-400">{{ t('vpn.hostPort') }}</th>
                <th class="px-4 py-3 text-left text-xs font-semibold text-slate-500 dark:text-slate-400">{{ t('vpn.sni') }}</th>
                <th class="hidden xl:table-cell px-4 py-3 text-left text-xs font-semibold text-slate-500 dark:text-slate-400">{{ t('vpn.contentType') }}</th>
                <th class="hidden xl:table-cell px-4 py-3 text-left text-xs font-semibold text-slate-500 dark:text-slate-400">{{ t('vpn.userAgent') }}</th>
                <th
                  class="px-4 py-3 text-left text-xs font-semibold text-slate-500 dark:text-slate-400 cursor-pointer select-none hover:text-slate-700 dark:hover:text-slate-200"
                  :title="t('vpn.clickToSort')"
                  @click="setTrafficSort('bytes')"
                >
                  <span class="inline-flex items-center gap-1">
                    {{ t('vpn.sentReceived') }}
                    <BaseIcon
                      :path="trafficSortKey === 'bytes' ? (trafficSortDir === 'asc' ? mdiSortAscending : mdiSortDescending) : mdiSort"
                      size="14"
                      :class="trafficSortKey === 'bytes' ? 'text-blue-500' : 'text-slate-300 dark:text-slate-600'"
                    />
                  </span>
                </th>
                <th class="px-4 py-3 text-left text-xs font-semibold text-slate-500 dark:text-slate-400">
                  <span class="inline-flex items-center gap-1">
                    <BaseIcon :path="mdiTimer" size="14" class="text-slate-300 dark:text-slate-600" />
                    {{ t('vpn.duration') }}
                  </span>
                </th>
                <th class="px-4 py-3 text-left text-xs font-semibold text-slate-500 dark:text-slate-400">{{ t('common.status') }}</th>
                <th class="px-4 py-3 text-left text-xs font-semibold text-slate-500 dark:text-slate-400"></th>
                <th class="px-4 py-3 text-left text-xs font-semibold text-slate-500 dark:text-slate-400"></th>
              </tr>
            </thead>
            <tbody>
              <template v-for="row in flowTransactionRows" :key="rowKey(row)">
                <tr
                  class="border-b border-slate-100 dark:border-slate-800 hover:bg-slate-50 dark:hover:bg-slate-800/50 cursor-pointer"
                  @click="toggleFlow(row)"
                >
                  <td class="px-4 py-3 text-sm">{{ formatTrafficTime(rowTimestamp(row)) }}</td>
                  <td class="px-4 py-3 text-sm">{{ usernameForFlow(row.flow.user_id) }}</td>
                  <td class="px-4 py-3 text-sm">
                    <span class="px-2 py-1 text-xs rounded bg-slate-100 text-slate-700 dark:bg-slate-700 dark:text-slate-300 font-mono">
                      {{ row.flow.protocol }}
                    </span>
                  </td>
                  <td class="px-4 py-3 text-sm font-mono max-w-xs truncate" :title="transactionInfoForRow(row).info">
                    {{ transactionInfoForRow(row).info || '-' }}
                  </td>
                  <td class="px-4 py-3 text-sm font-mono">{{ row.flow.host }}:{{ row.flow.port }}</td>
                  <td class="px-4 py-3 text-sm font-mono">{{ row.flow.sni || '-' }}</td>
                  <td class="hidden xl:table-cell px-4 py-3 text-sm max-w-[10rem] truncate" :title="transactionInfoForRow(row).contentType">
                    {{ transactionInfoForRow(row).contentType || '-' }}
                  </td>
                  <td class="hidden xl:table-cell px-4 py-3 text-sm max-w-[12rem] truncate" :title="transactionInfoForRow(row).userAgent">
                    {{ transactionInfoForRow(row).userAgent || '-' }}
                  </td>
                  <td class="px-4 py-3 text-xs leading-tight whitespace-nowrap">
                    <div class="flex flex-col">
                      <span class="text-emerald-600 dark:text-emerald-400" :title="t('vpn.sent')">↑ {{ formatBytes(rowByteCounts(row).sent) }}</span>
                      <span class="text-sky-600 dark:text-sky-400" :title="t('vpn.received')">↓ {{ formatBytes(rowByteCounts(row).received) }}</span>
                    </div>
                  </td>
                  <td class="px-4 py-3 text-sm whitespace-nowrap">{{ formatDuration(rowDurationSeconds(row)) }}</td>
                  <td class="px-4 py-3 text-sm">
                    <span
                      v-if="rowStatusLabel(row) === 'closed'"
                      class="px-2 py-1 text-xs rounded bg-gray-100 text-gray-700 dark:bg-gray-900/30 dark:text-gray-300"
                    >
                      {{ t('vpn.flowClosed') }}
                    </span>
                    <span
                      v-else-if="rowStatusLabel(row) === 'pending'"
                      class="px-2 py-1 text-xs rounded bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300"
                      :title="t('vpn.rowPendingHint')"
                    >
                      {{ t('vpn.flowPending') }}
                    </span>
                    <span
                      v-else
                      class="px-2 py-1 text-xs rounded bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300"
                    >
                      {{ t('vpn.flowActive') }}
                    </span>
                  </td>
                  <td class="px-4 py-3 text-sm">
                    <div class="flex items-center gap-1">
                      <BaseButton
                        :icon="mdiFileDocumentOutline"
                        small
                        color="lightDark"
                        :title="t('vpn.exportTxt')"
                        @click.stop="exportFlowAsText(row)"
                      />
                      <BaseButton
                        v-if="canExportHar(row)"
                        :icon="mdiCodeJson"
                        small
                        color="lightDark"
                        :title="t('vpn.exportHar')"
                        @click.stop="exportFlowAsHar(row)"
                      />
                      <BaseButton
                        v-if="canResendRow(row)"
                        :icon="mdiReplay"
                        small
                        color="lightDark"
                        :title="t('vpn.resend')"
                        @click.stop="openResendModal(row)"
                      />
                    </div>
                  </td>
                  <td class="px-4 py-3 text-sm">
                    <BaseIcon :path="expandedFlowId === rowKey(row) ? mdiChevronUp : mdiChevronDown" size="18" />
                  </td>
                </tr>
                <tr v-if="expandedFlowId === rowKey(row)">
                  <td colspan="13" class="p-0">
                    <div class="border-t border-slate-200 dark:border-slate-700 p-4 bg-slate-50 dark:bg-slate-800/40">
                      <div class="flex flex-wrap items-center justify-between gap-2 mb-3">
                        <div class="flex flex-wrap gap-2">
                          <BaseButton
                            :label="t('vpn.viewText')"
                            :color="payloadViewMode === 'text' ? 'info' : 'lightDark'"
                            small
                            @click="payloadViewMode = 'text'"
                          />
                          <BaseButton
                            :label="t('vpn.viewHex')"
                            :color="payloadViewMode === 'hex' ? 'info' : 'lightDark'"
                            small
                            @click="payloadViewMode = 'hex'"
                          />
                          <BaseButton
                            :label="t('vpn.viewDecoded')"
                            :color="payloadViewMode === 'decoded' ? 'info' : 'lightDark'"
                            small
                            @click="payloadViewMode = 'decoded'"
                          />
                        </div>
                        <span v-if="row.flow.error" class="text-xs text-red-500">{{ row.flow.error }}</span>
                      </div>
                      <pre
                        v-if="payloadViewMode !== 'decoded'"
                        class="text-xs font-mono whitespace-pre-wrap break-words bg-slate-100 dark:bg-slate-800 rounded-lg p-3 max-h-96 overflow-y-auto"
                      >{{ payloadViewMode === 'hex' ? expandedFlowHex : expandedFlowText }}</pre>
                      <div v-else class="space-y-3">
                        <p v-if="!decodedRequest && !decodedResponse" class="text-xs text-slate-500 dark:text-slate-400 italic">
                          {{ t('vpn.decodedNotHttp') }}
                        </p>
                        <div v-if="decodedRequest">
                          <p class="text-xs font-semibold text-slate-500 dark:text-slate-400 mb-1">{{ t('vpn.decodedRequest') }}</p>
                          <template v-if="decodedRequest.kind === 'text' || decodedRequest.kind === 'json'">
                            <pre class="text-xs font-mono whitespace-pre-wrap break-words bg-slate-100 dark:bg-slate-800 rounded-lg p-3 max-h-96 overflow-y-auto">{{ decodedRequest.headerText }}

{{ decodedRequest.bodyText }}{{ decodedRequest.note ? '\n' + decodedRequest.note : '' }}</pre>
                          </template>
                          <template v-else>
                            <pre class="text-xs font-mono whitespace-pre-wrap break-words bg-slate-100 dark:bg-slate-800 rounded-lg p-3 max-h-40 overflow-y-auto">{{ decodedRequest.headerText }}</pre>
                            <img v-if="decodedRequest.kind === 'image'" :src="decodedRequest.dataUrl" class="max-w-full max-h-96 rounded-lg border border-slate-200 dark:border-slate-700 mt-2 mx-auto block" />
                            <video v-else-if="decodedRequest.kind === 'video'" :src="decodedRequest.dataUrl" controls class="max-w-full max-h-96 rounded-lg border border-slate-200 dark:border-slate-700 mt-2 mx-auto block"></video>
                            <audio v-else-if="decodedRequest.kind === 'audio'" :src="decodedRequest.dataUrl" controls class="w-full mt-2"></audio>
                            <div v-else-if="decodedRequest.kind === 'pdf'" class="mt-2 space-y-1">
                              <embed :src="decodedRequest.dataUrl" type="application/pdf" class="w-full h-96 rounded-lg border border-slate-200 dark:border-slate-700" />
                              <a :href="decodedRequest.dataUrl" download="request.pdf" class="text-xs text-blue-600 dark:text-blue-400 underline inline-block">{{ t('vpn.downloadFile') }}</a>
                            </div>
                            <div v-else-if="decodedRequest.kind === 'binary'" class="mt-2 flex flex-wrap items-center gap-2 text-xs text-slate-500 dark:text-slate-400">
                              <span>{{ decodedRequest.contentType || t('vpn.unknownContentType') }} · {{ formatBytes(decodedRequest.byteLength) }}</span>
                              <a :href="decodedRequest.dataUrl" download="request.bin" class="text-blue-600 dark:text-blue-400 underline">{{ t('vpn.downloadFile') }}</a>
                            </div>
                            <p v-else-if="decodedRequest.kind === 'empty'" class="text-xs text-slate-500 dark:text-slate-400 italic mt-2">{{ t('vpn.emptyBody') }}</p>
                            <p v-if="decodedRequest.note" class="text-xs text-amber-600 dark:text-amber-400 mt-1">{{ decodedRequest.note }}</p>
                          </template>
                        </div>
                        <div v-if="decodedResponse">
                          <p class="text-xs font-semibold text-slate-500 dark:text-slate-400 mb-1">{{ t('vpn.decodedResponse') }}</p>
                          <template v-if="decodedResponse.kind === 'text' || decodedResponse.kind === 'json'">
                            <pre class="text-xs font-mono whitespace-pre-wrap break-words bg-slate-100 dark:bg-slate-800 rounded-lg p-3 max-h-96 overflow-y-auto">{{ decodedResponse.headerText }}

{{ decodedResponse.bodyText }}{{ decodedResponse.note ? '\n' + decodedResponse.note : '' }}</pre>
                          </template>
                          <template v-else>
                            <pre class="text-xs font-mono whitespace-pre-wrap break-words bg-slate-100 dark:bg-slate-800 rounded-lg p-3 max-h-40 overflow-y-auto">{{ decodedResponse.headerText }}</pre>
                            <img v-if="decodedResponse.kind === 'image'" :src="decodedResponse.dataUrl" class="max-w-full max-h-96 rounded-lg border border-slate-200 dark:border-slate-700 mt-2 mx-auto block" />
                            <video v-else-if="decodedResponse.kind === 'video'" :src="decodedResponse.dataUrl" controls class="max-w-full max-h-96 rounded-lg border border-slate-200 dark:border-slate-700 mt-2 mx-auto block"></video>
                            <audio v-else-if="decodedResponse.kind === 'audio'" :src="decodedResponse.dataUrl" controls class="w-full mt-2"></audio>
                            <div v-else-if="decodedResponse.kind === 'pdf'" class="mt-2 space-y-1">
                              <embed :src="decodedResponse.dataUrl" type="application/pdf" class="w-full h-96 rounded-lg border border-slate-200 dark:border-slate-700" />
                              <a :href="decodedResponse.dataUrl" download="response.pdf" class="text-xs text-blue-600 dark:text-blue-400 underline inline-block">{{ t('vpn.downloadFile') }}</a>
                            </div>
                            <div v-else-if="decodedResponse.kind === 'binary'" class="mt-2 flex flex-wrap items-center gap-2 text-xs text-slate-500 dark:text-slate-400">
                              <span>{{ decodedResponse.contentType || t('vpn.unknownContentType') }} · {{ formatBytes(decodedResponse.byteLength) }}</span>
                              <a :href="decodedResponse.dataUrl" download="response.bin" class="text-blue-600 dark:text-blue-400 underline">{{ t('vpn.downloadFile') }}</a>
                            </div>
                            <p v-else-if="decodedResponse.kind === 'empty'" class="text-xs text-slate-500 dark:text-slate-400 italic mt-2">{{ t('vpn.emptyBody') }}</p>
                            <p v-if="decodedResponse.note" class="text-xs text-amber-600 dark:text-amber-400 mt-1">{{ decodedResponse.note }}</p>
                          </template>
                        </div>
                      </div>
                    </div>
                  </td>
                </tr>
              </template>
            </tbody>
          </table>
        </div>
      </CardBox>

      <!-- Pipeline warnings/errors — a separate panel from the flow table above: operational
           problems in the interception pipeline itself (TLS handshake rejections, pf natlookup
           failures, upstream dial failures, …), not captured flow content. Collapsed by default
           so it stays secondary to the main flow table; the count badge stays visible either way. -->
      <CardBox>
        <div
          class="flex items-center justify-between cursor-pointer select-none"
          @click="trafficLogsExpanded = !trafficLogsExpanded"
        >
          <div class="flex items-center gap-2">
            <BaseIcon :path="mdiAlert" size="20" class="text-amber-500" />
            <span class="font-semibold text-slate-700 dark:text-slate-200">{{ t('vpn.trafficLogsTitle') }}</span>
            <span
              v-if="trafficLogs.length > 0"
              class="px-2 py-0.5 text-xs rounded-full bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300 font-mono"
            >
              {{ trafficLogs.length }}
            </span>
          </div>
          <div class="flex items-center gap-2">
            <BaseButton
              v-if="trafficLogs.length > 0"
              :label="t('vpn.clearTrafficLogs')"
              color="lightDark"
              small
              outline
              @click.stop="clearTrafficLogs"
            />
            <BaseIcon :path="trafficLogsExpanded ? mdiChevronUp : mdiChevronDown" size="18" class="text-slate-400" />
          </div>
        </div>

        <div v-if="trafficLogsExpanded" class="mt-3 pt-3 border-t border-slate-200 dark:border-slate-700">
          <div v-if="trafficLogs.length === 0" class="text-center py-6 text-sm text-slate-500 dark:text-slate-400">
            {{ t('vpn.noTrafficLogs') }}
          </div>
          <ul v-else class="space-y-1.5 max-h-72 overflow-y-auto">
            <li
              v-for="(log, idx) in sortedTrafficLogs"
              :key="`${log.timestamp}-${idx}`"
              class="flex gap-3 text-xs font-mono text-slate-600 dark:text-slate-300"
            >
              <span class="shrink-0 text-slate-400 dark:text-slate-500">{{ formatTrafficTime(log.timestamp) }}</span>
              <span class="break-all">{{ log.message }}</span>
            </li>
          </ul>
        </div>
      </CardBox>
    </div>

    <!-- Add Server Modal -->
    <CardBoxModal
      v-model="isAddServerModalActive"
      :title="t('vpn.addVpnServer')"
      :button-label="t('common.create')"
      :button-loading="loading"
      @confirm="createServer"
    >
      <FormField :label="t('common.name')">
        <FormControl v-model="newServer.name" placeholder="Main VPN Server" />
      </FormField>

      <FormField :label="t('vpn.addressCidr')">
        <FormControl v-model="newServer.address" placeholder="10.0.0.1/24" />
      </FormField>

      <FormField :label="t('vpn.endpoint')" :help="t('vpn.endpointHelp')">
        <FormControl v-model="newServer.endpoint" placeholder="192.168.1.187:51820" />
      </FormField>

      <FormField :label="t('vpn.dns')">
        <FormControl v-model="newServer.dns" placeholder="1.1.1.1,8.8.8.8" />
      </FormField>

      <FormField :label="t('vpn.listenPort')">
        <FormControl v-model.number="newServer.listen_port" type="number" />
      </FormField>

      <FormField :label="t('vpn.mtu')">
        <FormControl v-model.number="newServer.mtu" type="number" />
      </FormField>

      <FormField :label="t('vpn.description')">
        <FormControl v-model="newServer.description" type="textarea" />
      </FormField>
    </CardBoxModal>

    <!-- Edit Server Modal -->
    <CardBoxModal
      v-model="isEditServerModalActive"
      :title="t('vpn.editVpnServer')"
      :button-label="t('common.update')"
      :button-loading="loading"
      @confirm="updateServer"
    >
      <FormField v-if="editingServer" :label="t('common.name')">
        <FormControl v-model="editingServer.name" />
      </FormField>

      <FormField v-if="editingServer" :label="t('vpn.addressCidr')">
        <FormControl v-model="editingServer.address" />
      </FormField>

      <FormField v-if="editingServer" :label="t('vpn.endpoint')" :help="t('vpn.endpointHelp2')">
        <FormControl v-model="editingServer.endpoint" placeholder="192.168.1.187:51820" />
      </FormField>

      <FormField v-if="editingServer" :label="t('vpn.dns')">
        <FormControl v-model="editingServer.dns" />
      </FormField>

      <FormField v-if="editingServer" :label="t('vpn.listenPort')">
        <FormControl v-model.number="editingServer.listen_port" type="number" />
      </FormField>

      <FormField v-if="editingServer" :label="t('vpn.mtu')">
        <FormControl v-model.number="editingServer.mtu" type="number" />
      </FormField>

      <FormField v-if="editingServer" :label="t('vpn.description')">
        <FormControl v-model="editingServer.description" type="textarea" />
      </FormField>
    </CardBoxModal>

    <!-- Add User Modal -->
    <CardBoxModal
      v-model="isAddUserModalActive"
      :title="t('vpn.addVpnUser')"
      :button-label="t('common.create')"
      :button-loading="loading"
      @confirm="createUser"
    >
      <FormField :label="t('vpn.server')">
        <FormControl
          v-model="newUser.server_id"
          :options="servers.map(s => ({ value: s.id, label: s.name }))"
          :placeholder="t('vpn.selectServer')"
        />
      </FormField>

      <FormField :label="t('vpn.username')">
        <FormControl v-model="newUser.username" placeholder="john" />
      </FormField>

      <FormField :label="t('common.email')">
        <FormControl v-model="newUser.email" type="email" placeholder="john@example.com" />
      </FormField>

      <FormField :label="t('vpn.fullName')">
        <FormControl v-model="newUser.full_name" placeholder="John Doe" />
      </FormField>

      <FormField :label="t('vpn.allowedIps')">
        <FormControl v-model="newUser.allowed_ips" placeholder="0.0.0.0/0" />
      </FormField>

      <FormField :label="t('vpn.dnsOptional')">
        <FormControl v-model="newUser.dns" placeholder="1.1.1.1" />
      </FormField>

      <FormField :label="t('vpn.quotaUnlimited')">
        <FormControl v-model.number="newUser.quota" type="number" />
      </FormField>

      <FormField :label="t('vpn.notes')">
        <FormControl v-model="newUser.notes" type="textarea" />
      </FormField>
    </CardBoxModal>

    <!-- Edit User Modal -->
    <CardBoxModal
      v-model="isEditUserModalActive"
      :title="t('vpn.editVpnUser')"
      :button-label="t('common.update')"
      :button-loading="loading"
      @confirm="updateUser"
    >
      <FormField v-if="editingUser" :label="t('vpn.username')">
        <FormControl v-model="editingUser.username" />
      </FormField>

      <FormField v-if="editingUser" :label="t('common.email')">
        <FormControl v-model="editingUser.email" type="email" />
      </FormField>

      <FormField v-if="editingUser" :label="t('vpn.fullName')">
        <FormControl v-model="editingUser.full_name" />
      </FormField>

      <FormField v-if="editingUser" :label="t('vpn.allowedIps')">
        <FormControl v-model="editingUser.allowed_ips" />
      </FormField>

      <FormField v-if="editingUser" :label="t('vpn.dns')">
        <FormControl v-model="editingUser.dns" />
      </FormField>

      <FormField v-if="editingUser" :label="t('vpn.quotaBytes')">
        <FormControl v-model.number="editingUser.quota" type="number" />
      </FormField>

      <FormField v-if="editingUser" :label="t('vpn.notes')">
        <FormControl v-model="editingUser.notes" type="textarea" />
      </FormField>
    </CardBoxModal>

    <!-- Delete Confirmation Modal -->
    <CardBoxModal
      v-model="isDeleteModalActive"
      :title="t('vpn.confirmDelete')"
      button-:label="t('common.delete')"
      button-color="danger"
      :button-loading="loading"
      @confirm="confirmDelete"
    >
      <p class="text-slate-600 dark:text-slate-400">
        {{ t('vpn.deleteConfirmText', { type: deleteTarget.type }) }}
        <span v-if="deleteTarget.item" class="font-semibold">
          {{ deleteTarget.item.name || deleteTarget.item.username }}
        </span>
      </p>
    </CardBoxModal>

    <!-- QR Code Modal -->
    <CardBoxModal
      v-model="isQRCodeModalActive"
      :title="t('vpn.qrTitle')"
      :has-button="false"
    >
      <div class="text-center space-y-4">
        <div>
          <p class="text-sm text-slate-600 dark:text-slate-400 mb-4">
            {{ t('vpn.scanQr') }}
          </p>
          <div class="flex justify-center">
            <img
              v-if="qrCodeData.qrcode"
              :src="qrCodeData.qrcode"
              alt="WireGuard QR Code"
              class="border-2 border-slate-200 dark:border-slate-700 rounded-lg p-2 bg-white"
            />
          </div>
        </div>
        
        <div class="mt-6 pt-4 border-t border-slate-200 dark:border-slate-700">
          <p class="text-xs text-slate-500 dark:text-slate-400 mb-2">{{ t('vpn.copyConfigManually') }}</p>
          <div class="bg-slate-100 dark:bg-slate-800 rounded-lg p-3">
            <pre class="text-xs font-mono text-left overflow-x-auto whitespace-pre-wrap break-words">{{ qrCodeData.config }}</pre>
          </div>
          <BaseButton
            :icon="mdiContentDuplicate"
            :label="t('vpn.copyConfig')"
            color="info"
            small
            class="mt-3"
            @click="navigator.clipboard.writeText(qrCodeData.config); toast.success(t('vpn.configCopied'))"
          />
        </div>
      </div>
    </CardBoxModal>

    <!-- Resend (Replay) Modal -->
    <CardBoxModal
      v-model="isResendModalActive"
      :title="t('vpn.resendTitle')"
      :button-label="resendLoading ? t('vpn.resendSending') : t('vpn.resendSend')"
      :button-disabled="resendLoading"
      has-cancel
      @confirm="sendResendRequest"
      @cancel="closeResendModal"
    >
      <FormField :label="t('vpn.resendMethod')">
        <FormControl
          v-model="resendRequest.method"
          :options="['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS']"
        />
      </FormField>

      <FormField :label="t('vpn.resendUrl')">
        <FormControl v-model="resendRequest.url" placeholder="https://example.com/api/path" />
      </FormField>

      <FormField :label="t('vpn.resendHeaders')" :help="t('vpn.resendHeadersHelp')">
        <FormControl v-model="resendRequest.headersText" type="textarea" height="8rem" />
      </FormField>

      <FormField :label="t('vpn.resendBody')">
        <FormControl v-model="resendRequest.body" type="textarea" height="8rem" />
      </FormField>

      <div v-if="resendLoading" class="text-xs text-slate-500 dark:text-slate-400 italic">
        {{ t('vpn.resendSending') }}
      </div>

      <div
        v-else-if="resendResult && resendResult.error"
        class="rounded-lg border border-red-300 dark:border-red-700 bg-red-50 dark:bg-red-900/20 p-3 text-sm text-red-700 dark:text-red-300"
      >
        {{ t('vpn.resendNetworkError', { error: resendResult.error }) }}
      </div>

      <div v-else-if="resendResult" class="space-y-3 border-t border-slate-200 dark:border-slate-700 pt-3 mt-2">
        <div class="flex flex-wrap items-center gap-3 text-sm">
          <span
            class="px-2 py-1 text-xs rounded font-mono"
            :class="resendResult.status >= 200 && resendResult.status < 400
              ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300'
              : 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'"
          >
            {{ resendResult.status }} {{ resendResult.statusText }}
          </span>
          <span class="text-xs text-slate-500 dark:text-slate-400">
            {{ t('vpn.resendDuration') }}: {{ resendResult.durationMs }}ms
          </span>
        </div>

        <div v-if="resendResult.decoded">
          <p class="text-xs font-semibold text-slate-500 dark:text-slate-400 mb-1">{{ t('vpn.resendResponse') }}</p>
          <template v-if="resendResult.decoded.kind === 'text' || resendResult.decoded.kind === 'json'">
            <pre class="text-xs font-mono whitespace-pre-wrap break-words bg-slate-100 dark:bg-slate-800 rounded-lg p-3 max-h-96 overflow-y-auto">{{ resendResult.decoded.headerText }}

{{ resendResult.decoded.bodyText }}{{ resendResult.decoded.note ? '\n' + resendResult.decoded.note : '' }}</pre>
          </template>
          <template v-else>
            <pre class="text-xs font-mono whitespace-pre-wrap break-words bg-slate-100 dark:bg-slate-800 rounded-lg p-3 max-h-40 overflow-y-auto">{{ resendResult.decoded.headerText }}</pre>
            <img v-if="resendResult.decoded.kind === 'image'" :src="resendResult.decoded.dataUrl" class="max-w-full max-h-96 rounded-lg border border-slate-200 dark:border-slate-700 mt-2 mx-auto block" />
            <video v-else-if="resendResult.decoded.kind === 'video'" :src="resendResult.decoded.dataUrl" controls class="max-w-full max-h-96 rounded-lg border border-slate-200 dark:border-slate-700 mt-2 mx-auto block"></video>
            <audio v-else-if="resendResult.decoded.kind === 'audio'" :src="resendResult.decoded.dataUrl" controls class="w-full mt-2"></audio>
            <div v-else-if="resendResult.decoded.kind === 'pdf'" class="mt-2 space-y-1">
              <embed :src="resendResult.decoded.dataUrl" type="application/pdf" class="w-full h-96 rounded-lg border border-slate-200 dark:border-slate-700" />
              <a :href="resendResult.decoded.dataUrl" download="response.pdf" class="text-xs text-blue-600 dark:text-blue-400 underline inline-block">{{ t('vpn.downloadFile') }}</a>
            </div>
            <div v-else-if="resendResult.decoded.kind === 'binary'" class="mt-2 flex flex-wrap items-center gap-2 text-xs text-slate-500 dark:text-slate-400">
              <span>{{ resendResult.decoded.contentType || t('vpn.unknownContentType') }} · {{ formatBytes(resendResult.decoded.byteLength) }}</span>
              <a :href="resendResult.decoded.dataUrl" download="response.bin" class="text-blue-600 dark:text-blue-400 underline">{{ t('vpn.downloadFile') }}</a>
            </div>
            <p v-else-if="resendResult.decoded.kind === 'empty'" class="text-xs text-slate-500 dark:text-slate-400 italic mt-2">{{ t('vpn.emptyBody') }}</p>
            <p v-if="resendResult.decoded.note" class="text-xs text-amber-600 dark:text-amber-400 mt-1">{{ resendResult.decoded.note }}</p>
          </template>
        </div>
        <p v-else class="text-xs text-slate-500 dark:text-slate-400 italic">{{ t('vpn.decodedNotHttp') }}</p>
      </div>
    </CardBoxModal>
  </div>
</template>
