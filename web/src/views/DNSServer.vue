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
  mdiCog,
  mdiDatabase,
  mdiDelete,
  mdiDns,
  mdiMagnify,
  mdiPencil,
  mdiPlay,
  mdiPlus,
  mdiRefresh,
  mdiShield,
  mdiShieldCheck,
  mdiSpeedometer,
  mdiStop,
  mdiWeb
} from '@mdi/js';
import { computed, onMounted, onUnmounted, ref } from "vue";
import { useToast } from 'vue-toastification';

const toast = useToast();

// Reactive state
const loading = ref(false)
const status = ref({ running: false })
const stats = ref({
  total_queries_24h: 0,
  blocked_queries_24h: 0,
  block_percentage: 0,
  queries_per_minute: 0,
  avg_response_time: 0,
  active_clients: 0,
  cache_hit_rate: 0
})
const config = ref({
  enabled: false,
  udp_port: 53,
  tcp_port: 53,
  doh_enabled: false,
  doh_port: 443,
  dot_enabled: false,
  dot_port: 853,
  upstream_dns: '["1.1.1.1:53","8.8.8.8:53"]',
  blocking_enabled: true,
  query_logging: true,
  log_retention_days: 7,
  cache_enabled: true,
  cache_ttl: 3600,
  rate_limit_enabled: false,
  rate_limit_qps: 100
})
const blocklists = ref([])
const customFilters = ref([])
const rewrites = ref([])
const queryLogs = ref([])
const clients = ref([])
const customRules = ref({
  global_filters: [],
  client_rules: [],
  banned_clients: []
})
const searchQuery = ref('') // Log arama filtresi

// Modal states
const activeTab = ref('overview')
const isAddBlocklistModalActive = ref(false)
const isEditBlocklistModalActive = ref(false)
const isAddFilterModalActive = ref(false)
const isAddRewriteModalActive = ref(false)
const isEditRewriteModalActive = ref(false)
const isConfigModalActive = ref(false)
const isDeleteModalActive = ref(false)
const deleteTarget = ref({ type: '', item: null })
const editingBlocklist = ref(null)
const editingRewrite = ref(null)
const selectedResponse = ref(null)
const activeLogMenu = ref(null) // Active log dropdown menu
const activeClientMenu = ref(null) // Active client dropdown menu
const currentLogStatus = ref(null) // Real-time status for active log menu

// Form data
const newBlocklist = ref({
  name: '',
  url: '',
  enabled: true,
  update_interval: 86400
})

const newFilter = ref({
  domain: '',
  type: 'blacklist',
  comment: '',
  is_regex: false,
  is_wildcard: false
})

const newRewrite = ref({
  domain: '',
  answer: '',
  type: 'A',
  comment: '',
  enabled: true
})

// Auto-refresh interval
let refreshInterval = null

// Computed properties
const blockPercentage = computed(() => {
  if (stats.value.total_queries_24h === 0) return 0
  return ((stats.value.blocked_queries_24h / stats.value.total_queries_24h) * 100).toFixed(2)
})

const filteredQueryLogs = computed(() => {
  if (!searchQuery.value.trim()) {
    return queryLogs.value
  }
  
  const query = searchQuery.value.toLowerCase().trim()
  return queryLogs.value.filter(log => {
    return (
      log.client_ip?.toLowerCase().includes(query) ||
      log.domain?.toLowerCase().includes(query) ||
      log.response?.toLowerCase().includes(query)
    )
  })
})

const formattedUpstreamDNS = computed({
  get: () => {
    try {
      const parsed = JSON.parse(config.value.upstream_dns || '[]')
      return parsed.join('\n')
    } catch {
      return ''
    }
  },
  set: (value) => {
    const lines = value.split('\n').map(l => l.trim()).filter(l => l)
    config.value.upstream_dns = JSON.stringify(lines)
  }
})

// API Methods
const fetchStatus = async () => {
  try {
    const response = await ApiService.get('/v1/dns/status')
    if (response.data && !response.data.error) {
      status.value = response.data.data
    }
  } catch (error) {
    console.error('Failed to fetch DNS status:', error)
  }
}

const fetchConfig = async () => {
  try {
    const response = await ApiService.get('/v1/dns/config')
    if (response.data && !response.data.error) {
      config.value = response.data.data
    }
  } catch (error) {
    console.error('Failed to fetch DNS config:', error)
  }
}

const fetchStats = async () => {
  try {
    const response = await ApiService.get('/v1/dns/stats')
    if (response.data && !response.data.error) {
      stats.value = response.data.data
    }
  } catch (error) {
    console.error('Failed to fetch DNS stats:', error)
  }
}

const fetchBlocklists = async () => {
  try {
    const response = await ApiService.get('/v1/dns/blocklists')
    if (response.data && !response.data.error) {
      blocklists.value = response.data.data || []
    }
  } catch (error) {
    console.error('Failed to fetch blocklists:', error)
  }
}

const fetchCustomFilters = async () => {
  try {
    const response = await ApiService.get('/v1/dns/filters')
    if (response.data && !response.data.error) {
      customFilters.value = response.data.data || []
    }
  } catch (error) {
    console.error('Failed to fetch custom filters:', error)
  }
}

const fetchRewrites = async () => {
  try {
    const response = await ApiService.get('/v1/dns/rewrites')
    if (response.data && !response.data.error) {
      rewrites.value = response.data.rewrites || []
    }
  } catch (error) {
    console.error('Failed to fetch DNS rewrites:', error)
  }
}

const fetchQueryLogs = async () => {
  try {
    const response = await ApiService.get('/v1/dns/logs?limit=50')
    if (response.data && !response.data.error) {
      queryLogs.value = response.data.data.logs || []
    }
  } catch (error) {
    console.error('Failed to fetch query logs:', error)
  }
}

const fetchClients = async () => {
  try {
    const response = await ApiService.get('/v1/dns/clients')
    if (response.data && !response.data.error) {
      clients.value = response.data.data || []
    }
  } catch (error) {
    console.error('Failed to fetch clients:', error)
  }
}

const fetchCustomRules = async () => {
  try {
    const response = await ApiService.get('/v1/dns/custom-rules')
    if (response.data && !response.data.error) {
      customRules.value = response.data.data || {
        global_filters: [],
        client_rules: [],
        banned_clients: []
      }
    }
  } catch (error) {
    console.error('Failed to fetch custom rules:', error)
  }
}

const startServer = async () => {
  loading.value = true
  try {
    const response = await ApiService.post('/v1/dns/start')
    if (response.data && !response.data.error) {
      toast.success('DNS server started successfully')
      await fetchStatus()
    } else {
      toast.error('Failed to start DNS server: ' + (response.data.msg || 'Unknown error'))
    }
  } catch (error) {
    toast.error('Failed to start DNS server: ' + error.message)
  }
  loading.value = false
}

const stopServer = async () => {
  loading.value = true
  try {
    const response = await ApiService.post('/v1/dns/stop')
    if (response.data && !response.data.error) {
      toast.success('DNS server stopped successfully')
      await fetchStatus()
    } else {
      toast.error('Failed to stop DNS server: ' + (response.data.msg || 'Unknown error'))
    }
  } catch (error) {
    toast.error('Failed to stop DNS server: ' + error.message)
  }
  loading.value = false
}

const saveConfig = async () => {
  loading.value = true
  try {
    const response = await ApiService.put('/v1/dns/config', config.value)
    if (response.data && !response.data.error) {
      toast.success('Configuration saved successfully')
      await fetchConfig()
    } else {
      toast.error('Failed to save configuration: ' + (response.data.msg || 'Unknown error'))
    }
  } catch (error) {
    toast.error('Failed to save configuration: ' + error.message)
  }
  loading.value = false
  isConfigModalActive.value = false
}

const addBlocklist = async () => {
  loading.value = true
  try {
    const response = await ApiService.post('/v1/dns/blocklists', newBlocklist.value)
    if (response.data && !response.data.error) {
      toast.success('Blocklist added successfully')
      await fetchBlocklists()
      isAddBlocklistModalActive.value = false
      resetBlocklistForm()
    } else {
      toast.error('Failed to add blocklist: ' + (response.data.msg || 'Unknown error'))
    }
  } catch (error) {
    toast.error('Failed to add blocklist: ' + error.message)
  }
  loading.value = false
}

const updateBlocklist = async () => {
  loading.value = true
  try {
    const response = await ApiService.put(`/v1/dns/blocklists/${editingBlocklist.value.id}`, editingBlocklist.value)
    if (response.data && !response.data.error) {
      toast.success('Blocklist updated successfully')
      await fetchBlocklists()
      isEditBlocklistModalActive.value = false
      editingBlocklist.value = null
    } else {
      toast.error('Failed to update blocklist: ' + (response.data.msg || 'Unknown error'))
    }
  } catch (error) {
    toast.error('Failed to update blocklist: ' + error.message)
  }
  loading.value = false
}

const deleteBlocklist = async () => {
  loading.value = true
  try {
    const response = await ApiService.delete(`/v1/dns/blocklists/${deleteTarget.value.item.id}`)
    if (response.data && !response.data.error) {
      toast.success('Blocklist deleted successfully')
      await fetchBlocklists()
      isDeleteModalActive.value = false
    } else {
      toast.error('Failed to delete blocklist: ' + (response.data.msg || 'Unknown error'))
    }
  } catch (error) {
    toast.error('Failed to delete blocklist: ' + error.message)
  }
  loading.value = false
}

