<script setup>
import BaseButton from "@/components/BaseButton.vue";
import BaseIcon from "@/components/BaseIcon.vue";
import CardBox from "@/components/CardBox.vue";
import FormControl from "@/components/FormControl.vue";
import FormField from "@/components/FormField.vue";
import ApiService from "@/services/ApiService";
import { mdiContentCopy, mdiEthernet, mdiRefresh, mdiPlus, mdiMinus } from "@mdi/js";
import { onMounted, ref, computed } from "vue";
import { useToast } from "vue-toastification";

const toast = useToast();
const interfaces = ref([]);
const addresses = ref([]);
const selectedInterface = ref("");
const cidrOrRange = ref("");
const gatewayIp = ref("");
const loading = ref(false);
const loadingAddresses = ref(false);
const addRemoveLoading = ref(false);

const canSubmit = computed(() => selectedInterface.value?.trim() && cidrOrRange.value?.trim());

const selectedInterfaceInfo = computed(() =>
  interfaces.value.find((i) => i.name === selectedInterface.value) || null
);

const fetchInterfaces = async () => {
  loading.value = true;
  try {
    const res = await ApiService.getNetworkInterfaces();
    interfaces.value = res?.data?.data ?? [];
    if (interfaces.value.length && !selectedInterface.value) {
      selectedInterface.value = interfaces.value[0].name;
    }
    if (selectedInterface.value) {
      await fetchAddresses();
    }
  } catch (e) {
    toast.error("Failed to load interfaces: " + (e.response?.data?.msg || e.message));
  } finally {
    loading.value = false;
  }
};

const fetchAddresses = async () => {
  if (!selectedInterface.value) return;
  loadingAddresses.value = true;
  try {
    const res = await ApiService.getNetworkAddresses(selectedInterface.value);
    addresses.value = res?.data?.data ?? [];
  } catch (e) {
    addresses.value = [];
  } finally {
    loadingAddresses.value = false;
  }
};

const fetchClientCommand = async () => {
  try {
    const res = await ApiService.getNetworkClientCommand();
    gatewayIp.value = res?.data?.data?.gateway_ip ?? "";
  } catch {
    gatewayIp.value = "";
  }
};

const addAlias = async () => {
  if (!canSubmit.value) return;
  addRemoveLoading.value = true;
  try {
    await ApiService.addNetworkAlias({
      interface: selectedInterface.value,
      cidr_or_range: cidrOrRange.value.trim()
    });
    toast.success("IP addresses added.");
    cidrOrRange.value = "";
    await fetchAddresses();
  } catch (e) {
    toast.error(e.response?.data?.msg || e.message || "Failed to add.");
  } finally {
    addRemoveLoading.value = false;
  }
};

const removeAlias = async () => {
  if (!canSubmit.value) return;
  addRemoveLoading.value = true;
  try {
    await ApiService.removeNetworkAlias({
      interface: selectedInterface.value,
      cidr_or_range: cidrOrRange.value.trim()
    });
    toast.success("IP addresses removed.");
    cidrOrRange.value = "";
    await fetchAddresses();
  } catch (e) {
    toast.error(e.response?.data?.msg || e.message || "Failed to remove.");
  } finally {
    addRemoveLoading.value = false;
  }
};

/** Returns the route command to run on the client for the given address (IP, CIDR or range). */
function routeCommandForAddress(addr) {
  if (!addr || !gatewayIp.value) return "";
  const ip = String(addr).trim();
  let net = ip;
  if (ip.includes("-")) {
    net = ip.split("-")[0].trim();
  } else if (ip.includes("/")) {
    const [address, prefix] = ip.split("/");
    if (prefix === "32") net = address.trim();
    else net = ip;
  }
  return `sudo route -n add -net ${net} ${gatewayIp.value}`;
}

const copyCommand = (cmd) => {
  if (!cmd) return;
  navigator.clipboard.writeText(cmd).then(() => toast.success("Command copied.")).catch(() => toast.error("Copy failed."));
};

onMounted(() => {
  fetchInterfaces();
  fetchClientCommand();
});
</script>

