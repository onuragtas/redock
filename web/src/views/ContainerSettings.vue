<script setup>
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import ApiService from '@/services/ApiService'
import BaseButton from '@/components/BaseButton.vue'
import BaseIcon from '@/components/BaseIcon.vue'
import CardBox from '@/components/CardBox.vue'
import FormControl from '@/components/FormControl.vue'
import FormField from '@/components/FormField.vue'
import SectionTitleLineWithButton from '@/components/SectionTitleLineWithButton.vue'
import {
  mdiAlertCircle,
  mdiContentSave,
  mdiDatabaseCog,
  mdiDocker,
  mdiRefresh,
  mdiRenameBox,
  mdiSwapVertical,
  mdiWrench
} from '@mdi/js'

const { t } = useI18n()

const loading = ref(false)
const saving = ref(false)
const status = reactive({ type: '', message: '' })
const services = ref([])
const settings = reactive({
  container_name_prefix: ''
})
const overrides = reactive({})

const resetStatus = () => {
  status.type = ''
  status.message = ''
}

const ensureOverrideState = (serviceName) => {
  if (!overrides[serviceName]) {
    overrides[serviceName] = {
      customName: '',
      ports: ''
    }
  }
  return overrides[serviceName]
}

const hydrateOverrides = (incoming = {}) => {
  Object.keys(overrides).forEach((key) => delete overrides[key])
  services.value.forEach((service) => {
    const current = incoming[service.name]
    overrides[service.name] = {
      customName: current?.custom_name || '',
      ports: current?.ports?.length ? current.ports.join('\n') : ''
    }
  })
}

const fetchSettings = async () => {
  loading.value = true
  resetStatus()
  try {
    const response = await ApiService.getDockerServiceSettings()
    const data = response.data?.data || {}
    const incomingSettings = data.settings || {}
    services.value = data.services || []
    settings.container_name_prefix = incomingSettings.container_name_prefix || ''
    hydrateOverrides(incomingSettings.overrides || {})
  } catch (error) {
    status.type = 'error'
    status.message = t('cs.loadFailed')
    console.error('Failed to load service settings:', error)
  } finally {
    loading.value = false
  }
}

const clearOverride = (serviceName) => {
  const state = ensureOverrideState(serviceName)
  state.customName = ''
  state.ports = ''
}

const buildPayloadOverrides = () => {
  const payload = {}
  services.value.forEach((service) => {
    const state = ensureOverrideState(service.name)
    const customName = state.customName.trim()
    const ports = state.ports
      .split('\n')
      .map((line) => line.trim())
      .filter((line) => line.length)

    if (customName || ports.length) {
      payload[service.name] = {
        custom_name: customName,
        ports
      }
    }
  })
  return payload
}

const saveSettings = async () => {
  saving.value = true
  resetStatus()
  try {
    await ApiService.updateDockerServiceSettings({
      container_name_prefix: settings.container_name_prefix.trim(),
      overrides: buildPayloadOverrides()
    })
    status.type = 'success'
    status.message = t('cs.saved')
    await fetchSettings()
  } catch (error) {
    status.type = 'error'
    status.message = t('cs.saveFailed')
    console.error('Failed to save service settings:', error)
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  fetchSettings()
})
</script>

