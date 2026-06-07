<script setup>
import BaseButton from "@/components/BaseButton.vue";
import BaseIcon from "@/components/BaseIcon.vue";
import CardBox from "@/components/CardBox.vue";
import CardBoxModal from "@/components/CardBoxModal.vue";
import FormCheckRadio from "@/components/FormCheckRadio.vue";
import FormControl from "@/components/FormControl.vue";
import FormField from "@/components/FormField.vue";
import SectionTitleLineWithButton from "@/components/SectionTitleLineWithButton.vue";

import ApiService from "@/services/ApiService";
import { useToast } from "vue-toastification";
import {
  mdiCertificate,
  mdiChartLine,
  mdiCog,
  mdiDatabase,
  mdiDelete,
  mdiHeart,
  mdiHeartPulse,
  mdiLock,
  mdiMagnify,
  mdiMessageText,
  mdiPencil,
  mdiPlay,
  mdiPlus,
  mdiRefresh,
  mdiRouter,
  mdiServer,
  mdiShield,
  mdiSourceBranch,
  mdiSpeedometer,
  mdiStop,
  mdiSync,
  mdiWeb
} from '@mdi/js';
import { computed, onMounted, onUnmounted, ref } from "vue";
import { useI18n } from "vue-i18n";

const toast = useToast();
const { t } = useI18n();

// Reactive state
const loading = ref(false)
const status = ref({ running: false })
const stats = ref({})
const config = ref({
  http_port: 80,
  https_port: 443,
  https_enabled: false,
  services: [],
  routes: [],
  access_log_enabled: true,
  enabled: false
})
const services = ref([])
const upstreams = ref([])
const routes = ref([])
const routeSearch = ref('')
const routeStatusFilter = ref('all') // all | enabled | disabled
const serviceSearch = ref('')
const serviceStatusFilter = ref('all')
const upstreamSearch = ref('')
const upstreamStatusFilter = ref('all')
const serviceHealth = ref([])
const certificateInfo = ref({})
const renewerStatus = ref({ running: false })
const manualBlockForm = ref({
  ip: '',
  duration_seconds: 300,
  reason: ''
})

// Modal states
const activeTab = ref('overview')
const isAddServiceModalActive = ref(false)
const isEditServiceModalActive = ref(false)
const isAddRouteModalActive = ref(false)
const isEditRouteModalActive = ref(false)
const isAddUpstreamModalActive = ref(false)
const isEditUpstreamModalActive = ref(false)
const isDeleteModalActive = ref(false)
const isConfigModalActive = ref(false)
const isObservabilityModalActive = ref(false)
const deleteTarget = ref({ type: '', item: null })
const editingService = ref(null)
const editingRoute = ref(null)
const editingUpstream = ref(null)

// FormControl select stores the WHOLE option object as v-model value (see
// FormControl.vue :value="option"), so dropdown-bound fields are kept as
// {value,label} pairs in form state and unwrapped to plain strings in
// buildUpstreamPayload.
const strategyOptions = [
  { value: 'round_robin', label: 'Round Robin' },
  { value: 'weighted',    label: 'Weighted' },
  { value: 'random',      label: 'Random' },
  { value: 'least_conn',  label: 'Least Connections' }
]
const stickyModeOptions = [
  { value: 'ip_hash', label: 'IP hash' },
  { value: 'cookie',  label: 'Cookie' },
  { value: 'header',  label: 'Header' }
]
const findOption = (opts, val) => opts.find(o => o.value === val) || opts[0]
// Wrap a service ID into the {value,label} option shape FormControl expects;
// falls back to the raw id if the service isn't (yet) in the loaded list.
const findServiceOption = (id) => {
  if (!id) return ''
  const s = services.value.find(x => x.id === id)
  return s ? { value: s.id, label: s.name || s.id } : { value: id, label: id }
}

const createDefaultUpstream = () => ({
  name: '',
  strategy: findOption(strategyOptions, 'round_robin'),
  enabled: true,
  sticky_enabled: false,
  sticky: {
    mode: findOption(stickyModeOptions, 'ip_hash'),
    cookie_name: 'redock_lb',
    header_name: '',
    ttl_seconds: 0
  },
  targets: [{ service_id: '', weight: 1 }]
})

const newUpstream = ref(createDefaultUpstream())

// Form data
const newService = ref({
  name: '',
  host: '',
  port: 80,
  protocol: 'http',
  path: '',
  timeout: 30,
  enabled: true,
  health_check: {
    path: '/health',
    interval: 5,
    timeout: 5,
    healthy_threshold: 2,
    unhealthy_threshold: 3
  }
})

const newRoute = ref({
  name: '',
  upstream_id: '',
  paths: '',
  methods: '',
  hosts: '',
  strip_path: true,
  preserve_host: false,
  host_rewrite: '',
  priority: 0,
  timeout: 0,
  rate_limit_enabled: false,
  rate_limit_requests: 100,
  rate_limit_window: 60,
  auth_required: false,
  auth_type: '',
  auth_headers: [],
  basic_auth_users: [],
  basic_auth_realm: 'Restricted',
  jwt: { secret: '', issuer: '', audience: '' },
  observability_enabled: true,
  lets_encrypt: false,
  enabled: true,
  cors: {
    enabled: false,
    allow_origins: '',
    allow_methods: 'GET, POST, PUT, DELETE, OPTIONS',
    allow_headers: 'Content-Type, Authorization',
    expose_headers: '',
    allow_credentials: false,
    max_age: 86400
  },
  response_headers: []
})

const createDefaultLokiConfig = () => ({
  url: '',
  tenant_id: '',
  api_key: '',
  labels: {}
})

const createDefaultInfluxConfig = () => ({
  url: '',
  org: '',
  bucket: '',
  token: ''
})

const createDefaultGraylogConfig = () => ({
  endpoint: '',
  api_key: '',
  api_key_header: 'Authorization',
  stream_id: ''
})

const createDefaultObservabilityConfig = () => ({
  enabled: false,
  loki_enabled: false,
  loki: createDefaultLokiConfig(),
  influx_enabled: false,
  influx: createDefaultInfluxConfig(),
  graylog_enabled: false,
  graylog: createDefaultGraylogConfig(),
  otlp_enabled: false,
  otlp_endpoint: '',
  otlp_headers: {},
  clickhouse_enabled: false,
  clickhouse_endpoint: '',
  clickhouse_database: 'default',
  clickhouse_table: 'api_gateway_logs',
  clickhouse_username: '',
  clickhouse_password: '',
  batch_size: 100,
  flush_interval: 30
})

const createDefaultClientSecurityConfig = () => ({
  tracking_enabled: true,
  top_client_limit: 1000,
  auto_block_enabled: true,
  no_route_threshold: 5,
  auto_block_duration_seconds: 300,
  manual_blocks: []
})

const createDefaultTimeoutsConfig = () => ({
  server_read_seconds: 30,
  server_write_seconds: 30,
  server_idle_seconds: 60,
  shutdown_seconds: 10,
  request_timeout_seconds: 0,
  upstream_dial_seconds: 30,
  upstream_keep_alive_seconds: 30,
  upstream_idle_conn_seconds: 90,
  tls_handshake_seconds: 10,
  expect_continue_seconds: 1,
  health_check_seconds: 5
})

const normalizeObservabilityConfig = (cfg = {}) => {
  const base = createDefaultObservabilityConfig()
  return {
    ...base,
    enabled: cfg.enabled ?? base.enabled,
    loki_enabled: cfg.loki_enabled ?? base.loki_enabled,
    loki: {
      ...base.loki,
      ...(cfg.loki || cfg.loki_datasource || {}),
      labels: { ...(((cfg.loki || cfg.loki_datasource || {}).labels) || {}) }
    },
    influx_enabled: cfg.influx_enabled ?? base.influx_enabled,
    influx: {
      ...base.influx,
      ...(cfg.influx || cfg.influxdb || {})
    },
    graylog_enabled: cfg.graylog_enabled ?? base.graylog_enabled,
    graylog: {
      ...base.graylog,
      ...(cfg.graylog || {})
    },
    otlp_enabled: cfg.otlp_enabled ?? base.otlp_enabled,
    otlp_endpoint: cfg.otlp_endpoint ?? base.otlp_endpoint,
    otlp_headers: { ...(cfg.otlp_headers || {}) },
    clickhouse_enabled: cfg.clickhouse_enabled ?? base.clickhouse_enabled,
    clickhouse_endpoint: cfg.clickhouse_endpoint ?? base.clickhouse_endpoint,
    clickhouse_database: cfg.clickhouse_database ?? base.clickhouse_database,
    clickhouse_table: cfg.clickhouse_table ?? base.clickhouse_table,
    clickhouse_username: cfg.clickhouse_username ?? base.clickhouse_username,
    clickhouse_password: cfg.clickhouse_password ?? base.clickhouse_password,
    batch_size: cfg.batch_size ?? base.batch_size,
    flush_interval: cfg.flush_interval ?? base.flush_interval
  }
}

const observabilityConfig = ref(createDefaultObservabilityConfig())

const gatewayConfig = ref({
  http_port: 80,
  https_port: 443,
  https_enabled: false,
  access_log_enabled: true,
  client_security: createDefaultClientSecurityConfig(),
  timeouts: createDefaultTimeoutsConfig(),
  lets_encrypt: {
    enabled: false,
    email: '',
    auto_renew: true,
    renew_before_days: 30
  }
})

const isSuccessfulResponse = (response) => {
  return response && response.data && response.data.error === false
}

// Computed
const gatewayStats = computed(() => {
  const totalServices = services.value.length
  const healthyServices = serviceHealth.value.filter(h => h.healthy).length
  const totalRoutes = routes.value.length
  const activeRoutes = routes.value.filter(r => r.enabled).length

  return {
    totalServices,
    healthyServices,
    totalRoutes,
    activeRoutes,
    totalRequests: stats.value.total_requests || 0,
    totalErrors: stats.value.total_errors || 0,
    avgLatency: stats.value.average_latency_ms?.toFixed(2) || 0,
    uptime: formatUptime(stats.value.uptime_seconds || 0)
  }
})

const serviceNameMap = computed(() => {
  const map = {}
  services.value.forEach(service => {
    if (service?.id) {
      map[service.id] = service.name || service.id
    }
  })
  return map
})

const routeMap = computed(() => {
  const map = {}
  routes.value.forEach(route => {
    if (route?.id) {
      map[route.id] = route
    }
  })
  return map
})
// Single source of truth for the upstream dropdown's options. Both the
// add-route and edit-route modals bind to it so that the v-model object
// reference is preserved across init/update — Vue's <select> v-model does
// strict equality, so a freshly-built {value,label} pair would never match
// the array's entries even if they encode the same upstream.
const upstreamOptions = computed(() =>
  upstreams.value.map(u => ({
    value: u.id,
    label: (u.name || u.id) + ' — ' + (u.strategy || 'round_robin')
  }))
)
const findUpstreamOption = (id) => {
  if (!id) return null
  return upstreamOptions.value.find(o => o.value === id) || { value: id, label: id }
}

// Same shape as filteredRoutes/filteredServices: `enabled` boolean +
// substring match across the visible columns. Kept as a single helper so
// search semantics stay consistent across the three tabs.
const matchStatus = (enabled, status) => {
  if (status === 'enabled') return enabled !== false
  if (status === 'disabled') return enabled === false
  return true
}
const matchSubstring = (q, parts) => {
  if (!q) return true
  return parts.filter(Boolean).join(' ').toLowerCase().includes(q.trim().toLowerCase())
}

const filteredServices = computed(() =>
  services.value.filter(s =>
    matchStatus(s.enabled, serviceStatusFilter.value) &&
    matchSubstring(serviceSearch.value, [s.name, s.host, String(s.port || ''), s.protocol, s.id])
  )
)

const filteredUpstreams = computed(() =>
  upstreams.value.filter(u =>
    matchStatus(u.enabled, upstreamStatusFilter.value) &&
    matchSubstring(upstreamSearch.value, [
      u.name, u.id, u.strategy,
      ...(Array.isArray(u.targets) ? u.targets.map(t => getServiceName(t.service_id)) : [])
    ])
  )
)

// Filtered + sorted route list driven by the routes-tab search box and
// status chips. Matches against name, paths, hosts, and the resolved
// upstream name (so users can search by backend pool too).
const filteredRoutes = computed(() => {
  const q = routeSearch.value.trim().toLowerCase()
  const status = routeStatusFilter.value
  return routes.value.filter(r => {
    if (status === 'enabled' && !r.enabled) return false
    if (status === 'disabled' && r.enabled) return false
    if (!q) return true
    const haystack = [
      r.name || '',
      ...(Array.isArray(r.paths) ? r.paths : []),
      ...(Array.isArray(r.hosts) ? r.hosts : []),
      r.host_rewrite || '',
      getUpstreamName(r.upstream_id)
    ].join(' ').toLowerCase()
    return haystack.includes(q)
  })
})

const topClientDisplayOptions = [10, 20, 50, 100, 250, 500, 1000]
const topClientDisplayLimit = ref(10)
const allTopClients = computed(() => stats.value.top_clients || [])
const topClients = computed(() => allTopClients.value.slice(0, topClientDisplayLimit.value))
const blockedClients = computed(() => stats.value.blocked_clients || [])

// Methods
const formatUptime = (seconds) => {
  if (!seconds) return '0s'
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const mins = Math.floor((seconds % 3600) / 60)
  if (days > 0) return `${days}d ${hours}h`
  if (hours > 0) return `${hours}h ${mins}m`
  return `${mins}m`
}

const loadData = async () => {
  loading.value = true
  try {
    const [statusRes, statsRes, servicesRes, upstreamsRes, routesRes, healthRes, certRes, renewerRes, observabilityRes] = await Promise.all([
      ApiService.apiGatewayStatus().catch(() => ({ data: { data: {} } })),
      ApiService.apiGatewayStats().catch(() => ({ data: { data: {} } })),
      ApiService.apiGatewayListServices().catch(() => ({ data: { data: [] } })),
      ApiService.apiGatewayListUpstreams().catch(() => ({ data: { data: [] } })),
      ApiService.apiGatewayListRoutes().catch(() => ({ data: { data: [] } })),
      ApiService.apiGatewayHealth().catch(() => ({ data: { data: [] } })),
      ApiService.apiGatewayCertificateInfo().catch(() => ({ data: { data: {} } })),
      ApiService.apiGatewayRenewerStatus().catch(() => ({ data: { data: {} } })),
      ApiService.apiGatewayGetObservabilityStatus().catch(() => ({ data: { data: {} } }))
    ])

    status.value = statusRes.data.data || {}
    stats.value = statsRes.data.data || {}
    services.value = servicesRes.data.data || []
    upstreams.value = upstreamsRes.data.data || []
    routes.value = routesRes.data.data || []
    serviceHealth.value = healthRes.data.data || []
    certificateInfo.value = certRes.data.data || {}
    renewerStatus.value = renewerRes.data.data || {}
    observabilityConfig.value = normalizeObservabilityConfig(observabilityRes.data.data?.config || {})
  } catch (error) {
    console.error('Failed to load API Gateway data:', error)
  } finally {
    loading.value = false
  }
}

