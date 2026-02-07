<script setup>
import BaseButton from "@/components/BaseButton.vue";
import BaseIcon from "@/components/BaseIcon.vue";
import CardBox from "@/components/CardBox.vue";
import FormControl from "@/components/FormControl.vue";
import FormField from "@/components/FormField.vue";
import SectionTitleLineWithButton from "@/components/SectionTitleLineWithButton.vue";
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
  } catch (e) {
    toast.error("Arayüz listesi alınamadı: " + (e.response?.data?.msg || e.message));
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
    toast.success("IP adresleri eklendi.");
    cidrOrRange.value = "";
    await fetchAddresses();
  } catch (e) {
    toast.error(e.response?.data?.msg || e.message || "Ekleme başarısız.");
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
    toast.success("IP adresleri kaldırıldı.");
    cidrOrRange.value = "";
    await fetchAddresses();
  } catch (e) {
    toast.error(e.response?.data?.msg || e.message || "Kaldırma başarısız.");
  } finally {
    addRemoveLoading.value = false;
  }
};

const clientRouteCommand = computed(() => {
  const ip = cidrOrRange.value.trim();
  if (!ip || !gatewayIp.value) return "";
  // Tek IP için /32 yazmıyoruz (macOS/Linux route formatı: 88.255.136.70 gateway)
  let net = ip;
  if (ip.includes("-")) {
    net = ip.split("-")[0].trim();
  } else if (ip.includes("/")) {
    const [addr, prefix] = ip.split("/");
    if (prefix === "32") net = addr.trim();
    else net = ip;
  }
  return `sudo route -n add -net ${net} ${gatewayIp.value}`;
});

const copyCommand = () => {
  const cmd = clientRouteCommand.value;
  if (!cmd) {
    toast.info("Önce bir IP veya aralık girin.");
    return;
  }
  navigator.clipboard.writeText(cmd).then(() => toast.success("Komut kopyalandı.")).catch(() => toast.error("Kopyalama başarısız."));
};

onMounted(() => {
  fetchInterfaces();
  fetchClientCommand();
});
</script>

