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
  mdiPencil,
  mdiBroom
} from '@mdi/js';
import { computed, onMounted, ref } from "vue";
import { useToast } from 'vue-toastification';
import { useI18n } from 'vue-i18n';

const toast = useToast();
const { t } = useI18n();

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
const isPurgeModalActive = ref(false);
const recordModalMode = ref('create'); // 'create' | 'edit'
const recordSaving = ref(false);
const purging = ref(false);
const selectedAccount = ref(null);
const selectedZone = ref(null);
const recordToDelete = ref(null);
const zoneToPurge = ref(null);

// Purge cache form
const purgeForm = ref({
  everything: false,
  files: ''
});

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
    toast.error(t('cf.loadAccountsFailed'));
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
    toast.error(t('cf.loadZonesFailed'));
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
    toast.error(t('cf.loadRecordsFailed'));
  } finally {
    loading.value = false;
  }
};

const addAccount = async () => {
  try {
    const response = await ApiService.post('/api/cloudflare/accounts', newAccount.value);
    if (!response.data.error) {
      toast.success(t('cf.accountAdded'));
      await loadAccounts();
      isAddAccountModalActive.value = false;
      newAccount.value = { name: '', email: '', api_token: '' };
    } else {
      toast.error(t('cf.errorPrefix') + response.data.msg);
    }
  } catch (error) {
    toast.error(t('cf.errorColon') + error.message);
  }
};

const deleteAccount = async (accountId) => {
  if (!confirm('Are you sure you want to delete this account?')) {
    return;
  }
  
  try {
    const response = await ApiService.delete(`/api/cloudflare/accounts/${accountId}`);
    if (!response.data.error) {
      toast.success(t('cf.accountDeleted'));
      await loadAccounts();
    } else {
      toast.error(t('cf.errorPrefix') + response.data.msg);
    }
  } catch (error) {
    toast.error(t('cf.errorColon') + error.message);
  }
};

const syncZones = async (accountId) => {
  loading.value = true;
  try {
    const response = await ApiService.post(`/api/cloudflare/accounts/${accountId}/sync-zones`);
    if (!response.data.error) {
      toast.success(t('cf.syncedZones', { count: response.data.data?.count || 0 }));
      await loadZones();
    } else {
      toast.error(t('cf.errorPrefix') + response.data.msg);
    }
  } catch (error) {
    toast.error(t('cf.errorColon') + error.message);
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

const unwrap = (v) => (v && typeof v === 'object' ? v.id : v);

const buildRecordPayload = () => {
  const f = recordForm.value;
  const type = unwrap(f.type);
  const payload = {
    type,
    name: f.name.trim(),
    content: f.content.trim(),
    ttl: Number(unwrap(f.ttl)) || 1,
    comment: f.comment || ''
  };
  if (PROXYABLE_TYPES.includes(type)) {
    payload.proxied = !!f.proxied;
  }
  if (type === 'MX' || type === 'SRV') {
    payload.priority = Number(f.priority) || 0;
  }
  return payload;
};

const saveRecord = async () => {
  if (!selectedZone.value) {
    toast.error(t('cf.noZoneSelected'));
    return;
  }
  const f = recordForm.value;
  if (!f.type || !f.name || !f.content) {
    toast.error(t('cf.recordFieldsRequired'));
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
      toast.success(recordModalMode.value === 'edit' ? t('cf.recordUpdated') : t('cf.recordCreated'));
      isRecordModalActive.value = false;
      await loadDNSRecords(selectedZone.value.id);
    } else {
      toast.error(t('cf.errorPrefix') + response.data.msg);
    }
  } catch (error) {
    toast.error(t('cf.errorColon') + (error.response?.data?.msg || error.message));
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
      toast.success(t('cf.recordDeleted'));
      isDeleteRecordModalActive.value = false;
      recordToDelete.value = null;
      await loadDNSRecords(selectedZone.value.id);
    } else {
      toast.error(t('cf.errorPrefix') + response.data.msg);
    }
  } catch (error) {
    toast.error(t('cf.errorColon') + (error.response?.data?.msg || error.message));
  }
};

const openPurgeModal = (zone) => {
  zoneToPurge.value = zone;
  purgeForm.value = { everything: false, files: '' };
  isPurgeModalActive.value = true;
};

