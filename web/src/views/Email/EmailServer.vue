<script setup>
import BaseButton from "@/components/BaseButton.vue";
import BaseIcon from "@/components/BaseIcon.vue";
import CardBox from "@/components/CardBox.vue";
import CardBoxModal from "@/components/CardBoxModal.vue";
import FormControl from "@/components/FormControl.vue";
import FormField from "@/components/FormField.vue";
import DomainManagement from "@/components/Email/DomainManagement.vue";
import MailboxManagement from "@/components/Email/MailboxManagement.vue";

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
  mdiStar,
  mdiStarOutline,
  mdiArchive,
  mdiTrashCan,
  mdiAlertOctagon,
  mdiPencil,
  mdiReply,
  mdiReplyAll,
  mdiArrowLeft,
  mdiAttachment,
  mdiDotsVertical,
  mdiFolderOutline,
  mdiInformationOutline,
  mdiCloud,
  mdiKey,
  mdiChevronDown,
  mdiChevronUp,
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
const threads = ref([]);

// Modals
const isAddDomainModalActive = ref(false);
const isEditDomainModalActive = ref(false);
const isAddMailboxModalActive = ref(false);
const isEditMailboxModalActive = ref(false);
const isUpdatePasswordModalActive = ref(false);
const isComposeModalActive = ref(false);
const selectedMailbox = ref(null);
const selectedDomain = ref(null);
const selectedFolder = ref('INBOX');
const selectedEmail = ref(null);
const showEmailDetail = ref(false);
const threadMessages = ref([]);
const threadLoading = ref(false);
/** Thread / body segment kartlarında hangisi açık: 'msg-{uid}' veya 'seg-{idx}' */
const expandedCardKeys = ref(new Set());
/** Klasör listesinde hangi e-postalar açık (inline önizleme): uid set */
const expandedListUids = ref(new Set());
/** Thread listesinde hangi thread'ler açık (count > 1 olanlarda açılır/kapanır): thread_id set */
const expandedThreadIds = ref(new Set());

// Email folders (dynamically loaded from IMAP)
const folders = ref([
  { name: 'Inbox', value: 'INBOX', icon: mdiInbox, color: 'text-blue-600', message_count: 0 }
]);

// Icon mapping for common folders
const folderIconMap = {
  'INBOX': { icon: mdiInbox, color: 'text-blue-600' },
  'Sent': { icon: mdiSend, color: 'text-green-600' },
  'Drafts': { icon: mdiPencil, color: 'text-gray-600' },
  'Spam': { icon: mdiAlertOctagon, color: 'text-red-600' },
  'Trash': { icon: mdiTrashCan, color: 'text-gray-600' },
  'Archive': { icon: mdiArchive, color: 'text-purple-600' },
  'Starred': { icon: mdiStar, color: 'text-yellow-500' }
};

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

