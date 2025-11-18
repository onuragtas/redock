<script setup>
import BaseButton from "@/components/BaseButton.vue";
import BaseIcon from "@/components/BaseIcon.vue";
import CardBox from "@/components/CardBox.vue";
import FormControl from "@/components/FormControl.vue";
import FormField from "@/components/FormField.vue";
import ApiService from "@/services/ApiService";
import { mdiArrowLeft, mdiContentSave, mdiWrench } from '@mdi/js';
import { onMounted, ref } from 'vue';

// Reactive state
const env = ref('')
const loading = ref(false)

// Methods
const getEnv = async () => {
  try {
    loading.value = true
    const res = await ApiService.getEnv()
    env.value = res.data.data.env || ''
  } catch (error) {
    console.error('Error fetching environment:', error)
  } finally {
    loading.value = false
  }
}

const submit = async () => {
  try {
    loading.value = true
    const res = await ApiService.setEnv(env.value)
    env.value = res.data.data.env
  } catch (error) {
    console.error('Error updating environment:', error)
  } finally {
    loading.value = false
  }
}

// Lifecycle
onMounted(() => {
  getEnv()
})
</script>

<template>
  <div class="space-y-6">
    <!-- Header -->
    <div class="bg-gradient-to-r from-blue-600 to-purple-600 rounded-xl p-6 text-white">
      <div class="flex items-center space-x-4">
        <div class="w-12 h-12 bg-white/20 rounded-xl flex items-center justify-center backdrop-blur-sm">
          <BaseIcon :path="mdiWrench" size="24" class="text-white" />
        </div>
        <div>
          <h1 class="text-2xl lg:text-3xl font-bold mb-2">Setup Environment</h1>
          <p class="text-blue-100">Configure your environment variables</p>
        </div>
      </div>
    </div>

    <!-- Environment Configuration -->
    <CardBox>
      <div class="bg-gradient-to-r from-gray-50 to-gray-100 dark:from-gray-800 dark:to-gray-700 p-6 -m-6 mb-6">
        <div class="flex items-center space-x-3">
          <BaseIcon :path="mdiWrench" size="24" class="text-gray-600 dark:text-gray-400" />
          <h3 class="text-lg font-semibold text-gray-900 dark:text-white">Environment Variables</h3>
        </div>
      </div>

      <form class="space-y-6" @submit.prevent="submit">
        <FormField 
          label="Environment Configuration" 
          help="Enter your environment variables in .env format (KEY=VALUE)"
        >
          <FormControl 
            v-model="env" 
            type="textarea" 
            placeholder="DOCKER_HOST=unix:///var/run/docker.sock&#10;API_PORT=6001&#10;WEB_PORT=5173&#10;..."
            height="40vh"
            :disabled="loading"
          />
        </FormField>

        <div class="flex justify-end space-x-3 pt-4 border-t border-gray-200 dark:border-gray-700">
          <BaseButton
            :icon="mdiArrowLeft"
            label="Back"
            color="lightDark"
            to="/"
          />
          <BaseButton
            :icon="mdiContentSave"
            label="Save Environment"
            color="success"
            type="submit"
            :disabled="loading"
          />
        </div>
      </form>
    </CardBox>
  </div>
</template>
