<script setup>
import BaseButton from "@/components/BaseButton.vue";
import BaseIcon from "@/components/BaseIcon.vue";
import CardBox from "@/components/CardBox.vue";
import CardBoxModal from "@/components/CardBoxModal.vue";
import FormControl from "@/components/FormControl.vue";
import FormField from "@/components/FormField.vue";
import WebmailPanel from "@/components/Email/WebmailPanel.vue";
import FilterRules from "@/components/Email/FilterRules.vue";
import ConnectionSettings from "@/components/Email/ConnectionSettings.vue";

import ApiService from "@/services/ApiService";
import {
  mdiEmail,
  mdiEmailOpen,
  mdiEmailPlus,
  mdiServer,
  mdiPlay,
  mdiStop,
  mdiRefresh,
  mdiDomain,
  mdiPlus,
  mdiAccount,
  mdiDelete,
  mdiCog,
  mdiCloudUpload,
  mdiSend,
  mdiInbox,
  mdiPencil,
  mdiInformationOutline,
  mdiCloud,
  mdiKey,
  mdiContentCopy
} from '@mdi/js';
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { useToast } from 'vue-toastification';
import { useI18n } from 'vue-i18n';

const toast = useToast();
const { t } = useI18n();

// State
const loading = ref(false);
const activeTab = ref('overview');
const serverStatus = ref({
  is_running: false,
  hostname: '',
  ip_address: '',
  smtp_port: 25,
  submission_port: 587,
  imap_port: 143,
  imaps_port: 993,
  spam_enabled: true,
  virus_enabled: true,
  dkim_enabled: true,
  ssl_enabled: false
});

// Server IP configuration
const serverIPForm = ref({
  ip_address: ''
});
const isEditingIP = ref(false);

const domains = ref([]);
const mailboxes = ref([]);
/** API'den gelen thread listesi (her thread: { thread_id, subject, date, count, messages }) */

// Modals
const isAddDomainModalActive = ref(false);
const isEditDomainModalActive = ref(false);
const isAddMailboxModalActive = ref(false);
const isEditMailboxModalActive = ref(false);
const isUpdatePasswordModalActive = ref(false);
const selectedMailbox = ref(null);
const selectedDomain = ref(null);
// Forms
const newDomain = ref({
  domain: '',
  description: ''
});

const newMailbox = ref({
  domain_id: '',
  username: '',
  password: '',
  name: ''
});

const updatePasswordForm = ref({
  password: '',
  confirmPassword: ''
});

const editDomainForm = ref({
  description: '',
  enabled: true
});

const editMailboxForm = ref({
  name: '',
  quota: 10737418240,
  enabled: true,
  forward_to: '',
  keep_copy: true,
  auto_reply: false,
  auto_reply_msg: '',
  password: ''
});

// Computed
// The listener status is the live truth; the stored config flag is the fallback
// for the moment before the engine status has been fetched.
// Live status of the mail listeners, filled by loadEngine().
// running starts as null so the badge falls back to the stored flag until the
// live listener status has been fetched.
const nativeStatus = ref({ running: null, listeners: [], cert_source: '', queue_length: 0, mail_root: '' });

const serverRunning = computed(() => nativeStatus.value?.running ?? serverStatus.value.is_running);

/** Gövde metnini "On ... wrote:" / "şunu yazdı:" bloklarına böler; her blok ayrı kart (CardBox) için kullanılır */
// Methods
const loadData = async () => {
  await Promise.all([
    loadServerStatus(),
    loadDomains(),
    loadMailboxes()
  ]);
};

const loadServerStatus = async () => {
  try {
    const response = await ApiService.get('/api/email/server/status');
    if (!response.data.error) {
      serverStatus.value = response.data.data;
    }
  } catch (error) {
    console.error('Failed to load server status:', error);
  }
};

const loadDomains = async () => {
  try {
    const response = await ApiService.get('/api/email/domains');
    if (!response.data.error) {
      domains.value = response.data.data || [];
    }
  } catch (error) {
    console.error('Failed to load domains:', error);
  }
};

const loadMailboxes = async () => {
  try {
    const response = await ApiService.get('/api/email/mailboxes');
    if (!response.data.error) {
      mailboxes.value = response.data.data || [];
    }
  } catch (error) {
    console.error('Failed to load mailboxes:', error);
  }
};

const updateServerIP = async () => {
  if (!serverIPForm.value.ip_address) {
    toast.error(t('em.enterIpAddress'));
    return;
  }

  loading.value = true;
  try {
    const response = await ApiService.put('/api/email/server/ip', {
      ip_address: serverIPForm.value.ip_address
    });
    
    if (!response.data.error) {
      toast.success('✅ ' + response.data.msg);
      toast.info(t('em.dnsUpdatingCloudflare'), { timeout: 5000 });
      isEditingIP.value = false;
      await loadServerStatus();
    } else {
      toast.error('❌ ' + response.data.msg);
    }
  } catch (error) {
    toast.error(t('em.errorPrefix') + error.message);
  } finally {
    loading.value = false;
  }
};

// Adding a domain creates its postmaster mailbox and deleting one takes that
// mailbox and the domain's aliases away, so a domain change is never only a
// change to the domain list. Reloading just that list left the Accounts tab
// showing a postmaster whose domain was gone.
const refreshDomains = async () => {
  await Promise.all([loadDomains(), loadMailboxes(), loadPasswordCheck(), loadAliases()]);
};

const addDomain = async () => {
  try {
    const response = await ApiService.post('/api/email/domains', newDomain.value);
    if (!response.data.error) {
      toast.success(t('em.domainAdded'));
      toast.info(t('em.checkingCloudflare'), { timeout: 3000 });
      await refreshDomains();
      isAddDomainModalActive.value = false;
      newDomain.value = { domain: '', description: '' };
    } else {
      toast.error('❌ ' + response.data.msg);
    }
  } catch (error) {
    toast.error(t('em.errorPrefix') + error.message);
  }
};

const openEditDomainModal = (domain) => {
  selectedDomain.value = domain;
  editDomainForm.value = {
    description: domain.description || '',
    enabled: domain.enabled
  };
  isEditDomainModalActive.value = true;
};

const editDomain = async () => {
  try {
    const response = await ApiService.put(`/api/email/domains/${selectedDomain.value.id}`, editDomainForm.value);
    if (!response.data.error) {
      toast.success(t('em.domainUpdated'));
      toast.info(t('em.dnsQueued'), { timeout: 3000 });
      await refreshDomains();
      isEditDomainModalActive.value = false;
    } else {
      toast.error('❌ ' + response.data.msg);
    }
  } catch (error) {
    toast.error(t('em.errorPrefix') + error.message);
  }
};

const deleteDomain = async (domainId) => {
  if (!confirm(t('em.confirmDeleteDomain'))) {
    return;
  }
  
  try {
    const response = await ApiService.delete(`/api/email/domains/${domainId}`);
    if (!response.data.error) {
      toast.success(t('em.domainDeleted'));
      await refreshDomains();
    } else {
      toast.error('❌ ' + response.data.msg);
    }
  } catch (error) {
    toast.error(t('em.errorPrefix') + error.message);
  }
};

const openAddMailboxModal = () => {
  if (domains.value.length === 0) {
    toast.error(t('em.addDomainFirst'));
    return;
  }
  isAddMailboxModalActive.value = true;
};

// Opening the account form from a domain row: the domain is already known, so
// it arrives filled in rather than as one more thing to pick.
const openAddMailboxForDomain = (domain) => {
  newMailbox.value = { domain_id: domain.id, username: '', password: '', name: '' };
  isAddMailboxModalActive.value = true;
};

// Every domain gets a postmaster mailbox it did not ask for, so counting it
// would tell someone they have an account when they have none.
// Initials for a mailbox avatar. Kept here rather than imported from the
// webmail panel: this is the one place left that needs it.
const getInitials = (value) => {
  if (!value) return '?';
  const parts = String(value).split(/[\s._@-]+/).filter(Boolean);
  if (!parts.length) return '?';
  const letters = parts.length === 1 ? parts[0].slice(0, 2) : parts[0][0] + parts[1][0];
  return letters.toUpperCase();
};

const mailboxCount = (domainID) =>
  mailboxes.value.filter((mb) => mb.domain_id === domainID && mb.username !== 'postmaster').length;

// Anything that changes a mailbox changes both the list and what the password
// check says about it. Refreshing only the list left the warning standing
// after the password behind it had been set, and the only way out was to
// leave the tab and come back.
// Connection settings are read from the running listeners, which live in the
// engine payload — the Mailboxes tab did not need it before.
const isConnectionModalActive = ref(false);
const connectionMailbox = ref(null);

const openConnectionSettings = async (mailbox) => {
  connectionMailbox.value = mailbox;
  isConnectionModalActive.value = true;
  if (!nativeStatus.value?.listeners?.length) await loadEngine();
};

const refreshMailboxes = async () => {
  await Promise.all([loadMailboxes(), loadPasswordCheck()]);
};

const addMailbox = async () => {
  try {
    if (!newMailbox.value.domain_id) {
      toast.error(t('em.selectDomainErr'));
      return;
    }
    
    const response = await ApiService.post('/api/email/mailboxes', newMailbox.value);
    if (!response.data.error) {
      toast.success(t('em.mailboxCreated'));
      await refreshMailboxes();
      isAddMailboxModalActive.value = false;
      newMailbox.value = { domain_id: '', username: '', password: '', name: '' };
    } else {
      toast.error('❌ ' + response.data.msg);
    }
  } catch (error) {
    toast.error(t('em.errorPrefix') + error.message);
  }
};

const openEditMailboxModal = (mailbox) => {
  selectedMailbox.value = mailbox;
  editMailboxForm.value = {
    name: mailbox.name || '',
    quota: mailbox.quota || 10737418240,
    enabled: mailbox.enabled,
    forward_to: mailbox.forward_to || '',
    keep_copy: mailbox.keep_copy !== undefined ? mailbox.keep_copy : true,
    auto_reply: mailbox.auto_reply || false,
    auto_reply_msg: mailbox.auto_reply_msg || '',
    password: ''
  };
  isEditMailboxModalActive.value = true;
};

const editMailbox = async () => {
  try {
    const payload = { ...editMailboxForm.value };
    
    // Remove password if empty
    if (!payload.password) {
      delete payload.password;
    }
    
    const response = await ApiService.put(`/api/email/mailboxes/${selectedMailbox.value.id}`, payload);
    if (!response.data.error) {
      toast.success(t('em.mailboxUpdated'));
      toast.info(t('em.dnsQueued'), { timeout: 3000 });
      await refreshMailboxes();
      isEditMailboxModalActive.value = false;
    } else {
      toast.error('❌ ' + response.data.msg);
    }
  } catch (error) {
    toast.error(t('em.errorPrefix') + error.message);
  }
};

const openUpdatePasswordModal = (mailbox) => {
  selectedMailbox.value = mailbox;
  updatePasswordForm.value = {
    password: '',
    confirmPassword: ''
  };
  isUpdatePasswordModalActive.value = true;
};

const updateMailboxPassword = async () => {
  if (!updatePasswordForm.value.password) {
    toast.error(t('em.passwordRequired'));
    return;
  }

  if (updatePasswordForm.value.password !== updatePasswordForm.value.confirmPassword) {
    toast.error(t('em.passwordsNoMatch'));
    return;
  }

  if (updatePasswordForm.value.password.length < 6) {
    toast.error(t('em.passwordMin'));
    return;
  }

  try {
    const response = await ApiService.put(
      `/api/email/mailboxes/${selectedMailbox.value.id}/password`,
      { password: updatePasswordForm.value.password }
    );
    
    if (!response.data.error) {
      toast.success(t('em.passwordUpdated'));
      isUpdatePasswordModalActive.value = false;
      updatePasswordForm.value = { password: '', confirmPassword: '' };
      await refreshMailboxes();
    } else {
      toast.error('❌ ' + response.data.msg);
    }
  } catch (error) {
    toast.error(t('em.errorPrefix') + error.message);
  }
};

const deleteMailbox = async (mailboxId) => {
  if (!confirm(t('em.confirmDeleteMailbox'))) {
    return;
  }
  
  try {
    const response = await ApiService.delete(`/api/email/mailboxes/${mailboxId}`);
    if (!response.data.error) {
      toast.success(t('em.mailboxDeleted'));
      await refreshMailboxes();
    } else {
      toast.error('❌ ' + response.data.msg);
    }
  } catch (error) {
    toast.error(t('em.errorPrefix') + error.message);
  }
};

