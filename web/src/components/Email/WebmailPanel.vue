<script setup>
/**
 * The mail reading surface: accounts, folders, a message list and a reader.
 *
 * It owns its own state rather than sharing the page's. The page's
 * `selectedMailbox` is a mailbox *object* while the mailbox editor is open and
 * a mailbox *id* while reading mail, and the two meanings used to collide in
 * the same ref — opening the password editor left webmail requesting
 * /mailboxes/[object Object]/emails.
 */
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useToast } from 'vue-toastification';

import BaseButton from '@/components/BaseButton.vue';
import BaseIcon from '@/components/BaseIcon.vue';
import CardBoxModal from '@/components/CardBoxModal.vue';
import FormControl from '@/components/FormControl.vue';
import FormField from '@/components/FormField.vue';
import ApiService from '@/services/ApiService';

import {
  mdiAlertOctagon, mdiArchive, mdiArrowLeft, mdiAttachment, mdiCheckAll, mdiChevronDown,
  mdiChevronUp, mdiClose, mdiDownload, mdiEmailOpenOutline, mdiEmailOutline, mdiFolderOutline,
  mdiInbox, mdiMagnify, mdiPencil, mdiRefresh, mdiReply, mdiReplyAll, mdiSend, mdiStar,
  mdiStarOutline, mdiTrashCan, mdiContentSave, mdiEmailAlertOutline
} from '@mdi/js';

const props = defineProps({
  mailboxes: { type: Array, default: () => [] }
});

const { t, locale } = useI18n();
const toast = useToast();

/* ------------------------------------------------------------------ state */

const activeMailboxId = ref(null);
const activeFolder = ref('INBOX');
const folders = ref([]);
const threads = ref([]);
const loading = ref(false);
const busy = ref(false);

const search = ref('');
const unreadOnly = ref(false);

const openEmail = ref(null);
const threadMessages = ref([]);
const threadLoading = ref(false);
const attachments = ref([]);
const collapsedMessages = ref(new Set());
const expandedThreads = ref(new Set());
const selection = ref(new Set());

const composeOpen = ref(false);
const compose = ref(emptyCompose());
const showCc = ref(false);
const showBcc = ref(false);

const activeMailbox = computed(() =>
  props.mailboxes.find((mailbox) => mailbox.id === activeMailboxId.value) || null
);

/* ------------------------------------------------------------- formatting */

const FOLDER_LOOKS = {
  INBOX: { icon: mdiInbox, tone: 'text-sky-500' },
  Sent: { icon: mdiSend, tone: 'text-emerald-500' },
  Drafts: { icon: mdiPencil, tone: 'text-amber-500' },
  Junk: { icon: mdiEmailAlertOutline, tone: 'text-orange-500' },
  Spam: { icon: mdiAlertOctagon, tone: 'text-red-500' },
  Trash: { icon: mdiTrashCan, tone: 'text-slate-400' },
  Archive: { icon: mdiArchive, tone: 'text-violet-500' }
};

const folderLook = (name) => FOLDER_LOOKS[name] || { icon: mdiFolderOutline, tone: 'text-slate-400' };

const folderLabel = (value) => {
  const known = folders.value.find((folder) => folder.value === value);
  return known ? known.label : value;
};

// Sent and Drafts are the folders where every message is from the account
// owner, so the name worth showing in the list is the recipient's.
const OUTGOING = ['Sent', 'Drafts'];
const isOutgoing = computed(() => OUTGOING.includes(activeFolder.value));

const addressOnly = (value) => {
  if (!value || typeof value !== 'string') return '';
  const angled = value.match(/<([^>]+)>/);
  return (angled ? angled[1] : value).trim();
};

const displayName = (value) => {
  if (!value) return t('em.unknown');
  const named = value.match(/^\s*"?([^"<]+?)"?\s*</);
  if (named && named[1].trim()) return named[1].trim();
  return addressOnly(value) || value;
};

const correspondent = (email) => displayName(isOutgoing.value ? email.to : email.from);

const initials = (value) => {
  const name = displayName(value);
  const parts = name.split(/[\s._@-]+/).filter(Boolean);
  if (!parts.length) return '?';
  return (parts.length === 1 ? parts[0].slice(0, 2) : parts[0][0] + parts[1][0]).toUpperCase();
};

// A stable colour per correspondent makes the list scannable without adding a
// legend for the reader to learn.
const AVATAR_TONES = [
  'bg-sky-500', 'bg-emerald-500', 'bg-violet-500', 'bg-amber-500',
  'bg-rose-500', 'bg-teal-500', 'bg-indigo-500', 'bg-orange-500'
];

const avatarTone = (value) => {
  const key = addressOnly(value) || String(value || '');
  let hash = 0;
  for (let i = 0; i < key.length; i++) hash = (hash * 31 + key.charCodeAt(i)) >>> 0;
  return AVATAR_TONES[hash % AVATAR_TONES.length];
};

const formatFullDate = (value) => (value ? new Date(value).toLocaleString(locale.value) : '');