<template>
  <div class="space-y-6">
    <!-- Header -->
    <div class="bg-gradient-to-r from-cyan-600 to-blue-600 rounded-xl p-6 text-white">
      <div class="flex flex-col lg:flex-row lg:items-center lg:justify-between">
        <div class="flex items-center space-x-4">
          <div class="w-12 h-12 bg-white/20 rounded-xl flex items-center justify-center backdrop-blur-sm">
            <BaseIcon :path="mdiEthernet" size="24" class="text-white" />
          </div>
          <div>
            <h1 class="text-2xl lg:text-3xl font-bold mb-2">IP Alias</h1>
            <p class="text-cyan-100">Add and manage IP addresses on network interfaces</p>
          </div>
        </div>
        <div class="flex space-x-3 mt-4 lg:mt-0">
          <BaseButton
            :icon="mdiRefresh"
            label="Refresh"
            color="lightDark"
            :loading="loading"
            @click="fetchInterfaces"
          />
        </div>
      </div>
    </div>

  <CardBox>
    <p class="text-slate-600 dark:text-slate-400 text-sm mb-4">
      Add an IP address or range to the interface. The kernel will accept traffic to these addresses.
      The <strong>route</strong> command to run on the client for each IP is listed below.
    </p>
    <p class="text-sm text-slate-500 dark:text-slate-400 mb-6">
      IP alias is only supported on Linux.
    </p>

    <div class="grid grid-cols-1 md:grid-cols-2 gap-6 mb-6">
      <FormField label="Interface">
        <select
          v-model="selectedInterface"
          class="w-full rounded-lg border border-slate-200 dark:border-slate-600 bg-white dark:bg-slate-800 px-3 py-2 text-sm"
          @change="fetchAddresses"
        >
          <option value="">Select</option>
          <option v-for="iface in interfaces" :key="iface.name" :value="iface.name">
            {{ iface.name }}{{ iface.up ? " (up)" : "" }}{{ iface.ips?.length ? " — " + iface.ips.join(", ") : "" }}
          </option>
        </select>
      </FormField>
      <FormField label="IP or range (CIDR or start-end)">
        <FormControl
          v-model="cidrOrRange"
          placeholder="88.255.136.0/24 or 88.255.136.1-88.255.136.254"
        />
      </FormField>
    </div>

    <div
      v-if="selectedInterfaceInfo"
      class="mb-6 p-4 rounded-lg bg-slate-50 dark:bg-slate-800/50 border border-slate-200 dark:border-slate-600"
    >
      <p class="text-sm font-medium text-slate-700 dark:text-slate-300 mb-3">Interface: {{ selectedInterfaceInfo.name }}</p>
      <dl class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-x-4 gap-y-2 text-sm">
        <dt class="text-slate-500 dark:text-slate-400">Status</dt>
        <dd class="text-slate-800 dark:text-slate-200">{{ selectedInterfaceInfo.up ? "Up" : "Down" }}</dd>
        <dt class="text-slate-500 dark:text-slate-400">MAC</dt>
        <dd class="text-slate-800 dark:text-slate-200 font-mono">{{ selectedInterfaceInfo.mac || "—" }}</dd>
        <dt class="text-slate-500 dark:text-slate-400">MTU</dt>
        <dd class="text-slate-800 dark:text-slate-200">{{ selectedInterfaceInfo.mtu ?? "—" }}</dd>
        <dt class="text-slate-500 dark:text-slate-400">Gateway</dt>
        <dd class="text-slate-800 dark:text-slate-200 font-mono">{{ selectedInterfaceInfo.gateway || "—" }}</dd>
        <dt class="text-slate-500 dark:text-slate-400 sm:col-span-2 lg:col-span-4">IP addresses</dt>
        <dd class="text-slate-800 dark:text-slate-200 font-mono sm:col-span-2 lg:col-span-4">
          {{ (selectedInterfaceInfo.ips && selectedInterfaceInfo.ips.length) ? selectedInterfaceInfo.ips.join(", ") : "—" }}
        </dd>
      </dl>
    </div>

    <div class="flex flex-wrap gap-3 mb-6">
      <BaseButton
        :icon="mdiPlus"
        label="Add"
        color="info"
        :disabled="!canSubmit || addRemoveLoading"
        :loading="addRemoveLoading"
        @click="addAlias"
      />
      <BaseButton
        :icon="mdiMinus"
        label="Remove"
        color="danger"
        :disabled="!canSubmit || addRemoveLoading"
        :loading="addRemoveLoading"
        @click="removeAlias"
      />
      <BaseButton
        v-if="selectedInterface"
        :icon="mdiRefresh"
        label="Refresh addresses"
        outline
        :loading="loadingAddresses"
        @click="fetchAddresses"
      />
    </div>

    <FormField v-if="selectedInterface" label="Addresses on this interface and client commands">
      <div v-if="loadingAddresses" class="rounded-lg bg-slate-100 dark:bg-slate-700/50 p-4 text-sm text-slate-500 dark:text-slate-400">
        Loading...
      </div>
      <div v-else-if="addresses.length === 0" class="rounded-lg bg-slate-100 dark:bg-slate-700/50 p-4 text-sm text-slate-500 dark:text-slate-400">
        No addresses yet or list not loaded. Click "Refresh addresses" to load.
      </div>
      <div v-else class="rounded-lg border border-slate-200 dark:border-slate-600 overflow-hidden">
        <div v-if="!gatewayIp" class="p-3 bg-amber-50 dark:bg-amber-900/20 border-b border-slate-200 dark:border-slate-600 text-sm text-amber-800 dark:text-amber-200">
          Gateway not available; commands cannot be generated.
        </div>
        <ul class="divide-y divide-slate-200 dark:divide-slate-600 max-h-96 overflow-y-auto">
          <li
            v-for="addr in addresses"
            :key="addr"
            class="flex flex-col sm:flex-row sm:items-center gap-2 sm:gap-3 p-3 bg-slate-50/50 dark:bg-slate-800/30 hover:bg-slate-100/50 dark:hover:bg-slate-800/50"
          >
            <span class="font-mono text-sm text-slate-700 dark:text-slate-300 shrink-0">{{ addr }}</span>
            <code class="flex-1 min-w-0 text-sm bg-slate-200 dark:bg-slate-700 px-2 py-1.5 rounded break-all font-mono">
              {{ routeCommandForAddress(addr) || "—" }}
            </code>
            <BaseButton
              :icon="mdiContentCopy"
              label="Copy"
              small
              outline
              :disabled="!routeCommandForAddress(addr)"
              @click="copyCommand(routeCommandForAddress(addr))"
            />
          </li>
        </ul>
        <p v-if="gatewayIp && addresses.length" class="text-xs text-slate-500 dark:text-slate-400 px-3 py-2 border-t border-slate-200 dark:border-slate-600">
          Gateway: {{ gatewayIp }} (client route forwards to this address)
        </p>
      </div>
    </FormField>

  </CardBox>
  </div>
</template>
