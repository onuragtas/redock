<script setup>
import { ref, watch } from 'vue'
import { mdiClose, mdiAlertCircle, mdiCheckCircle, mdiInformationOutline } from '@mdi/js'
import BaseIcon from '@/components/BaseIcon.vue'
import BaseButton from '@/components/BaseButton.vue'

const props = defineProps({
  modelValue: {
    type: Boolean,
    default: false
  },
  title: {
    type: String,
    default: 'Confirm Action'
  },
  message: {
    type: String,
    default: 'Are you sure you want to proceed?'
  },
  confirmText: {
    type: String,
    default: 'Confirm'
  },
  cancelText: {
    type: String,
    default: 'Cancel'
  },
  type: {
    type: String,
    default: 'info', // 'success', 'warning', 'danger', 'info'
    validator: (value) => ['success', 'warning', 'danger', 'info'].includes(value)
  },
  confirmColor: {
    type: String,
    default: null // Will be set based on type if null
  }
})

const emit = defineEmits(['update:modelValue', 'confirm', 'cancel'])

const isOpen = ref(props.modelValue)

watch(() => props.modelValue, (newVal) => {
  isOpen.value = newVal
})

const close = () => {
  isOpen.value = false
  emit('update:modelValue', false)
}

const confirm = () => {
  emit('confirm')
  close()
}

const cancel = () => {
  emit('cancel')
  close()
}

// Icon and color based on type
const getIcon = () => {
  switch (props.type) {
    case 'success':
      return mdiCheckCircle
    case 'warning':
      return mdiAlertCircle
    case 'danger':
      return mdiAlertCircle
    case 'info':
    default:
      return mdiInformationOutline
  }
}

const getIconClass = () => {
  switch (props.type) {
    case 'success':
      return 'text-green-600 bg-green-100 dark:bg-green-900/30'
    case 'warning':
      return 'text-orange-600 bg-orange-100 dark:bg-orange-900/30'
    case 'danger':
      return 'text-red-600 bg-red-100 dark:bg-red-900/30'
    case 'info':
    default:
      return 'text-blue-600 bg-blue-100 dark:bg-blue-900/30'
  }
}

const getConfirmButtonColor = () => {
  if (props.confirmColor) return props.confirmColor
  
  switch (props.type) {
    case 'success':
      return 'success'
    case 'warning':
      return 'warning'
    case 'danger':
      return 'danger'
    case 'info':
    default:
      return 'info'
  }
}
</script>

<template>
  <Transition name="modal">
    <div
      v-if="isOpen"
      class="fixed inset-0 z-50 flex items-center justify-center p-4"
      @click.self="cancel"
    >
      <!-- Backdrop -->
      <div class="absolute inset-0 bg-black/50 backdrop-blur-sm"></div>

      <!-- Modal -->
      <div
        class="relative bg-white dark:bg-slate-800 rounded-2xl shadow-2xl max-w-md w-full transform transition-all"
      >
        <!-- Close Button -->
        <button
          @click="cancel"
          class="absolute top-4 right-4 p-2 rounded-lg hover:bg-gray-100 dark:hover:bg-slate-700 transition-colors"
        >
          <BaseIcon :path="mdiClose" w="w-5" h="h-5" class="text-gray-500 dark:text-gray-400" />
        </button>

        <!-- Content -->
        <div class="p-6">
          <!-- Icon -->
          <div class="flex items-center justify-center mb-4">
            <div
              :class="[
                'p-4 rounded-full',
                getIconClass()
              ]"
            >
              <BaseIcon :path="getIcon()" w="w-8" h="h-8" />
            </div>
          </div>

          <!-- Title -->
          <h3 class="text-xl font-bold text-center mb-2 text-gray-900 dark:text-white">
            {{ title }}
          </h3>

          <!-- Message -->
          <p class="text-center text-gray-600 dark:text-gray-400 mb-6 whitespace-pre-line">
            {{ message }}
          </p>

          <!-- Actions -->
          <div class="flex gap-3">
            <BaseButton
              :label="cancelText"
              color="contrast"
              outline
              class="flex-1"
              @click="cancel"
            />
            <BaseButton
              :label="confirmText"
              :color="getConfirmButtonColor()"
              class="flex-1"
              @click="confirm"
            />
          </div>
        </div>
      </div>
    </div>
  </Transition>
</template>

<style scoped>
.modal-enter-active,
.modal-leave-active {
  transition: opacity 0.3s ease;
}

.modal-enter-from,
.modal-leave-to {
  opacity: 0;
}

.modal-enter-active .relative,
.modal-leave-active .relative {
  transition: transform 0.3s ease, opacity 0.3s ease;
}

.modal-enter-from .relative,
.modal-leave-to .relative {
  transform: scale(0.9);
  opacity: 0;
}
</style>