<template>
  <div class="space-y-8">
    <div class="bg-gradient-to-r from-sky-600 via-blue-600 to-indigo-600 text-white rounded-2xl p-8 shadow-lg">
      <div class="flex flex-col lg:flex-row lg:items-center lg:justify-between gap-6">
        <div>
          <p class="uppercase tracking-wider text-sm text-blue-200 mb-2">{{ t('cs.dockerControls') }}</p>
          <h1 class="text-3xl lg:text-4xl font-bold flex items-center gap-3">
            <BaseIcon :path="mdiDocker" size="40" />
            {{ t('cs.title') }}
          </h1>
          <p class="text-blue-100 mt-2">{{ t('cs.subtitle') }}</p>
        </div>
        <BaseButton
          :icon="mdiRefresh"
          color="white"
          outline
          :label="t('common.refresh')"
          :disabled="loading"
          @click="fetchSettings"
        />
      </div>
    </div>

    <CardBox>
      <SectionTitleLineWithButton :icon="mdiWrench" :title="t('cs.generalOptions')" main>
        <BaseButton
          :icon="mdiContentSave"
          color="success"
          :disabled="saving || loading"
          :label="t('cs.saveChanges')"
          @click="saveSettings"
        />
      </SectionTitleLineWithButton>

      <div class="grid gap-6 lg:grid-cols-2">
        <div>
          <FormField :label="t('cs.containerNamePrefix')" :help="t('cs.prefixHelp')">
            <div class="relative">
              <div class="absolute inset-y-0 left-0 pl-3 flex items-center text-slate-400">
                <BaseIcon :path="mdiRenameBox" size="20" />
              </div>
              <FormControl
                v-model="settings.container_name_prefix"
                class="pl-10"
                placeholder="team-alpha"
                :disabled="loading"
              />
            </div>
          </FormField>
        </div>
        <div class="bg-slate-50 dark:bg-slate-800 rounded-xl p-4 text-sm text-slate-600 dark:text-slate-300">
          <p class="font-semibold mb-2 flex items-center gap-2">
            <BaseIcon :path="mdiDatabaseCog" size="18" />
            {{ t('cs.tips') }}
          </p>
          <ul class="space-y-1 list-disc list-inside">
            <li>{{ t('cs.tip1') }}</li>
            <li v-html="t('cs.tip2')"></li>
            <li>{{ t('cs.tip3') }}</li>
          </ul>
        </div>
      </div>

      <div v-if="status.message" class="mt-6">
        <div
          :class="[
            'rounded-xl px-4 py-3 flex items-center gap-2',
            status.type === 'success' ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300' : 'bg-red-50 text-red-700 dark:bg-red-900/20 dark:text-red-300'
          ]"
        >
          <BaseIcon :path="status.type === 'success' ? mdiDocker : mdiAlertCircle" size="20" />
          <span>{{ status.message }}</span>
        </div>
      </div>
    </CardBox>

    <CardBox>
      <SectionTitleLineWithButton :icon="mdiSwapVertical" :title="t('cs.perServiceOverrides')" main>
        <span class="text-sm text-slate-500">{{ t('cs.servicesDetected', { n: services.length }) }}</span>
      </SectionTitleLineWithButton>

      <div v-if="loading" class="py-12 text-center text-slate-500">
        {{ t('cs.loadingMeta') }}
      </div>

      <div v-else class="grid gap-6 md:grid-cols-2">
        <div
          v-for="service in services"
          :key="service.name"
          class="p-5 border border-slate-200 dark:border-slate-700 rounded-2xl bg-white dark:bg-slate-800 shadow-sm"
        >
          <div class="flex items-start justify-between gap-4">
            <div>
              <p class="text-sm text-slate-500 uppercase tracking-wide">{{ service.image || t('cs.customImage') }}</p>
              <h3 class="text-2xl font-semibold mt-1">{{ service.name }}</h3>
              <p class="text-xs text-slate-500">{{ t('cs.defaultContainer') }} {{ service.default_container_name }}</p>
              <p class="text-xs text-slate-500">{{ t('cs.currentContainer') }} {{ service.effective_container_name }}</p>
            </div>
            <BaseButton
              :icon="mdiRefresh"
              color="lightDark"
              outline
              small
              :title="t('cs.resetOverrides')"
              @click="clearOverride(service.name)"
            />
          </div>

          <div class="mt-4 space-y-4">
            <FormField :label="t('cs.customContainerName')" :help="t('cs.customNameHelp')">
              <FormControl
                v-model="ensureOverrideState(service.name).customName"
                :placeholder="`dev-${service.name}`"
                :disabled="loading"
              />
            </FormField>

            <FormField :label="t('cs.portOverrides')" :help="t('cs.portOverridesHelp')">
              <FormControl
                v-model="ensureOverrideState(service.name).ports"
                type="textarea"
                :rows="3"
                placeholder="15432:5432"
                :disabled="loading"
              />
              <p v-if="service.default_ports?.length" class="text-xs text-slate-500 mt-2">
                {{ t('cs.defaults') }} {{ service.default_ports.join(', ') }}
              </p>
            </FormField>
          </div>
        </div>
      </div>

      <div class="mt-8 flex justify-end">
        <BaseButton
          :icon="mdiContentSave"
          color="success"
          :disabled="saving || loading"
          :label="t('cs.saveAll')"
          @click="saveSettings"
        />
      </div>
    </CardBox>
  </div>
</template>
