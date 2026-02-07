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
  mdiAccountPlus,
  mdiAutorenew,
  mdiCheckCircle,
  mdiChevronLeft,
  mdiChevronRight,
  mdiCloseCircle,
  mdiConnection,
  mdiContentCopy,
  mdiDelete,
  mdiEarth,
  mdiEmail,
  mdiEthernet,
  mdiLan,
  mdiLock,
  mdiLogin,
  mdiLogout,
  mdiMagnify,
  mdiPlay,
  mdiPlus,
  mdiRefresh,
  mdiServer,
  mdiStop,
  mdiTunnel,
  mdiViewGridOutline,
  mdiViewList
} from "@mdi/js";
import { computed, onMounted, ref, watch } from "vue";
import { useToast } from "vue-toastification";
import { useRoute, useRouter } from "vue-router";

const route = useRoute();
const router = useRouter();
const login = ref(false);
const proxies = ref([]);
const isAddModalActive = ref(false);
const isStartModalActive = ref(false);
const isRegisterModalActive = ref(false);
const isDeleteModalActive = ref(false);
const isAddServerModalActive = ref(false);
const isExternalLoginModalActive = ref(false);
const externalLoginLoading = ref(false);
const externalAuthMode = ref("login"); // "login" | "register"
const loading = ref(false);
const addLoading = ref(false);
const selectedTunnel = ref(null);
const startDomain = ref({});
const tunnelServers = ref([]);
const selectedServerId = ref(null);
const serversLoading = ref(false);
const addServerForm = ref({ name: "", base_url: "" });
const selectedServer = computed(
  () => tunnelServers.value.find((s) => s.id === selectedServerId.value) || null
);

const credentials = ref({
  email: "",
  password: ""
});

const start = ref({
  localHttpIp: "127.0.0.1",
  localHttpPort: 80,
  localTcpIp: "127.0.0.1",
  localTcpPort: "",
  localUdpIp: "127.0.0.1",
  localUdpPort: "",
  sourceBindIp: "",
  hostRewrite: ""
});

const toast = useToast();

