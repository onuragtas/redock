<script setup>
/**
 * Filter rules: what happens to a message as it arrives.
 *
 * The engine behind this has been running on every inbound message for a while
 * — it reads the rules straight out of the database. What was missing was any
 * way to write one without editing the database by hand, which made a working
 * feature unreachable.
 *
 * Conditions and actions are stored as JSON strings. Nobody should have to type
 * JSON to file their newsletters, so this builds them from a form and parses
 * them back for editing.
 */
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useToast } from 'vue-toastification';

import BaseButton from '@/components/BaseButton.vue';
import BaseIcon from '@/components/BaseIcon.vue';
import CardBox from '@/components/CardBox.vue';
import CardBoxModal from '@/components/CardBoxModal.vue';
import ApiService from '@/services/ApiService';

import { mdiFilterVariant, mdiPencil, mdiPlus, mdiRefresh, mdiTrashCan } from '@mdi/js';

const props = defineProps({
  mailboxes: { type: Array, default: () => [] }
});

const { t } = useI18n();
const toast = useToast();

const activeMailboxId = ref(null);
const filters = ref([]);
const loading = ref(false);
const busy = ref(false);

const editorOpen = ref(false);
const editing = ref(null);
const form = ref(emptyRule());

const FIELDS = ['from', 'to', 'cc', 'subject', 'body', 'header'];
const OPERATORS = ['contains', 'not_contains', 'equals', 'starts_with', 'ends_with'];
const ACTIONS = ['move_to', 'mark_read', 'star', 'delete', 'stop'];
const FOLDERS = ['INBOX', 'Archive', 'Junk', 'Trash', 'Drafts', 'Sent'];

function emptyRule() {
  return {
    name: '',
    priority: 10,
    enabled: true,
    match_all: true,
    conditions: [{ field: 'from', operator: 'contains', value: '', header: '' }],
    actions: [{ type: 'move_to', folder: 'Archive' }]
  };
}

/* ------------------------------------------------------------------ loading */

const loadFilters = async () => {
  if (!activeMailboxId.value) return;
  loading.value = true;
  try {
    const response = await ApiService.get(`/api/email/mailboxes/${activeMailboxId.value}/filters`);
    if (!response.data.error) filters.value = response.data.data || [];
  } catch (error) {
    toast.error(error.response?.data?.msg || t('em.filterLoadFailed'));
  } finally {
    loading.value = false;
  }
};

watch(
  () => props.mailboxes,
  (list) => {
    if (!activeMailboxId.value && list.length) activeMailboxId.value = list[0].id;
  },
  { immediate: true, deep: true }
);

watch(activeMailboxId, loadFilters, { immediate: true });

/* ------------------------------------------------------------ reading rules */

// Stored rules may hold a single object rather than a list, which is what a
// hand-written rule tends to look like; the server accepts both, so this does.
const asList = (stored, fallback) => {
  if (!stored) return fallback;
  try {
    const parsed = JSON.parse(stored);
    const list = Array.isArray(parsed) ? parsed : [parsed];
    return list.length ? list : fallback;
  } catch {
    return fallback;
  }
};

const describe = (filter) => {
  const conditions = asList(filter.conditions, []);
  const actions = asList(filter.actions, []);

  const when = conditions
    .map((condition) => {
      const field = condition.field === 'header' ? condition.header || 'header' : condition.field;
      return `${field} ${t('em.op_' + (condition.operator || 'contains'))} "${condition.value}"`;
    })
    .join(filter.match_all ? ` ${t('em.filterAnd')} ` : ` ${t('em.filterOr')} `);

  const then = actions
    .map((action) => (action.type === 'move_to'
      ? t('em.act_move_to') + ' → ' + action.folder
      : t('em.act_' + action.type)))
    .join(', ');

  return { when, then };
};

/* ------------------------------------------------------------------ editing */

const openNew = () => {
  editing.value = null;
  form.value = emptyRule();
  editorOpen.value = true;
};