const formatDate = (date) => {
  if (!date) return '';
  return new Date(date).toLocaleString();
};



// ---- Mail traffic logs -------------------------------------------------
const logsLoading = ref(false);
const logEntries = ref([]);
const logStats = ref({ incoming: 0, outgoing: 0, rejected: 0, deferred: 0, bounced: 0 });
const logSource = ref('');
const logTotal = ref(0);
const logDirection = ref('');
const logStatus = ref('');
const logSearch = ref('');
const logTail = ref(1000);
const logAutoRefresh = ref(true);
const expandedLogIds = ref(new Set());
const logError = ref('');
const rawLogLines = ref([]);
let logTimer = null;

const loadLogs = async () => {
  logsLoading.value = true;
  try {
    const response = await ApiService.get('/api/email/logs', {
      params: {
        limit: logTail.value,
        tail: logTail.value,
        direction: logDirection.value || undefined,
        status: logStatus.value || undefined,
        search: logSearch.value || undefined
      }
    });
    if (!response.data.error) {
      const data = response.data.data || {};
      logEntries.value = data.entries || [];
      logStats.value = data.stats || logStats.value;
      logSource.value = data.source || '';
      logTotal.value = data.total || 0;
      logError.value = '';
    } else {
      logError.value = response.data.msg || t('em.logsFailed');
    }
  } catch (error) {
    logError.value = error.response?.data?.msg || error.message || t('em.logsFailed');
  } finally {
    logsLoading.value = false;
  }
};

const loadRawLog = async () => {
  logsLoading.value = true;
  try {
    const response = await ApiService.get('/api/email/logs/raw', { params: { tail: logTail.value } });
    if (!response.data.error) {
      rawLogLines.value = response.data.data?.lines || [];
      logError.value = '';
    } else {
      logError.value = response.data.msg || t('em.logsFailed');
    }
  } catch (error) {
    logError.value = error.response?.data?.msg || error.message || t('em.logsFailed');
  } finally {
    logsLoading.value = false;
  }
};

const connections = ref([]);
const expandedConnIds = ref(new Set());
const logView = ref('events'); // events | connections | raw

// Connection traces show what happened on the wire: attempts that never became
// a message (refused TLS, probes, bad passwords) live here, not in the message log.
const certificate = ref(null);
const requestingCert = ref(false);

const deliverability = ref(null);
const checkingDeliverability = ref(false);

// The inbox-versus-spam verdict is decided by DNS the receiver looks up, not by
// anything in the message — so the server checks that DNS and reports it.
const runDeliverabilityCheck = async () => {
  checkingDeliverability.value = true;
  try {
    const response = await ApiService.get('/api/email/deliverability');
    if (!response.data.error) deliverability.value = response.data.data;
  } catch (error) {
    toast.error(error.response?.data?.msg || t('em.deliverFailed'));
  } finally {
    checkingDeliverability.value = false;
  }
};

const checkLevelClass = (level) => {
  switch (level) {
    case 'ok':
      return 'bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400';
    case 'fail':
      return 'bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-400';
    case 'warning':
      return 'bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400';
    default:
      return 'bg-gray-100 dark:bg-slate-800 text-gray-600 dark:text-gray-300';
  }
};

const loadCertificate = async () => {
  try {
    const response = await ApiService.get('/api/email/certificate');
    if (!response.data.error) certificate.value = response.data.data;
  } catch (error) {
    console.error('Failed to load certificate status:', error);
  }
};

// Ask Let's Encrypt — through the API Gateway's ACME account — for a
// certificate covering the mail hostname.
const requestCertificate = async () => {
  requestingCert.value = true;
  try {
    const response = await ApiService.post('/api/email/certificate/request');
    if (!response.data.error) {
      certificate.value = response.data.data;
      toast.success(t('em.certIssued'));
      await loadEngine();
    } else {
      certificate.value = response.data.data || certificate.value;
      toast.error(response.data.msg || t('em.certFailed'));
    }
  } catch (error) {
    toast.error(error.response?.data?.msg || t('em.certFailed'));
  } finally {
    requestingCert.value = false;
  }
};

const loadConnections = async () => {
  logsLoading.value = true;
  try {
    const response = await ApiService.get('/api/email/logs/connections', { params: { limit: 200 } });
    if (!response.data.error) {
      connections.value = response.data.data || [];
      logError.value = '';
    } else {
      logError.value = response.data.msg || t('em.logsFailed');
    }
  } catch (error) {
    logError.value = error.response?.data?.msg || error.message || t('em.logsFailed');
  } finally {
    logsLoading.value = false;
  }
};

const toggleConnection = (id) => {
  const next = new Set(expandedConnIds.value);
  if (next.has(id)) {
    next.delete(id);
  } else {
    next.add(id);
  }
  expandedConnIds.value = next;
};

const formatTraceTime = (value) => {
  if (!value) return '';
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleTimeString();
};

const connectionDuration = (conn) => {
  if (!conn.started_at) return '-';
  const start = new Date(conn.started_at).getTime();
  const end = conn.ended_at ? new Date(conn.ended_at).getTime() : Date.now();
  const ms = end - start;
  if (ms < 1000) return ms + ' ms';
  return (ms / 1000).toFixed(1) + ' s';
};

const refreshLogs = () => {
  if (logView.value === 'connections') return loadConnections();
  if (logView.value === 'raw') return loadRawLog();
  return loadLogs();
};

const toggleLogDetail = (id) => {
  const next = new Set(expandedLogIds.value);
  if (next.has(id)) {
    next.delete(id);
  } else {
    next.add(id);
  }
  expandedLogIds.value = next;
};

const directionMeta = (direction) => {
  switch (direction) {
    case 'in':
      return { icon: mdiInbox, label: t('em.logIncoming'), cls: 'text-emerald-500' };
    case 'out':
      return { icon: mdiSend, label: t('em.logOutgoing'), cls: 'text-blue-500' };
    default:
      return { icon: mdiInformationOutline, label: t('em.logSystem'), cls: 'text-gray-400' };
  }
};

const statusClass = (status) => {
  switch (status) {
    case 'sent':
    case 'delivered':
      return 'bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400';
    case 'rejected':
    case 'bounced':
      return 'bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-400';
    case 'deferred':
    case 'expired':
      return 'bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400';
    case 'login':
      return 'bg-slate-200 dark:bg-slate-700 text-slate-600 dark:text-slate-300';
    default:
      return 'bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-400';
  }
};

const formatLogTime = (value) => {
  if (!value) return '-';
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? value : d.toLocaleString();
};

const formatLogSize = (bytes) => {
  if (!bytes) return '';
  if (bytes < 1024) return bytes + ' B';
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
};

// Poll only while the logs tab is open, so the other tabs cost nothing.
watch([activeTab, logAutoRefresh, logView], () => {
  if (logTimer) {
    clearInterval(logTimer);
    logTimer = null;
  }
  if (activeTab.value !== 'logs') return;

  refreshLogs();
  if (logAutoRefresh.value) {
    logTimer = setInterval(refreshLogs, 10000);
  }
});

watch([logDirection, logStatus], () => {
  if (activeTab.value === 'logs' && logView.value === 'events') loadLogs();
});

onUnmounted(() => {
  if (logTimer) clearInterval(logTimer);
});

// ---- Mail server administration ----
const engineLoading = ref(false);
const engineSaving = ref(false);
const controlBusy = ref(false);
const nativeConfig = ref(null);
const selfTest = ref([]);
const queueItems = ref([]);
const dnsRecords = ref([]);
const dnsSyncing = ref(false);
const dnsPreview = ref([]);
const dnsChecking = ref(false);

// Reading the zone before writing to it shows which records are already
// correct, which differ and which are missing — without changing anything.
const previewDns = async () => {
  dnsChecking.value = true;
  try {
    const response = await ApiService.get('/api/email/dns-records/preview');
    if (!response.data.error) dnsPreview.value = response.data.data || [];
  } catch (error) {
    toast.error(error.response?.data?.msg || t('em.dnsPreviewFailed'));
  } finally {
    dnsChecking.value = false;
  }
};

const dnsActionClass = (action) => {
  switch (action) {
    case 'unchanged':
    case 'created':
    case 'updated':
      return 'bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400';
    case 'missing':
      return 'bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400';
    case 'differs':
      return 'bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-400';
    default:
      return 'bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-400';
  }
};
const legacyArtifacts = ref([]);
const cleaningUp = ref(false);

// Tabs, in the order an operator works through them.
const mailTabs = ['overview', 'domains', 'mailboxes', 'webmail', 'filters', 'logs', 'listeners', 'queue', 'dns', 'security', 'cleanup'];

const loadEngine = async () => {
  engineLoading.value = true;
  try {
    const [engineRes, queueRes, dnsRes, legacyRes] = await Promise.all([
      ApiService.get('/api/email/engine'),
      ApiService.get('/api/email/queue'),
      ApiService.get('/api/email/dns-records'),
      ApiService.get('/api/email/legacy')
    ]);

    if (!engineRes.data.error) {
      const data = engineRes.data.data || {};
      nativeStatus.value = data.native || nativeStatus.value;
      nativeConfig.value = data.config || null;
      selfTest.value = data.self_test || [];
    }
    queueItems.value = queueRes.data?.data || [];
    dnsRecords.value = dnsRes.data?.data || [];
    legacyArtifacts.value = legacyRes.data?.data || [];
    await loadCertificate();
  } catch (error) {
    console.error('Failed to load mail server status:', error);
  } finally {
    engineLoading.value = false;
  }
};

// controlServer starts, stops or restarts the listeners.
const controlServer = async (action) => {
  controlBusy.value = true;
  try {
    const response = await ApiService.post('/api/email/control', { action });
    if (!response.data.error) {
      toast.success(t('em.control_' + action));
      nativeStatus.value = response.data.data?.native || nativeStatus.value;
      await Promise.all([loadServerStatus(), loadEngine()]);
    } else {
      toast.error(response.data.msg || t('em.controlFailed'));
    }
  } catch (error) {
    toast.error(error.response?.data?.msg || t('em.controlFailed'));
  } finally {
    controlBusy.value = false;
  }
};

// syncDns publishes MX/SPF/DKIM/DMARC to Cloudflare for one domain or all.
// syncDomainByName is what the preview's per-domain button calls.
const syncDomainByName = (name) => {
  const domain = domains.value.find((d) => d.domain === name);
  return syncDns(domain?.id || 0);
};

const syncDns = async (domainId) => {
  dnsSyncing.value = true;
  try {
    const response = await ApiService.post('/api/email/dns-records/sync', { domain_id: domainId || 0 });
    if (!response.data.error) {
      const data = response.data.data;
      const results = Array.isArray(data) ? data : [data];
      const ok = results.filter((r) => r.synced).length;
      if (ok > 0) {
        toast.success(t('em.dnsSynced', { count: ok }));
        // Show what went out; a missing MX is the usual reason mail vanishes.
        const published = results.flatMap((r) => (r.records || []).map((rec) => `${rec.kind} ${rec.action}`));
        if (published.length) toast.info(published.join(' · '), { timeout: 8000 });
      } else {
        toast.warning(results[0]?.message || t('em.dnsSyncSkipped'));
      }
      await runDeliverabilityCheck();
      await Promise.all([loadEngine(), loadDomains(), previewDns()]);
    } else {
      toast.error(response.data.msg || t('em.dnsSyncFailed'));
    }
  } catch (error) {
    toast.error(error.response?.data?.msg || t('em.dnsSyncFailed'));
  } finally {
    dnsSyncing.value = false;
  }
};

const cleanupLegacy = async () => {
  cleaningUp.value = true;
  try {
    const response = await ApiService.delete('/api/email/legacy');
    if (!response.data.error) {
      const removed = response.data.data || [];
      toast.success(t('em.cleanupDone', { count: removed.length }));
      await loadEngine();
    }
  } catch (error) {
    toast.error(error.response?.data?.msg || t('em.cleanupFailed'));
  } finally {
    cleaningUp.value = false;
  }
};

const saveNativeSettings = async () => {
  if (!nativeConfig.value) return;
  engineSaving.value = true;
  try {
    const response = await ApiService.put('/api/email/native/settings', nativeConfig.value);
    if (!response.data.error) {
      nativeConfig.value = response.data.data;
      toast.success(t('em.engineSettingsSaved'));
      await loadEngine();
    } else {
      toast.error(response.data.msg || t('em.engineSettingsFailed'));
    }
  } catch (error) {
    toast.error(error.response?.data?.msg || t('em.engineSettingsFailed'));
  } finally {
    engineSaving.value = false;
  }
};