// Connection info from outside: address and short description per protocol
function getConnectionLines(tunnel) {
  const domain = tunnel.domain || "";
  const port = tunnel.port;
  const protocol = (tunnel.protocol || "all").toLowerCase();
  const lines = [];
  const hasHTTP = protocol === "http" || protocol === "https" || protocol === "all";
  const hasTCP = protocol === "tcp" || protocol === "tcp+udp" || protocol === "all";
  const hasUDP = protocol === "udp" || protocol === "tcp+udp" || protocol === "all";
  if (hasHTTP) {
    lines.push(
      { label: "HTTP (Web)", value: `http://${domain}`, desc: "Open this URL in a browser or send HTTP requests." },
      { label: "HTTPS (Web)", value: `https://${domain}`, desc: "Secure HTTPS endpoint." }
    );
  }
  if (hasTCP) {
    lines.push({
      label: "TCP",
      value: `${domain}:${port}`,
      desc: "Raw TCP connection (connect with telnet, netcat, or a socket to this address)."
    });
  }
  if (hasUDP) {
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
  const active = proxies.value.filter((proxy) => proxy.started).length;
  return { total, active, inactive: total - active };
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

const checkLogin = async () => {
  if (!selectedServer.value) return;
  try {
    const ctx = ApiService.getTunnelServerContext();
    const canList = !!(ctx && ctx.token);
    if (canList) {
      await tunnelList();
      if (proxies.value.length >= 0) login.value = true;
      return;
    }
    login.value = false;
  } catch (error) {
    console.error("Login check failed:", error);
    login.value = false;
  }
};

const loginSubmit = async () => {
  try {
    const response = await ApiService.tunnelLogin(
      credentials.value.email,
      credentials.value.password
    );
    if (response?.data?.data?.token) {
      ApiService.setTunnelToken(response.data.data.token);
    }
    if (
      !response?.data?.error &&
      (response?.data?.data?.token || response?.data?.data)
    ) {
      login.value = true;
      await tunnelList();
    }
  } catch (error) {
    console.error("Login failed:", error);
  }
};

const registerSubmit = async () => {
  try {
    const response = await ApiService.tunnelRegister(
      credentials.value.email,
      credentials.value.password
    );
    if (response?.data?.data?.token) {
      ApiService.setTunnelToken(response.data.data.token);
    }
    if (
      !response?.data?.error &&
      (response?.data?.data?.token || response?.data?.data)
    ) {
      login.value = true;
      isRegisterModalActive.value = false;
      await tunnelList();
    } else {
      isRegisterModalActive.value = false;
    }
  } catch (error) {
    console.error("Registration failed:", error);
  }
};

const logoutSubmit = async () => {
  try {
    ApiService.clearTunnelToken();
    ApiService.setTunnelServerContext(null);
    login.value = false;
    proxies.value = [];
    credentials.value = { email: "", password: "" };
    start.value = {
      destinationIp: "127.0.0.1",
      destinationPort: 80,
      localUdpIp: "127.0.0.1",
      localUdpPort: "",
      sourceBindIp: "",
      hostRewrite: ""
    };
    selectedTunnel.value = null;
    isAddModalActive.value = false;
    isStartModalActive.value = false;
    isDeleteModalActive.value = false;
    isExternalLoginModalActive.value = false;
  } catch (error) {
    console.error("Logout failed:", error);
    ApiService.clearTunnelToken();
    ApiService.setTunnelServerContext(null);
  }
};

const tunnelList = async () => {
  if (!selectedServer.value) return;
  loading.value = true;
  try {
    const response = await ApiService.tunnelDomainsList(selectedServerId.value);
    const data = response?.data?.data || [];
    proxies.value = data.map((d) => ({
      id: d.id,
      domain: d.full_domain || d.subdomain,
      port: d.port,
      protocol: d.protocol || "all",
      started: !!d.started,
      created_at: d.created_at,
      UpdatedAt: d.updated_at || d.created_at
    }));
    login.value = true;
  } catch (error) {
    console.error("Failed to load tunnel list:", error);
    login.value = false;
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
    await ApiService.tunnelDomainDelete(selectedTunnel.value.id, selectedServerId.value);
    isDeleteModalActive.value = false;
    selectedTunnel.value = null;
    await tunnelList();
  } catch (error) {
    console.error("Failed to delete tunnel:", error);
  }
};

const addSubmit = async () => {
  if (addLoading.value) return;
  addLoading.value = true;
  try {
    await ApiService.tunnelDomainCreate({ protocol: 'all' }, selectedServerId.value);
    await tunnelList();
    isAddModalActive.value = false;
  } catch (error) {
    console.error('Failed to create tunnel:', error);
  } finally {
    addLoading.value = false;
  }
};

const startModal = (data) => {
  startDomain.value = data;
  isStartModalActive.value = true;
};

const startSubmit = async () => {
  try {
    const httpIp = (start.value.localHttpIp || "").trim();
    const httpPort = parseInt(start.value.localHttpPort) || 0;
    const tcpIp = (start.value.localTcpIp || "").trim();
    const tcpPort = parseInt(start.value.localTcpPort) || 0;
    const udpIp = (start.value.localUdpIp || "").trim();
    const udpPort = parseInt(start.value.localUdpPort) || 0;
    const hasHttp = httpIp !== "" && httpPort > 0;
    const hasTcp = tcpIp !== "" && tcpPort > 0;
    const hasUdp = udpIp !== "" && udpPort > 0;
    if (!hasHttp && !hasTcp && !hasUdp) {
      toast.warning("Fill at least one target: HTTP, TCP, or UDP (IP + Port)");
      return;
    }
    const data = {
      DomainId: startDomain.value.id,
      Domain: startDomain.value.domain,
      LocalHttpIp: hasHttp ? httpIp : "",
      LocalHttpPort: hasHttp ? httpPort : 0,
      LocalTcpIp: hasTcp ? tcpIp : "",
      LocalTcpPort: hasTcp ? tcpPort : 0,
      LocalUdpIp: hasUdp ? udpIp : "",
      LocalUdpPort: hasUdp ? udpPort : 0,
      SourceBindIp: (start.value.sourceBindIp || "").trim(),
      HostRewrite: (start.value.hostRewrite || "").trim()
    };
    await ApiService.tunnelStart(data, selectedServerId.value);
    isStartModalActive.value = false;
    setTimeout(() => {
      tunnelList();
    }, 2000);
  } catch (error) {
    console.error("Failed to start tunnel:", error);
  }
};

const stopModal = async (item) => {
  try {
    const data = {
      DomainId: item.id,
      Domain: item.domain
    };
    await ApiService.tunnelStop(data, selectedServerId.value);
    setTimeout(() => {
      tunnelList();
    }, 2000);
  } catch (error) {
    console.error("Failed to stop tunnel:", error);
  }
};

const renewTunnel = async (item) => {
  try {
    const data = {
      id: item.id,
      domain: item.domain
    };
    await ApiService.tunnelRenew(data, selectedServerId.value);
    setTimeout(() => {
      tunnelList();
    }, 2000);
  } catch (error) {
    console.error("Failed to renew tunnel:", error);
  }
};

const getStatusColor = (started) => {
  return started
    ? "text-green-600 bg-green-100 dark:text-green-400 dark:bg-green-900/30"
    : "text-gray-600 bg-gray-100 dark:text-gray-400 dark:bg-gray-900/30";
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

const loadServers = async () => {
  if (!ApiService.getJWT()) return;
  serversLoading.value = true;
  try {
    const res = await ApiService.tunnelServersList();
    tunnelServers.value = res?.data?.data || [];
    if (selectedServerId.value == null && tunnelServers.value.length > 0) {
      const def = tunnelServers.value.find((s) => s.is_default) || tunnelServers.value[0];
      selectedServerId.value = def?.id ?? null;
    }
  } catch (e) {
    console.error("Failed to load tunnel servers:", e);
  } finally {
    serversLoading.value = false;
  }
};

const addServerSubmit = async () => {
  const name = (addServerForm.value.name || "").trim() || "Tunnel Server";
  const base_url = (addServerForm.value.base_url || "").trim();
  try {
    await ApiService.tunnelServerCreate({ name, base_url });
    addServerForm.value = { name: "", base_url: "" };
    isAddServerModalActive.value = false;
    await loadServers();
  } catch (e) {
    console.error("Failed to add server:", e);
  }
};

const setDefaultServer = async (server) => {
  try {
    await ApiService.tunnelServerUpdate(server.id, { is_default: true });
    await loadServers();
  } catch (e) {
    console.error("Failed to set default server:", e);
  }
};

const deleteServer = async (server) => {
  if (tunnelServers.value.length <= 1) return;
  try {
    await ApiService.tunnelServerDelete(server.id);
    if (selectedServerId.value === server.id) {
      const next = tunnelServers.value.find((s) => s.id !== server.id);
      selectedServerId.value = next?.id ?? null;
    }
    await loadServers();
  } catch (e) {
    console.error("Failed to delete server:", e);
  }
};

const DEFAULT_TUNNEL_SERVER_URL = "https://redock.tnpx.org";

const getEffectiveBaseUrl = (server) => {
  const url = (server?.base_url || "").trim();
  return url || DEFAULT_TUNNEL_SERVER_URL;
};

const applyTunnelServerContext = async (server) => {
  if (!server) {
    ApiService.setTunnelServerContext(null);
    return;
  }
  const effectiveBaseUrl = getEffectiveBaseUrl(server);
  try {
    const res = await ApiService.tunnelCredentialsList(effectiveBaseUrl);
    const d = res?.data?.data;
    if (d?.has_token && d?.access_token) {
      ApiService.setTunnelServerContext({ baseURL: effectiveBaseUrl, token: d.access_token });
      isExternalLoginModalActive.value = false;
      login.value = true;
    } else {
      ApiService.setTunnelServerContext(null);
      isExternalLoginModalActive.value = false;
      externalAuthMode.value = "login";
      login.value = false;
    }
  } catch (e) {
    console.error("Failed to load credential:", e);
    ApiService.setTunnelServerContext(null);
    isExternalLoginModalActive.value = false;
    externalAuthMode.value = "login";
    login.value = false;
  }
};

const externalLoginSubmit = async () => {
  if (!selectedServer.value) return;
  const effectiveBaseUrl = getEffectiveBaseUrl(selectedServer.value);
  externalLoginLoading.value = true;
  try {
    let token;
    if (externalAuthMode.value === "register") {
      const res = await ApiService.tunnelRegisterExternal(
        effectiveBaseUrl,
        credentials.value.email,
        credentials.value.password
      );
      token = res?.data?.data?.token;
    } else {
      const res = await ApiService.tunnelLoginExternal(
        effectiveBaseUrl,
        credentials.value.email,
        credentials.value.password
      );
      token = res?.data?.data?.token;
    }
    if (token) {
      await ApiService.tunnelCredentialSave({
        base_url: effectiveBaseUrl,
        access_token: token
      });
      ApiService.setTunnelServerContext({
        baseURL: effectiveBaseUrl,
        token
      });
      isExternalLoginModalActive.value = false;
      externalAuthMode.value = "login";
      login.value = true;
      await tunnelList();
    }
  } catch (e) {
    console.error("External auth failed:", e);
  } finally {
    externalLoginLoading.value = false;
  }
};

const tunnelAuthPrepareLoading = ref(false);

const goToTunnelAuth = async () => {
  const server = selectedServer.value;
  if (!server || !ApiService.getJWT()) return;
  tunnelAuthPrepareLoading.value = true;
  try {
    const ourOrigin = typeof window !== "undefined" ? window.location.origin + window.location.pathname : "";
    const clientRedirect = ourOrigin + "#/tunnel-proxy-client?server=" + encodeURIComponent(server.id);
    const res = await ApiService.tunnelAuthPrepare(server.id, clientRedirect);
    const state = res?.data?.data?.state;
    if (!state) {
      console.error("Prepare: state not received");
      return;
    }
    const baseUrl = getEffectiveBaseUrl(server);
    const redirectOrigin = typeof window !== "undefined" ? window.location.origin : "";
    const params = new URLSearchParams({
      state: String(state),
      server_id: String(server.id),
      base_url: baseUrl,
      server_name: server.name || "Tunnel server",
      redirect_origin: redirectOrigin
    });
    const authUrl = baseUrl.replace(/\/$/, "") + "/#/tunnel-auth?" + params.toString();
    window.location.href = authUrl;
  } catch (e) {
    console.error("Tunnel auth prepare failed:", e);
  } finally {
    tunnelAuthPrepareLoading.value = false;
  }
};

watch(selectedServerId, async (newId) => {
  const server = tunnelServers.value.find((s) => s.id === newId) || null;
  await applyTunnelServerContext(server);
  if (server && login.value) {
    await tunnelList();
  }
});

onMounted(async () => {
  await loadServers();

  const q = route.query;
  const tunnelToken = typeof q.tunnel_token === "string" ? q.tunnel_token : "";
  const tunnelBaseUrl = typeof q.tunnel_base_url === "string" ? q.tunnel_base_url : "";
  const serverFromUrl = q.server != null ? String(q.server) : null;

  if (serverFromUrl) {
    const id = parseInt(serverFromUrl, 10);
    if (!isNaN(id) && tunnelServers.value.some((s) => s.id === id)) {
      selectedServerId.value = id;
    }
  }

  if (tunnelToken && tunnelBaseUrl) {
    ApiService.setTunnelServerContext({ baseURL: tunnelBaseUrl, token: tunnelToken });
    login.value = true;
    const cleanQuery = { ...route.query };
    delete cleanQuery.tunnel_token;
    delete cleanQuery.tunnel_base_url;
    router.replace({ path: route.path, query: cleanQuery }).catch(() => {});
    if (selectedServer.value) {
      await tunnelList();
    }
    return;
  }

  if (selectedServer.value) {
    await applyTunnelServerContext(selectedServer.value);
  }
  await checkLogin();
});
</script>

<template>
  <div class="space-y-8">
    <div
      class="bg-gradient-to-r from-purple-600 via-indigo-600 to-blue-600 rounded-2xl p-8 text-white shadow-lg"
    >
      <div class="flex flex-col lg:flex-row lg:items-center lg:justify-between">
        <div>
          <h1 class="text-3xl lg:text-4xl font-bold mb-2 flex items-center">
            <BaseIcon :path="mdiTunnel" size="40" class="mr-4" />
            Tunnel Proxy Client
          </h1>
        </div>
        <div v-if="login" class="mt-6 lg:mt-0 flex space-x-3">
          <BaseButton
            label="Refresh"
            :icon="mdiRefresh"
            color="white"
            outline
            :disabled="loading"
            class="shadow-lg hover:shadow-xl transform hover:scale-105 transition-all duration-200"
            @click="tunnelList"
          />
          <BaseButton
            label="Sign out"
            :icon="mdiLogout"
            color="white"
            outline
            class="shadow-lg hover:shadow-xl transform hover:scale-105 transition-all duration-200"
            @click="logoutSubmit"
          />
        </div>
      </div>
    </div>

    <CardBox v-if="ApiService.getJWT()" class="mb-6">
      <FormField label="Tunnel server" help="Domain list and tunnel operations use the selected server.">
        <div class="flex flex-wrap items-center gap-3">
          <select
            v-model="selectedServerId"
            class="rounded-lg border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-800 px-3 py-2 text-slate-700 dark:text-slate-200 min-w-[200px]"
          >
            <option
              v-for="s in tunnelServers"
              :key="s.id"
              :value="s.id"
            >
              {{ s.name }}{{ s.is_default ? " (default)" : "" }}
            </option>
          </select>
          <BaseButton
            label="Set as default"
            :icon="mdiCheckCircle"
            color="info"
            outline
            small
            :disabled="!selectedServer || selectedServer.is_default"
            @click="selectedServer && setDefaultServer(selectedServer)"
          />
          <BaseButton
            label="Add another server"
            :icon="mdiPlus"
            color="info"
            outline
            @click="isAddServerModalActive = true"
          />
          <BaseButton
            v-if="selectedServer && tunnelServers.length > 1"
            label="Delete server"
            :icon="mdiDelete"
            color="danger"
            outline
            small
            @click="selectedServer && deleteServer(selectedServer)"
          />
        </div>
      </FormField>
    </CardBox>

    <CardBox
      v-if="ApiService.getJWT() && tunnelServers.length > 0 && selectedServer && !login"
      class="mb-6 border-2 border-purple-200 dark:border-purple-700 bg-purple-50/50 dark:bg-purple-900/10"
    >
      <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div class="flex items-center gap-3">
          <BaseIcon :path="mdiConnection" size="32" class="text-purple-600 dark:text-purple-400" />
          <div>
            <p class="font-medium text-slate-800 dark:text-slate-200">
              Sign in to connect to {{ selectedServer.name }}
            </p>
            <p class="text-sm text-slate-500 dark:text-slate-400">
              Sign in or register on this server to view the tunnel list and manage tunnels.
            </p>
          </div>
        </div>
        <BaseButton
          :label="tunnelAuthPrepareLoading ? 'Redirecting...' : 'Connect'"
          :icon="mdiLogin"
          color="info"
          class="shrink-0"
          :disabled="tunnelAuthPrepareLoading"
          @click="goToTunnelAuth"
        />
      </div>
    </CardBox>

    <div class="grid grid-cols-1 md:grid-cols-3 gap-6">
      <CardBox
        class="bg-gradient-to-br from-purple-50 to-purple-100 dark:from-purple-900/20 dark:to-purple-800/20 border-purple-200 dark:border-purple-700"
      >
        <div class="flex items-center justify-between">
          <div>
            <div
              class="text-2xl font-bold text-purple-600 dark:text-purple-400"
            >
              {{ tunnelStats.total }}
            </div>
            <div
              class="text-sm text-purple-600/70 dark:text-purple-400/70"
            >
              Total Tunnels
            </div>
          </div>
          <BaseIcon
            :path="mdiServer"
            size="48"
            class="text-purple-500 opacity-20"
          />
        </div>
      </CardBox>

      <CardBox
        class="bg-gradient-to-br from-green-50 to-green-100 dark:from-green-900/20 dark:to-green-800/20 border-green-200 dark:border-green-700"
      >
        <div class="flex items-center justify-between">
          <div>
            <div
              class="text-2xl font-bold text-green-600 dark:text-green-400"
            >
              {{ tunnelStats.active }}
            </div>
            <div
              class="text-sm text-green-600/70 dark:text-green-400/70"
            >
              Active Tunnels
            </div>
          </div>
          <BaseIcon
            :path="mdiPlay"
            size="48"
            class="text-green-500 opacity-20"
          />
        </div>
      </CardBox>

      <CardBox
        class="bg-gradient-to-br from-gray-50 to-gray-100 dark:from-gray-900/20 dark:to-gray-800/20 border-gray-200 dark:border-gray-700"
      >
        <div class="flex items-center justify-between">
          <div>
            <div
              class="text-2xl font-bold text-gray-600 dark:text-gray-400"
            >
              {{ tunnelStats.inactive }}
            </div>
            <div
              class="text-sm text-gray-600/70 dark:text-gray-400/70"
            >
              Inactive Tunnels
            </div>
          </div>
          <BaseIcon
            :path="mdiStop"
            size="48"
            class="text-gray-500 opacity-20"
          />
        </div>
      </CardBox>
    </div>

    <CardBox v-if="login">
      <SectionTitleLineWithButton
        :icon="mdiConnection"
        title="Tunnel List"
        main
      >
        <div class="flex flex-col gap-3 md:flex-row md:items-center">
          <div class="w-full md:w-64">
            <FormControl
              v-model="searchQuery"
              :icon="mdiMagnify"
              placeholder="Search tunnels"
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
            label="Add Domain"
            :icon="mdiPlus"
            color="success"
            class="shrink-0"
            @click="isAddModalActive = true"
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

      <div
        class="mb-4 px-4 py-2 rounded-lg bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 text-sm text-amber-800 dark:text-amber-200"
      >
        Domains not used for more than 1 month will be automatically deleted and their ports may be reassigned.
      </div>

      <div v-if="loading" class="text-center py-12">
        <div
          class="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-purple-600"
        />
        <p class="text-slate-500 dark:text-slate-400 mt-4">
          Loading tunnels...
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
              ? "No tunnels match your search."
              : "No tunnels defined yet."
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
                <h3 class="font-semibold text-lg flex items-center">
                  <BaseIcon
                    :path="mdiEarth"
                    size="20"
                    class="mr-2 text-blue-500"
                  />
                  {{ tunnel.domain }}
                </h3>
                <div
                  class="flex flex-wrap items-center gap-x-4 gap-y-2 text-sm text-slate-500 dark:text-slate-400"
                >
                  <div class="flex items-center gap-2 flex-wrap">
                    <span class="flex items-center">
                      <BaseIcon :path="mdiEthernet" size="16" class="mr-1" />
                      Port: {{ tunnel.port }}
                    </span>
                    <span
                      v-if="['tcp', 'tcp+udp', 'all'].includes(tunnel.protocol)"
                      class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-300"
                    >
                      TCP
                    </span>
                    <span
                      v-if="['udp', 'tcp+udp', 'all'].includes(tunnel.protocol)"
                      class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-cyan-100 text-cyan-800 dark:bg-cyan-900/40 dark:text-cyan-300"
                    >
                      UDP
                    </span>
                    <span
                      v-if="['http', 'https', 'all'].includes(tunnel.protocol)"
                      class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-emerald-100 text-emerald-800 dark:bg-emerald-900/40 dark:text-emerald-300"
                    >
                      HTTP
                    </span>
                    <span
                      v-if="['https', 'all'].includes(tunnel.protocol)"
                      class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-blue-100 text-blue-800 dark:bg-blue-900/40 dark:text-blue-300"
                    >
                      HTTPS
                    </span>
                  </div>
                  <div class="flex items-center">
                    <BaseIcon
                      :path="
                        tunnel.keep_alive ? mdiCheckCircle : mdiCloseCircle
                      "
                      size="16"
                      class="mr-1"
                      :class="
                        tunnel.keep_alive ? 'text-green-500' : 'text-red-500'
                      "
                    />
                    Keep Alive: {{ tunnel.keep_alive ? "Yes" : "No" }}
                  </div>
                  <div>Updated: {{ formatDate(tunnel.UpdatedAt) }}</div>
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
              class="flex items-start lg:flex-none justify-start lg:justify-end"
            >
              <span
                :class="[
                  'inline-flex items-center px-3 py-1 rounded-full text-xs font-medium',
                  getStatusColor(tunnel.started)
                ]"
              >
                {{ tunnel.started ? "Active" : "Inactive" }}
              </span>
            </div>
          </div>
          <div class="mt-6 flex flex-wrap items-center justify-end gap-2">
            <BaseButton
              :icon="tunnel.started ? mdiStop : mdiPlay"
              :color="tunnel.started ? 'danger' : 'success'"
              small
              :title="tunnel.started ? 'Stop' : 'Start'"
              @click="tunnel.started ? stopModal(tunnel) : startModal(tunnel)"
            />
            <BaseButton
              :icon="mdiAutorenew"
              color="info"
              small
              title="Yenile"
              @click="renewTunnel(tunnel)"
            />
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

  <!-- Add Domain Modal (random subdomain generated by server) -->
  <CardBoxModal
    v-model="isAddModalActive"
    title="Add Domain"
    button="success"
    :button-label="addLoading ? 'Adding...' : 'Add'"
    :button-disabled="addLoading"
    :cancel-disabled="addLoading"
    has-cancel
    @confirm="addSubmit"
  >
    <div class="add-domain-modal">
      <div class="add-domain-header">
        <div class="add-domain-header-icon">
          <BaseIcon :path="mdiPlus" size="24" />
        </div>
        <div class="add-domain-header-text">
          <span class="add-domain-title">New tunnel domain</span>
          <span class="add-domain-subtitle">A random subdomain will be generated automatically</span>
        </div>
      </div>
      <div class="add-domain-body">
        <p class="add-domain-desc">
          The full domain (e.g. <code>a1b2c3d4.tnpx.org</code>) will appear in your list after creation. HTTP, HTTPS, TCP and UDP will all be available.
        </p>
        <div v-if="addLoading" class="add-domain-loading">
          <div class="add-domain-spinner" />
          <span>Creating domain...</span>
        </div>
      </div>
    </div>
  </CardBoxModal>

  <!-- External server login/register (federation – OAuth2, e.g. api.tnpx.org) -->
  <CardBoxModal
    v-model="isExternalLoginModalActive"
    :title="externalAuthMode === 'register' ? 'Register on tunnel server' : 'Sign in to tunnel server'"
    button="info"
    :button-label="externalLoginLoading ? (externalAuthMode === 'register' ? 'Registering...' : 'Signing in...') : (externalAuthMode === 'register' ? 'Register' : 'Sign in')"
    :button-disabled="externalLoginLoading || !credentials.email || !credentials.password"
    :cancel-disabled="externalLoginLoading"
    has-cancel
    @confirm="externalLoginSubmit"
  >
    <p class="text-slate-600 dark:text-slate-400 mb-4">
      Sign in or register with OAuth2 for {{ selectedServer?.name }} ({{ selectedServer?.base_url }}).
    </p>
    <div class="flex gap-2 mb-4">
      <button
        type="button"
        :class="externalAuthMode === 'login' ? 'bg-indigo-600 text-white' : 'bg-slate-200 dark:bg-slate-700 text-slate-600 dark:text-slate-400'"
        class="px-3 py-2 rounded-lg text-sm font-medium"
        @click="externalAuthMode = 'login'"
      >
        Sign in
      </button>
      <button
        type="button"
        :class="externalAuthMode === 'register' ? 'bg-indigo-600 text-white' : 'bg-slate-200 dark:bg-slate-700 text-slate-600 dark:text-slate-400'"
        class="px-3 py-2 rounded-lg text-sm font-medium"
        @click="externalAuthMode = 'register'"
      >
        Register
      </button>
    </div>
    <FormField label="E-posta">
      <FormControl
        v-model="credentials.email"
        type="email"
        placeholder="email@example.com"
      />
    </FormField>
    <FormField label="Password" class="mt-4">
      <FormControl
        v-model="credentials.password"
        type="password"
        placeholder="••••••••"
      />
    </FormField>
  </CardBoxModal>

  <!-- Add Tunnel Server Modal (Federation) -->
  <CardBoxModal
    v-model="isAddServerModalActive"
    title="Add another server"
    button="info"
    button-label="Add"
    has-cancel
    @confirm="addServerSubmit"
  >
    <form class="space-y-6">
      <FormField label="Name" help="Display name for the server (e.g. External Redock)">
        <FormControl
          v-model="addServerForm.name"
          placeholder="Tunnel Server"
        />
      </FormField>
      <FormField label="Base URL" help="Tunnel server API URL (e.g. https://tunnel.example.com)">
        <FormControl
          v-model="addServerForm.base_url"
          placeholder="https://..."
        />
      </FormField>
    </form>
  </CardBoxModal>

  <!-- Start Tunnel Modal -->
  <CardBoxModal
    v-model="isStartModalActive"
    title="Start Tunnel"
    button="success"
    button-label="Start"
    has-cancel
    @confirm="startSubmit"
  >
    <form class="start-tunnel-form">
      <!-- Domain header -->
      <div class="start-tunnel-header">
        <div class="start-tunnel-header-icon">
          <BaseIcon :path="mdiTunnel" size="24" />
        </div>
        <div class="start-tunnel-header-text">
          <span class="start-tunnel-domain">{{ startDomain.domain }}</span>
          <span class="start-tunnel-subtitle">Configure where to forward traffic</span>
        </div>
      </div>

      <!-- HTTP forwarding -->
      <div class="start-tunnel-section">
        <h5 class="start-tunnel-section-title">HTTP / HTTPS (optional)</h5>
        <p class="text-sm text-slate-500 dark:text-slate-400 mb-2">Web traffic (HTTP/HTTPS) will be forwarded to this address.</p>
        <div class="start-tunnel-row">
          <FormField label="Target IP" help="e.g. 127.0.0.1 or 192.168.1.100">
            <FormControl
              v-model="start.localHttpIp"
              placeholder="127.0.0.1"
            />
          </FormField>
          <FormField label="Port">
            <FormControl
              v-model="start.localHttpPort"
              type="number"
              placeholder="80"
              class="w-full"
            />
          </FormField>
        </div>
      </div>

      <!-- TCP forwarding -->
      <div class="start-tunnel-section">
        <h5 class="start-tunnel-section-title">TCP (optional)</h5>
        <p class="text-sm text-slate-500 dark:text-slate-400 mb-2">Raw TCP connections will be forwarded to this address.</p>
        <div class="start-tunnel-row">
          <FormField label="Target IP">
            <FormControl
              v-model="start.localTcpIp"
              placeholder="127.0.0.1"
            />
          </FormField>
          <FormField label="Port">
            <FormControl
              v-model="start.localTcpPort"
              type="number"
              placeholder="9000"
              class="w-full"
            />
          </FormField>
        </div>
      </div>

      <!-- UDP forwarding -->
      <div class="start-tunnel-section">
        <h5 class="start-tunnel-section-title">UDP (optional)</h5>
        <p class="text-sm text-slate-500 dark:text-slate-400 mb-2">UDP packets will be forwarded to this address.</p>
        <div class="start-tunnel-row">
          <FormField label="Target IP">
            <FormControl
              v-model="start.localUdpIp"
              placeholder="127.0.0.1"
            />
          </FormField>
          <FormField label="Port">
            <FormControl
              v-model="start.localUdpPort"
              type="number"
              placeholder="53"
              class="w-full"
            />
          </FormField>
        </div>
      </div>

      <!-- Advanced -->
      <div class="start-tunnel-section start-tunnel-section-advanced">
        <h5 class="start-tunnel-section-title">Advanced</h5>
        <FormField
          label="Source bind IP"
          help="Local IP to use when connecting to target (leave empty for default)"
        >
          <FormControl
            v-model="start.sourceBindIp"
            placeholder="e.g. 10.0.0.2"
          />
        </FormField>
        <FormField
          v-if="['http', 'https', 'all'].includes(startDomain.protocol)"
          label="Host rewrite"
          help="Override Host header (e.g. backend.example.com). Empty = no override."
        >
          <FormControl
            v-model="start.hostRewrite"
            placeholder="backend.example.com"
          />
        </FormField>
      </div>
    </form>
  </CardBoxModal>

  <!-- Delete Confirmation Modal -->
  <CardBoxModal
    v-model="isDeleteModalActive"
    title="Delete Tunnel"
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
        This tunnel will be permanently deleted and active connections will be closed.
      </p>
      <div
        class="bg-yellow-50 dark:bg-yellow-900/20 p-4 rounded-lg"
      >
        <div class="flex items-start">
          <BaseIcon
            :path="mdiDelete"
            size="20"
            class="text-yellow-600 dark:text-yellow-400 mt-0.5 mr-2 flex-shrink-0"
          />
          <p class="text-sm text-yellow-800 dark:text-yellow-200">
            <strong>Warning:</strong> This action cannot be undone.
          </p>
        </div>
      </div>
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

/* Start Tunnel modal */
.start-tunnel-form {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}
.start-tunnel-form :deep(.mb-6) {
  margin-bottom: 0.5rem;
}
.start-tunnel-form :deep(.mb-2) {
  margin-bottom: 0.375rem;
}
.start-tunnel-form :deep(.mt-1) {
  margin-top: 0.25rem;
}
.start-tunnel-header {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.75rem 1rem;
  background: linear-gradient(135deg, rgb(59 130 246 / 0.08) 0%, rgb(99 102 241 / 0.06) 100%);
  border: 1px solid rgb(59 130 246 / 0.2);
  border-radius: 0.75rem;
}
.dark .start-tunnel-header {
  background: linear-gradient(135deg, rgb(59 130 246 / 0.12) 0%, rgb(99 102 241 / 0.08) 100%);
  border-color: rgb(59 130 246 / 0.25);
}
.start-tunnel-header-icon {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 2.75rem;
  height: 2.75rem;
  background: rgb(59 130 246 / 0.15);
  border-radius: 0.5rem;
  color: rgb(37 99 235);
}
.dark .start-tunnel-header-icon {
  background: rgb(59 130 246 / 0.2);
  color: rgb(96 165 250);
}
.start-tunnel-header-text {
  display: flex;
  flex-direction: column;
  gap: 0.125rem;
  min-width: 0;
}
.start-tunnel-domain {
  font-weight: 600;
  font-size: 1.0625rem;
  color: rgb(30 58 138);
  letter-spacing: -0.01em;
}
.dark .start-tunnel-domain {
  color: rgb(191 219 254);
}
.start-tunnel-subtitle {
  font-size: 0.8125rem;
  color: rgb(71 85 105);
}
.dark .start-tunnel-subtitle {
  color: rgb(148 163 184);
}
.start-tunnel-section {
  padding: 0.6rem 0;
  border-bottom: 1px solid rgb(226 232 240);
}
.dark .start-tunnel-section {
  border-bottom-color: rgb(51 65 85);
}
.start-tunnel-section:last-child {
  border-bottom: none;
  padding-bottom: 0;
}
.start-tunnel-section-advanced {
  padding-top: 0.5rem;
}
.start-tunnel-section-title {
  margin: 0 0 0.5rem 0;
  font-size: 0.6875rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: rgb(100 116 139);
}
.dark .start-tunnel-section-title {
  color: rgb(148 163 184);
}
.start-tunnel-row {
  display: grid;
  grid-template-columns: 1fr 6rem;
  gap: 0.75rem;
  align-items: start;
}
@media (max-width: 420px) {
  .start-tunnel-row {
    grid-template-columns: 1fr;
  }
}

/* Add Domain modal */
.add-domain-modal {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}
.add-domain-header {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.75rem 1rem;
  background: linear-gradient(135deg, rgb(34 197 94 / 0.1) 0%, rgb(22 163 74 / 0.06) 100%);
  border: 1px solid rgb(34 197 94 / 0.25);
  border-radius: 0.75rem;
}
.dark .add-domain-header {
  background: linear-gradient(135deg, rgb(34 197 94 / 0.14) 0%, rgb(22 163 74 / 0.08) 100%);
  border-color: rgb(34 197 94 / 0.3);
}
.add-domain-header-icon {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 2.75rem;
  height: 2.75rem;
  background: rgb(34 197 94 / 0.2);
  border-radius: 0.5rem;
  color: rgb(22 163 74);
}
.dark .add-domain-header-icon {
  background: rgb(34 197 94 / 0.25);
  color: rgb(74 222 128);
}
.add-domain-header-text {
  display: flex;
  flex-direction: column;
  gap: 0.125rem;
  min-width: 0;
}
.add-domain-title {
  font-weight: 600;
  font-size: 1rem;
  color: rgb(21 128 61);
  letter-spacing: -0.01em;
}
.dark .add-domain-title {
  color: rgb(134 239 172);
}
.add-domain-subtitle {
  font-size: 0.8125rem;
  color: rgb(71 85 105);
}
.dark .add-domain-subtitle {
  color: rgb(148 163 184);
}
.add-domain-body {
  padding: 0.25rem 0;
}
.add-domain-desc {
  margin: 0;
  font-size: 0.875rem;
  line-height: 1.5;
  color: rgb(71 85 105);
}
.dark .add-domain-desc {
  color: rgb(148 163 184);
}
.add-domain-desc code {
  font-size: 0.8125rem;
  padding: 0.125rem 0.375rem;
  background: rgb(226 232 240);
  border-radius: 0.25rem;
  color: rgb(51 65 85);
}
.dark .add-domain-desc code {
  background: rgb(51 65 85);
  color: rgb(203 213 225);
}
.add-domain-loading {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin-top: 1rem;
  padding: 0.75rem 1rem;
  background: rgb(254 252 232 / 0.8);
  border: 1px solid rgb(253 224 71 / 0.3);
  border-radius: 0.5rem;
  font-size: 0.875rem;
  color: rgb(161 98 7);
}
.dark .add-domain-loading {
  background: rgb(253 224 71 / 0.08);
  border-color: rgb(253 224 71 / 0.2);
  color: rgb(253 224 71);
}
.add-domain-spinner {
  width: 1rem;
  height: 1rem;
  border: 2px solid currentColor;
  border-right-color: transparent;
  border-radius: 50%;
  animation: spin 0.6s linear infinite;
}
</style>
