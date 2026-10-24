<script setup>
// A radio group drawn as cards: each option shows what it means, so the
// choice is made on its consequences rather than its name.
defineProps({
  name: { type: String, required: true },
  modelValue: { type: String, default: '' },
  options: { type: Array, required: true }, // [{ value, label, description, badge }]
})
const emit = defineEmits(['update:modelValue'])
</script>

<template>
  <div class="grid grid-cols-1 sm:grid-cols-2 gap-3" role="radiogroup">
    <label
      v-for="o in options"
      :key="o.value"
      :class="[
        'relative flex flex-col gap-1 rounded-lg border p-3 cursor-pointer transition-colors',
        modelValue === o.value
          ? 'border-emerald-500 bg-emerald-50 dark:bg-emerald-900/20 ring-1 ring-emerald-500'
          : 'border-slate-300 dark:border-slate-600 hover:border-slate-400 dark:hover:border-slate-500',
      ]"
    >
      <input
        class="sr-only"
        type="radio"
        :name="name"
        :value="o.value"
        :checked="modelValue === o.value"
        @change="emit('update:modelValue', o.value)"
      />
      <span class="flex items-center gap-2 text-sm font-medium text-slate-800 dark:text-slate-100">
        {{ o.label }}
        <span v-if="o.badge" class="text-[11px] px-1.5 py-0.5 rounded bg-emerald-600 text-white">{{ o.badge }}</span>
      </span>
      <span class="text-xs text-slate-500 dark:text-slate-400 leading-relaxed">{{ o.description }}</span>
    </label>
  </div>
</template>
