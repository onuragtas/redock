<script setup>
import BaseButton from "@/components/BaseButton.vue";
import BaseIcon from "@/components/BaseIcon.vue";
import CardBox from "@/components/CardBox.vue";
import CardBoxModal from "@/components/CardBoxModal.vue";
import FormControl from "@/components/FormControl.vue";
import FormField from "@/components/FormField.vue";
import SectionTitleLineWithButton from "@/components/SectionTitleLineWithButton.vue";
import { useLayoutToggle } from "@/composables/useLayoutToggle";
import { usePaginationFilter } from "@/composables/usePaginationFilter";

import ApiService from "@/services/ApiService";
import {
  mdiAccount,
  mdiCircle,
  mdiConnection,
  mdiContentCopy,
  mdiDelete,
  mdiEarth,
  mdiEthernet,
  mdiChevronLeft,
  mdiChevronRight,
  mdiMagnify,
  mdiRefresh,
  mdiServerNetwork,
  mdiTunnel,
  mdiViewGridOutline,
  mdiViewList
} from "@mdi/js";
import { computed, onMounted, ref } from "vue";
import { useToast } from "vue-toastification";

const proxies = ref([]);
const isDeleteModalActive = ref(false);
const loading = ref(false);
const configLoading = ref(false);
const selectedTunnel = ref(null);
const serverConfig = ref({
  cloudflare_zone_id: "",
  domain_suffix: "",
  enabled: false
});
const cloudflareZones = ref([]);
const configSaving = ref(false);
const toast = useToast();

// Connection info from outside: address and short description per protocol
function getConnectionLines(tunnel) {
  const domain = tunnel.domain || "";
  const port = tunnel.port;
  const protocol = (tunnel.protocol || "http").toLowerCase();
  const lines = [];
  if (protocol === "http" || protocol === "https") {
    const scheme = protocol === "https" ? "https" : "http";
    lines.push({
      label: protocol === "https" ? "HTTPS (Web)" : "HTTP (Web)",
      value: `${scheme}://${domain}`,
      desc: "Open this URL in a browser or send requests to it."
    });
  }
  if (protocol === "tcp" || protocol === "tcp+udp") {
    lines.push({
      label: "TCP",
      value: `${domain}:${port}`,
      desc: "Raw TCP connection (connect with telnet, netcat, or a socket to this address)."
    });
  }
  if (protocol === "udp" || protocol === "tcp+udp") {
    lines.push({
      label: "UDP",
      value: `${domain}:${port}`,
      desc: "Send UDP packets to this target address."
    });
  }
  return lines;
}

function copyConnection(value) {
  navigator.clipboard.writeText(value).then(() => {
    toast.success("Copied to clipboard");
  }).catch(() => {
    toast.error("Copy failed");
  });
}

const tunnelStats = computed(() => {
  const total = proxies.value.length;
  const active = activeCount.value;
  return { total, active };
});

const {
  searchQuery,
  filteredItems,
  paginatedItems,
  currentPage,
  totalPages,
  paginationInfo,
  pages,
  nextPage,
  prevPage,
  goToPage
} = usePaginationFilter(proxies, undefined, 8);

const activeCount = ref(0);

const tunnelList = async () => {
  loading.value = true;
  try {
    const response = await ApiService.tunnelDomainsList();
    if (response?.data?.data) {
      proxies.value = (response.data.data || []).map((d) => ({
        id: d.id,
        domain: d.full_domain || d.subdomain,
        Domain: d.full_domain || d.subdomain,
        port: d.port,
        Port: d.port,
        protocol: d.protocol || "http",
        UpdatedAt: d.created_at,
        created_at: d.created_at,
        owner_email: d.owner_email,
        owner_label: d.owner_label || (d.user_id === 0 ? "admin" : (d.owner_email || "")),
        user_id: d.user_id,
        active: d.active,
        started: d.started,
        bound_user_id: d.bound_user_id,
        status: d.status,
        status_summary: d.status_summary,
        last_used_at: d.last_used_at
      }));
      activeCount.value = response?.data?.active_count ?? proxies.value.filter((p) => p.active).length;
    }
  } catch (error) {
    console.error("Failed to load tunnel domains:", error);
  } finally {
    loading.value = false;
  }
};