const purgeCache = async () => {
  if (!zoneToPurge.value) return;

  const files = purgeForm.value.files
    .split('\n')
    .map(f => f.trim())
    .filter(Boolean);

  if (!purgeForm.value.everything && files.length === 0) {
    toast.error(t('cf.enterUrls'));
    return;
  }
  if (files.length > 30) {
    toast.error(t('cf.maxUrls'));
    return;
  }

  purging.value = true;
  try {
    const payload = purgeForm.value.everything
      ? { everything: true }
      : { files };
    const response = await ApiService.post(
      `/api/cloudflare/zones/${zoneToPurge.value.id}/purge-cache`,
      payload
    );
    if (!response.data.error) {
      toast.success(t('cf.cachePurged'));
      isPurgeModalActive.value = false;
      zoneToPurge.value = null;
    } else {
      toast.error(t('cf.errorPrefix') + response.data.msg);
    }
  } catch (error) {
    toast.error(t('cf.errorColon') + (error.response?.data?.msg || error.message));
  } finally {
    purging.value = false;
  }
};
</script>

<template>
  <div>
    <SectionTitleLineWithButton :icon="mdiCloud" :title="t('cf.title')" main />

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
          {{ t('cf.tabs.' + tab) }}
        </button>
      </div>
    </div>

    <!-- Accounts Tab -->
    <div v-if="activeTab === 'accounts'">
      <CardBox>
        <div class="flex items-center justify-between mb-6">
          <h3 class="text-xl font-semibold">{{ t('cf.cfAccounts') }}</h3>
          <BaseButton
            :icon="mdiPlus"
            color="success"
            :label="t('cf.addAccount')"
            @click="isAddAccountModalActive = true"
          />
        </div>

        <div v-if="accounts.length === 0" class="text-center py-12 text-gray-500">
          {{ t('cf.noAccounts') }}
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
                <p class="text-xs text-gray-400 mt-1">{{ t('cf.added') }}: {{ formatDate(account.created_at) }}</p>
              </div>
              <div class="flex items-center gap-2">
                <BaseButton
                  :icon="mdiSync"
                  color="info"
                  :label="t('cf.syncZones')"
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
          <h3 class="text-xl font-semibold">{{ t('cf.cfZones') }}</h3>
          <BaseButton
            :icon="mdiRefresh"
            color="info"
            :label="t('common.refresh')"
            small
            @click="loadZones()"
          />
        </div>

        <div v-if="zones.length === 0" class="text-center py-12 text-gray-500">
          {{ t('cf.noZones') }}
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
              :label="t('cf.viewDnsRecords')"
              color="info"
              small
              class="w-full"
              @click="activeTab = 'dns'; selectedZone = zone; loadDNSRecords(zone.id)"
            />
            <BaseButton
              :icon="mdiBroom"
              :label="t('cf.purgeCache')"
              color="warning"
              small
              outline
              class="w-full mt-2"
              @click="openPurgeModal(zone)"
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
              <h3 class="text-xl font-semibold">{{ t('cf.dnsRecords') }}</h3>
              <p v-if="selectedZone" class="text-sm text-gray-500">{{ selectedZone.name }}</p>
            </div>
            <div class="flex items-center gap-2">
              <BaseButton
                :icon="mdiPlus"
                color="success"
                :label="t('cf.addRecord')"
                small
                :disabled="!selectedZone"
                @click="openCreateRecord"
              />
              <BaseButton
                :icon="mdiRefresh"
                color="info"
                :label="t('common.refresh')"
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
              <strong>{{ t('cf.emailDnsLabel') }}</strong> {{ t('cf.emailDnsText') }}
            </p>
          </div>
        </div>

        <div v-if="!selectedZone" class="text-center py-12 text-gray-500">
          {{ t('cf.selectZoneHint') }}
        </div>

        <div v-else-if="loading" class="text-center py-12">
          <div class="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600 mx-auto"></div>
        </div>

        <div v-else-if="dnsRecords.length === 0" class="text-center py-12 text-gray-500">
          {{ t('cf.noRecords') }}
        </div>

        <div v-else class="overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
            <thead class="bg-gray-50 dark:bg-gray-800">
              <tr>
                <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">{{ t('cf.type') }}</th>
                <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">{{ t('common.name') }}</th>
                <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">{{ t('cf.content') }}</th>
                <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">{{ t('cf.ttl') }}</th>
                <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">{{ t('cf.proxied') }}</th>
                <th class="px-6 py-3 text-right text-xs font-medium text-gray-500 uppercase">{{ t('common.actions') }}</th>
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
      :title="recordModalMode === 'edit' ? t('cf.editRecord') : t('cf.addRecordTitle')"
      :button-label="recordSaving ? t('common.saving') : (recordModalMode === 'edit' ? t('common.update') : t('common.create'))"
      has-cancel
      @confirm="saveRecord"
    >
      <FormField :label="t('cf.type')">
        <FormControl
          v-model="recordForm.type"
          type="select"
          :options="RECORD_TYPES"
        />
      </FormField>
      <FormField :label="t('common.name')">
        <FormControl
          v-model="recordForm.name"
          :placeholder="selectedZone ? selectedZone.name + '  (use @ for root, or sub.' + selectedZone.name + ')' : 'example.com'"
        />
        <p class="text-xs text-gray-500 mt-1" v-html="t('cf.nameTip', { zone: selectedZone?.name || 'example.com' })"></p>
      </FormField>
      <FormField :label="t('cf.content')">
        <FormControl
          v-model="recordForm.content"
          :placeholder="recordForm.type === 'A' ? '192.0.2.1' : recordForm.type === 'CNAME' ? 'target.example.com' : recordForm.type === 'MX' ? 'mail.example.com' : 'value'"
        />
      </FormField>
      <FormField v-if="typeNeedsPriority" :label="t('cf.priority')">
        <FormControl v-model="recordForm.priority" type="number" placeholder="10" />
      </FormField>
      <FormField :label="t('cf.ttl')">
        <FormControl
          v-model="recordForm.ttl"
          type="select"
          :options="ttlOptions"
        />
      </FormField>
      <FormField v-if="typeSupportsProxy" :label="t('cf.proxyThrough')">
        <label class="inline-flex items-center gap-2 cursor-pointer">
          <input v-model="recordForm.proxied" type="checkbox" class="form-checkbox" />
          <span class="text-sm">{{ t('cf.proxiedHint') }}</span>
        </label>
      </FormField>
      <FormField :label="t('cf.comment')">
        <FormControl v-model="recordForm.comment" :placeholder="t('cf.commentPlaceholder')" />
      </FormField>
    </CardBoxModal>

    <!-- DNS Record Delete Confirmation -->
    <CardBoxModal
      v-model="isDeleteRecordModalActive"
      :title="t('cf.deleteRecord')"
      :button-label="t('common.delete')"
      button="danger"
      has-cancel
      @confirm="confirmDeleteRecord"
    >
      <p v-if="recordToDelete">
        {{ t('cf.deleteRecordConfirm') }}
      </p>
      <div v-if="recordToDelete" class="mt-3 p-3 bg-gray-50 dark:bg-gray-800 rounded text-sm font-mono">
        <div><strong>{{ recordToDelete.type }}</strong> {{ recordToDelete.name }}</div>
        <div class="text-gray-500 break-all">{{ recordToDelete.content }}</div>
      </div>
    </CardBoxModal>

    <!-- Purge Cache Modal -->
    <CardBoxModal
      v-model="isPurgeModalActive"
      :title="t('cf.purgeCache') + (zoneToPurge ? ' — ' + zoneToPurge.name : '')"
      :button-label="purging ? t('cf.purging') : t('cf.purge')"
      button="warning"
      has-cancel
      @confirm="purgeCache"
    >
      <FormField :label="t('cf.purgeEverything')">
        <label class="inline-flex items-center gap-2 cursor-pointer">
          <input v-model="purgeForm.everything" type="checkbox" class="form-checkbox" />
          <span class="text-sm">{{ t('cf.purgeEverythingHint') }}</span>
        </label>
      </FormField>
      <FormField v-if="!purgeForm.everything" :label="t('cf.urlsToPurge')">
        <textarea
          v-model="purgeForm.files"
          rows="5"
          class="w-full px-3 py-2 border border-gray-300 dark:border-gray-700 rounded-lg bg-white dark:bg-gray-800 text-sm font-mono focus:outline-none focus:ring focus:border-blue-300"
          placeholder="https://example.com/assets/index-abc123.js&#10;https://example.com/assets/index-abc123.css"
        ></textarea>
        <p class="text-xs text-gray-500 mt-1">
          {{ t('cf.urlsHint') }}
        </p>
      </FormField>
      <div v-else class="p-3 bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-lg text-sm text-yellow-800 dark:text-yellow-200" v-html="t('cf.purgeEverythingWarning')"></div>
    </CardBoxModal>

    <!-- Add Account Modal -->
    <CardBoxModal
      v-model="isAddAccountModalActive"
      :title="t('cf.addAccountTitle')"
      :button-label="t('common.add')"
      has-cancel
      @confirm="addAccount"
    >
      <FormField :label="t('cf.accountName')">
        <FormControl v-model="newAccount.name" placeholder="My Cloudflare Account" required />
      </FormField>
      <FormField :label="t('cf.email')">
        <FormControl v-model="newAccount.email" type="email" placeholder="user@example.com" required />
      </FormField>
      <FormField :label="t('cf.apiToken')">
        <FormControl v-model="newAccount.api_token" :placeholder="t('cf.apiTokenPlaceholder')" required />
        <p class="text-sm text-gray-500 mt-2" v-html="t('cf.apiTokenHint')"></p>
      </FormField>
    </CardBoxModal>
  </div>
</template>
