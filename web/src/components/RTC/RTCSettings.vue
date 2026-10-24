<script setup>
// SFU settings, grouped by what the admin is deciding. Changes are validated
// as they are typed and saved together from a bar that only appears when
// something changed.
import CardBox from "@/components/CardBox.vue";
import ChoiceCards from "@/components/RTC/settings/ChoiceCards.vue";
import SaveBar from "@/components/RTC/settings/SaveBar.vue";
import SettingField from "@/components/RTC/settings/SettingField.vue";
import SettingsSection from "@/components/RTC/settings/SettingsSection.vue";
import SwitchRow from "@/components/RTC/settings/SwitchRow.vue";
import { isDomain, isHostPort, rangeError } from "@/components/RTC/settings/validators";
import ApiService from "@/services/ApiService";
import { mdiArchiveClock, mdiChevronDown, mdiLan, mdiPower, mdiTransitConnectionVariant, mdiWeb } from '@mdi/js';
import BaseIcon from "@/components/BaseIcon.vue";
import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useToast } from "vue-toastification";

const props = defineProps({ running: { type: Boolean, default: false } })
const emit = defineEmits(['loaded', 'saved'])

const { t } = useI18n();
const toast = useToast();

const empty = () => ({ enabled: false, listen_addr: '', udp_min: 0, udp_max: 0, turnMode: 'auto', turnCustom: '', public_host: '', retention_days: 0 })
const form = ref(empty())
const saved = ref(JSON.stringify(empty()))
const loaded = ref(false)
const busy = ref(false)
const showAdvanced = ref(false)

const fromServer = (c) => {
  let turnMode = 'custom'
  if (c.turn === 'redock' || c.turn === 'auto') turnMode = c.turn
  else if (!c.turn) turnMode = 'off'
  return {
    enabled: !!c.enabled,
    listen_addr: c.listen_addr || '127.0.0.1:8445',
    udp_min: c.udp_min || 0,
    udp_max: c.udp_max || 0,
    turnMode,
    turnCustom: turnMode === 'custom' ? c.turn : '',
    public_host: c.public_host || '',
    retention_days: c.retention_days ?? 0,
  }
}

const load = async () => {
  try {
    const res = await ApiService.rtcGetConfig()
    const c = res.data?.data || {}
    form.value = fromServer(c)
    saved.value = JSON.stringify(form.value)
    loaded.value = true
    emit('loaded', c)
  } catch (e) {
    toast.error(e?.response?.data?.msg || e?.message || t('rtc.actionFailed'))
  }
}

const dirty = computed(() => loaded.value && JSON.stringify(form.value) !== saved.value)

const errors = computed(() => {
  const f = form.value
  const e = {}
  if (f.public_host && !isDomain(f.public_host.trim())) e.public_host = t('settingsForm.errDomain')
  if (!isHostPort(f.listen_addr)) e.listen_addr = t('settingsForm.errHostPort')
  const r = rangeError(f.udp_min, f.udp_max, t, true)
  if (r) e.udp = r
  if (f.turnMode === 'custom' && !isHostPort(f.turnCustom)) e.turnCustom = t('settingsForm.errHostPort')
  if (!Number.isInteger(f.retention_days) || f.retention_days < 0 || f.retention_days > 3650) e.retention = t('settingsForm.errRetention')
  return e
})
const errorCount = computed(() => Object.keys(errors.value).length)

const udpCount = computed(() => {
  const { udp_min: a, udp_max: b } = form.value
  return a && b && b >= a ? b - a + 1 : 0
})

const turnOptions = computed(() => [
  { value: 'redock', label: t('rtc.turnRedockShort'), badge: t('settingsForm.recommended'), description: t('rtc.turnRedockDesc') },
  { value: 'auto', label: t('rtc.turnAutoShort'), description: t('rtc.turnAutoDesc') },
  { value: 'off', label: t('rtc.turnOffShort'), description: t('rtc.turnOffDesc') },
  { value: 'custom', label: t('rtc.turnCustomShort'), description: t('rtc.turnCustomDesc') },
])

const joinPreview = computed(() => (form.value.public_host ? `https://${form.value.public_host.trim()}/redock/join.html#…` : ''))

const revert = () => { form.value = JSON.parse(saved.value) }

const save = async () => {
  if (errorCount.value) return
  busy.value = true
  const f = form.value
  try {
    const turn = f.turnMode === 'custom' ? f.turnCustom.trim() : f.turnMode === 'off' ? '' : f.turnMode
    const res = await ApiService.rtcUpdateConfig({
      enabled: f.enabled, listen_addr: f.listen_addr.trim(), udp_min: f.udp_min, udp_max: f.udp_max,
      turn, public_host: f.public_host.trim(), retention_days: f.retention_days,
    })
    if (res.data?.error) throw new Error(res.data.msg)
    toast.success(t('rtc.saved'))
    emit('saved', res.data.data)
    await load()
  } catch (e) {
    toast.error(e?.response?.data?.msg || e?.message || t('rtc.actionFailed'))
  } finally {
    busy.value = false
  }
}