const deleteModal = (tunnel) => {
  selectedTunnel.value = tunnel;
  isDeleteModalActive.value = true;
};

const deleteSubmit = async () => {
  if (!selectedTunnel.value) return;
  try {
    await ApiService.tunnelDomainDelete(selectedTunnel.value.id);
    isDeleteModalActive.value = false;
    selectedTunnel.value = null;
    await tunnelList();
  } catch (error) {
    console.error("Failed to delete domain:", error);
  }
};

const formatDate = (dateString) => {
  return new Date(dateString).toLocaleDateString("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit"
  });
};

const GRID_MIN_ITEMS = 2;
const {
  isGridLayout,
  layoutClass,
  toggleLayout
} = useLayoutToggle(paginatedItems, { minItemsForGrid: GRID_MIN_ITEMS });
const layoutToggleLabel = computed(() =>
  isGridLayout.value ? "List View" : "Grid View"
);
const layoutToggleIcon = computed(() =>
  isGridLayout.value ? mdiViewList : mdiViewGridOutline
);

const loadConfig = async () => {
  try {
    const res = await ApiService.tunnelServerGetConfig();
    const d = res?.data?.data;
    if (d) {
      serverConfig.value = {
        cloudflare_zone_id: d.cloudflare_zone_id ?? "",
        domain_suffix: d.domain_suffix ?? "",
        enabled: !!d.enabled
      };
    }
  } catch (e) {
    console.error("Failed to load tunnel server config:", e);
  }
};

const saveConfigField = async (updates) => {
  if (configSaving.value) return;
  configSaving.value = true;
  try {
    await ApiService.tunnelServerUpdateConfig(updates);
    if (updates.enabled !== undefined) serverConfig.value.enabled = updates.enabled;
  } catch (e) {
    console.error("Failed to update tunnel server config:", e);
  } finally {
    configSaving.value = false;
  }
};

const loadZones = async () => {
  configLoading.value = true;
  try {
    const res = await ApiService.getCloudflareZones();
    cloudflareZones.value = res?.data?.data || [];
  } catch (e) {
    console.error("Failed to load Cloudflare zones:", e);
  } finally {
    configLoading.value = false;
  }
};

const onZoneChange = async (zoneId) => {
  if (!zoneId) return;
  const zone = cloudflareZones.value.find((z) => z.zone_id === zoneId);
  const domainSuffix = zone ? zone.name : "";
  try {
    await ApiService.tunnelServerUpdateConfig({
      cloudflare_zone_id: zoneId,
      domain_suffix: domainSuffix
    });
    serverConfig.value = { ...serverConfig.value, cloudflare_zone_id: zoneId, domain_suffix: domainSuffix };
  } catch (e) {
    console.error("Failed to update tunnel server config:", e);
  }
};

// --- Remote agents (control plane) ---
const agents = ref([]);
const agentAssignments = ref({}); // agentId -> assignments[]
const expandedAgentId = ref(null);
const isAssignmentModalActive = ref(false);
const assignmentMode = ref("create"); // create | edit
const assignmentAgentId = ref(null);
const assignmentSaving = ref(false);

const blankAssignment = () => ({
  id: null,
  domain: "",
  local_http_ip: "127.0.0.1",
  local_http_port: 80,
  local_tcp_ip: "",
  local_tcp_port: "",
  local_udp_ip: "",
  local_udp_port: "",
  source_bind_ip: "",
  host_rewrite: "",
  keepalive_interval_seconds: 5,
  auto_connect: true,
  enabled: true
});
const assignmentForm = ref(blankAssignment());

const loadAgents = async () => {
  try {
    const res = await ApiService.tunnelAgentsList();
    agents.value = res?.data?.data || [];
  } catch (e) {
    console.error("Failed to load agents:", e);
  }
};

const loadAgentAssignments = async (agentId) => {
  try {
    const res = await ApiService.tunnelAgentAssignmentsList(agentId);
    agentAssignments.value = { ...agentAssignments.value, [agentId]: res?.data?.data || [] };
  } catch (e) {
    console.error("Failed to load assignments:", e);
  }
};

