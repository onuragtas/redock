<script setup>
// A settings group: what the group is about on the left, its fields on the
// right (stacked on narrow screens). Keeps long forms scannable.
import BaseIcon from "@/components/BaseIcon.vue";

defineProps({
  title: { type: String, required: true },
  description: { type: String, default: '' },
  icon: { type: String, default: '' },
})
</script>

<template>
  <div class="grid grid-cols-1 lg:grid-cols-3 gap-4 lg:gap-8 py-6 first:pt-0 last:pb-0 border-b last:border-b-0 border-slate-200 dark:border-slate-700">
    <div>
      <h4 class="flex items-center gap-2 font-semibold text-slate-800 dark:text-slate-100">
        <BaseIcon v-if="icon" :path="icon" size="18" class="text-emerald-600 dark:text-emerald-400" />
        {{ title }}
      </h4>
      <p v-if="description" class="mt-1 text-sm text-slate-500 dark:text-slate-400 leading-relaxed">{{ description }}</p>
    </div>
    <div class="lg:col-span-2 space-y-5 min-w-0">
      <slot />
    </div>
  </div>
</template>