onMounted(load)
defineExpose({ load })
</script>

<template>
  <CardBox>
    <h3 class="text-lg font-semibold mb-6">{{ t('rtc.settings') }}</h3>

    <SettingsSection :title="t('rtc.secGeneral')" :description="t('rtc.secGeneralDesc')" :icon="mdiPower">
      <SwitchRow id="rtc-enabled" v-model="form.enabled" :label="t('rtc.enabled')" :description="t('rtc.enabledDesc')" />
    </SettingsSection>

    <SettingsSection :title="t('rtc.secAccess')" :description="t('rtc.secAccessDesc')" :icon="mdiWeb">
      <SettingField
        id="rtc-public-host"
        v-model="form.public_host"
        :label="t('rtc.publicHost')"
        placeholder="rtc.example.com"
        :error="errors.public_host"
        :help="t('rtc.publicHostHelpShort')"
      />
      <p v-if="joinPreview && !errors.public_host" class="text-xs text-slate-500">
        {{ t('rtc.joinPreview') }} <code class="font-mono text-emerald-700 dark:text-emerald-400 break-all">{{ joinPreview }}</code>
      </p>
      <p v-else-if="!form.public_host" class="text-xs rounded-lg bg-amber-50 dark:bg-amber-900/20 text-amber-700 dark:text-amber-300 px-3 py-2">
        {{ t('rtc.httpsNote') }}
      </p>
    </SettingsSection>

    <SettingsSection :title="t('rtc.secTurn')" :description="t('rtc.secTurnDesc')" :icon="mdiTransitConnectionVariant">
      <ChoiceCards v-model="form.turnMode" name="rtc-turn" :options="turnOptions" />
      <SettingField
        v-if="form.turnMode === 'custom'"
        id="rtc-turn-custom"
        v-model="form.turnCustom"
        :label="t('rtc.turnCustomAddr')"
        placeholder=":3478"
        mono
        :error="errors.turnCustom"
      />
      <p v-if="form.turnMode === 'redock'" class="text-xs text-slate-500">
        {{ t('rtc.turnRedockHint') }}
        <router-link :to="{ path: '/rtc', query: { tab: 'turn' } }" class="text-emerald-600 dark:text-emerald-400 underline">{{ t('rtc.openTurnTab') }}</router-link>
      </p>
    </SettingsSection>

    <SettingsSection :title="t('rtc.secMedia')" :description="t('rtc.secMediaDesc')" :icon="mdiLan">
      <div class="grid grid-cols-2 gap-3">
        <SettingField id="rtc-udp-min" v-model="form.udp_min" type="number" min="0" max="65535" :label="t('settingsForm.from')" placeholder="40000" mono />
        <SettingField id="rtc-udp-max" v-model="form.udp_max" type="number" min="0" max="65535" :label="t('settingsForm.to')" placeholder="40199" mono />
      </div>
      <p :class="['text-xs', errors.udp ? 'text-red-600 dark:text-red-400' : 'text-slate-500']">
        {{ errors.udp || (udpCount ? t('rtc.udpCount', { n: udpCount }) : t('rtc.udpAny')) }}
      </p>
    </SettingsSection>

    <SettingsSection :title="t('rtc.secRecording')" :description="t('rtc.secRecordingDesc')" :icon="mdiArchiveClock">
      <SettingField
        id="rtc-retention"
        v-model="form.retention_days"
        type="number"
        min="0"
        max="3650"
        :unit="t('settingsForm.days')"
        :label="t('rtc.retentionDays')"
        :error="errors.retention"
        :help="form.retention_days ? t('rtc.retentionInfo', { days: form.retention_days }) : t('rtc.retentionForever')"
      />
    </SettingsSection>

    <div class="pt-6">
      <button
        type="button"
        class="flex items-center gap-1 text-sm text-slate-500 hover:text-slate-700 dark:hover:text-slate-300"
        :aria-expanded="showAdvanced"
        @click="showAdvanced = !showAdvanced"
      >
        <BaseIcon :path="mdiChevronDown" size="18" :class="['transition-transform', showAdvanced ? 'rotate-180' : '']" />
        {{ t('settingsForm.advanced') }}
      </button>
      <div v-show="showAdvanced || errors.listen_addr" class="mt-4 max-w-md">
        <SettingField
          id="rtc-listen"
          v-model="form.listen_addr"
          :label="t('rtc.listenAddr')"
          placeholder="127.0.0.1:8445"
          mono
          :error="errors.listen_addr"
          :help="t('rtc.listenAddrHelp')"
        />
      </div>
    </div>

    <SaveBar
      :dirty="dirty"
      :busy="busy"
      :errors="errorCount"
      :hint="props.running ? t('rtc.saveHint') : ''"
      @save="save"
      @revert="revert"
    />
  </CardBox>
</template>