const toggleAgent = async (agent) => {
  if (expandedAgentId.value === agent.id) {
    expandedAgentId.value = null;
    return;
  }
  expandedAgentId.value = agent.id;
  await loadAgentAssignments(agent.id);
};

const deleteAgent = async (agent) => {
  if (!confirm(`Delete agent '${agent.label || agent.client_id}' and all its assignments?`)) return;
  try {
    await ApiService.tunnelAgentDelete(agent.id);
    toast.success("Agent deleted");
    await loadAgents();
  } catch (e) {
    toast.error("Failed to delete agent: " + (e.response?.data?.msg || e.message));
  }
};

const openCreateAssignment = (agent) => {
  assignmentMode.value = "create";
  assignmentAgentId.value = agent.id;
  assignmentForm.value = blankAssignment();
  isAssignmentModalActive.value = true;
};

const openEditAssignment = (agentId, a) => {
  assignmentMode.value = "edit";
  assignmentAgentId.value = agentId;
  assignmentForm.value = {
    id: a.id,
    domain: a.domain,
    local_http_ip: a.local_http_ip || "",
    local_http_port: a.local_http_port || "",
    local_tcp_ip: a.local_tcp_ip || "",
    local_tcp_port: a.local_tcp_port || "",
    local_udp_ip: a.local_udp_ip || "",
    local_udp_port: a.local_udp_port || "",
    source_bind_ip: a.source_bind_ip || "",
    host_rewrite: a.host_rewrite || "",
    keepalive_interval_seconds: a.keepalive_interval_seconds ?? 30,
    auto_connect: !!a.auto_connect,
    enabled: !!a.enabled
  };
  isAssignmentModalActive.value = true;
};

const saveAssignment = async () => {
  const f = assignmentForm.value;
  const httpP = parseInt(f.local_http_port) || 0;
  const tcpP = parseInt(f.local_tcp_port) || 0;
  const udpP = parseInt(f.local_udp_port) || 0;
  if (httpP <= 0 && tcpP <= 0 && udpP <= 0) {
    toast.warning("Enter at least one local target (HTTP/TCP/UDP ip+port)");
    return;
  }
  assignmentSaving.value = true;
  try {
    const payload = {
      local_http_ip: (f.local_http_ip || "").trim(),
      local_http_port: parseInt(f.local_http_port) || 0,
      local_tcp_ip: (f.local_tcp_ip || "").trim(),
      local_tcp_port: parseInt(f.local_tcp_port) || 0,
      local_udp_ip: (f.local_udp_ip || "").trim(),
      local_udp_port: parseInt(f.local_udp_port) || 0,
      source_bind_ip: (f.source_bind_ip || "").trim(),
      host_rewrite: (f.host_rewrite || "").trim(),
      keepalive_interval_seconds: parseInt(f.keepalive_interval_seconds) || 0,
      auto_connect: !!f.auto_connect,
      enabled: !!f.enabled
    };
    if (assignmentMode.value === "edit") {
      await ApiService.tunnelAgentAssignmentUpdate(assignmentAgentId.value, f.id, payload);
    } else {
      await ApiService.tunnelAgentAssignmentCreate(assignmentAgentId.value, payload);
    }
    isAssignmentModalActive.value = false;
    await loadAgentAssignments(assignmentAgentId.value);
    toast.success("Assignment saved (client applies within ~20s)");
  } catch (e) {
    toast.error("Save failed: " + (e.response?.data?.msg || e.message));
  } finally {
    assignmentSaving.value = false;
  }
};

const toggleAssignmentAutoConnect = async (agentId, a) => {
  try {
    await ApiService.tunnelAgentAssignmentUpdate(agentId, a.id, { auto_connect: !a.auto_connect });
    await loadAgentAssignments(agentId);
  } catch (e) {
    toast.error("Update failed: " + (e.response?.data?.msg || e.message));
  }
};

