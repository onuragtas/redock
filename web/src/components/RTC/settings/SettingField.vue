<script setup>
// One labelled input with help text, an inline error and an optional unit.
// The input itself is the default slot, or a built-in text/number input
// when v-model is given.
import { computed, useSlots } from 'vue'

const props = defineProps({
  id: { type: String, required: true },
  label: { type: String, required: true },
  help: { type: String, default: '' },
  error: { type: String, default: '' },
  unit: { type: String, default: '' },
  modelValue: { type: [String, Number], default: undefined },
  type: { type: String, default: 'text' },
  placeholder: { type: String, default: '' },
  min: { type: [String, Number], default: undefined },
  max: { type: [String, Number], default: undefined },
  mono: { type: Boolean, default: false },
})
const emit = defineEmits(['update:modelValue'])
const slots = useSlots()

const value = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', props.type === 'number' ? (v === '' ? 0 : Number(v)) : v),
})

const inputClass = computed(() => [
  'w-full rounded-lg border bg-white dark:bg-slate-800 px-3 py-2 text-sm',
  'focus:outline-none focus:ring-2 focus:ring-emerald-500/40',
  props.error ? 'border-red-500' : 'border-slate-300 dark:border-slate-600',
  props.mono ? 'font-mono' : '',
  props.unit ? 'pr-16' : '',
])
</script>

<template>
  <div>
    <label :for="id" class="block text-sm font-medium text-slate-700 dark:text-slate-200 mb-1">{{ label }}</label>
    <slot v-if="slots.default" />
    <div v-else class="relative">
      <input
        :id="id"
        v-model="value"
        :type="type"
        :placeholder="placeholder"
        :min="min"
        :max="max"
        :class="inputClass"
        :aria-invalid="!!error"
        :aria-describedby="error || help ? `${id}-hint` : undefined"
      />
      <span v-if="unit" class="absolute inset-y-0 right-3 flex items-center text-xs text-slate-400 pointer-events-none">{{ unit }}</span>
    </div>
    <p v-if="error" :id="`${id}-hint`" class="mt-1 text-xs text-red-600 dark:text-red-400">{{ error }}</p>
    <p v-else-if="help" :id="`${id}-hint`" class="mt-1 text-xs text-slate-500 dark:text-slate-400">{{ help }}</p>
  </div>
</template>