const openEdit = (filter) => {
  editing.value = filter;
  form.value = {
    name: filter.name,
    priority: filter.priority,
    enabled: filter.enabled,
    match_all: filter.match_all,
    conditions: asList(filter.conditions, emptyRule().conditions).map((condition) => ({
      field: condition.field || 'from',
      operator: condition.operator || 'contains',
      value: condition.value || '',
      header: condition.header || ''
    })),
    actions: asList(filter.actions, emptyRule().actions).map((action) => ({
      type: action.type || 'move_to',
      folder: action.folder || 'Archive'
    }))
  };
  editorOpen.value = true;
};

const addCondition = () =>
  form.value.conditions.push({ field: 'from', operator: 'contains', value: '', header: '' });

const removeCondition = (index) => {
  if (form.value.conditions.length > 1) form.value.conditions.splice(index, 1);
};

const addAction = () => form.value.actions.push({ type: 'mark_read', folder: '' });

const removeAction = (index) => {
  if (form.value.actions.length > 1) form.value.actions.splice(index, 1);
};

// The server refuses a rule it could not run; catching the obvious cases here
// means the reason arrives next to the field rather than as a toast.
const problem = computed(() => {
  if (!form.value.name.trim()) return t('em.filterNeedsName');
  for (const condition of form.value.conditions) {
    if (!condition.value.trim()) return t('em.filterNeedsValue');
    if (condition.field === 'header' && !condition.header.trim()) return t('em.filterNeedsHeader');
  }
  for (const action of form.value.actions) {
    if (action.type === 'move_to' && !action.folder) return t('em.filterNeedsFolder');
  }
  return '';
});

const save = async () => {
  if (problem.value) {
    toast.error(problem.value);
    return;
  }

  const payload = {
    name: form.value.name.trim(),
    priority: Number(form.value.priority) || 0,
    enabled: form.value.enabled,
    match_all: form.value.match_all,
    conditions: JSON.stringify(form.value.conditions.map((condition) => ({
      field: condition.field,
      operator: condition.operator,
      value: condition.value.trim(),
      ...(condition.field === 'header' ? { header: condition.header.trim() } : {})
    }))),
    actions: JSON.stringify(form.value.actions.map((action) => ({
      type: action.type,
      ...(action.type === 'move_to' ? { folder: action.folder } : {})
    })))
  };

  busy.value = true;
  try {
    const response = editing.value
      ? await ApiService.put(`/api/email/filters/${editing.value.id}`, payload)
      : await ApiService.post(`/api/email/mailboxes/${activeMailboxId.value}/filters`, payload);

    if (response.data.error) {
      toast.error(response.data.msg);
      return;
    }
    toast.success(editing.value ? t('em.filterUpdated') : t('em.filterCreated'));
    editorOpen.value = false;
    await loadFilters();
  } catch (error) {
    toast.error(error.response?.data?.msg || t('em.filterSaveFailed'));
  } finally {
    busy.value = false;
  }
};

const toggleEnabled = async (filter) => {
  busy.value = true;
  try {
    await ApiService.put(`/api/email/filters/${filter.id}`, {
      name: filter.name,
      priority: filter.priority,
      enabled: !filter.enabled,
      match_all: filter.match_all,
      conditions: filter.conditions,
      actions: filter.actions
    });
    await loadFilters();
  } catch (error) {
    toast.error(error.response?.data?.msg || t('em.filterSaveFailed'));
  } finally {
    busy.value = false;
  }
};

const remove = async (filter) => {
  if (!confirm(t('em.filterConfirmDelete', { name: filter.name }))) return;
  busy.value = true;
  try {
    await ApiService.delete(`/api/email/filters/${filter.id}`);
    toast.success(t('em.filterDeleted'));
    await loadFilters();
  } catch (error) {
    toast.error(error.response?.data?.msg || t('em.filterSaveFailed'));
  } finally {
    busy.value = false;
  }
};
</script>

