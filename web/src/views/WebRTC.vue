<script setup>
// One place for real-time media: the SFU (rooms, recordings, moderation)
// and the TURN/STUN relay it relies on. The selected tab lives in the URL
// (?tab=turn) so either part can be linked to directly.
import BaseIcon from "@/components/BaseIcon.vue";
import { mdiTransitConnectionVariant, mdiVideo } from '@mdi/js';
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { useRoute, useRouter } from "vue-router";
import RTCServer from "./RTCServer.vue";
import TURNServer from "./TURNServer.vue";

const { t } = useI18n();
const route = useRoute();
const router = useRouter();

const tabs = [
  { id: 'sfu', icon: mdiVideo, label: 'webrtc.tabSfu' },
  { id: 'turn', icon: mdiTransitConnectionVariant, label: 'webrtc.tabTurn' },
]

const tab = computed({
  get: () => (route.query.tab === 'turn' ? 'turn' : 'sfu'),
  set: (v) => router.replace({ query: { ...route.query, tab: v === 'sfu' ? undefined : v } }),
})
</script>

<template>
  <div class="space-y-6">
    <div class="flex gap-2 border-b border-slate-200 dark:border-slate-700">
      <button
        v-for="item in tabs"
        :key="item.id"
        :class="[
          'flex items-center gap-2 px-4 py-2 -mb-px border-b-2 text-sm font-medium transition-colors',
          tab === item.id
            ? 'border-emerald-500 text-emerald-600 dark:text-emerald-400'
            : 'border-transparent text-slate-500 hover:text-slate-700 dark:hover:text-slate-300',
        ]"
        @click="tab = item.id"
      >
        <BaseIcon :path="item.icon" size="18" />
        {{ t(item.label) }}
      </button>
    </div>
    <RTCServer v-if="tab === 'sfu'" />
    <TURNServer v-else />
  </div>
</template>
