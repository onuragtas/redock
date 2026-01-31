<script setup>
import BaseButton from "@/components/BaseButton.vue";
import BaseIcon from "@/components/BaseIcon.vue";
import CardBox from "@/components/CardBox.vue";
import CardBoxModal from "@/components/CardBoxModal.vue";
import FormControl from "@/components/FormControl.vue";
import FormField from "@/components/FormField.vue";
import SectionTitleLineWithButton from "@/components/SectionTitleLineWithButton.vue";
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
  mdiChevronUp
} from '@mdi/js';
import { computed, onMounted, ref, watch } from "vue";
import { useToast } from 'vue-toastification';

const toast = useToast();

// State
const loading = ref(false);
const activeTab = ref('overview');
const serverStatus = ref({
  is_running: false,
  container_name: '',
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
const serverRunning = computed(() => serverStatus.value.is_running);

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
    toast.error('Failed to load emails');
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

const startServer = async () => {
  loading.value = true;
  try {
    const response = await ApiService.post('/api/email/server/start');
    if (!response.data.error) {
      toast.success('✅ Email server started');
      await loadServerStatus();
    } else {
      toast.error('❌ ' + response.data.msg);
    }
  } catch (error) {
    toast.error('❌ Error: ' + error.message);
  } finally {
    loading.value = false;
  }
};

const stopServer = async () => {
  loading.value = true;
  try {
    const response = await ApiService.post('/api/email/server/stop');
    if (!response.data.error) {
      toast.success('✅ Email server stopped');
      await loadServerStatus();
    } else {
      toast.error('❌ ' + response.data.msg);
    }
  } catch (error) {
    toast.error('❌ Error: ' + error.message);
  } finally {
    loading.value = false;
  }
};

const updateServerIP = async () => {
  if (!serverIPForm.value.ip_address) {
    toast.error('❌ Please enter an IP address');
    return;
  }

  loading.value = true;
  try {
    const response = await ApiService.put('/api/email/server/ip', {
      ip_address: serverIPForm.value.ip_address
    });
    
    if (!response.data.error) {
      toast.success('✅ ' + response.data.msg);
      toast.info('☁️ DNS records are being updated in Cloudflare...', { timeout: 5000 });
      isEditingIP.value = false;
      await loadServerStatus();
    } else {
      toast.error('❌ ' + response.data.msg);
    }
  } catch (error) {
    toast.error('❌ Error: ' + error.message);
  } finally {
    loading.value = false;
  }
};

const addDomain = async () => {
  try {
    const response = await ApiService.post('/api/email/domains', newDomain.value);
    if (!response.data.error) {
      toast.success('✅ Domain added successfully');
      toast.info('☁️ Checking Cloudflare for automatic DNS setup...', { timeout: 3000 });
      await loadDomains();
      isAddDomainModalActive.value = false;
      newDomain.value = { domain: '', description: '' };
    } else {
      toast.error('❌ ' + response.data.msg);
    }
  } catch (error) {
    toast.error('❌ Error: ' + error.message);
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
      toast.success('✅ Domain updated successfully');
      toast.info('☁️ DNS records queued for update...', { timeout: 3000 });
      await loadDomains();
      isEditDomainModalActive.value = false;
    } else {
      toast.error('❌ ' + response.data.msg);
    }
  } catch (error) {
    toast.error('❌ Error: ' + error.message);
  }
};

const deleteDomain = async (domainId) => {
  if (!confirm('Are you sure you want to delete this domain? All associated mailboxes must be deleted first.')) {
    return;
  }
  
  try {
    const response = await ApiService.delete(`/api/email/domains/${domainId}`);
    if (!response.data.error) {
      toast.success('✅ Domain deleted successfully');
      await loadDomains();
    } else {
      toast.error('❌ ' + response.data.msg);
    }
  } catch (error) {
    toast.error('❌ Error: ' + error.message);
  }
};

const openAddMailboxModal = () => {
  if (domains.value.length === 0) {
    toast.error('❌ Please add a domain first before creating mailboxes');
    return;
  }
  isAddMailboxModalActive.value = true;
};

const addMailbox = async () => {
  try {
    if (!newMailbox.value.domain_id) {
      toast.error('❌ Please select a domain');
      return;
    }
    
    const response = await ApiService.post('/api/email/mailboxes', newMailbox.value);
    if (!response.data.error) {
      toast.success('✅ Mailbox created successfully');
      await loadMailboxes();
      isAddMailboxModalActive.value = false;
      newMailbox.value = { domain_id: '', username: '', password: '', name: '' };
    } else {
      toast.error('❌ ' + response.data.msg);
    }
  } catch (error) {
    toast.error('❌ Error: ' + error.message);
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
      toast.success('✅ Mailbox updated successfully');
      toast.info('☁️ DNS records queued for update...', { timeout: 3000 });
      await loadMailboxes();
      isEditMailboxModalActive.value = false;
    } else {
      toast.error('❌ ' + response.data.msg);
    }
  } catch (error) {
    toast.error('❌ Error: ' + error.message);
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
    toast.error('❌ Password is required');
    return;
  }

  if (updatePasswordForm.value.password !== updatePasswordForm.value.confirmPassword) {
    toast.error('❌ Passwords do not match');
    return;
  }

  if (updatePasswordForm.value.password.length < 6) {
    toast.error('❌ Password must be at least 6 characters');
    return;
  }

  try {
    const response = await ApiService.put(
      `/api/email/mailboxes/${selectedMailbox.value.id}/password`,
      { password: updatePasswordForm.value.password }
    );
    
    if (!response.data.error) {
      toast.success('✅ Password updated successfully! You can now send emails.');
      isUpdatePasswordModalActive.value = false;
      updatePasswordForm.value = { password: '', confirmPassword: '' };
    } else {
      toast.error('❌ ' + response.data.msg);
    }
  } catch (error) {
    toast.error('❌ Error: ' + error.message);
  }
};

