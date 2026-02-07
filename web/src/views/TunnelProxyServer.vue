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

onMounted(() => {
  loadConfig();
  loadZones();
  tunnelList();
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
  </div>

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