<template>
  <SectionTitleLineWithButton :icon="mdiEthernet" title="IP Alias" main>
    <BaseButton
      :icon="mdiRefresh"
      label="Yenile"
      :loading="loading"
      @click="fetchInterfaces"
    />
  </SectionTitleLineWithButton>

  <CardBox>
    <p class="text-slate-600 dark:text-slate-400 text-sm mb-6">
      Arayüze IP adresi veya aralığı ekleyin. Kernel bu adreslere gelen trafiği kabul eder.
      İstemci tarafında trafiği bu sunucuya yönlendirmek için aşağıdaki <strong>route</strong> komutunu kullanın.
    </p>

    <div class="grid grid-cols-1 md:grid-cols-2 gap-6 mb-6">
      <FormField label="Arayüz">
        <select
          v-model="selectedInterface"
          class="w-full rounded-lg border border-slate-200 dark:border-slate-600 bg-white dark:bg-slate-800 px-3 py-2 text-sm"
          @change="fetchAddresses"
        >
          <option value="">Seçin</option>
          <option v-for="iface in interfaces" :key="iface.name" :value="iface.name">
            {{ iface.name }}{{ iface.up ? " (up)" : "" }}{{ iface.ips?.length ? " — " + iface.ips.join(", ") : "" }}
          </option>
        </select>
      </FormField>
      <FormField label="IP veya aralık (CIDR veya başlangıç-bitiş)">
        <FormControl
          v-model="cidrOrRange"
          placeholder="88.255.136.0/24 veya 88.255.136.1-88.255.136.254"
        />
      </FormField>
    </div>

    <div
      v-if="selectedInterfaceInfo"
      class="mb-6 p-4 rounded-lg bg-slate-50 dark:bg-slate-800/50 border border-slate-200 dark:border-slate-600"
    >
      <p class="text-sm font-medium text-slate-700 dark:text-slate-300 mb-3">Arayüz özellikleri: {{ selectedInterfaceInfo.name }}</p>
      <dl class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-x-4 gap-y-2 text-sm">
        <dt class="text-slate-500 dark:text-slate-400">Durum</dt>
        <dd class="text-slate-800 dark:text-slate-200">{{ selectedInterfaceInfo.up ? "Up" : "Down" }}</dd>
        <dt class="text-slate-500 dark:text-slate-400">MAC</dt>
        <dd class="text-slate-800 dark:text-slate-200 font-mono">{{ selectedInterfaceInfo.mac || "—" }}</dd>
        <dt class="text-slate-500 dark:text-slate-400">MTU</dt>
        <dd class="text-slate-800 dark:text-slate-200">{{ selectedInterfaceInfo.mtu ?? "—" }}</dd>
        <dt class="text-slate-500 dark:text-slate-400">Gateway</dt>
        <dd class="text-slate-800 dark:text-slate-200 font-mono">{{ selectedInterfaceInfo.gateway || "—" }}</dd>
        <dt class="text-slate-500 dark:text-slate-400 sm:col-span-2 lg:col-span-4">IP adresleri</dt>
        <dd class="text-slate-800 dark:text-slate-200 font-mono sm:col-span-2 lg:col-span-4">
          {{ (selectedInterfaceInfo.ips && selectedInterfaceInfo.ips.length) ? selectedInterfaceInfo.ips.join(", ") : "—" }}
        </dd>
      </dl>
    </div>

    <div class="flex flex-wrap gap-3 mb-6">
      <BaseButton
        :icon="mdiPlus"
        label="Ekle"
        color="info"
        :disabled="!canSubmit || addRemoveLoading"
        :loading="addRemoveLoading"
        @click="addAlias"
      />
      <BaseButton
        :icon="mdiMinus"
        label="Kaldır"
        color="danger"
        :disabled="!canSubmit || addRemoveLoading"
        :loading="addRemoveLoading"
        @click="removeAlias"
      />
      <BaseButton
        v-if="selectedInterface"
        :icon="mdiRefresh"
        label="Adresleri listele"
        outline
        :loading="loadingAddresses"
        @click="fetchAddresses"
      />
    </div>

    <FormField v-if="selectedInterface" label="Bu arayüzdeki adresler">
      <div class="rounded-lg bg-slate-100 dark:bg-slate-700/50 p-3 text-sm max-h-32 overflow-y-auto">
        <span v-if="loadingAddresses">Yükleniyor...</span>
        <span v-else-if="addresses.length === 0">Henüz liste alınmadı veya adres yok. "Adresleri listele" ile yenileyin.</span>
        <span v-else>{{ addresses.join(", ") }}</span>
      </div>
    </FormField>

    <div class="mt-6 p-4 rounded-lg bg-slate-100 dark:bg-slate-800 border border-slate-200 dark:border-slate-600">
      <p class="text-sm font-medium text-slate-700 dark:text-slate-300 mb-2">İstemcide çalıştır (trafiği bu sunucuya yönlendirmek için)</p>
      <div class="flex items-center gap-2 flex-wrap">
        <code class="flex-1 min-w-0 text-sm bg-slate-200 dark:bg-slate-700 px-2 py-1 rounded break-all">
          {{ clientRouteCommand || "IP/aralık girin, komut burada görünecek" }}
        </code>
        <BaseButton
          :icon="mdiContentCopy"
          label="Kopyala"
          small
          outline
          :disabled="!clientRouteCommand"
          @click="copyCommand"
        />
      </div>
      <p class="text-xs text-slate-500 dark:text-slate-400 mt-2">
        Redock sunucu IP’si: {{ gatewayIp || "—" }} (istemcide route ile bu adres kullanılır)
      </p>
    </div>
  </CardBox>
</template>
