<script setup>
/**
 * Alerts: which of the platform's own states are worth being told about, and
 * where to send them.
 *
 * The mail goes out through one of this server's own mailboxes rather than a
 * made-up sender, so it is signed by the domain it comes from and passes that
 * domain's SPF and DKIM. An alert that lands in spam is not an alert.
 */
import { computed, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useToast } from 'vue-toastification';

import BaseButton from '@/components/BaseButton.vue';
import BaseIcon from '@/components/BaseIcon.vue';
import CardBox from '@/components/CardBox.vue';
import SectionMain from '@/components/SectionMain.vue';
import SectionTitleLineWithButton from '@/components/SectionTitleLineWithButton.vue';
import ApiService from '@/services/ApiService';

import {
  mdiAlertCircleOutline, mdiBellOutline, mdiCheckCircleOutline, mdiEmailFastOutline,
  mdiRefresh, mdiContentSave
} from '@mdi/js';

const { t } = useI18n();
const toast = useToast();

const settings = ref(null);
const recent = ref([]);
const mailboxes = ref([]);
const loading = ref(false);
const busy = ref(false);
const loadError = ref('');
// The mail server only runs once a domain exists, so the sender list can be
// legitimately unavailable. That is worth explaining, not worth hiding the
// whole page over.
const mailUnavailable = ref(false);

const loadSettings = async () => {
  try {
    const response = await ApiService.get('/api/v1/system/notifications');
    if (response.data.error) {
      loadError.value = response.data.msg || t('nt.loadFailed');
      return;
    }
    settings.value = response.data.data.settings;
    recent.value = response.data.data.recent || [];
    loadError.value = '';
  } catch (error) {
    loadError.value = error.response?.data?.msg || t('nt.loadFailed');
  }
};

// Best effort: the page is about alerts, and the sender list is one field on it.
const loadMailboxes = async () => {
  try {
    const response = await ApiService.get('/api/email/mailboxes');
    mailboxes.value = response.data?.data || [];
    mailUnavailable.value = false;
  } catch (error) {
    mailboxes.value = [];
    mailUnavailable.value = error.response?.status === 503;
  }
};

// Loaded separately on purpose. Asking for both at once meant a mail server
// that was not running took the settings down with it, and the page rendered
// nothing at all.
const load = async () => {
  loading.value = true;
  await Promise.all([loadSettings(), loadMailboxes()]);
  loading.value = false;
};

// Returns whether the settings reached the server, so callers that depend on
// them can stop rather than act on values the server never saw.
const persist = async () => {
  try {
    const response = await ApiService.put('/api/v1/system/notifications', settings.value);
    if (response.data.error) {
      toast.error(response.data.msg);
      return false;
    }
    settings.value = response.data.data;
    return true;
  } catch (error) {
    toast.error(error.response?.data?.msg || t('nt.saveFailed'));
    return false;
  }
};

const save = async () => {
  busy.value = true;
  if (await persist()) toast.success(t('nt.saved'));
  busy.value = false;
};

// Confirming the address before anything depends on it: the worst time to find
// out it was wrong is when something is actually broken.
const sendTest = async () => {
  busy.value = true;
  try {
    // The server sends to the address it has stored, not the one on screen, so
    // save first. Testing an address the server was never told about failed
    // with "choose a mailbox first" while the form showed one filled in.
    if (!(await persist())) return;

    const response = await ApiService.post('/api/v1/system/notifications/test');
    if (response.data.error) {
      toast.error(response.data.msg);
      return;
    }
    toast.success(t('nt.testSent'));
  } catch (error) {
    toast.error(error.response?.data?.msg || t('nt.testFailed'));
  } finally {
    busy.value = false;
  }
};

const runCheck = async () => {
  busy.value = true;
  try {
    const response = await ApiService.post('/api/v1/system/notifications/check');
    if (!response.data.error) {
      const raised = response.data.data || [];
      toast.success(raised.length ? t('nt.checkRaised', { n: raised.length }) : t('nt.checkClean'));
      await load();
    }
  } catch (error) {
    toast.error(error.response?.data?.msg || t('nt.checkFailed'));
  } finally {
    busy.value = false;
  }
};