<template>
  <div>
    <CardBox>
      <div class="mb-4 flex flex-wrap items-center gap-3">
        <BaseIcon :path="mdiFilterVariant" class="text-sky-500" w="w-6" h="h-6" />
        <h3 class="text-lg font-semibold">{{ t('em.filterTitle') }}</h3>

        <select
          v-model.number="activeMailboxId"
          class="ml-auto rounded-lg border border-gray-300 dark:border-slate-700 bg-white dark:bg-slate-800 px-3 py-2 text-sm"
        >
          <option :value="null" disabled>{{ t('em.selectEmailAccount') }}</option>
          <option v-for="mailbox in mailboxes" :key="mailbox.id" :value="mailbox.id">
            {{ mailbox.email }}
          </option>
        </select>

        <BaseButton :icon="mdiRefresh" color="light" small :disabled="loading" @click="loadFilters" />
        <BaseButton :icon="mdiPlus" :label="t('em.filterNew')" color="info" small :disabled="!activeMailboxId" @click="openNew" />
      </div>

      <p class="mb-4 text-sm text-gray-500">{{ t('em.filterHint') }}</p>

      <div v-if="loading" class="py-8 text-center text-sm text-gray-500">{{ t('common.loading') }}</div>

      <div v-else-if="!filters.length" class="rounded-lg border border-dashed border-gray-300 dark:border-slate-700 py-10 text-center">
        <BaseIcon :path="mdiFilterVariant" class="mx-auto mb-3 text-gray-300 dark:text-slate-600" w="w-12" h="h-12" />
        <p class="text-sm text-gray-500">{{ t('em.filterEmpty') }}</p>
      </div>

      <!-- Rules run top to bottom, so the order on screen is the order they
           are applied; the priority number is what decides it. -->
      <ol v-else class="space-y-2">
        <li
          v-for="filter in filters"
          :key="filter.id"
          class="rounded-lg border border-gray-200 dark:border-slate-700 px-4 py-3"
          :class="filter.enabled ? '' : 'opacity-60'"
        >
          <div class="flex flex-wrap items-center gap-3">
            <span class="rounded-full bg-gray-100 dark:bg-slate-800 px-2 py-0.5 text-xs font-mono text-gray-500">
              {{ filter.priority }}
            </span>
            <span class="font-medium">{{ filter.name }}</span>
            <span
              v-if="!filter.enabled"
              class="rounded-full bg-gray-200 dark:bg-slate-700 px-2 py-0.5 text-xs text-gray-600 dark:text-gray-300"
            >{{ t('em.filterDisabled') }}</span>

            <div class="ml-auto flex items-center gap-1.5">
              <label class="flex cursor-pointer items-center gap-1.5 text-xs text-gray-500">
                <input type="checkbox" :checked="filter.enabled" :disabled="busy" @change="toggleEnabled(filter)" />
                {{ t('em.filterEnabled') }}
              </label>
              <BaseButton :icon="mdiPencil" color="light" small :disabled="busy" @click="openEdit(filter)" />
              <BaseButton :icon="mdiTrashCan" color="danger" small :disabled="busy" @click="remove(filter)" />
            </div>
          </div>

          <p class="mt-2 text-sm">
            <span class="text-gray-500">{{ t('em.filterWhen') }}</span>
            <span class="ml-1 font-mono text-xs">{{ describe(filter).when }}</span>
          </p>
          <p class="text-sm">
            <span class="text-gray-500">{{ t('em.filterThen') }}</span>
            <span class="ml-1 font-mono text-xs">{{ describe(filter).then }}</span>
          </p>
        </li>
      </ol>
    </CardBox>

    <!-- Editor -->
    <CardBoxModal
      v-model="editorOpen"
      :title="editing ? t('em.filterEdit') : t('em.filterNew')"
      :button-label="t('em.saveChanges')"
      has-cancel
      @confirm="save"
    >
      <div class="mb-4 grid grid-cols-1 gap-3 sm:grid-cols-3">
        <label class="sm:col-span-2 text-sm">
          <span class="mb-1 block text-gray-500">{{ t('em.filterName') }}</span>
          <input
            v-model="form.name"
            type="text"
            class="w-full rounded-lg border border-gray-300 dark:border-slate-700 bg-white dark:bg-slate-800 px-3 py-2"
          />
        </label>
        <label class="text-sm">
          <span class="mb-1 block text-gray-500">{{ t('em.filterPriority') }}</span>
          <input
            v-model.number="form.priority"
            type="number"
            class="w-full rounded-lg border border-gray-300 dark:border-slate-700 bg-white dark:bg-slate-800 px-3 py-2"
          />
        </label>
      </div>
      <p class="mb-4 -mt-2 text-xs text-gray-500">{{ t('em.filterPriorityHint') }}</p>

      <!-- Conditions -->
      <div class="mb-4">
        <div class="mb-2 flex items-center gap-3">
          <h4 class="font-medium">{{ t('em.filterWhen') }}</h4>
          <select v-model="form.match_all" class="rounded border border-gray-300 dark:border-slate-700 bg-white dark:bg-slate-800 px-2 py-1 text-xs">
            <option :value="true">{{ t('em.filterMatchAll') }}</option>
            <option :value="false">{{ t('em.filterMatchAny') }}</option>
          </select>
          <BaseButton class="ml-auto" :icon="mdiPlus" color="light" small @click="addCondition" />
        </div>

        <div v-for="(condition, index) in form.conditions" :key="index" class="mb-2 flex flex-wrap items-center gap-2">
          <select v-model="condition.field" class="rounded border border-gray-300 dark:border-slate-700 bg-white dark:bg-slate-800 px-2 py-1.5 text-sm">
            <option v-for="field in FIELDS" :key="field" :value="field">{{ t('em.fld_' + field) }}</option>
          </select>
          <input
            v-if="condition.field === 'header'"
            v-model="condition.header"
            type="text"
            :placeholder="t('em.filterHeaderName')"
            class="w-32 rounded border border-gray-300 dark:border-slate-700 bg-white dark:bg-slate-800 px-2 py-1.5 text-sm"
          />
          <select v-model="condition.operator" class="rounded border border-gray-300 dark:border-slate-700 bg-white dark:bg-slate-800 px-2 py-1.5 text-sm">
            <option v-for="operator in OPERATORS" :key="operator" :value="operator">{{ t('em.op_' + operator) }}</option>
          </select>
          <input
            v-model="condition.value"
            type="text"
            :placeholder="t('em.filterValue')"
            class="min-w-[8rem] flex-1 rounded border border-gray-300 dark:border-slate-700 bg-white dark:bg-slate-800 px-2 py-1.5 text-sm"
          />
          <BaseButton
            :icon="mdiTrashCan"
            color="light"
            small
            :disabled="form.conditions.length === 1"
            @click="removeCondition(index)"
          />
        </div>
      </div>

      <!-- Actions -->
      <div class="mb-3">
        <div class="mb-2 flex items-center gap-3">
          <h4 class="font-medium">{{ t('em.filterThen') }}</h4>
          <BaseButton class="ml-auto" :icon="mdiPlus" color="light" small @click="addAction" />
        </div>

        <div v-for="(action, index) in form.actions" :key="index" class="mb-2 flex flex-wrap items-center gap-2">
          <select v-model="action.type" class="rounded border border-gray-300 dark:border-slate-700 bg-white dark:bg-slate-800 px-2 py-1.5 text-sm">
            <option v-for="type in ACTIONS" :key="type" :value="type">{{ t('em.act_' + type) }}</option>
          </select>
          <select
            v-if="action.type === 'move_to'"
            v-model="action.folder"
            class="rounded border border-gray-300 dark:border-slate-700 bg-white dark:bg-slate-800 px-2 py-1.5 text-sm"
          >
            <option v-for="folder in FOLDERS" :key="folder" :value="folder">{{ folder }}</option>
          </select>
          <BaseButton
            :icon="mdiTrashCan"
            color="light"
            small
            :disabled="form.actions.length === 1"
            @click="removeAction(index)"
          />
        </div>
        <p class="text-xs text-gray-500">{{ t('em.filterStopHint') }}</p>
      </div>

      <p v-if="problem" class="rounded-lg bg-amber-50 dark:bg-amber-500/10 px-3 py-2 text-sm text-amber-700 dark:text-amber-400">
        {{ problem }}
      </p>
    </CardBoxModal>
  </div>
</template>