const flushQueue = async () => {
  try {
    const response = await ApiService.post('/api/email/queue/flush');
    if (!response.data.error) {
      toast.success(t('em.queueFlushed', { count: response.data.data?.flushed ?? 0 }));
      await loadEngine();
    }
  } catch (error) {
    toast.error(error.response?.data?.msg || t('em.queueFlushFailed'));
  }
};

const deleteQueueItem = async (id) => {
  try {
    const response = await ApiService.delete(`/api/email/queue/${id}`);
    if (!response.data.error) {
      toast.success(t('em.queueItemDeleted'));
      await loadEngine();
    }
  } catch (error) {
    toast.error(error.response?.data?.msg || t('em.queueDeleteFailed'));
  }
};

const copyText = async (value) => {
  try {
    await navigator.clipboard.writeText(value);
    toast.success(t('em.copied'));
  } catch {
    toast.error(t('em.copyFailed'));
  }
};

const tlsBadge = (mode) => {
  switch (mode) {
    case 'implicit':
      return 'bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400';
    case 'starttls':
      return 'bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-400';
    default:
      return 'bg-gray-200 dark:bg-gray-700 text-gray-600 dark:text-gray-300';
  }
};

const formatQueueTime = (value) => {
  if (!value) return '-';
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? value : d.toLocaleString();
};

// immediate: the page opens on "overview", which needs this data straight away —
// without it the section stays empty until the tab is switched or refreshed by hand.
watch(
  activeTab,
  (tab) => {
      if (['overview', 'listeners', 'queue', 'dns', 'cleanup'].includes(tab)) loadEngine();
    if (tab === 'dns' && !deliverability.value) runDeliverabilityCheck();
    if (tab === 'dns' && !dnsPreview.value.length) previewDns();
    if (['domains', 'mailboxes'].includes(tab)) loadAliases();
    if (tab === 'security') loadBlockedClients();
    if (tab === 'mailboxes') loadPasswordCheck();
  },
  { immediate: true }
);

// ---- Aliases and abuse protection ----
const aliases = ref([]);
const blockedClients = ref([]);
const newAlias = ref({ alias: '', destination: '' });
const aliasBusy = ref(false);

const loadAliases = async () => {
  try {
    const response = await ApiService.get('/api/email/aliases');
    if (!response.data.error) aliases.value = response.data.data || [];
  } catch (error) {
    console.error('Failed to load aliases:', error);
  }
};

const addAlias = async () => {
  if (!newAlias.value.alias || !newAlias.value.destination) {
    toast.error(t('em.aliasIncomplete'));
    return;
  }
  aliasBusy.value = true;
  try {
    const response = await ApiService.post('/api/email/aliases', newAlias.value);
    if (!response.data.error) {
      toast.success(t('em.aliasCreated'));
      newAlias.value = { alias: '', destination: '' };
      await loadAliases();
    } else {
      toast.error(response.data.msg);
    }
  } catch (error) {
    toast.error(error.response?.data?.msg || t('em.aliasFailed'));
  } finally {
    aliasBusy.value = false;
  }
};

const toggleAlias = async (alias) => {
  try {
    await ApiService.put(`/api/email/aliases/${alias.id}`, { enabled: !alias.enabled });
    await loadAliases();
  } catch (error) {
    toast.error(error.response?.data?.msg || t('em.aliasFailed'));
  }
};

const deleteAlias = async (alias) => {
  try {
    const response = await ApiService.delete(`/api/email/aliases/${alias.id}`);
    if (!response.data.error) {
      toast.success(t('em.aliasDeleted'));
      await loadAliases();
    }
  } catch (error) {
    toast.error(error.response?.data?.msg || t('em.aliasFailed'));
  }
};

const loadBlockedClients = async () => {
  try {
    const response = await ApiService.get('/api/email/blocked');
    if (!response.data.error) blockedClients.value = response.data.data || [];
  } catch (error) {
    console.error('Failed to load blocked clients:', error);
  }
};

// The dashboard is the only place a broken password would show before a mail
// client failed to log in, so the check runs whenever Mailboxes is opened.
const passwordCheck = ref({ total: 0, unusable: 0, repairable: 0, mailboxes: [] });

const unusableMailboxes = computed(() =>
  (passwordCheck.value.mailboxes || []).filter((entry) => entry.state === 'unusable')
);

const loadPasswordCheck = async () => {
  try {
    const response = await ApiService.get('/api/email/server/check-passwords');
    if (!response.data.error) passwordCheck.value = response.data;
  } catch (error) {
    console.error('Failed to check mailbox passwords:', error);
  }
};

// Blocking by hand. The guard reacts to what an address does here; this is for
// one that has already earned a block somewhere else.
const newBlock = ref({ ip: '', reason: '', minutes: 60 });
const blockBusy = ref(false);

const blockClient = async () => {
  const ip = newBlock.value.ip.trim();
  if (!ip) {
    toast.error(t('em.blockNeedsIP'));
    return;
  }

  blockBusy.value = true;
  try {
    const response = await ApiService.post('/api/email/blocked', {
      ip,
      reason: newBlock.value.reason.trim(),
      minutes: Number(newBlock.value.minutes) || 0
    });
    if (response.data.error) {
      toast.error(response.data.msg);
      return;
    }
    toast.success(t('em.blockAdded', { ip }));
    newBlock.value = { ip: '', reason: '', minutes: 60 };
    await loadBlockedClients();
  } catch (error) {
    toast.error(error.response?.data?.msg || t('em.messageActionFailed'));
  } finally {
    blockBusy.value = false;
  }
};

const unblockClient = async (ip) => {
  try {
    const response = await ApiService.delete(`/api/email/blocked/${encodeURIComponent(ip)}`);
    if (!response.data.error) {
      toast.success(t('em.unblocked'));
      await loadBlockedClients();
    }
  } catch (error) {
    toast.error(error.response?.data?.msg || t('em.unblockFailed'));
  }
};

onMounted(() => {
  loadData();
  // The header's traffic counters read the log stats, so they are loaded once
  // regardless of which tab is open.
  loadLogs();
});
</script>