const startGateway = async () => {
  try {
    await ApiService.apiGatewayStart()
    await loadData()
  } catch (error) {
    console.error('Failed to start gateway:', error)
  }
}

const stopGateway = async () => {
  try {
    await ApiService.apiGatewayStop()
    await loadData()
  } catch (error) {
    console.error('Failed to stop gateway:', error)
  }
}

const openAddServiceModal = () => {
  newService.value = {
    name: '',
    host: '',
    port: 80,
    protocol: 'http',
    path: '',
    timeout: 30,
    enabled: true,
    health_check: {
      path: '/health',
      interval: 5,
      timeout: 5,
      healthy_threshold: 2,
      unhealthy_threshold: 3
    }
  }
  isAddServiceModalActive.value = true
}

const addService = async () => {
  try {
    const response = await ApiService.apiGatewayAddService(newService.value)
    if (isSuccessfulResponse(response)) {
      isAddServiceModalActive.value = false
      await loadData()
    }
  } catch (error) {
    console.error('Failed to add service:', error)
  }
}

const openEditServiceModal = (service) => {
  editingService.value = { ...service }
  if (!editingService.value.health_check) {
    editingService.value.health_check = {
      path: '/health',
      interval: 5,
      timeout: 5,
      healthy_threshold: 2,
      unhealthy_threshold: 3
    }
  }
  isEditServiceModalActive.value = true
}

const updateService = async () => {
  try {
    const response = await ApiService.apiGatewayUpdateService(editingService.value)
    if (isSuccessfulResponse(response)) {
      isEditServiceModalActive.value = false
      await loadData()
    }
  } catch (error) {
    console.error('Failed to update service:', error)
  }
}

const openAddRouteModal = () => {
  newRoute.value = {
    name: '',
    upstream_id: null,
    paths: '',
    methods: '',
    hosts: '',
    strip_path: true,
    preserve_host: false,
    host_rewrite: '',
    priority: 0,
    timeout: 0,
    rate_limit_enabled: false,
    rate_limit_requests: 100,
    rate_limit_window: 60,
    auth_required: false,
    auth_type: '',
    auth_headers: [],
    basic_auth_users: [],
    basic_auth_realm: 'Restricted',
    jwt: { secret: '', issuer: '', audience: '' },
    observability_enabled: true,
    enabled: true,
    cors: {
      enabled: false,
      allow_origins: '',
      allow_methods: 'GET, POST, PUT, DELETE, OPTIONS',
      allow_headers: 'Content-Type, Authorization',
      expose_headers: '',
      allow_credentials: false,
      max_age: 86400
    },
    response_headers: []
  }
  isAddRouteModalActive.value = true
}

function buildCorsPayload(cors) {
  if (!cors || !cors.enabled) return null
  return {
    enabled: true,
    allow_origins: (cors.allow_origins || '').split(',').map(s => s.trim()).filter(Boolean),
    allow_methods: (cors.allow_methods || '').split(',').map(s => s.trim().toUpperCase()).filter(Boolean),
    allow_headers: (cors.allow_headers || '').split(',').map(s => s.trim()).filter(Boolean),
    expose_headers: (cors.expose_headers || '').split(',').map(s => s.trim()).filter(Boolean),
    allow_credentials: !!cors.allow_credentials,
    max_age: parseInt(cors.max_age, 10) || 0
  }
}

function buildResponseHeadersPayload(arr) {
  if (!Array.isArray(arr)) return undefined
  const out = {}
  for (const { key, value } of arr) {
    if (key && String(key).trim()) out[String(key).trim()] = value == null ? '' : String(value)
  }
  return out
}

function buildAuthHeadersPayload(arr) {
  if (!Array.isArray(arr)) return undefined
  const out = []
  for (const { key, value } of arr) {
    const k = key != null ? String(key).trim() : ''
    if (k) out.push({ key: k, value: value == null ? '' : String(value) })
  }
  return out.length ? out : undefined
}

function buildBasicAuthUsersPayload(arr) {
  if (!Array.isArray(arr)) return undefined
  const out = []
  for (const u of arr) {
    const username = (u?.username != null ? String(u.username) : '').trim()
    if (!username) continue
    const password = u?.password != null ? String(u.password) : ''
    const hash = u?.password_hash != null ? String(u.password_hash) : ''
    const entry = { username }
    if (password) entry.password = password
    else if (hash) entry.password_hash = hash
    out.push(entry)
  }
  return out.length ? out : undefined
}

function buildJWTPayload(j) {
  if (!j) return undefined
  const secret = j.secret != null ? String(j.secret).trim() : ''
  if (!secret) return undefined
  const out = { secret }
  if (j.issuer && String(j.issuer).trim()) out.issuer = String(j.issuer).trim()
  if (j.audience && String(j.audience).trim()) out.audience = String(j.audience).trim()
  return out
}

function normalizeAuthType(v) {
  if (v == null) return ''
  if (typeof v === 'string') return v
  if (typeof v === 'object' && v !== null && 'value' in v) return v.value ?? ''
  return ''
}

const addRoute = async () => {
  try {
    const authType = normalizeAuthType(newRoute.value.auth_type)
    const routeData = {
      ...newRoute.value,
      paths: newRoute.value.paths.split(',').map(p => p.trim()).filter(p => p),
      methods: newRoute.value.methods ? newRoute.value.methods.split(',').map(m => m.trim().toUpperCase()).filter(m => m) : [],
      hosts: newRoute.value.hosts ? newRoute.value.hosts.split(',').map(h => h.trim()).filter(h => h) : [],
      upstream_id: newRoute.value.upstream_id?.value || newRoute.value.upstream_id,
      auth_type: authType,
      lets_encrypt: newRoute.value.lets_encrypt === true,
      cors: buildCorsPayload(newRoute.value.cors),
      response_headers: buildResponseHeadersPayload(newRoute.value.response_headers),
      auth_headers: authType === 'header' ? buildAuthHeadersPayload(newRoute.value.auth_headers) : undefined,
      basic_auth_users: authType === 'basic' ? buildBasicAuthUsersPayload(newRoute.value.basic_auth_users) : undefined,
      basic_auth_realm: authType === 'basic' ? (newRoute.value.basic_auth_realm || 'Restricted') : undefined,
      jwt: authType === 'jwt' ? buildJWTPayload(newRoute.value.jwt) : undefined
    }
    const response = await ApiService.apiGatewayAddRoute(routeData)
    if (isSuccessfulResponse(response)) {
      isAddRouteModalActive.value = false
      await loadData()
    }
  } catch (error) {
    console.error('Failed to add route:', error)
  }
}

const openEditRouteModal = (route) => {
  const upstreamId = route.upstream_id?.value || route.upstream_id
  const cors = route.cors
  const responseHeaders = route.response_headers && typeof route.response_headers === 'object'
    ? Object.entries(route.response_headers).map(([key, value]) => ({ key, value }))
    : []
  const authHeaders = Array.isArray(route.auth_headers)
    ? route.auth_headers.map(h => ({ key: h.key || h.Key || '', value: h.value != null ? String(h.value) : (h.Value != null ? String(h.Value) : '') }))
    : []
  const basicUsers = Array.isArray(route.basic_auth_users)
    ? route.basic_auth_users.map(u => ({
        username: u.username || '',
        password: '', // never round-trip; blank means "keep existing hash"
        password_hash: u.password_hash || ''
      }))
    : []
  const jwtCfg = route.jwt && typeof route.jwt === 'object'
    ? { secret: route.jwt.secret || '', issuer: route.jwt.issuer || '', audience: route.jwt.audience || '' }
    : { secret: '', issuer: '', audience: '' }

  editingRoute.value = {
    ...route,
    paths: Array.isArray(route.paths) ? route.paths.join(', ') : route.paths || '',
    methods: Array.isArray(route.methods) ? route.methods.join(', ') : route.methods || '',
    hosts: Array.isArray(route.hosts) ? route.hosts.join(', ') : route.hosts || '',
    host_rewrite: route.host_rewrite || '',
    preserve_host: route.preserve_host === true,
    observability_enabled: route.observability_enabled !== false,
    lets_encrypt: route.lets_encrypt === true,
    upstream_id: findUpstreamOption(upstreamId),
    cors: cors ? {
      enabled: !!cors.enabled,
      allow_origins: Array.isArray(cors.allow_origins) ? cors.allow_origins.join(', ') : (cors.allow_origins || ''),
      allow_methods: Array.isArray(cors.allow_methods) ? cors.allow_methods.join(', ') : (cors.allow_methods || ''),
      allow_headers: Array.isArray(cors.allow_headers) ? cors.allow_headers.join(', ') : (cors.allow_headers || ''),
      expose_headers: Array.isArray(cors.expose_headers) ? cors.expose_headers.join(', ') : (cors.expose_headers || ''),
      allow_credentials: !!cors.allow_credentials,
      max_age: parseInt(cors.max_age, 10) || 0
    } : { enabled: false, allow_origins: '', allow_methods: '', allow_headers: '', expose_headers: '', allow_credentials: false, max_age: 0 },
    response_headers: responseHeaders,
    auth_headers: authHeaders,
    basic_auth_users: basicUsers,
    basic_auth_realm: route.basic_auth_realm || 'Restricted',
    jwt: jwtCfg
  }
  isEditRouteModalActive.value = true
}

const updateRoute = async () => {
  try {
    const authType = normalizeAuthType(editingRoute.value.auth_type)
    const routeData = {
      ...editingRoute.value,
      paths: editingRoute.value.paths.split(',').map(p => p.trim()).filter(p => p),
      methods: editingRoute.value.methods ? editingRoute.value.methods.split(',').map(m => m.trim().toUpperCase()).filter(m => m) : [],
      hosts: editingRoute.value.hosts ? editingRoute.value.hosts.split(',').map(h => h.trim()).filter(h => h) : [],
      upstream_id: editingRoute.value.upstream_id?.value || editingRoute.value.upstream_id,
      auth_type: authType,
      lets_encrypt: editingRoute.value.lets_encrypt === true,
      cors: buildCorsPayload(editingRoute.value.cors),
      response_headers: buildResponseHeadersPayload(editingRoute.value.response_headers),
      auth_headers: authType === 'header' ? buildAuthHeadersPayload(editingRoute.value.auth_headers) : undefined,
      basic_auth_users: authType === 'basic' ? buildBasicAuthUsersPayload(editingRoute.value.basic_auth_users) : undefined,
      basic_auth_realm: authType === 'basic' ? (editingRoute.value.basic_auth_realm || 'Restricted') : undefined,
      jwt: authType === 'jwt' ? buildJWTPayload(editingRoute.value.jwt) : undefined
    }
    const response = await ApiService.apiGatewayUpdateRoute(routeData)
    if (isSuccessfulResponse(response)) {
      isEditRouteModalActive.value = false
      await loadData()
    }
  } catch (error) {
    console.error('Failed to update route:', error)
  }
}

const confirmDelete = (type, item) => {
  deleteTarget.value = { type, item }
  isDeleteModalActive.value = true
}

const deleteItem = async () => {
  try {
    if (deleteTarget.value.type === 'service') {
      await ApiService.apiGatewayDeleteService({ id: deleteTarget.value.item.id })
    } else if (deleteTarget.value.type === 'route') {
      await ApiService.apiGatewayDeleteRoute({ id: deleteTarget.value.item.id })
    } else if (deleteTarget.value.type === 'upstream') {
      await ApiService.apiGatewayDeleteUpstream({ id: deleteTarget.value.item.id })
    }
    isDeleteModalActive.value = false
    await loadData()
  } catch (error) {
    console.error('Failed to delete:', error)
  }
}

const buildUpstreamPayload = (form) => {
  // FormControl wraps select values as {value,label}; unwrap to the plain
  // string the backend expects.
  const unwrap = (v) => (v && typeof v === 'object' && 'value' in v) ? v.value : v
  const payload = {
    id: form.id,
    name: form.name,
    strategy: unwrap(form.strategy) || 'round_robin',
    enabled: form.enabled !== false,
    targets: (form.targets || [])
      .map(t => ({
        service_id: unwrap(t.service_id) || '',
        weight: Number.isFinite(Number(t.weight)) && Number(t.weight) > 0 ? Math.round(Number(t.weight)) : 1
      }))
      .filter(t => t.service_id)
  }
  if (form.sticky_enabled && form.sticky?.mode) {
    payload.sticky = {
      mode: unwrap(form.sticky.mode),
      cookie_name: form.sticky.cookie_name || '',
      header_name: form.sticky.header_name || '',
      ttl_seconds: Number.isFinite(Number(form.sticky.ttl_seconds)) ? Math.max(0, Math.round(Number(form.sticky.ttl_seconds))) : 0
    }
  }
  return payload
}

const openAddUpstreamModal = () => {
  newUpstream.value = createDefaultUpstream()
  isAddUpstreamModalActive.value = true
}

const addUpstream = async () => {
  try {
    const response = await ApiService.apiGatewayAddUpstream(buildUpstreamPayload(newUpstream.value))
    if (isSuccessfulResponse(response)) {
      isAddUpstreamModalActive.value = false
      await loadData()
    }
  } catch (error) {
    console.error('Failed to add upstream:', error)
  }
}

const openEditUpstreamModal = (upstream) => {
  editingUpstream.value = {
    id: upstream.id,
    name: upstream.name || '',
    strategy: findOption(strategyOptions, upstream.strategy || 'round_robin'),
    enabled: upstream.enabled !== false,
    sticky_enabled: !!upstream.sticky,
    sticky: upstream.sticky
      ? {
          mode: findOption(stickyModeOptions, upstream.sticky.mode || 'ip_hash'),
          cookie_name: upstream.sticky.cookie_name || 'redock_lb',
          header_name: upstream.sticky.header_name || '',
          ttl_seconds: upstream.sticky.ttl_seconds || 0
        }
      : {
          mode: findOption(stickyModeOptions, 'ip_hash'),
          cookie_name: 'redock_lb',
          header_name: '',
          ttl_seconds: 0
        },
    targets: Array.isArray(upstream.targets) && upstream.targets.length
      ? upstream.targets.map(t => ({ service_id: findServiceOption(t.service_id), weight: t.weight || 1 }))
      : [{ service_id: '', weight: 1 }]
  }
  isEditUpstreamModalActive.value = true
}