const deleteMailbox = async (mailboxId) => {
  if (!confirm('Are you sure you want to delete this mailbox? All emails will be permanently deleted.')) {
    return;
  }
  
  try {
    const response = await ApiService.delete(`/api/email/mailboxes/${mailboxId}`);
    if (!response.data.error) {
      toast.success('✅ Mailbox deleted successfully');
      await loadMailboxes();
    } else {
      toast.error('❌ ' + response.data.msg);
    }
  } catch (error) {
    toast.error('❌ Error: ' + error.message);
  }
};

const sendEmail = async () => {
  if (!selectedMailbox.value) {
    toast.error('Önce bir posta kutusu seçin');
    return;
  }
  const toList = newEmail.value.to.split(/[,;]/).map(e => e.trim()).filter(Boolean);
  if (!toList.length) {
    toast.error('En az bir alıcı girin');
    return;
  }
  if (!(newEmail.value.subject || '').trim()) {
    toast.error('Konu girin');
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
      toast.success('✅ E-posta gönderildi');
      isComposeModalActive.value = false;
      newEmail.value = { to: '', subject: '', body: '' };
      loadEmails(selectedMailbox.value, selectedFolder.value);
    } else {
      toast.error('❌ ' + response.data.msg);
    }
  } catch (error) {
    toast.error('❌ Gönderilemedi: ' + (error.response?.data?.msg || error.message));
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
    return 'Yesterday';
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
  toast.info(email.flagged ? '⭐ Yıldızlandı' : 'Yıldız kaldırıldı');
};

const onArchive = () => {
  toast.info('Arşiv özelliği yakında eklenecek.');
};

