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
  mdiChartLine,
  mdiCog,
  mdiContentDuplicate,
  mdiDelete,
  mdiDownload,
  mdiLock,
  mdiNetwork,
  mdiPlay,
  mdiPlus,
  mdiQrcode,
  mdiRefresh,
  mdiServer,
  mdiServerNetwork,
  mdiStop,
  mdiTimer
} from '@mdi/js';
import { computed, onMounted, onUnmounted, ref } from "vue";
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

const downloadConfig = async (userId) => {
  try {
    const response = await ApiService.get(`/v1/vpn/users/${userId}/config`, {
      responseType: 'blob'
    })
    
    // Create blob and download
    const blob = new Blob([response.data], { type: 'text/plain' })
    const url = window.URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `wg-${userId}.conf`
    document.body.appendChild(link)
    link.click()
    document.body.removeChild(link)
    window.URL.revokeObjectURL(url)
    
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
          v-for="tab in ['overview', 'servers', 'users', 'statistics']"
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
  </div>
</template>