const updateUpstream = async () => {
  try {
    const response = await ApiService.apiGatewayUpdateUpstream(buildUpstreamPayload(editingUpstream.value))
    if (isSuccessfulResponse(response)) {
      isEditUpstreamModalActive.value = false
      await loadData()
    }
  } catch (error) {
    console.error('Failed to update upstream:', error)
  }
}

const addUpstreamTarget = (form) => {
  if (!Array.isArray(form.targets)) form.targets = []
  form.targets.push({ service_id: '', weight: 1 })
}

const removeUpstreamTarget = (form, idx) => {
  form.targets.splice(idx, 1)
  if (form.targets.length === 0) form.targets.push({ service_id: '', weight: 1 })
}

const requestCertificate = async () => {
  try {
    await ApiService.apiGatewayRequestCertificate()
    toast.success(t('gw.certIssued'))
    await loadData()
  } catch (error) {
    const msg = error.response?.data?.msg || error.message || 'Failed to issue certificate'
    toast.error(t('gw.certError') + msg)
    console.error('Failed to request certificate:', error)
  }
}

const toggleRenewer = async () => {
  try {
    if (renewerStatus.value.running) {
      await ApiService.apiGatewayStopRenewer()
    } else {
      await ApiService.apiGatewayStartRenewer()
    }
    await loadData()
  } catch (error) {
    console.error('Failed to toggle renewer:', error)
  }
}

const openConfigModal = async () => {
  try {
    const res = await ApiService.apiGatewayGetConfig()
    const cfg = res.data.data || {}
    const clientSecurity = {
      ...createDefaultClientSecurityConfig(),
      ...(cfg.client_security || {})
    }
    if (!clientSecurity.top_client_limit || clientSecurity.top_client_limit < 1) {
      clientSecurity.top_client_limit = 1000
    }
    const le = cfg.lets_encrypt || {}
    gatewayConfig.value = {
      http_port: cfg.http_port || 80,
      https_port: cfg.https_port || 443,
      https_enabled: cfg.https_enabled || false,
      access_log_enabled: cfg.access_log_enabled !== false,
      client_security: clientSecurity,
      timeouts: { ...createDefaultTimeoutsConfig(), ...(cfg.timeouts || {}) },
      lets_encrypt: {
        enabled: le.enabled || false,
        email: le.email || '',
        auto_renew: le.auto_renew !== false,
        renew_before_days: le.renew_before_days || 30
      }
    }
    isConfigModalActive.value = true
  } catch (error) {
    console.error('Failed to load config:', error)
  }
}

const saveConfig = async () => {
  try {
    const currentConfig = (await ApiService.apiGatewayGetConfig()).data.data || {}
    const clientSecurityPayload = {
      ...(currentConfig.client_security || {}),
      ...((gatewayConfig.value && gatewayConfig.value.client_security) || {})
    }
    const rawLimit = Number(clientSecurityPayload.top_client_limit)
    const boundedLimit = Math.min(1000, Math.max(1, Number.isFinite(rawLimit) ? rawLimit : 1000))
    clientSecurityPayload.top_client_limit = Math.round(boundedLimit)
    const timeoutsPayload = {
      ...(currentConfig.timeouts || {}),
      ...((gatewayConfig.value && gatewayConfig.value.timeouts) || {})
    }
    Object.keys(timeoutsPayload).forEach(k => {
      const n = Number(timeoutsPayload[k])
      timeoutsPayload[k] = Number.isFinite(n) && n >= 0 ? Math.round(n) : 0
    })
    // Merge Let's Encrypt: keep runtime fields (domains, expiry, cert state) from
    // the existing config, override only the account settings edited here.
    const leForm = (gatewayConfig.value && gatewayConfig.value.lets_encrypt) || {}
    const letsEncryptPayload = {
      ...(currentConfig.lets_encrypt || {}),
      enabled: !!leForm.enabled,
      email: (leForm.email || '').trim(),
      auto_renew: leForm.auto_renew !== false,
      renew_before_days: Math.max(1, Number(leForm.renew_before_days) || 30)
    }
    const updatedConfig = {
      ...currentConfig,
      ...gatewayConfig.value,
      client_security: clientSecurityPayload,
      timeouts: timeoutsPayload,
      lets_encrypt: letsEncryptPayload
    }
    await ApiService.apiGatewayUpdateConfig(updatedConfig)
    isConfigModalActive.value = false
    await loadData()
  } catch (error) {
    console.error('Failed to save config:', error)
  }
}

const openObservabilityModal = async () => {
  try {
    const res = await ApiService.apiGatewayGetObservabilityStatus()
    const cfg = res.data.data?.config || {}
    observabilityConfig.value = normalizeObservabilityConfig(cfg)
    isObservabilityModalActive.value = true
  } catch (error) {
    console.error('Failed to load observability config:', error)
  }
}

const saveObservability = async () => {
  try {
    const payload = JSON.parse(JSON.stringify(observabilityConfig.value))
    await ApiService.apiGatewayConfigureObservability(payload)
    isObservabilityModalActive.value = false
    await loadData()
  } catch (error) {
    console.error('Failed to save observability config:', error)
  }
}

const getServiceHealth = (serviceId) => {
  const health = serviceHealth.value.find(h => h.service_id === serviceId)
  return health ? health.healthy : null
}

const getServiceName = (serviceId) => {
  if (!serviceId) return 'Unknown Service'
  return serviceNameMap.value[serviceId] || serviceId
}

const getRouteName = (routeId) => {
  if (!routeId) return '-'
  return routeMap.value[routeId]?.name || routeId
}

const getRouteServiceName = (routeId) => {
  const route = routeMap.value[routeId]
  if (!route) return '-'
  return getUpstreamName(route.upstream_id)
}

const getUpstreamName = (upstreamId) => {
  if (!upstreamId) return 'Unknown Upstream'
  const u = upstreams.value.find(x => x.id === upstreamId)
  return u ? (u.name || u.id) : upstreamId
}

const getHealthColor = (healthy) => {
  if (healthy === null) return 'text-gray-500'
  return healthy ? 'text-green-500' : 'text-red-500'
}

const formatDateTime = (value) => {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  return date.toLocaleString()
}

const submitManualBlock = async () => {
  if (!manualBlockForm.value.ip) {
    return
  }
  try {
    await ApiService.apiGatewayBlockClient({
      ip: manualBlockForm.value.ip.trim(),
      duration_seconds: Number(manualBlockForm.value.duration_seconds) || 0,
      reason: manualBlockForm.value.reason
    })
    manualBlockForm.value.ip = ''
    manualBlockForm.value.reason = ''
    await loadData()
  } catch (error) {
    console.error('Failed to block client:', error)
  }
}

const unblockClient = async (ip) => {
  if (!ip) return
  try {
    await ApiService.apiGatewayUnblockClient({ ip })
    await loadData()
  } catch (error) {
    console.error('Failed to unblock client:', error)
  }
}

const quickBlockClient = async (ip) => {
  if (!ip) return
  try {
    await ApiService.apiGatewayBlockClient({
      ip,
      duration_seconds: Number(manualBlockForm.value.duration_seconds) || 0,
      reason: 'Blocked from dashboard'
    })
    await loadData()
  } catch (error) {
    console.error('Failed to quick block client:', error)
  }
}

// Auto-refresh
let refreshInterval = null

onMounted(() => {
  loadData()
  refreshInterval = setInterval(loadData, 10000) // Refresh every 10 seconds
})

onUnmounted(() => {
  if (refreshInterval) {
    clearInterval(refreshInterval)
  }
})
</script>