const addFilter = async () => {
  loading.value = true
  try {
    const response = await ApiService.post('/v1/dns/filters', newFilter.value)
    if (response.data && !response.data.error) {
      toast.success('Filter added successfully')
      await fetchCustomFilters()
      isAddFilterModalActive.value = false
      resetFilterForm()
    } else {
      toast.error('Failed to add filter: ' + (response.data.msg || 'Unknown error'))
    }
  } catch (error) {
    toast.error('Failed to add filter: ' + error.message)
  }
  loading.value = false
}

const deleteFilter = async () => {
  loading.value = true
  try {
    const response = await ApiService.delete(`/v1/dns/filters/${deleteTarget.value.item.id}`)
    if (response.data && !response.data.error) {
      toast.success('Filter deleted successfully')
      await fetchCustomFilters()
      isDeleteModalActive.value = false
    } else {
      toast.error('Failed to delete filter: ' + (response.data.msg || 'Unknown error'))
    }
  } catch (error) {
    toast.error('Failed to delete filter: ' + error.message)
  }
  loading.value = false
}

const addRewrite = async () => {
  loading.value = true
  try {
    const response = await ApiService.post('/v1/dns/rewrites', newRewrite.value)
    if (response.data && !response.data.error) {
      toast.success('DNS Rewrite eklendi')
      await fetchRewrites()
      isAddRewriteModalActive.value = false
      resetRewriteForm()
    } else {
      toast.error('Rewrite eklenemedi: ' + (response.data.msg || 'Bilinmeyen hata'))
    }
  } catch (error) {
    toast.error('Rewrite eklenemedi: ' + error.message)
  }
  loading.value = false
}

const updateRewrite = async () => {
  loading.value = true
  try {
    const response = await ApiService.put(`/v1/dns/rewrites/${editingRewrite.value.id}`, editingRewrite.value)
    if (response.data && !response.data.error) {
      toast.success('DNS Rewrite updated')
      await fetchRewrites()
      isEditRewriteModalActive.value = false
      editingRewrite.value = null
    } else {
      toast.error('Failed to update rewrite: ' + (response.data.msg || 'Unknown error'))
    }
  } catch (error) {
    toast.error('Failed to update rewrite: ' + error.message)
  }
  loading.value = false
}

const deleteRewrite = async () => {
  loading.value = true
  try {
    const response = await ApiService.delete(`/v1/dns/rewrites/${deleteTarget.value.item.id}`)
    if (response.data && !response.data.error) {
      toast.success('DNS Rewrite silindi')
      await fetchRewrites()
      isDeleteModalActive.value = false
    } else {
      toast.error('Rewrite silinemedi: ' + (response.data.msg || 'Bilinmeyen hata'))
    }
  } catch (error) {
    toast.error('Rewrite silinemedi: ' + error.message)
  }
  loading.value = false
}

const reloadFilters = async () => {
  loading.value = true
  try {
    const response = await ApiService.post('/v1/dns/reload')
    if (response.data && !response.data.error) {
      toast.success('Filters reloaded successfully')
      await fetchBlocklists()
      await fetchCustomFilters()
    } else {
      toast.error('Failed to reload filters: ' + (response.data.msg || 'Unknown error'))
    }
  } catch (error) {
    toast.error('Failed to reload filters: ' + error.message)
  }
  loading.value = false
}

// Helper methods
const resetBlocklistForm = () => {
  newBlocklist.value = {
    name: '',
    url: '',
    enabled: true,
    update_interval: 86400
  }
}

const resetFilterForm = () => {
  newFilter.value = {
    domain: '',
    type: 'blacklist',
    comment: '',
    is_regex: false,
    is_wildcard: false
  }
}

const resetRewriteForm = () => {
  newRewrite.value = {
    domain: '',
    answer: '',
    type: 'A',
    comment: '',
    enabled: true
  }
}

const openEditBlocklist = (blocklist) => {
  editingBlocklist.value = { ...blocklist }
  isEditBlocklistModalActive.value = true
}

const openEditRewrite = (rewrite) => {
  editingRewrite.value = { ...rewrite }
  isEditRewriteModalActive.value = true
}

const openDeleteModal = (type, item) => {
  deleteTarget.value = { type, item }
  isDeleteModalActive.value = true
}

const confirmDelete = async () => {
  if (deleteTarget.value.type === 'blocklist') {
    await deleteBlocklist()
  } else if (deleteTarget.value.type === 'filter') {
    await deleteFilter()
  } else if (deleteTarget.value.type === 'rewrite') {
    await deleteRewrite()
  }
}

const formatDate = (dateString) => {
  if (!dateString) return 'Never'
  return new Date(dateString).toLocaleString()
}

const formatNumber = (num) => {
  return new Intl.NumberFormat().format(num)
}

// Log actions
const blockDomainGlobally = async (domain) => {
  try {
    await ApiService.post('/v1/dns/filters', {
      domain: domain,
      type: 'blacklist',
      comment: 'Blocked from logs'
    })
    toast.success(`Domain ${domain} blocked globally`)
    activeLogMenu.value = null
    currentLogStatus.value = null
  } catch (error) {
    toast.error('Failed to block domain')
  }
}

const removeGlobalBlock = async (domain) => {
  try {
    await ApiService.delete(`/v1/dns/filters?domain=${encodeURIComponent(domain)}&type=blacklist`)
    toast.success(`Global block removed for ${domain}`)
    activeLogMenu.value = null
    currentLogStatus.value = null
  } catch (error) {
    toast.error('Failed to remove global block')
  }
}

const blockDomainForClient = async (clientIP, domain) => {
  try {
    await ApiService.post('/v1/dns/client-rules', {
      client_ip: clientIP,
      domain: domain,
      type: 'block',
      comment: `Blocked for ${clientIP} from logs`
    })
    toast.success(`Domain ${domain} blocked for ${clientIP}`)
    activeLogMenu.value = null
    // Refresh status
    currentLogStatus.value = null
  } catch (error) {
    toast.error('Failed to block domain for client')
  }
}

const removeClientDomainRule = async (clientIP, domain, ruleType) => {
  try {
    await ApiService.delete(`/v1/dns/client-rules?client_ip=${encodeURIComponent(clientIP)}&domain=${encodeURIComponent(domain)}&type=${ruleType}`)
    toast.success(`Client rule removed for ${clientIP}`)
    activeLogMenu.value = null
    // Refresh status
    currentLogStatus.value = null
  } catch (error) {
    toast.error('Failed to remove client rule')
  }
}

const blockClient = async (clientIP) => {
  try {
    await ApiService.post('/v1/dns/clients/block', {
      client_ip: clientIP,
      reason: 'Blocked from logs'
    })
    toast.success(`Client ${clientIP} has been banned`)
    activeLogMenu.value = null
    await fetchClients()
  } catch (error) {
    toast.error('Failed to block client')
  }
}

const unblockClient = async (clientIP) => {
  try {
    await ApiService.post(`/v1/dns/clients/${clientIP}/unblock`)
    toast.success(`Client ${clientIP} has been unblocked`)
    activeClientMenu.value = null
    await fetchClients()
  } catch (error) {
    toast.error('Failed to unblock client')
  }
}

const toggleLogMenu = async (logId) => {
  if (activeLogMenu.value === logId) {
    activeLogMenu.value = null
    currentLogStatus.value = null
  } else {
    activeLogMenu.value = logId
    
    // Find the log and fetch real-time status
    const log = queryLogs.value.find(l => l.id === logId)
    if (log) {
      try {
        const response = await ApiService.get(`/v1/dns/check-domain-status?domain=${encodeURIComponent(log.domain)}&client_ip=${encodeURIComponent(log.client_ip)}`)
        if (response.data && !response.data.error) {
          currentLogStatus.value = {
            logId: logId,
            ...response.data.data
          }
        }
      } catch (error) {
        console.error('Failed to fetch domain status:', error)
      }
    }
  }
}

const toggleClientMenu = (clientIP) => {
  activeClientMenu.value = activeClientMenu.value === clientIP ? null : clientIP
}

// Lifecycle hooks
onMounted(async () => {
  // Fetch all data in parallel (faster load)
  await Promise.all([
    fetchStatus(),
    fetchConfig(),
    fetchStats(),
    fetchBlocklists(),
    fetchCustomFilters(),
    fetchRewrites(),
    fetchClients()
  ])
  
  // Auto-refresh every 5 seconds
  refreshInterval = setInterval(() => {
    fetchStatus()
    fetchStats()
    if (activeTab.value === 'overview') {
      fetchClients()
    } else if (activeTab.value === 'logs') {
      fetchQueryLogs()
    } else if (activeTab.value === 'rewrites') {
      fetchRewrites()
    }
  }, 5000)
})

onUnmounted(() => {
  if (refreshInterval) {
    clearInterval(refreshInterval)
  }
})

// Watch tab changes
const switchTab = async (tab) => {
  activeTab.value = tab
  
  if (tab === 'overview') {
    await fetchClients()
  } else if (tab === 'blocklists') {
    await fetchBlocklists()
  } else if (tab === 'filters') {
    await fetchCustomFilters()
  } else if (tab === 'rewrites') {
    await fetchRewrites()
  } else if (tab === 'custom-rules') {
    await fetchCustomRules()
  } else if (tab === 'logs') {
    await fetchQueryLogs()
  }
}

