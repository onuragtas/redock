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
  mdiCloud,
  mdiPlus,
  mdiRefresh,
  mdiDelete,
  mdiDomain,
  mdiDns,
  mdiSync,
  mdiCheck,
  mdiAlert,
  mdiPencil
} from '@mdi/js';
import { computed, onMounted, ref } from "vue";
import { useToast } from 'vue-toastification';

const toast = useToast();

// State
const accounts = ref([]);
const zones = ref([]);
const dnsRecords = ref([]);
const loading = ref(false);
const activeTab = ref('accounts');

// Modals
const isAddAccountModalActive = ref(false);
const isAddZoneModalActive = ref(false);
const isRecordModalActive = ref(false);
const isDeleteRecordModalActive = ref(false);
const recordModalMode = ref('create'); // 'create' | 'edit'
const recordSaving = ref(false);
const selectedAccount = ref(null);
const selectedZone = ref(null);
const recordToDelete = ref(null);

// Forms
const newAccount = ref({
  name: '',
  email: '',
  api_token: ''
});

const newZone = ref({
  account_id: '',
  domain: ''
});

const RECORD_TYPES = ['A', 'AAAA', 'CNAME', 'MX', 'TXT', 'NS', 'SRV', 'CAA'];
const PROXYABLE_TYPES = ['A', 'AAAA', 'CNAME'];

const blankRecord = () => ({
  id: null,
  record_id: '',
  type: 'A',
  name: '',
  content: '',
  ttl: 1,
  priority: 10,
  proxied: false,
  comment: ''
});

const recordForm = ref(blankRecord());

const typeSupportsProxy = computed(() => PROXYABLE_TYPES.includes(recordForm.value.type));
const typeNeedsPriority = computed(() => recordForm.value.type === 'MX' || recordForm.value.type === 'SRV');

const ttlOptions = [
  { id: 1, label: 'Auto' },
  { id: 60, label: '1 minute' },
  { id: 300, label: '5 minutes' },
  { id: 1800, label: '30 minutes' },
  { id: 3600, label: '1 hour' },
  { id: 86400, label: '1 day' }
];

const ttlLabel = (ttl) => {
  const opt = ttlOptions.find(o => o.id === ttl);
  return opt ? opt.label : `${ttl}s`;
};

// Tabs
const tabs = ['accounts', 'zones', 'dns'];

onMounted(() => {
  loadAccounts();
});

const loadAccounts = async () => {
  try {
    const response = await ApiService.get('/api/cloudflare/accounts');
    if (!response.data.error) {
      accounts.value = response.data.data || [];
    }
  } catch (error) {
    console.error('Failed to load accounts:', error);
    toast.error('Failed to load Cloudflare accounts');
  }
};

const loadZones = async (accountId = null) => {
  try {
    const url = accountId 
      ? `/api/cloudflare/accounts/${accountId}/zones`
      : '/api/cloudflare/zones';
    const response = await ApiService.get(url);
    if (!response.data.error) {
      zones.value = response.data.data || [];
    }
  } catch (error) {
    console.error('Failed to load zones:', error);
    toast.error('Failed to load zones');
  }
};

const loadDNSRecords = async (zoneId) => {
  if (!zoneId) return;
  
  loading.value = true;
  try {
    const response = await ApiService.get(`/api/cloudflare/zones/${zoneId}/dns`);
    if (!response.data.error) {
      dnsRecords.value = response.data.data || [];
    }
  } catch (error) {
    console.error('Failed to load DNS records:', error);
    toast.error('Failed to load DNS records');
  } finally {
    loading.value = false;
  }
};

const addAccount = async () => {
  try {
    const response = await ApiService.post('/api/cloudflare/accounts', newAccount.value);
    if (!response.data.error) {
      toast.success('✅ Account added successfully');
      await loadAccounts();
      isAddAccountModalActive.value = false;
      newAccount.value = { name: '', email: '', api_token: '' };
    } else {
      toast.error('❌ ' + response.data.msg);
    }
  } catch (error) {
    toast.error('❌ Error: ' + error.message);
  }
};

