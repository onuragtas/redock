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
import {
  mdiCertificate,
  mdiChartLine,
  mdiCog,
  mdiDatabase,
  mdiDelete,
  mdiMessageText,
  mdiHeart,
  mdiHeartPulse,
  mdiLock,
  mdiPencil,
  mdiPlay,
  mdiPlus,
  mdiRefresh,
  mdiRouter,
  mdiServer,
  mdiShield,
  mdiSpeedometer,
  mdiStop,
  mdiSync,
  mdiWeb
} from '@mdi/js';
import { computed, onMounted, onUnmounted, ref } from "vue";

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
const routes = ref([])
const serviceHealth = ref([])
const certificateInfo = ref({})
const renewerStatus = ref({ running: false })

// Modal states
const activeTab = ref('overview')
const isAddServiceModalActive = ref(false)
const isEditServiceModalActive = ref(false)
const isAddRouteModalActive = ref(false)
const isEditRouteModalActive = ref(false)
const isDeleteModalActive = ref(false)
const isLetsEncryptModalActive = ref(false)
const isConfigModalActive = ref(false)
const isObservabilityModalActive = ref(false)
const deleteTarget = ref({ type: '', item: null })
const editingService = ref(null)
const editingRoute = ref(null)

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
    interval: 30,
    timeout: 5,
    healthy_threshold: 2,
    unhealthy_threshold: 3
  }
})

const newRoute = ref({
  name: '',
  service_id: '',
  paths: '',
  methods: '',
  hosts: '',
  strip_path: true,
  preserve_host: false,
  priority: 0,
  rate_limit_enabled: false,
  rate_limit_requests: 100,
  rate_limit_window: 60,
  auth_required: false,
  auth_type: '',
  enabled: true
})

const letsEncryptConfig = ref({
  enabled: false,
  email: '',
  domains: '',
  staging: true,
  auto_renew: true,
  renew_before_days: 30
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
  access_log_enabled: true
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
    const [statusRes, statsRes, servicesRes, routesRes, healthRes, certRes, renewerRes] = await Promise.all([
      ApiService.apiGatewayStatus().catch(() => ({ data: { data: {} } })),
      ApiService.apiGatewayStats().catch(() => ({ data: { data: {} } })),
      ApiService.apiGatewayListServices().catch(() => ({ data: { data: [] } })),
      ApiService.apiGatewayListRoutes().catch(() => ({ data: { data: [] } })),
      ApiService.apiGatewayHealth().catch(() => ({ data: { data: [] } })),
      ApiService.apiGatewayCertificateInfo().catch(() => ({ data: { data: {} } })),
      ApiService.apiGatewayRenewerStatus().catch(() => ({ data: { data: {} } }))
    ])

    status.value = statusRes.data.data || {}
    stats.value = statsRes.data.data || {}
    services.value = servicesRes.data.data || []
    routes.value = routesRes.data.data || []
    serviceHealth.value = healthRes.data.data || []
    certificateInfo.value = certRes.data.data || {}
    renewerStatus.value = renewerRes.data.data || {}
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
      interval: 30,
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
      interval: 30,
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
    service_id: null,
    paths: '',
    methods: '',
    hosts: '',
    strip_path: true,
    preserve_host: false,
    priority: 0,
    rate_limit_enabled: false,
    rate_limit_requests: 100,
    rate_limit_window: 60,
    auth_required: false,
    auth_type: '',
    enabled: true
  }
  isAddRouteModalActive.value = true
}