<template>
  <div class="space-y-8">
    <!-- Header -->
    <div class="bg-gradient-to-r from-purple-600 via-indigo-600 to-blue-600 rounded-2xl p-8 text-white shadow-lg">
      <div class="flex flex-col lg:flex-row lg:items-center lg:justify-between">
        <div>
          <h1 class="text-3xl lg:text-4xl font-bold mb-2 flex items-center">
            <BaseIcon :path="mdiRouter" size="40" class="mr-4" />
            {{ t('gw.title') }}
          </h1>
          <p class="text-blue-100 text-lg">{{ t('gw.subtitle') }}</p>
        </div>
        <div class="mt-6 lg:mt-0 flex flex-wrap gap-3">
          <BaseButton
            v-if="!status.running"
            :label="t('gw.startGateway')"
            :icon="mdiPlay"
            color="white"
            class="shadow-lg hover:shadow-xl transform hover:scale-105 transition-all duration-200"
            @click="startGateway"
          />
          <BaseButton
            v-else
            :label="t('gw.stopGateway')"
            :icon="mdiStop"
            color="danger"
            class="shadow-lg hover:shadow-xl transform hover:scale-105 transition-all duration-200"
            @click="stopGateway"
          />
          <BaseButton
            :label="t('common.settings')"
            :icon="mdiCog"
            color="white"
            outline
            class="shadow-lg hover:shadow-xl"
            @click="openConfigModal"
          />
          <BaseButton
            :icon="mdiRefresh"
            color="white"
            outline
            :disabled="loading"
            class="shadow-lg"
            @click="loadData"
          />
        </div>
      </div>
      
      <!-- Status Badge -->
      <div class="mt-4 flex items-center gap-4">
        <span 
          :class="[
            'inline-flex items-center px-4 py-2 rounded-full text-sm font-medium',
            status.running 
              ? 'bg-green-500/20 text-green-100 border border-green-400/30' 
              : 'bg-gray-500/20 text-gray-100 border border-gray-400/30'
          ]"
        >
          <span :class="['w-2 h-2 rounded-full mr-2', status.running ? 'bg-green-400' : 'bg-gray-400']"></span>
          {{ status.running ? t('common.running') : t('common.stopped') }}
        </span>
        <span v-if="status.running" class="text-blue-100 text-sm">
          HTTP: {{ status.http_port }} | HTTPS: {{ status.https_enabled ? status.https_port : t('common.disabled') }}
        </span>
      </div>
    </div>

    <!-- Statistics -->
    <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
      <CardBox class="bg-gradient-to-br from-blue-50 to-blue-100 dark:from-blue-900/20 dark:to-blue-800/20 border-blue-200 dark:border-blue-700">
        <div class="flex items-center justify-between">
          <div>
            <div class="text-2xl font-bold text-blue-600 dark:text-blue-400">{{ gatewayStats.totalServices }}</div>
            <div class="text-sm text-blue-600/70">{{ t('gw.statServices') }}</div>
          </div>
          <BaseIcon :path="mdiServer" size="36" class="text-blue-500 opacity-20" />
        </div>
      </CardBox>

      <CardBox class="bg-gradient-to-br from-green-50 to-green-100 dark:from-green-900/20 dark:to-green-800/20 border-green-200 dark:border-green-700">
        <div class="flex items-center justify-between">
          <div>
            <div class="text-2xl font-bold text-green-600 dark:text-green-400">{{ gatewayStats.healthyServices }}/{{ gatewayStats.totalServices }}</div>
            <div class="text-sm text-green-600/70">{{ t('gw.statHealthy') }}</div>
          </div>
          <BaseIcon :path="mdiHeartPulse" size="36" class="text-green-500 opacity-20" />
        </div>
      </CardBox>

      <CardBox class="bg-gradient-to-br from-purple-50 to-purple-100 dark:from-purple-900/20 dark:to-purple-800/20 border-purple-200 dark:border-purple-700">
        <div class="flex items-center justify-between">
          <div>
            <div class="text-2xl font-bold text-purple-600 dark:text-purple-400">{{ gatewayStats.totalRoutes }}</div>
            <div class="text-sm text-purple-600/70">{{ t('gw.statRoutes') }}</div>
          </div>
          <BaseIcon :path="mdiWeb" size="36" class="text-purple-500 opacity-20" />
        </div>
      </CardBox>

      <CardBox class="bg-gradient-to-br from-orange-50 to-orange-100 dark:from-orange-900/20 dark:to-orange-800/20 border-orange-200 dark:border-orange-700">
        <div class="flex items-center justify-between">
          <div>
            <div class="text-2xl font-bold text-orange-600 dark:text-orange-400">{{ gatewayStats.totalRequests }}</div>
            <div class="text-sm text-orange-600/70">{{ t('gw.statRequests') }}</div>
          </div>
          <BaseIcon :path="mdiSpeedometer" size="36" class="text-orange-500 opacity-20" />
        </div>
      </CardBox>
    </div>

    <!-- Tabs (responsive: horizontal scroll on small screens) -->
    <div class="overflow-x-auto pb-px -mx-1 px-1">
      <div class="flex flex-nowrap gap-1 sm:gap-2 border-b border-gray-200 dark:border-gray-700">
        <button
          v-for="tab in ['overview', 'services', 'upstreams', 'routes', 'clients', 'certificates', 'observability']"
          :key="tab"
          :class="[
            'shrink-0 whitespace-nowrap px-4 sm:px-6 py-3 font-medium text-sm border-b-2 transition-colors capitalize',
            activeTab === tab
              ? 'border-blue-500 text-blue-600 dark:text-blue-400'
              : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'
          ]"
          @click="activeTab = tab"
        >
          {{ t('gw.tabs.' + tab) }}
        </button>
      </div>
    </div>

    <!-- Overview Tab -->
    <div v-if="activeTab === 'overview'" class="grid grid-cols-1 lg:grid-cols-2 gap-6">
      <!-- Performance Stats -->
      <CardBox>
        <SectionTitleLineWithButton :icon="mdiSpeedometer" :title="t('gw.performance')" main />
        <div class="grid grid-cols-2 gap-4 mt-4">
          <div class="p-4 bg-slate-50 dark:bg-slate-800/50 rounded-lg">
            <div class="text-2xl font-bold text-slate-800 dark:text-slate-200">{{ gatewayStats.avgLatency }}ms</div>
            <div class="text-sm text-slate-500">{{ t('gw.avgLatency') }}</div>
          </div>
          <div class="p-4 bg-slate-50 dark:bg-slate-800/50 rounded-lg">
            <div class="text-2xl font-bold text-slate-800 dark:text-slate-200">{{ gatewayStats.uptime }}</div>
            <div class="text-sm text-slate-500">{{ t('gw.uptime') }}</div>
          </div>
          <div class="p-4 bg-slate-50 dark:bg-slate-800/50 rounded-lg">
            <div class="text-2xl font-bold text-slate-800 dark:text-slate-200">{{ gatewayStats.totalRequests }}</div>
            <div class="text-sm text-slate-500">{{ t('gw.totalRequests') }}</div>
          </div>
          <div class="p-4 bg-slate-50 dark:bg-slate-800/50 rounded-lg">
            <div class="text-2xl font-bold text-red-600 dark:text-red-400">{{ gatewayStats.totalErrors }}</div>
            <div class="text-sm text-slate-500">{{ t('gw.totalErrors') }}</div>
          </div>
        </div>
      </CardBox>

      <!-- Service Health -->
      <CardBox>
        <SectionTitleLineWithButton :icon="mdiHeartPulse" :title="t('gw.serviceHealth')" main />
        <div class="space-y-3 mt-4">
          <div v-if="serviceHealth.length === 0" class="text-center py-8 text-slate-500">
            {{ t('gw.noHealthData') }}
          </div>
          <div
            v-for="health in serviceHealth"
            :key="health.service_id"
            class="flex items-center justify-between p-3 bg-slate-50 dark:bg-slate-800/50 rounded-lg"
          >
            <div class="flex items-center gap-3">
              <BaseIcon 
                :path="health.healthy ? mdiHeart : mdiHeartPulse" 
                :class="health.healthy ? 'text-green-500' : 'text-red-500'"
                size="20" 
              />
              <span class="font-medium">{{ getServiceName(health.service_id) }}</span>
            </div>
            <div class="flex items-center gap-4 text-sm text-slate-500">
              <span>{{ health.response_time_ms }}ms</span>
              <span :class="health.healthy ? 'text-green-500' : 'text-red-500'">
                {{ health.healthy ? t('gw.healthy') : t('gw.unhealthy') }}
              </span>
            </div>
          </div>
        </div>
      </CardBox>
    </div>

    <!-- Services Tab -->
    <CardBox v-if="activeTab === 'services'">
      <SectionTitleLineWithButton :icon="mdiServer" :title="t('gw.upstreamServices')" main>
        <BaseButton
          :label="t('gw.addService')"
          :icon="mdiPlus"
          color="info"
          small
          @click="openAddServiceModal"
        />
      </SectionTitleLineWithButton>

      <div v-if="services.length > 0" class="mt-3 flex flex-wrap gap-3 items-center">
        <div class="flex-1 min-w-[200px]">
          <FormControl
            v-model="serviceSearch"
            :placeholder="t('gw.serviceSearchPlaceholder')"
            :icon="mdiMagnify"
          />
        </div>
        <div class="inline-flex rounded-lg overflow-hidden border border-slate-200 dark:border-slate-700 text-sm">
          <button
            v-for="opt in [
              { value: 'all',      label: t('gw.all') },
              { value: 'enabled',  label: t('gw.active') },
              { value: 'disabled', label: t('common.disabled') }
            ]"
            :key="opt.value"
            type="button"
            class="px-3 py-1.5 transition-colors"
            :class="serviceStatusFilter === opt.value
              ? 'bg-blue-500 text-white'
              : 'bg-white dark:bg-slate-800 text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700'"
            @click="serviceStatusFilter = opt.value"
          >
            {{ opt.label }}
          </button>
        </div>
        <span class="text-xs text-slate-500">{{ filteredServices.length }} / {{ services.length }}</span>
      </div>

      <div v-if="services.length === 0" class="text-center py-12">
        <BaseIcon :path="mdiServer" size="64" class="mx-auto text-slate-300 dark:text-slate-600 mb-4" />
        <p class="text-slate-500 mb-4">{{ t('gw.noServices') }}</p>
        <BaseButton :label="t('gw.addFirstService')" :icon="mdiPlus" color="info" @click="openAddServiceModal" />
      </div>

      <div v-else-if="filteredServices.length === 0" class="text-center py-12 text-slate-500">
        {{ t('gw.noServicesMatch') }}
      </div>

      <div v-else class="grid grid-cols-1 md:grid-cols-2 gap-4 mt-4">
        <div
          v-for="service in filteredServices"
          :key="service.id"
          class="p-4 bg-slate-50 dark:bg-slate-800/50 rounded-xl"
        >
          <div class="flex items-start justify-between">
            <div class="flex items-center gap-3">
              <div class="w-10 h-10 bg-blue-500 rounded-lg flex items-center justify-center">
                <BaseIcon :path="mdiServer" size="20" class="text-white" />
              </div>
              <div>
                <h3 class="font-semibold">{{ service.name }}</h3>
                <p class="text-sm text-slate-500">{{ service.host }}:{{ service.port }}</p>
              </div>
            </div>
            <BaseIcon 
              :path="mdiHeart" 
              :class="getHealthColor(getServiceHealth(service.id))"
              size="20" 
            />
          </div>
          <div class="mt-4 flex items-center justify-between">
            <div class="flex items-center gap-2 text-sm text-slate-500">
              <span class="px-2 py-1 bg-slate-200 dark:bg-slate-700 rounded">{{ service.protocol }}</span>
              <span>{{ t('gw.timeout') }}: {{ service.timeout }}s</span>
            </div>
            <div class="flex gap-2">
              <BaseButton :icon="mdiPencil" color="info" small @click="openEditServiceModal(service)" />
              <BaseButton :icon="mdiDelete" color="danger" small @click="confirmDelete('service', service)" />
            </div>
          </div>
        </div>
      </div>
    </CardBox>

    <!-- Upstreams Tab -->
    <CardBox v-if="activeTab === 'upstreams'">
      <SectionTitleLineWithButton :icon="mdiSourceBranch" :title="t('gw.upstreamPools')" main>
        <BaseButton
          :label="t('gw.addUpstream')"
          :icon="mdiPlus"
          color="info"
          small
          @click="openAddUpstreamModal"
        />
      </SectionTitleLineWithButton>

      <p class="text-xs text-slate-500 mt-2">
        {{ t('gw.upstreamDesc') }}
        
      </p>

      <div v-if="upstreams.length > 0" class="mt-3 flex flex-wrap gap-3 items-center">
        <div class="flex-1 min-w-[200px]">
          <FormControl
            v-model="upstreamSearch"
            :placeholder="t('gw.upstreamSearchPlaceholder')"
            :icon="mdiMagnify"
          />
        </div>
        <div class="inline-flex rounded-lg overflow-hidden border border-slate-200 dark:border-slate-700 text-sm">
          <button
            v-for="opt in [
              { value: 'all',      label: t('gw.all') },
              { value: 'enabled',  label: t('gw.active') },
              { value: 'disabled', label: t('common.disabled') }
            ]"
            :key="opt.value"
            type="button"
            class="px-3 py-1.5 transition-colors"
            :class="upstreamStatusFilter === opt.value
              ? 'bg-blue-500 text-white'
              : 'bg-white dark:bg-slate-800 text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700'"
            @click="upstreamStatusFilter = opt.value"
          >
            {{ opt.label }}
          </button>
        </div>
        <span class="text-xs text-slate-500">{{ filteredUpstreams.length }} / {{ upstreams.length }}</span>
      </div>

      <div v-if="upstreams.length === 0" class="text-center py-12">
        <BaseIcon :path="mdiSourceBranch" size="64" class="mx-auto text-slate-300 dark:text-slate-600 mb-4" />
        <p class="text-slate-500 mb-4">{{ t('gw.noUpstreams') }}</p>
        <BaseButton :label="t('gw.addFirstUpstream')" :icon="mdiPlus" color="info" @click="openAddUpstreamModal" />
      </div>

      <div v-else-if="filteredUpstreams.length === 0" class="text-center py-12 text-slate-500">
        {{ t('gw.noUpstreamsMatch') }}
      </div>

      <div v-else class="grid grid-cols-1 lg:grid-cols-2 gap-4 mt-4">
        <div
          v-for="upstream in filteredUpstreams"
          :key="upstream.id"
          class="p-4 bg-slate-50 dark:bg-slate-800/50 rounded-xl"
        >
          <div class="flex items-start justify-between">
            <div>
              <h3 class="font-semibold">{{ upstream.name || upstream.id }}</h3>
              <p class="text-xs text-slate-500 break-all">{{ upstream.id }}</p>
            </div>
            <span
              class="px-2 py-0.5 text-xs rounded"
              :class="upstream.enabled
                ? 'bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-400'
                : 'bg-slate-200 dark:bg-slate-700 text-slate-500'"
            >
              {{ upstream.enabled ? t('common.enabled') : t('common.disabled') }}
            </span>
          </div>
          <div class="flex flex-wrap gap-2 mt-3 text-xs">
            <span class="px-2 py-1 bg-slate-200 dark:bg-slate-700 rounded">{{ upstream.strategy || 'round_robin' }}</span>
            <span v-if="upstream.sticky" class="px-2 py-1 bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400 rounded">
              sticky: {{ upstream.sticky.mode }}
            </span>
            <span class="px-2 py-1 bg-slate-200 dark:bg-slate-700 rounded">
              {{ (upstream.targets || []).length }} {{ t('gw.targetsLabel') }}
            </span>
          </div>
          <div class="mt-3 text-xs text-slate-600 dark:text-slate-400 space-y-1">
            <div v-for="tgt in (upstream.targets || [])" :key="tgt.service_id" class="flex justify-between">
              <span>{{ getServiceName(tgt.service_id) }}</span>
              <span class="text-slate-400">{{ t('gw.weight') }} {{ tgt.weight || 1 }}</span>
            </div>
          </div>
          <div class="mt-3 flex justify-end gap-2">
            <BaseButton :icon="mdiPencil" color="info" small @click="openEditUpstreamModal(upstream)" />
            <BaseButton :icon="mdiDelete" color="danger" small @click="confirmDelete('upstream', upstream)" />
          </div>
        </div>
      </div>
    </CardBox>

    <!-- Clients Tab -->
    <div v-if="activeTab === 'clients'" class="grid grid-cols-1 lg:grid-cols-2 gap-6">
      <CardBox>
        <SectionTitleLineWithButton :icon="mdiShield" :title="t('gw.blockedClients')" main />
        <div v-if="blockedClients.length === 0" class="text-center py-10 text-slate-500">
          {{ t('gw.noBlockedClients') }}
        </div>
        <div v-else class="space-y-3 mt-4">
          <div
            v-for="client in blockedClients"
            :key="client.ip"
            class="p-4 bg-slate-50 dark:bg-slate-800/50 rounded-lg flex items-center justify-between"
          >
            <div>
              <div class="font-semibold">{{ client.ip }}</div>
              <div class="text-xs text-slate-500">
                {{ t('gw.reason') }}: {{ client.reason || 'N/A' }}
              </div>
              <div class="text-xs text-slate-400">
                {{ client.manual ? t('gw.manualBlock') : t('gw.autoBlock') }} •
                {{ client.blocked_until ? t('gw.until') + ' ' + formatDateTime(client.blocked_until) : t('gw.indefinite') }}
              </div>
            </div>
            <BaseButton :label="t('gw.unblock')" color="warning" small outline @click="unblockClient(client.ip)" />
          </div>
        </div>
      </CardBox>

      <CardBox>
        <SectionTitleLineWithButton :icon="mdiShield" :title="t('gw.manualBlockTitle')" main />
        <form class="space-y-4 mt-4" @submit.prevent="submitManualBlock">
          <FormField :label="t('gw.clientIp')">
            <FormControl
              v-model="manualBlockForm.ip"
              placeholder="e.g. 192.168.1.10"
              required
            />
          </FormField>
          <FormField :label="t('gw.durationSeconds')">
            <FormControl
              v-model.number="manualBlockForm.duration_seconds"
              type="number"
              min="0"
              :placeholder="t('gw.zeroForIndefinite')"
            />
          </FormField>
          <FormField :label="t('gw.reason')">
            <FormControl v-model="manualBlockForm.reason" :placeholder="t('gw.reasonPlaceholder')" />
          </FormField>
          <BaseButton type="submit" :label="t('gw.blockClient')" color="danger" :icon="mdiShield" />
        </form>
      </CardBox>

      <CardBox class="lg:col-span-2">
        <SectionTitleLineWithButton :icon="mdiShield" :title="t('gw.topClients')" main />
        <div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between mt-4">
          <p class="text-sm text-slate-500">
            {{ t('gw.showingClients', { shown: topClients.length, total: allTopClients.length }) }}
          </p>
          <label class="text-sm text-slate-500 flex items-center gap-2">
            <span>{{ t('gw.show') }}</span>
            <select
              v-model.number="topClientDisplayLimit"
              class="border border-slate-200 dark:border-slate-700 rounded-lg px-3 py-1 bg-white dark:bg-slate-800 text-slate-700 dark:text-slate-200"
            >
              <option
                v-for="option in topClientDisplayOptions"
                :key="option"
                :value="option"
              >
                {{ option }}
              </option>
            </select>
          </label>
        </div>
        <div v-if="topClients.length === 0" class="text-center py-10 text-slate-500">
          {{ t('gw.noTraffic') }}
        </div>
        <div v-else class="mt-4 space-y-4">
          <div class="grid gap-3 md:hidden">
            <div
              v-for="client in topClients"
              :key="`mobile-${client.ip}`"
              class="p-4 bg-slate-50 dark:bg-slate-800/50 rounded-xl shadow-sm"
            >
              <div class="flex items-start justify-between gap-3">
                <div>
                  <div class="font-semibold text-slate-800 dark:text-slate-100">{{ client.ip }}</div>
                  <div class="text-xs text-slate-500">{{ t('gw.lastSeen') }} {{ formatDateTime(client.last_seen) }}</div>
                </div>
                <span
                  :class="[
                    'px-2 py-1 text-xs rounded-full font-semibold',
                    client.blocked
                      ? 'bg-red-100 text-red-700 dark:bg-red-500/20 dark:text-red-200'
                      : 'bg-green-100 text-green-700 dark:bg-green-500/20 dark:text-green-200'
                  ]"
                >
                  {{ client.blocked ? t('gw.blocked') : t('gw.active') }}
                </span>
              </div>
              <div class="grid grid-cols-2 gap-3 mt-4 text-sm text-slate-600 dark:text-slate-300">
                <div>
                  <p class="text-xs uppercase tracking-wide text-slate-500">{{ t('gw.requests') }}</p>
                  <p class="font-semibold">{{ client.request_count }}</p>
                </div>
                <div>
                  <p class="text-xs uppercase tracking-wide text-slate-500">{{ t('gw.lastPath') }}</p>
                  <p class="truncate">{{ client.last_path || '-' }}</p>
                </div>
                <div>
                  <p class="text-xs uppercase tracking-wide text-slate-500">{{ t('gw.route') }}</p>
                  <p class="font-medium">{{ getRouteName(client.last_route_id) }}</p>
                </div>
                <div>
                  <p class="text-xs uppercase tracking-wide text-slate-500">{{ t('gw.service') }}</p>
                  <p class="font-medium">{{ getRouteServiceName(client.last_route_id) }}</p>
                </div>
              </div>
              <div class="mt-4 flex justify-end">
                <BaseButton
                  v-if="client.blocked"
                  :label="t('gw.unblock')"
                  color="warning"
                  small
                  outline
                  @click="unblockClient(client.ip)"
                />
                <BaseButton
                  v-else
                  :label="t('gw.block')"
                  color="danger"
                  small
                  outline
                  @click="quickBlockClient(client.ip)"
                />
              </div>
            </div>
          </div>

          <div class="hidden md:block overflow-x-auto">
            <table class="min-w-full text-sm">
              <thead>
                <tr class="text-left text-slate-500 border-b border-slate-100 dark:border-slate-700">
                  <th class="py-3">{{ t('gw.clientIp') }}</th>
                  <th class="py-3">{{ t('gw.requests') }}</th>
                  <th class="py-3">{{ t('gw.lastSeen') }}</th>
                  <th class="py-3">{{ t('gw.lastPath') }}</th>
                  <th class="py-3">{{ t('gw.route') }}</th>
                  <th class="py-3">{{ t('gw.service') }}</th>
                  <th class="py-3">{{ t('common.status') }}</th>
                  <th class="py-3 text-right">{{ t('gw.action') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="client in topClients" :key="client.ip" class="border-b border-slate-100 dark:border-slate-800">
                  <td class="py-3 font-medium">{{ client.ip }}</td>
                  <td class="py-3">{{ client.request_count }}</td>
                  <td class="py-3">{{ formatDateTime(client.last_seen) }}</td>
                  <td class="py-3 truncate max-w-xs">{{ client.last_path || '-' }}</td>
                  <td class="py-3">{{ getRouteName(client.last_route_id) }}</td>
                  <td class="py-3">{{ getRouteServiceName(client.last_route_id) }}</td>
                  <td class="py-3">
                    <span
                      :class="[
                        'px-2 py-1 text-xs rounded-full font-semibold',
                        client.blocked
                          ? 'bg-red-100 text-red-700 dark:bg-red-500/20 dark:text-red-200'
                          : 'bg-green-100 text-green-700 dark:bg-green-500/20 dark:text-green-200'
                      ]"
                    >
                      {{ client.blocked ? t('gw.blocked') : t('gw.active') }}
                    </span>
                  </td>
                  <td class="py-3 text-right">
                    <BaseButton
                      v-if="client.blocked"
                      :label="t('gw.unblock')"
                      color="warning"
                      small
                      outline
                      @click="unblockClient(client.ip)"
                    />
                    <BaseButton
                      v-else
                      :label="t('gw.block')"
                      color="danger"
                      small
                      outline
                      @click="quickBlockClient(client.ip)"
                    />
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </CardBox>
    </div>

    <!-- Routes Tab -->
    <CardBox v-if="activeTab === 'routes'">
      <SectionTitleLineWithButton :icon="mdiWeb" :title="t('gw.routingRules')" main>
        <BaseButton
          :label="t('gw.addRoute')"
          :icon="mdiPlus"
          color="info"
          small
          @click="openAddRouteModal"
        />
      </SectionTitleLineWithButton>

      <!-- Filter bar -->
      <div v-if="routes.length > 0" class="mt-3 flex flex-wrap gap-3 items-center">
        <div class="flex-1 min-w-[200px]">
          <FormControl
            v-model="routeSearch"
            :placeholder="t('gw.routeSearchPlaceholder')"
            :icon="mdiMagnify"
          />
        </div>
        <div class="inline-flex rounded-lg overflow-hidden border border-slate-200 dark:border-slate-700 text-sm">
          <button
            v-for="opt in [
              { value: 'all',      label: t('gw.all') },
              { value: 'enabled',  label: t('gw.active') },
              { value: 'disabled', label: t('common.disabled') }
            ]"
            :key="opt.value"
            type="button"
            class="px-3 py-1.5 transition-colors"
            :class="routeStatusFilter === opt.value
              ? 'bg-blue-500 text-white'
              : 'bg-white dark:bg-slate-800 text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700'"
            @click="routeStatusFilter = opt.value"
          >
            {{ opt.label }}
          </button>
        </div>
        <span class="text-xs text-slate-500">
          {{ filteredRoutes.length }} / {{ routes.length }}
        </span>
      </div>

      <div v-if="routes.length === 0" class="text-center py-12">
        <BaseIcon :path="mdiWeb" size="64" class="mx-auto text-slate-300 dark:text-slate-600 mb-4" />
        <p class="text-slate-500 mb-4">{{ t('gw.noRoutes') }}</p>
        <BaseButton :label="t('gw.addFirstRoute')" :icon="mdiPlus" color="info" @click="openAddRouteModal" />
      </div>

      <div v-else-if="filteredRoutes.length === 0" class="text-center py-12 text-slate-500">
        {{ t('gw.noRoutesMatch') }}
      </div>

      <div v-else class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4 mt-4">
        <div
          v-for="route in filteredRoutes"
          :key="route.id"
          class="p-4 bg-slate-50 dark:bg-slate-800/50 rounded-xl flex flex-col"
        >
          <div class="flex items-start justify-between gap-2">
            <div class="min-w-0 flex-1">
              <h3 class="font-semibold flex items-center gap-2 flex-wrap">
                <span class="truncate">{{ route.name || route.id }}</span>
                <span
                  :class="[
                    'px-2 py-0.5 text-xs rounded shrink-0',
                    route.enabled ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400' : 'bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-400'
                  ]"
                >
                  {{ route.enabled ? t('gw.active') : t('common.disabled') }}
                </span>
              </h3>
              <p class="text-sm text-slate-500 mt-1 break-all">
                <span class="font-mono">{{ route.paths?.join(', ') }}</span>
              </p>
              <p class="text-xs text-slate-400 mt-1 truncate">
                → {{ getUpstreamName(route.upstream_id) }}
              </p>
              <p v-if="route.hosts?.length" class="text-xs text-slate-400 mt-1 truncate">
                {{ t('gw.hostsLabel') }}: {{ route.hosts.join(', ') }}
              </p>
            </div>
            <div class="flex gap-2 shrink-0">
              <BaseButton :icon="mdiPencil" color="info" small @click="openEditRouteModal(route)" />
              <BaseButton :icon="mdiDelete" color="danger" small @click="confirmDelete('route', route)" />
            </div>
          </div>
          <div class="mt-3 flex flex-wrap gap-2">
            <span v-if="route.rate_limit_enabled" class="px-2 py-1 bg-orange-100 dark:bg-orange-900/30 text-orange-700 dark:text-orange-300 text-xs rounded">
              {{ t('gw.rateLimited') }}
            </span>
            <span v-if="route.auth_required" class="px-2 py-1 bg-purple-100 dark:bg-purple-900/30 text-purple-700 dark:text-purple-300 text-xs rounded">
              {{ t('gw.authLabel') }}: {{ route.auth_type }}{{ route.auth_type === 'header' && (route.auth_headers?.length) ? ` (${route.auth_headers.length})` : '' }}
            </span>
            <span v-if="route.strip_path" class="px-2 py-1 bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300 text-xs rounded">
              {{ t('gw.stripPath') }}
            </span>
            <span v-if="route.timeout > 0" class="px-2 py-1 bg-slate-200 dark:bg-slate-700 text-slate-700 dark:text-slate-300 text-xs rounded">
              {{ t('gw.timeout') }} {{ route.timeout }}s
            </span>
          </div>
        </div>
      </div>
    </CardBox>

    <!-- Certificates Tab -->
    <CardBox v-if="activeTab === 'certificates'">
      <SectionTitleLineWithButton :icon="mdiCertificate" :title="t('gw.sslCerts')" main>
        <BaseButton
          :label="t('gw.gatewaySettings')"
          :icon="mdiCog"
          color="info"
          small
          @click="openConfigModal"
        />
      </SectionTitleLineWithButton>

      <div class="grid grid-cols-1 lg:grid-cols-2 gap-6 mt-4">
        <!-- {{ t('gw.currentCertificate') }} -->
        <div class="p-6 bg-slate-50 dark:bg-slate-800/50 rounded-xl">
          <h3 class="font-semibold mb-4 flex items-center gap-2">
            <BaseIcon :path="mdiShield" size="20" />
            {{ t('gw.currentCertificate') }}
          </h3>
          <div v-if="certificateInfo.cert_subject" class="space-y-3">
            <div class="flex justify-between">
              <span class="text-slate-500">{{ t('gw.commonName') }}</span>
              <span class="font-medium text-right break-all">{{ certificateInfo.cert_subject }}</span>
            </div>
            <div class="flex justify-between">
              <span class="text-slate-500">{{ t('gw.issuer') }}</span>
              <span class="font-medium text-right break-all">{{ certificateInfo.cert_issuer }}</span>
            </div>
            <div class="flex justify-between">
              <span class="text-slate-500">{{ t('gw.validFrom') }}</span>
              <span class="font-medium">{{ certificateInfo.cert_not_before }}</span>
            </div>
            <div class="flex justify-between">
              <span class="text-slate-500">{{ t('gw.validUntil') }}</span>
              <span class="font-medium">{{ certificateInfo.cert_not_after }}</span>
            </div>
            <div class="flex justify-between">
              <span class="text-slate-500">{{ t('common.status') }}</span>
              <span :class="certificateInfo.cert_valid ? 'text-green-500' : 'text-red-500'">
                {{ certificateInfo.cert_valid ? t('gw.valid') : t('gw.invalidExpired') }}
              </span>
            </div>
            <div v-if="certificateInfo.cert_dns_names?.length" class="space-y-1">
              <span class="text-slate-500">{{ t('gw.subjectAltNames') }}</span>
              <div class="flex flex-wrap gap-2 justify-end">
                <span
                  v-for="domain in certificateInfo.cert_dns_names"
                  :key="domain"
                  class="px-2 py-0.5 rounded bg-slate-200 dark:bg-slate-700 text-xs font-mono"
                >
                  {{ domain }}
                </span>
              </div>
            </div>
            <div
              v-if="certificateInfo.cert_file || certificateInfo.key_file"
              class="pt-3 border-t border-slate-200 dark:border-slate-700 space-y-1 text-xs"
            >
              <div class="flex justify-between">
                <span class="text-slate-500">{{ t('gw.certFile') }}</span>
                <span class="font-mono text-right break-all text-slate-700 dark:text-slate-200">
                  {{ certificateInfo.cert_file || 'N/A' }}
                </span>
              </div>
              <div class="flex justify-between">
                <span class="text-slate-500">{{ t('gw.keyFile') }}</span>
                <span class="font-mono text-right break-all text-slate-700 dark:text-slate-200">
                  {{ certificateInfo.key_file || 'N/A' }}
                </span>
              </div>
            </div>
          </div>
          <div v-else class="text-center py-8 text-slate-500">
            {{ t('gw.noCertificate') }}
          </div>
        </div>

        <!-- Let's Encrypt Status -->
        <div class="p-6 bg-slate-50 dark:bg-slate-800/50 rounded-xl">
          <h3 class="font-semibold mb-4 flex items-center gap-2">
            <BaseIcon :path="mdiSync" size="20" />
            {{ t('gw.autoRenewalStatus') }}
          </h3>
          <div class="space-y-4">
            <div class="flex items-center justify-between">
              <span class="text-slate-500">Let's Encrypt</span>
              <span :class="certificateInfo.lets_encrypt ? 'text-green-500' : 'text-gray-500'">
                {{ certificateInfo.lets_encrypt ? t('common.enabled') : t('common.disabled') }}
              </span>
            </div>
            <div class="flex items-center justify-between">
              <span class="text-slate-500">{{ t('gw.autoRenewScheduler') }}</span>
              <span :class="renewerStatus.running ? 'text-green-500' : 'text-gray-500'">
                {{ renewerStatus.running ? t('common.running') : t('common.stopped') }}
              </span>
            </div>
            <div v-if="certificateInfo.expires_at" class="flex items-center justify-between">
              <span class="text-slate-500">{{ t('gw.expires') }}</span>
              <span>{{ certificateInfo.expires_at }}</span>
            </div>
            <div v-if="certificateInfo.lets_encrypt_domains?.length" class="space-y-1">
              <span class="text-slate-500">{{ t('gw.domainsFromRoutes') }}</span>
              <div class="flex flex-wrap gap-2">
                <span
