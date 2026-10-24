<script setup>
import BaseButton from "@/components/BaseButton.vue";
import CardBox from "@/components/CardBox.vue";
import CardBoxModal from "@/components/CardBoxModal.vue";
import FormControl from "@/components/FormControl.vue";
import FormField from "@/components/FormField.vue";
import ApiService from "@/services/ApiService";
import { mdiContentCopy, mdiDelete, mdiPlus } from '@mdi/js';
import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useToast } from "vue-toastification";

const { t } = useI18n();
const toast = useToast();

const keys = ref([])
const createModal = ref(false)
const newName = ref('')
const created = ref(null)

const errMsg = (e) => e?.response?.data?.msg || e?.message || t('rtc.actionFailed')
const call = async (fn) => {
  const res = await fn.call(ApiService) // keep `this` for ApiService.get/post
  if (res.data?.error) throw new Error(res.data.msg)
  return res.data?.data
}

const load = async () => {
  try {
    keys.value = (await call(ApiService.rtcListAPIKeys)) || []
  } catch (e) {
    toast.error(errMsg(e))
  }
}

const openCreate = () => {
  newName.value = ''
  created.value = null
  createModal.value = true
}
const create = async () => {
  try {
    created.value = await call(() => ApiService.rtcCreateAPIKey(newName.value))
    await load()
  } catch (e) {
    toast.error(errMsg(e))
  }
}
const remove = async (k) => {
  if (!window.confirm(t('rtc.deleteKeyConfirm', { name: k.name }))) return
  try {
    await call(() => ApiService.rtcDeleteAPIKey(k.id))
    await load()
  } catch (e) {
    toast.error(errMsg(e))
  }
}

const copy = async (text) => {
  try {
    await navigator.clipboard.writeText(text)
    toast.success(t('rtc.copied'))
  } catch {
    toast.error(t('rtc.copyFailed'))
  }
}

const example = computed(() => `# 1) Create the room (idempotent)
curl -X POST ${location.origin}/api/v1/rtc/ext/rooms \\
  -H 'X-API-Key: ${created.value?.key || 'rtk_...'}' -H 'Content-Type: application/json' \\
  -d '{"name":"order-1234","display_name":"Order #1234"}'

# 2) Get a join link for one of your users
curl -X POST ${location.origin}/api/v1/rtc/ext/tokens \\
  -H 'X-API-Key: ${created.value?.key || 'rtk_...'}' -H 'Content-Type: application/json' \\
  -d '{"room":"order-1234","username":"Alice","role":"presenter","ttl_hours":2}'
# -> {"data":{"join_url":"https://.../redock/join.html#room=...&token=...", "token":"...", ...}}`)

onMounted(load)
</script>

<template>
  <CardBox>
    <div class="flex items-center justify-between mb-2">
      <h3 class="text-lg font-semibold">{{ t('rtc.apiKeys') }}</h3>
      <BaseButton :icon="mdiPlus" :label="t('rtc.newKey')" color="info" small @click="openCreate" />
    </div>
    <p class="text-sm text-slate-500 mb-4">{{ t('rtc.apiKeysHelp') }}</p>
    <div v-for="k in keys" :key="k.id" class="flex items-center gap-3 text-sm border-t border-slate-200 dark:border-slate-700 py-2">
      <span class="flex-1 font-medium">{{ k.name }}</span>
      <code class="text-xs text-slate-500">{{ k.prefix }}…</code>
      <span class="text-xs text-slate-500 whitespace-nowrap">{{ k.last_used_at ? t('rtc.lastUsed', { at: new Date(k.last_used_at).toLocaleString() }) : t('rtc.neverUsed') }}</span>
      <BaseButton :icon="mdiDelete" small color="danger" outline @click="remove(k)" />
    </div>
    <p v-if="!keys.length" class="text-sm text-slate-400">{{ t('rtc.noKeys') }}</p>
  </CardBox>

  <CardBoxModal v-model="createModal" :title="t('rtc.newKey')" :button-label="t('rtc.done')">
    <template v-if="!created">
      <FormField :label="t('rtc.keyName')" :help="t('rtc.keyNameHelp')">
        <FormControl v-model="newName" placeholder="my-platform" />
      </FormField>
      <BaseButton :label="t('rtc.createKey')" color="info" :disabled="!newName.trim()" @click="create" />
    </template>
    <template v-else>
      <p class="text-sm text-amber-600 mb-2">{{ t('rtc.keyOnce') }}</p>
      <div class="flex items-center gap-2 mb-4">
        <code class="flex-1 break-all text-sm bg-slate-100 dark:bg-slate-800 rounded p-2">{{ created.key }}</code>
        <BaseButton :icon="mdiContentCopy" small color="info" @click="copy(created.key)" />
      </div>
      <p class="text-sm text-slate-500 mb-2">{{ t('rtc.apiExample') }}</p>
      <pre class="text-xs bg-slate-900 text-slate-100 rounded p-3 overflow-x-auto">{{ example }}</pre>
    </template>
  </CardBoxModal>
</template>
