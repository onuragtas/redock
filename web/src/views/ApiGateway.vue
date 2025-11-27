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
  mdiArrowRight,
  mdiCertificate,
  mdiChevronLeft,
  mdiChevronRight,
  mdiCog,
  mdiDelete,
  mdiEarth,
  mdiHeart,
  mdiHeartPulse,
  mdiLock,
  mdiMagnify,
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
  mdiViewGridOutline,
  mdiViewList,
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
const isAddRouteModalActive = ref(false)
const isDeleteModalActive = ref(false)
const isLetsEncryptModalActive = ref(false)
const isConfigModalActive = ref(false)
const deleteTarget = ref({ type: '', item: null })

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

const gatewayConfig = ref({
  http_port: 80,
  https_port: 443,
  https_enabled: false,
  access_log_enabled: true
})

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
    await ApiService.apiGatewayAddService(newService.value)
    isAddServiceModalActive.value = false
    await loadData()
  } catch (error) {
    console.error('Failed to add service:', error)
  }
}

const openAddRouteModal = () => {
  newRoute.value = {
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
  }
  isAddRouteModalActive.value = true
}

const addRoute = async () => {
  try {
    const routeData = {
      ...newRoute.value,
      paths: newRoute.value.paths.split(',').map(p => p.trim()).filter(p => p),
      methods: newRoute.value.methods ? newRoute.value.methods.split(',').map(m => m.trim().toUpperCase()).filter(m => m) : [],
      hosts: newRoute.value.hosts ? newRoute.value.hosts.split(',').map(h => h.trim()).filter(h => h) : []
    }
    await ApiService.apiGatewayAddRoute(routeData)
    isAddRouteModalActive.value = false
    await loadData()
  } catch (error) {
    console.error('Failed to add route:', error)
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

const getServiceHealth = (serviceId) => {
  const health = serviceHealth.value.find(h => h.service_id === serviceId)
  return health ? health.healthy : null
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
        v-for="tab in ['overview', 'services', 'routes', 'certificates']"
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
              <span class="font-medium">{{ health.service_id }}</span>
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
              <BaseButton :icon="mdiPencil" color="info" small />
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
                → {{ route.service_id }}
              </p>
            </div>
            <div class="flex gap-2">
              <BaseButton :icon="mdiPencil" color="info" small />
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