<template>
  <div class="space-y-8">
    <!-- Hero header -->
    <div class="bg-gradient-to-r from-sky-600 via-blue-600 to-indigo-600 rounded-2xl p-8 text-white shadow-lg">
      <div class="flex flex-col lg:flex-row lg:items-center lg:justify-between">
        <div>
          <h1 class="text-3xl lg:text-4xl font-bold mb-2 flex items-center">
            <BaseIcon :path="mdiEmail" size="40" class="mr-4" />
            {{ t('em.title') }}
          </h1>
          <p class="text-blue-100 text-lg">{{ t('em.shellSubtitle') }}</p>
        </div>

        <div class="mt-6 lg:mt-0 flex flex-wrap gap-3">
          <BaseButton
            v-if="!serverRunning"
            :icon="mdiPlay"
            color="success"
            :label="t('em.startServer')"
            :disabled="controlBusy"
            class="shadow-lg hover:shadow-xl transform hover:scale-105 transition-all duration-200"
            @click="controlServer('start')"
          />
          <template v-else>
            <BaseButton
              :icon="mdiRefresh"
              color="info"
              :label="t('em.restartServer')"
              :disabled="controlBusy"
              class="shadow-lg hover:shadow-xl transform hover:scale-105 transition-all duration-200"
              @click="controlServer('restart')"
            />
            <BaseButton
              :icon="mdiStop"
              color="danger"
              :label="t('em.stopServer')"
              :disabled="controlBusy"
              class="shadow-lg hover:shadow-xl"
              @click="controlServer('stop')"
            />
          </template>
        </div>
      </div>

      <div class="mt-4 flex flex-wrap items-center gap-4">
        <span
          :class="[
            'inline-flex items-center px-3 py-1 rounded-full text-sm font-medium',
            serverRunning ? 'bg-white/20' : 'bg-black/20'
          ]"
        >
          <span :class="['w-2 h-2 rounded-full mr-2', serverRunning ? 'bg-green-400' : 'bg-gray-400']"></span>
          {{ serverRunning ? t('em.running') : t('em.stopped') }}
        </span>
        <span class="text-blue-100 text-sm">{{ serverStatus.hostname || t('em.na') }}</span>
        <span v-if="serverStatus.ip_address" class="text-blue-100 text-sm font-mono">{{ serverStatus.ip_address }}</span>
        <span v-if="nativeStatus.cert_source" class="text-blue-100 text-sm">
          TLS: {{ nativeStatus.cert_source }}
        </span>
      </div>
    </div>

    <!-- Headline numbers -->
    <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
      <CardBox class="bg-gradient-to-br from-blue-50 to-blue-100 dark:from-blue-900/20 dark:to-blue-800/20 border-blue-200 dark:border-blue-700">
        <div class="flex items-center justify-between">
          <div>
            <div class="text-2xl font-bold text-blue-600 dark:text-blue-400">{{ domains.length }}</div>
            <div class="text-sm text-blue-600/70">{{ t('em.navDomains') }}</div>
          </div>
          <BaseIcon :path="mdiDomain" size="36" class="text-blue-500 opacity-20" />
        </div>
      </CardBox>

      <CardBox class="bg-gradient-to-br from-green-50 to-green-100 dark:from-green-900/20 dark:to-green-800/20 border-green-200 dark:border-green-700">
        <div class="flex items-center justify-between">
          <div>
            <div class="text-2xl font-bold text-green-600 dark:text-green-400">{{ mailboxes.length }}</div>
            <div class="text-sm text-green-600/70">{{ t('em.navAccounts') }}</div>
          </div>
          <BaseIcon :path="mdiAccount" size="36" class="text-green-500 opacity-20" />
        </div>
      </CardBox>

      <CardBox class="bg-gradient-to-br from-purple-50 to-purple-100 dark:from-purple-900/20 dark:to-purple-800/20 border-purple-200 dark:border-purple-700">
        <div class="flex items-center justify-between">
          <div>
            <div class="text-2xl font-bold text-purple-600 dark:text-purple-400">
              {{ logStats.incoming }} / {{ logStats.outgoing }}
            </div>
            <div class="text-sm text-purple-600/70">{{ t('em.overviewTraffic') }}</div>
          </div>
          <BaseIcon :path="mdiEmailOpen" size="36" class="text-purple-500 opacity-20" />
        </div>
      </CardBox>

      <CardBox class="bg-gradient-to-br from-orange-50 to-orange-100 dark:from-orange-900/20 dark:to-orange-800/20 border-orange-200 dark:border-orange-700">
        <div class="flex items-center justify-between">
          <div>
            <div class="text-2xl font-bold text-orange-600 dark:text-orange-400">{{ queueItems.length }}</div>
            <div class="text-sm text-orange-600/70">{{ t('em.navQueue') }}</div>
          </div>
          <BaseIcon :path="mdiSend" size="36" class="text-orange-500 opacity-20" />
        </div>
      </CardBox>
    </div>

    <!-- Tabs -->
    <div class="overflow-x-auto pb-px -mx-1 px-1">
      <div class="flex flex-nowrap gap-1 sm:gap-2 border-b border-gray-200 dark:border-gray-700">
        <button
          v-for="tab in mailTabs"
          :key="tab"
          :class="[
            'shrink-0 whitespace-nowrap px-4 sm:px-6 py-3 font-medium text-sm border-b-2 transition-colors',
            activeTab === tab
              ? 'border-blue-500 text-blue-600 dark:text-blue-400'
              : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'
          ]"
          @click="activeTab = tab"
        >
          {{ t('em.tab_' + tab) }}
        </button>
      </div>
    </div>

    <!-- Overview Tab -->
    <div v-if="activeTab === 'overview'" class="grid grid-cols-1 lg:grid-cols-2 gap-6">
      <!-- What the server is serving right now -->
      <CardBox>
        <div class="flex items-center justify-between mb-4">
          <h3 class="text-lg font-semibold">{{ t('em.serverInformation') }}</h3>
          <BaseButton :icon="mdiRefresh" color="info" small @click="loadEngine(); loadServerStatus()" />
        </div>

        <div class="grid grid-cols-1 md:grid-cols-3 gap-4 mb-5 text-sm">
          <div>
            <span class="text-gray-500 block text-xs">{{ t('em.hostname') }}</span>
            <span class="font-medium">{{ serverStatus.hostname || t('em.na') }}</span>
          </div>
          <div>
            <span class="text-gray-500 block text-xs">{{ t('em.publicIpAddress') }}</span>
            <span class="font-medium font-mono">{{ serverStatus.ip_address || t('em.autoDetecting') }}</span>
          </div>
          <div>
            <span class="text-gray-500 block text-xs">{{ t('em.engineCert') }}</span>
            <span class="font-medium">{{ nativeStatus.cert_source || '-' }}</span>
          </div>
        </div>

        <div v-if="nativeStatus.listeners?.length" class="flex flex-wrap gap-2">
          <span
            v-for="listener in nativeStatus.listeners"
            :key="listener.name"
            class="px-3 py-1 rounded-full text-xs font-mono"
            :class="tlsBadge(listener.tls)"
          >
            {{ listener.name }}:{{ listener.port }}
          </span>
        </div>
        <p v-else class="text-sm text-gray-500">{{ t('em.engineNoListeners') }}</p>
      </CardBox>

      <!-- Health checks -->
      <CardBox v-if="selfTest.length">
        <h3 class="text-lg font-semibold mb-3">{{ t('em.engineChecks') }}</h3>
        <div
          v-for="(finding, i) in selfTest"
          :key="i"
          class="text-sm px-3 py-2 rounded mb-1 last:mb-0"
          :class="finding === 'no problems found'
            ? 'bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400'
            : 'bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400'"
        >
          {{ finding }}
        </div>
      </CardBox>

      <!-- Server IP Configuration -->
      <CardBox class="mb-6">
        <div class="flex items-center justify-between mb-6">
          <div class="flex items-center gap-3">
            <BaseIcon :path="mdiCog" class="text-blue-600" w="w-6" h="h-6" />
            <h3 class="text-xl font-semibold">{{ t('em.serverConfiguration') }}</h3>
          </div>
          <BaseButton
            v-if="!isEditingIP"
            :icon="mdiPencil"
            color="info"
            small
            :label="t('em.editIp')"
            @click="isEditingIP = true"
          />
        </div>

        <div class="space-y-4">
          <!-- Current IP Display -->
          <div v-if="!isEditingIP" class="p-4 bg-gray-50 dark:bg-gray-800 rounded-lg">
            <div class="flex items-center justify-between">
              <div>
                <p class="text-sm text-gray-500 dark:text-gray-400 mb-1">{{ t('em.publicIpAddress') }}</p>
                <p class="text-lg font-mono font-semibold">
                  {{ serverStatus.ip_address || t('em.autoDetecting') }}
                </p>
                <p v-if="serverStatus.ip_address" class="text-xs text-gray-500 mt-2">
                  {{ t('em.ipSpfHint') }}
                </p>
              </div>
              <div v-if="serverStatus.ip_address" class="text-right">
                <span class="inline-flex items-center px-3 py-1 rounded-full bg-green-100 text-green-800 dark:bg-green-900/20 dark:text-green-400 text-sm font-medium">
                  {{ t('em.configured') }}
                </span>
              </div>
            </div>
          </div>

          <!-- IP Edit Form -->
          <div v-else class="p-4 bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg">
            <FormField :label="t('em.publicIpAddress')" :help="t('em.publicIpHelp')">
              <FormControl
                v-model="serverIPForm.ip_address"
                type="text"
                :placeholder="t('em.ipPlaceholder')"
                :icon="mdiCog"
              />
            </FormField>
            <div class="flex gap-2 mt-4">
              <BaseButton
                color="success"
                :label="t('em.saveIp')"
                :disabled="!serverIPForm.ip_address"
                @click="updateServerIP"
              />
              <BaseButton
                color="danger"
                outline
                :label="t('common.cancel')"
                @click="isEditingIP = false; serverIPForm.ip_address = serverStatus.ip_address"
              />
            </div>
            <p class="text-xs text-blue-700 dark:text-blue-300 mt-3" v-html="t('em.autoDetectionNote')"></p>
          </div>
        </div>
      </CardBox>

      <!-- Cloudflare Integration Status -->
      <CardBox>
        <div class="flex items-center justify-between mb-6">
          <div class="flex items-center gap-3">
            <BaseIcon :path="mdiCloud" class="text-blue-600" w="w-6" h="h-6" />
            <h3 class="text-xl font-semibold">{{ t('em.cloudflareIntegration') }}</h3>
          </div>
        </div>

        <div class="space-y-4">
          <div class="p-4 bg-gradient-to-r from-blue-50 to-cyan-50 dark:from-blue-900/20 dark:to-cyan-900/20 border border-blue-200 dark:border-blue-800 rounded-lg">
            <div class="flex items-start gap-4">
              <BaseIcon :path="mdiInformationOutline" class="text-blue-600 dark:text-blue-400 flex-shrink-0 mt-1" w="w-6" h="h-6" />
              <div class="flex-1">
                <h4 class="font-semibold text-blue-900 dark:text-blue-100 mb-2">
                  {{ t('em.autoDnsConfig') }}
                </h4>
                <p class="text-sm text-blue-700 dark:text-blue-300 mb-3">
                  {{ t('em.autoDnsDesc') }}
                </p>
                <div class="space-y-2 text-sm text-blue-800 dark:text-blue-200">
                  <div class="flex items-center gap-2">
                    <span class="font-mono bg-blue-100 dark:bg-blue-800 px-2 py-0.5 rounded">SPF</span>
                    <span>{{ t('em.spfDesc') }}</span>
                  </div>
                  <div class="flex items-center gap-2">
                    <span class="font-mono bg-blue-100 dark:bg-blue-800 px-2 py-0.5 rounded">DKIM</span>
                    <span>{{ t('em.dkimDesc') }}</span>
                  </div>
                  <div class="flex items-center gap-2">
                    <span class="font-mono bg-blue-100 dark:bg-blue-800 px-2 py-0.5 rounded">DMARC</span>
                    <span>{{ t('em.dmarcDesc') }}</span>
                  </div>
                  <div class="flex items-center gap-2">
                    <span class="font-mono bg-blue-100 dark:bg-blue-800 px-2 py-0.5 rounded">MX</span>
                    <span>{{ t('em.mxDesc') }}</span>
                  </div>
                </div>
                <div class="mt-4 pt-4 border-t border-blue-200 dark:border-blue-700">
                  <p class="text-sm text-blue-700 dark:text-blue-300">
                    💡 <strong>{{ t('em.setupBold') }}</strong> {{ t('em.setupText1') }}
                    <router-link to="/cloudflare" class="underline hover:text-blue-900 dark:hover:text-blue-100">
                      {{ t('em.cfSettingsLink') }}
                    </router-link>
                    {{ t('em.setupText2') }}
                  </p>
                </div>
              </div>
            </div>
          </div>
        </div>
      </CardBox>
    </div>

    <!-- Domains Tab -->
    <div v-if="activeTab === 'domains'">
      <CardBox class="mb-6">
        <div class="flex items-center justify-between mb-6">
          <h3 class="text-xl font-semibold">{{ t('em.emailDomains') }}</h3>
          <BaseButton
            :icon="mdiPlus"
            color="success"
            :label="t('em.addDomain')"
            @click="isAddDomainModalActive = true"
          />
        </div>

        <div v-if="domains.length === 0" class="text-center py-12 text-gray-500">
          {{ t('em.noDomains') }}
        </div>

        <div v-else class="space-y-4">
          <div
            v-for="domain in domains"
            :key="domain.id"
            class="p-4 border border-gray-200 dark:border-gray-700 rounded-lg hover:border-blue-500 transition-colors"
          >
            <div class="flex items-center justify-between">
              <div class="flex-1">
                <h4 class="text-lg font-semibold">{{ domain.domain }}</h4>
                <p v-if="domain.description" class="text-sm text-gray-500">{{ domain.description }}</p>
                <p class="text-sm text-gray-500">
                  {{ t('em.domainMailboxCount', { n: mailboxCount(domain.id) }) }}
                </p>
              </div>
              <div class="flex items-center gap-3">
                <span
