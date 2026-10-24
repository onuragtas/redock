<script setup>
// An on/off setting with its explanation next to it. `danger` marks a
// switch whose "on" position weakens security.
defineProps({
  id: { type: String, required: true },
  label: { type: String, required: true },
  description: { type: String, default: '' },
  modelValue: { type: Boolean, default: false },
  danger: { type: Boolean, default: false },
})
const emit = defineEmits(['update:modelValue'])
</script>

<template>
  <div class="flex items-start justify-between gap-4">
    <div class="min-w-0">
      <label :for="id" class="text-sm font-medium text-slate-700 dark:text-slate-200 cursor-pointer">{{ label }}</label>
      <p v-if="description" class="text-xs text-slate-500 dark:text-slate-400 mt-0.5 leading-relaxed">{{ description }}</p>
    </div>
    <button
      :id="id"
      type="button"
      role="switch"
      :aria-checked="modelValue"
      :class="[
        'relative inline-flex h-6 w-11 flex-shrink-0 rounded-full transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-emerald-500',
        modelValue ? (danger ? 'bg-red-600' : 'bg-emerald-600') : 'bg-slate-300 dark:bg-slate-600',
      ]"
      @click="emit('update:modelValue', !modelValue)"
    >
      <span
        :class="['inline-block h-5 w-5 mt-0.5 rounded-full bg-white shadow transition-transform', modelValue ? 'translate-x-5' : 'translate-x-0.5']"
      />
    </button>
  </div>
</template>
