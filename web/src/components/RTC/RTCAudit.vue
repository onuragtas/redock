<script setup>
import BaseButton from "@/components/BaseButton.vue";
import CardBox from "@/components/CardBox.vue";
import FormControl from "@/components/FormControl.vue";
import ApiService from "@/services/ApiService";
import { mdiRefresh } from '@mdi/js';
import { onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useToast } from "vue-toastification";

const { t } = useI18n();
const toast = useToast();

const entries = ref([])
const room = ref('')

const load = async () => {
  try {
    const res = await ApiService.rtcAudit(room.value.trim(), 300)
    if (res.data?.error) throw new Error(res.data.msg)
    entries.value = res.data?.data || []
  } catch (e) {
    toast.error(e?.response?.data?.msg || e?.message || t('rtc.actionFailed'))
  }
}

const danger = (a) => ['kick', 'end-session', 'room-deleted', 'apikey-deleted', 'lock', 'clear-chat'].includes(a)

defineExpose({ load })
onMounted(load)
</script>

<template>
  <CardBox>
    <div class="flex items-center justify-between mb-2 gap-3">
      <h3 class="text-lg font-semibold">{{ t('rtc.audit') }}</h3>
      <div class="flex items-center gap-2">
        <FormControl v-model="room" :placeholder="t('rtc.filterRoom')" class="w-40" @keyup.enter="load" />
        <BaseButton :icon="mdiRefresh" small color="lightDark" outline @click="load" />
      </div>
    </div>
    <p class="text-sm text-slate-500 mb-3">{{ t('rtc.auditHelp') }}</p>
    <div class="max-h-96 overflow-y-auto">
      <table class="w-full text-xs">
        <thead>
          <tr class="text-left text-slate-500">
            <th class="py-1">{{ t('rtc.time') }}</th>
            <th class="py-1">{{ t('rtc.actor') }}</th>
            <th class="py-1">{{ t('rtc.action') }}</th>
            <th class="py-1">{{ t('rtc.room') }}</th>
            <th class="py-1">{{ t('rtc.target') }}</th>
            <th class="py-1">{{ t('rtc.detail') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="e in entries" :key="e.id" class="border-t border-slate-200 dark:border-slate-700">
            <td class="py-1 whitespace-nowrap">{{ new Date(e.time).toLocaleString() }}</td>
            <td class="py-1">{{ e.actor }}</td>
            <td :class="['py-1 font-mono', danger(e.action) ? 'text-red-500' : 'text-emerald-600']">{{ e.action }}</td>
            <td class="py-1 font-mono">{{ e.room }}</td>
            <td class="py-1">{{ e.target }}</td>
            <td class="py-1 text-slate-500">{{ e.detail }}</td>
          </tr>
          <tr v-if="!entries.length"><td colspan="6" class="py-3 text-slate-400">{{ t('rtc.noAudit') }}</td></tr>
        </tbody>
      </table>
    </div>
  </CardBox>
</template>