:class="[
                  'px-3 py-1 rounded-full text-sm font-medium',
                  domain.enabled ? 'bg-green-100 text-green-800' : 'bg-red-100 text-red-800'
                ]">
                  {{ domain.enabled ? t('em.active') : t('em.disabled') }}
                </span>
                <span
                  v-if="domain.dns_configured"
                  class="px-2 py-1 rounded-full text-xs bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-400"
                >
                  {{ t('em.dnsPublished') }}
                </span>
                <BaseButton
                  :icon="mdiCloudUpload"
                  color="info"
                  small
                  :label="t('em.dnsSync')"
                  :disabled="dnsSyncing"
                  @click="syncDns(domain.id)"
                />
                <BaseButton
                  :icon="mdiEmailPlus"
                  color="success"
                  small
                  :label="t('em.addMailboxHere')"
                  @click="openAddMailboxForDomain(domain)"
                />
                <BaseButton
                  :icon="mdiPencil"
                  color="info"
                  small
                  :label="t('common.edit')"
                  @click="openEditDomainModal(domain)"
                />
                <BaseButton
                  :icon="mdiDelete"
                  color="danger"
                  small
                  @click="deleteDomain(domain.id)"
                />
              </div>
            </div>
          </div>
        </div>
      </CardBox>
    </div>

    <!-- Mailboxes Tab -->
    <div v-if="activeTab === 'mailboxes'">
      <CardBox class="mb-6">
        <div class="flex items-center justify-between mb-6">
          <h3 class="text-xl font-semibold">{{ t('em.mailboxes') }}</h3>
          <BaseButton
            :icon="mdiEmailPlus"
            color="success"
            :label="t('em.createMailbox')"
            :disabled="domains.length === 0"
            @click="openAddMailboxModal"
          />
        </div>
        
        <div v-if="domains.length === 0" class="mb-4 p-4 bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-lg">
          <p class="text-yellow-800 dark:text-yellow-200">
            {{ t('em.addDomainFirstWarn') }}
          </p>
        </div>

        <!-- A mailbox can lose its password without anyone noticing until a
             client fails to log in; say so here instead. -->
        <div
          v-if="passwordCheck.unusable"
          class="mb-4 rounded-lg border border-red-300 bg-red-50 px-4 py-3 dark:border-red-800 dark:bg-red-900/20"
        >
          <p class="text-sm font-medium text-red-800 dark:text-red-200">
            {{ t('em.pwdBrokenTitle', { n: passwordCheck.unusable }) }}
          </p>
          <p class="mt-1 text-xs text-red-700 dark:text-red-300">{{ t('em.pwdBrokenHint') }}</p>
          <ul class="mt-2 space-y-0.5 text-xs">
            <li v-for="entry in unusableMailboxes" :key="entry.id" class="font-mono">{{ entry.email }}</li>
          </ul>
        </div>
        <div
          v-else-if="passwordCheck.repairable"
          class="mb-4 rounded-lg border border-amber-300 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-200"
        >
          {{ t('em.pwdRepairable', { n: passwordCheck.repairable }) }}
        </div>

        <div v-if="mailboxes.length === 0" class="text-center py-12 text-gray-500">
          {{ t('em.noMailboxes') }}
        </div>

        <div v-else class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          <div
            v-for="mailbox in mailboxes"
            :key="mailbox.id"
            class="p-4 border border-gray-200 dark:border-gray-700 rounded-lg hover:border-blue-500 transition-colors"
          >
            <div class="flex items-start justify-between mb-3">
              <div class="flex items-center space-x-3">
                <div class="w-12 h-12 rounded-full bg-gradient-to-br from-blue-500 to-purple-600 flex items-center justify-center text-white font-bold">
                  {{ getInitials(mailbox.name || mailbox.username) }}
                </div>
                <div>
                  <h4 class="font-semibold">{{ mailbox.name || mailbox.username }}</h4>
                  <p class="text-sm text-gray-500">{{ mailbox.email }}</p>
                </div>
              </div>
              <div class="flex gap-2">
                <BaseButton
                  :icon="mdiCog"
                  color="light"
                  small
                  :title="t('em.connTitle')"
                  @click="openConnectionSettings(mailbox)"
                />
                <BaseButton
                  :icon="mdiPencil"
                  color="info"
                  small
                  @click="openEditMailboxModal(mailbox)"
                />
                <BaseButton
                  :icon="mdiKey"
                  color="warning"
                  small
                  @click="openUpdatePasswordModal(mailbox)"
                />
                <BaseButton
                  :icon="mdiDelete"
                  color="danger"
                  small
                  @click="deleteMailbox(mailbox.id)"
                />
              </div>
            </div>
            <div class="text-sm text-gray-600 dark:text-gray-400">
              <p>{{ t('em.messagesCount', { n: mailbox.message_count || 0 }) }}</p>
              <p>{{ t('em.lastLogin', { value: formatDate(mailbox.last_login) || t('em.never') }) }}</p>
            </div>
          </div>
        </div>
      </CardBox>
    </div>

    <!-- Aliases live with the accounts they point at -->
    <div v-if="activeTab === 'mailboxes'" class="mt-6">
      <CardBox>
        <div class="flex items-center justify-between mb-1">
          <h3 class="text-lg font-semibold">{{ t('em.aliasTitle') }}</h3>
        </div>
        <p class="text-sm text-gray-500 mb-4">{{ t('em.aliasHint') }}</p>

        <div class="flex flex-wrap gap-2 mb-4">
          <input
            v-model="newAlias.alias"
            type="text"
            :placeholder="t('em.aliasAddress')"
            class="flex-1 min-w-[200px] px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-slate-800"
          />
          <select
            v-model="newAlias.destination"
            class="flex-1 min-w-[200px] px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-slate-800"
          >
            <option value="">{{ t('em.aliasDestination') }}</option>
            <option v-for="mailbox in mailboxes" :key="mailbox.id" :value="mailbox.email">
              {{ mailbox.email }}
            </option>
          </select>
          <BaseButton :icon="mdiPlus" :label="t('em.aliasAdd')" color="success" :disabled="aliasBusy" @click="addAlias" />
        </div>

        <div v-if="aliases.length" class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead class="text-left text-gray-500">
              <tr>
                <th class="py-2">{{ t('em.aliasAddress') }}</th>
                <th class="py-2">{{ t('em.aliasDestination') }}</th>
                <th class="py-2">{{ t('em.status') }}</th>
                <th class="py-2"></th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="alias in aliases" :key="alias.id" class="border-t border-gray-200 dark:border-gray-700">
                <td class="py-2 font-mono text-xs">{{ alias.alias }}</td>
                <td class="py-2 font-mono text-xs">{{ alias.destination }}</td>
                <td class="py-2">
                  <button
                    class="px-2 py-1 rounded-full text-xs"
                    :class="alias.enabled
                      ? 'bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400'
                      : 'bg-gray-200 dark:bg-slate-700 text-gray-600 dark:text-gray-300'"
                    @click="toggleAlias(alias)"
                  >
                    {{ alias.enabled ? t('em.active') : t('em.disabled') }}
                  </button>
                </td>
                <td class="py-2 text-right">
                  <BaseButton :icon="mdiDelete" color="danger" small @click="deleteAlias(alias)" />
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <p v-else class="text-sm text-gray-500">{{ t('em.aliasEmpty') }}</p>
      </CardBox>
    </div>

    <!-- Webmail -->
    <div v-if="activeTab === 'webmail'">
      <WebmailPanel :mailboxes="mailboxes" />
    </div>


    <!-- Filter rules -->
    <div v-if="activeTab === 'filters'">
      <FilterRules :mailboxes="mailboxes" />
    </div>

    <!-- Logs Tab - mail traffic in / out -->
    <div v-if="activeTab === 'logs'">
      <!-- Counters for the scanned window -->
      <div class="grid grid-cols-2 md:grid-cols-5 gap-4 mb-6">
        <CardBox>
          <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('em.logIncoming') }}</p>
          <p class="text-2xl font-semibold mt-1 text-emerald-500">{{ logStats.incoming }}</p>
        </CardBox>
        <CardBox>
          <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('em.logOutgoing') }}</p>
          <p class="text-2xl font-semibold mt-1 text-blue-500">{{ logStats.outgoing }}</p>
        </CardBox>
        <CardBox>
          <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('em.logRejected') }}</p>
          <p class="text-2xl font-semibold mt-1 text-red-500">{{ logStats.rejected }}</p>
        </CardBox>
        <CardBox>
          <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('em.logDeferred') }}</p>
          <p class="text-2xl font-semibold mt-1 text-amber-500">{{ logStats.deferred }}</p>
        </CardBox>
        <CardBox>
          <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('em.logBounced') }}</p>
          <p class="text-2xl font-semibold mt-1 text-red-500">{{ logStats.bounced }}</p>
        </CardBox>
      </div>

      <CardBox>
        <!-- Toolbar -->
        <div class="flex flex-wrap items-center gap-3 mb-4">
          <select
            v-model="logDirection"
            class="px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-slate-800 text-sm"
          >
            <option value="">{{ t('em.logAllDirections') }}</option>
            <option value="in">{{ t('em.logIncoming') }}</option>
            <option value="out">{{ t('em.logOutgoing') }}</option>
            <option value="system">{{ t('em.logSystem') }}</option>
          </select>

          <select
            v-model="logStatus"
            class="px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-slate-800 text-sm"
          >
            <option value="">{{ t('em.logAllStatuses') }}</option>
            <option value="sent">sent</option>
            <option value="delivered">delivered</option>
            <option value="deferred">deferred</option>
            <option value="bounced">bounced</option>
            <option value="rejected">rejected</option>
            <option value="login">login</option>
            <option value="auth-failed">auth-failed</option>
            <option value="connect">connect</option>
            <option value="disconnect">disconnect</option>
            <option value="tls-handshake">tls-handshake</option>
            <option value="conn-error">conn-error</option>
            <option value="error">error</option>
          </select>

          <input
            v-model="logSearch"
            type="text"
            :placeholder="t('em.logSearchPlaceholder')"
            class="px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-slate-800 text-sm flex-1 min-w-[200px]"
            @keyup.enter="loadLogs"
          />

          <select
            v-model.number="logTail"
            class="px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-slate-800 text-sm"
            @change="refreshLogs"
          >
            <option :value="200">200 {{ t('em.logLines') }}</option>
            <option :value="500">500 {{ t('em.logLines') }}</option>
            <option :value="1000">1000 {{ t('em.logLines') }}</option>
            <option :value="2000">2000 {{ t('em.logLines') }}</option>
          </select>

          <label class="flex items-center gap-2 text-sm text-gray-500">
            <input v-model="logAutoRefresh" type="checkbox" />
            {{ t('em.logAutoRefresh') }}
          </label>

          <div class="flex rounded-lg overflow-hidden border border-gray-300 dark:border-gray-700 text-sm">
            <button
              v-for="view in ['events', 'connections', 'raw']"
              :key="view"
              :class="[
                'px-3 py-2 transition-colors',
                logView === view
                  ? 'bg-blue-500 text-white'
                  : 'bg-white dark:bg-slate-800 text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-slate-700'
              ]"
              @click="logView = view"
            >
              {{ t('em.logView_' + view) }}
            </button>
          </div>

          <BaseButton :icon="mdiRefresh" color="info" small :disabled="logsLoading" @click="refreshLogs" />
        </div>

        <p v-if="logSource" class="text-xs text-gray-500 mb-3">
          {{ t('em.logSource', { source: logSource, tail: logTail }) }}
          <span v-if="logView === 'events' && logTotal">
            — {{ t('em.logShowing', { shown: logEntries.length, total: logTotal }) }}
          </span>
        </p>

        <div v-if="logError" class="p-4 rounded-lg bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-400 text-sm">
          {{ logError }}
        </div>

        <!-- Raw view -->
        <pre
          v-else-if="logView === 'raw'"
          class="text-xs font-mono bg-gray-50 dark:bg-slate-900 p-3 rounded-lg overflow-auto max-h-[60vh] whitespace-pre-wrap"
        >{{ rawLogLines.join('\n') }}</pre>

        <!-- Parsed view -->
        <div v-else-if="logView === 'events' && logEntries.length" class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead class="text-left text-gray-500 dark:text-gray-400">
              <tr>
                <th class="py-2 pr-3">{{ t('em.logTime') }}</th>
                <th class="py-2 pr-3">{{ t('em.logDirection') }}</th>
                <th class="py-2 pr-3">{{ t('em.logFrom') }}</th>
                <th class="py-2 pr-3">{{ t('em.logTo') }}</th>
                <th class="py-2 pr-3">{{ t('em.logStatus') }}</th>
                <th class="py-2">{{ t('em.logDetail') }}</th>
              </tr>
            </thead>
            <tbody>
              <template v-for="entry in logEntries" :key="entry.id">
                <tr
                  class="border-t border-gray-200 dark:border-gray-700 hover:bg-gray-50 dark:hover:bg-slate-800/50 cursor-pointer"
                  @click="toggleLogDetail(entry.id)"
                >
                  <td class="py-2 pr-3 whitespace-nowrap text-xs text-gray-500">{{ formatLogTime(entry.timestamp) }}</td>
                  <td class="py-2 pr-3 whitespace-nowrap">
                    <span class="inline-flex items-center gap-1" :class="directionMeta(entry.direction).cls">
                      <BaseIcon :path="directionMeta(entry.direction).icon" size="16" />
                      <span class="text-xs">{{ directionMeta(entry.direction).label }}</span>
                    </span>
                  </td>
                  <td class="py-2 pr-3 max-w-[220px] truncate">{{ entry.from || '-' }}</td>
                  <td class="py-2 pr-3 max-w-[220px] truncate">{{ (entry.to || []).join(', ') || '-' }}</td>
                  <td class="py-2 pr-3 whitespace-nowrap">
                    <span class="px-2 py-1 rounded-full text-xs font-medium" :class="statusClass(entry.status)">
                      {{ entry.status }}
                    </span>
                  </td>
                  <td class="py-2 text-xs text-gray-500 max-w-[320px] truncate">
                    <span
                      v-if="entry.smtp_code"
                      class="font-mono mr-1 px-1.5 rounded"
                      :class="entry.smtp_code >= 500
                        ? 'bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-400'
                        : entry.smtp_code >= 400
                          ? 'bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400'
                          : 'bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400'"
                    >{{ entry.smtp_code }}</span>
                    {{ entry.detail }}
                  </td>
                </tr>
                <tr v-if="expandedLogIds.has(entry.id)" class="bg-gray-50 dark:bg-slate-900/60">
                  <td colspan="6" class="p-4">
                    <div class="grid grid-cols-2 md:grid-cols-4 gap-3 text-xs mb-3">
                      <div>
                        <span class="text-gray-500 block">{{ t('em.logService') }}</span>
                        <span class="font-mono">{{ entry.service }}</span>
                      </div>
                      <div v-if="entry.queue_id">
                        <span class="text-gray-500 block">{{ t('em.logQueueId') }}</span>
                        <span class="font-mono">{{ entry.queue_id }}</span>
                      </div>
                      <div v-if="entry.smtp_code">
                        <span class="text-gray-500 block">{{ t('em.logSmtpCode') }}</span>
                        <span class="font-mono">{{ entry.smtp_code }}</span>
                      </div>
                      <div v-if="entry.remote_ip">
                        <span class="text-gray-500 block">{{ t('em.logRemote') }}</span>
                        <span class="font-mono">{{ entry.remote_host }} {{ entry.remote_ip }}</span>
                      </div>
                      <div v-if="entry.size">
                        <span class="text-gray-500 block">{{ t('em.logSize') }}</span>
                        <span>{{ formatLogSize(entry.size) }}</span>
                      </div>
                      <div v-if="entry.message_id" class="col-span-2 md:col-span-4">
                        <span class="text-gray-500 block">Message-ID</span>
                        <span class="font-mono break-all">{{ entry.message_id }}</span>
                      </div>
                    </div>
                    <pre
                      v-if="entry.raw && entry.raw.length"
                      class="text-[11px] font-mono bg-white dark:bg-slate-950 p-3 rounded overflow-x-auto"
                    >{{ entry.raw.join('\n') }}</pre>
                  </td>
                </tr>
              </template>
            </tbody>
          </table>
        </div>

        <!-- Connections: every accepted connection with its protocol trace -->
        <div v-else-if="logView === 'connections' && connections.length" class="space-y-2">
          <div
            v-for="conn in connections"
            :key="conn.id"
            class="border border-gray-200 dark:border-gray-700 rounded-lg overflow-hidden"
          >
            <button
              class="w-full flex flex-wrap items-center gap-3 px-3 py-2 text-left text-sm hover:bg-gray-50 dark:hover:bg-slate-800/50"
              @click="toggleConnection(conn.id)"
            >
              <span class="text-xs text-gray-500 w-36 shrink-0">{{ formatLogTime(conn.started_at) }}</span>
              <span class="font-mono text-xs px-2 py-0.5 rounded-full" :class="tlsBadge(conn.tls)">
                {{ conn.service }}
              </span>
              <span class="font-mono text-xs">{{ conn.remote_ip }}<span class="text-gray-400">:{{ conn.remote_port }}</span></span>
              <span
                class="text-xs px-2 py-0.5 rounded-full"
                :class="conn.encrypted
                  ? 'bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400'
                  : 'bg-gray-200 dark:bg-slate-700 text-gray-600 dark:text-gray-300'"
              >
                {{ conn.encrypted ? t('em.connEncrypted') : t('em.connPlaintext') }}
              </span>
              <span class="text-xs text-gray-500">{{ connectionDuration(conn) }}</span>
              <span v-if="conn.error" class="text-xs text-red-500 truncate flex-1">{{ conn.error }}</span>
              <span v-else class="text-xs text-gray-400 flex-1">{{ conn.lines?.length || 0 }} {{ t('em.connLines') }}</span>
              <span v-if="!conn.ended_at" class="text-xs text-blue-500">{{ t('em.connOpen') }}</span>
            </button>

            <div v-if="expandedConnIds.has(conn.id)" class="bg-gray-50 dark:bg-slate-900/60 px-3 py-2">
              <div v-if="conn.lines?.length" class="font-mono text-[11px] space-y-0.5 max-h-80 overflow-y-auto">
                <div v-for="(line, i) in conn.lines" :key="i" class="flex gap-2">
                  <span class="w-16 shrink-0 text-gray-400">{{ formatTraceTime(line.timestamp) }}</span>
                  <span
                    class="w-6 shrink-0"
                    :class="line.direction === 'in' ? 'text-blue-500' : line.direction === 'out' ? 'text-emerald-500' : 'text-amber-500'"
                  >
                    {{ line.direction === 'in' ? '→' : line.direction === 'out' ? '←' : '·' }}
                  </span>
                  <span class="break-all whitespace-pre-wrap">{{ line.text }}</span>
                </div>
              </div>
              <p v-else class="text-xs text-gray-500">{{ t('em.connNoLines') }}</p>
              <p v-if="conn.truncated" class="text-xs text-amber-500 mt-1">{{ t('em.connTruncated') }}</p>
            </div>
          </div>
        </div>

        <div v-else class="text-center py-12 text-gray-500">
          <BaseIcon :path="mdiInbox" size="48" class="mx-auto mb-3 opacity-40" />
          <p>{{ logsLoading ? t('em.logLoading') : t('em.logEmpty') }}</p>
        </div>
      </CardBox>
    </div>

    <!-- Listeners & TLS -->
    <div v-if="activeTab === 'listeners'">
      <CardBox class="mb-6">
        <div class="flex items-center justify-between mb-4">
          <div class="flex items-center gap-3">
            <BaseIcon :path="mdiServer" class="text-blue-500" w="w-6" h="h-6" />
            <div>
            <h3 class="text-lg font-semibold">{{ t('em.engineListeners') }}</h3>
            <p class="text-sm text-gray-500">{{ t('em.listenersHint') }}</p>
            </div>
          </div>
          <BaseButton :icon="mdiRefresh" color="info" small :disabled="engineLoading" @click="loadEngine" />
        </div>

        <div v-if="nativeStatus.listeners?.length" class="grid grid-cols-2 md:grid-cols-4 gap-3">
          <div
            v-for="listener in nativeStatus.listeners"
            :key="listener.name"
            class="p-3 rounded-lg bg-gray-50 dark:bg-slate-800"
          >
            <div class="font-mono text-sm font-semibold uppercase">{{ listener.name }}</div>
            <div class="text-2xl font-semibold">{{ listener.port }}</div>
            <span class="text-xs px-2 py-0.5 rounded-full" :class="tlsBadge(listener.tls)">{{ listener.tls }}</span>
          </div>
        </div>
        <p v-else class="text-sm text-gray-500">{{ t('em.engineNoListeners') }}</p>

        <div class="grid grid-cols-1 md:grid-cols-3 gap-4 mt-5 text-sm">
          <div>
            <span class="text-gray-500 block text-xs">{{ t('em.engineCert') }}</span>
            <span class="font-medium">{{ nativeStatus.cert_source || '-' }}</span>
          </div>
          <div>
            <span class="text-gray-500 block text-xs">{{ t('em.engineCertExpires') }}</span>
            <span class="font-medium">{{ formatQueueTime(nativeStatus.cert_expires) }}</span>
          </div>
          <div>
            <span class="text-gray-500 block text-xs">{{ t('em.engineMailRoot') }}</span>
            <span class="font-mono text-xs break-all">{{ nativeStatus.mail_root }}</span>
          </div>
        </div>
      </CardBox>

      <!-- TLS certificate -->
      <CardBox v-if="certificate" class="mb-6">
        <div class="flex flex-wrap items-start justify-between gap-3 mb-4">
          <div>
            <h3 class="text-lg font-semibold">{{ t('em.certTitle') }}</h3>
            <p class="text-sm text-gray-500">{{ t('em.certHint') }}</p>
          </div>
          <BaseButton
            :icon="mdiKey"
            :label="t('em.certRequest')"
            color="success"
            small
            :disabled="requestingCert || !certificate.acme_ready"
            @click="requestCertificate"
          />
        </div>

        <div
          v-if="!certificate.acme_ready && certificate.acme_reason"
          class="mb-4 px-3 py-2 rounded text-sm bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400"
        >
          {{ certificate.acme_reason }}
        </div>

        <div class="grid grid-cols-1 md:grid-cols-4 gap-4 text-sm mb-4">
          <div>
            <span class="text-gray-500 block text-xs">{{ t('em.certSource') }}</span>
            <span class="font-medium">{{ certificate.source }}</span>
            <span
              v-if="certificate.self_signed"
              class="ml-1 text-xs px-2 py-0.5 rounded-full bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400"
            >
              {{ t('em.certSelfSigned') }}
            </span>
          </div>
          <div>
            <span class="text-gray-500 block text-xs">{{ t('em.certIssuer') }}</span>
            <span class="font-medium">{{ certificate.issuer || '-' }}</span>
          </div>
          <div>
            <span class="text-gray-500 block text-xs">{{ t('em.certExpiry') }}</span>
            <span class="font-medium" :class="certificate.days_left < 15 ? 'text-red-500' : ''">
              {{ formatQueueTime(certificate.not_after) }}
              <span class="text-gray-500">({{ certificate.days_left }} {{ t('em.certDays') }})</span>
            </span>
          </div>
          <div>
            <span class="text-gray-500 block text-xs">{{ t('em.certPath') }}</span>
            <span class="font-mono text-xs break-all">{{ certificate.cert_path || t('em.certSelfPath') }}</span>
          </div>
        </div>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-4 text-sm">
          <div>
            <span class="text-gray-500 block text-xs mb-1">{{ t('em.certNames') }}</span>
            <div class="flex flex-wrap gap-1">
              <span
                v-for="name in certificate.names"
                :key="name"
                class="text-xs font-mono px-2 py-0.5 rounded-full bg-gray-100 dark:bg-slate-800"
              >{{ name }}</span>
              <span
                v-for="ip in certificate.ips"
                :key="ip"
                class="text-xs font-mono px-2 py-0.5 rounded-full bg-gray-100 dark:bg-slate-800"
              >{{ ip }}</span>
            </div>
          </div>
          <div v-if="certificate.name_checks?.length">
            <span class="text-gray-500 block text-xs mb-1">{{ t('em.certNameChecks') }}</span>
            <div class="space-y-1">
              <div
                v-for="check in certificate.name_checks"
                :key="check.name"
                class="flex items-start gap-2 text-xs"
              >
                <span :class="check.points_at_us ? 'text-emerald-500' : 'text-amber-500'">
                  {{ check.points_at_us ? '✓' : '!' }}
                </span>
                <span class="font-mono">{{ check.name }}</span>
                <span v-if="!check.points_at_us" class="text-gray-500">— {{ check.reason }}</span>
              </div>
            </div>
            <p class="text-xs text-gray-500 mt-1">{{ t('em.certNameChecksHint') }}</p>
          </div>

          <div v-if="certificate.missing?.length">
            <span class="text-gray-500 block text-xs mb-1">{{ t('em.certMissing') }}</span>
            <div class="flex flex-wrap gap-1">
              <span
                v-for="name in certificate.missing"
                :key="name"
                class="text-xs font-mono px-2 py-0.5 rounded-full bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-400"
              >{{ name }}</span>
            </div>
            <p class="text-xs text-gray-500 mt-1">{{ t('em.certMissingHint') }}</p>
          </div>
        </div>
      </CardBox>

      <CardBox v-if="nativeConfig">
        <h3 class="text-lg font-semibold mb-4">{{ t('em.engineSettings') }}</h3>

        <!-- Identity: this is the name clients dial and the name a certificate
             can be issued for, so it comes first. -->
        <div class="grid grid-cols-1 md:grid-cols-2 gap-4 mb-5">
          <label class="block">
            <span class="text-xs text-gray-500">{{ t('em.settingsHostname') }}</span>
            <input
              v-model="nativeConfig.hostname"
              type="text"
              placeholder="mail.example.com"
              class="w-full mt-1 px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-slate-800"
            />
            <span class="text-xs text-gray-500">{{ t('em.settingsHostnameHint') }}</span>
          </label>
          <label class="block">
            <span class="text-xs text-gray-500">{{ t('em.publicIpAddress') }}</span>
            <input
              v-model="nativeConfig.ip_address"
              type="text"
              placeholder="203.0.113.10"
              class="w-full mt-1 px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-slate-800"
            />
            <span class="text-xs text-gray-500">{{ t('em.settingsIpHint') }}</span>
          </label>
        </div>

        <div class="grid grid-cols-2 md:grid-cols-4 gap-4 mb-5">
          <label class="block">
            <span class="text-xs text-gray-500">SMTP</span>
            <input v-model.number="nativeConfig.smtp_port" type="number" class="w-full mt-1 px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-slate-800" />
          </label>
          <label class="block">
            <span class="text-xs text-gray-500">Submission</span>
            <input v-model.number="nativeConfig.submission_port" type="number" class="w-full mt-1 px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-slate-800" />
          </label>
          <label class="block">
            <span class="text-xs text-gray-500">SMTPS</span>
            <input v-model.number="nativeConfig.smtps_port" type="number" class="w-full mt-1 px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-slate-800" />
          </label>
          <label class="block">
            <span class="text-xs text-gray-500">IMAP</span>
            <input v-model.number="nativeConfig.imap_port" type="number" class="w-full mt-1 px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-slate-800" />
          </label>
          <label class="block">
            <span class="text-xs text-gray-500">IMAPS</span>
            <input v-model.number="nativeConfig.imaps_port" type="number" class="w-full mt-1 px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-slate-800" />
          </label>
          <label class="block">
            <span class="text-xs text-gray-500">POP3</span>
            <input v-model.number="nativeConfig.pop3_port" type="number" class="w-full mt-1 px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-slate-800" />
          </label>
          <label class="block">
            <span class="text-xs text-gray-500">POP3S</span>
            <input v-model.number="nativeConfig.pop3s_port" type="number" class="w-full mt-1 px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-slate-800" />
          </label>
          <label class="block">
            <span class="text-xs text-gray-500">{{ t('em.optMaxAuthFailures') }}</span>
            <input v-model.number="nativeConfig.max_auth_failures" type="number" min="1" class="w-full mt-1 px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-slate-800" />
          </label>
          <label class="block">
            <span class="text-xs text-gray-500">{{ t('em.optMaxRelayAttempts') }}</span>
            <input v-model.number="nativeConfig.max_relay_attempts" type="number" min="1" class="w-full mt-1 px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-slate-800" />
          </label>
          <label class="block">
            <span class="text-xs text-gray-500">{{ t('em.optMaxConnections') }}</span>
            <input v-model.number="nativeConfig.max_connections_per_minute" type="number" min="1" class="w-full mt-1 px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-slate-800" />
          </label>
          <label class="block">
            <span class="text-xs text-gray-500">{{ t('em.optBlockMinutes') }}</span>
            <input v-model.number="nativeConfig.block_minutes" type="number" min="1" class="w-full mt-1 px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-slate-800" />
          </label>
          <label class="block">
            <span class="text-xs text-gray-500">HELO</span>
            <input v-model="nativeConfig.outbound_helo" type="text" :placeholder="nativeConfig.hostname" class="w-full mt-1 px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-slate-800" />
          </label>
        </div>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-2 mb-5 text-sm">
          <label class="flex items-center gap-2">
            <input v-model="nativeConfig.starttls_required" type="checkbox" />
            {{ t('em.optStartTLS') }}
          </label>
          <label class="flex items-center gap-2">
            <input v-model="nativeConfig.smtps_enabled" type="checkbox" />
            {{ t('em.optSMTPS') }}
          </label>
          <label class="flex items-center gap-2">
            <input v-model="nativeConfig.imap_enabled" type="checkbox" />
            {{ t('em.optIMAP') }}
          </label>
          <label class="flex items-center gap-2">
            <input v-model="nativeConfig.imaps_enabled" type="checkbox" />
            {{ t('em.optIMAPS') }}
          </label>
          <label class="flex items-center gap-2">
            <input v-model="nativeConfig.pop3_enabled" type="checkbox" />
            {{ t('em.optPOP3') }}
          </label>
          <label class="flex items-center gap-2">
            <input v-model="nativeConfig.pop3s_enabled" type="checkbox" />
            {{ t('em.optPOP3S') }}
          </label>
          <label class="flex items-center gap-2">
            <input v-model="nativeConfig.check_spf" type="checkbox" />
            {{ t('em.optSPF') }}
          </label>
          <label class="flex items-center gap-2">
            <input v-model="nativeConfig.check_dkim" type="checkbox" />
            {{ t('em.optDKIM') }}
          </label>
          <label class="flex items-center gap-2">
            <input v-model="nativeConfig.check_dmarc" type="checkbox" />
            {{ t('em.optDMARC') }}
          </label>
          <label class="flex items-center gap-2">
            <input v-model="nativeConfig.reject_on_dmarc_fail" type="checkbox" />
            {{ t('em.optDMARCReject') }}
          </label>
          <label class="flex items-center gap-2">
            <input v-model="nativeConfig.log_connections" type="checkbox" />
            {{ t('em.optLogConnections') }}
          </label>
          <label class="flex items-center gap-2">
            <input v-model="nativeConfig.guard_enabled" type="checkbox" />
            {{ t('em.optGuard') }}
          </label>
          <label class="flex items-center gap-2">
            <input v-model="nativeConfig.dnsbl_enabled" type="checkbox" />
            {{ t('em.optDNSBL') }}
          </label>
          <label class="flex items-center gap-2">
            <input v-model="nativeConfig.dnsbl_reject" type="checkbox" :disabled="!nativeConfig.dnsbl_enabled" />
            {{ t('em.optDNSBLReject') }}
          </label>
        </div>

        <div v-if="nativeConfig.dnsbl_enabled" class="mb-4">
          <label class="text-sm">
            <span class="mb-1 block text-gray-500">{{ t('em.dnsblZones') }}</span>
            <input
              v-model="nativeConfig.dnsbl_zones"
              type="text"
              placeholder="zen.spamhaus.org, bl.spamcop.net"
              class="w-full rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-slate-800 px-3 py-2"
            />
          </label>
          <p class="mt-1 text-xs text-gray-500">{{ t('em.dnsblHint') }}</p>
        </div>

        <BaseButton :icon="mdiCog" :label="t('em.engineSave')" color="success" :disabled="engineSaving" @click="saveNativeSettings" />
      </CardBox>
    </div>

    <!-- Outbound queue -->
    <div v-if="activeTab === 'queue'">
      <CardBox>
        <div class="flex items-center justify-between mb-4">
          <div>
            <h3 class="text-lg font-semibold">{{ t('em.queueTitle') }}</h3>
            <p class="text-sm text-gray-500">{{ t('em.queueHint') }}</p>
          </div>
          <BaseButtons>
            <BaseButton :icon="mdiSend" :label="t('em.queueFlush')" color="info" small :disabled="!queueItems.length" @click="flushQueue" />
            <BaseButton :icon="mdiRefresh" color="info" small :disabled="engineLoading" @click="loadEngine" />
          </BaseButtons>
        </div>

        <div v-if="queueItems.length" class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead class="text-left text-gray-500">
              <tr>
                <th class="py-2 pr-3">{{ t('em.logFrom') }}</th>
                <th class="py-2 pr-3">{{ t('em.logTo') }}</th>
                <th class="py-2 pr-3">{{ t('em.logDetail') }}</th>
                <th class="py-2 pr-3">{{ t('em.queueAttempts') }}</th>
                <th class="py-2 pr-3">{{ t('em.queueNextTry') }}</th>
                <th class="py-2"></th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in queueItems" :key="item.id" class="border-t border-gray-200 dark:border-gray-700">
                <td class="py-2 pr-3 truncate max-w-[180px]">{{ item.from }}</td>
                <td class="py-2 pr-3 truncate max-w-[200px]">{{ (item.recipients || []).join(', ') }}</td>
                <td class="py-2 pr-3 text-xs text-gray-500 truncate max-w-[260px]">{{ item.last_error || item.subject }}</td>
                <td class="py-2 pr-3">{{ item.attempts }}</td>
                <td class="py-2 pr-3 text-xs">{{ formatQueueTime(item.next_try) }}</td>
                <td class="py-2 text-right">
                  <BaseButton :icon="mdiDelete" color="danger" small @click="deleteQueueItem(item.id)" />
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <p v-else class="text-sm text-gray-500">{{ t('em.queueEmpty') }}</p>
      </CardBox>
    </div>

    <!-- DNS -->
    <div v-if="activeTab === 'dns'">
      <!-- What a receiving server sees -->
      <CardBox class="mb-6">
        <div class="flex flex-wrap items-start justify-between gap-3 mb-4">
          <div>
            <h3 class="text-lg font-semibold">{{ t('em.deliverTitle') }}</h3>
            <p class="text-sm text-gray-500">{{ t('em.deliverHint') }}</p>
          </div>
          <div class="flex items-center gap-3">
            <span v-if="deliverability" class="text-sm font-medium">
              {{ deliverability.passed }} / {{ deliverability.total }}
            </span>
            <BaseButton
              :icon="mdiRefresh"
              :label="t('em.deliverRun')"
              color="info"
              small
              :disabled="checkingDeliverability"
              @click="runDeliverabilityCheck"
            />
          </div>
        </div>

        <div v-if="deliverability?.checks?.length" class="space-y-2">
          <div
            v-for="(check, i) in deliverability.checks"
            :key="i"
            class="p-3 rounded-lg"
            :class="checkLevelClass(check.level)"
          >
            <div class="flex flex-wrap items-center gap-2 text-sm font-medium">
              <span>{{ check.level === 'ok' ? '✓' : check.level === 'fail' ? '✕' : '!' }}</span>
              <span>{{ check.title }}</span>
              <span v-if="check.domain" class="font-mono text-xs opacity-70">{{ check.domain }}</span>
            </div>
            <p class="text-xs mt-1 opacity-90 break-all">{{ check.detail }}</p>
            <p v-if="check.advice" class="text-xs mt-1 opacity-80">→ {{ check.advice }}</p>
          </div>
        </div>
        <p v-else class="text-sm text-gray-500">
          {{ checkingDeliverability ? t('em.deliverRunning') : t('em.deliverEmpty') }}
        </p>
      </CardBox>

      <CardBox>
        <div class="flex flex-wrap items-start justify-between gap-3 mb-4">
          <div>
            <h3 class="text-lg font-semibold">{{ t('em.dnsTitle') }}</h3>
            <p class="text-sm text-gray-500">{{ t('em.dnsCloudflareHint') }}</p>
          </div>
          <BaseButton
            :icon="mdiCloudUpload"
            :label="t('em.dnsSyncAll')"
            color="info"
            small
            :disabled="dnsSyncing || !dnsRecords.length"
            @click="syncDns(0)"
          />
        </div>

        <!-- What the zone holds today, checked without changing anything -->
        <div v-if="dnsPreview.length" class="mb-6 space-y-3">
          <div class="flex items-center justify-between">
            <p class="text-sm font-medium">{{ t('em.dnsPreviewTitle') }}</p>
            <BaseButton
              :icon="mdiRefresh"
              :label="t('em.dnsCheck')"
              color="info"
              small
              :disabled="dnsChecking"
              @click="previewDns"
            />
          </div>

          <div v-for="entry in dnsPreview" :key="'preview-' + entry.domain" class="rounded-lg border border-gray-200 dark:border-gray-700 p-3">
            <div class="flex flex-wrap items-center gap-2 mb-2">
              <span class="font-semibold">{{ entry.domain }}</span>
              <span class="text-xs text-gray-500">{{ entry.message }}</span>
              <BaseButton
                v-if="!entry.synced"
                :icon="mdiCloudUpload"
                :label="t('em.dnsPublishMissing')"
                color="success"
                small
                :disabled="dnsSyncing"
                class="ml-auto"
                @click="syncDomainByName(entry.domain)"
              />
            </div>

            <div v-if="entry.records?.length" class="space-y-1">
              <div v-for="(record, i) in entry.records" :key="i" class="flex flex-wrap items-start gap-2 text-xs">
                <span class="px-2 py-0.5 rounded-full font-medium" :class="dnsActionClass(record.action)">
                  {{ t('em.dnsAction_' + record.action) }}
                </span>
                <span class="font-mono w-14">{{ record.kind }}</span>
                <span class="font-mono break-all flex-1">{{ record.content }}</span>
                <span v-if="record.current && record.action === 'differs'" class="text-gray-500 break-all w-full pl-2">
                  {{ t('em.dnsCurrently') }}: <span class="font-mono">{{ record.current }}</span>
                </span>
                <span v-if="record.detail" class="text-red-500 w-full pl-2">{{ record.detail }}</span>
              </div>
            </div>
          </div>
        </div>

        <div v-for="entry in dnsRecords" :key="entry.domain" class="mb-6 last:mb-0">
          <p class="font-semibold mb-2">{{ entry.domain }}</p>
          <div class="overflow-x-auto">
            <table class="w-full text-sm">
              <thead class="text-left text-gray-500">
                <tr>
                  <th class="py-2 pr-3">{{ t('em.dnsType') }}</th>
                  <th class="py-2 pr-3">{{ t('em.dnsName') }}</th>
                  <th class="py-2 pr-3">{{ t('em.dnsValue') }}</th>
                  <th class="py-2"></th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="(record, i) in entry.records" :key="i" class="border-t border-gray-200 dark:border-gray-700 align-top">
                  <td class="py-2 pr-3 font-mono text-xs">
                    {{ record.type }}<span v-if="record.priority" class="text-gray-500"> ({{ record.priority }})</span>
                  </td>
                  <td class="py-2 pr-3 font-mono text-xs break-all">{{ record.name }}</td>
                  <td class="py-2 pr-3 font-mono text-xs break-all">
                    {{ record.value }}
                    <span class="block text-gray-500 font-sans">{{ record.note }}</span>
                  </td>
                  <td class="py-2 text-right">
                    <BaseButton :icon="mdiContentCopy" color="info" small @click="copyText(record.value)" />
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
        <p v-if="!dnsRecords.length" class="text-sm text-gray-500">{{ t('em.dnsEmpty') }}</p>
      </CardBox>
    </div>

    <!-- Cleanup of the retired container setup -->
    <!-- Security: who the guard is refusing, and blocking by hand -->
    <div v-if="activeTab === 'security'">
      <CardBox class="mb-6">
        <div class="flex items-center justify-between mb-1">
          <h3 class="text-lg font-semibold">{{ t('em.blockedTitle') }}</h3>
          <BaseButton :icon="mdiRefresh" color="info" small @click="loadBlockedClients" />
        </div>
        <p class="text-sm text-gray-500 mb-4">{{ t('em.blockedHint') }}</p>

        <div v-if="blockedClients.length" class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead class="text-left text-gray-500">
              <tr>
                <th class="py-2">IP</th>
                <th class="py-2">{{ t('em.blockedReason') }}</th>
                <th class="py-2">{{ t('em.blockedUntil') }}</th>
                <th class="py-2"></th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="client in blockedClients" :key="client.ip" class="border-t border-gray-200 dark:border-gray-700">
                <td class="py-2 font-mono text-xs">{{ client.ip }}</td>
                <td class="py-2 text-xs">
                  {{ client.reason }}
                  <span v-if="client.manual" class="ml-1 text-xs text-gray-500">({{ t('em.blockedManual') }})</span>
                </td>
                <td class="py-2 text-xs">{{ formatQueueTime(client.until) }}</td>
                <td class="py-2 text-right">
                  <BaseButton :label="t('em.unblock')" color="info" small @click="unblockClient(client.ip)" />
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <p v-else class="text-sm text-emerald-600 dark:text-emerald-400">{{ t('em.blockedNone') }}</p>
      </CardBox>

      <!-- Blocking by hand: the guard reacts to behaviour, but an address that
           has already made itself a nuisance elsewhere can be shut out now. -->
      <CardBox class="mb-6">
        <h3 class="text-lg font-semibold mb-1">{{ t('em.blockAddTitle') }}</h3>
        <p class="text-sm text-gray-500 mb-4">{{ t('em.blockAddHint') }}</p>

        <div class="flex flex-wrap items-end gap-3">
          <label class="text-sm">
            <span class="mb-1 block text-gray-500">{{ t('em.blockedIP') }}</span>
            <input
              v-model="newBlock.ip"
              type="text"
              placeholder="203.0.113.5"
              class="w-48 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-slate-800 px-3 py-2"
            />
          </label>
          <label class="text-sm flex-1 min-w-[12rem]">
            <span class="mb-1 block text-gray-500">{{ t('em.blockedReason') }}</span>
            <input
              v-model="newBlock.reason"
              type="text"
              :placeholder="t('em.blockReasonPlaceholder')"
              class="w-full rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-slate-800 px-3 py-2"
            />
          </label>
          <label class="text-sm">
            <span class="mb-1 block text-gray-500">{{ t('em.blockMinutes') }}</span>
            <input
              v-model.number="newBlock.minutes"
              type="number"
              min="1"
              class="w-28 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-slate-800 px-3 py-2"
            />
          </label>
          <BaseButton :label="t('em.blockAdd')" color="danger" :disabled="blockBusy" @click="blockClient" />
        </div>
      </CardBox>
    </div>

    <!-- Cleanup: leftovers from the container-based setup -->
    <div v-if="activeTab === 'cleanup'">
      <CardBox>
        <h3 class="text-lg font-semibold mb-1">{{ t('em.cleanupTitle') }}</h3>
        <p class="text-sm text-gray-500 mb-4">{{ t('em.cleanupHint') }}</p>

        <div v-if="legacyArtifacts.length">
          <ul class="mb-4 space-y-1">
            <li
              v-for="(artifact, i) in legacyArtifacts"
              :key="i"
              class="text-sm font-mono px-3 py-2 rounded bg-gray-50 dark:bg-slate-800 break-all"
            >
              {{ artifact }}
            </li>
          </ul>
          <BaseButton
            :icon="mdiDelete"
            :label="t('em.cleanupRun')"
            color="danger"
            :disabled="cleaningUp"
            @click="cleanupLegacy"
          />
        </div>
        <p v-else class="text-sm text-emerald-600 dark:text-emerald-400">{{ t('em.cleanupNothing') }}</p>
      </CardBox>
    </div>

    <!-- Add Domain Modal -->
    <CardBoxModal
      v-model="isAddDomainModalActive"
      :title="t('em.addEmailDomain')"
      :button-label="t('common.add')"
      has-cancel
      @confirm="addDomain"
    >
      <!-- Cloudflare Auto DNS Info -->
      <div class="mb-4 p-4 bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg">
        <div class="flex items-start gap-3">
          <BaseIcon :path="mdiCloud" class="text-blue-600 dark:text-blue-400 flex-shrink-0 mt-0.5" w="w-5" h="h-5" />
          <div class="flex-1">
            <h4 class="font-semibold text-blue-900 dark:text-blue-100 mb-1">
              {{ t('em.cfAutoDns') }}
            </h4>
            <p class="text-sm text-blue-700 dark:text-blue-300">
              {{ t('em.cfAutoDnsDesc') }}
            </p>
          </div>
        </div>
      </div>

      <FormField :label="t('em.domain')">
        <FormControl v-model="newDomain.domain" placeholder="example.com" required />
      </FormField>
      <FormField :label="t('em.description')">
        <FormControl v-model="newDomain.description" :placeholder="t('em.optionalDescription')" />
      </FormField>
    </CardBoxModal>

    <!-- Add Mailbox Modal -->
    <CardBoxModal
      v-model="isAddMailboxModalActive"
      :title="t('em.createMailbox')"
      :button-label="t('common.create')"
      has-cancel
      @confirm="addMailbox"
    >
      <FormField :label="t('em.domain')">
        <select
          v-model.number="newMailbox.domain_id"
          class="px-3 py-2 max-w-full focus:ring focus:outline-none border-gray-700 rounded w-full h-12 border bg-white dark:bg-slate-800"
          required
        >
          <option value="" disabled selected>{{ domains.length === 0 ? t('em.noDomainsAvailable') : t('em.selectDomain') }}</option>
          <option v-for="domain in domains" :key="domain.id" :value="domain.id">
            {{ domain.domain }}
          </option>
        </select>
      </FormField>
      <FormField :label="t('em.username')">
        <FormControl v-model="newMailbox.username" placeholder="username" required />
      </FormField>
      <FormField :label="t('em.password')">
        <FormControl v-model="newMailbox.password" type="password" :placeholder="t('em.password')" required />
      </FormField>
      <FormField :label="t('em.displayName')">
        <FormControl v-model="newMailbox.name" placeholder="John Doe" />
      </FormField>
    </CardBoxModal>

    <!-- What to type into a mail client for this mailbox -->
    <CardBoxModal
      v-model="isConnectionModalActive"
      :title="t('em.connTitle')"
      :button-label="t('common.close')"
      @confirm="isConnectionModalActive = false"
    >
      <ConnectionSettings
        v-if="connectionMailbox"
        :mailbox="connectionMailbox"
        :listeners="nativeStatus.listeners || []"
        :hostname="nativeConfig?.hostname || ''"
        :ip-address="nativeConfig?.ip_address || ''"
      />
    </CardBoxModal>

    <!-- Update Password Modal -->
    <CardBoxModal
      v-model="isUpdatePasswordModalActive"
      :title="t('em.updateMailboxPassword')"
      :button-label="t('em.updatePassword')"
      has-cancel
      @confirm="updateMailboxPassword"
    >
      <div v-if="selectedMailbox" class="mb-4 p-3 bg-blue-50 dark:bg-blue-900/20 rounded-lg">
        <p class="text-sm text-blue-800 dark:text-blue-200">
