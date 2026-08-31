<script setup>
/**
 * What to type into a mail client to reach this mailbox.
 *
 * Everything here is read from the running server rather than written down:
 * ports and TLS modes come from the listeners that are actually up. A page
 * that hardcodes "993 for IMAPS" tells people to use a port this install may
 * have turned off, and they spend the afternoon on a connection that was never
 * going to answer.
 */
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useToast } from 'vue-toastification';

import BaseButton from '@/components/BaseButton.vue';
import BaseIcon from '@/components/BaseIcon.vue';

import { mdiContentCopy, mdiDownload, mdiInboxArrowDown, mdiSend } from '@mdi/js';

const props = defineProps({
  mailbox: { type: Object, required: true },
  // ListenerStatus rows from /api/email/engine: { name, port, tls }
  listeners: { type: Array, default: () => [] },
  hostname: { type: String, default: '' },
  ipAddress: { type: String, default: '' }
});

const { t } = useI18n();
const toast = useToast();

// The address a client should be pointed at. The configured hostname is the
// right answer because that is the name the certificate covers; the address is
// only worth showing as the fallback when no name has been set.
const server = computed(() => props.hostname || props.ipAddress || window.location.hostname);

const certificateMismatch = computed(() => !props.hostname && Boolean(props.ipAddress));

const listenerFor = (name) => props.listeners.find((listener) => listener.name === name) || null;

// One row per way in, built only from listeners that are running.
const rows = computed(() => {
  const out = [];

  const add = (name, label, direction) => {
    const listener = listenerFor(name);
    if (!listener) return;
    out.push({
      key: name,
      label,
      direction,
      port: listener.port,
      security: listener.tls === 'implicit' ? 'SSL/TLS' : listener.tls === 'starttls' ? 'STARTTLS' : t('em.connNone')
    });
  };

  add('imaps', 'IMAP', 'in');
  add('imap', 'IMAP', 'in');
  add('pop3s', 'POP3', 'in');
  add('pop3', 'POP3', 'in');
  // Submission is the port a person's client sends through; 25 is for other
  // mail servers and is deliberately not offered here.
  add('submission', 'SMTP', 'out');
  add('smtps', 'SMTP', 'out');

  return out;
});

const incoming = computed(() => rows.value.filter((row) => row.direction === 'in'));
const outgoing = computed(() => rows.value.filter((row) => row.direction === 'out'));

const summary = computed(() => {
  const lines = [
    `${t('em.connAccount')}: ${props.mailbox.email}`,
    `${t('em.connServer')}: ${server.value}`,
    `${t('em.connUsername')}: ${props.mailbox.email}`,
    ''
  ];
  for (const row of rows.value) {
    const heading = row.direction === 'in' ? t('em.connIncoming') : t('em.connOutgoing');
    lines.push(`${heading} — ${row.label}: ${server.value}:${row.port} (${row.security})`);
  }
  return lines.join('\n');
});

const copied = ref('');

const copy = async (value, key) => {
  try {
    await navigator.clipboard.writeText(value);
    copied.value = key;
    setTimeout(() => { if (copied.value === key) copied.value = ''; }, 1500);
  } catch {
    // Clipboard access is refused outside a secure context, which is exactly
    // where a local dashboard often runs; say so instead of failing silently.
    toast.error(t('em.connCopyFailed'));
  }
};

const downloadSummary = () => {
  const blob = new Blob([summary.value], { type: 'text/plain' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = `${props.mailbox.email}-settings.txt`;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
};
</script>

<template>
  <div>
    <!-- The two values every client asks for first. -->
    <div class="mb-4 grid grid-cols-1 gap-3 sm:grid-cols-2">
      <div class="rounded-lg border border-gray-200 dark:border-slate-700 px-3 py-2">
        <p class="text-xs text-gray-500">{{ t('em.connUsername') }}</p>
        <div class="flex items-center gap-2">
          <span class="min-w-0 flex-1 truncate font-mono text-sm">{{ mailbox.email }}</span>
          <BaseButton
            :icon="mdiContentCopy"
            color="light"
            small
            :title="copied === 'user' ? t('em.connCopied') : t('em.connCopy')"
            @click="copy(mailbox.email, 'user')"
          />
        </div>
      </div>

      <div class="rounded-lg border border-gray-200 dark:border-slate-700 px-3 py-2">
        <p class="text-xs text-gray-500">{{ t('em.connServer') }}</p>
        <div class="flex items-center gap-2">
          <span class="min-w-0 flex-1 truncate font-mono text-sm">{{ server }}</span>
          <BaseButton
            :icon="mdiContentCopy"
            color="light"
            small
            :title="copied === 'server' ? t('em.connCopied') : t('em.connCopy')"
            @click="copy(server, 'server')"
          />
        </div>
      </div>
    </div>

    <p
      v-if="certificateMismatch"
      class="mb-4 rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:bg-amber-500/10 dark:text-amber-300"
    >
      {{ t('em.connNoHostname') }}
    </p>

    <div v-if="!rows.length" class="rounded-lg border border-dashed border-gray-300 dark:border-slate-700 py-8 text-center text-sm text-gray-500">
      {{ t('em.connNoListeners') }}
    </div>

    <template v-else>
      <section v-if="incoming.length" class="mb-4">
        <h4 class="mb-2 flex items-center gap-2 font-medium">
          <BaseIcon :path="mdiInboxArrowDown" class="text-sky-500" w="w-5" h="h-5" />
          {{ t('em.connIncoming') }}
        </h4>
        <table class="w-full text-sm">
          <tbody>
            <tr v-for="row in incoming" :key="row.key" class="border-t border-gray-100 dark:border-slate-800">
              <td class="py-2 pr-3 font-medium">{{ row.label }}</td>
              <td class="py-2 pr-3 font-mono">{{ server }}:{{ row.port }}</td>
              <td class="py-2 text-right">
                <span class="rounded-full bg-gray-100 px-2 py-0.5 text-xs dark:bg-slate-800">{{ row.security }}</span>
              </td>
            </tr>
          </tbody>
        </table>
      </section>

      <section v-if="outgoing.length" class="mb-4">
        <h4 class="mb-2 flex items-center gap-2 font-medium">
          <BaseIcon :path="mdiSend" class="text-emerald-500" w="w-5" h="h-5" />
          {{ t('em.connOutgoing') }}
        </h4>
        <table class="w-full text-sm">
          <tbody>
            <tr v-for="row in outgoing" :key="row.key" class="border-t border-gray-100 dark:border-slate-800">
              <td class="py-2 pr-3 font-medium">{{ row.label }}</td>
              <td class="py-2 pr-3 font-mono">{{ server }}:{{ row.port }}</td>
              <td class="py-2 text-right">
                <span class="rounded-full bg-gray-100 px-2 py-0.5 text-xs dark:bg-slate-800">{{ row.security }}</span>
              </td>
            </tr>
          </tbody>
        </table>
        <p class="mt-2 text-xs text-gray-500">{{ t('em.connAuthHint') }}</p>
      </section>
    </template>

    <p class="mb-4 text-xs text-gray-500">{{ t('em.connPasswordHint') }}</p>

    <div class="flex flex-wrap gap-2">
      <BaseButton
        :icon="mdiContentCopy"
        :label="copied === 'all' ? t('em.connCopied') : t('em.connCopyAll')"
        color="info"
        small
        @click="copy(summary, 'all')"
      />
      <BaseButton :icon="mdiDownload" :label="t('em.connDownload')" color="light" small @click="downloadSummary" />
    </div>
  </div>
</template>
