<script setup>
// Appears only while there are unsaved changes, so the page never shows a
// Save button that does nothing; states what saving will do before it does.
import { useI18n } from "vue-i18n";

defineProps({
  dirty: { type: Boolean, default: false },
  busy: { type: Boolean, default: false },
  errors: { type: Number, default: 0 },
  hint: { type: String, default: '' },
})
const emit = defineEmits(['save', 'revert'])
const { t } = useI18n()
</script>

<template>
  <Transition
    enter-active-class="transition duration-150 ease-out"
    enter-from-class="opacity-0 translate-y-2"
    leave-active-class="transition duration-100 ease-in"
    leave-to-class="opacity-0 translate-y-2"
  >
    <div
      v-if="dirty"
      class="sticky bottom-4 z-20 mt-6 flex flex-wrap items-center gap-3 rounded-xl border border-slate-200 dark:border-slate-700 bg-white/95 dark:bg-slate-900/95 backdrop-blur px-4 py-3 shadow-lg"
    >
      <div class="flex-1 min-w-[12rem] text-sm">
        <p class="font-medium text-slate-800 dark:text-slate-100">
          {{ errors ? t('settingsForm.fixErrors', { n: errors }) : t('settingsForm.unsaved') }}
        </p>
        <p v-if="hint && !errors" class="text-xs text-slate-500 dark:text-slate-400">{{ hint }}</p>
      </div>
      <button
        type="button"
        class="px-3 py-2 rounded-lg text-sm text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800"
        :disabled="busy"
        @click="emit('revert')"
      >{{ t('settingsForm.revert') }}</button>
      <button
        type="button"
        class="px-4 py-2 rounded-lg text-sm font-semibold bg-emerald-600 hover:bg-emerald-500 text-white disabled:opacity-50 disabled:cursor-not-allowed"
        :disabled="busy || errors > 0"
        @click="emit('save')"
      >{{ busy ? t('settingsForm.saving') : t('settingsForm.save') }}</button>
    </div>
  </Transition>
</template>