<strong>{{ t('em.mailboxLabel') }}</strong> {{ selectedMailbox.email }}
        </p>
        <p class="text-xs text-blue-600 dark:text-blue-400 mt-1">
          {{ t('em.passwordUpdateHint') }}
        </p>
      </div>
      
      <FormField :label="t('em.newPassword')">
        <FormControl
          v-model="updatePasswordForm.password"
          type="password"
          :placeholder="t('em.enterNewPassword')"
          autocomplete="new-password"
          required
        />
      </FormField>
      
      <FormField :label="t('em.confirmPassword')">
        <FormControl
          v-model="updatePasswordForm.confirmPassword"
          type="password"
          :placeholder="t('em.confirmNewPassword')"
          autocomplete="new-password"
          required
        />
      </FormField>
      
      <div class="mt-4 p-3 bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-lg">
        <p class="text-xs text-yellow-800 dark:text-yellow-200" v-html="t('em.whyUpdatePassword')"></p>
      </div>
    </CardBoxModal>

    <!-- Edit Domain Modal -->
    <CardBoxModal
      v-model="isEditDomainModalActive"
      :title="t('em.editDomain')"
      :button-label="t('em.saveChanges')"
      has-cancel
      @confirm="editDomain"
    >
      <div v-if="selectedDomain" class="mb-4 p-3 bg-blue-50 dark:bg-blue-900/20 rounded-lg">
        <p class="text-sm text-blue-800 dark:text-blue-200">