const deleteAccount = async (accountId) => {
  if (!confirm('Are you sure you want to delete this account?')) {
    return;
  }
  
  try {
    const response = await ApiService.delete(`/api/cloudflare/accounts/${accountId}`);
    if (!response.data.error) {
      toast.success('✅ Account deleted');
      await loadAccounts();
    } else {
      toast.error('❌ ' + response.data.msg);
    }
  } catch (error) {
    toast.error('❌ Error: ' + error.message);
  }
};

const syncZones = async (accountId) => {
  loading.value = true;
  try {
    const response = await ApiService.post(`/api/cloudflare/accounts/${accountId}/sync-zones`);
    if (!response.data.error) {
      toast.success(`✅ Synced ${response.data.data?.count || 0} zones`);
      await loadZones();
    } else {
      toast.error('❌ ' + response.data.msg);
    }
  } catch (error) {
    toast.error('❌ Error: ' + error.message);
  } finally {
    loading.value = false;
  }
};


const formatDate = (date) => {
  if (!date) return 'Never';
  return new Date(date).toLocaleString();
};

const openCreateRecord = () => {
  if (!selectedZone.value) return;
  recordModalMode.value = 'create';
  recordForm.value = blankRecord();
  isRecordModalActive.value = true;
};

const openEditRecord = (record) => {
  recordModalMode.value = 'edit';
  recordForm.value = {
    id: record.id,
    record_id: record.record_id,
    type: record.type,
    name: record.name,
    content: record.content,
    ttl: record.ttl || 1,
    priority: record.priority || 10,
    proxied: !!record.proxied,
    comment: record.comment || ''
  };
  isRecordModalActive.value = true;
};

const buildRecordPayload = () => {
  const f = recordForm.value;
  const payload = {
    type: f.type,
    name: f.name.trim(),
    content: f.content.trim(),
    ttl: Number(f.ttl) || 1,
    comment: f.comment || ''
  };
  if (PROXYABLE_TYPES.includes(f.type)) {
    payload.proxied = !!f.proxied;
  }
  if (f.type === 'MX' || f.type === 'SRV') {
    payload.priority = Number(f.priority) || 0;
  }
  return payload;
};

const saveRecord = async () => {
  if (!selectedZone.value) {
    toast.error('No zone selected');
    return;
  }
  const f = recordForm.value;
  if (!f.type || !f.name || !f.content) {
    toast.error('Type, name and content are required');
    return;
  }
  recordSaving.value = true;
  try {
    const payload = buildRecordPayload();
    const url = recordModalMode.value === 'edit'
      ? `/api/cloudflare/zones/${selectedZone.value.id}/dns/${f.record_id}`
      : `/api/cloudflare/zones/${selectedZone.value.id}/dns`;
    const response = recordModalMode.value === 'edit'
      ? await ApiService.put(url, payload)
      : await ApiService.post(url, payload);
    if (!response.data.error) {
      toast.success(recordModalMode.value === 'edit' ? '✅ Record updated' : '✅ Record created');
      isRecordModalActive.value = false;
      await loadDNSRecords(selectedZone.value.id);
    } else {
      toast.error('❌ ' + response.data.msg);
    }
  } catch (error) {
    toast.error('❌ Error: ' + (error.response?.data?.msg || error.message));
  } finally {
    recordSaving.value = false;
  }
};

const askDeleteRecord = (record) => {
  recordToDelete.value = record;
  isDeleteRecordModalActive.value = true;
};

const confirmDeleteRecord = async () => {
  if (!selectedZone.value || !recordToDelete.value) return;
  try {
    const response = await ApiService.delete(
      `/api/cloudflare/zones/${selectedZone.value.id}/dns/${recordToDelete.value.record_id}`
    );
    if (!response.data.error) {
      toast.success('✅ Record deleted');
      isDeleteRecordModalActive.value = false;
      recordToDelete.value = null;
      await loadDNSRecords(selectedZone.value.id);
    } else {
      toast.error('❌ ' + response.data.msg);
    }
  } catch (error) {
    toast.error('❌ Error: ' + (error.response?.data?.msg || error.message));
  }
};
</script>

