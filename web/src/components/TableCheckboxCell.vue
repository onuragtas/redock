<script setup>
import { ref, watch } from 'vue';

defineProps({
  type: {
    type: String,
    default: 'td'
  },
  disabled: {
    type: Boolean,
    default: false
  }
})

const emit = defineEmits(['checked'])

const checked = ref(false)

watch(checked, (newVal) => {
  emit('checked', newVal)
})

const onchange = (e) => {
  checked.value = e.target.checked
}

</script>

<template>
  <component :is="type" class="lg:w-1">
    <label class="checkbox">
      <input @change="onchange" :disabled="disabled" :checked="disabled" type="checkbox" />
      <span class="check" />
    </label>
  </component>
</template>
