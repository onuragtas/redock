<script setup>
// Inbound rules WebRTC needs, written as firewall rules: source → destination
// (this server), protocol and port. Computed by the backend from the gateway,
// SFU and TURN settings.
import BaseButton from "@/components/BaseButton.vue";
import CardBox from "@/components/CardBox.vue";
import ApiService from "@/services/ApiService";
import { mdiContentCopy, mdiRefresh } from '@mdi/js';
import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useToast } from "vue-toastification";

const props = defineProps({
  // Limit the table to some services (e.g. ['turn'] on the TURN tab).
  services: { type: Array, default: null },
})

const { t } = useI18n();
const toast = useToast();

const plan = ref({ local_ip: '', rules: [], warnings: [] })

const load = async () => {
  try {
    const res = await ApiService.rtcPorts()
    if (res.data?.error) throw new Error(res.data.msg)
    plan.value = res.data?.data || plan.value
  } catch (e) {
    toast.error(e?.response?.data?.msg || e?.message || t('rtc.actionFailed'))
  }
}

const rules = computed(() => {
  const all = plan.value.rules || []
  return props.services ? all.filter((r) => props.services.includes(r.service)) : all
})
const warnings = computed(() => {
  const w = plan.value.warnings || []
  if (!props.services) return w
  return w.filter((k) => (props.services.includes('turn') ? k.startsWith('turn') : true))
})

const destination = computed(() => plan.value.local_ip || t('ports.thisServer'))

// ufw rules apply on the server itself, so they use the internal ports.
const ufw = computed(() => {
  const lines = []
  for (const r of rules.value) {
    const port = r.internal.replace('-', ':')
    for (const proto of r.protocol.split('+')) {
      lines.push(`sudo ufw allow ${port}/${proto.toLowerCase()}   # ${t('ports.purpose.' + r.purpose)}`)
    }
  }
  return lines.join('\n')
})

const copy = async (text) => {
  try {
    await navigator.clipboard.writeText(text)
    toast.success(t('rtc.copied'))
  } catch {
    toast.error(t('rtc.copyFailed'))
  }
}

defineExpose({ load })
onMounted(load)
</script>

<template>
  <CardBox>
    <div class="flex items-center justify-between mb-2">
      <h3 class="text-lg font-semibold">{{ t('ports.title') }}</h3>
      <BaseButton :icon="mdiRefresh" small color="lightDark" outline @click="load" />
    </div>
    <p class="text-sm text-slate-500 mb-4">{{ t('ports.help') }}</p>

    <ul v-if="warnings.length" class="mb-4 space-y-2">
      <li v-for="w in warnings" :key="w" class="text-xs rounded-lg bg-amber-50 dark:bg-amber-900/20 text-amber-700 dark:text-amber-300 px-3 py-2">
        {{ t('ports.warn.' + w) }}
      </li>
    </ul>

    <div v-if="rules.length" class="overflow-x-auto">
      <table class="w-full text-sm">
        <thead>
          <tr class="text-left text-slate-500 text-xs uppercase tracking-wide">
            <th class="py-2 pr-4">{{ t('ports.source') }}</th>
            <th class="py-2 pr-4">{{ t('ports.destination') }}</th>
            <th class="py-2 pr-4">{{ t('ports.protocol') }}</th>
            <th class="py-2">{{ t('ports.purposeCol') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(r, i) in rules" :key="i" class="border-t border-slate-200 dark:border-slate-700 align-top">
            <td class="py-2 pr-4 font-mono whitespace-nowrap">{{ t('ports.any') }}:{{ r.external }}</td>
            <td class="py-2 pr-4 font-mono whitespace-nowrap">{{ destination }}:{{ r.internal }}</td>
            <td class="py-2 pr-4 font-mono whitespace-nowrap">{{ r.protocol }}</td>
            <td class="py-2">
              {{ t('ports.purpose.' + r.purpose) }}
              <span v-if="r.optional" class="ml-1 text-[11px] px-1.5 py-0.5 rounded bg-slate-100 dark:bg-slate-700 text-slate-600 dark:text-slate-300">{{ t('ports.recommended') }}</span>
              <span v-else class="ml-1 text-[11px] px-1.5 py-0.5 rounded bg-emerald-100 dark:bg-emerald-900/40 text-emerald-700 dark:text-emerald-300">{{ t('ports.required') }}</span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <p v-else class="text-sm text-slate-400">{{ t('ports.none') }}</p>

    <div v-if="rules.length" class="mt-5 text-xs text-slate-500 min-w-0">
      <div class="flex items-center justify-between mb-1">
        <p class="font-semibold text-slate-700 dark:text-slate-200">{{ t('ports.firewallTitle') }}</p>
        <BaseButton :icon="mdiContentCopy" small color="lightDark" outline @click="copy(ufw)" />
      </div>
      <pre class="bg-slate-900 text-slate-100 rounded-lg p-3 overflow-x-auto">{{ ufw }}</pre>
    </div>
  </CardBox>
</template>