const newEmail = ref({
  to: '',
  subject: '',
  body: ''
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
const parseBodyIntoQuoteCards = (body) => {
  if (!body || typeof body !== 'string') return [{ header: null, content: '' }];
  const lines = body.split('\n');
  const segments = [];
  const isQuoteHeader = (line) => (
    /^On .+ wrote:\s*$/i.test(line.trim()) ||
    /tarihinde şunu yazdı:\s*$/.test(line) ||
    (line.includes('adresine sahip kullanıcı') && line.includes('şunu yazdı:'))
  );
  let i = 0;
  while (i < lines.length) {
    if (segments.length === 0) {
      const contentLines = [];
      while (i < lines.length && !isQuoteHeader(lines[i])) {
        contentLines.push(lines[i]);
        i++;
      }
      segments.push({ header: null, content: contentLines.join('\n').trim() });
      continue;
    }
    if (isQuoteHeader(lines[i])) {
      const header = lines[i].trim();
      i++;
      const contentLines = [];
      while (i < lines.length && !isQuoteHeader(lines[i])) {
        contentLines.push(lines[i]);
        i++;
      }
      segments.push({ header, content: contentLines.join('\n').trim() });
    } else {
      i++;
    }
  }
  return segments.filter((s) => s.content || s.header);
};

/** Tek e-posta gösterilirken gövde "On ... wrote:" bloklarına bölünmüş hali (her blok ayrı CardBox) */
const bodySegments = computed(() =>
  parseBodyIntoQuoteCards(selectedEmail.value?.body_plain || '')
);

/** Açılır kart: sadece ilk kart açık başlar */
watch(
  () => [selectedEmail.value, threadMessages.value, bodySegments.value],
  () => {
    if (!selectedEmail.value) {
      expandedCardKeys.value = new Set();
      return;
    }
    if (threadMessages.value.length > 0) {
      expandedCardKeys.value = new Set([`msg-${threadMessages.value[0].uid}`]);
      return;
    }
    if (bodySegments.value.length > 0) {
      expandedCardKeys.value = new Set(['seg-0']);
      return;
    }
    expandedCardKeys.value = new Set();
  },
  { immediate: true }
);

const toggleThreadCard = (key) => {
  const next = new Set(expandedCardKeys.value);
  if (next.has(key)) next.delete(key);
  else next.add(key);
  expandedCardKeys.value = next;
};

const toggleListRow = (uid, event) => {
  event?.stopPropagation();
  const next = new Set(expandedListUids.value);
  if (next.has(uid)) next.delete(uid);
  else next.add(uid);
  expandedListUids.value = next;
};

const toggleThreadRow = (threadId, event) => {
  event?.stopPropagation();
  const next = new Set(expandedThreadIds.value);
  if (next.has(threadId)) next.delete(threadId);
  else next.add(threadId);
  expandedThreadIds.value = next;
};

/** Çok mesajlı thread ana satırına tıklanınca sağ panelde thread mesajlarını CardBox olarak açar */
const openThreadInDetail = (thread) => {
  if (!thread?.messages?.length) return;
  const msgs = thread.messages;
  const latest = msgs[msgs.length - 1];
  selectedEmail.value = latest;
  threadMessages.value = [...msgs];
  showEmailDetail.value = true;
  expandedCardKeys.value = new Set([`msg-${latest.uid}`]);
};

/** Toplam e-posta sayısı (thread'lerdeki mesaj toplamı) */
const totalEmailCount = computed(() =>
  threads.value.reduce((acc, t) => acc + (t.count || t.messages?.length || 0), 0)
);

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

const loadEmails = async (mailboxId, folder = 'INBOX') => {
  if (!mailboxId) return;
  
  loading.value = true;
  try {
    const response = await ApiService.get(`/api/email/mailboxes/${mailboxId}/emails`, {
      params: { folder, limit: 50 }
    });
    if (!response.data.error) {
      threads.value = response.data.data || [];
      expandedListUids.value = new Set();
      expandedThreadIds.value = new Set();
    }
  } catch (error) {
    console.error('Failed to load emails:', error);
    toast.error(t('em.failedToLoadEmails'));
  } finally {
    loading.value = false;
  }
};

const loadThread = async (mailboxId, folder, uid) => {
  if (!mailboxId || !uid) return;
  threadLoading.value = true;
  threadMessages.value = [];
  try {
    const response = await ApiService.get(`/api/email/mailboxes/${mailboxId}/thread`, {
      params: { folder, uid }
    });
    if (!response.data.error) {
      threadMessages.value = response.data.data || [];
    }
  } catch (error) {
    console.error('Failed to load thread:', error);
    threadMessages.value = [];
  } finally {
    threadLoading.value = false;
  }
};

const loadFolders = async (mailboxId) => {
  if (!mailboxId) return;
  
  try {
    const response = await ApiService.get(`/api/email/mailboxes/${mailboxId}/folders`);
    if (!response.data.error) {
      const imapFolders = response.data.data || [];
      
      // Map IMAP folders to UI format
      folders.value = imapFolders.map(folder => {
        // Get folder name (remove leading dot for special folders)
        const cleanName = folder.name.startsWith('.') ? folder.name.substring(1) : folder.name;
        const displayName = cleanName === 'INBOX' ? 'Inbox' : cleanName;
        
        // Get icon and color from mapping or use default
        const iconInfo = folderIconMap[cleanName] || { icon: mdiFolderOutline, color: 'text-gray-600' };
        
        return {
          name: displayName,
          value: folder.name, // Use original name for IMAP commands
          icon: iconInfo.icon,
          color: iconInfo.color,
          message_count: folder.message_count || 0,
          has_children: folder.has_children,
          no_select: folder.no_select
        };
      }).filter(f => !f.no_select); // Filter out non-selectable folders
      
      console.log('📁 Loaded folders:', folders.value);
    }
  } catch (error) {
    console.error('Failed to load folders:', error);
    // Keep default INBOX folder on error
    folders.value = [
      { name: 'Inbox', value: 'INBOX', icon: mdiInbox, color: 'text-blue-600', message_count: 0 }
    ];
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

const addDomain = async () => {
  try {
    const response = await ApiService.post('/api/email/domains', newDomain.value);
    if (!response.data.error) {
      toast.success(t('em.domainAdded'));
      toast.info(t('em.checkingCloudflare'), { timeout: 3000 });
      await loadDomains();
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
      await loadDomains();
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
      await loadDomains();
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

const addMailbox = async () => {
  try {
    if (!newMailbox.value.domain_id) {
      toast.error(t('em.selectDomainErr'));
      return;
    }
    
    const response = await ApiService.post('/api/email/mailboxes', newMailbox.value);
    if (!response.data.error) {
      toast.success(t('em.mailboxCreated'));
      await loadMailboxes();
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
      await loadMailboxes();
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
      await loadMailboxes();
    } else {
      toast.error('❌ ' + response.data.msg);
    }
  } catch (error) {
    toast.error(t('em.errorPrefix') + error.message);
  }
};

const sendEmail = async () => {
  if (!selectedMailbox.value) {
    toast.error(t('em.selectMailboxFirst'));
    return;
  }
  const toList = newEmail.value.to.split(/[,;]/).map(e => e.trim()).filter(Boolean);
  if (!toList.length) {
    toast.error(t('em.enterRecipient'));
    return;
  }
  if (!(newEmail.value.subject || '').trim()) {
    toast.error(t('em.enterSubject'));
    return;
  }
  try {
    const emailData = {
      to: toList,
      subject: newEmail.value.subject.trim(),
      body: newEmail.value.body || ''
    };
    const response = await ApiService.post(
      `/api/email/mailboxes/${selectedMailbox.value}/send`,
      emailData
    );
    if (!response.data.error) {
      toast.success(t('em.emailSent'));
      isComposeModalActive.value = false;
      newEmail.value = { to: '', subject: '', body: '' };
      loadEmails(selectedMailbox.value, selectedFolder.value);
    } else {
      toast.error('❌ ' + response.data.msg);
    }
  } catch (error) {
    toast.error(t('em.failedToSend') + (error.response?.data?.msg || error.message));
  }
};

const formatDate = (date) => {
  if (!date) return '';
  return new Date(date).toLocaleString();
};

const formatTime = (date) => {
  if (!date) return '';
  const d = new Date(date);
  const now = new Date();
  const diff = now - d;
  const days = Math.floor(diff / (1000 * 60 * 60 * 24));
  
  if (days === 0) {
    return d.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' });
  } else if (days === 1) {
    return t('em.yesterday');
  } else if (days < 7) {
    return d.toLocaleDateString('en-US', { weekday: 'short' });
  } else if (now.getFullYear() === d.getFullYear()) {
    return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
  } else {
    return d.toLocaleDateString('en-US', { year: 'numeric', month: 'short', day: 'numeric' });
  }
};

const getInitials = (name) => {
  if (!name) return '?';
  const email = name.includes('<') ? name.match(/<(.+)>/)?.[1] : name;
  const username = email?.split('@')[0] || name;
  return username.split(/[._-]/).map(n => n[0]).join('').toUpperCase().slice(0, 2);
};

/** Adres string'inden e-posta adresini çıkar (örn: "Ad Soyad <a@b.com>" -> "a@b.com") */
const extractEmailFromAddress = (str) => {
  if (!str || typeof str !== 'string') return '';
  const m = str.match(/<([^>]+)>/);
  return m ? m[1].trim() : str.trim();
};

/** Reply: compose modal aç, To=gönderen, Subject=Re:..., Body=alıntı */
const openReplyCompose = (replyAll = false) => {
  if (!selectedEmail.value) return;
  const fromAddr = extractEmailFromAddress(selectedEmail.value.from);
  let toAddr = fromAddr;
  if (replyAll && selectedEmail.value.to) {
    const toList = selectedEmail.value.to.split(/[,;]/).map(s => extractEmailFromAddress(s.trim())).filter(Boolean);
    const combined = new Set([fromAddr, ...toList]);
    toAddr = [...combined].join(', ');
  }
  let subj = selectedEmail.value.subject || '';
  if (subj && !/^re:\s+/i.test(subj)) subj = 'Re: ' + subj;
  const quoted = selectedEmail.value.body_plain
    ? `\n\nOn ${formatDate(selectedEmail.value.date)} ${selectedEmail.value.from} wrote:\n${selectedEmail.value.body_plain.split('\n').map(l => '> ' + l).join('\n')}`
    : '';
  newEmail.value = {
    to: toAddr,
    subject: subj,
    body: quoted
  };
  isComposeModalActive.value = true;
};

const openComposeNew = () => {
  newEmail.value = { to: '', subject: '', body: '' };
  isComposeModalActive.value = true;
};

const toggleStar = (email) => {
  email.flagged = !email.flagged;
  toast.info(email.flagged ? t('em.starred') : t('em.unstarred'));
};

const onArchive = () => {
  toast.info(t('em.archiveSoon'));
};

const onMoveToTrash = () => {
  toast.info(t('em.trashSoon'));
};

const formatFileSize = (bytes) => {
  if (!bytes) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i];
};

/** Plain metindeki > alıntı satırlarını blockquote HTML'e çevirir (daha profesyonel görünüm) */
const plainTextToHtml = (plain) => {
  if (!plain || typeof plain !== 'string') return '';
  const escape = (s) => String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
  const lines = plain.split('\n');
  const parts = [];
  let quoteLines = [];
  const flushQuote = () => {
    if (quoteLines.length === 0) return;
    const content = quoteLines
      .map((l) => l.replace(/^(>\s*)+/, '').trim())
      .map(escape)
      .join('<br/>');
    parts.push(`<blockquote class="email-plain-quote">${content}</blockquote>`);
    quoteLines = [];
  };
  for (const line of lines) {
    if (line.startsWith('>')) {
      quoteLines.push(line);
    } else {
      flushQuote();
      parts.push(escape(line) + '<br/>');
    }
  }
  flushQuote();
  return parts.join('');
};

// ---- Mail traffic logs -------------------------------------------------
const logsLoading = ref(false);
const logEntries = ref([]);
const logStats = ref({ incoming: 0, outgoing: 0, rejected: 0, deferred: 0, bounced: 0 });
const logSource = ref('');
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
const logView = ref('messages'); // messages | connections | raw

// Connection traces show what happened on the wire: attempts that never became
// a message (refused TLS, probes, bad passwords) live here, not in the message log.
const certificate = ref(null);
const requestingCert = ref(false);

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
  if (activeTab.value === 'logs' && logView.value === 'messages') loadLogs();
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
const legacyArtifacts = ref([]);
const cleaningUp = ref(false);

// Tabs, in the order an operator works through them.
const mailTabs = ['overview', 'domains', 'mailboxes', 'webmail', 'logs', 'listeners', 'queue', 'dns', 'cleanup'];

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
      } else {
        toast.info(results[0]?.message || t('em.dnsSyncSkipped'));
      }
      await Promise.all([loadEngine(), loadDomains()]);
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
  },
  { immediate: true }
);

// Opening Webmail should show mail, not an account picker, when there is an
// obvious account to open.
watch(activeTab, (tab) => {
  if (tab !== 'webmail' || selectedMailbox.value || mailboxes.value.length === 0) return;
  selectedMailbox.value = mailboxes.value[0].id;
  loadFolders(selectedMailbox.value);
  loadEmails(selectedMailbox.value, selectedFolder.value);
});

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

    <!-- Webmail Tab - Gmail Style -->
    <div v-if="activeTab === 'webmail'" class="h-[calc(100vh-250px)]">
      <!-- Mailbox Selector Bar -->
      <div class="mb-4">
        <select
          v-model.number="selectedMailbox"
          class="px-4 py-2 max-w-xs focus:ring focus:outline-none border-gray-300 dark:border-gray-700 rounded-lg w-full border bg-white dark:bg-slate-800 font-medium"
          @change="loadFolders(selectedMailbox); loadEmails(selectedMailbox, selectedFolder)"
        >
          <option value="" disabled>{{ t('em.selectEmailAccount') }}</option>
          <option v-for="mailbox in mailboxes" :key="mailbox.id" :value="mailbox.id">
            {{ mailbox.email }}
          </option>
        </select>
      </div>

      <!-- Gmail-like 3-Panel Layout -->
      <div v-if="selectedMailbox" class="flex gap-4 h-full">
        <!-- Left Sidebar - Folders -->
        <div class="w-64 bg-white dark:bg-slate-800 rounded-lg border border-gray-200 dark:border-gray-700 p-4 flex flex-col">
          <!-- Compose Button -->
          <BaseButton
            :icon="mdiPencil"
            color="info"
            :label="t('em.newEmail')"
            class="mb-6 w-full justify-center"
            @click="openComposeNew"
          />

          <!-- Folders List -->
          <nav class="space-y-1 flex-1">
            <button
              v-for="folder in folders"
              :key="folder.value"
              :class="[
                'w-full flex items-center gap-3 px-3 py-2 rounded-lg text-left transition-colors',
                selectedFolder === folder.value
                  ? 'bg-blue-50 dark:bg-blue-900/20 text-blue-600 dark:text-blue-400 font-medium'
                  : 'hover:bg-gray-100 dark:hover:bg-gray-700 text-gray-700 dark:text-gray-300'
              ]"
              @click="selectedFolder = folder.value; loadEmails(selectedMailbox, folder.value)"
            >
              <BaseIcon :path="folder.icon" :class="folder.color" w="w-5" h="h-5" />
              <span class="flex-1">{{ folder.name }}</span>
              <span v-if="folder.value === selectedFolder" class="text-xs bg-gray-200 dark:bg-gray-600 px-2 py-1 rounded-full">
                {{ totalEmailCount }}
              </span>
            </button>
          </nav>
        </div>

        <!-- Middle Panel - Email List -->
        <div class="flex-1 bg-white dark:bg-slate-800 rounded-lg border border-gray-200 dark:border-gray-700 overflow-hidden flex flex-col">
          <!-- List Header -->
          <div class="px-4 py-3 border-b border-gray-200 dark:border-gray-700 flex items-center justify-between">
            <div class="flex items-center gap-2">
              <h3 class="font-semibold text-lg">{{ folders.find(f => f.value === selectedFolder)?.name || t('em.inbox') }}</h3>
              <span class="text-sm text-gray-500">{{ t('em.emailsCount', { n: totalEmailCount }) }}</span>
            </div>
            <BaseButton :icon="mdiRefresh" color="light" small @click="loadEmails(selectedMailbox, selectedFolder)" />
          </div>

          <!-- Loading State -->
          <div v-if="loading" class="flex-1 flex items-center justify-center text-gray-500">
            <div class="text-center">
              <div class="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600 mx-auto mb-4"></div>
              <p>{{ t('em.loadingEmails') }}</p>
            </div>
          </div>

          <!-- Empty State -->
          <div v-else-if="threads.length === 0" class="flex-1 flex items-center justify-center text-gray-500">
            <div class="text-center">
              <BaseIcon :path="mdiEmailOpen" class="w-16 h-16 mx-auto mb-4 text-gray-300" />
              <p class="text-lg font-medium mb-2">{{ t('em.noEmailsIn', { folder: selectedFolder }) }}</p>
              <p class="text-sm">{{ t('em.folderEmpty') }}</p>
            </div>
          </div>

          <!-- Email List (thread gruplu; count > 1 ise açılır/kapanır) -->
          <div v-else class="flex-1 overflow-y-auto">
            <div
              v-for="thread in threads"
              :key="thread.thread_id"
              class="border-b border-gray-100 dark:border-gray-700"
            >
              <!-- Tek mesajlı thread: tek satır, açılır/kapanır yok; tıklanınca sağda detay -->
              <template v-if="thread.count === 1">
                <div
                  v-for="email in thread.messages"
                  :key="email.uid"
                  :class="[
                    'px-4 py-3 flex items-start gap-3 cursor-pointer transition-colors hover:bg-gray-50 dark:hover:bg-gray-700',
                    selectedEmail?.uid === email.uid ? 'bg-blue-50 dark:bg-blue-900/10' : '',
                    !email.seen ? 'bg-white dark:bg-slate-800' : 'bg-gray-50/50 dark:bg-slate-800/50'
                  ]"
                  @click="selectedEmail = email; showEmailDetail = true; loadThread(selectedMailbox, selectedFolder, email.uid)"
                >
                  <button class="mt-1 shrink-0" @click.stop="toggleStar(email)">
                    <BaseIcon
                      :path="email.flagged ? mdiStar : mdiStarOutline"
                      :class="email.flagged ? 'text-yellow-500' : 'text-gray-400 hover:text-yellow-500'"
                      w="w-5"
                      h="h-5"
                    />
                  </button>
                  <div class="flex-1 min-w-0">
                    <div class="flex items-center justify-between mb-1">
                      <p :class="['truncate', !email.seen ? 'font-bold text-gray-900 dark:text-white' : 'font-medium text-gray-700 dark:text-gray-300']">
                        {{ email.from || t('em.unknown') }}
                      </p>
                      <span class="text-xs text-gray-500 ml-2 shrink-0">{{ formatTime(email.date) }}</span>
                    </div>
                    <p :class="['text-sm truncate mb-1', !email.seen ? 'font-semibold text-gray-900 dark:text-white' : 'text-gray-600 dark:text-gray-400']">
                      {{ email.subject || t('em.noSubject') }}
                    </p>
                    <p class="text-sm text-gray-500 dark:text-gray-500 truncate">
                      {{ email.snippet || (email.body_plain || '').substring(0, 100) || t('em.noPreview') }}
                    </p>
                  </div>
                  <BaseIcon
                    v-if="email.has_attachments"
                    :path="mdiAttachment"
                    class="text-gray-400 mt-1 shrink-0"
                    w="w-4"
                    h="h-4"
                  />
                </div>
              </template>

              <!-- Çok mesajlı thread: Chevron → liste açılır; satıra tıklanınca sağda detay (CardBox) -->
              <template v-else>
                <div
                  :class="[
                    'px-4 py-3 flex items-start gap-3 cursor-pointer transition-colors hover:bg-gray-50 dark:hover:bg-gray-700',
                    selectedEmail && thread.messages.some(m => m.uid === selectedEmail.uid) ? 'bg-blue-50 dark:bg-blue-900/10' : '',
                    thread.messages.some(m => !m.seen) ? 'bg-white dark:bg-slate-800' : 'bg-gray-50/50 dark:bg-slate-800/50'
                  ]"
                  @click="openThreadInDetail(thread)"
                >
                  <button
                    type="button"
                    class="mt-1 shrink-0 p-0.5 rounded hover:bg-gray-200 dark:hover:bg-gray-600"
                    :aria-label="expandedThreadIds.has(thread.thread_id) ? t('em.collapse') : t('em.expand')"
                    @click.stop="toggleThreadRow(thread.thread_id, $event)"
                  >
                    <BaseIcon
                      :path="expandedThreadIds.has(thread.thread_id) ? mdiChevronUp : mdiChevronDown"
                      class="text-gray-400"
                      w="w-5"
                      h="h-5"
                    />
                  </button>
                  <div class="flex-1 min-w-0">
                    <div class="flex items-center justify-between mb-1">
                      <p class="truncate font-medium text-gray-700 dark:text-gray-300">
                        {{ thread.messages[0]?.from || t('em.unknown') }}
                      </p>
                      <span class="text-xs text-gray-500 ml-2 shrink-0">{{ formatTime(thread.date) }}</span>
                    </div>
                    <p class="text-sm truncate mb-1 font-medium text-gray-600 dark:text-gray-400">
                      {{ thread.subject || t('em.noSubject') }}
                      <span class="text-gray-500 dark:text-gray-500 font-normal">({{ thread.count }})</span>
                    </p>
                    <p class="text-sm text-gray-500 truncate">
                      {{ (thread.messages[0]?.body_plain || '').substring(0, 80) || t('em.noPreview') }}
                    </p>
                  </div>
                </div>
                <!-- Chevron ile açılan inline liste -->
                <div
                  v-show="expandedThreadIds.has(thread.thread_id)"
                  class="border-t border-gray-100 dark:border-gray-700 bg-gray-50/50 dark:bg-slate-800/80"
                >
                  <div
                    v-for="email in thread.messages"
                    :key="email.uid"
                    :class="[
                      'px-4 py-2.5 pl-12 flex items-start gap-3 cursor-pointer border-b border-gray-100 dark:border-gray-700 last:border-b-0 hover:bg-gray-100 dark:hover:bg-gray-700/50',
                      selectedEmail?.uid === email.uid ? 'bg-blue-50 dark:bg-blue-900/10' : ''
                    ]"
                    @click="selectedEmail = email; showEmailDetail = true; loadThread(selectedMailbox, selectedFolder, email.uid)"
                  >
                    <button class="mt-0.5 shrink-0" @click.stop="toggleStar(email)">
                      <BaseIcon
                        :path="email.flagged ? mdiStar : mdiStarOutline"
                        :class="email.flagged ? 'text-yellow-500' : 'text-gray-400 hover:text-yellow-500'"
                        w="w-4"
                        h="h-4"
                      />
                    </button>
                    <div class="flex-1 min-w-0">
                      <p :class="['text-sm truncate', !email.seen ? 'font-semibold text-gray-900 dark:text-white' : 'text-gray-600 dark:text-gray-400']">
                        {{ email.from }} · {{ formatTime(email.date) }}
                      </p>
                      <p class="text-xs text-gray-500 truncate">{{ email.subject || t('em.noSubject') }}</p>
                    </div>
                    <BaseIcon v-if="email.has_attachments" :path="mdiAttachment" class="text-gray-400 shrink-0" w="w-4" h="h-4" />
                  </div>
                </div>
              </template>
            </div>
          </div>
        </div>

        <!-- Right Panel - Email Detail (Slide-in) -->
        <transition
          enter-active-class="transition ease-out duration-200"
          enter-from-class="transform translate-x-full opacity-0"
          enter-to-class="transform translate-x-0 opacity-100"
          leave-active-class="transition ease-in duration-150"
          leave-from-class="transform translate-x-0 opacity-100"
          leave-to-class="transform translate-x-full opacity-0"
        >
          <div
            v-if="showEmailDetail && selectedEmail"
            class="w-[600px] bg-white dark:bg-slate-800 rounded-lg border border-gray-200 dark:border-gray-700 flex flex-col overflow-hidden"
          >
            <!-- Thread Header -->
            <div class="px-6 py-4 border-b border-gray-200 dark:border-gray-700">
              <div class="flex items-start justify-between mb-2">
                <h2 class="text-xl font-semibold flex-1 pr-4">{{ selectedEmail.subject || t('em.noSubject') }}</h2>
                <button class="text-gray-500 hover:text-gray-700 dark:hover:text-gray-300" @click="showEmailDetail = false">
                  <BaseIcon :path="mdiArrowLeft" w="w-6" h="h-6" />
                </button>
              </div>
              <div class="flex items-center gap-2">
                <BaseButton :icon="mdiReply" :label="t('em.reply')" color="light" small @click="openReplyCompose(false)" />
                <BaseButton :icon="mdiReplyAll" :label="t('em.replyAll')" color="light" small @click="openReplyCompose(true)" />
                <BaseButton :icon="mdiArchive" :label="t('em.archive')" color="light" small @click="onArchive" />
                <BaseButton :icon="mdiTrashCan" :label="t('em.moveToTrash')" color="danger" small @click="onMoveToTrash" />
              </div>
            </div>

            <!-- Konu zinciri (orijinal + cevaplar) + mail içindeki alıntı -->
            <div class="flex-1 overflow-y-auto px-6 py-4 space-y-6">
              <template v-if="threadLoading">
                <p class="text-sm text-gray-500">{{ t('common.loading') }}</p>
              </template>
              <template v-else-if="threadMessages.length === 0">
                <!-- HTML gövde: tek kart -->
                <CardBox v-if="selectedEmail.body_html" class="border-l-4 border-blue-500">
                  <div class="flex items-center gap-3 mb-3">
                    <div class="w-8 h-8 rounded-full bg-gradient-to-br from-blue-500 to-purple-600 flex items-center justify-center text-white text-sm font-bold">
                      {{ getInitials(selectedEmail.from) }}
                    </div>
                    <div class="flex-1 min-w-0">
                      <p class="font-medium text-sm truncate">{{ selectedEmail.from }}</p>
                      <p class="text-xs text-gray-500">{{ formatDate(selectedEmail.date) }}</p>
                    </div>
                  </div>
                  <div class="email-body-content text-sm">
                    <div class="prose prose-sm dark:prose-invert max-w-none email-quoted" v-html="selectedEmail.body_html"></div>
                  </div>
                </CardBox>
                <!-- Plain gövde: "On ... wrote:" bloklarına göre her biri ayrı CardBox, hep açık -->
                <template v-else>
                  <CardBox
                    v-for="(seg, idx) in bodySegments"
                    :key="idx"
                    class="border-l-4 border-blue-500"
                  >
                    <div class="w-full flex items-center gap-3 mb-3">
                      <div class="w-8 h-8 rounded-full bg-gradient-to-br from-blue-500 to-purple-600 flex items-center justify-center text-white text-sm font-bold shrink-0">
                        {{ idx === 0 ? getInitials(selectedEmail.from) : '…' }}
                      </div>
                      <div class="flex-1 min-w-0">
                        <p v-if="idx === 0" class="font-medium text-sm truncate">{{ selectedEmail.from }}</p>
                        <p v-else class="font-medium text-sm truncate text-gray-600 dark:text-gray-400">{{ seg.header }}</p>
                        <p v-if="idx === 0" class="text-xs text-gray-500">{{ formatDate(selectedEmail.date) }}</p>
                      </div>
                    </div>
                    <div class="email-body-content text-sm pt-3 border-t border-gray-200 dark:border-gray-600">
                      <div v-if="seg.content" class="prose prose-sm dark:prose-invert max-w-none email-body-plain email-quoted" v-html="plainTextToHtml(seg.content)"></div>
                      <p v-else class="text-gray-500 dark:text-gray-400">{{ t('em.noContent') }}</p>
                    </div>
                  </CardBox>
                </template>
              </template>
              <template v-else>
                <CardBox
                  v-for="msg in threadMessages"
                  :key="msg.uid"
                  class="border-l-4 border-blue-500"
                >
                  <div class="w-full flex items-center gap-3 mb-3">
                    <div class="w-8 h-8 rounded-full bg-gradient-to-br from-blue-500 to-purple-600 flex items-center justify-center text-white text-sm font-bold shrink-0">
                      {{ getInitials(msg.from) }}
                    </div>
                    <div class="flex-1 min-w-0">
                      <p class="font-medium text-sm truncate">{{ msg.from }}</p>
                      <p class="text-xs text-gray-500">{{ formatDate(msg.date) }}</p>
                    </div>
                  </div>
                  <div class="email-body-content text-sm pt-3 border-t border-gray-200 dark:border-gray-600">
                    <div v-if="msg.body_html" class="prose prose-sm dark:prose-invert max-w-none email-quoted" v-html="msg.body_html"></div>
                    <template v-else>
                      <div v-if="msg.body_plain" class="prose prose-sm dark:prose-invert max-w-none email-body-plain email-quoted" v-html="plainTextToHtml(msg.body_plain)"></div>
                      <p v-else class="text-gray-500 dark:text-gray-400">{{ t('em.noContent') }}</p>
                    </template>
                  </div>
                </CardBox>
              </template>
            </div>

            <!-- Attachments -->
            <div v-if="selectedEmail.attachments && selectedEmail.attachments.length > 0" class="px-6 py-4 border-t border-gray-200 dark:border-gray-700">
              <h4 class="font-semibold mb-3">{{ t('em.attachmentsCount', { n: selectedEmail.attachments.length }) }}</h4>
              <div class="space-y-2">
                <div
                  v-for="(attachment, idx) in selectedEmail.attachments"
                  :key="idx"
                  class="flex items-center gap-3 p-3 border border-gray-200 dark:border-gray-700 rounded-lg hover:bg-gray-50 dark:hover:bg-gray-700"
                >
                  <BaseIcon :path="mdiAttachment" class="text-gray-400" w="w-5" h="h-5" />
                  <div class="flex-1">
                    <p class="text-sm font-medium">{{ attachment.filename }}</p>
                    <p class="text-xs text-gray-500">{{ formatFileSize(attachment.size) }}</p>
                  </div>
                  <BaseButton :label="t('em.download')" color="light" small />
                </div>
              </div>
            </div>
          </div>
        </transition>
      </div>

      <!-- No Mailbox Selected -->
      <div v-else class="h-full flex items-center justify-center bg-white dark:bg-slate-800 rounded-lg border border-gray-200 dark:border-gray-700">
        <div class="text-center text-gray-500">
          <BaseIcon :path="mdiEmail" class="w-20 h-20 mx-auto mb-4 text-gray-300" />
          <p class="text-lg font-medium mb-2">{{ t('em.selectEmailAccountTitle') }}</p>
          <p class="text-sm">{{ t('em.selectAccountAbove') }}</p>
        </div>
      </div>
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
            <option :value="500">500 {{ t('em.logLines') }}</option>
            <option :value="1000">1000 {{ t('em.logLines') }}</option>
            <option :value="5000">5000 {{ t('em.logLines') }}</option>
          </select>

          <label class="flex items-center gap-2 text-sm text-gray-500">
            <input v-model="logAutoRefresh" type="checkbox" />
            {{ t('em.logAutoRefresh') }}
          </label>

          <div class="flex rounded-lg overflow-hidden border border-gray-300 dark:border-gray-700 text-sm">
            <button
              v-for="view in ['messages', 'connections', 'raw']"
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
        <div v-else-if="logView === 'messages' && logEntries.length" class="overflow-x-auto">
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
                  <td class="py-2 text-xs text-gray-500 max-w-[320px] truncate">{{ entry.detail }}</td>
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

    <!-- Compose Email Modal -->
    <CardBoxModal
      v-model="isComposeModalActive"
      :title="t('em.newEmail')"
      :button-label="t('em.send')"
      has-cancel
      @confirm="sendEmail"
    >
      <FormField :label="t('em.to')">
        <FormControl v-model="newEmail.to" :placeholder="t('em.toPlaceholder')" />
        <p class="text-xs text-gray-500 mt-1">{{ t('em.multipleRecipientsHint') }}</p>
      </FormField>
      <FormField :label="t('em.subject')">
        <FormControl v-model="newEmail.subject" :placeholder="t('em.subject')" />
      </FormField>
      <FormField :label="t('em.message')">
        <FormControl
          v-model="newEmail.body"
          type="textarea"
          :placeholder="t('em.writeMessage')"
          :rows="8"
        />
      </FormField>
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