// Delete custom rule functions
const deleteGlobalFilter = async (filterId) => {
  try {
    const response = await ApiService.delete(`/v1/dns/filters/${filterId}`)
    if (response.data && !response.data.error) {
      toast.success('Global filter deleted')
      await fetchCustomRules()
    }
  } catch (error) {
    toast.error('Failed to delete filter: ' + error.message)
  }
}

const deleteClientRule = async (ruleId) => {
  try {
    const response = await ApiService.delete(`/v1/dns/client-rules/${ruleId}`)
    if (response.data && !response.data.error) {
      toast.success('Client rule deleted')
      await fetchCustomRules()
    }
  } catch (error) {
    toast.error('Failed to delete rule: ' + error.message)
  }
}

const deleteClientBan = async (clientIP) => {
  try {
    const response = await ApiService.post(`/v1/dns/clients/${clientIP}/unblock`)
    if (response.data && !response.data.error) {
      toast.success('Client unblocked')
      await fetchCustomRules()
    }
  } catch (error) {
    toast.error('Failed to unblock client: ' + error.message)
  }
}

</script>

<template>
  <div class="space-y-8">
    <!-- Header -->
    <div class="bg-gradient-to-r from-emerald-600 via-teal-600 to-cyan-600 rounded-2xl p-8 text-white shadow-lg">
      <div class="flex flex-col lg:flex-row lg:items-center lg:justify-between">
        <div>
          <h1 class="text-3xl lg:text-4xl font-bold mb-2 flex items-center">
            <BaseIcon :path="mdiDns" size="40" class="mr-4" />
            DNS Server
          </h1>
          <p class="text-cyan-100 text-lg">Ad-blocking DNS server with filtering, caching, and analytics</p>
        </div>
        <div class="mt-6 lg:mt-0 flex flex-wrap gap-3">
          <BaseButton
            v-if="!status.running"
            label="Start Server"
            :icon="mdiPlay"
            color="white"
            class="shadow-lg hover:shadow-xl transform hover:scale-105 transition-all duration-200"
            @click="startServer"
          />
          <BaseButton
            v-else
            label="Stop Server"
            :icon="mdiStop"
            color="danger"
            class="shadow-lg hover:shadow-xl transform hover:scale-105 transition-all duration-200"
            @click="stopServer"
          />
          <BaseButton
            label="Settings"
            :icon="mdiCog"
            color="white"
            outline
            class="shadow-lg hover:shadow-xl"
            @click="isConfigModalActive = true"
          />
          <BaseButton
            :icon="mdiRefresh"
            color="white"
            outline
            :disabled="loading"
            class="shadow-lg"
            @click="fetchStats"
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
        <span v-if="status.running" class="text-cyan-100 text-sm">
          UDP: {{ config.udp_port }} | TCP: {{ config.tcp_port }} | Cache: {{ config.cache_enabled ? 'On' : 'Off' }}
        </span>
      </div>
    </div>

    <!-- Statistics -->
    <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
      <CardBox class="bg-gradient-to-br from-blue-50 to-blue-100 dark:from-blue-900/20 dark:to-blue-800/20 border-blue-200 dark:border-blue-700">
        <div class="flex items-center justify-between">
          <div>
            <div class="text-2xl font-bold text-blue-600 dark:text-blue-400">{{ formatNumber(stats.total_queries_24h) }}</div>
            <div class="text-sm text-blue-600/70">Queries (24h)</div>
            <div class="text-xs text-blue-600/50 mt-1">{{ stats.queries_per_minute.toFixed(2) }} qpm</div>
          </div>
          <BaseIcon :path="mdiSpeedometer" size="36" class="text-blue-500 opacity-20" />
        </div>
      </CardBox>

      <CardBox class="bg-gradient-to-br from-red-50 to-red-100 dark:from-red-900/20 dark:to-red-800/20 border-red-200 dark:border-red-700">
        <div class="flex items-center justify-between">
          <div>
            <div class="text-2xl font-bold text-red-600 dark:text-red-400">{{ formatNumber(stats.blocked_queries_24h) }}</div>
            <div class="text-sm text-red-600/70">Blocked</div>
            <div class="text-xs text-red-600/50 mt-1">{{ blockPercentage }}% blocked</div>
          </div>
          <BaseIcon :path="mdiShieldCheck" size="36" class="text-red-500 opacity-20" />
        </div>
      </CardBox>

      <CardBox class="bg-gradient-to-br from-green-50 to-green-100 dark:from-green-900/20 dark:to-green-800/20 border-green-200 dark:border-green-700">
        <div class="flex items-center justify-between">
          <div>
            <div class="text-2xl font-bold text-green-600 dark:text-green-400">{{ stats.cache_hit_rate.toFixed(1) }}%</div>
            <div class="text-sm text-green-600/70">Cache Hit Rate</div>
            <div class="text-xs text-green-600/50 mt-1">{{ stats.avg_response_time.toFixed(1) }}ms avg</div>
          </div>
          <BaseIcon :path="mdiDatabase" size="36" class="text-green-500 opacity-20" />
        </div>
      </CardBox>

      <CardBox class="bg-gradient-to-br from-purple-50 to-purple-100 dark:from-purple-900/20 dark:to-purple-800/20 border-purple-200 dark:border-purple-700">
        <div class="flex items-center justify-between">
          <div>
            <div class="text-2xl font-bold text-purple-600 dark:text-purple-400">{{ stats.active_clients }}</div>
            <div class="text-sm text-purple-600/70">Active Clients</div>
            <div class="text-xs text-purple-600/50 mt-1">Last hour</div>
          </div>
          <BaseIcon :path="mdiWeb" size="36" class="text-purple-500 opacity-20" />
        </div>
      </CardBox>
    </div>

    <!-- Tabs -->
    <div class="flex border-b border-gray-200 dark:border-gray-700">
      <button
        v-for="tab in ['overview', 'blocklists', 'filters', 'rewrites', 'custom-rules', 'logs']"
        :key="tab"
        :class="[
          'px-6 py-3 font-medium text-sm border-b-2 transition-colors capitalize',
          activeTab === tab
            ? 'border-emerald-500 text-emerald-600 dark:text-emerald-400'
            : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'
        ]"
        @click="switchTab(tab)"
      >
        {{ tab.replace('-', ' ') }}
      </button>
    </div>

    <!-- Overview Tab -->
    <div v-if="activeTab === 'overview'" class="space-y-6">
      <!-- Performance & Configuration - First Row -->
      <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <!-- Performance Stats -->
        <CardBox>
          <SectionTitleLineWithButton :icon="mdiSpeedometer" title="Performance" main />
          <div class="grid grid-cols-2 gap-4 mt-4">
            <div class="p-4 bg-slate-50 dark:bg-slate-800/50 rounded-lg">
              <div class="text-2xl font-bold text-slate-800 dark:text-slate-200">{{ stats.avg_response_time.toFixed(2) }}ms</div>
              <div class="text-sm text-slate-500">Avg Response Time</div>
            </div>
            <div class="p-4 bg-slate-50 dark:bg-slate-800/50 rounded-lg">
              <div class="text-2xl font-bold text-slate-800 dark:text-slate-200">{{ stats.queries_per_minute.toFixed(2) }}</div>
              <div class="text-sm text-slate-500">Queries/Minute</div>
            </div>
            <div class="p-4 bg-slate-50 dark:bg-slate-800/50 rounded-lg">
              <div class="text-2xl font-bold text-slate-800 dark:text-slate-200">{{ formatNumber(stats.total_queries_24h) }}</div>
              <div class="text-sm text-slate-500">Total Queries (24h)</div>
            </div>
            <div class="p-4 bg-slate-50 dark:bg-slate-800/50 rounded-lg">
              <div class="text-2xl font-bold text-red-600 dark:text-red-400">{{ formatNumber(stats.blocked_queries_24h) }}</div>
              <div class="text-sm text-slate-500">Blocked (24h)</div>
            </div>
          </div>
        </CardBox>

        <!-- Configuration -->
        <CardBox>
          <SectionTitleLineWithButton :icon="mdiCog" title="Configuration" main>
            <BaseButton
              label="Edit"
              :icon="mdiPencil"
              color="info"
              small
              @click="isConfigModalActive = true"
            />
          </SectionTitleLineWithButton>
          <div class="space-y-3 mt-4">
            <div class="flex items-center justify-between p-3 bg-slate-50 dark:bg-slate-800/50 rounded-lg">
              <div class="flex items-center gap-3">
                <span class="font-medium">UDP Port</span>
              </div>
              <span class="text-slate-600 dark:text-slate-300">{{ config.udp_port }}</span>
            </div>
            <div class="flex items-center justify-between p-3 bg-slate-50 dark:bg-slate-800/50 rounded-lg">
              <div class="flex items-center gap-3">
                <span class="font-medium">TCP Port</span>
              </div>
              <span class="text-slate-600 dark:text-slate-300">{{ config.tcp_port }}</span>
            </div>
            <div class="flex items-center justify-between p-3 bg-slate-50 dark:bg-slate-800/50 rounded-lg">
              <div class="flex items-center gap-3">
                <BaseIcon 
                  :path="mdiShield" 
                  :class="config.blocking_enabled ? 'text-green-500' : 'text-gray-500'"
                  size="20" 
                />
                <span class="font-medium">Blocking</span>
              </div>
              <span :class="config.blocking_enabled ? 'text-green-500' : 'text-gray-500'">
                {{ config.blocking_enabled ? 'Enabled' : 'Disabled' }}
              </span>
            </div>
            <div class="flex items-center justify-between p-3 bg-slate-50 dark:bg-slate-800/50 rounded-lg">
              <div class="flex items-center gap-3">
                <BaseIcon 
                  :path="mdiDatabase" 
                  :class="config.cache_enabled ? 'text-green-500' : 'text-gray-500'"
                  size="20" 
                />
                <span class="font-medium">Cache</span>
              </div>
              <span :class="config.cache_enabled ? 'text-green-500' : 'text-gray-500'">
                {{ config.cache_enabled ? 'Enabled' : 'Disabled' }}
              </span>
            </div>
            <div class="flex items-center justify-between p-3 bg-slate-50 dark:bg-slate-800/50 rounded-lg">
              <div class="flex items-center gap-3">
                <span class="font-medium">DoH / DoT</span>
              </div>
              <span :class="(config.doh_enabled || config.dot_enabled) ? 'text-green-500' : 'text-gray-500'">
                {{ config.doh_enabled ? 'DoH' : '' }}{{ config.doh_enabled && config.dot_enabled ? ' & ' : '' }}{{ config.dot_enabled ? 'DoT' : '' }}{{ !config.doh_enabled && !config.dot_enabled ? 'Disabled' : '' }}
              </span>
            </div>
          </div>
        </CardBox>
      </div>

      <!-- Active Clients - Full Width -->
      <CardBox>
        <SectionTitleLineWithButton :icon="mdiWeb" title="Active Clients (Last 24h)" main>
          <BaseButton
            label="Refresh"
            :icon="mdiRefresh"
            color="info"
            small
            @click="fetchClients"
          />
        </SectionTitleLineWithButton>

        <div v-if="clients.length === 0" class="text-center py-12">
          <BaseIcon :path="mdiWeb" size="64" class="mx-auto text-slate-300 dark:text-slate-600 mb-4" />
          <p class="text-slate-500">No active clients</p>
        </div>

        <div v-else class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 mt-4">
          <div
            v-for="client in clients.slice(0, 12)"
            :key="client.ip"
            class="p-4 bg-slate-50 dark:bg-slate-800/50 rounded-xl relative"
            :class="{ 'border-2 border-red-500': client.is_banned }"
          >
            <div class="flex items-start justify-between mb-3">
              <div>
                <div class="flex items-center gap-2">
                  <h3 class="font-semibold font-mono">{{ client.ip }}</h3>
                  <span 
                    v-if="client.is_banned" 
                    class="px-2 py-0.5 text-xs rounded bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300 font-semibold"
                  >
                    BANNED
                  </span>
                </div>
                <p class="text-xs text-slate-500 mt-1">{{ formatDate(client.last_seen) }}</p>
              </div>
              <div class="flex items-center gap-2">
                <span 
                  class="px-2 py-0.5 text-xs rounded bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300"
                >
                  {{ client.query_count }} queries
                </span>
                <button 
                  @click="toggleClientMenu(client.ip)"
                  class="px-2 py-1 hover:bg-slate-100 dark:hover:bg-slate-700 rounded transition-colors"
                >
                  ...
                </button>
              </div>
            </div>

            <div v-if="activeClientMenu === client.ip" class="absolute right-4 top-16 w-48 bg-white dark:bg-slate-800 rounded-lg shadow-xl border border-slate-200 dark:border-slate-700 z-50 py-1" @click.stop>
              <button 
                v-if="client.is_banned"
                @click="unblockClient(client.ip)" 
                class="w-full px-4 py-2 text-left text-sm hover:bg-green-50 dark:hover:bg-green-900/20 text-green-600"
              >
                ✅ Unban this client
              </button>
              <button 
                v-else
                @click="blockClient(client.ip)" 
                class="w-full px-4 py-2 text-left text-sm hover:bg-red-50 dark:hover:bg-red-900/20 text-red-600"
              >
                🚫 Block this client (IP Ban)
              </button>
            </div>

            <div class="grid grid-cols-2 gap-2">
              <div class="p-2 bg-white dark:bg-slate-900/50 rounded-lg">
                <div class="text-xs text-slate-500">Total</div>
                <div class="text-lg font-bold">{{ client.query_count }}</div>
              </div>
              <div class="p-2 bg-white dark:bg-slate-900/50 rounded-lg">
                <div class="text-xs text-slate-500">Blocked</div>
                <div class="text-lg font-bold text-red-600 dark:text-red-400">{{ client.blocked_count || 0 }}</div>
              </div>
            </div>
          </div>
        </div>
      </CardBox>

      <!-- Top Domains - Two Columns -->
      <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <!-- Top Domains -->
        <CardBox>
          <SectionTitleLineWithButton :icon="mdiWeb" title="Top Queried Domains" main />
          <div v-if="!stats.top_domains || stats.top_domains.length === 0" class="text-center py-10 text-slate-500">
            No data available
          </div>
          <div v-else class="space-y-3 mt-4">
            <div
              v-for="(domain, index) in stats.top_domains?.slice(0, 10)"
              :key="domain.domain"
              class="flex items-center justify-between p-3 bg-slate-50 dark:bg-slate-800/50 rounded-lg"
            >
              <div class="flex items-center gap-3">
                <span class="text-xs font-bold text-slate-400 w-6">#{{ index + 1 }}</span>
                <span class="font-mono text-sm truncate max-w-xs">{{ domain.domain }}</span>
              </div>
              <span class="font-semibold">{{ domain.count }}</span>
            </div>
          </div>
        </CardBox>

        <!-- Top Blocked Domains -->
        <CardBox>
          <SectionTitleLineWithButton :icon="mdiShield" title="Top Blocked Domains" main />
          <div v-if="!stats.top_blocked || stats.top_blocked.length === 0" class="text-center py-10 text-slate-500">
            No blocked domains yet
          </div>
          <div v-else class="space-y-3 mt-4">
            <div
              v-for="(domain, index) in stats.top_blocked?.slice(0, 10)"
              :key="domain.domain"
              class="flex items-center justify-between p-3 bg-slate-50 dark:bg-slate-800/50 rounded-lg"
            >
              <div class="flex items-center gap-3">
                <span class="text-xs font-bold text-red-400 w-6">#{{ index + 1 }}</span>
                <span class="font-mono text-sm truncate max-w-xs">{{ domain.domain }}</span>
              </div>
              <span class="font-semibold text-red-600 dark:text-red-400">{{ domain.count }}</span>
            </div>
          </div>
        </CardBox>
      </div>
    </div>

    <!-- Blocklists Tab -->
    <CardBox v-if="activeTab === 'blocklists'">
      <SectionTitleLineWithButton :icon="mdiShield" title="Blocklists" main>
        <div class="flex gap-2">
          <BaseButton
            label="Add Blocklist"
            :icon="mdiPlus"
            color="info"
            small
            @click="isAddBlocklistModalActive = true"
          />
          <BaseButton
            label="Reload"
            :icon="mdiRefresh"
            color="success"
            small
            @click="reloadFilters"
          />
        </div>
      </SectionTitleLineWithButton>

      <!-- Info Banner -->
      <div class="mt-4 p-4 bg-blue-50 dark:bg-blue-900/20 rounded-lg border border-blue-200 dark:border-blue-800">
        <div class="flex items-start gap-3">
          <BaseIcon :path="mdiShield" class="text-blue-600 dark:text-blue-400 mt-0.5" size="20" />
          <div class="flex-1">
            <h4 class="font-semibold text-blue-900 dark:text-blue-100 mb-1">What are Blocklists?</h4>
            <p class="text-sm text-blue-800 dark:text-blue-200 mb-2">
              Blocklists are large domain lists automatically downloaded from external sources and regularly updated. 
              They contain thousands/millions of ads, trackers, and malicious domains.
            </p>
            <div class="grid grid-cols-1 md:grid-cols-2 gap-2 text-xs">
              <div class="flex items-center gap-2">
                <span class="text-blue-600 dark:text-blue-400">✓</span>
                <span class="text-blue-700 dark:text-blue-300">Auto-updates (daily/weekly)</span>
              </div>
              <div class="flex items-center gap-2">
                <span class="text-blue-600 dark:text-blue-400">✓</span>
                <span class="text-blue-700 dark:text-blue-300">Supports multiple formats</span>
              </div>
              <div class="flex items-center gap-2">
                <span class="text-blue-600 dark:text-blue-400">✓</span>
                <span class="text-blue-700 dark:text-blue-300">Contains millions of domains</span>
              </div>
              <div class="flex items-center gap-2">
                <span class="text-blue-600 dark:text-blue-400">✓</span>
                <span class="text-blue-700 dark:text-blue-300">Examples: AdGuard, OISD, StevenBlack</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div v-if="blocklists.length === 0" class="text-center py-12">
        <BaseIcon :path="mdiShield" size="64" class="mx-auto text-slate-300 dark:text-slate-600 mb-4" />
        <p class="text-slate-500 mb-4">No blocklists configured</p>
        <BaseButton label="Add Your First Blocklist" :icon="mdiPlus" color="info" @click="isAddBlocklistModalActive = true" />
      </div>

      <div v-else class="space-y-4 mt-4">
        <div
          v-for="blocklist in blocklists"
          :key="blocklist.id"
          class="p-4 bg-slate-50 dark:bg-slate-800/50 rounded-xl"
        >
          <div class="flex items-start justify-between">
            <div>
              <h3 class="font-semibold flex items-center gap-2">
                {{ blocklist.name }}
                <span 
                  :class="[
                    'px-2 py-0.5 text-xs rounded',
                    blocklist.enabled ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300' : 'bg-gray-100 text-gray-700 dark:bg-gray-900/30 dark:text-gray-300'
                  ]"
                >
                  {{ blocklist.enabled ? 'Enabled' : 'Disabled' }}
                </span>
              </h3>
              <p class="text-sm text-slate-500 mt-1 truncate max-w-xl">{{ blocklist.url }}</p>
            </div>
            <div class="flex gap-2">
              <BaseButton :icon="mdiPencil" color="info" small @click="openEditBlocklist(blocklist)" />
              <BaseButton :icon="mdiDelete" color="danger" small @click="openDeleteModal('blocklist', blocklist)" />
            </div>
          </div>
          <div class="mt-3 flex flex-wrap gap-2">
            <span class="px-2 py-1 bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300 text-xs rounded">
              {{ blocklist.format }}
            </span>
            <span class="px-2 py-1 bg-purple-100 dark:bg-purple-900/30 text-purple-700 dark:text-purple-300 text-xs rounded">
              {{ formatNumber(blocklist.domain_count) }} domains
            </span>
            <span v-if="blocklist.last_updated" class="px-2 py-1 bg-gray-100 dark:bg-gray-900/30 text-gray-700 dark:text-gray-300 text-xs rounded">
              Updated: {{ formatDate(blocklist.last_updated) }}
            </span>
          </div>
        </div>
      </div>
    </CardBox>

    <!-- Filters Tab -->
    <CardBox v-if="activeTab === 'filters'">
      <SectionTitleLineWithButton :icon="mdiWeb" title="Custom Filters" main>
        <BaseButton
          label="Add Filter"
          :icon="mdiPlus"
          color="info"
          small
          @click="isAddFilterModalActive = true"
        />
      </SectionTitleLineWithButton>

      <!-- Info Banner -->
      <div class="mt-4 p-4 bg-emerald-50 dark:bg-emerald-900/20 rounded-lg border border-emerald-200 dark:border-emerald-800">
        <div class="flex items-start gap-3">
          <BaseIcon :path="mdiWeb" class="text-emerald-600 dark:text-emerald-400 mt-0.5" size="20" />
          <div class="flex-1">
            <h4 class="font-semibold text-emerald-900 dark:text-emerald-100 mb-1">What are Custom Filters?</h4>
            <p class="text-sm text-emerald-800 dark:text-emerald-200 mb-2">
              Custom Filters are manually added domain rules. 
              Use them to block or allow specific domains.
            </p>
            <div class="grid grid-cols-1 md:grid-cols-2 gap-2 text-xs">
              <div class="flex items-center gap-2">
                <span class="text-emerald-600 dark:text-emerald-400">✓</span>
                <span class="text-emerald-700 dark:text-emerald-300">Manual control (you add them)</span>
              </div>
              <div class="flex items-center gap-2">
                <span class="text-emerald-600 dark:text-emerald-400">✓</span>
                <span class="text-emerald-700 dark:text-emerald-300">Regex and wildcard support</span>
              </div>
              <div class="flex items-center gap-2">
                <span class="text-emerald-600 dark:text-emerald-400">✓</span>
                <span class="text-emerald-700 dark:text-emerald-300">Blacklist: Block domain</span>
              </div>
              <div class="flex items-center gap-2">
                <span class="text-emerald-600 dark:text-emerald-400">✓</span>
                <span class="text-emerald-700 dark:text-emerald-300">Whitelist: Allow domain</span>
              </div>
            </div>
            <div class="mt-3 p-2 bg-emerald-100 dark:bg-emerald-900/30 rounded text-xs text-emerald-800 dark:text-emerald-200">
              <strong>💡 Usage:</strong> Block domains that escape blocklists, or whitelist domains that are blocked but you need access to.
            </div>
          </div>
        </div>
      </div>

      <div v-if="customFilters.length === 0" class="text-center py-12">
        <BaseIcon :path="mdiWeb" size="64" class="mx-auto text-slate-300 dark:text-slate-600 mb-4" />
        <p class="text-slate-500 mb-4">No custom filters configured</p>
        <BaseButton label="Add Your First Filter" :icon="mdiPlus" color="info" @click="isAddFilterModalActive = true" />
      </div>

      <div v-else class="space-y-4 mt-4">
        <div
          v-for="filter in customFilters"
          :key="filter.id"
          class="p-4 bg-slate-50 dark:bg-slate-800/50 rounded-xl"
        >
          <div class="flex items-start justify-between">
            <div>
              <h3 class="font-semibold font-mono flex items-center gap-2">
                {{ filter.domain }}
                <span 
                  :class="[
                    'px-2 py-0.5 text-xs rounded',
                    filter.type === 'blacklist' ? 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300' : 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300'
                  ]"
                >
                  {{ filter.type }}
                </span>
              </h3>
              <p v-if="filter.comment" class="text-sm text-slate-500 mt-1">{{ filter.comment }}</p>
            </div>
            <BaseButton :icon="mdiDelete" color="danger" small @click="openDeleteModal('filter', filter)" />
          </div>
          <div class="mt-3 flex flex-wrap gap-2">
            <span v-if="filter.is_regex" class="px-2 py-1 bg-purple-100 dark:bg-purple-900/30 text-purple-700 dark:text-purple-300 text-xs rounded">
              Regex
            </span>
            <span v-if="filter.is_wildcard" class="px-2 py-1 bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300 text-xs rounded">
              Wildcard
            </span>
          </div>
        </div>
      </div>
    </CardBox>

    <!-- DNS Rewrites Tab -->
    <CardBox v-if="activeTab === 'rewrites'">
      <SectionTitleLineWithButton :icon="mdiWeb" title="DNS Rewrites" main>
        <BaseButton
          :icon="mdiPlus"
          color="info"
          label="Add Rewrite"
          @click="isAddRewriteModalActive = true"
        />
      </SectionTitleLineWithButton>

      <!-- Info Banner -->
      <div class="mt-4 p-4 bg-purple-50 dark:bg-purple-900/20 rounded-lg border border-purple-200 dark:border-purple-800">
        <div class="flex items-start gap-3">
          <BaseIcon :path="mdiWeb" class="text-purple-600 dark:text-purple-400 mt-0.5" size="20" />
          <div class="flex-1">
            <h4 class="font-semibold text-purple-900 dark:text-purple-100 mb-1">What are DNS Rewrites?</h4>
            <p class="text-sm text-purple-800 dark:text-purple-200 mb-2">
              DNS Rewrites allow you to define custom DNS responses for specific domains. 
              Redirect domains to different IP addresses or other domains.
            </p>
            <div class="grid grid-cols-1 md:grid-cols-2 gap-2 text-xs">
              <div class="flex items-center gap-2">
                <span class="text-purple-600 dark:text-purple-400">✓</span>
                <span class="text-purple-700 dark:text-purple-300">Local development (*.test.local → 127.0.0.1)</span>
              </div>
              <div class="flex items-center gap-2">
                <span class="text-purple-600 dark:text-purple-400">✓</span>
                <span class="text-purple-700 dark:text-purple-300">Redirect to internal services</span>
              </div>
              <div class="flex items-center gap-2">
                <span class="text-purple-600 dark:text-purple-400">✓</span>
                <span class="text-purple-700 dark:text-purple-300">Create CNAME aliases</span>
              </div>
              <div class="flex items-center gap-2">
                <span class="text-purple-600 dark:text-purple-400">✓</span>
                <span class="text-purple-700 dark:text-purple-300">Wildcard support (*.domain.com)</span>
              </div>
            </div>
            <div class="mt-3 p-2 bg-purple-100 dark:bg-purple-900/30 rounded text-xs text-purple-800 dark:text-purple-200">
              <strong>🔄 Difference:</strong> Rewrite redirects, not blocks. Blocklist/Filter blocks domains, Rewrite redirects them to your specified IP/domain.
            </div>
          </div>
        </div>
      </div>

      <div class="mt-6 space-y-4">
        <div v-if="rewrites.length === 0" class="text-center py-12 text-slate-500">
          No DNS rewrite rules added yet
        </div>

        <div v-for="rewrite in rewrites" :key="rewrite.id" class="group relative bg-slate-50 dark:bg-slate-800/50 rounded-lg p-6 hover:shadow-md transition-all border border-slate-200 dark:border-slate-700">
          <div class="flex items-start justify-between">
            <div class="flex-1 space-y-3">
              <!-- Domain -->
              <div class="flex items-center gap-3">
                <span class="text-xs font-medium px-2 py-1 rounded bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300">
                  DOMAIN
                </span>
                <code class="font-mono text-sm text-slate-800 dark:text-slate-200">{{ rewrite.domain }}</code>
                <span v-if="rewrite.domain.startsWith('*.')" class="text-xs px-2 py-1 rounded bg-purple-100 dark:bg-purple-900/30 text-purple-700 dark:text-purple-300">
                  Wildcard
                </span>
              </div>

              <!-- Arrow -->
              <div class="flex items-center gap-3 ml-2">
                <svg class="w-5 h-5 text-emerald-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 8l4 4m0 0l-4 4m4-4H3"/>
                </svg>
              </div>

              <!-- Answer -->
              <div class="flex items-center gap-3">
                <span :class="[
                  'text-xs font-medium px-2 py-1 rounded',
                  rewrite.type === 'A' ? 'bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-300' :
                  rewrite.type === 'AAAA' ? 'bg-cyan-100 dark:bg-cyan-900/30 text-cyan-700 dark:text-cyan-300' :
                  'bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-300'
                ]">
                  {{ rewrite.type }}
                </span>
                <code class="font-mono text-sm text-slate-800 dark:text-slate-200">{{ rewrite.answer }}</code>
                <span v-if="rewrite.answer === 'A' || rewrite.answer === 'AAAA'" class="text-xs px-2 py-1 rounded bg-orange-100 dark:bg-orange-900/30 text-orange-700 dark:text-orange-300">
                  Keep Upstream
                </span>
              </div>

              <!-- Comment -->
              <div v-if="rewrite.comment" class="ml-2 text-sm text-slate-600 dark:text-slate-400">
                💬 {{ rewrite.comment }}
              </div>

              <!-- Status -->
              <div class="flex items-center gap-2 ml-2">
                <span :class="[
                  'inline-flex items-center px-2 py-1 rounded-full text-xs font-medium',
                  rewrite.enabled 
                    ? 'bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-300' 
                    : 'bg-gray-100 dark:bg-gray-900/30 text-gray-700 dark:text-gray-300'
                ]">
                  {{ rewrite.enabled ? '✓ Enabled' : '✗ Disabled' }}
                </span>
                <span class="text-xs text-slate-500">
                  {{ formatDate(rewrite.created_at) }}
                </span>
              </div>
            </div>

            <!-- Actions -->
            <div class="flex gap-2 opacity-0 group-hover:opacity-100 transition-opacity">
              <BaseButton
                :icon="mdiPencil"
                color="info"
                small
                @click="openEditRewrite(rewrite)"
              />
              <BaseButton
                :icon="mdiDelete"
                color="danger"
                small
                @click="openDeleteModal('rewrite', rewrite)"
              />
            </div>
          </div>
        </div>
      </div>
    </CardBox>

    <!-- Logs Tab -->
    <CardBox v-if="activeTab === 'logs'">
      <SectionTitleLineWithButton :icon="mdiChartLine" title="Query Logs" main>
        <BaseButton
          label="Refresh"
          :icon="mdiRefresh"
          color="info"
          small
          @click="fetchQueryLogs"
        />
      </SectionTitleLineWithButton>

      <!-- Search Input -->
      <div class="mt-4 mb-6">
        <div class="relative">
          <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
            <BaseIcon :path="mdiMagnify" class="text-slate-400" size="20" />
          </div>
          <input
            v-model="searchQuery"
            type="text"
            placeholder="Search by client IP, domain or response..."
            class="w-full pl-10 pr-4 py-2.5 border border-slate-300 dark:border-slate-600 rounded-lg bg-white dark:bg-slate-800 text-slate-900 dark:text-slate-100 placeholder-slate-400 focus:ring-2 focus:ring-emerald-500 focus:border-transparent transition-all"
          />
          <div v-if="searchQuery" class="absolute inset-y-0 right-0 pr-3 flex items-center">
            <button
              @click="searchQuery = ''"
              class="text-slate-400 hover:text-slate-600 dark:hover:text-slate-300"
            >
              <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
              </svg>
            </button>
          </div>
        </div>
        <div v-if="searchQuery" class="mt-2 text-sm text-slate-600 dark:text-slate-400">
          {{ filteredQueryLogs.length }} result(s) ({{ queryLogs.length }} total)
        </div>
      </div>

      <div v-if="queryLogs.length === 0" class="text-center py-12">
        <BaseIcon :path="mdiChartLine" size="64" class="mx-auto text-slate-300 dark:text-slate-600 mb-4" />
        <p class="text-slate-500">No query logs available</p>
      </div>

      <div v-else-if="searchQuery && filteredQueryLogs.length === 0" class="text-center py-12">
        <BaseIcon :path="mdiMagnify" size="64" class="mx-auto text-slate-300 dark:text-slate-600 mb-4" />
        <p class="text-slate-500">No results for search</p>
        <p class="text-sm text-slate-400 mt-2">No logs matching "{{ searchQuery }}"</p>
      </div>

      <div v-else class="mt-4 overflow-x-auto">
        <table class="min-w-full text-sm">
          <thead>
            <tr class="text-left text-slate-500 border-b border-slate-100 dark:border-slate-700">
              <th class="py-3">Time</th>
              <th class="py-3">Client IP</th>
              <th class="py-3">Domain</th>
              <th class="py-3">Type</th>
              <th class="py-3">Response</th>
              <th class="py-3">Status</th>
              <th class="py-3">Response Time</th>
              <th class="py-3 text-center w-20">Actions</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="log in filteredQueryLogs" :key="log.id" class="border-b border-slate-100 dark:border-slate-800">
              <td class="py-3 text-xs">{{ formatDate(log.created_at) }}</td>
              <td class="py-3 font-medium">{{ log.client_ip }}</td>
              <td class="py-3 font-mono text-xs truncate max-w-xs">{{ log.domain }}</td>
              <td class="py-3">{{ log.query_type }}</td>
              <td class="py-3 font-mono text-xs relative">
                <button 
                  v-if="log.response" 
                  @click="selectedResponse = log.response"
                  class="group text-slate-700 dark:text-slate-300 hover:text-blue-600 dark:hover:text-blue-400 cursor-pointer text-left"
                >
                  {{ log.response.length > 40 ? log.response.substring(0, 40) + '...' : log.response }}
                  
                  <!-- Tooltip (visible on hover) -->
                  <span 
                    v-if="log.response.length > 40"
                    class="invisible group-hover:visible absolute z-50 left-0 top-full mt-1 px-3 py-2 bg-slate-800 dark:bg-slate-700 text-white text-xs rounded-lg shadow-xl max-w-sm break-all whitespace-normal pointer-events-none"
                  >
                    {{ log.response }}
                    <span class="absolute bottom-full left-4 border-4 border-transparent border-b-slate-800 dark:border-b-slate-700"></span>
                  </span>
                </button>
                <span v-else class="text-slate-400">-</span>
              </td>
              <td class="py-3">
                <span
                  :class="[
                    'px-2 py-1 text-xs rounded-full font-semibold',
                    log.blocked
                      ? 'bg-red-100 text-red-700 dark:bg-red-500/20 dark:text-red-200'
                      : log.block_reason === 'Rewrite'
                        ? 'bg-purple-100 text-purple-700 dark:bg-purple-500/20 dark:text-purple-200'
                        : log.cached
                          ? 'bg-blue-100 text-blue-700 dark:bg-blue-500/20 dark:text-blue-200'
                          : 'bg-green-100 text-green-700 dark:bg-green-500/20 dark:text-green-200'
                  ]"
                >
                  {{ log.blocked ? 'Blocked' : log.block_reason === 'Rewrite' ? 'Rewrite' : log.cached ? 'Cached' : 'Allowed' }}
                </span>
              </td>
              <td class="py-3">{{ log.response_time }}ms</td>
              <td class="py-3 text-center relative">
                <button 
                  @click="toggleLogMenu(log.id)"
                  class="px-2 py-1 hover:bg-slate-100 dark:hover:bg-slate-700 rounded transition-colors"
                >
                  ...
                </button>

                <div 
                  v-if="activeLogMenu === log.id"
                  class="absolute right-0 top-full mt-1 w-64 bg-white dark:bg-slate-800 rounded-lg shadow-xl border border-slate-200 dark:border-slate-700 z-50 py-1"
                  @click.stop
                >
                  <!-- Loading state -->
                  <div v-if="!currentLogStatus || currentLogStatus.logId !== log.id" class="px-4 py-3 text-center text-sm text-slate-500">
                    Loading...
                  </div>

                  <!-- Real-time status loaded -->
                  <template v-else>
                    <!-- Global Actions -->
                    <div class="px-3 py-1 text-xs font-semibold text-slate-500 dark:text-slate-400 uppercase">Global</div>
                    
                    <button
                      v-if="currentLogStatus.global_domain_block"
                      @click="removeGlobalBlock(log.domain)"
                      class="w-full px-4 py-2 text-left text-sm hover:bg-slate-100 dark:hover:bg-slate-700"
                    >
                      ✅ Allow domain (globally)
                    </button>
                    
                    <button
                      v-else
                      @click="blockDomainGlobally(log.domain)"
                      class="w-full px-4 py-2 text-left text-sm hover:bg-slate-100 dark:hover:bg-slate-700"
                    >
                      🚫 Block domain (globally)
                    </button>

                    <div class="border-t border-slate-200 dark:border-slate-700 my-1"></div>

                    <!-- Client-Specific Actions -->
                    <div class="px-3 py-1 text-xs font-semibold text-slate-500 dark:text-slate-400 uppercase">For {{ log.client_ip }}</div>
                    
                    <button
                      v-if="currentLogStatus.client_specific_block"
                      @click="removeClientDomainRule(log.client_ip, log.domain, 'block')"
                      class="w-full px-4 py-2 text-left text-sm hover:bg-slate-100 dark:hover:bg-slate-700"
                    >
                      ✅ Allow for this client
                    </button>
                    
                    <button
                      v-else
                      @click="blockDomainForClient(log.client_ip, log.domain)"
                      class="w-full px-4 py-2 text-left text-sm hover:bg-slate-100 dark:hover:bg-slate-700"
                    >
                      🚫 Block for this client
                    </button>

                    <div class="border-t border-slate-200 dark:border-slate-700 my-1"></div>

                    <!-- Client Ban Actions -->
                    <button
                      v-if="currentLogStatus.client_block"
                      @click="unblockClient(log.client_ip)"
                      class="w-full px-4 py-2 text-left text-sm hover:bg-green-50 dark:hover:bg-green-900/20 text-green-600"
                    >
                      ✅ Unban this client
                    </button>
                    
                    <button
                      v-else
                      @click="blockClient(log.client_ip)"
                      class="w-full px-4 py-2 text-left text-sm hover:bg-red-50 dark:hover:bg-red-900/20 text-red-600"
                    >
                      🚫 Ban this client (IP Ban)
                    </button>
                  </template>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </CardBox>

    <!-- Custom Rules Tab -->
    <CardBox v-if="activeTab === 'custom-rules'">
      <SectionTitleLineWithButton :icon="mdiShieldCheck" title="Custom Rules" main>
        <BaseButton
          label="Refresh"
          :icon="mdiRefresh"
          color="info"
          small
          @click="fetchCustomRules"
        />
      </SectionTitleLineWithButton>

      <!-- Global Filters -->
      <div class="mt-6">
        <h3 class="text-lg font-semibold text-slate-700 dark:text-slate-300 mb-3">🌍 Global Filters</h3>
        <div v-if="customRules.global_filters.length === 0" class="text-center py-8 text-slate-500">
          No global custom filters
        </div>
        <div v-else class="overflow-x-auto">
          <table class="min-w-full text-sm">
            <thead>
              <tr class="text-left text-slate-500 border-b border-slate-200 dark:border-slate-700">
                <th class="py-3">Domain</th>
                <th class="py-3">Type</th>
                <th class="py-3">Comment</th>
                <th class="py-3">Regex</th>
                <th class="py-3">Wildcard</th>
                <th class="py-3 text-center">Actions</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="filter in customRules.global_filters" :key="filter.id" class="border-b border-slate-100 dark:border-slate-800">
                <td class="py-3 font-mono text-xs">{{ filter.domain }}</td>
                <td class="py-3">
                  <span
                    :class="[
                      'px-2 py-1 text-xs rounded-full font-semibold',
                      filter.type === 'blacklist'
                        ? 'bg-red-100 text-red-700 dark:bg-red-500/20 dark:text-red-200'
                        : 'bg-green-100 text-green-700 dark:bg-green-500/20 dark:text-green-200'
                    ]"
                  >
                    {{ filter.type }}
                  </span>
                </td>
                <td class="py-3 text-slate-600 dark:text-slate-400">{{ filter.comment || '-' }}</td>
                <td class="py-3">{{ filter.is_regex ? '✓' : '-' }}</td>
                <td class="py-3">{{ filter.is_wildcard ? '✓' : '-' }}</td>
                <td class="py-3 text-center">
                  <BaseButton
                    :icon="mdiDelete"
                    color="danger"
                    small
                    @click="deleteGlobalFilter(filter.id)"
                  />
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- Client-Specific Rules -->
      <div class="mt-8">
        <h3 class="text-lg font-semibold text-slate-700 dark:text-slate-300 mb-3">👤 Client-Specific Domain Rules</h3>
        <div v-if="customRules.client_rules.length === 0" class="text-center py-8 text-slate-500">
          No client-specific rules
        </div>
        <div v-else class="overflow-x-auto">
          <table class="min-w-full text-sm">
            <thead>
              <tr class="text-left text-slate-500 border-b border-slate-200 dark:border-slate-700">
                <th class="py-3">Client IP</th>
                <th class="py-3">Domain</th>
                <th class="py-3">Type</th>
                <th class="py-3">Comment</th>
                <th class="py-3">Regex</th>
                <th class="py-3">Wildcard</th>
                <th class="py-3 text-center">Actions</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="rule in customRules.client_rules" :key="rule.id" class="border-b border-slate-100 dark:border-slate-800">
                <td class="py-3 font-medium">{{ rule.client_ip }}</td>
                <td class="py-3 font-mono text-xs">{{ rule.domain }}</td>
                <td class="py-3">
                  <span
                    :class="[
                      'px-2 py-1 text-xs rounded-full font-semibold',
                      rule.type === 'block'
                        ? 'bg-red-100 text-red-700 dark:bg-red-500/20 dark:text-red-200'
                        : 'bg-green-100 text-green-700 dark:bg-green-500/20 dark:text-green-200'
                    ]"
                  >
                    {{ rule.type }}
                  </span>
                </td>
                <td class="py-3 text-slate-600 dark:text-slate-400">{{ rule.comment || '-' }}</td>
                <td class="py-3">{{ rule.is_regex ? '✓' : '-' }}</td>
                <td class="py-3">{{ rule.is_wildcard ? '✓' : '-' }}</td>
                <td class="py-3 text-center">
                  <BaseButton
                    :icon="mdiDelete"
                    color="danger"
                    small
                    @click="deleteClientRule(rule.id)"
                  />
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- Banned Clients -->
      <div class="mt-8">
        <h3 class="text-lg font-semibold text-slate-700 dark:text-slate-300 mb-3">🚫 Banned Clients (IP Ban)</h3>
        <div v-if="customRules.banned_clients.length === 0" class="text-center py-8 text-slate-500">
          No banned clients
        </div>
        <div v-else class="overflow-x-auto">
          <table class="min-w-full text-sm">
            <thead>
              <tr class="text-left text-slate-500 border-b border-slate-200 dark:border-slate-700">
                <th class="py-3">Client IP</th>
                <th class="py-3">Reason</th>
                <th class="py-3">Banned At</th>
                <th class="py-3 text-center">Actions</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="client in customRules.banned_clients" :key="client.id" class="border-b border-slate-100 dark:border-slate-800">
                <td class="py-3 font-medium">{{ client.client_ip }}</td>
                <td class="py-3 text-slate-600 dark:text-slate-400">{{ client.block_reason || 'Manually banned' }}</td>
                <td class="py-3 text-xs">{{ client.blocked_at ? formatDate(client.blocked_at) : '-' }}</td>
                <td class="py-3 text-center">
                  <BaseButton
                    label="Unban"
                    color="success"
                    small
                    @click="deleteClientBan(client.client_ip)"
                  />
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </CardBox>

    <!-- Modals -->
    
    <!-- Config Modal -->
    <CardBoxModal
      v-model="isConfigModalActive"
      title="DNS Server Configuration"
      has-cancel
      button-label="Save"
      @confirm="saveConfig"
    >
      <FormField label="Server Enabled">
        <FormCheckRadio
          v-model="config.enabled"
          name="enabled"
          type="checkbox"
          label="Enable DNS Server"
        />
      </FormField>

      <FormField label="UDP Port">
        <FormControl v-model="config.udp_port" type="number" placeholder="53" />
      </FormField>

      <FormField label="TCP Port">
        <FormControl v-model="config.tcp_port" type="number" placeholder="53" />
      </FormField>

      <FormField label="DNS-over-HTTPS (DoH)">
        <FormCheckRadio
          v-model="config.doh_enabled"
          name="doh_enabled"
          type="checkbox"
          label="Enable DoH"
        />
        <FormControl v-model="config.doh_port" type="number" placeholder="443" class="mt-2" />
      </FormField>

      <FormField label="DNS-over-TLS (DoT)">
        <FormCheckRadio
          v-model="config.dot_enabled"
          name="dot_enabled"
          type="checkbox"
          label="Enable DoT"
        />
        <FormControl v-model="config.dot_port" type="number" placeholder="853" class="mt-2" />
      </FormField>

      <FormField label="Upstream DNS Servers (one per line)">
        <FormControl 
          v-model="formattedUpstreamDNS" 
          type="textarea" 
          placeholder="1.1.1.1:53&#10;8.8.8.8:53"
        />
        <p class="text-xs text-gray-500 mt-1">
          Upstream DNS servers to forward queries to. 
          Default: Cloudflare (1.1.1.1), Google (8.8.8.8), Quad9 (9.9.9.9)
        </p>
      </FormField>

      <FormField label="Blocking">
        <FormCheckRadio
          v-model="config.blocking_enabled"
          name="blocking_enabled"
          type="checkbox"
          label="Enable domain blocking"
        />
      </FormField>

      <FormField label="Query Logging">
        <FormCheckRadio
          v-model="config.query_logging"
          name="query_logging"
          type="checkbox"
          label="Enable query logging"
        />
        <FormControl v-model="config.log_retention_days" type="number" placeholder="7" class="mt-2" />
        <p class="text-xs text-gray-500 mt-1">Log retention in days</p>
      </FormField>

      <FormField label="Cache">
        <FormCheckRadio
          v-model="config.cache_enabled"
          name="cache_enabled"
          type="checkbox"
          label="Enable DNS cache"
        />
        <FormControl v-model="config.cache_ttl" type="number" placeholder="3600" class="mt-2" />
        <p class="text-xs text-gray-500 mt-1">Cache TTL in seconds</p>
      </FormField>

      <FormField label="Rate Limiting">
        <FormCheckRadio
          v-model="config.rate_limit_enabled"
          name="rate_limit_enabled"
          type="checkbox"
          label="Enable rate limiting"
        />
        <FormControl v-model="config.rate_limit_qps" type="number" placeholder="100" class="mt-2" />
        <p class="text-xs text-gray-500 mt-1">Queries per second limit</p>
      </FormField>
    </CardBoxModal>

    <!-- Add Blocklist Modal -->
    <CardBoxModal
      v-model="isAddBlocklistModalActive"
      title="Add Blocklist"
      has-cancel
      button-label="Add"
      @confirm="addBlocklist"
    >
      <FormField label="Name">
        <FormControl v-model="newBlocklist.name" placeholder="AdGuard DNS" required />
      </FormField>

      <FormField label="URL">
        <FormControl v-model="newBlocklist.url" placeholder="https://..." required />
      </FormField>

      <FormField label="Update Interval (seconds)">
        <FormControl v-model="newBlocklist.update_interval" type="number" placeholder="86400" />
      </FormField>

      <FormField label="Enabled">
        <FormCheckRadio
          v-model="newBlocklist.enabled"
          name="blocklist_enabled"
          type="checkbox"
          label="Enable this blocklist"
        />
      </FormField>
    </CardBoxModal>

    <!-- Edit Blocklist Modal -->
    <CardBoxModal
      v-model="isEditBlocklistModalActive"
      title="Edit Blocklist"
      has-cancel
      button-label="Update"
      @confirm="updateBlocklist"
    >
      <div v-if="editingBlocklist">
        <FormField label="Name">
          <FormControl v-model="editingBlocklist.name" placeholder="AdGuard DNS" required />
        </FormField>

        <FormField label="URL">
          <FormControl v-model="editingBlocklist.url" placeholder="https://..." required />
        </FormField>

        <FormField label="Update Interval (seconds)">
          <FormControl v-model="editingBlocklist.update_interval" type="number" placeholder="86400" />
        </FormField>

        <FormField label="Enabled">
          <FormCheckRadio
            v-model="editingBlocklist.enabled"
            name="edit_blocklist_enabled"
            type="checkbox"
            label="Enable this blocklist"
          />
        </FormField>
      </div>
    </CardBoxModal>

    <!-- Add Filter Modal -->
    <CardBoxModal
      v-model="isAddFilterModalActive"
      title="Add Custom Filter"
      has-cancel
      button-label="Add"
      @confirm="addFilter"
    >
      <FormField label="Domain">
        <FormControl v-model="newFilter.domain" placeholder="example.com" required />
      </FormField>

      <FormField label="Type">
        <select v-model="newFilter.type" class="w-full px-3 py-2 border rounded">
          <option value="blacklist">Blacklist</option>
          <option value="whitelist">Whitelist</option>
        </select>
      </FormField>

      <FormField label="Options">
        <FormCheckRadio
          v-model="newFilter.is_regex"
          name="is_regex"
          type="checkbox"
          label="Regex pattern"
        />
        <FormCheckRadio
          v-model="newFilter.is_wildcard"
          name="is_wildcard"
          type="checkbox"
          label="Wildcard (*.example.com)"
          class="mt-2"
        />
      </FormField>

      <FormField label="Comment">
        <FormControl v-model="newFilter.comment" placeholder="Optional comment" />
      </FormField>
    </CardBoxModal>

    <!-- Add DNS Rewrite Modal -->
    <CardBoxModal
      v-model="isAddRewriteModalActive"
      title="Add DNS Rewrite"
      has-cancel
      button-label="Add"
      @confirm="addRewrite"
    >
      <div class="space-y-4">
        <!-- Domain Field with Info -->
        <FormField label="Domain Name">
          <FormControl v-model="newRewrite.domain" placeholder="example.org or *.example.org" required />
          <div class="mt-2 p-3 bg-blue-50 dark:bg-blue-900/20 rounded-lg text-sm text-blue-700 dark:text-blue-300">
            <div class="font-medium mb-1">Examples:</div>
            <ul class="list-disc list-inside space-y-1 text-xs">
              <li><code class="bg-white dark:bg-slate-800 px-1 rounded">example.org</code> - This domain only</li>
              <li><code class="bg-white dark:bg-slate-800 px-1 rounded">*.example.org</code> - All subdomains (api.example.org, cdn.example.org, etc.)</li>
            </ul>
          </div>
        </FormField>

        <!-- Answer/Target Field with Info -->
        <FormField label="Target (IP Address / Domain)">
          <FormControl v-model="newRewrite.answer" placeholder="192.168.1.1 or target.example.com" required />
          <div class="mt-2 p-3 bg-emerald-50 dark:bg-emerald-900/20 rounded-lg text-sm text-emerald-700 dark:text-emerald-300">
            <div class="font-medium mb-1">Usage:</div>
            <ul class="list-disc list-inside space-y-1 text-xs">
              <li><strong>IP Address:</strong> <code class="bg-white dark:bg-slate-800 px-1 rounded">192.168.1.100</code> - Resolve to this IP</li>
              <li><strong>Domain:</strong> <code class="bg-white dark:bg-slate-800 px-1 rounded">real.example.com</code> - Create CNAME record</li>
              <li><strong>Special:</strong> <code class="bg-white dark:bg-slate-800 px-1 rounded">A</code> - Preserve A records from upstream</li>
              <li><strong>Special:</strong> <code class="bg-white dark:bg-slate-800 px-1 rounded">AAAA</code> - Preserve AAAA records from upstream</li>
            </ul>
          </div>
        </FormField>

        <!-- Type Field -->
        <FormField label="Record Type">
          <select v-model="newRewrite.type" class="w-full px-3 py-2 border dark:border-slate-600 rounded bg-white dark:bg-slate-800">
            <option value="A">A (IPv4)</option>
            <option value="AAAA">AAAA (IPv6)</option>
            <option value="CNAME">CNAME (Alias)</option>
          </select>
          <div class="mt-1 text-xs text-slate-500">
            Choose A/AAAA for IP address, CNAME for domain
          </div>
        </FormField>

        <!-- Comment Field -->
        <FormField label="Description (Optional)">
          <FormControl v-model="newRewrite.comment" placeholder="Note about this rewrite rule..." />
        </FormField>

        <!-- Enabled Checkbox -->
        <FormField label="Durum">
          <FormCheckRadio
            v-model="newRewrite.enabled"
            name="rewrite_enabled"
            type="checkbox"
            label="Enable rule"
          />
        </FormField>
      </div>
    </CardBoxModal>

    <!-- Edit DNS Rewrite Modal -->
    <CardBoxModal
      v-model="isEditRewriteModalActive"
      title="Edit DNS Rewrite"
      has-cancel
      button-label="Update"
      @confirm="updateRewrite"
    >
      <div v-if="editingRewrite" class="space-y-4">
        <FormField label="Domain Name">
          <FormControl v-model="editingRewrite.domain" placeholder="example.org or *.example.org" required />
        </FormField>

        <FormField label="Target (IP Address / Domain)">
          <FormControl v-model="editingRewrite.answer" placeholder="192.168.1.1 or target.example.com" required />
        </FormField>

        <FormField label="Record Type">
          <select v-model="editingRewrite.type" class="w-full px-3 py-2 border dark:border-slate-600 rounded bg-white dark:bg-slate-800">
            <option value="A">A (IPv4)</option>
            <option value="AAAA">AAAA (IPv6)</option>
            <option value="CNAME">CNAME (Alias)</option>
          </select>
        </FormField>

        <FormField label="Description (Optional)">
          <FormControl v-model="editingRewrite.comment" placeholder="Note about this rewrite rule..." />
        </FormField>

        <FormField label="Durum">
          <FormCheckRadio
            v-model="editingRewrite.enabled"
            name="edit_rewrite_enabled"
            type="checkbox"
            label="Enable rule"
          />
        </FormField>
      </div>
    </CardBoxModal>

    <!-- Delete Confirmation Modal -->
    <CardBoxModal
      v-model="isDeleteModalActive"
      title="Confirm Delete"
      button="danger"
      has-cancel
      button-label="Delete"
      @confirm="confirmDelete"
    >
      <p>Are you sure you want to delete this {{ deleteTarget.type }}?</p>
      <p class="font-semibold mt-2" v-if="deleteTarget.item">
        {{ deleteTarget.item.name || deleteTarget.item.domain }}
      </p>
    </CardBoxModal>

    <!-- Response Detail Modal -->
    <CardBoxModal
      v-model="selectedResponse"
      title="DNS Response"
      has-cancel
    >
      <div class="bg-slate-50 dark:bg-slate-800 p-4 rounded-lg">
        <p class="font-mono text-sm break-all select-all text-slate-800 dark:text-slate-200">
          {{ selectedResponse }}
        </p>
      </div>
      <p class="text-xs text-slate-500 mt-2">
        You can select and copy the text (Ctrl+C / Cmd+C)
      </p>
    </CardBoxModal>
  </div>
</template>