const formatListDate = (value) => {
  if (!value) return '';
  const date = new Date(value);
  const now = new Date();
  const sameDay = date.toDateString() === now.toDateString();
  if (sameDay) return date.toLocaleTimeString(locale.value, { hour: '2-digit', minute: '2-digit' });

  const yesterday = new Date(now);
  yesterday.setDate(now.getDate() - 1);
  if (date.toDateString() === yesterday.toDateString()) return t('em.yesterday');

  if (now - date < 7 * 24 * 60 * 60 * 1000) {
    return date.toLocaleDateString(locale.value, { weekday: 'short' });
  }
  if (date.getFullYear() === now.getFullYear()) {
    return date.toLocaleDateString(locale.value, { day: 'numeric', month: 'short' });
  }
  return date.toLocaleDateString(locale.value, { year: 'numeric', month: 'short', day: 'numeric' });
};

const formatSize = (bytes) => {
  if (!bytes) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB'];
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return `${Math.round((bytes / Math.pow(1024, index)) * 10) / 10} ${units[index]}`;
};

const snippetOf = (email) =>
  (email.snippet || email.body_plain || '').replace(/\s+/g, ' ').trim().slice(0, 140);

/* ------------------------------------------------------------- quoted text */

const escapeHtml = (value) => String(value)
  .replace(/&/g, '&amp;').replace(/</g, '&lt;')
  .replace(/>/g, '&gt;').replace(/"/g, '&quot;');

// Quoted lines become a blockquote so a long reply chain reads as history
// rather than as part of the message.
const plainToHtml = (plain) => {
  if (!plain || typeof plain !== 'string') return '';
  const out = [];
  let quote = [];
  const flush = () => {
    if (!quote.length) return;
    out.push(`<blockquote class="wm-quote">${quote
      .map((line) => escapeHtml(line.replace(/^(>\s*)+/, '')))
      .join('<br/>')}</blockquote>`);
    quote = [];
  };
  for (const line of plain.split('\n')) {
    if (line.startsWith('>')) quote.push(line);
    else { flush(); out.push(`${escapeHtml(line)}<br/>`); }
  }
  flush();
  return out.join('');
};

/* ----------------------------------------------------------------- loading */

const requestParams = () => ({ folder: activeFolder.value });

const loadFolders = async () => {
  if (!activeMailboxId.value) return;
  try {
    const response = await ApiService.get(`/api/email/mailboxes/${activeMailboxId.value}/folders`);
    if (response.data.error) return;

    folders.value = (response.data.data || [])
      .filter((folder) => !folder.no_select)
      .map((folder) => {
        const name = folder.name.startsWith('.') ? folder.name.slice(1) : folder.name;
        return {
          value: folder.name,
          label: name === 'INBOX' ? t('em.inbox') : name,
          ...folderLook(name),
          total: folder.message_count || 0,
          unseen: folder.unseen_count || 0
        };
      });
  } catch (error) {
    toast.error(error.response?.data?.msg || t('em.failedToLoadEmails'));
  }
};

const loadEmails = async () => {
  if (!activeMailboxId.value) return;
  loading.value = true;
  try {
    const response = await ApiService.get(`/api/email/mailboxes/${activeMailboxId.value}/emails`, {
      params: { folder: activeFolder.value, limit: 100 }
    });
    if (!response.data.error) {
      threads.value = response.data.data || [];
      selection.value = new Set();
      expandedThreads.value = new Set();
    }
  } catch (error) {
    toast.error(error.response?.data?.msg || t('em.failedToLoadEmails'));
  } finally {
    loading.value = false;
  }
};

const refresh = async () => {
  await Promise.all([loadEmails(), loadFolders()]);
};

/* -------------------------------------------------------------- the list */

// Search filters what has been loaded rather than asking the server, which has
// no search endpoint; the label says so, so nobody reads an empty result as
// "the account has no such mail".
const visibleThreads = computed(() => {
  const needle = search.value.trim().toLowerCase();

  return threads.value
    .map((thread) => {
      let messages = thread.messages || [];
      if (unreadOnly.value) messages = messages.filter((email) => !email.seen);
      if (needle) {
        messages = messages.filter((email) =>
          [email.from, email.to, email.subject, email.body_plain, email.snippet]
            .some((field) => (field || '').toLowerCase().includes(needle))
        );
      }
      return messages.length ? { ...thread, messages, count: messages.length } : null;
    })
    .filter(Boolean);
});

const visibleMessages = computed(() => visibleThreads.value.flatMap((thread) => thread.messages));

const shownCount = computed(() => visibleMessages.value.length);
const filtering = computed(() => Boolean(search.value.trim()) || unreadOnly.value);

const leadOf = (thread) => thread.messages[thread.messages.length - 1];
const threadUnread = (thread) => thread.messages.some((email) => !email.seen);

const toggleThread = (threadId) => {
  const next = new Set(expandedThreads.value);
  next.has(threadId) ? next.delete(threadId) : next.add(threadId);
  expandedThreads.value = next;
};

/* --------------------------------------------------------------- selection */

const toggleSelected = (uid) => {
  const next = new Set(selection.value);
  next.has(uid) ? next.delete(uid) : next.add(uid);
  selection.value = next;
};

const allSelected = computed(() =>
  shownCount.value > 0 && visibleMessages.value.every((email) => selection.value.has(email.uid))
);

const toggleSelectAll = () => {
  selection.value = allSelected.value
    ? new Set()
    : new Set(visibleMessages.value.map((email) => email.uid));
};

const selectedMessages = () => visibleMessages.value.filter((email) => selection.value.has(email.uid));

/* ----------------------------------------------------------------- reading */

const readMessage = async (email) => {
  openEmail.value = email;
  attachments.value = [];
  threadMessages.value = [];
  collapsedMessages.value = new Set();

  if (!email.seen) setFlag(email, 'seen', true);
  loadAttachments(email);

  threadLoading.value = true;
  try {
    const response = await ApiService.get(`/api/email/mailboxes/${activeMailboxId.value}/thread`, {
      params: { folder: activeFolder.value, uid: email.uid }
    });
    if (!response.data.error) {
      threadMessages.value = response.data.data || [];
      // Older messages in a chain start collapsed; the one being read is open.
      collapsedMessages.value = new Set(
        threadMessages.value.slice(0, -1).map((message) => message.uid)
      );
    }
  } catch (error) {
    console.error('Failed to load the conversation:', error);
  } finally {
    threadLoading.value = false;
  }
};

const closeReader = () => {
  openEmail.value = null;
  threadMessages.value = [];
  attachments.value = [];
};

const toggleMessageBody = (uid) => {
  const next = new Set(collapsedMessages.value);
  next.has(uid) ? next.delete(uid) : next.add(uid);
  collapsedMessages.value = next;
};

const readerMessages = computed(() => {
  if (threadMessages.value.length) return threadMessages.value;
  return openEmail.value ? [openEmail.value] : [];
});

const loadAttachments = async (email) => {
  if (!email?.has_attachments) return;
  try {
    const response = await ApiService.get(
      `/api/email/mailboxes/${activeMailboxId.value}/messages/${email.uid}/attachments`,
      { params: requestParams() }
    );
    if (!response.data.error) attachments.value = response.data.data || [];
  } catch (error) {
    console.error('Failed to list attachments:', error);
  }
};

const saveBlob = (data, filename) => {
  const url = window.URL.createObjectURL(new Blob([data]));
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  link.remove();
  window.URL.revokeObjectURL(url);
};

const downloadAttachment = async (attachment) => {
  try {
    const response = await ApiService.get(
      `/api/email/mailboxes/${activeMailboxId.value}/messages/${openEmail.value.uid}/attachments/${attachment.index}`,
      { params: requestParams(), options: { responseType: 'blob' } }
    );
    saveBlob(response.data, attachment.filename || 'attachment');
  } catch (error) {
    toast.error(error.response?.data?.msg || t('em.attachmentFailed'));
  }
};

const downloadRaw = async () => {
  try {
    const response = await ApiService.get(
      `/api/email/mailboxes/${activeMailboxId.value}/messages/${openEmail.value.uid}/raw`,
      { params: requestParams(), options: { responseType: 'blob' } }
    );
    saveBlob(response.data, `message-${openEmail.value.uid}.eml`);
  } catch (error) {
    toast.error(error.response?.data?.msg || t('em.attachmentFailed'));
  }
};

/* ----------------------------------------------------------------- actions */

// Starring from the list used to flip a local flag and show a toast without
// telling the server, so the star was gone on the next refresh.
const setFlag = async (email, flag, value) => {
  try {
    await ApiService.put(
      `/api/email/mailboxes/${activeMailboxId.value}/messages/${email.uid}/flag`,
      { flag, value },
      { options: { params: requestParams() } }
    );
    if (flag === 'seen') email.seen = value;
    if (flag === 'flagged') email.flagged = value;
    if (flag === 'seen') loadFolders();
  } catch (error) {
    toast.error(error.response?.data?.msg || t('em.messageActionFailed'));
  }
};

const moveMessage = async (email, destination) => {
  await ApiService.post(
    `/api/email/mailboxes/${activeMailboxId.value}/messages/${email.uid}/move`,
    { to: destination },
    { options: { params: requestParams() } }
  );
};

const deleteMessage = async (email) => {
  await ApiService.delete(
    `/api/email/mailboxes/${activeMailboxId.value}/messages/${email.uid}`,
    { params: requestParams() }
  );
};

// Every bulk action runs over a snapshot of the selection and refreshes once at
// the end, so the list is not rebuilt under the loop that is still reading it.
const runOver = async (messages, action, message) => {
  if (!messages.length) return;
  busy.value = true;
  let failed = 0;
  try {
    for (const email of messages) {
      try {
        await action(email);
      } catch {
        failed++;
      }
    }
    if (failed) toast.error(t('em.bulkPartial', { n: failed }));
    else if (message) toast.success(message);
  } finally {
    busy.value = false;
    if (openEmail.value && messages.some((email) => email.uid === openEmail.value.uid)) closeReader();
    await refresh();
  }
};

const archiveMessages = (messages) =>
  runOver(messages, (email) => moveMessage(email, 'Archive'), t('em.messageMoved', { folder: 'Archive' }));

const trashMessages = (messages) => runOver(messages, deleteMessage, t('em.messageDeleted'));

const markMessages = (messages, seen) =>
  runOver(messages, (email) => setFlag(email, 'seen', seen), null);

/* ----------------------------------------------------------------- compose */

function emptyCompose() {
  return { to: '', cc: '', bcc: '', subject: '', body: '' };
}

const parseAddresses = (value) =>
  (value || '').split(/[,;]/).map((address) => address.trim()).filter(Boolean);

const openCompose = () => {
  compose.value = emptyCompose();
  showCc.value = false;
  showBcc.value = false;
  composeOpen.value = true;
};

const openReply = (all = false) => {
  const source = openEmail.value;
  if (!source) return;

  const sender = addressOnly(source.from);
  let to = sender;
  if (all && source.to) {
    const mine = (activeMailbox.value?.email || '').toLowerCase();
    const others = parseAddresses(source.to)
      .map(addressOnly)
      .filter((address) => address && address.toLowerCase() !== mine);
    to = [...new Set([sender, ...others])].join(', ');
  }

  const cc = all && source.cc
    ? parseAddresses(source.cc)
      .map(addressOnly)
      .filter((address) => address.toLowerCase() !== (activeMailbox.value?.email || '').toLowerCase())
      .join(', ')
    : '';

  const subject = /^re:\s/i.test(source.subject || '')
    ? source.subject
    : `Re: ${source.subject || ''}`.trim();

  const quoted = source.body_plain
    ? `\n\n${t('em.quoteHeader', { date: formatFullDate(source.date), sender: source.from })}\n${
      source.body_plain.split('\n').map((line) => `> ${line}`).join('\n')}`
    : '';

  compose.value = { ...emptyCompose(), to, cc, subject, body: quoted };
  showCc.value = Boolean(cc);
  showBcc.value = false;
  composeOpen.value = true;
};

const sendMail = async () => {
  if (!activeMailboxId.value) return toast.error(t('em.selectMailboxFirst'));
  const to = parseAddresses(compose.value.to);
  if (!to.length) return toast.error(t('em.enterRecipient'));
  if (!compose.value.subject.trim()) return toast.error(t('em.enterSubject'));

  busy.value = true;
  try {
    const response = await ApiService.post(`/api/email/mailboxes/${activeMailboxId.value}/send`, {
      to,
      cc: parseAddresses(compose.value.cc),
      bcc: parseAddresses(compose.value.bcc),
      subject: compose.value.subject.trim(),
      body: compose.value.body || ''
    });
    if (response.data.error) {
      toast.error(response.data.msg);
      return;
    }
    toast.success(t('em.emailSent'));
    composeOpen.value = false;
    compose.value = emptyCompose();
    await refresh();
  } catch (error) {
    toast.error(error.response?.data?.msg || error.message);
  } finally {
    busy.value = false;
  }
};

const saveDraft = async () => {
  if (!activeMailboxId.value) return toast.error(t('em.selectMailboxFirst'));
  busy.value = true;
  try {
    const response = await ApiService.post(`/api/email/mailboxes/${activeMailboxId.value}/drafts`, {
      to: parseAddresses(compose.value.to),
      cc: parseAddresses(compose.value.cc),
      bcc: parseAddresses(compose.value.bcc),
      subject: compose.value.subject || '',
      body: compose.value.body || ''
    });
    if (!response.data.error) {
      toast.success(t('em.draftSaved'));
      composeOpen.value = false;
      compose.value = emptyCompose();
      await refresh();
    }
  } catch (error) {
    toast.error(error.response?.data?.msg || t('em.draftFailed'));
  } finally {
    busy.value = false;
  }
};

/* --------------------------------------------------------------- shortcuts */

// The keys mail clients have used for decades. They stay out of the way of
// anyone typing: a field with focus keeps every key it is given.
const onKeydown = (event) => {
  if (composeOpen.value || event.metaKey || event.ctrlKey || event.altKey) return;
  const tag = event.target?.tagName;
  if (tag === 'INPUT' || tag === 'TEXTAREA' || event.target?.isContentEditable) return;

  const list = visibleMessages.value;
  if (!list.length) return;
  const current = openEmail.value ? list.findIndex((email) => email.uid === openEmail.value.uid) : -1;

  switch (event.key) {
    case 'j':
      event.preventDefault();
      readMessage(list[Math.min(current + 1, list.length - 1)]);
      break;
    case 'k':
      event.preventDefault();
      if (current > 0) readMessage(list[current - 1]);
      break;
    case 'u':
    case 'Escape':
      if (openEmail.value) { event.preventDefault(); closeReader(); }
      break;
    case 'e':
      if (openEmail.value) { event.preventDefault(); archiveMessages([openEmail.value]); }
      break;
    case '#':
      if (openEmail.value) { event.preventDefault(); trashMessages([openEmail.value]); }
      break;
    case 's':
      if (openEmail.value) {
        event.preventDefault();
        setFlag(openEmail.value, 'flagged', !openEmail.value.flagged);
      }
      break;
    case 'c':
      event.preventDefault();
      openCompose();
      break;
  }
};

onMounted(() => window.addEventListener('keydown', onKeydown));
onBeforeUnmount(() => window.removeEventListener('keydown', onKeydown));

/* ------------------------------------------------------------------ wiring */

// Open on an account straight away: a picker with one obvious answer is a
// question not worth asking.
watch(
  () => props.mailboxes,
  (list) => {
    if (!activeMailboxId.value && list.length) activeMailboxId.value = list[0].id;
  },
  { immediate: true, deep: true }
);

watch(activeMailboxId, (id) => {
  if (!id) return;
  activeFolder.value = 'INBOX';
  closeReader();
  refresh();
}, { immediate: true });

watch(activeFolder, () => {
  closeReader();
  search.value = '';
  loadEmails();
});
</script>

<template>
  <div class="flex flex-col gap-4 h-[calc(100vh-15rem)] min-h-[34rem]">
    <!-- Toolbar -->
    <div class="flex flex-wrap items-center gap-3 rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-900/60 px-3 py-2.5">
      <select
        v-model.number="activeMailboxId"
        class="rounded-lg border border-gray-300 dark:border-slate-700 bg-white dark:bg-slate-800 px-3 py-2 text-sm font-medium focus:outline-none focus:ring-2 focus:ring-sky-500/40"
      >
        <option :value="null" disabled>{{ t('em.selectEmailAccount') }}</option>
        <option v-for="mailbox in mailboxes" :key="mailbox.id" :value="mailbox.id">
          {{ mailbox.email }}
        </option>
      </select>

      <div class="relative flex-1 min-w-[12rem]">
        <BaseIcon :path="mdiMagnify" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" w="w-5" h="h-5" />
        <input
          v-model="search"
          type="search"
          :placeholder="t('em.searchPlaceholder')"
          class="w-full rounded-lg border border-gray-300 dark:border-slate-700 bg-white dark:bg-slate-800 pl-10 pr-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-sky-500/40"
        />
      </div>

      <button
        type="button"
        :class="[
          'rounded-lg px-3 py-2 text-sm font-medium border transition-colors',
          unreadOnly
            ? 'border-sky-500 bg-sky-50 dark:bg-sky-500/10 text-sky-600 dark:text-sky-400'
            : 'border-gray-300 dark:border-slate-700 text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-slate-800'
        ]"
        @click="unreadOnly = !unreadOnly"
      >
        {{ t('em.unreadOnly') }}
      </button>

      <BaseButton :icon="mdiRefresh" color="light" small :disabled="loading" @click="refresh" />
    </div>

    <div v-if="!activeMailboxId" class="flex-1 flex items-center justify-center rounded-xl border border-dashed border-gray-300 dark:border-slate-700">
      <div class="text-center text-gray-500">
        <BaseIcon :path="mdiEmailOutline" class="mx-auto mb-3 text-gray-300 dark:text-slate-600" w="w-16" h="h-16" />
        <p class="font-medium">{{ t('em.selectEmailAccountTitle') }}</p>
        <p class="text-sm">{{ t('em.selectAccountAbove') }}</p>
      </div>
    </div>

    <div v-else class="flex-1 flex gap-4 min-h-0">
      <!-- Folders -->
      <aside class="hidden lg:flex w-56 shrink-0 flex-col gap-3">
        <BaseButton
          :icon="mdiPencil"
          color="info"
          :label="t('em.newEmail')"
          class="w-full justify-center"
          @click="openCompose"
        />

        <nav class="flex-1 space-y-0.5 overflow-y-auto rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-900/60 p-2">
          <button
            v-for="folder in folders"
            :key="folder.value"
            type="button"
            :class="[
              'w-full flex items-center gap-3 rounded-lg px-3 py-2 text-left text-sm transition-colors',
              activeFolder === folder.value
                ? 'bg-sky-50 dark:bg-sky-500/10 text-sky-700 dark:text-sky-300 font-semibold'
                : 'text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-slate-800'
            ]"
            @click="activeFolder = folder.value"
          >
            <BaseIcon :path="folder.icon" :class="folder.tone" w="w-5" h="h-5" />
            <span class="flex-1 truncate">{{ folder.label }}</span>
            <span
              v-if="folder.unseen"
              class="rounded-full bg-sky-500 px-2 py-0.5 text-xs font-semibold text-white"
            >{{ folder.unseen }}</span>
            <span
              v-else-if="folder.total"
              class="text-xs text-gray-400"
              :title="t('em.folderServerCount')"
            >{{ folder.total }}</span>
          </button>
        </nav>
      </aside>

      <!-- Message list -->
      <section
        :class="[
          'flex flex-col min-w-0 rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-900/60 overflow-hidden',
          openEmail ? 'hidden xl:flex xl:w-[24rem] xl:shrink-0' : 'flex-1'
        ]"
      >
        <div class="flex items-center gap-2 border-b border-gray-200 dark:border-slate-700 px-3 py-2.5">
          <button
            type="button"
            class="rounded p-1 text-gray-400 hover:text-sky-600"
            :title="t('em.selectAll')"
            @click="toggleSelectAll"
          >
            <BaseIcon :path="mdiCheckAll" :class="allSelected ? 'text-sky-500' : ''" w="w-5" h="h-5" />
          </button>
          <h3 class="font-semibold truncate">{{ folderLabel(activeFolder) }}</h3>
          <span class="text-xs text-gray-500 shrink-0">
            {{ filtering ? t('em.shownOfLoaded', { n: shownCount }) : t('em.emailsCount', { n: shownCount }) }}
          </span>
          <BaseButton
            v-if="openEmail"
            class="ml-auto xl:hidden"
            :icon="mdiArrowLeft"
            color="light"
            small
            @click="closeReader"
          />
        </div>

        <!-- Bulk actions appear only when there is a selection to act on. -->
        <div
          v-if="selection.size"
          class="flex flex-wrap items-center gap-2 border-b border-gray-200 dark:border-slate-700 bg-sky-50 dark:bg-sky-500/10 px-3 py-2"
        >
          <span class="text-xs font-medium text-sky-700 dark:text-sky-300">
            {{ t('em.selectedCount', { n: selection.size }) }}
          </span>
          <div class="ml-auto flex flex-wrap gap-1.5">
            <BaseButton :icon="mdiEmailOpenOutline" color="light" small :disabled="busy" :title="t('em.markRead')" @click="markMessages(selectedMessages(), true)" />
            <BaseButton :icon="mdiEmailOutline" color="light" small :disabled="busy" :title="t('em.markUnread')" @click="markMessages(selectedMessages(), false)" />
            <BaseButton :icon="mdiArchive" color="light" small :disabled="busy" :title="t('em.archive')" @click="archiveMessages(selectedMessages())" />
            <BaseButton :icon="mdiTrashCan" color="danger" small :disabled="busy" :title="t('em.moveToTrash')" @click="trashMessages(selectedMessages())" />
            <BaseButton :icon="mdiClose" color="light" small @click="selection = new Set()" />
          </div>
        </div>

        <div v-if="loading" class="flex-1 flex items-center justify-center">
          <div class="text-center text-gray-500">
            <div class="mx-auto mb-3 h-8 w-8 animate-spin rounded-full border-2 border-sky-500 border-t-transparent"></div>
            <p class="text-sm">{{ t('em.loadingEmails') }}</p>
          </div>
        </div>

        <div v-else-if="!shownCount" class="flex-1 flex items-center justify-center px-6">
          <div class="text-center text-gray-500">
            <BaseIcon :path="mdiEmailOpenOutline" class="mx-auto mb-3 text-gray-300 dark:text-slate-600" w="w-14" h="h-14" />
            <p class="font-medium">{{ filtering ? t('em.noMatches') : t('em.noEmailsIn', { folder: folderLabel(activeFolder) }) }}</p>
            <p class="text-sm">{{ filtering ? t('em.searchScopeHint') : t('em.folderEmpty') }}</p>
          </div>
        </div>

        <div v-else class="flex-1 overflow-y-auto divide-y divide-gray-100 dark:divide-slate-800">
          <template v-for="thread in visibleThreads" :key="thread.thread_id">
            <!-- The newest message represents the conversation; the rest open
                 underneath it rather than hiding behind a second click. -->
            <div
              :class="[
                'group flex cursor-pointer items-start gap-3 px-3 py-3 transition-colors',
                openEmail && thread.messages.some((m) => m.uid === openEmail.uid)
                  ? 'bg-sky-50 dark:bg-sky-500/10'
                  : 'hover:bg-gray-50 dark:hover:bg-slate-800/60'
              ]"
              @click="readMessage(leadOf(thread))"
            >
              <input
                type="checkbox"
                class="mt-2 h-4 w-4 shrink-0 rounded border-gray-300 text-sky-600 focus:ring-sky-500"
                :checked="selection.has(leadOf(thread).uid)"
                @click.stop
                @change="toggleSelected(leadOf(thread).uid)"
              />

              <div
                :class="['mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-full text-xs font-bold text-white', avatarTone(isOutgoing ? leadOf(thread).to : leadOf(thread).from)]"
              >
                {{ initials(isOutgoing ? leadOf(thread).to : leadOf(thread).from) }}
              </div>

              <div class="min-w-0 flex-1">
                <div class="flex items-baseline gap-2">
                  <p :class="['truncate text-sm', threadUnread(thread) ? 'font-bold text-gray-900 dark:text-white' : 'font-medium text-gray-700 dark:text-gray-300']">
                    {{ correspondent(leadOf(thread)) }}
                  </p>
                  <span
                    v-if="thread.count > 1"
                    class="shrink-0 rounded-full bg-gray-200 px-1.5 text-[11px] font-semibold text-gray-600 dark:bg-slate-700 dark:text-gray-300"
                  >{{ thread.count }}</span>
                  <span class="ml-auto shrink-0 text-xs text-gray-400">{{ formatListDate(leadOf(thread).date) }}</span>
                </div>

                <p :class="['truncate text-sm', threadUnread(thread) ? 'font-semibold text-gray-900 dark:text-white' : 'text-gray-600 dark:text-gray-400']">
                  {{ leadOf(thread).subject || t('em.noSubject') }}
                </p>
                <p class="truncate text-xs text-gray-500">{{ snippetOf(leadOf(thread)) || t('em.noPreview') }}</p>
              </div>

              <div class="flex shrink-0 flex-col items-center gap-1.5">
                <button type="button" @click.stop="setFlag(leadOf(thread), 'flagged', !leadOf(thread).flagged)">
                  <BaseIcon
                    :path="leadOf(thread).flagged ? mdiStar : mdiStarOutline"
                    :class="leadOf(thread).flagged ? 'text-amber-400' : 'text-gray-300 hover:text-amber-400'"
                    w="w-5" h="h-5"
                  />
                </button>
                <BaseIcon v-if="leadOf(thread).has_attachments" :path="mdiAttachment" class="text-gray-400" w="w-4" h="h-4" />
                <button v-if="thread.count > 1" type="button" @click.stop="toggleThread(thread.thread_id)">
                  <BaseIcon :path="expandedThreads.has(thread.thread_id) ? mdiChevronUp : mdiChevronDown" class="text-gray-400" w="w-5" h="h-5" />
                </button>
              </div>
            </div>

            <div v-if="thread.count > 1 && expandedThreads.has(thread.thread_id)" class="bg-gray-50/70 dark:bg-slate-800/40">
              <div
                v-for="email in thread.messages"
                :key="email.uid"
                :class="[
                  'flex cursor-pointer items-center gap-3 border-t border-gray-100 py-2 pl-14 pr-3 dark:border-slate-800',
                  openEmail?.uid === email.uid ? 'bg-sky-50 dark:bg-sky-500/10' : 'hover:bg-gray-100 dark:hover:bg-slate-800'
                ]"
                @click="readMessage(email)"
              >
                <div class="min-w-0 flex-1">
                  <p :class="['truncate text-xs', !email.seen ? 'font-semibold text-gray-900 dark:text-white' : 'text-gray-600 dark:text-gray-400']">
                    {{ correspondent(email) }}
                  </p>
                  <p class="truncate text-xs text-gray-500">{{ snippetOf(email) || t('em.noPreview') }}</p>
                </div>
                <span class="shrink-0 text-[11px] text-gray-400">{{ formatListDate(email.date) }}</span>
              </div>
            </div>
          </template>
        </div>
      </section>

      <!-- Reader -->
      <section
        v-if="openEmail"
        class="flex flex-1 min-w-0 flex-col overflow-hidden rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-900/60"
      >
        <header class="border-b border-gray-200 dark:border-slate-700 px-5 py-4">
          <div class="mb-3 flex items-start gap-3">
            <button class="xl:hidden rounded p-1 text-gray-400 hover:text-gray-600" @click="closeReader">
              <BaseIcon :path="mdiArrowLeft" w="w-5" h="h-5" />
            </button>
            <h2 class="flex-1 text-lg font-semibold leading-snug">
              {{ openEmail.subject || t('em.noSubject') }}
            </h2>
            <button class="hidden xl:block rounded p-1 text-gray-400 hover:text-gray-600" @click="closeReader">
              <BaseIcon :path="mdiClose" w="w-5" h="h-5" />
            </button>
          </div>

          <dl class="mb-3 space-y-0.5 text-sm">
            <div class="flex gap-2">
              <dt class="w-14 shrink-0 text-gray-500">{{ t('em.logFrom') }}</dt>
              <dd class="min-w-0 break-all font-medium">{{ openEmail.from || '-' }}</dd>
            </div>
            <div v-if="openEmail.to" class="flex gap-2">
              <dt class="w-14 shrink-0 text-gray-500">{{ t('em.to') }}</dt>
              <dd class="min-w-0 break-all">{{ openEmail.to }}</dd>
            </div>
            <div v-if="openEmail.cc" class="flex gap-2">
              <dt class="w-14 shrink-0 text-gray-500">Cc</dt>
              <dd class="min-w-0 break-all">{{ openEmail.cc }}</dd>
            </div>
            <div v-if="openEmail.reply_to && openEmail.reply_to !== openEmail.from" class="flex gap-2">
              <dt class="w-14 shrink-0 text-gray-500">{{ t('em.replyTo') }}</dt>
              <dd class="min-w-0 break-all">{{ openEmail.reply_to }}</dd>
            </div>
            <div class="flex gap-2">
              <dt class="w-14 shrink-0 text-gray-500">{{ t('em.logDate') }}</dt>
              <dd class="text-gray-500">{{ formatFullDate(openEmail.date) }}</dd>
            </div>
          </dl>

          <div class="flex flex-wrap gap-1.5">
            <BaseButton :icon="mdiReply" :label="t('em.reply')" color="light" small @click="openReply(false)" />
            <BaseButton :icon="mdiReplyAll" :label="t('em.replyAll')" color="light" small @click="openReply(true)" />
            <BaseButton
              :icon="openEmail.flagged ? mdiStar : mdiStarOutline"
              color="light"
              small
              :title="t('em.star')"
              @click="setFlag(openEmail, 'flagged', !openEmail.flagged)"
            />
            <BaseButton :icon="mdiEmailOutline" color="light" small :title="t('em.markUnread')" @click="setFlag(openEmail, 'seen', false)" />
            <BaseButton :icon="mdiArchive" color="light" small :disabled="busy" :title="t('em.archive')" @click="archiveMessages([openEmail])" />
            <BaseButton :icon="mdiDownload" color="light" small :title="t('em.downloadRaw')" @click="downloadRaw" />
            <BaseButton :icon="mdiTrashCan" color="danger" small :disabled="busy" :title="t('em.moveToTrash')" @click="trashMessages([openEmail])" />
          </div>
        </header>

        <div v-if="attachments.length" class="border-b border-gray-200 dark:border-slate-700 px-5 py-3">
          <p class="mb-2 text-xs font-medium text-gray-500">{{ t('em.attachmentsCount', { n: attachments.length }) }}</p>
          <div class="flex flex-wrap gap-2">
            <button
              v-for="attachment in attachments"
              :key="attachment.index"
              class="flex items-center gap-2 rounded-lg border border-gray-200 px-3 py-2 text-left hover:border-sky-500 dark:border-slate-700"
              @click="downloadAttachment(attachment)"
            >
              <BaseIcon :path="mdiAttachment" class="text-gray-400" w="w-4" h="h-4" />
              <span class="max-w-[12rem] truncate text-sm">{{ attachment.filename }}</span>
              <span class="text-xs text-gray-400">{{ formatSize(attachment.size) }}</span>
            </button>
          </div>
        </div>

        <div class="flex-1 space-y-3 overflow-y-auto px-5 py-4">
          <p v-if="threadLoading" class="text-sm text-gray-500">{{ t('common.loading') }}</p>

          <article
            v-for="message in readerMessages"
            :key="message.uid"
            class="overflow-hidden rounded-lg border border-gray-200 dark:border-slate-700"
          >
            <button
              type="button"
              class="flex w-full items-center gap-3 bg-gray-50 px-4 py-3 text-left dark:bg-slate-800/60"
              @click="toggleMessageBody(message.uid)"
            >
              <div :class="['flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-xs font-bold text-white', avatarTone(message.from)]">
                {{ initials(message.from) }}
              </div>
              <div class="min-w-0 flex-1">
                <p class="truncate text-sm font-medium">{{ displayName(message.from) }}</p>
                <p class="truncate text-xs text-gray-500">{{ formatFullDate(message.date) }}</p>
              </div>
              <BaseIcon
                v-if="readerMessages.length > 1"
                :path="collapsedMessages.has(message.uid) ? mdiChevronDown : mdiChevronUp"
                class="text-gray-400"
                w="w-5" h="h-5"
              />
            </button>

            <div v-if="!collapsedMessages.has(message.uid)" class="px-4 py-4 text-sm">
              <div v-if="message.body_html" class="wm-body prose prose-sm dark:prose-invert max-w-none" v-html="message.body_html"></div>
              <div v-else-if="message.body_plain" class="wm-body prose prose-sm dark:prose-invert max-w-none" v-html="plainToHtml(message.body_plain)"></div>
              <p v-else class="text-gray-500">{{ t('em.noContent') }}</p>
            </div>
          </article>
        </div>
      </section>

      <!-- Nothing open yet: say what the pane is for instead of leaving a hole. -->
      <section v-else class="hidden xl:flex flex-1 items-center justify-center rounded-xl border border-dashed border-gray-300 dark:border-slate-700">
        <div class="px-6 text-center text-gray-400">
          <BaseIcon :path="mdiEmailOpenOutline" class="mx-auto mb-3 text-gray-300 dark:text-slate-600" w="w-14" h="h-14" />
          <p class="text-sm font-medium">{{ t('em.readerEmpty') }}</p>
          <p class="mt-2 text-xs">{{ t('em.shortcutHint') }}</p>
        </div>
      </section>
    </div>

    <!-- Compose -->
    <CardBoxModal
      v-model="composeOpen"
      :title="t('em.newEmail')"
      :button-label="t('em.send')"
      has-cancel
      @confirm="sendMail"
    >
      <p v-if="activeMailbox" class="mb-3 text-sm">
        <span class="text-gray-500">{{ t('em.logFrom') }}:</span>
        <span class="ml-1 font-medium">{{ activeMailbox.email }}</span>
      </p>

      <FormField :label="t('em.to')" :help="t('em.multipleRecipientsHint')">
        <FormControl v-model="compose.to" :placeholder="t('em.toPlaceholder')" />
      </FormField>

      <div v-if="!showCc || !showBcc" class="mb-3 flex gap-2 text-sm">
        <button v-if="!showCc" type="button" class="text-sky-600 hover:underline" @click="showCc = true">{{ t('em.addCc') }}</button>
        <button v-if="!showBcc" type="button" class="text-sky-600 hover:underline" @click="showBcc = true">{{ t('em.addBcc') }}</button>
      </div>

      <FormField v-if="showCc" label="Cc">
        <FormControl v-model="compose.cc" :placeholder="t('em.toPlaceholder')" />
      </FormField>

      <FormField v-if="showBcc" label="Bcc" :help="t('em.bccHint')">
        <FormControl v-model="compose.bcc" :placeholder="t('em.toPlaceholder')" />
      </FormField>

      <FormField :label="t('em.subject')">
        <FormControl v-model="compose.subject" :placeholder="t('em.subject')" />
      </FormField>

      <FormField :label="t('em.message')">
        <FormControl v-model="compose.body" type="textarea" :rows="10" :placeholder="t('em.writeMessage')" />
      </FormField>

      <div class="flex justify-end">
        <BaseButton :icon="mdiContentSave" :label="t('em.saveDraft')" color="light" small :disabled="busy" @click="saveDraft" />
      </div>
    </CardBoxModal>
  </div>
</template>

<style scoped>
/* Quoted history is dimmed and ruled off so a long reply chain does not read
   as part of the message being answered. */
.wm-body :deep(blockquote),
.wm-body :deep(.gmail_quote),
.wm-body :deep(.wm-quote) {
  margin: 0.75rem 0;
  padding: 0.5rem 0 0.5rem 1rem;
  border-left: 3px solid #d1d5db;
  border-radius: 0 6px 6px 0;
  background: rgba(0, 0, 0, 0.03);
  color: #4b5563;
  font-size: 0.9em;
}

.dark .wm-body :deep(blockquote),
.dark .wm-body :deep(.gmail_quote),
.dark .wm-body :deep(.wm-quote) {
  border-left-color: #475569;
  background: rgba(255, 255, 255, 0.04);
  color: #94a3b8;
}

/* Remote HTML must not push the reader sideways. */
.wm-body :deep(img),
.wm-body :deep(table) {
  max-width: 100%;
  height: auto;
}
</style>