v-for="domain in certificateInfo.lets_encrypt_domains" :key="domain"
                      class="px-2 py-0.5 rounded bg-slate-200 dark:bg-slate-700 text-xs font-mono">
                  {{ domain }}
                </span>
              </div>
            </div>
            <div class="pt-4 flex gap-2">
              <BaseButton
                :label="renewerStatus.running ? t('gw.stopScheduler') : t('gw.startScheduler')"
                :icon="renewerStatus.running ? mdiStop : mdiPlay"
                :color="renewerStatus.running ? 'danger' : 'success'"
                small
                @click="toggleRenewer"
              />
              <BaseButton
                :label="t('gw.requestCertificate')"
                :icon="mdiCertificate"
                color="info"
                small
                @click="requestCertificate"
              />
            </div>
            <p v-if="!certificateInfo.lets_encrypt" class="mt-3 text-sm text-amber-600 dark:text-amber-400" v-html="t('gw.leHintNoLE')"></p>
            <p v-else-if="!certificateInfo.lets_encrypt_domains?.length" class="mt-3 text-sm text-amber-600 dark:text-amber-400" v-html="t('gw.leHintNoRoutes')"></p>
          </div>
        </div>
      </div>
    </CardBox>

    <!-- Observability Tab -->
    <CardBox v-if="activeTab === 'observability'">
      <SectionTitleLineWithButton :icon="mdiChartLine" :title="t('gw.observabilityTitle')" main>
        <BaseButton
          :label="t('gw.configure')"
          :icon="mdiCog"
          color="info"
          small
          @click="openObservabilityModal"
        />
      </SectionTitleLineWithButton>

      <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-5 gap-6 mt-4">
        <!-- Loki Status -->
        <div class="p-6 bg-slate-50 dark:bg-slate-800/50 rounded-xl">
          <h3 class="font-semibold mb-4 flex items-center gap-2">
            <BaseIcon :path="mdiChartLine" size="20" />
            Loki
          </h3>
          <div class="space-y-3">
            <div class="flex items-center justify-between">
              <span class="text-slate-500">{{ t('common.status') }}</span>
              <span :class="observabilityConfig.loki_enabled ? 'text-green-500' : 'text-gray-500'">
                {{ observabilityConfig.loki_enabled ? t('common.enabled') : t('common.disabled') }}
              </span>
            </div>
            <div v-if="observabilityConfig.loki?.url" class="text-sm text-slate-500 truncate">
              {{ observabilityConfig.loki?.url }}
            </div>
          </div>
        </div>

        <!-- InfluxDB Status -->
        <div class="p-6 bg-slate-50 dark:bg-slate-800/50 rounded-xl">
          <h3 class="font-semibold mb-4 flex items-center gap-2">
            <BaseIcon :path="mdiDatabase" size="20" />
            InfluxDB
          </h3>
          <div class="space-y-3">
            <div class="flex items-center justify-between">
              <span class="text-slate-500">{{ t('common.status') }}</span>
              <span :class="observabilityConfig.influx_enabled ? 'text-green-500' : 'text-gray-500'">
                {{ observabilityConfig.influx_enabled ? t('common.enabled') : t('common.disabled') }}
              </span>
            </div>
            <div v-if="observabilityConfig.influx?.url" class="text-sm text-slate-500 truncate">
              {{ observabilityConfig.influx?.url }}
            </div>
          </div>
        </div>

        <!-- Graylog Status -->
        <div class="p-6 bg-slate-50 dark:bg-slate-800/50 rounded-xl">
          <h3 class="font-semibold mb-4 flex items-center gap-2">
            <BaseIcon :path="mdiMessageText" size="20" />
            Graylog
          </h3>
          <div class="space-y-3">
            <div class="flex items-center justify-between">
              <span class="text-slate-500">{{ t('common.status') }}</span>
              <span :class="observabilityConfig.graylog_enabled ? 'text-green-500' : 'text-gray-500'">
                {{ observabilityConfig.graylog_enabled ? t('common.enabled') : t('common.disabled') }}
              </span>
            </div>
            <div v-if="observabilityConfig.graylog?.endpoint" class="text-sm text-slate-500 truncate">
              {{ observabilityConfig.graylog?.endpoint }}
            </div>
          </div>
        </div>

        <!-- OTLP Status -->
        <div class="p-6 bg-slate-50 dark:bg-slate-800/50 rounded-xl">
          <h3 class="font-semibold mb-4 flex items-center gap-2">
            <BaseIcon :path="mdiSync" size="20" />
            OpenTelemetry
          </h3>
          <div class="space-y-3">
            <div class="flex items-center justify-between">
              <span class="text-slate-500">{{ t('common.status') }}</span>
              <span :class="observabilityConfig.otlp_enabled ? 'text-green-500' : 'text-gray-500'">
                {{ observabilityConfig.otlp_enabled ? t('common.enabled') : t('common.disabled') }}
              </span>
            </div>
            <div v-if="observabilityConfig.otlp_endpoint" class="text-sm text-slate-500 truncate">
              {{ observabilityConfig.otlp_endpoint }}
            </div>
          </div>
        </div>

        <!-- ClickHouse Status -->
        <div class="p-6 bg-slate-50 dark:bg-slate-800/50 rounded-xl">
          <h3 class="font-semibold mb-4 flex items-center gap-2">
            <BaseIcon :path="mdiServer" size="20" />
            ClickHouse
          </h3>
          <div class="space-y-3">
            <div class="flex items-center justify-between">
              <span class="text-slate-500">{{ t('common.status') }}</span>
              <span :class="observabilityConfig.clickhouse_enabled ? 'text-green-500' : 'text-gray-500'">
                {{ observabilityConfig.clickhouse_enabled ? t('common.enabled') : t('common.disabled') }}
              </span>
            </div>
            <div v-if="observabilityConfig.clickhouse_endpoint" class="text-sm text-slate-500 truncate">
              {{ observabilityConfig.clickhouse_endpoint }}
            </div>
          </div>
        </div>
      </div>

      <div class="mt-6 p-4 bg-blue-50 dark:bg-blue-900/20 rounded-lg">
        <p class="text-sm text-blue-800 dark:text-blue-200" v-html="t('gw.observabilityNote')"></p>
      </div>
    </CardBox>

    <!-- Add Service Modal -->
    <CardBoxModal 
      v-model="isAddServiceModalActive" 
      :title="t('gw.addUpstreamService')" 
      button="success" 
      :button-label="t('gw.addService')"
      has-cancel
      @confirm="addService"
    >
      <div class="space-y-4">
        <FormField :label="t('gw.serviceName')">
          <FormControl v-model="newService.name" placeholder="my-service" />
        </FormField>
        <div class="grid grid-cols-2 gap-4">
          <FormField :label="t('gw.host')">
            <FormControl v-model="newService.host" placeholder="localhost or IP" />
          </FormField>
          <FormField :label="t('gw.port')">
            <FormControl v-model="newService.port" type="number" placeholder="80" />
          </FormField>
        </div>
        <div class="grid grid-cols-2 gap-4">
          <FormField :label="t('gw.protocol')">
            <FormControl v-model="newService.protocol" :options="['http', 'https']" />
          </FormField>
          <FormField :label="t('gw.requestTimeout')">
            <FormControl v-model.number="newService.timeout" type="number" min="0" placeholder="0" />
          </FormField>
        </div>
        <FormField :label="t('gw.basePath')">
          <FormControl v-model="newService.path" placeholder="/api" />
        </FormField>
        <div class="grid grid-cols-2 gap-4">
          <FormField :label="t('gw.healthCheckPath')">
            <FormControl v-model="newService.health_check.path" placeholder="/health" />
          </FormField>
          <FormField :label="t('gw.healthCheckInterval')">
            <FormControl
              v-model.number="newService.health_check.interval"
              type="number"
              min="1"
              placeholder="5"
            />
          </FormField>
        </div>
      </div>
    </CardBoxModal>

    <!-- Add Upstream Modal -->
    <CardBoxModal
      v-model="isAddUpstreamModalActive"
      :title="t('gw.addUpstream')"
      button="success"
      :button-label="t('gw.addUpstream')"
      has-cancel
      @confirm="addUpstream"
    >
      <div class="space-y-4">
        <FormField :label="t('gw.name')">
          <FormControl v-model="newUpstream.name" placeholder="my-pool" />
        </FormField>
        <div class="grid grid-cols-2 gap-4">
          <FormField :label="t('gw.distributionStrategy')">
            <FormControl v-model="newUpstream.strategy" :options="strategyOptions" />
          </FormField>
          <FormField>
            <FormCheckRadio v-model="newUpstream.enabled" :label="t('common.enabled')" name="new_upstream_enabled" />
          </FormField>
        </div>
        <div class="border-t pt-4">
          <FormField>
            <FormCheckRadio v-model="newUpstream.sticky_enabled" :label="t('gw.enableSticky')" name="new_upstream_sticky" />
          </FormField>
          <p class="text-xs text-slate-500 mb-2">{{ t('gw.stickyHint') }}</p>
          <template v-if="newUpstream.sticky_enabled">
            <div class="grid grid-cols-2 gap-4">
              <FormField :label="t('gw.stickyMode')">
                <FormControl v-model="newUpstream.sticky.mode" :options="stickyModeOptions" />
              </FormField>
              <FormField v-if="newUpstream.sticky.mode?.value === 'cookie'" :label="t('gw.cookieName')">
                <FormControl v-model="newUpstream.sticky.cookie_name" placeholder="redock_lb" />
              </FormField>
              <FormField v-if="newUpstream.sticky.mode?.value === 'header'" :label="t('gw.headerName')">
                <FormControl v-model="newUpstream.sticky.header_name" placeholder="X-Session-Id" />
              </FormField>
              <FormField v-if="newUpstream.sticky.mode?.value === 'cookie'" :label="t('gw.cookieTtl')">
                <FormControl v-model.number="newUpstream.sticky.ttl_seconds" type="number" min="0" placeholder="0" />
              </FormField>
            </div>
          </template>
        </div>
        <div class="border-t pt-4">
          <h4 class="font-semibold mb-2">{{ t('gw.targets') }}</h4>
          <p class="text-xs text-slate-500 mb-2">{{ t('gw.targetsHint') }}</p>
          <div v-for="(tg, idx) in newUpstream.targets" :key="'nu-t-' + idx" class="flex gap-2 items-end mb-2">
            <FormField :label="t('gw.service')" class="flex-1">
              <FormControl v-model="tg.service_id" :options="services.map(s => ({ value: s.id, label: s.name || s.id }))" />
            </FormField>
            <FormField :label="t('gw.weight')" class="w-24">
              <FormControl v-model.number="tg.weight" type="number" min="1" placeholder="1" />
            </FormField>
            <BaseButton :icon="mdiDelete" color="danger" small @click="removeUpstreamTarget(newUpstream, idx)" />
          </div>
          <BaseButton :label="t('gw.addTarget')" :icon="mdiPlus" color="info" small @click="addUpstreamTarget(newUpstream)" />
        </div>
      </div>
    </CardBoxModal>

    <!-- Edit Upstream Modal -->
    <CardBoxModal
      v-model="isEditUpstreamModalActive"
      :title="t('gw.editUpstream')"
      button="success"
      :button-label="t('gw.updateUpstream')"
      has-cancel
      @confirm="updateUpstream"
    >
      <div v-if="editingUpstream" class="space-y-4">
        <FormField :label="t('gw.name')">
          <FormControl v-model="editingUpstream.name" placeholder="my-pool" />
        </FormField>
        <div class="grid grid-cols-2 gap-4">
          <FormField :label="t('gw.distributionStrategy')">
            <FormControl v-model="editingUpstream.strategy" :options="strategyOptions" />
          </FormField>
          <FormField>
            <FormCheckRadio v-model="editingUpstream.enabled" :label="t('common.enabled')" name="edit_upstream_enabled" />
          </FormField>
        </div>
        <div class="border-t pt-4">
          <FormField>
            <FormCheckRadio v-model="editingUpstream.sticky_enabled" :label="t('gw.enableSticky')" name="edit_upstream_sticky" />
          </FormField>
          <template v-if="editingUpstream.sticky_enabled">
            <div class="grid grid-cols-2 gap-4">
              <FormField :label="t('gw.stickyMode')">
                <FormControl v-model="editingUpstream.sticky.mode" :options="stickyModeOptions" />
              </FormField>
              <FormField v-if="editingUpstream.sticky.mode?.value === 'cookie'" :label="t('gw.cookieName')">
                <FormControl v-model="editingUpstream.sticky.cookie_name" placeholder="redock_lb" />
              </FormField>
              <FormField v-if="editingUpstream.sticky.mode?.value === 'header'" :label="t('gw.headerName')">
                <FormControl v-model="editingUpstream.sticky.header_name" placeholder="X-Session-Id" />
              </FormField>
              <FormField v-if="editingUpstream.sticky.mode?.value === 'cookie'" :label="t('gw.cookieTtl')">
                <FormControl v-model.number="editingUpstream.sticky.ttl_seconds" type="number" min="0" placeholder="0" />
              </FormField>
            </div>
          </template>
        </div>
        <div class="border-t pt-4">
          <h4 class="font-semibold mb-2">{{ t('gw.targets') }}</h4>
          <div v-for="(tg, idx) in editingUpstream.targets" :key="'eu-t-' + idx" class="flex gap-2 items-end mb-2">
            <FormField :label="t('gw.service')" class="flex-1">
              <FormControl v-model="tg.service_id" :options="services.map(s => ({ value: s.id, label: s.name || s.id }))" />
            </FormField>
            <FormField :label="t('gw.weight')" class="w-24">
              <FormControl v-model.number="tg.weight" type="number" min="1" placeholder="1" />
            </FormField>
            <BaseButton :icon="mdiDelete" color="danger" small @click="removeUpstreamTarget(editingUpstream, idx)" />
          </div>
          <BaseButton :label="t('gw.addTarget')" :icon="mdiPlus" color="info" small @click="addUpstreamTarget(editingUpstream)" />
        </div>
      </div>
    </CardBoxModal>

    <!-- Add Route Modal -->
    <CardBoxModal 
      v-model="isAddRouteModalActive" 
      :title="t('gw.addRoute')" 
      button="success" 
      :button-label="t('gw.addRoute')"
      has-cancel
      @confirm="addRoute"
    >
      <div class="space-y-4">
        <FormField :label="t('gw.routeName')">
          <FormControl v-model="newRoute.name" placeholder="my-route" />
        </FormField>
        <FormField :label="t('gw.upstreamBackend')">
          <FormControl v-model="newRoute.upstream_id" :options="upstreamOptions" />
        </FormField>
        <FormField :label="t('gw.pathsLabel')">
          <FormControl v-model="newRoute.paths" placeholder="/api, /api/*" />
        </FormField>
        <FormField :label="t('gw.hostsField')">
          <FormControl v-model="newRoute.hosts" placeholder="api.example.com" />
        </FormField>
        <FormField :label="t('gw.hostRewrite')">
          <FormControl v-model="newRoute.host_rewrite" placeholder="order.test.com" />
        </FormField>
        <FormField :label="t('gw.methods')">
          <FormControl v-model="newRoute.methods" placeholder="GET, POST, PUT" />
        </FormField>
        <FormField :label="t('gw.requestTimeoutOpt')">
          <FormControl v-model.number="newRoute.timeout" type="number" min="0" :placeholder="t('gw.timeoutInheritPlaceholder')" />
        </FormField>
        <div class="grid grid-cols-2 gap-4">
          <FormField>
            <FormCheckRadio v-model="newRoute.strip_path" :label="t('gw.stripPath')" name="strip_path" />
          </FormField>
          <FormField>
            <FormCheckRadio v-model="newRoute.rate_limit_enabled" :label="t('gw.enableRateLimiting')" name="rate_limit" />
          </FormField>
        </div>
        <FormField>
          <FormCheckRadio v-model="newRoute.preserve_host" :label="t('gw.preserveHost')" name="preserve_host" />
        </FormField>
        <FormField>
          <FormCheckRadio v-model="newRoute.observability_enabled" :label="t('gw.sendObservability')" name="new_route_observability" />
        </FormField>
        <FormField>
          <FormCheckRadio v-model="newRoute.lets_encrypt" :label="t('gw.leCheckbox')" name="new_route_lets_encrypt" />
          <p class="text-xs text-gray-500 mt-1">
            {{ t('gw.leCheckboxHint') }}
          </p>
        </FormField>
        <div class="grid grid-cols-2 gap-4">
          <FormField>
            <FormCheckRadio v-model="newRoute.auth_required" :label="t('gw.requireAuth')" name="new_route_auth" />
          </FormField>
          <FormField v-if="newRoute.auth_required" :label="t('gw.authType')">
            <FormControl v-model="newRoute.auth_type" :options="[{ value: '', label: t('gw.selectPlaceholder') }, { value: 'basic', label: t('gw.authBasic') }, { value: 'jwt', label: t('gw.authJwt') }, { value: 'header', label: t('gw.authHeaderOpt') }]" />
          </FormField>
        </div>
        <template v-if="newRoute.auth_required && normalizeAuthType(newRoute.auth_type) === 'header'">
          <div class="border-t pt-4 mt-4">
            <h4 class="font-semibold mb-3">{{ t('gw.requiredHeaders') }}</h4>
            <p class="text-xs text-slate-500 mb-2">{{ t('gw.requiredHeadersHint') }}</p>
            <div v-for="(entry, idx) in newRoute.auth_headers" :key="'ah-' + idx" class="flex gap-2 items-end mb-2">
              <FormControl v-model="entry.key" :placeholder="t('gw.headerNamePlaceholder')" class="flex-1" />
              <FormControl v-model="entry.value" :placeholder="t('gw.expectedValue')" class="flex-1" />
              <BaseButton label="" color="danger" small :icon="mdiDelete" @click="newRoute.auth_headers.splice(idx, 1)" />
            </div>
            <BaseButton :label="t('gw.addHeader')" color="info" small :icon="mdiPlus" @click="newRoute.auth_headers.push({ key: '', value: '' })" />
          </div>
        </template>
        <template v-if="newRoute.auth_required && normalizeAuthType(newRoute.auth_type) === 'basic'">
          <div class="border-t pt-4 mt-4">
            <h4 class="font-semibold mb-3">{{ t('gw.basicAuthUsers') }}</h4>
            <p class="text-xs text-slate-500 mb-2">{{ t('gw.basicAuthHint') }}</p>
            <FormField :label="t('gw.realm')">
              <FormControl v-model="newRoute.basic_auth_realm" placeholder="Restricted" />
            </FormField>
            <div v-for="(u, idx) in newRoute.basic_auth_users" :key="'bu-' + idx" class="flex gap-2 items-end mb-2 mt-2">
              <FormControl v-model="u.username" :placeholder="t('gw.usernamePlaceholder')" class="flex-1" />
              <FormControl v-model="u.password" type="password" :placeholder="t('gw.passwordPlaceholder')" class="flex-1" />
              <BaseButton label="" color="danger" small :icon="mdiDelete" @click="newRoute.basic_auth_users.splice(idx, 1)" />
            </div>
            <BaseButton :label="t('gw.addUser')" color="info" small :icon="mdiPlus" @click="newRoute.basic_auth_users.push({ username: '', password: '', password_hash: '' })" />
          </div>
        </template>
        <template v-if="newRoute.auth_required && normalizeAuthType(newRoute.auth_type) === 'jwt'">
          <div class="border-t pt-4 mt-4">
            <h4 class="font-semibold mb-3">{{ t('gw.jwtHs256') }}</h4>
            <p class="text-xs text-slate-500 mb-2">{{ t('gw.jwtHint') }}</p>
            <FormField :label="t('gw.secret')">
              <FormControl v-model="newRoute.jwt.secret" type="password" placeholder="Shared HMAC secret" />
            </FormField>
            <div class="grid grid-cols-2 gap-4">
              <FormField :label="t('gw.issuerOptional')">
                <FormControl v-model="newRoute.jwt.issuer" placeholder="iss claim" />
              </FormField>
              <FormField :label="t('gw.audienceOptional')">
                <FormControl v-model="newRoute.jwt.audience" placeholder="aud claim" />
              </FormField>
            </div>
          </div>
        </template>
        <div v-if="newRoute.rate_limit_enabled" class="grid grid-cols-2 gap-4">
          <FormField :label="t('gw.requests')">
            <FormControl v-model="newRoute.rate_limit_requests" type="number" placeholder="100" />
          </FormField>
          <FormField :label="t('gw.windowSeconds')">
            <FormControl v-model="newRoute.rate_limit_window" type="number" placeholder="60" />
          </FormField>
        </div>
        <div class="border-t pt-4 mt-4">
          <h4 class="font-semibold mb-3">{{ t('gw.corsTitle') }}</h4>
          <p class="text-xs text-slate-500 mb-2">{{ t('gw.corsHint') }}</p>
          <FormField>
            <FormCheckRadio v-model="newRoute.cors.enabled" :label="t('gw.enableCors')" name="new_route_cors" />
          </FormField>
          <template v-if="newRoute.cors.enabled">
            <FormField :label="t('gw.allowOrigins')">
              <FormControl v-model="newRoute.cors.allow_origins" placeholder="* or https://app.example.com" />
            </FormField>
            <FormField :label="t('gw.allowMethods')">
              <FormControl v-model="newRoute.cors.allow_methods" placeholder="GET, POST, OPTIONS" />
            </FormField>
            <FormField :label="t('gw.allowHeaders')">
              <FormControl v-model="newRoute.cors.allow_headers" placeholder="Content-Type, Authorization" />
            </FormField>
            <FormField :label="t('gw.exposeHeaders')">
              <FormControl v-model="newRoute.cors.expose_headers" placeholder="X-Request-Id" />
            </FormField>
            <div class="grid grid-cols-2 gap-4">
              <FormField>
                <FormCheckRadio v-model="newRoute.cors.allow_credentials" :label="t('gw.allowCredentials')" name="new_cors_creds" />
              </FormField>
              <FormField :label="t('gw.maxAge')">
                <FormControl v-model.number="newRoute.cors.max_age" type="number" placeholder="86400" />
              </FormField>
            </div>
          </template>
        </div>
        <div class="border-t pt-4 mt-4">
          <h4 class="font-semibold mb-3">{{ t('gw.additionalHeaders') }}</h4>
          <div v-for="(entry, idx) in newRoute.response_headers" :key="'rh-' + idx" class="flex gap-2 items-end mb-2">
            <FormControl v-model="entry.key" placeholder="Header-Name" class="flex-1" />
            <FormControl v-model="entry.value" placeholder="value" class="flex-1" />
            <BaseButton label="" color="danger" small :icon="mdiDelete" @click="newRoute.response_headers.splice(idx, 1)" />
          </div>
          <BaseButton :label="t('gw.addHeader')" color="info" small :icon="mdiPlus" @click="newRoute.response_headers.push({ key: '', value: '' })" />
        </div>
      </div>
    </CardBoxModal>

    <!-- Config Modal -->
    <CardBoxModal 
      v-model="isConfigModalActive" 
      :title="t('gw.gatewaySettings')" 
      button="success" 
      :button-label="t('gw.saveSettings')"
      has-cancel
      @confirm="saveConfig"
    >
      <div class="space-y-4">
        <div class="grid grid-cols-2 gap-4">
          <FormField :label="t('gw.httpPort')">
            <FormControl v-model="gatewayConfig.http_port" type="number" placeholder="80" />
          </FormField>
          <FormField :label="t('gw.httpsPort')">
            <FormControl v-model="gatewayConfig.https_port" type="number" placeholder="443" />
          </FormField>
        </div>
        <FormField>
          <FormCheckRadio v-model="gatewayConfig.https_enabled" :label="t('gw.enableHttps')" name="https_enabled" />
        </FormField>
        <FormField>
          <FormCheckRadio v-model="gatewayConfig.access_log_enabled" :label="t('gw.enableAccessLogging')" name="access_log" />
        </FormField>

        <!-- Let's Encrypt / SSL -->
        <div class="border-t pt-4 mt-4">
          <h4 class="font-semibold mb-3 flex items-center gap-2">
            <BaseIcon :path="mdiLock" size="18" />
            {{ t('gw.letsEncryptSsl') }}
          </h4>
          <FormField>
            <FormCheckRadio v-model="gatewayConfig.lets_encrypt.enabled" :label="t('gw.enableLetsEncrypt')" name="le_enabled" />
          </FormField>
          <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
            <FormField :label="t('gw.emailAddress')">
              <FormControl v-model="gatewayConfig.lets_encrypt.email" type="email" placeholder="admin@example.com" />
            </FormField>
            <FormField :label="t('gw.renewBeforeExpiry')">
              <FormControl v-model.number="gatewayConfig.lets_encrypt.renew_before_days" type="number" min="1" placeholder="30" />
            </FormField>
          </div>
          <FormField>
            <FormCheckRadio v-model="gatewayConfig.lets_encrypt.auto_renew" :label="t('gw.autoRenewCerts')" name="le_auto_renew" />
          </FormField>
          <p class="text-xs text-gray-500" v-html="t('gw.leConfigHint')"></p>
        </div>

        <div class="border-t pt-4 mt-4">
          <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
            <FormField :label="t('gw.topClientLimit')">
              <FormControl
                v-model.number="gatewayConfig.client_security.top_client_limit"
                type="number"
                min="1"
                max="1000"
                placeholder="1000"
              />
            </FormField>
            <FormField>
              <FormCheckRadio
                v-model="gatewayConfig.client_security.tracking_enabled"
                :label="t('gw.enableClientTracking')"
                name="client_tracking_enabled"
              />
            </FormField>
          </div>
          <p class="text-xs text-slate-500 mt-2">
            Controls how many clients are stored for analytics and manual blocking (up to 1000).
          </p>
        </div>
        <div class="border-t pt-4 mt-4">
          <h4 class="font-semibold mb-3">{{ t('gw.timeoutsSeconds') }}</h4>
          <p class="text-xs text-slate-500 mb-3">
            Global defaults. Per-route timeout overrides per-service, which overrides the request timeout below.
            Server Write must be ≥ the longest expected request, otherwise it caps everything.
          </p>
          <div class="grid grid-cols-1 lg:grid-cols-3 gap-4">
            <FormField :label="t('gw.requestTimeoutOverall')">
              <FormControl v-model.number="gatewayConfig.timeouts.request_timeout_seconds" type="number" min="0" placeholder="0 = disabled" />
            </FormField>
            <FormField :label="t('gw.serverRead')">
              <FormControl v-model.number="gatewayConfig.timeouts.server_read_seconds" type="number" min="0" placeholder="30" />
            </FormField>
            <FormField :label="t('gw.serverWrite')">
              <FormControl v-model.number="gatewayConfig.timeouts.server_write_seconds" type="number" min="0" placeholder="30" />
            </FormField>
            <FormField :label="t('gw.serverIdle')">
              <FormControl v-model.number="gatewayConfig.timeouts.server_idle_seconds" type="number" min="0" placeholder="60" />
            </FormField>
            <FormField :label="t('gw.shutdown')">
              <FormControl v-model.number="gatewayConfig.timeouts.shutdown_seconds" type="number" min="0" placeholder="10" />
            </FormField>
            <FormField :label="t('gw.healthCheck')">
              <FormControl v-model.number="gatewayConfig.timeouts.health_check_seconds" type="number" min="0" placeholder="5" />
            </FormField>
            <FormField :label="t('gw.upstreamDial')">
              <FormControl v-model.number="gatewayConfig.timeouts.upstream_dial_seconds" type="number" min="0" placeholder="30" />
            </FormField>
            <FormField :label="t('gw.upstreamKeepAlive')">
              <FormControl v-model.number="gatewayConfig.timeouts.upstream_keep_alive_seconds" type="number" min="0" placeholder="30" />
            </FormField>
            <FormField :label="t('gw.upstreamIdleConn')">
              <FormControl v-model.number="gatewayConfig.timeouts.upstream_idle_conn_seconds" type="number" min="0" placeholder="90" />
            </FormField>
            <FormField :label="t('gw.tlsHandshake')">
              <FormControl v-model.number="gatewayConfig.timeouts.tls_handshake_seconds" type="number" min="0" placeholder="10" />
            </FormField>
            <FormField :label="t('gw.expectContinue')">
              <FormControl v-model.number="gatewayConfig.timeouts.expect_continue_seconds" type="number" min="0" placeholder="1" />
            </FormField>
          </div>
        </div>
      </div>
    </CardBoxModal>

    <!-- Edit Service Modal -->
    <CardBoxModal 
      v-model="isEditServiceModalActive" 
      :title="t('gw.editService')" 
      button="success" 
      :button-label="t('gw.updateService')"
      has-cancel
      @confirm="updateService"
    >
      <div v-if="editingService" class="space-y-4">
        <FormField :label="t('gw.serviceName')">
          <FormControl v-model="editingService.name" placeholder="my-service" />
        </FormField>
        <div class="grid grid-cols-2 gap-4">
          <FormField :label="t('gw.host')">
            <FormControl v-model="editingService.host" placeholder="localhost or IP" />
          </FormField>
          <FormField :label="t('gw.port')">
            <FormControl v-model="editingService.port" type="number" placeholder="80" />
          </FormField>
        </div>
        <div class="grid grid-cols-2 gap-4">
          <FormField :label="t('gw.protocol')">
            <FormControl v-model="editingService.protocol" :options="['http', 'https']" />
          </FormField>
          <FormField :label="t('gw.requestTimeout')">
            <FormControl v-model.number="editingService.timeout" type="number" min="0" placeholder="0" />
          </FormField>
        </div>
        <FormField :label="t('gw.basePath')">
          <FormControl v-model="editingService.path" placeholder="/api" />
        </FormField>
        <FormField>
          <FormCheckRadio v-model="editingService.enabled" :label="t('common.enabled')" name="edit_service_enabled" />
        </FormField>
        <div class="grid grid-cols-2 gap-4">
          <FormField :label="t('gw.healthCheckPath')">
            <FormControl v-model="editingService.health_check.path" placeholder="/health" />
          </FormField>
          <FormField :label="t('gw.healthCheckInterval')">
            <FormControl
              v-model.number="editingService.health_check.interval"
              type="number"
              min="1"
              placeholder="5"
            />
          </FormField>
        </div>
      </div>
    </CardBoxModal>

    <!-- Edit Route Modal -->
    <CardBoxModal 
      v-model="isEditRouteModalActive" 
      :title="t('gw.editRoute')" 
      button="success" 
      :button-label="t('gw.updateRoute')"
      has-cancel
      @confirm="updateRoute"
    >
      <div v-if="editingRoute" class="space-y-4">
        <FormField :label="t('gw.routeName')">
          <FormControl v-model="editingRoute.name" placeholder="my-route" />
        </FormField>
        <FormField :label="t('gw.upstreamBackend')">
          <FormControl v-model="editingRoute.upstream_id" :options="upstreamOptions" />
        </FormField>
        <FormField :label="t('gw.pathsLabel')">
          <FormControl v-model="editingRoute.paths" placeholder="/api, /api/*" />
        </FormField>
        <FormField :label="t('gw.hostsField')">
          <FormControl v-model="editingRoute.hosts" placeholder="api.example.com" />
        </FormField>
        <FormField :label="t('gw.hostRewrite')">
          <FormControl v-model="editingRoute.host_rewrite" placeholder="order.test.com" />
        </FormField>
        <FormField :label="t('gw.methods')">
          <FormControl v-model="editingRoute.methods" placeholder="GET, POST, PUT" />
        </FormField>
        <FormField :label="t('gw.requestTimeoutOpt')">
          <FormControl v-model.number="editingRoute.timeout" type="number" min="0" :placeholder="t('gw.timeoutInheritPlaceholder')" />
        </FormField>
        <div class="grid grid-cols-2 gap-4">
          <FormField>
            <FormCheckRadio v-model="editingRoute.strip_path" :label="t('gw.stripPath')" name="edit_strip_path" />
          </FormField>
          <FormField>
            <FormCheckRadio v-model="editingRoute.enabled" :label="t('common.enabled')" name="edit_route_enabled" />
          </FormField>
        </div>
        <FormField>
          <FormCheckRadio v-model="editingRoute.preserve_host" :label="t('gw.preserveHost')" name="edit_preserve_host" />
        </FormField>
        <FormField>
          <FormCheckRadio v-model="editingRoute.observability_enabled" :label="t('gw.sendObservability')" name="edit_route_observability" />
        </FormField>
        <FormField>
          <FormCheckRadio v-model="editingRoute.lets_encrypt" :label="t('gw.leCheckbox')" name="edit_route_lets_encrypt" />
          <p class="text-xs text-gray-500 mt-1">
            {{ t('gw.leCheckboxHint') }}
          </p>
        </FormField>
        <div class="grid grid-cols-2 gap-4">
          <FormField>
            <FormCheckRadio v-model="editingRoute.rate_limit_enabled" :label="t('gw.enableRateLimiting')" name="edit_rate_limit" />
          </FormField>
          <FormField>
            <FormCheckRadio v-model="editingRoute.auth_required" :label="t('gw.requireAuth')" name="edit_auth" />
          </FormField>
        </div>
        <FormField v-if="editingRoute.auth_required" :label="t('gw.authType')">
          <FormControl v-model="editingRoute.auth_type" :options="[{ value: '', label: t('gw.selectPlaceholder') }, { value: 'basic', label: t('gw.authBasic') }, { value: 'jwt', label: t('gw.authJwt') }, { value: 'header', label: t('gw.authHeaderOpt') }]" />
        </FormField>
        <template v-if="editingRoute.auth_required && normalizeAuthType(editingRoute.auth_type) === 'basic'">
          <div class="border-t pt-4 mt-4">
            <h4 class="font-semibold mb-3">{{ t('gw.basicAuthUsers') }}</h4>
            <p class="text-xs text-slate-500 mb-2">Leave password blank to keep the existing one. Browser shows a native login prompt.</p>
            <FormField :label="t('gw.realm')">
              <FormControl v-model="editingRoute.basic_auth_realm" placeholder="Restricted" />
            </FormField>
            <div v-for="(u, idx) in (editingRoute.basic_auth_users || [])" :key="'ebu-' + idx" class="flex gap-2 items-end mb-2 mt-2">
              <FormControl v-model="u.username" :placeholder="t('gw.usernamePlaceholder')" class="flex-1" />
              <FormControl v-model="u.password" type="password" :placeholder="u.password_hash ? '••• (unchanged)' : 'Password'" class="flex-1" />
              <BaseButton label="" color="danger" small :icon="mdiDelete" @click="(editingRoute.basic_auth_users || []).splice(idx, 1)" />
            </div>
            <BaseButton :label="t('gw.addUser')" color="info" small :icon="mdiPlus" @click="(editingRoute.basic_auth_users = editingRoute.basic_auth_users || []).push({ username: '', password: '', password_hash: '' })" />
          </div>
        </template>
        <template v-if="editingRoute.auth_required && normalizeAuthType(editingRoute.auth_type) === 'jwt'">
          <div class="border-t pt-4 mt-4">
            <h4 class="font-semibold mb-3">{{ t('gw.jwtHs256') }}</h4>
            <p class="text-xs text-slate-500 mb-2">{{ t('gw.jwtHint') }}</p>
            <FormField :label="t('gw.secret')">
              <FormControl v-model="editingRoute.jwt.secret" type="password" placeholder="Shared HMAC secret" />
            </FormField>
            <div class="grid grid-cols-2 gap-4">
              <FormField :label="t('gw.issuerOptional')">
                <FormControl v-model="editingRoute.jwt.issuer" placeholder="iss claim" />
              </FormField>
              <FormField :label="t('gw.audienceOptional')">
                <FormControl v-model="editingRoute.jwt.audience" placeholder="aud claim" />
              </FormField>
            </div>
          </div>
        </template>
        <template v-if="editingRoute.auth_required && normalizeAuthType(editingRoute.auth_type) === 'header'">
          <div class="border-t pt-4 mt-4">
            <h4 class="font-semibold mb-3">{{ t('gw.requiredHeaders') }}</h4>
            <p class="text-xs text-slate-500 mb-2">Request must include each header with the given value.</p>
            <div v-for="(entry, idx) in (editingRoute.auth_headers || [])" :key="'eah-' + idx" class="flex gap-2 items-end mb-2">
              <FormControl v-model="entry.key" :placeholder="t('gw.headerNamePlaceholder')" class="flex-1" />
              <FormControl v-model="entry.value" :placeholder="t('gw.expectedValue')" class="flex-1" />
              <BaseButton label="" color="danger" small :icon="mdiDelete" @click="(editingRoute.auth_headers || []).splice(idx, 1)" />
            </div>
            <BaseButton :label="t('gw.addHeader')" color="info" small :icon="mdiPlus" @click="(editingRoute.auth_headers = editingRoute.auth_headers || []).push({ key: '', value: '' })" />
          </div>
        </template>
        <div v-if="editingRoute.rate_limit_enabled" class="grid grid-cols-2 gap-4">
          <FormField :label="t('gw.requests')">
            <FormControl v-model="editingRoute.rate_limit_requests" type="number" placeholder="100" />
          </FormField>
          <FormField :label="t('gw.windowSeconds')">
            <FormControl v-model="editingRoute.rate_limit_window" type="number" placeholder="60" />
          </FormField>
        </div>
        <div class="border-t pt-4 mt-4">
          <h4 class="font-semibold mb-3">{{ t('gw.corsTitle') }}</h4>
          <p class="text-xs text-slate-500 mb-2">{{ t('gw.corsHint') }}</p>
          <FormField>
            <FormCheckRadio v-model="editingRoute.cors.enabled" :label="t('gw.enableCors')" name="edit_route_cors" />
          </FormField>
          <template v-if="editingRoute.cors && editingRoute.cors.enabled">
            <FormField :label="t('gw.allowOrigins')">
              <FormControl v-model="editingRoute.cors.allow_origins" placeholder="* or https://app.example.com" />
            </FormField>
            <FormField :label="t('gw.allowMethods')">
              <FormControl v-model="editingRoute.cors.allow_methods" placeholder="GET, POST, OPTIONS" />
            </FormField>
            <FormField :label="t('gw.allowHeaders')">
              <FormControl v-model="editingRoute.cors.allow_headers" placeholder="Content-Type, Authorization" />
            </FormField>
            <FormField :label="t('gw.exposeHeaders')">
              <FormControl v-model="editingRoute.cors.expose_headers" placeholder="X-Request-Id" />
            </FormField>
            <div class="grid grid-cols-2 gap-4">
              <FormField>
                <FormCheckRadio v-model="editingRoute.cors.allow_credentials" :label="t('gw.allowCredentials')" name="edit_cors_creds" />
              </FormField>
              <FormField :label="t('gw.maxAge')">
                <FormControl v-model.number="editingRoute.cors.max_age" type="number" placeholder="86400" />
              </FormField>
            </div>
          </template>
        </div>
        <div class="border-t pt-4 mt-4">
          <h4 class="font-semibold mb-3">{{ t('gw.additionalHeaders') }}</h4>
          <div v-for="(entry, idx) in editingRoute.response_headers" :key="'erh-' + idx" class="flex gap-2 items-end mb-2">
            <FormControl v-model="entry.key" placeholder="Header-Name" class="flex-1" />
            <FormControl v-model="entry.value" placeholder="value" class="flex-1" />
            <BaseButton label="" color="danger" small :icon="mdiDelete" @click="editingRoute.response_headers.splice(idx, 1)" />
          </div>
          <BaseButton :label="t('gw.addHeader')" color="info" small :icon="mdiPlus" @click="editingRoute.response_headers.push({ key: '', value: '' })" />
        </div>
      </div>
    </CardBoxModal>

    <!-- Observability Modal -->
    <CardBoxModal 
      v-model="isObservabilityModalActive" 
      :title="t('gw.configureObservability')" 
      button="success" 
      :button-label="t('gw.saveConfiguration')"
      has-cancel
      @confirm="saveObservability"
    >
      <div class="space-y-4">
        <FormField>
          <FormCheckRadio v-model="observabilityConfig.enabled" :label="t('gw.enableObservability')" name="obs_enabled" />
        </FormField>
        
        <div class="border-t pt-4 mt-4">
          <h4 class="font-semibold mb-3">Loki</h4>
          <FormField>
            <FormCheckRadio v-model="observabilityConfig.loki_enabled" :label="t('gw.enableLoki')" name="loki_enabled" />
          </FormField>
          <FormField :label="t('gw.lokiEndpoint')">
            <FormControl v-model="observabilityConfig.loki.url" placeholder="http://loki:3100/loki/api/v1/push" />
          </FormField>
          <FormField :label="t('gw.tenantIdOptional')">
            <FormControl v-model="observabilityConfig.loki.tenant_id" placeholder="acme-org" />
          </FormField>
          <FormField :label="t('gw.apiKeyOptional')">
            <FormControl v-model="observabilityConfig.loki.api_key" type="password" placeholder="API Key" />
          </FormField>
        </div>

        <div class="border-t pt-4 mt-4">
          <h4 class="font-semibold mb-3">InfluxDB</h4>
          <FormField>
            <FormCheckRadio v-model="observabilityConfig.influx_enabled" :label="t('gw.enableInflux')" name="influx_enabled" />
          </FormField>
          <FormField :label="t('gw.influxUrl')">
            <FormControl v-model="observabilityConfig.influx.url" placeholder="http://influxdb:8086" />
          </FormField>
          <div class="grid grid-cols-2 gap-4">
            <FormField :label="t('gw.organization')">
              <FormControl v-model="observabilityConfig.influx.org" placeholder="my-org" />
            </FormField>
            <FormField :label="t('gw.bucket')">
              <FormControl v-model="observabilityConfig.influx.bucket" placeholder="api_gateway" />
            </FormField>
          </div>
          <FormField :label="t('gw.accessToken')">
            <FormControl v-model="observabilityConfig.influx.token" type="password" placeholder="Token" />
          </FormField>
        </div>

        <div class="border-t pt-4 mt-4">
          <h4 class="font-semibold mb-3">Graylog</h4>
          <FormField>
            <FormCheckRadio v-model="observabilityConfig.graylog_enabled" :label="t('gw.enableGraylog')" name="graylog_enabled" />
          </FormField>
          <FormField :label="t('gw.graylogEndpoint')">
            <FormControl v-model="observabilityConfig.graylog.endpoint" placeholder="http://graylog:12201/gelf" />
          </FormField>
          <FormField :label="t('gw.streamIdOptional')">
            <FormControl v-model="observabilityConfig.graylog.stream_id" placeholder="graylog-stream-id" />
          </FormField>
          <div class="grid grid-cols-2 gap-4">
            <FormField :label="t('gw.apiKeyOptional')">
              <FormControl v-model="observabilityConfig.graylog.api_key" type="password" placeholder="API Key" />
            </FormField>
            <FormField :label="t('gw.apiKeyHeader')">
              <FormControl v-model="observabilityConfig.graylog.api_key_header" placeholder="Authorization" />
            </FormField>
          </div>
        </div>

        <div class="border-t pt-4 mt-4">
          <h4 class="font-semibold mb-3">OpenTelemetry (OTLP)</h4>
          <FormField>
            <FormCheckRadio v-model="observabilityConfig.otlp_enabled" :label="t('gw.enableOtlp')" name="otlp_enabled" />
          </FormField>
          <FormField :label="t('gw.otlpEndpoint')">
            <FormControl v-model="observabilityConfig.otlp_endpoint" placeholder="http://otel-collector:4318" />
          </FormField>
        </div>

        <div class="border-t pt-4 mt-4">
          <h4 class="font-semibold mb-3">ClickHouse</h4>
          <FormField>
            <FormCheckRadio v-model="observabilityConfig.clickhouse_enabled" :label="t('gw.enableClickhouse')" name="ch_enabled" />
          </FormField>
          <FormField :label="t('gw.clickhouseEndpoint')">
            <FormControl v-model="observabilityConfig.clickhouse_endpoint" placeholder="http://clickhouse:8123" />
          </FormField>
          <div class="grid grid-cols-2 gap-4">
            <FormField :label="t('gw.database')">
              <FormControl v-model="observabilityConfig.clickhouse_database" placeholder="default" />
            </FormField>
            <FormField :label="t('gw.table')">
              <FormControl v-model="observabilityConfig.clickhouse_table" placeholder="api_gateway_logs" />
            </FormField>
          </div>
          <div class="grid grid-cols-2 gap-4">
            <FormField :label="t('gw.username')">
              <FormControl v-model="observabilityConfig.clickhouse_username" placeholder="default" />
            </FormField>
            <FormField :label="t('gw.password')">
              <FormControl v-model="observabilityConfig.clickhouse_password" type="password" />
            </FormField>
          </div>
        </div>

        <div class="border-t pt-4 mt-4">
          <h4 class="font-semibold mb-3">{{ t('gw.batching') }}</h4>
          <div class="grid grid-cols-2 gap-4">
            <FormField :label="t('gw.batchSize')">
              <FormControl v-model="observabilityConfig.batch_size" type="number" placeholder="100" />
            </FormField>
            <FormField :label="t('gw.flushInterval')">
              <FormControl v-model="observabilityConfig.flush_interval" type="number" placeholder="30" />
            </FormField>
          </div>
        </div>
      </div>
    </CardBoxModal>

    <!-- Delete Confirmation Modal -->
    <CardBoxModal 
      v-model="isDeleteModalActive" 
      :title="t('gw.confirmDelete')" 
      button="danger" 
      :button-label="t('common.delete')"
      has-cancel
      @confirm="deleteItem"
    >
      <p class="text-slate-600 dark:text-slate-400">
        {{ t('gw.deleteConfirmText', { type: deleteTarget.type }) }}
        <strong v-if="deleteTarget.item">{{ deleteTarget.item.name }}</strong>
      </p>
    </CardBoxModal>
  </div>
</template>