const addRoute = async () => {
  try {
    const routeData = {
      ...newRoute.value,
      paths: newRoute.value.paths.split(',').map(p => p.trim()).filter(p => p),
      methods: newRoute.value.methods ? newRoute.value.methods.split(',').map(m => m.trim().toUpperCase()).filter(m => m) : [],
      hosts: newRoute.value.hosts ? newRoute.value.hosts.split(',').map(h => h.trim()).filter(h => h) : [],
      service_id: newRoute.value.service_id?.value || newRoute.value.service_id
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
  const serviceId = route.service_id?.value || route.service_id
  const serviceMatch = services.value.find(s => s.id === serviceId)

  editingRoute.value = {
    ...route,
    paths: Array.isArray(route.paths) ? route.paths.join(', ') : route.paths || '',
    methods: Array.isArray(route.methods) ? route.methods.join(', ') : route.methods || '',
    hosts: Array.isArray(route.hosts) ? route.hosts.join(', ') : route.hosts || '',
    service_id: serviceMatch
      ? { value: serviceMatch.id, label: serviceMatch.name }
      : serviceId
        ? { value: serviceId, label: route.service_name || serviceId }
        : null
  }
  isEditRouteModalActive.value = true
}

const updateRoute = async () => {
  try {
    const routeData = {
      ...editingRoute.value,
      paths: editingRoute.value.paths.split(',').map(p => p.trim()).filter(p => p),
      methods: editingRoute.value.methods ? editingRoute.value.methods.split(',').map(m => m.trim().toUpperCase()).filter(m => m) : [],
      hosts: editingRoute.value.hosts ? editingRoute.value.hosts.split(',').map(h => h.trim()).filter(h => h) : [],
      service_id: editingRoute.value.service_id?.value || editingRoute.value.service_id
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
    }
    isDeleteModalActive.value = false
    await loadData()
  } catch (error) {
    console.error('Failed to delete:', error)
  }
}

const openLetsEncryptModal = () => {
  if (certificateInfo.value.lets_encrypt_email) {
    letsEncryptConfig.value = {
      enabled: certificateInfo.value.lets_encrypt || false,
      email: certificateInfo.value.lets_encrypt_email || '',
      domains: (certificateInfo.value.lets_encrypt_domains || []).join(', '),
      staging: certificateInfo.value.lets_encrypt_staging || true,
      auto_renew: certificateInfo.value.auto_renew !== false,
      renew_before_days: certificateInfo.value.renew_before_days || 30
    }
  }
  isLetsEncryptModalActive.value = true
}

const saveLetsEncrypt = async () => {
  try {
    const config = {
      ...letsEncryptConfig.value,
      domains: letsEncryptConfig.value.domains.split(',').map(d => d.trim()).filter(d => d)
    }
    await ApiService.apiGatewayConfigureLetsEncrypt(config)
    isLetsEncryptModalActive.value = false
    await loadData()
  } catch (error) {
    console.error('Failed to save Let\'s Encrypt config:', error)
  }
}

const requestCertificate = async () => {
  try {
    await ApiService.apiGatewayRequestCertificate()
    await loadData()
  } catch (error) {
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
    gatewayConfig.value = {
      http_port: cfg.http_port || 80,
      https_port: cfg.https_port || 443,
      https_enabled: cfg.https_enabled || false,
      access_log_enabled: cfg.access_log_enabled !== false
    }
    isConfigModalActive.value = true
  } catch (error) {
    console.error('Failed to load config:', error)
  }
}

const saveConfig = async () => {
  try {
    const currentConfig = (await ApiService.apiGatewayGetConfig()).data.data || {}
    const updatedConfig = {
      ...currentConfig,
      ...gatewayConfig.value
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

const getHealthColor = (healthy) => {
  if (healthy === null) return 'text-gray-500'
  return healthy ? 'text-green-500' : 'text-red-500'
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
            API Gateway
          </h1>
          <p class="text-blue-100 text-lg">Manage HTTP traffic routing, rate limiting, and SSL certificates</p>
        </div>
        <div class="mt-6 lg:mt-0 flex flex-wrap gap-3">
          <BaseButton
            v-if="!status.running"
            label="Start Gateway"
            :icon="mdiPlay"
            color="white"
            class="shadow-lg hover:shadow-xl transform hover:scale-105 transition-all duration-200"
            @click="startGateway"
          />
          <BaseButton
            v-else
            label="Stop Gateway"
            :icon="mdiStop"
            color="danger"
            class="shadow-lg hover:shadow-xl transform hover:scale-105 transition-all duration-200"
            @click="stopGateway"
          />
          <BaseButton
            label="Settings"
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
          {{ status.running ? 'Running' : 'Stopped' }}
        </span>
        <span v-if="status.running" class="text-blue-100 text-sm">
          HTTP: {{ status.http_port }} | HTTPS: {{ status.https_enabled ? status.https_port : 'Disabled' }}
        </span>
      </div>
    </div>

    <!-- Statistics -->
    <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
      <CardBox class="bg-gradient-to-br from-blue-50 to-blue-100 dark:from-blue-900/20 dark:to-blue-800/20 border-blue-200 dark:border-blue-700">
        <div class="flex items-center justify-between">
          <div>
            <div class="text-2xl font-bold text-blue-600 dark:text-blue-400">{{ gatewayStats.totalServices }}</div>
            <div class="text-sm text-blue-600/70">Services</div>
          </div>
          <BaseIcon :path="mdiServer" size="36" class="text-blue-500 opacity-20" />
        </div>
      </CardBox>

      <CardBox class="bg-gradient-to-br from-green-50 to-green-100 dark:from-green-900/20 dark:to-green-800/20 border-green-200 dark:border-green-700">
        <div class="flex items-center justify-between">
          <div>
            <div class="text-2xl font-bold text-green-600 dark:text-green-400">{{ gatewayStats.healthyServices }}/{{ gatewayStats.totalServices }}</div>
            <div class="text-sm text-green-600/70">Healthy</div>
          </div>
          <BaseIcon :path="mdiHeartPulse" size="36" class="text-green-500 opacity-20" />
        </div>
      </CardBox>

      <CardBox class="bg-gradient-to-br from-purple-50 to-purple-100 dark:from-purple-900/20 dark:to-purple-800/20 border-purple-200 dark:border-purple-700">
        <div class="flex items-center justify-between">
          <div>
            <div class="text-2xl font-bold text-purple-600 dark:text-purple-400">{{ gatewayStats.totalRoutes }}</div>
            <div class="text-sm text-purple-600/70">Routes</div>
          </div>
          <BaseIcon :path="mdiWeb" size="36" class="text-purple-500 opacity-20" />
        </div>
      </CardBox>

      <CardBox class="bg-gradient-to-br from-orange-50 to-orange-100 dark:from-orange-900/20 dark:to-orange-800/20 border-orange-200 dark:border-orange-700">
        <div class="flex items-center justify-between">
          <div>
            <div class="text-2xl font-bold text-orange-600 dark:text-orange-400">{{ gatewayStats.totalRequests }}</div>
            <div class="text-sm text-orange-600/70">Requests</div>
          </div>
          <BaseIcon :path="mdiSpeedometer" size="36" class="text-orange-500 opacity-20" />
        </div>
      </CardBox>
    </div>

    <!-- Tabs -->
    <div class="flex border-b border-gray-200 dark:border-gray-700">
      <button
        v-for="tab in ['overview', 'services', 'routes', 'certificates', 'observability']"
        :key="tab"
        :class="[
          'px-6 py-3 font-medium text-sm border-b-2 transition-colors capitalize',
          activeTab === tab
            ? 'border-blue-500 text-blue-600 dark:text-blue-400'
            : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'
        ]"
        @click="activeTab = tab"
      >
        {{ tab }}
      </button>
    </div>

    <!-- Overview Tab -->
    <div v-if="activeTab === 'overview'" class="grid grid-cols-1 lg:grid-cols-2 gap-6">
      <!-- Performance Stats -->
      <CardBox>
        <SectionTitleLineWithButton :icon="mdiSpeedometer" title="Performance" main />
        <div class="grid grid-cols-2 gap-4 mt-4">
          <div class="p-4 bg-slate-50 dark:bg-slate-800/50 rounded-lg">
            <div class="text-2xl font-bold text-slate-800 dark:text-slate-200">{{ gatewayStats.avgLatency }}ms</div>
            <div class="text-sm text-slate-500">Avg Latency</div>
          </div>
          <div class="p-4 bg-slate-50 dark:bg-slate-800/50 rounded-lg">
            <div class="text-2xl font-bold text-slate-800 dark:text-slate-200">{{ gatewayStats.uptime }}</div>
            <div class="text-sm text-slate-500">Uptime</div>
          </div>
          <div class="p-4 bg-slate-50 dark:bg-slate-800/50 rounded-lg">
            <div class="text-2xl font-bold text-slate-800 dark:text-slate-200">{{ gatewayStats.totalRequests }}</div>
            <div class="text-sm text-slate-500">Total Requests</div>
          </div>
          <div class="p-4 bg-slate-50 dark:bg-slate-800/50 rounded-lg">
            <div class="text-2xl font-bold text-red-600 dark:text-red-400">{{ gatewayStats.totalErrors }}</div>
            <div class="text-sm text-slate-500">Total Errors</div>
          </div>
        </div>
      </CardBox>

      <!-- Service Health -->
      <CardBox>
        <SectionTitleLineWithButton :icon="mdiHeartPulse" title="Service Health" main />
        <div class="space-y-3 mt-4">
          <div v-if="serviceHealth.length === 0" class="text-center py-8 text-slate-500">
            No health data available
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
                {{ health.healthy ? 'Healthy' : 'Unhealthy' }}
              </span>
            </div>
          </div>
        </div>
      </CardBox>
    </div>

    <!-- Services Tab -->
    <CardBox v-if="activeTab === 'services'">
      <SectionTitleLineWithButton :icon="mdiServer" title="Upstream Services" main>
        <BaseButton
          label="Add Service"
          :icon="mdiPlus"
          color="info"
          small
          @click="openAddServiceModal"
        />
      </SectionTitleLineWithButton>

      <div v-if="services.length === 0" class="text-center py-12">
        <BaseIcon :path="mdiServer" size="64" class="mx-auto text-slate-300 dark:text-slate-600 mb-4" />
        <p class="text-slate-500 mb-4">No services configured</p>
        <BaseButton label="Add Your First Service" :icon="mdiPlus" color="info" @click="openAddServiceModal" />
      </div>

      <div v-else class="grid grid-cols-1 md:grid-cols-2 gap-4 mt-4">
        <div
          v-for="service in services"
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
              <span>Timeout: {{ service.timeout }}s</span>
            </div>
            <div class="flex gap-2">
              <BaseButton :icon="mdiPencil" color="info" small @click="openEditServiceModal(service)" />
              <BaseButton :icon="mdiDelete" color="danger" small @click="confirmDelete('service', service)" />
            </div>
          </div>
        </div>
      </div>
    </CardBox>

    <!-- Routes Tab -->
    <CardBox v-if="activeTab === 'routes'">
      <SectionTitleLineWithButton :icon="mdiWeb" title="Routing Rules" main>
        <BaseButton
          label="Add Route"
          :icon="mdiPlus"
          color="info"
          small
          @click="openAddRouteModal"
        />
      </SectionTitleLineWithButton>

      <div v-if="routes.length === 0" class="text-center py-12">
        <BaseIcon :path="mdiWeb" size="64" class="mx-auto text-slate-300 dark:text-slate-600 mb-4" />
        <p class="text-slate-500 mb-4">No routes configured</p>
        <BaseButton label="Add Your First Route" :icon="mdiPlus" color="info" @click="openAddRouteModal" />
      </div>

      <div v-else class="space-y-4 mt-4">
        <div
          v-for="route in routes"
          :key="route.id"
          class="p-4 bg-slate-50 dark:bg-slate-800/50 rounded-xl"
        >
          <div class="flex items-start justify-between">
            <div>
              <h3 class="font-semibold flex items-center gap-2">
                {{ route.name }}
                <span 
                  :class="[
                    'px-2 py-0.5 text-xs rounded',
                    route.enabled ? 'bg-green-100 text-green-700' : 'bg-gray-100 text-gray-700'
                  ]"
                >
                  {{ route.enabled ? 'Active' : 'Disabled' }}
                </span>
              </h3>
              <p class="text-sm text-slate-500 mt-1">
                <span class="font-mono">{{ route.paths?.join(', ') }}</span>
                → {{ getServiceName(route.service_id) }}
              </p>
            </div>
            <div class="flex gap-2">
              <BaseButton :icon="mdiPencil" color="info" small @click="openEditRouteModal(route)" />
              <BaseButton :icon="mdiDelete" color="danger" small @click="confirmDelete('route', route)" />
            </div>
          </div>
          <div class="mt-3 flex flex-wrap gap-2">
            <span v-if="route.rate_limit_enabled" class="px-2 py-1 bg-orange-100 dark:bg-orange-900/30 text-orange-700 dark:text-orange-300 text-xs rounded">
              Rate Limited
            </span>
            <span v-if="route.auth_required" class="px-2 py-1 bg-purple-100 dark:bg-purple-900/30 text-purple-700 dark:text-purple-300 text-xs rounded">
              Auth: {{ route.auth_type }}
            </span>
            <span v-if="route.strip_path" class="px-2 py-1 bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300 text-xs rounded">
              Strip Path
            </span>
          </div>
        </div>
      </div>
    </CardBox>

    <!-- Certificates Tab -->
    <CardBox v-if="activeTab === 'certificates'">
      <SectionTitleLineWithButton :icon="mdiCertificate" title="SSL/TLS Certificates" main>
        <BaseButton
          label="Configure Let's Encrypt"
          :icon="mdiLock"
          color="info"
          small
          @click="openLetsEncryptModal"
        />
      </SectionTitleLineWithButton>

      <div class="grid grid-cols-1 lg:grid-cols-2 gap-6 mt-4">
        <!-- Current Certificate -->
        <div class="p-6 bg-slate-50 dark:bg-slate-800/50 rounded-xl">
          <h3 class="font-semibold mb-4 flex items-center gap-2">
            <BaseIcon :path="mdiShield" size="20" />
            Current Certificate
          </h3>
          <div v-if="certificateInfo.cert_subject" class="space-y-3">
            <div class="flex justify-between">
              <span class="text-slate-500">Subject</span>
              <span class="font-medium">{{ certificateInfo.cert_subject }}</span>
            </div>
            <div class="flex justify-between">
              <span class="text-slate-500">Issuer</span>
              <span class="font-medium">{{ certificateInfo.cert_issuer }}</span>
            </div>
            <div class="flex justify-between">
              <span class="text-slate-500">Valid Until</span>
              <span class="font-medium">{{ certificateInfo.cert_not_after }}</span>
            </div>
            <div class="flex justify-between">
              <span class="text-slate-500">Status</span>
              <span :class="certificateInfo.cert_valid ? 'text-green-500' : 'text-red-500'">
                {{ certificateInfo.cert_valid ? 'Valid' : 'Invalid/Expired' }}
              </span>
            </div>
          </div>
          <div v-else class="text-center py-8 text-slate-500">
            No certificate configured
          </div>
        </div>

        <!-- Let's Encrypt Status -->
        <div class="p-6 bg-slate-50 dark:bg-slate-800/50 rounded-xl">
          <h3 class="font-semibold mb-4 flex items-center gap-2">
            <BaseIcon :path="mdiSync" size="20" />
            Auto-Renewal Status
          </h3>
          <div class="space-y-4">
            <div class="flex items-center justify-between">
              <span class="text-slate-500">Let's Encrypt</span>
              <span :class="certificateInfo.lets_encrypt ? 'text-green-500' : 'text-gray-500'">
                {{ certificateInfo.lets_encrypt ? 'Enabled' : 'Disabled' }}
              </span>
            </div>
            <div class="flex items-center justify-between">
              <span class="text-slate-500">Auto-Renew Scheduler</span>
              <span :class="renewerStatus.running ? 'text-green-500' : 'text-gray-500'">
                {{ renewerStatus.running ? 'Running' : 'Stopped' }}
              </span>
            </div>
            <div v-if="certificateInfo.expires_at" class="flex items-center justify-between">
              <span class="text-slate-500">Expires</span>
              <span>{{ certificateInfo.expires_at }}</span>
            </div>
            <div class="pt-4 flex gap-2">
              <BaseButton
                :label="renewerStatus.running ? 'Stop Scheduler' : 'Start Scheduler'"
                :icon="renewerStatus.running ? mdiStop : mdiPlay"
                :color="renewerStatus.running ? 'danger' : 'success'"
                small
                @click="toggleRenewer"
              />
              <BaseButton
                label="Request Certificate"
                :icon="mdiCertificate"
                color="info"
                small
                @click="requestCertificate"
              />
            </div>
          </div>
        </div>
      </div>
    </CardBox>

    <!-- Observability Tab -->
    <CardBox v-if="activeTab === 'observability'">
      <SectionTitleLineWithButton :icon="mdiChartLine" title="Observability & Telemetry" main>
        <BaseButton
          label="Configure"
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
              <span class="text-slate-500">Status</span>
              <span :class="observabilityConfig.loki_enabled ? 'text-green-500' : 'text-gray-500'">
                {{ observabilityConfig.loki_enabled ? 'Enabled' : 'Disabled' }}
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
              <span class="text-slate-500">Status</span>
              <span :class="observabilityConfig.influx_enabled ? 'text-green-500' : 'text-gray-500'">
                {{ observabilityConfig.influx_enabled ? 'Enabled' : 'Disabled' }}
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
              <span class="text-slate-500">Status</span>
              <span :class="observabilityConfig.graylog_enabled ? 'text-green-500' : 'text-gray-500'">
                {{ observabilityConfig.graylog_enabled ? 'Enabled' : 'Disabled' }}
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
              <span class="text-slate-500">Status</span>
              <span :class="observabilityConfig.otlp_enabled ? 'text-green-500' : 'text-gray-500'">
                {{ observabilityConfig.otlp_enabled ? 'Enabled' : 'Disabled' }}
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
              <span class="text-slate-500">Status</span>
              <span :class="observabilityConfig.clickhouse_enabled ? 'text-green-500' : 'text-gray-500'">
                {{ observabilityConfig.clickhouse_enabled ? 'Enabled' : 'Disabled' }}
              </span>
            </div>
            <div v-if="observabilityConfig.clickhouse_endpoint" class="text-sm text-slate-500 truncate">
              {{ observabilityConfig.clickhouse_endpoint }}
            </div>
          </div>
        </div>
      </div>

      <div class="mt-6 p-4 bg-blue-50 dark:bg-blue-900/20 rounded-lg">
        <p class="text-sm text-blue-800 dark:text-blue-200">
          <strong>Note:</strong> When enabled, all request and response data will be sent to the configured endpoints.
          This includes method, path, status code, latency, service ID, and route ID.
        </p>
      </div>
    </CardBox>

    <!-- Add Service Modal -->
    <CardBoxModal 
      v-model="isAddServiceModalActive" 
      title="Add Upstream Service" 
      button="success" 
      button-label="Add Service"
      has-cancel
      @confirm="addService"
    >
      <div class="space-y-4">
        <FormField label="Service Name">
          <FormControl v-model="newService.name" placeholder="my-service" />
        </FormField>
        <div class="grid grid-cols-2 gap-4">
          <FormField label="Host">
            <FormControl v-model="newService.host" placeholder="localhost or IP" />
          </FormField>
          <FormField label="Port">
            <FormControl v-model="newService.port" type="number" placeholder="80" />
          </FormField>
        </div>
        <div class="grid grid-cols-2 gap-4">
          <FormField label="Protocol">
            <FormControl v-model="newService.protocol" :options="['http', 'https']" />
          </FormField>
          <FormField label="Timeout (seconds)">
            <FormControl v-model="newService.timeout" type="number" placeholder="30" />
          </FormField>
        </div>
        <FormField label="Base Path (optional)">
          <FormControl v-model="newService.path" placeholder="/api" />
        </FormField>
        <FormField label="Health Check Path">
          <FormControl v-model="newService.health_check.path" placeholder="/health" />
        </FormField>
      </div>
    </CardBoxModal>

    <!-- Add Route Modal -->
    <CardBoxModal 
      v-model="isAddRouteModalActive" 
      title="Add Route" 
      button="success" 
      button-label="Add Route"
      has-cancel
      @confirm="addRoute"
    >
      <div class="space-y-4">
        <FormField label="Route Name">
          <FormControl v-model="newRoute.name" placeholder="my-route" />
        </FormField>
        <FormField label="Service">
          <FormControl v-model="newRoute.service_id" :options="services.length ? services.map(s => ({ value: s.id, label: s.name })) : []" />
        </FormField>
        <FormField label="Paths (comma-separated)">
          <FormControl v-model="newRoute.paths" placeholder="/api, /api/*" />
        </FormField>
        <FormField label="Hosts (comma-separated, optional)">
          <FormControl v-model="newRoute.hosts" placeholder="api.example.com" />
        </FormField>
        <FormField label="Methods (comma-separated, optional)">
          <FormControl v-model="newRoute.methods" placeholder="GET, POST, PUT" />
        </FormField>
        <div class="grid grid-cols-2 gap-4">
          <FormField>
            <FormCheckRadio v-model="newRoute.strip_path" label="Strip Path" name="strip_path" />
          </FormField>
          <FormField>
            <FormCheckRadio v-model="newRoute.rate_limit_enabled" label="Enable Rate Limiting" name="rate_limit" />
          </FormField>
        </div>
        <div v-if="newRoute.rate_limit_enabled" class="grid grid-cols-2 gap-4">
          <FormField label="Requests">
            <FormControl v-model="newRoute.rate_limit_requests" type="number" placeholder="100" />
          </FormField>
          <FormField label="Window (seconds)">
            <FormControl v-model="newRoute.rate_limit_window" type="number" placeholder="60" />
          </FormField>
        </div>
      </div>
    </CardBoxModal>

    <!-- Let's Encrypt Modal -->
    <CardBoxModal 
      v-model="isLetsEncryptModalActive" 
      title="Configure Let's Encrypt" 
      button="success" 
      button-label="Save Configuration"
      has-cancel
      @confirm="saveLetsEncrypt"
    >
      <div class="space-y-4">
        <FormField>
          <FormCheckRadio v-model="letsEncryptConfig.enabled" label="Enable Let's Encrypt" name="le_enabled" />
        </FormField>
        <FormField label="Email Address">
          <FormControl v-model="letsEncryptConfig.email" type="email" placeholder="admin@example.com" />
        </FormField>
        <FormField label="Domains (comma-separated)">
          <FormControl v-model="letsEncryptConfig.domains" placeholder="example.com, www.example.com" />
        </FormField>
        <FormField>
          <FormCheckRadio v-model="letsEncryptConfig.staging" label="Use Staging Server (for testing)" name="le_staging" />
        </FormField>
        <FormField>
          <FormCheckRadio v-model="letsEncryptConfig.auto_renew" label="Auto-Renew Certificates" name="le_auto_renew" />
        </FormField>
        <FormField label="Renew Before Expiry (days)">
          <FormControl v-model="letsEncryptConfig.renew_before_days" type="number" placeholder="30" />
        </FormField>
      </div>
    </CardBoxModal>

    <!-- Config Modal -->
    <CardBoxModal 
      v-model="isConfigModalActive" 
      title="Gateway Settings" 
      button="success" 
      button-label="Save Settings"
      has-cancel
      @confirm="saveConfig"
    >
      <div class="space-y-4">
        <div class="grid grid-cols-2 gap-4">
          <FormField label="HTTP Port">
            <FormControl v-model="gatewayConfig.http_port" type="number" placeholder="80" />
          </FormField>
          <FormField label="HTTPS Port">
            <FormControl v-model="gatewayConfig.https_port" type="number" placeholder="443" />
          </FormField>
        </div>
        <FormField>
          <FormCheckRadio v-model="gatewayConfig.https_enabled" label="Enable HTTPS" name="https_enabled" />
        </FormField>
        <FormField>
          <FormCheckRadio v-model="gatewayConfig.access_log_enabled" label="Enable Access Logging" name="access_log" />
        </FormField>
      </div>
    </CardBoxModal>

    <!-- Edit Service Modal -->
    <CardBoxModal 
      v-model="isEditServiceModalActive" 
      title="Edit Service" 
      button="success" 
      button-label="Update Service"
      has-cancel
      @confirm="updateService"
    >
      <div v-if="editingService" class="space-y-4">
        <FormField label="Service Name">
          <FormControl v-model="editingService.name" placeholder="my-service" />
        </FormField>
        <div class="grid grid-cols-2 gap-4">
          <FormField label="Host">
            <FormControl v-model="editingService.host" placeholder="localhost or IP" />
          </FormField>
          <FormField label="Port">
            <FormControl v-model="editingService.port" type="number" placeholder="80" />
          </FormField>
        </div>
        <div class="grid grid-cols-2 gap-4">
          <FormField label="Protocol">
            <FormControl v-model="editingService.protocol" :options="['http', 'https']" />
          </FormField>
          <FormField label="Timeout (seconds)">
            <FormControl v-model="editingService.timeout" type="number" placeholder="30" />
          </FormField>
        </div>
        <FormField label="Base Path (optional)">
          <FormControl v-model="editingService.path" placeholder="/api" />
        </FormField>
        <FormField>
          <FormCheckRadio v-model="editingService.enabled" label="Enabled" name="edit_service_enabled" />
        </FormField>
        <FormField label="Health Check Path">
          <FormControl v-model="editingService.health_check.path" placeholder="/health" />
        </FormField>
      </div>
    </CardBoxModal>

    <!-- Edit Route Modal -->
    <CardBoxModal 
      v-model="isEditRouteModalActive" 
      title="Edit Route" 
      button="success" 
      button-label="Update Route"
      has-cancel
      @confirm="updateRoute"
    >
      <div v-if="editingRoute" class="space-y-4">
        <FormField label="Route Name">
          <FormControl v-model="editingRoute.name" placeholder="my-route" />
        </FormField>
        <FormField label="Service">
          <FormControl v-model="editingRoute.service_id" :options="services.length ? services.map(s => ({ value: s.id, label: s.name })) : []" />
        </FormField>
        <FormField label="Paths (comma-separated)">
          <FormControl v-model="editingRoute.paths" placeholder="/api, /api/*" />
        </FormField>
        <FormField label="Hosts (comma-separated, optional)">
          <FormControl v-model="editingRoute.hosts" placeholder="api.example.com" />
        </FormField>
        <FormField label="Methods (comma-separated, optional)">
          <FormControl v-model="editingRoute.methods" placeholder="GET, POST, PUT" />
        </FormField>
        <div class="grid grid-cols-2 gap-4">
          <FormField>
            <FormCheckRadio v-model="editingRoute.strip_path" label="Strip Path" name="edit_strip_path" />
          </FormField>
          <FormField>
            <FormCheckRadio v-model="editingRoute.enabled" label="Enabled" name="edit_route_enabled" />
          </FormField>
        </div>
        <div class="grid grid-cols-2 gap-4">
          <FormField>
            <FormCheckRadio v-model="editingRoute.rate_limit_enabled" label="Enable Rate Limiting" name="edit_rate_limit" />
          </FormField>
          <FormField>
            <FormCheckRadio v-model="editingRoute.auth_required" label="Require Auth" name="edit_auth" />
          </FormField>
        </div>
        <div v-if="editingRoute.rate_limit_enabled" class="grid grid-cols-2 gap-4">
          <FormField label="Requests">
            <FormControl v-model="editingRoute.rate_limit_requests" type="number" placeholder="100" />
          </FormField>
          <FormField label="Window (seconds)">
            <FormControl v-model="editingRoute.rate_limit_window" type="number" placeholder="60" />
          </FormField>
        </div>
      </div>
    </CardBoxModal>

    <!-- Observability Modal -->
    <CardBoxModal 
      v-model="isObservabilityModalActive" 
      title="Configure Observability" 
      button="success" 
      button-label="Save Configuration"
      has-cancel
      @confirm="saveObservability"
    >
      <div class="space-y-4">
        <FormField>
          <FormCheckRadio v-model="observabilityConfig.enabled" label="Enable Observability" name="obs_enabled" />
        </FormField>
        
        <div class="border-t pt-4 mt-4">
          <h4 class="font-semibold mb-3">Loki</h4>
          <FormField>
            <FormCheckRadio v-model="observabilityConfig.loki_enabled" label="Enable Loki" name="loki_enabled" />
          </FormField>
          <FormField label="Loki Endpoint">
            <FormControl v-model="observabilityConfig.loki.url" placeholder="http://loki:3100/loki/api/v1/push" />
          </FormField>
          <FormField label="Tenant ID (optional)">
            <FormControl v-model="observabilityConfig.loki.tenant_id" placeholder="acme-org" />
          </FormField>
          <FormField label="API Key (optional)">
            <FormControl v-model="observabilityConfig.loki.api_key" type="password" placeholder="API Key" />
          </FormField>
        </div>

        <div class="border-t pt-4 mt-4">
          <h4 class="font-semibold mb-3">InfluxDB</h4>
          <FormField>
            <FormCheckRadio v-model="observabilityConfig.influx_enabled" label="Enable InfluxDB" name="influx_enabled" />
          </FormField>
          <FormField label="InfluxDB URL">
            <FormControl v-model="observabilityConfig.influx.url" placeholder="http://influxdb:8086" />
          </FormField>
          <div class="grid grid-cols-2 gap-4">
            <FormField label="Organization">
              <FormControl v-model="observabilityConfig.influx.org" placeholder="my-org" />
            </FormField>
            <FormField label="Bucket">
              <FormControl v-model="observabilityConfig.influx.bucket" placeholder="api_gateway" />
            </FormField>
          </div>
          <FormField label="Access Token">
            <FormControl v-model="observabilityConfig.influx.token" type="password" placeholder="Token" />
          </FormField>
        </div>

        <div class="border-t pt-4 mt-4">
          <h4 class="font-semibold mb-3">Graylog</h4>
          <FormField>
            <FormCheckRadio v-model="observabilityConfig.graylog_enabled" label="Enable Graylog" name="graylog_enabled" />
          </FormField>
          <FormField label="Graylog Endpoint">
            <FormControl v-model="observabilityConfig.graylog.endpoint" placeholder="http://graylog:12201/gelf" />
          </FormField>
          <FormField label="Stream ID (optional)">
            <FormControl v-model="observabilityConfig.graylog.stream_id" placeholder="graylog-stream-id" />
          </FormField>
          <div class="grid grid-cols-2 gap-4">
            <FormField label="API Key (optional)">
              <FormControl v-model="observabilityConfig.graylog.api_key" type="password" placeholder="API Key" />
            </FormField>
            <FormField label="API Key Header">
              <FormControl v-model="observabilityConfig.graylog.api_key_header" placeholder="Authorization" />
            </FormField>
          </div>
        </div>

        <div class="border-t pt-4 mt-4">
          <h4 class="font-semibold mb-3">OpenTelemetry (OTLP)</h4>
          <FormField>
            <FormCheckRadio v-model="observabilityConfig.otlp_enabled" label="Enable OTLP" name="otlp_enabled" />
          </FormField>
          <FormField label="OTLP Endpoint">
            <FormControl v-model="observabilityConfig.otlp_endpoint" placeholder="http://otel-collector:4318" />
          </FormField>
        </div>

        <div class="border-t pt-4 mt-4">
          <h4 class="font-semibold mb-3">ClickHouse</h4>
          <FormField>
            <FormCheckRadio v-model="observabilityConfig.clickhouse_enabled" label="Enable ClickHouse" name="ch_enabled" />
          </FormField>
          <FormField label="ClickHouse Endpoint">
            <FormControl v-model="observabilityConfig.clickhouse_endpoint" placeholder="http://clickhouse:8123" />
          </FormField>
          <div class="grid grid-cols-2 gap-4">
            <FormField label="Database">
              <FormControl v-model="observabilityConfig.clickhouse_database" placeholder="default" />
            </FormField>
            <FormField label="Table">
              <FormControl v-model="observabilityConfig.clickhouse_table" placeholder="api_gateway_logs" />
            </FormField>
          </div>
          <div class="grid grid-cols-2 gap-4">
            <FormField label="Username">
              <FormControl v-model="observabilityConfig.clickhouse_username" placeholder="default" />
            </FormField>
            <FormField label="Password">
              <FormControl v-model="observabilityConfig.clickhouse_password" type="password" />
            </FormField>
          </div>
        </div>

        <div class="border-t pt-4 mt-4">
          <h4 class="font-semibold mb-3">Batching</h4>
          <div class="grid grid-cols-2 gap-4">
            <FormField label="Batch Size">
              <FormControl v-model="observabilityConfig.batch_size" type="number" placeholder="100" />
            </FormField>
            <FormField label="Flush Interval (seconds)">
              <FormControl v-model="observabilityConfig.flush_interval" type="number" placeholder="30" />
            </FormField>
          </div>
        </div>
      </div>
    </CardBoxModal>

    <!-- Delete Confirmation Modal -->
    <CardBoxModal 
      v-model="isDeleteModalActive" 
      title="Confirm Delete" 
      button="danger" 
      button-label="Delete"
      has-cancel
      @confirm="deleteItem"
    >
      <p class="text-slate-600 dark:text-slate-400">
        Are you sure you want to delete this {{ deleteTarget.type }}?
        <strong v-if="deleteTarget.item">{{ deleteTarget.item.name }}</strong>
      </p>
    </CardBoxModal>
  </div>
</template>