const onMoveToTrash = () => {
  toast.info('Çöp kutusuna taşıma yakında eklenecek.');
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

onMounted(() => {
  loadData();
});
</script>

<template>
  <div>
    <SectionTitleLineWithButton :icon="mdiEmail" title="Email Server" main>
      <BaseButton
        v-if="!serverRunning"
        :icon="mdiPlay"
        color="success"
        :disabled="loading"
        label="Start Server"
        @click="startServer"
      />
      <BaseButton
        v-else
        :icon="mdiStop"
        color="danger"
        :disabled="loading"
        label="Stop Server"
        @click="stopServer"
      />
    </SectionTitleLineWithButton>

    <!-- Server Status Overview -->
    <div class="grid grid-cols-1 lg:grid-cols-4 gap-6 mb-6">
      <CardBox>
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-gray-500 dark:text-gray-400">Status</p>
            <p class="text-2xl font-semibold mt-1">
              <span :class="serverRunning ? 'text-green-500' : 'text-red-500'">
                {{ serverRunning ? '🟢 Running' : '🔴 Stopped' }}
              </span>
            </p>
          </div>
          <BaseIcon :path="mdiServer" :size="48" class="text-blue-500" />
        </div>
      </CardBox>

      <CardBox>
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-gray-500 dark:text-gray-400">Domains</p>
            <p class="text-2xl font-semibold mt-1">{{ domains.length }}</p>
          </div>
          <BaseIcon :path="mdiDomain" :size="48" class="text-purple-500" />
        </div>
      </CardBox>

      <CardBox>
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-gray-500 dark:text-gray-400">Mailboxes</p>
            <p class="text-2xl font-semibold mt-1">{{ mailboxes.length }}</p>
          </div>
          <BaseIcon :path="mdiAccount" :size="48" class="text-green-500" />
        </div>
      </CardBox>

      <CardBox>
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-gray-500 dark:text-gray-400">Emails</p>
            <p class="text-2xl font-semibold mt-1">{{ totalEmailCount }}</p>
          </div>
          <BaseIcon :path="mdiEmailOpen" :size="48" class="text-orange-500" />
        </div>
      </CardBox>
    </div>

    <!-- Tabs -->
    <div class="mb-6">
      <div class="flex space-x-2 border-b border-gray-200 dark:border-gray-700">
        <button
          v-for="tab in ['overview', 'domains', 'mailboxes', 'webmail']"
          :key="tab"
          :class="[
            'px-4 py-2 font-medium transition-colors',
            activeTab === tab
              ? 'border-b-2 border-blue-500 text-blue-600 dark:text-blue-400'
              : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300'
          ]"
          @click="activeTab = tab"
        >
          {{ tab.charAt(0).toUpperCase() + tab.slice(1) }}
        </button>
      </div>
    </div>

    <!-- Overview Tab -->
    <div v-if="activeTab === 'overview'">
      <CardBox class="mb-6">
        <div class="flex items-center justify-between mb-6">
          <h3 class="text-xl font-semibold">Server Information</h3>
          <BaseButton :icon="mdiRefresh" color="info" small @click="loadServerStatus" />
        </div>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div class="p-4 bg-gray-50 dark:bg-gray-800 rounded">
            <p class="text-sm text-gray-500 dark:text-gray-400">Container</p>
            <p class="font-semibold">{{ serverStatus.container_name || 'N/A' }}</p>
          </div>
          <div class="p-4 bg-gray-50 dark:bg-gray-800 rounded">
            <p class="text-sm text-gray-500 dark:text-gray-400">Hostname</p>
            <p class="font-semibold">{{ serverStatus.hostname || 'N/A' }}</p>
          </div>
          <div class="p-4 bg-gray-50 dark:bg-gray-800 rounded">
            <p class="text-sm text-gray-500 dark:text-gray-400">SMTP Port</p>
            <p class="font-semibold">{{ serverStatus.smtp_port }}</p>
          </div>
          <div class="p-4 bg-gray-50 dark:bg-gray-800 rounded">
            <p class="text-sm text-gray-500 dark:text-gray-400">IMAP Port</p>
            <p class="font-semibold">{{ serverStatus.imap_port }} / {{ serverStatus.imaps_port }}</p>
          </div>
        </div>
      </CardBox>

      <!-- Server IP Configuration -->
      <CardBox class="mb-6">
        <div class="flex items-center justify-between mb-6">
          <div class="flex items-center gap-3">
            <BaseIcon :path="mdiCog" class="text-blue-600" w="w-6" h="h-6" />
            <h3 class="text-xl font-semibold">Server Configuration</h3>
          </div>
          <BaseButton
            v-if="!isEditingIP"
            :icon="mdiPencil"
            color="info"
            small
            label="Edit IP"
            @click="isEditingIP = true"
          />
        </div>

        <div class="space-y-4">
          <!-- Current IP Display -->
          <div v-if="!isEditingIP" class="p-4 bg-gray-50 dark:bg-gray-800 rounded-lg">
            <div class="flex items-center justify-between">
              <div>
                <p class="text-sm text-gray-500 dark:text-gray-400 mb-1">Public IP Address</p>
                <p class="text-lg font-mono font-semibold">
                  {{ serverStatus.ip_address || '🔄 Auto-detecting...' }}
                </p>
                <p v-if="serverStatus.ip_address" class="text-xs text-gray-500 mt-2">
                  💡 This IP is used in SPF records for email authentication
                </p>
              </div>
              <div v-if="serverStatus.ip_address" class="text-right">
                <span class="inline-flex items-center px-3 py-1 rounded-full bg-green-100 text-green-800 dark:bg-green-900/20 dark:text-green-400 text-sm font-medium">
                  ✅ Configured
                </span>
              </div>
            </div>
          </div>

          <!-- IP Edit Form -->
          <div v-else class="p-4 bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg">
            <FormField label="Public IP Address" help="Your server's public IPv4 address for SPF/DNS records">
              <FormControl
                v-model="serverIPForm.ip_address"
                type="text"
                placeholder="e.g., 157.180.1.14"
                :icon="mdiCog"
              />
            </FormField>
            <div class="flex gap-2 mt-4">
              <BaseButton
                color="success"
                label="Save IP"
                :disabled="!serverIPForm.ip_address"
                @click="updateServerIP"
              />
              <BaseButton
                color="danger"
                outline
                label="Cancel"
                @click="isEditingIP = false; serverIPForm.ip_address = serverStatus.ip_address"
              />
            </div>
            <p class="text-xs text-blue-700 dark:text-blue-300 mt-3">
              💡 <strong>Auto-Detection:</strong> System automatically detects your public IP on startup (via ifconfig.me). You can manually override it here if needed.
            </p>
          </div>
        </div>
      </CardBox>

      <!-- Cloudflare Integration Status -->
      <CardBox>
        <div class="flex items-center justify-between mb-6">
          <div class="flex items-center gap-3">
            <BaseIcon :path="mdiCloud" class="text-blue-600" w="w-6" h="h-6" />
            <h3 class="text-xl font-semibold">Cloudflare Integration</h3>
          </div>
        </div>

        <div class="space-y-4">
          <div class="p-4 bg-gradient-to-r from-blue-50 to-cyan-50 dark:from-blue-900/20 dark:to-cyan-900/20 border border-blue-200 dark:border-blue-800 rounded-lg">
            <div class="flex items-start gap-4">
              <BaseIcon :path="mdiInformationOutline" class="text-blue-600 dark:text-blue-400 flex-shrink-0 mt-1" w="w-6" h="h-6" />
              <div class="flex-1">
                <h4 class="font-semibold text-blue-900 dark:text-blue-100 mb-2">
                  🚀 Automatic DNS Configuration
                </h4>
                <p class="text-sm text-blue-700 dark:text-blue-300 mb-3">
                  When you add a new email domain, the system automatically creates DNS records in Cloudflare if the domain exists in your account.
                </p>
                <div class="space-y-2 text-sm text-blue-800 dark:text-blue-200">
                  <div class="flex items-center gap-2">
                    <span class="font-mono bg-blue-100 dark:bg-blue-800 px-2 py-0.5 rounded">SPF</span>
                    <span>Sender Policy Framework record</span>
                  </div>
                  <div class="flex items-center gap-2">
                    <span class="font-mono bg-blue-100 dark:bg-blue-800 px-2 py-0.5 rounded">DKIM</span>
                    <span>DomainKeys Identified Mail with RSA key</span>
                  </div>
                  <div class="flex items-center gap-2">
                    <span class="font-mono bg-blue-100 dark:bg-blue-800 px-2 py-0.5 rounded">DMARC</span>
                    <span>Domain-based Message Authentication</span>
                  </div>
                  <div class="flex items-center gap-2">
                    <span class="font-mono bg-blue-100 dark:bg-blue-800 px-2 py-0.5 rounded">MX</span>
                    <span>Mail Exchange record</span>
                  </div>
                </div>
                <div class="mt-4 pt-4 border-t border-blue-200 dark:border-blue-700">
                  <p class="text-sm text-blue-700 dark:text-blue-300">
                    💡 <strong>Setup:</strong> Add your Cloudflare API token in the 
                    <router-link to="/cloudflare" class="underline hover:text-blue-900 dark:hover:text-blue-100">
                      Cloudflare settings
                    </router-link>
                    and sync your zones to enable this feature.
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
          <h3 class="text-xl font-semibold">Email Domains</h3>
          <BaseButton
            :icon="mdiPlus"
            color="success"
            label="Add Domain"
            @click="isAddDomainModalActive = true"
          />
        </div>

        <div v-if="domains.length === 0" class="text-center py-12 text-gray-500">
          No domains yet. Add your first domain!
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
                <span :class="[
                  'px-3 py-1 rounded-full text-sm font-medium',
                  domain.enabled ? 'bg-green-100 text-green-800' : 'bg-red-100 text-red-800'
                ]">
                  {{ domain.enabled ? '✅ Active' : '❌ Disabled' }}
                </span>
                <BaseButton
                  :icon="mdiPencil"
                  color="info"
                  small
                  label="Edit"
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
          <h3 class="text-xl font-semibold">Mailboxes</h3>
          <BaseButton
            :icon="mdiEmailPlus"
            color="success"
            label="Create Mailbox"
            :disabled="domains.length === 0"
            @click="openAddMailboxModal"
          />
        </div>
        
        <div v-if="domains.length === 0" class="mb-4 p-4 bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-lg">
          <p class="text-yellow-800 dark:text-yellow-200">
            ⚠️ Please add a domain first before creating mailboxes.
          </p>
        </div>

        <div v-if="mailboxes.length === 0" class="text-center py-12 text-gray-500">
          No mailboxes yet. Create your first mailbox!
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
              <p>Messages: {{ mailbox.message_count || 0 }}</p>
              <p>Last Login: {{ formatDate(mailbox.last_login) || 'Never' }}</p>
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
          <option value="" disabled>Select an email account</option>
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
            label="Yeni e-posta"
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
              <h3 class="font-semibold text-lg">{{ folders.find(f => f.value === selectedFolder)?.name || 'Inbox' }}</h3>
              <span class="text-sm text-gray-500">({{ totalEmailCount }} e-posta)</span>
            </div>
            <BaseButton :icon="mdiRefresh" color="light" small @click="loadEmails(selectedMailbox, selectedFolder)" />
          </div>

          <!-- Loading State -->
          <div v-if="loading" class="flex-1 flex items-center justify-center text-gray-500">
            <div class="text-center">
              <div class="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600 mx-auto mb-4"></div>
              <p>E-postalar yükleniyor...</p>
            </div>
          </div>

          <!-- Empty State -->
          <div v-else-if="threads.length === 0" class="flex-1 flex items-center justify-center text-gray-500">
            <div class="text-center">
              <BaseIcon :path="mdiEmailOpen" class="w-16 h-16 mx-auto mb-4 text-gray-300" />
              <p class="text-lg font-medium mb-2">{{ selectedFolder }} klasöründe e-posta yok</p>
              <p class="text-sm">Bu klasör şu an boş</p>
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
                        {{ email.from || 'Bilinmeyen' }}
                      </p>
                      <span class="text-xs text-gray-500 ml-2 shrink-0">{{ formatTime(email.date) }}</span>
                    </div>
                    <p :class="['text-sm truncate mb-1', !email.seen ? 'font-semibold text-gray-900 dark:text-white' : 'text-gray-600 dark:text-gray-400']">
                      {{ email.subject || '(Konu yok)' }}
                    </p>
                    <p class="text-sm text-gray-500 dark:text-gray-500 truncate">
                      {{ email.snippet || (email.body_plain || '').substring(0, 100) || 'Önizleme yok' }}
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
                    :aria-label="expandedThreadIds.has(thread.thread_id) ? 'Daralt' : 'Genişlet'"
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
                        {{ thread.messages[0]?.from || 'Bilinmeyen' }}
                      </p>
                      <span class="text-xs text-gray-500 ml-2 shrink-0">{{ formatTime(thread.date) }}</span>
                    </div>
                    <p class="text-sm truncate mb-1 font-medium text-gray-600 dark:text-gray-400">
                      {{ thread.subject || '(Konu yok)' }}
                      <span class="text-gray-500 dark:text-gray-500 font-normal">({{ thread.count }})</span>
                    </p>
                    <p class="text-sm text-gray-500 truncate">
                      {{ (thread.messages[0]?.body_plain || '').substring(0, 80) || 'Önizleme yok' }}
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
                      <p class="text-xs text-gray-500 truncate">{{ email.subject || '(Konu yok)' }}</p>
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
                <h2 class="text-xl font-semibold flex-1 pr-4">{{ selectedEmail.subject || '(Konu yok)' }}</h2>
                <button @click="showEmailDetail = false" class="text-gray-500 hover:text-gray-700 dark:hover:text-gray-300">
                  <BaseIcon :path="mdiArrowLeft" w="w-6" h="h-6" />
                </button>
              </div>
              <div class="flex items-center gap-2">
                <BaseButton :icon="mdiReply" label="Yanıtla" color="light" small @click="openReplyCompose(false)" />
                <BaseButton :icon="mdiReplyAll" label="Tümünü yanıtla" color="light" small @click="openReplyCompose(true)" />
                <BaseButton :icon="mdiArchive" label="Arşivle" color="light" small @click="onArchive" />
                <BaseButton :icon="mdiTrashCan" label="Çöpe taşı" color="danger" small @click="onMoveToTrash" />
              </div>
            </div>

            <!-- Konu zinciri (orijinal + cevaplar) + mail içindeki alıntı -->
            <div class="flex-1 overflow-y-auto px-6 py-4 space-y-6">
              <template v-if="threadLoading">
                <p class="text-sm text-gray-500">Yükleniyor...</p>
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
                    <div v-html="selectedEmail.body_html" class="prose prose-sm dark:prose-invert max-w-none email-quoted"></div>
                  </div>
                </CardBox>
                <!-- Plain gövde: "On ... wrote:" bloklarına göre her biri ayrı CardBox, açılır -->
                <template v-else>
                  <CardBox
                    v-for="(seg, idx) in bodySegments"
                    :key="idx"
                    class="border-l-4 border-blue-500"
                  >
                    <button
                      type="button"
                      class="w-full flex items-center gap-3 mb-0 text-left cursor-pointer hover:opacity-90"
                      @click="toggleThreadCard('seg-' + idx)"
                    >
                      <div class="w-8 h-8 rounded-full bg-gradient-to-br from-blue-500 to-purple-600 flex items-center justify-center text-white text-sm font-bold shrink-0">
                        {{ idx === 0 ? getInitials(selectedEmail.from) : '…' }}
                      </div>
                      <div class="flex-1 min-w-0">
                        <p v-if="idx === 0" class="font-medium text-sm truncate">{{ selectedEmail.from }}</p>
                        <p v-else class="font-medium text-sm truncate text-gray-600 dark:text-gray-400">{{ seg.header }}</p>
                        <p v-if="idx === 0" class="text-xs text-gray-500">{{ formatDate(selectedEmail.date) }}</p>
                      </div>
                      <BaseIcon
                        :path="expandedCardKeys.has('seg-' + idx) ? mdiChevronUp : mdiChevronDown"
                        class="shrink-0 text-gray-400"
                        w="w-5"
                        h="h-5"
                      />
                    </button>
                    <div v-show="expandedCardKeys.has('seg-' + idx)" class="email-body-content text-sm mt-3 pt-3 border-t border-gray-200 dark:border-gray-600">
                      <div v-if="seg.content" v-html="plainTextToHtml(seg.content)" class="prose prose-sm dark:prose-invert max-w-none email-body-plain email-quoted"></div>
                      <p v-else class="text-gray-500 dark:text-gray-400">İçerik yok</p>
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
                  <button
                    type="button"
                    class="w-full flex items-center gap-3 mb-0 text-left cursor-pointer hover:opacity-90"
                    @click="toggleThreadCard('msg-' + msg.uid)"
                  >
                    <div class="w-8 h-8 rounded-full bg-gradient-to-br from-blue-500 to-purple-600 flex items-center justify-center text-white text-sm font-bold shrink-0">
                      {{ getInitials(msg.from) }}
                    </div>
                    <div class="flex-1 min-w-0">
                      <p class="font-medium text-sm truncate">{{ msg.from }}</p>
                      <p class="text-xs text-gray-500">{{ formatDate(msg.date) }}</p>
                    </div>
                    <BaseIcon
                      :path="expandedCardKeys.has('msg-' + msg.uid) ? mdiChevronUp : mdiChevronDown"
                      class="shrink-0 text-gray-400"
                      w="w-5"
                      h="h-5"
                    />
                  </button>
                  <div v-show="expandedCardKeys.has('msg-' + msg.uid)" class="email-body-content text-sm mt-3 pt-3 border-t border-gray-200 dark:border-gray-600">
                    <div v-if="msg.body_html" v-html="msg.body_html" class="prose prose-sm dark:prose-invert max-w-none email-quoted"></div>
                    <template v-else>
                      <div v-if="msg.body_plain" v-html="plainTextToHtml(msg.body_plain)" class="prose prose-sm dark:prose-invert max-w-none email-body-plain email-quoted"></div>
                      <p v-else class="text-gray-500 dark:text-gray-400">İçerik yok</p>
                    </template>
                  </div>
                </CardBox>
              </template>
            </div>

            <!-- Attachments -->
            <div v-if="selectedEmail.attachments && selectedEmail.attachments.length > 0" class="px-6 py-4 border-t border-gray-200 dark:border-gray-700">
              <h4 class="font-semibold mb-3">Attachments ({{ selectedEmail.attachments.length }})</h4>
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
                  <BaseButton label="Download" color="light" small />
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
          <p class="text-lg font-medium mb-2">E-posta hesabı seçin</p>
          <p class="text-sm">E-postaları okumak için yukarıdan bir hesap seçin</p>
        </div>
      </div>
    </div>

    <!-- Add Domain Modal -->
    <CardBoxModal
      v-model="isAddDomainModalActive"
      title="Add Email Domain"
      button-label="Add"
      has-cancel
      @confirm="addDomain"
    >
      <!-- Cloudflare Auto DNS Info -->
      <div class="mb-4 p-4 bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg">
        <div class="flex items-start gap-3">
          <BaseIcon :path="mdiCloud" class="text-blue-600 dark:text-blue-400 flex-shrink-0 mt-0.5" w="w-5" h="h-5" />
          <div class="flex-1">
            <h4 class="font-semibold text-blue-900 dark:text-blue-100 mb-1">
              🚀 Cloudflare Auto DNS
            </h4>
            <p class="text-sm text-blue-700 dark:text-blue-300">
              If this domain exists in your Cloudflare account, SPF, DKIM, and DMARC records will be automatically created for email authentication.
            </p>
          </div>
        </div>
      </div>

      <FormField label="Domain">
        <FormControl v-model="newDomain.domain" placeholder="example.com" required />
      </FormField>
      <FormField label="Description">
        <FormControl v-model="newDomain.description" placeholder="Optional description" />
      </FormField>
    </CardBoxModal>

    <!-- Add Mailbox Modal -->
    <CardBoxModal
      v-model="isAddMailboxModalActive"
      title="Create Mailbox"
      button-label="Create"
      has-cancel
      @confirm="addMailbox"
    >
      <FormField label="Domain">
        <select
          v-model.number="newMailbox.domain_id"
          class="px-3 py-2 max-w-full focus:ring focus:outline-none border-gray-700 rounded w-full h-12 border bg-white dark:bg-slate-800"
          required
        >
          <option value="" disabled selected>{{ domains.length === 0 ? 'No domains available - Add a domain first' : 'Select Domain' }}</option>
          <option v-for="domain in domains" :key="domain.id" :value="domain.id">
            {{ domain.domain }}
          </option>
        </select>
      </FormField>
      <FormField label="Username">
        <FormControl v-model="newMailbox.username" placeholder="username" required />
      </FormField>
      <FormField label="Password">
        <FormControl v-model="newMailbox.password" type="password" placeholder="Password" required />
      </FormField>
      <FormField label="Display Name">
        <FormControl v-model="newMailbox.name" placeholder="John Doe" />
      </FormField>
    </CardBoxModal>

    <!-- Compose Email Modal -->
    <CardBoxModal
      v-model="isComposeModalActive"
      title="Yeni e-posta"
      button-label="Gönder"
      has-cancel
      @confirm="sendEmail"
    >
      <FormField label="Alıcı">
        <FormControl v-model="newEmail.to" placeholder="alici@example.com (virgülle birden fazla)" />
        <p class="text-xs text-gray-500 mt-1">Birden fazla alıcı için virgül veya noktalı virgül kullanın</p>
      </FormField>
      <FormField label="Konu">
        <FormControl v-model="newEmail.subject" placeholder="Konu" />
      </FormField>
      <FormField label="Mesaj">
        <FormControl
          v-model="newEmail.body"
          type="textarea"
          placeholder="Mesajınızı yazın..."
          :rows="8"
        />
      </FormField>
    </CardBoxModal>

    <!-- Update Password Modal -->
    <CardBoxModal
      v-model="isUpdatePasswordModalActive"
      title="Update Mailbox Password"
      button-label="Update Password"
      has-cancel
      @confirm="updateMailboxPassword"
    >
      <div v-if="selectedMailbox" class="mb-4 p-3 bg-blue-50 dark:bg-blue-900/20 rounded-lg">
        <p class="text-sm text-blue-800 dark:text-blue-200">
          <strong>Mailbox:</strong> {{ selectedMailbox.email }}
        </p>
        <p class="text-xs text-blue-600 dark:text-blue-400 mt-1">
          💡 After updating the password, you'll be able to send emails from this mailbox.
        </p>
      </div>
      
      <FormField label="New Password">
        <FormControl
          v-model="updatePasswordForm.password"
          type="password"
          placeholder="Enter new password"
          autocomplete="new-password"
          required
        />
      </FormField>
      
      <FormField label="Confirm Password">
        <FormControl
          v-model="updatePasswordForm.confirmPassword"
          type="password"
          placeholder="Confirm new password"
          autocomplete="new-password"
          required
        />
      </FormField>
      
      <div class="mt-4 p-3 bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-lg">
        <p class="text-xs text-yellow-800 dark:text-yellow-200">
          <strong>Why update password?</strong><br>
          If you're seeing "password not found in cache" errors when sending emails, 
          updating the password will fix it. This can happen after a server restart.
        </p>
      </div>
    </CardBoxModal>

    <!-- Edit Domain Modal -->
    <CardBoxModal
      v-model="isEditDomainModalActive"
      title="Edit Domain"
      button-label="Save Changes"
      has-cancel
      @confirm="editDomain"
    >
      <div v-if="selectedDomain" class="mb-4 p-3 bg-blue-50 dark:bg-blue-900/20 rounded-lg">
        <p class="text-sm text-blue-800 dark:text-blue-200">
          <strong>Domain:</strong> {{ selectedDomain.domain }}
        </p>
      </div>

      <FormField label="Description">
        <FormControl v-model="editDomainForm.description" placeholder="Domain description" />
      </FormField>
      
      <FormField label="Status">
        <label class="flex items-center space-x-3 cursor-pointer">
          <input
            v-model="editDomainForm.enabled"
            type="checkbox"
            class="w-5 h-5 text-blue-600 rounded focus:ring-2 focus:ring-blue-500"
          />
          <span class="text-sm font-medium">Enable Domain</span>
        </label>
      </FormField>

      <div class="mt-4 p-3 bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800 rounded-lg">
        <p class="text-xs text-green-800 dark:text-green-200">
          ☁️ DNS records (SPF, DKIM, DMARC, MX) will be automatically updated in Cloudflare after saving.
        </p>
      </div>
    </CardBoxModal>

    <!-- Edit Mailbox Modal -->
    <CardBoxModal
      v-model="isEditMailboxModalActive"
      title="Edit Mailbox Settings"
      button-label="Save Changes"
      has-cancel
      @confirm="editMailbox"
    >
      <div v-if="selectedMailbox" class="mb-4 p-3 bg-blue-50 dark:bg-blue-900/20 rounded-lg">
        <p class="text-sm text-blue-800 dark:text-blue-200">
          <strong>Mailbox:</strong> {{ selectedMailbox.email }}
        </p>
      </div>

      <FormField label="Display Name">
        <FormControl v-model="editMailboxForm.name" placeholder="John Doe" />
      </FormField>
      
      <FormField label="Quota (bytes)">
        <FormControl
          v-model.number="editMailboxForm.quota"
          type="number"
          placeholder="10737418240"
        />
        <p class="text-xs text-gray-500 mt-1">Default: 10GB = 10737418240 bytes</p>
      </FormField>
      
      <FormField label="Forward To (optional)">
        <FormControl
          v-model="editMailboxForm.forward_to"
          type="email"
          placeholder="forward@example.com"
        />
      </FormField>
      
      <FormField label="Auto Reply Message (optional)">
        <FormControl
          v-model="editMailboxForm.auto_reply_msg"
          type="textarea"
          placeholder="I'm out of office..."
        />
      </FormField>
      
      <FormField label="New Password (optional)">
        <FormControl
          v-model="editMailboxForm.password"
          type="password"
          placeholder="Leave empty to keep current"
        />
      </FormField>
      
      <FormField label="Status">
        <label class="flex items-center space-x-3 cursor-pointer mb-3">
          <input
            v-model="editMailboxForm.enabled"
            type="checkbox"
            class="w-5 h-5 text-blue-600 rounded focus:ring-2 focus:ring-blue-500"
          />
          <span class="text-sm font-medium">Enable Mailbox</span>
        </label>
        
        <label class="flex items-center space-x-3 cursor-pointer mb-3">
          <input
            v-model="editMailboxForm.keep_copy"
            type="checkbox"
            class="w-5 h-5 text-blue-600 rounded focus:ring-2 focus:ring-blue-500"
          />
          <span class="text-sm font-medium">Keep Copy When Forwarding</span>
        </label>
        
        <label class="flex items-center space-x-3 cursor-pointer">
          <input
            v-model="editMailboxForm.auto_reply"
            type="checkbox"
            class="w-5 h-5 text-blue-600 rounded focus:ring-2 focus:ring-blue-500"
          />
          <span class="text-sm font-medium">Enable Auto Reply</span>
        </label>
      </FormField>

      <div class="mt-4 p-3 bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800 rounded-lg">
        <p class="text-xs text-green-800 dark:text-green-200">
          ☁️ DNS records will be automatically updated in Cloudflare after saving.
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