const ready = computed(() =>
  Boolean(settings.value?.mailbox_id) && Boolean((settings.value?.recipient || '').trim())
);

const levelLook = (level) => {
  if (level === 'critical') return { icon: mdiAlertCircleOutline, cls: 'text-red-500' };
  if (level === 'warning') return { icon: mdiAlertCircleOutline, cls: 'text-amber-500' };
  return { icon: mdiCheckCircleOutline, cls: 'text-emerald-500' };
};

const formatTime = (value) => (value ? new Date(value).toLocaleString() : '');

onMounted(load);
</script>

<template>
  <SectionMain>
    <SectionTitleLineWithButton :icon="mdiBellOutline" :title="t('nt.title')" main>
      <BaseButton :icon="mdiRefresh" color="light" small :disabled="loading" @click="load" />
    </SectionTitleLineWithButton>

    <p class="mb-6 text-sm text-gray-500">{{ t('nt.intro') }}</p>

    <CardBox v-if="loadError" class="mb-6">
      <p class="text-sm text-red-700 dark:text-red-400">{{ loadError }}</p>
      <p class="mt-2 text-xs text-gray-500">{{ t('nt.loadFailedHint') }}</p>
    </CardBox>

    <CardBox v-else-if="loading && !settings" class="mb-6">
      <p class="text-sm text-gray-500">{{ t('common.loading') }}</p>
    </CardBox>

    <CardBox v-if="settings" class="mb-6">
      <h3 class="mb-4 text-lg font-semibold">{{ t('nt.whereTitle') }}</h3>

      <label class="mb-4 flex items-center gap-2">
        <input v-model="settings.enabled" type="checkbox" />
        <span>{{ t('nt.enabled') }}</span>
      </label>

      <div class="mb-4 grid grid-cols-1 gap-4 md:grid-cols-2">
        <label class="block text-sm">
          <span class="mb-1 block text-gray-500">{{ t('nt.sender') }}</span>
          <select
            v-model.number="settings.mailbox_id"
            class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 dark:border-gray-700 dark:bg-slate-800"
          >
            <option :value="0" disabled>{{ t('nt.selectSender') }}</option>
            <option v-for="mailbox in mailboxes" :key="mailbox.id" :value="mailbox.id">
              {{ mailbox.email }}
            </option>
          </select>
          <span class="mt-1 block text-xs text-gray-500">{{ t('nt.senderHint') }}</span>
        </label>

        <label class="block text-sm">
          <span class="mb-1 block text-gray-500">{{ t('nt.recipient') }}</span>
          <input
            v-model="settings.recipient"
            type="email"
            :placeholder="t('nt.recipientPlaceholder')"
            class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 dark:border-gray-700 dark:bg-slate-800"
          />
          <span class="mt-1 block text-xs text-gray-500">{{ t('nt.recipientHint') }}</span>
        </label>
      </div>

      <p v-if="!mailboxes.length" class="mb-4 rounded-lg bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:bg-amber-500/10 dark:text-amber-300">
        {{ mailUnavailable ? t('nt.mailOffline') : t('nt.noMailboxes') }}
      </p>

      <div class="flex flex-wrap gap-2">
        <BaseButton :icon="mdiContentSave" :label="t('nt.save')" color="success" :disabled="busy" @click="save" />
        <BaseButton
          :icon="mdiEmailFastOutline"
          :label="t('nt.sendTest')"
          color="info"
          :disabled="busy || !ready"
          @click="sendTest"
        />
      </div>
    </CardBox>

    <CardBox v-if="settings" class="mb-6">
      <h3 class="mb-1 text-lg font-semibold">{{ t('nt.watchTitle') }}</h3>
      <p class="mb-4 text-sm text-gray-500">{{ t('nt.watchHint') }}</p>

      <div class="space-y-4">
        <div class="flex flex-wrap items-center gap-3">
          <label class="flex w-56 items-center gap-2">
            <input v-model="settings.watch_certificate" type="checkbox" />
            <span>{{ t('nt.watchCertificate') }}</span>
          </label>
          <label class="text-sm text-gray-500">
            {{ t('nt.certDays') }}
            <input
              v-model.number="settings.cert_days_before"
              type="number"
              min="1"
              class="ml-2 w-20 rounded border border-gray-300 px-2 py-1 dark:border-gray-700 dark:bg-slate-800"
            />
          </label>
        </div>

        <div class="flex flex-wrap items-center gap-3">
          <label class="flex w-56 items-center gap-2">
            <input v-model="settings.watch_queue" type="checkbox" />
            <span>{{ t('nt.watchQueue') }}</span>
          </label>
          <label class="text-sm text-gray-500">
            {{ t('nt.queueThreshold') }}
            <input
              v-model.number="settings.queue_threshold"
              type="number"
              min="1"
              class="ml-2 w-20 rounded border border-gray-300 px-2 py-1 dark:border-gray-700 dark:bg-slate-800"
            />
          </label>
        </div>

        <div class="flex flex-wrap items-center gap-3">
          <label class="flex w-56 items-center gap-2">
            <input v-model="settings.watch_blocked" type="checkbox" />
            <span>{{ t('nt.watchBlocked') }}</span>
          </label>
          <label class="text-sm text-gray-500">
            {{ t('nt.blockedThreshold') }}
            <input
              v-model.number="settings.blocked_threshold"
              type="number"
              min="1"
              class="ml-2 w-20 rounded border border-gray-300 px-2 py-1 dark:border-gray-700 dark:bg-slate-800"
            />
          </label>
        </div>

        <label class="flex w-56 items-center gap-2">
          <input v-model="settings.watch_memory" type="checkbox" />
          <span>{{ t('nt.watchMemory') }}</span>
        </label>

        <label class="block text-sm text-gray-500">
          {{ t('nt.repeatHours') }}
          <input
            v-model.number="settings.repeat_hours"
            type="number"
            min="1"
            class="ml-2 w-20 rounded border border-gray-300 px-2 py-1 dark:border-gray-700 dark:bg-slate-800"
          />
          <span class="mt-1 block text-xs">{{ t('nt.repeatHint') }}</span>
        </label>
      </div>

      <div class="mt-4 flex flex-wrap gap-2">
        <BaseButton :icon="mdiContentSave" :label="t('nt.save')" color="success" :disabled="busy" @click="save" />
        <BaseButton :icon="mdiRefresh" :label="t('nt.checkNow')" color="light" :disabled="busy" @click="runCheck" />
      </div>
    </CardBox>

    <CardBox>
      <h3 class="mb-1 text-lg font-semibold">{{ t('nt.recentTitle') }}</h3>
      <p class="mb-4 text-sm text-gray-500">{{ t('nt.recentHint') }}</p>

      <div v-if="!recent.length" class="rounded-lg border border-dashed border-gray-300 py-10 text-center text-sm text-gray-500 dark:border-slate-700">
        {{ t('nt.recentEmpty') }}
      </div>

      <ul v-else class="space-y-2">
        <li
          v-for="(alert, index) in recent"
          :key="index"
          class="flex items-start gap-3 rounded-lg border border-gray-200 px-4 py-3 dark:border-slate-700"
        >
          <BaseIcon :path="levelLook(alert.level).icon" :class="levelLook(alert.level).cls" w="w-5" h="h-5" />
          <div class="min-w-0 flex-1">
            <p class="font-medium">{{ alert.title }}</p>
            <p v-if="alert.detail" class="text-sm text-gray-500">{{ alert.detail }}</p>
            <p class="mt-1 text-xs text-gray-400">{{ formatTime(alert.at) }}</p>
          </div>
          <span
            v-if="alert.sent"
            class="shrink-0 rounded-full bg-emerald-100 px-2 py-0.5 text-xs text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400"
          >{{ t('nt.delivered') }}</span>
          <span
            v-else
            class="shrink-0 rounded-full bg-red-100 px-2 py-0.5 text-xs text-red-700 dark:bg-red-900/30 dark:text-red-400"
            :title="alert.send_error"
          >{{ t('nt.notDelivered') }}</span>
        </li>
      </ul>
    </CardBox>
  </SectionMain>
</template>