const deleteAssignment = async (agentId, a) => {
  if (!confirm(`Delete assignment for '${a.domain}'?`)) return;
  try {
    await ApiService.tunnelAgentAssignmentDelete(agentId, a.id);
    await loadAgentAssignments(agentId);
    toast.success("Assignment deleted");
  } catch (e) {
    toast.error("Delete failed: " + (e.response?.data?.msg || e.message));
  }
};

const formatSeen = (ts) => {
  if (!ts) return "—";
  return new Date(ts).toLocaleString();
};

onMounted(() => {
  loadConfig();
  loadZones();
  tunnelList();
  loadAgents();
});
</script>

<template>
  <div class="space-y-8">
    <div
      class="bg-gradient-to-r from-purple-600 via-indigo-600 to-blue-600 rounded-2xl p-8 text-white shadow-lg"
    >
      <div class="flex flex-col lg:flex-row lg:items-center lg:justify-between">
        <div>
          <h1
            class="text-3xl lg:text-4xl font-bold mb-2 flex items-center"
          >
            <BaseIcon :path="mdiServerNetwork" size="40" class="mr-4" />
            Tunnel Proxy Server
          </h1>
          <p class="text-purple-100 text-lg">
            Domain management – Redock tunnel server
          </p>
        </div>
        <div class="mt-6 lg:mt-0 flex space-x-3">
          <BaseButton
            label="Refresh"
            :icon="mdiRefresh"
            color="white"
            outline
            :disabled="loading"
            class="shadow-lg hover:shadow-xl transform hover:scale-105 transition-all duration-200"
            @click="tunnelList"
          />
        </div>
      </div>
    </div>

    <CardBox class="mb-6">
      <FormField label="Tunnel server" help="Enable/disable and Cloudflare zone. Tunnel subdomains are created under this zone; register/login use this setting.">
        <div class="flex flex-wrap items-center gap-6">
          <label class="flex items-center gap-2 cursor-pointer">
            <input
              type="checkbox"
              :checked="serverConfig.enabled"
              :disabled="configSaving"
              class="rounded border-slate-300 dark:border-slate-600"
              @change="saveConfigField({ enabled: $event.target.checked })"
            />
            <span class="text-slate-700 dark:text-slate-200">Tunnel server enabled</span>
          </label>
          <div class="flex items-center gap-2">
            <label for="cloudflare-zone" class="text-slate-700 dark:text-slate-200 shrink-0">Cloudflare zone:</label>
            <select
              id="cloudflare-zone"
              :value="serverConfig.cloudflare_zone_id"
              class="rounded-lg border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-800 px-3 py-2 text-slate-700 dark:text-slate-200 min-w-[180px]"
              :disabled="configLoading || configSaving"
              @change="onZoneChange(($event.target).value)"
            >
              <option value="">— Select zone —</option>
              <option
                v-for="z in cloudflareZones"
                :key="z.zone_id"
                :value="z.zone_id"
              >
                {{ z.name }}
              </option>
            </select>
          </div>
        </div>
      </FormField>
    </CardBox>

    <div class="grid grid-cols-1 md:grid-cols-3 gap-6">
      <CardBox
        class="bg-gradient-to-br from-purple-50 to-purple-100 dark:from-purple-900/20 dark:to-purple-800/20 border-purple-200 dark:border-purple-700"
      >
        <div class="flex items-center justify-between">
          <div>
            <div class="text-2xl font-bold text-purple-600 dark:text-purple-400">
              {{ tunnelStats.total }}
            </div>
            <div class="text-sm text-purple-600/70 dark:text-purple-400/70">
              Total domains
            </div>
          </div>
          <BaseIcon :path="mdiServerNetwork" size="48" class="text-purple-500 opacity-20" />
        </div>
      </CardBox>
      <CardBox
        class="bg-gradient-to-br from-emerald-50 to-emerald-100 dark:from-emerald-900/20 dark:to-emerald-800/20 border-emerald-200 dark:border-emerald-700"
      >
        <div class="flex items-center justify-between">
          <div>
            <div class="text-2xl font-bold text-emerald-600 dark:text-emerald-400">
              {{ tunnelStats.active }}
            </div>
            <div class="text-sm text-emerald-600/70 dark:text-emerald-400/70">
              Active (connected)
            </div>
          </div>
          <BaseIcon :path="mdiConnection" size="48" class="text-emerald-500 opacity-20" />
        </div>
      </CardBox>
      <CardBox
        class="bg-gradient-to-br from-slate-50 to-slate-100 dark:from-slate-800/40 dark:to-slate-700/40 border-slate-200 dark:border-slate-600"
      >
        <div class="flex items-center justify-between">
          <div>
            <div class="text-2xl font-bold text-slate-600 dark:text-slate-300">
              {{ tunnelStats.total - tunnelStats.active }}
            </div>
            <div class="text-sm text-slate-500 dark:text-slate-400">
              Not connected
            </div>
          </div>
          <BaseIcon :path="mdiTunnel" size="48" class="text-slate-400 opacity-30" />
        </div>
      </CardBox>
    </div>

    <CardBox>
      <SectionTitleLineWithButton
        :icon="mdiConnection"
        title="Tunnel Domains"
        main
      >
        <div class="flex flex-col gap-3 md:flex-row md:items-center">
          <div class="w-full md:w-64">
            <FormControl
              v-model="searchQuery"
              :icon="mdiMagnify"
              placeholder="Search domains"
            />
          </div>
          <BaseButton
            :icon="layoutToggleIcon"
            :label="layoutToggleLabel"
            color="lightDark"
            outline
            class="shrink-0"
            @click="toggleLayout"
          />
          <BaseButton
            :icon="mdiRefresh"
            color="info"
            rounded-full
            :disabled="loading"
            class="shadow-sm hover:shadow-md"
            @click="tunnelList"
          />
        </div>
      </SectionTitleLineWithButton>

      <div v-if="loading" class="text-center py-12">
        <div
          class="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-purple-600"
        />
        <p class="text-slate-500 dark:text-slate-400 mt-4">
          Loading domains...
        </p>
      </div>

      <div
        v-else-if="filteredItems.length === 0"
        class="text-center py-12"
      >
        <BaseIcon
          :path="mdiTunnel"
          size="64"
          class="mx-auto text-slate-300 dark:text-slate-600 mb-4"
        />
        <p class="text-slate-500 dark:text-slate-400 mb-4">
          {{
            searchQuery
              ? "No domains match your search."
              : "No domains defined yet."
          }}
        </p>
      </div>

      <div v-else :class="layoutClass">
        <div
          v-for="tunnel in paginatedItems"
          :key="tunnel.id"
          class="p-6 bg-slate-50 dark:bg-slate-800/50 rounded-xl hover:bg-slate-100 dark:hover:bg-slate-700/50 transition-colors flex flex-col h-full"
        >
          <div
            class="flex flex-col gap-6 lg:flex-row lg:items-start lg:justify-between"
          >
            <div class="flex items-start gap-4 flex-1">
              <div class="flex-shrink-0">
                <div
                  class="w-12 h-12 bg-gradient-to-br from-purple-500 to-indigo-600 rounded-xl flex items-center justify-center"
                >
                  <BaseIcon :path="mdiTunnel" size="24" class="text-white" />
                </div>
              </div>
              <div class="space-y-2 flex-1">
                <h3 class="font-semibold text-lg flex items-center flex-wrap gap-2">
                  <BaseIcon :path="mdiEarth" size="20" class="text-blue-500" />
                  {{ tunnel.domain }}
                  <span
                    v-if="tunnel.active"
                    class="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-emerald-100 text-emerald-800 dark:bg-emerald-900/50 dark:text-emerald-200"
                    :title="tunnel.status_summary"
                  >
                    <BaseIcon :path="mdiCircle" size="10" class="fill-current" />
                    Active
                  </span>
                  <span
                    v-else-if="tunnel.started"
                    class="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-amber-100 text-amber-800 dark:bg-amber-900/50 dark:text-amber-200"
                    :title="tunnel.status_summary"
                  >
                    Local client running
                  </span>
                  <span
                    v-else
                    class="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-slate-100 text-slate-600 dark:bg-slate-700 dark:text-slate-300"
                    :title="tunnel.status_summary"
                  >
                    Idle
                  </span>
                </h3>
                <div class="flex flex-wrap items-center gap-x-4 gap-y-2 text-sm text-slate-500 dark:text-slate-400">
                  <span class="flex items-center gap-1" :title="'Owner: ' + (tunnel.owner_label || '—')">
                    <BaseIcon :path="mdiAccount" size="14" class="shrink-0" />
                    <span class="font-medium text-slate-600 dark:text-slate-300">{{ tunnel.owner_label || "—" }}</span>
                  </span>
                  <span class="flex items-center">
                    <BaseIcon :path="mdiEthernet" size="16" class="mr-1 shrink-0" />
                    Port: {{ tunnel.port }}
                  </span>
                  <span
                    v-if="['tcp', 'tcp+udp'].includes(tunnel.protocol)"
                    class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-300"
                  >
                    TCP
                  </span>
                  <span
                    v-if="['udp', 'tcp+udp'].includes(tunnel.protocol)"
                    class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-cyan-100 text-cyan-800 dark:bg-cyan-900/40 dark:text-cyan-300"
                  >
                    UDP
                  </span>
                  <span
                    v-if="['http', 'https'].includes(tunnel.protocol)"
                    class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-emerald-100 text-emerald-800 dark:bg-emerald-900/40 dark:text-emerald-300"
                  >
                    {{ tunnel.protocol === "https" ? "HTTPS" : "HTTP" }}
                  </span>
                </div>
                <p v-if="tunnel.status_summary" class="text-xs text-slate-500 dark:text-slate-400">
                  {{ tunnel.status_summary }}
                </p>
                <div class="flex flex-wrap gap-x-4 gap-y-1 text-xs text-slate-500 dark:text-slate-400">
                  <span>Created: {{ formatDate(tunnel.UpdatedAt || tunnel.created_at) }}</span>
                  <span v-if="tunnel.last_used_at">Last used: {{ formatDate(tunnel.last_used_at) }}</span>
                </div>
                <!-- How to connect from outside -->
                <div class="mt-4 pt-4 border-t border-slate-200 dark:border-slate-600">
                  <p class="text-xs font-medium text-slate-500 dark:text-slate-400 mb-2">
                    How to connect from outside
                  </p>
                  <div class="space-y-2">
                    <div
                      v-for="line in getConnectionLines(tunnel)"
                      :key="line.label"
                      class="flex flex-col gap-1"
                    >
                      <div class="flex items-center gap-2 flex-wrap">
                        <span class="text-xs font-medium text-slate-600 dark:text-slate-300">{{ line.label }}:</span>
                        <code class="text-xs bg-slate-200 dark:bg-slate-700 px-2 py-1 rounded flex-1 min-w-0 truncate max-w-full">
                          {{ line.value }}
                        </code>
                        <BaseButton
                          :icon="mdiContentCopy"
                          color="lightDark"
                          small
                          title="Copy"
                          @click="copyConnection(line.value)"
                        />
                      </div>
                      <p class="text-xs text-slate-500 dark:text-slate-400 pl-0">
                        {{ line.desc }}
                      </p>
                    </div>
                  </div>
                </div>
              </div>
            </div>
            <div
              class="mt-6 flex flex-wrap items-center justify-end gap-2"
            >
              <BaseButton
                :icon="mdiDelete"
                color="danger"
                small
                title="Delete"
                @click="deleteModal(tunnel)"
              />
            </div>
          </div>
        </div>
      </div>

      <div
        v-if="filteredItems.length > 0"
        class="flex flex-col gap-4 md:flex-row md:items-center md:justify-between mt-6 px-6 pb-4"
      >
        <div class="text-sm text-slate-500 dark:text-slate-400">
          {{ paginationInfo }}
        </div>
        <div class="flex items-center gap-2">
          <BaseButton
            :icon="mdiChevronLeft"
            color="lightDark"
            small
            :disabled="currentPage === 1"
            @click="prevPage"
          />
          <div class="flex flex-wrap gap-1">
            <BaseButton
              v-for="page in pages"
              :key="page"
              :label="String(page)"
              color="lightDark"
              small
              :active="page === currentPage"
              @click="goToPage(page)"
            />
          </div>
          <BaseButton
            :icon="mdiChevronRight"
            color="lightDark"
            small
            :disabled="currentPage === totalPages"
            @click="nextPage"
          />
        </div>
      </div>
    </CardBox>

    <!-- Remote Agents (control plane) -->
    <CardBox>
      <SectionTitleLineWithButton :icon="mdiServerNetwork" title="Remote Agents" main>
        <BaseButton :icon="mdiRefresh" color="info" small label="Refresh" @click="loadAgents" />
      </SectionTitleLineWithButton>

      <div v-if="agents.length === 0" class="text-center py-10 text-slate-500">
        No agents registered yet. A client appears here once it registers with this server.
      </div>

      <div v-else class="space-y-3">
        <div
          v-for="agent in agents"
          :key="agent.id"
          class="border border-slate-200 dark:border-slate-700 rounded-xl overflow-hidden"
        >
          <div class="flex items-center justify-between p-4 gap-3">
            <div class="flex items-center gap-3 min-w-0">
              <BaseIcon
                :path="mdiCircle"
                size="14"
                :class="agent.online ? 'text-green-500' : 'text-slate-400'"
              />
              <div class="min-w-0">
                <div class="font-semibold truncate">
                  {{ agent.label || agent.client_id }}
                  <span class="text-xs text-slate-400 font-normal">({{ agent.online ? "online" : "offline" }})</span>
                </div>
                <div class="text-xs text-slate-500 truncate">
                  {{ agent.client_id }} · last seen: {{ formatSeen(agent.last_seen_at) }}
                </div>
              </div>
            </div>
            <div class="flex items-center gap-2 shrink-0">
              <BaseButton :icon="mdiConnection" color="success" small label="Assign tunnel" @click="openCreateAssignment(agent)" />
              <BaseButton
                :icon="expandedAgentId === agent.id ? mdiChevronLeft : mdiChevronRight"
                color="lightDark"
                small
                @click="toggleAgent(agent)"
              />
              <BaseButton :icon="mdiDelete" color="danger" small @click="deleteAgent(agent)" />
            </div>
          </div>

          <div v-if="expandedAgentId === agent.id" class="border-t border-slate-200 dark:border-slate-700 p-4 bg-slate-50 dark:bg-slate-800/40">
            <div v-if="!(agentAssignments[agent.id] || []).length" class="text-sm text-slate-500 py-2">
              No tunnels assigned. Use "Assign tunnel" to add one.
            </div>
            <table v-else class="min-w-full text-sm">
              <thead>
                <tr class="text-left text-slate-500">
                  <th class="py-1 pr-3">Domain</th>
                  <th class="py-1 pr-3">Target (HTTP/TCP/UDP)</th>
                  <th class="py-1 pr-3">Auto</th>
                  <th class="py-1 pr-3 text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="a in agentAssignments[agent.id]" :key="a.id" class="border-t border-slate-200 dark:border-slate-700">
                  <td class="py-2 pr-3 font-mono">{{ a.domain }}</td>
                  <td class="py-2 pr-3 font-mono text-xs text-slate-500">
                    <span v-if="a.local_http_port">{{ a.local_http_ip }}:{{ a.local_http_port }} (http)</span>
                    <span v-if="a.local_tcp_port"> · {{ a.local_tcp_ip }}:{{ a.local_tcp_port }} (tcp)</span>
                    <span v-if="a.local_udp_port"> · {{ a.local_udp_ip }}:{{ a.local_udp_port }} (udp)</span>
                  </td>
                  <td class="py-2 pr-3">
                    <button
                      type="button"
                      class="px-2 py-0.5 rounded text-xs font-medium"
                      :class="a.auto_connect ? 'bg-green-100 text-green-700 dark:bg-green-900/40 dark:text-green-300' : 'bg-slate-200 text-slate-500 dark:bg-slate-700'"
                      @click="toggleAssignmentAutoConnect(agent.id, a)"
                    >
                      {{ a.auto_connect ? "ON" : "OFF" }}
                    </button>
                  </td>
                  <td class="py-2 pr-3 text-right">
                    <div class="inline-flex gap-2">
                      <BaseButton :icon="mdiRefresh" color="info" small title="Edit" @click="openEditAssignment(agent.id, a)" />
                      <BaseButton :icon="mdiDelete" color="danger" small title="Delete" @click="deleteAssignment(agent.id, a)" />
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </CardBox>
  </div>

  <!-- Assignment Create/Edit Modal -->
  <CardBoxModal
    v-model="isAssignmentModalActive"
    :title="assignmentMode === 'edit' ? 'Edit Tunnel Assignment' : 'Assign Tunnel'"
    :button-label="assignmentSaving ? 'Saving...' : 'Save'"
    has-cancel
    @confirm="saveAssignment"
  >
    <div class="space-y-3">
      <p v-if="assignmentMode === 'edit'" class="text-xs text-slate-500">
        Domain: <span class="font-mono">{{ assignmentForm.domain }}</span>
      </p>
      <p v-else class="text-xs text-slate-500">
        The domain is auto-generated (random subdomain). Just enter the local target the client will forward to.
      </p>
      <div class="grid grid-cols-2 gap-3">
        <FormField label="HTTP IP">
          <FormControl v-model="assignmentForm.local_http_ip" placeholder="127.0.0.1" />
        </FormField>
        <FormField label="HTTP Port">
          <FormControl v-model="assignmentForm.local_http_port" type="number" placeholder="80" />
        </FormField>
        <FormField label="TCP IP">
          <FormControl v-model="assignmentForm.local_tcp_ip" placeholder="(optional)" />
        </FormField>
        <FormField label="TCP Port">
          <FormControl v-model="assignmentForm.local_tcp_port" type="number" placeholder="" />
        </FormField>
        <FormField label="UDP IP">
          <FormControl v-model="assignmentForm.local_udp_ip" placeholder="(optional)" />
        </FormField>
        <FormField label="UDP Port">
          <FormControl v-model="assignmentForm.local_udp_port" type="number" placeholder="" />
        </FormField>
      </div>
      <p class="text-xs text-slate-500">
        This ip:port is the target the client forwards to on its own network. At least one (HTTP/TCP/UDP) must be set.
      </p>
      <div class="grid grid-cols-2 gap-3">
        <FormField label="Source Bind IP">
          <FormControl v-model="assignmentForm.source_bind_ip" placeholder="(optional)" />
        </FormField>
        <FormField label="Keepalive (s)">
          <FormControl v-model="assignmentForm.keepalive_interval_seconds" type="number" placeholder="30" />
        </FormField>
      </div>
      <FormField label="Host Rewrite">
        <FormControl v-model="assignmentForm.host_rewrite" placeholder="(optional, HTTP Host override)" />
      </FormField>
      <label class="inline-flex items-center gap-2 cursor-pointer">
        <input v-model="assignmentForm.auto_connect" type="checkbox" class="form-checkbox" />
        <span class="text-sm">Auto-connect (client opens it automatically)</span>
      </label>
      <br />
      <label class="inline-flex items-center gap-2 cursor-pointer">
        <input v-model="assignmentForm.enabled" type="checkbox" class="form-checkbox" />
        <span class="text-sm">Enabled</span>
      </label>
    </div>
  </CardBoxModal>

  <!-- Delete Confirmation Modal -->
  <CardBoxModal
    v-model="isDeleteModalActive"
    title="Delete Domain"
    button="danger"
    button-label="Delete"
    has-cancel
    @confirm="deleteSubmit"
  >
    <div v-if="selectedTunnel" class="space-y-4">
      <div
        class="bg-red-50 dark:bg-red-900/20 p-4 rounded-lg"
      >
        <h4
          class="font-semibold text-red-800 dark:text-red-200"
        >
          {{ selectedTunnel.domain }}
        </h4>
        <p class="text-sm text-red-600 dark:text-red-300 mt-1">
          Port: {{ selectedTunnel.port }}
        </p>
      </div>
      <p class="text-slate-600 dark:text-slate-400">
        This domain will be permanently deleted.
      </p>
    </div>
  </CardBoxModal>
</template>

<style scoped>
.animate-spin {
  animation: spin 1s linear infinite;
}
@keyframes spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}
</style>