<strong>{{ t('em.domainLabel') }}</strong> {{ selectedDomain.domain }}
        </p>
      </div>

      <FormField :label="t('em.description')">
        <FormControl v-model="editDomainForm.description" :placeholder="t('em.domainDescription')" />
      </FormField>
      
      <FormField :label="t('common.status')">
        <label class="flex items-center space-x-3 cursor-pointer">
          <input
            v-model="editDomainForm.enabled"
            type="checkbox"
            class="w-5 h-5 text-blue-600 rounded focus:ring-2 focus:ring-blue-500"
          />
          <span class="text-sm font-medium">{{ t('em.enableDomain') }}</span>
        </label>
      </FormField>

      <div class="mt-4 p-3 bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800 rounded-lg">
        <p class="text-xs text-green-800 dark:text-green-200">
          {{ t('em.dnsAutoUpdateFull') }}
        </p>
      </div>
    </CardBoxModal>

    <!-- Edit Mailbox Modal -->
    <CardBoxModal
      v-model="isEditMailboxModalActive"
      :title="t('em.editMailboxSettings')"
      :button-label="t('em.saveChanges')"
      has-cancel
      @confirm="editMailbox"
    >
      <div v-if="selectedMailbox" class="mb-4 p-3 bg-blue-50 dark:bg-blue-900/20 rounded-lg">
        <p class="text-sm text-blue-800 dark:text-blue-200">