<template>
  <div>
    <SectionTitleLineWithButton :icon="mdiCloud" title="Cloudflare Management" main />

    <!-- Tabs (responsive: horizontal scroll on small screens) -->
    <div class="mb-6 overflow-x-auto pb-px -mx-1 px-1">
      <div class="flex flex-nowrap gap-2 sm:gap-4 border-b border-gray-200 dark:border-gray-700">
        <button
          v-for="tab in tabs"
          :key="tab"
          :class="[
            'shrink-0 whitespace-nowrap px-4 py-2 font-medium transition-colors',
            activeTab === tab
              ? 'border-b-2 border-blue-500 text-blue-600 dark:text-blue-400'
              : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300'
          ]"
          @click="activeTab = tab; if(tab === 'zones') loadZones()"
        >
          {{ tab.charAt(0).toUpperCase() + tab.slice(1) }}
        </button>
      </div>
    </div>

    <!-- Accounts Tab -->
    <div v-if="activeTab === 'accounts'">
      <CardBox>
        <div class="flex items-center justify-between mb-6">
          <h3 class="text-xl font-semibold">Cloudflare Accounts</h3>
          <BaseButton
            :icon="mdiPlus"
            color="success"
            label="Add Account"
            @click="isAddAccountModalActive = true"
          />
        </div>

        <div v-if="accounts.length === 0" class="text-center py-12 text-gray-500">
          No Cloudflare accounts yet. Add your first account with an API token!
        </div>

        <div v-else class="space-y-4">
          <div
            v-for="account in accounts"
            :key="account.id"
            class="p-4 border border-gray-200 dark:border-gray-700 rounded-lg"
          >
            <div class="flex items-center justify-between">
              <div class="flex-1">
                <h4 class="text-lg font-semibold">{{ account.name }}</h4>
                <p class="text-sm text-gray-500">{{ account.email }}</p>
                <p class="text-xs text-gray-400 mt-1">Added: {{ formatDate(account.created_at) }}</p>
              </div>
              <div class="flex items-center gap-2">
                <BaseButton
                  :icon="mdiSync"
                  color="info"
                  label="Sync Zones"
                  small
                  @click="syncZones(account.id)"
                />
                <BaseButton
                  :icon="mdiDelete"
                  color="danger"
                  small
                  @click="deleteAccount(account.id)"
                />
              </div>
            </div>
          </div>
        </div>
      </CardBox>
    </div>

    <!-- Zones Tab -->
    <div v-if="activeTab === 'zones'">
      <CardBox>
        <div class="flex items-center justify-between mb-6">
          <h3 class="text-xl font-semibold">Cloudflare Zones</h3>
          <BaseButton
            :icon="mdiRefresh"
            color="info"
            label="Refresh"
            small
            @click="loadZones()"
          />
        </div>

        <div v-if="zones.length === 0" class="text-center py-12 text-gray-500">
          No zones found. Sync zones from your Cloudflare accounts first!
        </div>

        <div v-else class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          <div
            v-for="zone in zones"
            :key="zone.id"
            class="p-4 border border-gray-200 dark:border-gray-700 rounded-lg hover:border-blue-500 transition-colors"
          >
            <div class="flex items-start justify-between mb-3">
              <div>
                <h4 class="text-lg font-semibold">{{ zone.name }}</h4>
                <p class="text-sm text-gray-500">{{ zone.status }}</p>
              </div>
              <BaseIcon
                :path="zone.status === 'active' ? mdiCheck : mdiAlert"
                :class="zone.status === 'active' ? 'text-green-500' : 'text-yellow-500'"
                w="w-6"
                h="h-6"
              />
            </div>
            <BaseButton
              :icon="mdiDns"
              label="View DNS Records"
              color="info"
              small
              class="w-full"
              @click="activeTab = 'dns'; selectedZone = zone; loadDNSRecords(zone.id)"
            />
          </div>
        </div>
      </CardBox>
    </div>

    <!-- DNS Tab -->
    <div v-if="activeTab === 'dns'">
      <CardBox>
        <div class="mb-6">
          <div class="flex items-center justify-between mb-3">
            <div>
              <h3 class="text-xl font-semibold">DNS Records</h3>
              <p v-if="selectedZone" class="text-sm text-gray-500">{{ selectedZone.name }}</p>
            </div>
            <div class="flex items-center gap-2">
              <BaseButton
                :icon="mdiPlus"
                color="success"
                label="Add Record"
                small
                :disabled="!selectedZone"
                @click="openCreateRecord"
              />
              <BaseButton
                :icon="mdiRefresh"
                color="info"
                label="Refresh"
                small
                :disabled="!selectedZone"
                @click="loadDNSRecords(selectedZone?.id)"
              />
            </div>
          </div>
          <div class="bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg p-3">
            <p class="text-sm text-blue-800 dark:text-blue-200">
              <svg class="w-5 h-5 inline-block mr-2 -mt-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
              <strong>Email DNS Management:</strong> Email DNS records (SPF, DKIM, DMARC, MX) are automatically created when you add a domain in the Email Server section.
            </p>
          </div>
        </div>

        <div v-if="!selectedZone" class="text-center py-12 text-gray-500">
          Select a zone from the Zones tab to view DNS records
        </div>

        <div v-else-if="loading" class="text-center py-12">
          <div class="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600 mx-auto"></div>
        </div>

        <div v-else-if="dnsRecords.length === 0" class="text-center py-12 text-gray-500">
          No DNS records found for this zone
        </div>

        <div v-else class="overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
            <thead class="bg-gray-50 dark:bg-gray-800">
              <tr>
                <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Type</th>
                <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Name</th>
                <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Content</th>
                <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">TTL</th>
                <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Proxied</th>
                <th class="px-6 py-3 text-right text-xs font-medium text-gray-500 uppercase">Actions</th>
              </tr>
            </thead>
            <tbody class="bg-white dark:bg-gray-900 divide-y divide-gray-200 dark:divide-gray-700">
              <tr v-for="record in dnsRecords" :key="record.record_id || record.id">
                <td class="px-6 py-4 whitespace-nowrap">
                  <span class="px-2 py-1 text-xs font-semibold rounded bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200">
                    {{ record.type }}
                  </span>
                </td>
                <td class="px-6 py-4 text-sm">{{ record.name }}</td>
                <td class="px-6 py-4 text-sm font-mono text-gray-600 dark:text-gray-400 max-w-md truncate" :title="record.content">
                  {{ record.content }}
                </td>
                <td class="px-6 py-4 whitespace-nowrap text-sm">{{ ttlLabel(record.ttl) }}</td>
                <td class="px-6 py-4 whitespace-nowrap">
                  <BaseIcon
                    v-if="PROXYABLE_TYPES.includes(record.type)"
                    :path="record.proxied ? mdiCheck : mdiAlert"
                    :class="record.proxied ? 'text-orange-500' : 'text-gray-400'"
                    w="w-5"
                    h="h-5"
                  />
                  <span v-else class="text-xs text-gray-400">—</span>
                </td>
                <td class="px-6 py-4 whitespace-nowrap text-right">
                  <div class="inline-flex items-center gap-2">
                    <BaseButton
                      :icon="mdiPencil"
                      color="info"
                      small
                      @click="openEditRecord(record)"
                    />
                    <BaseButton
                      :icon="mdiDelete"
                      color="danger"
                      small
                      @click="askDeleteRecord(record)"
                    />
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </CardBox>
    </div>

    <!-- DNS Record Create/Edit Modal -->
    <CardBoxModal
      v-model="isRecordModalActive"
      :title="recordModalMode === 'edit' ? 'Edit DNS Record' : 'Add DNS Record'"
      :button-label="recordSaving ? 'Saving...' : (recordModalMode === 'edit' ? 'Update' : 'Create')"
      has-cancel
      @confirm="saveRecord"
    >
      <FormField label="Type">
        <FormControl
          v-model="recordForm.type"
          type="select"
          :options="RECORD_TYPES.map(t => ({ id: t, label: t }))"
        />
      </FormField>
      <FormField label="Name">
        <FormControl
          v-model="recordForm.name"
          :placeholder="selectedZone ? selectedZone.name + '  (use @ for root, or sub.' + selectedZone.name + ')' : 'example.com'"
        />
        <p class="text-xs text-gray-500 mt-1">
          Tip: use the apex domain (e.g. <code>{{ selectedZone?.name || 'example.com' }}</code>) or a subdomain
          (e.g. <code>www.{{ selectedZone?.name || 'example.com' }}</code>). Cloudflare accepts <code>@</code> as a shortcut for the root.
        </p>
      </FormField>
      <FormField label="Content">
        <FormControl
          v-model="recordForm.content"
          :placeholder="recordForm.type === 'A' ? '192.0.2.1' : recordForm.type === 'CNAME' ? 'target.example.com' : recordForm.type === 'MX' ? 'mail.example.com' : 'value'"
        />
      </FormField>
      <FormField v-if="typeNeedsPriority" label="Priority">
        <FormControl v-model="recordForm.priority" type="number" placeholder="10" />
      </FormField>
      <FormField label="TTL">
        <FormControl
          v-model="recordForm.ttl"
          type="select"
          :options="ttlOptions"
        />
      </FormField>
      <FormField v-if="typeSupportsProxy" label="Proxy through Cloudflare">
        <label class="inline-flex items-center gap-2 cursor-pointer">
          <input type="checkbox" v-model="recordForm.proxied" class="form-checkbox" />
          <span class="text-sm">Proxied (orange cloud — provides CDN, WAF, hides origin IP)</span>
        </label>
      </FormField>
      <FormField label="Comment">
        <FormControl v-model="recordForm.comment" placeholder="Optional note about this record" />
      </FormField>
    </CardBoxModal>

    <!-- DNS Record Delete Confirmation -->
    <CardBoxModal
      v-model="isDeleteRecordModalActive"
      title="Delete DNS Record"
      button-label="Delete"
      button="danger"
      has-cancel
      @confirm="confirmDeleteRecord"
    >
      <p v-if="recordToDelete">
        Are you sure you want to delete this DNS record?
      </p>
      <div v-if="recordToDelete" class="mt-3 p-3 bg-gray-50 dark:bg-gray-800 rounded text-sm font-mono">
        <div><strong>{{ recordToDelete.type }}</strong> {{ recordToDelete.name }}</div>
        <div class="text-gray-500 break-all">{{ recordToDelete.content }}</div>
      </div>
    </CardBoxModal>

    <!-- Add Account Modal -->
    <CardBoxModal
      v-model="isAddAccountModalActive"
      title="Add Cloudflare Account"
      button-label="Add"
      has-cancel
      @confirm="addAccount"
    >
      <FormField label="Account Name">
        <FormControl v-model="newAccount.name" placeholder="My Cloudflare Account" required />
      </FormField>
      <FormField label="Email">
        <FormControl v-model="newAccount.email" type="email" placeholder="user@example.com" required />
      </FormField>
      <FormField label="API Token">
        <FormControl v-model="newAccount.api_token" placeholder="Your Cloudflare API Token" required />
        <p class="text-sm text-gray-500 mt-2">
          Get your API token from: 
          <a href="https://dash.cloudflare.com/profile/api-tokens" target="_blank" class="text-blue-500 hover:underline">
            Cloudflare Dashboard → My Profile → API Tokens
          </a>
        </p>
      </FormField>
    </CardBoxModal>
  </div>
</template>