<strong>{{ t('em.mailboxLabel') }}</strong> {{ selectedMailbox.email }}
        </p>
      </div>

      <FormField :label="t('em.displayName')">
        <FormControl v-model="editMailboxForm.name" placeholder="John Doe" />
      </FormField>
      
      <FormField :label="t('em.quotaBytes')">
        <FormControl
          v-model.number="editMailboxForm.quota"
          type="number"
          placeholder="10737418240"
        />
        <p class="text-xs text-gray-500 mt-1">{{ t('em.quotaHint') }}</p>
      </FormField>
      
      <FormField :label="t('em.forwardTo')">
        <FormControl
          v-model="editMailboxForm.forward_to"
          type="email"
          placeholder="forward@example.com"
        />
      </FormField>
      
      <FormField :label="t('em.autoReplyMsg')">
        <FormControl
          v-model="editMailboxForm.auto_reply_msg"
          type="textarea"
          :placeholder="t('em.outOfOffice')"
        />
      </FormField>
      
      <FormField :label="t('em.newPasswordOptional')">
        <FormControl
          v-model="editMailboxForm.password"
          type="password"
          :placeholder="t('em.leaveEmptyKeep')"
        />
      </FormField>
      
      <FormField :label="t('common.status')">
        <label class="flex items-center space-x-3 cursor-pointer mb-3">
          <input
            v-model="editMailboxForm.enabled"
            type="checkbox"
            class="w-5 h-5 text-blue-600 rounded focus:ring-2 focus:ring-blue-500"
          />
          <span class="text-sm font-medium">{{ t('em.enableMailbox') }}</span>
        </label>
        
        <label class="flex items-center space-x-3 cursor-pointer mb-3">
          <input
            v-model="editMailboxForm.keep_copy"
            type="checkbox"
            class="w-5 h-5 text-blue-600 rounded focus:ring-2 focus:ring-blue-500"
          />
          <span class="text-sm font-medium">{{ t('em.keepCopyForwarding') }}</span>
        </label>
        
        <label class="flex items-center space-x-3 cursor-pointer">
          <input
            v-model="editMailboxForm.auto_reply"
            type="checkbox"
            class="w-5 h-5 text-blue-600 rounded focus:ring-2 focus:ring-blue-500"
          />
          <span class="text-sm font-medium">{{ t('em.enableAutoReply') }}</span>
        </label>
      </FormField>

      <div class="mt-4 p-3 bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800 rounded-lg">
        <p class="text-xs text-green-800 dark:text-green-200">
          {{ t('em.dnsAutoUpdate') }}
        </p>
      </div>
    </CardBoxModal>
  </div>
</template>

<style scoped>
/* Mail içindeki alıntı (On ... wrote:) - blockquote ve quoted satırlar */
.email-quoted :deep(blockquote),
.email-quoted :deep(.gmail_quote),
.email-quoted :deep(.email-plain-quote) {
  border-left: 4px solid var(--color-gray-300, #d1d5db);
  margin: 0.75rem 0;
  padding: 0.5rem 0 0.5rem 1rem;
  background: rgba(0, 0, 0, 0.03);
  border-radius: 0 6px 6px 0;
  color: var(--color-gray-600, #4b5563);
  font-size: 0.9em;
}
.dark .email-quoted :deep(blockquote),
.dark .email-quoted :deep(.gmail_quote),
.dark .email-quoted :deep(.email-plain-quote) {
  border-left-color: #475569;
  background: rgba(255, 255, 255, 0.04);
  color: #94a3b8;
}
</style>
