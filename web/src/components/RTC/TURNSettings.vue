<script setup>
// TURN/STUN settings, grouped by decision: where clients reach the server,
// which ports, how much each session may use, how abuse is handled, and
// where relayed traffic may go.
import BaseIcon from "@/components/BaseIcon.vue";
import CardBox from "@/components/CardBox.vue";
import SaveBar from "@/components/RTC/settings/SaveBar.vue";
import SettingField from "@/components/RTC/settings/SettingField.vue";
import SettingsSection from "@/components/RTC/settings/SettingsSection.vue";
import SwitchRow from "@/components/RTC/settings/SwitchRow.vue";
import { isCIDR, isDomain, isIP, isPort, rangeError, splitList } from "@/components/RTC/settings/validators";
import ApiService from "@/services/ApiService";
import { mdiChevronDown, mdiGauge, mdiLan, mdiPower, mdiRouterNetwork, mdiShieldAlert, mdiWeb } from '@mdi/js';
import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useToast } from "vue-toastification";

const props = defineProps({ running: { type: Boolean, default: false } })
const emit = defineEmits(['saved'])

const { t } = useI18n();
const toast = useToast();

const form = ref(null)
const saved = ref('')
const busy = ref(false)
const showAdvanced = ref(false)

const fromServer = (c) => ({
  ...c,
  tls: !!c.tls_port,
  tls_port: c.tls_port || 5349,
  allowText: (c.allow_cidrs || []).join('\n'),
  denyText: (c.deny_cidrs || []).join('\n'),
})

const load = async () => {
  try {
    const res = await ApiService.turnGetConfig()
    if (res.data?.error) throw new Error(res.data.msg)
    form.value = fromServer(res.data.data || {})
    saved.value = JSON.stringify(form.value)
  } catch (e) {
    toast.error(e?.response?.data?.msg || e?.message || t('turn.actionFailed'))
  }
}

const dirty = computed(() => !!form.value && JSON.stringify(form.value) !== saved.value)

const invalidEntries = (text) => splitList(text).filter((v) => !isCIDR(v))

const errors = computed(() => {
  const f = form.value
  const e = {}
  if (!f) return e
  const host = (f.public_host || '').trim()
  if (host && !isDomain(host) && !isIP(host)) e.public_host = t('settingsForm.errHost')
  if (!isIP((f.relay_ip || '').trim())) e.relay_ip = t('settingsForm.errIP')
  if (!isPort(f.port)) e.port = t('settingsForm.errPort')
  if (f.tls && (!isPort(f.tls_port) || f.tls_port === f.port)) e.tls_port = t('turn.errTlsPort')
  const r = rangeError(f.relay_min, f.relay_max, t)
  if (r) e.relay = r
  for (const k of ['max_sessions_per_ip', 'max_bitrate_kbps', 'max_session_minutes', 'auth_fail_limit']) {
    if (!Number.isInteger(f[k]) || f[k] < 0) e[k] = t('settingsForm.errNonNegative')
  }
  if (!Number.isInteger(f.block_minutes) || f.block_minutes < 1) e.block_minutes = t('settingsForm.errPositive')
  const badAllow = invalidEntries(f.allowText)
  if (badAllow.length) e.allow = t('settingsForm.errCidr', { list: badAllow.join(', ') })
  const badDeny = invalidEntries(f.denyText)
  if (badDeny.length) e.deny = t('settingsForm.errCidr', { list: badDeny.join(', ') })
  return e
})
const errorCount = computed(() => Object.keys(errors.value).length)

const relayCount = computed(() => {
  const { relay_min: a, relay_max: b } = form.value || {}
  return a && b && b >= a ? b - a + 1 : 0
})
const mbps = computed(() => (form.value?.max_bitrate_kbps ? (form.value.max_bitrate_kbps / 1000).toLocaleString(undefined, { maximumFractionDigits: 1 }) : ''))

const revert = () => { form.value = JSON.parse(saved.value) }

const save = async () => {
  if (errorCount.value) return
  busy.value = true
  const f = form.value
  try {
    const body = {
      enabled: f.enabled, port: f.port, tls_port: f.tls ? f.tls_port : 0,
      public_host: (f.public_host || '').trim(), relay_ip: (f.relay_ip || '').trim(),
      relay_min: f.relay_min, relay_max: f.relay_max, realm: f.realm,
      allow_private: f.allow_private, allow_cidrs: splitList(f.allowText), deny_cidrs: splitList(f.denyText),
      max_sessions_per_ip: f.max_sessions_per_ip, max_bitrate_kbps: f.max_bitrate_kbps,
      max_session_minutes: f.max_session_minutes, auth_fail_limit: f.auth_fail_limit, block_minutes: f.block_minutes,
    }
    const res = await ApiService.turnUpdateConfig(body)
    if (res.data?.error) throw new Error(res.data.msg)
    toast.success(t('turn.saved'))
    emit('saved', res.data.data)
    await load()
  } catch (e) {
    toast.error(e?.response?.data?.msg || e?.message || t('turn.actionFailed'))
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>

<template>
  <CardBox v-if="form">
    <h3 class="text-lg font-semibold mb-6">{{ t('turn.settings') }}</h3>

    <SettingsSection :title="t('turn.secGeneral')" :description="t('turn.secGeneralDesc')" :icon="mdiPower">
      <SwitchRow id="turn-enabled" v-model="form.enabled" :label="t('turn.enabled')" :description="t('turn.enabledDesc')" />
    </SettingsSection>

    <SettingsSection :title="t('turn.secAddress')" :description="t('turn.secAddressDesc')" :icon="mdiWeb">
      <SettingField id="turn-host" v-model="form.public_host" :label="t('turn.publicHost')" placeholder="turn.example.com" :error="errors.public_host" :help="t('turn.publicHostHelp')" />
      <SettingField id="turn-relay-ip" v-model="form.relay_ip" :label="t('turn.relayIP')" placeholder="203.0.113.10" mono :error="errors.relay_ip" :help="t('turn.relayIPHelp')" />
    </SettingsSection>

    <SettingsSection :title="t('turn.secPorts')" :description="t('turn.secPortsDesc')" :icon="mdiLan">
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
        <SettingField id="turn-port" v-model="form.port" type="number" min="1" max="65535" :label="t('turn.port')" mono :error="errors.port" :help="t('turn.portHelp')" />
        <SettingField v-if="form.tls" id="turn-tls-port" v-model="form.tls_port" type="number" min="1" max="65535" :label="t('turn.tlsPort')" mono :error="errors.tls_port" />
      </div>
      <SwitchRow id="turn-tls" v-model="form.tls" :label="t('turn.tlsSwitch')" :description="t('turn.tlsPortHelp')" />
      <div class="grid grid-cols-2 gap-3">
        <SettingField id="turn-relay-min" v-model="form.relay_min" type="number" min="1" max="65535" :label="t('turn.relayFrom')" mono />
        <SettingField id="turn-relay-max" v-model="form.relay_max" type="number" min="1" max="65535" :label="t('turn.relayTo')" mono />
      </div>
      <p :class="['text-xs', errors.relay ? 'text-red-600 dark:text-red-400' : 'text-slate-500']">
        {{ errors.relay || t('turn.relayCount', { n: relayCount }) }}
      </p>
    </SettingsSection>

    <SettingsSection :title="t('turn.secLimits')" :description="t('turn.secLimitsDesc')" :icon="mdiGauge">
      <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
        <SettingField id="turn-per-ip" v-model="form.max_sessions_per_ip" type="number" min="0" :label="t('turn.maxPerIP')" :unit="t('turn.unitSessions')" :error="errors.max_sessions_per_ip" :help="t('turn.zeroIsUnlimited')" />
        <SettingField id="turn-bitrate" v-model="form.max_bitrate_kbps" type="number" min="0" :label="t('turn.bitrate')" unit="kbps" :error="errors.max_bitrate_kbps" :help="mbps ? t('turn.bitrateMbps', { mbps }) : t('turn.zeroIsUnlimited')" />
        <SettingField id="turn-minutes" v-model="form.max_session_minutes" type="number" min="0" :label="t('turn.maxMinutes')" :unit="t('settingsForm.minutes')" :error="errors.max_session_minutes" :help="t('turn.zeroIsUnlimited')" />
      </div>
    </SettingsSection>

    <SettingsSection :title="t('turn.secAbuse')" :description="t('turn.secAbuseDesc')" :icon="mdiShieldAlert">
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
        <SettingField id="turn-fail" v-model="form.auth_fail_limit" type="number" min="0" :label="t('turn.authFailLimit')" :unit="t('turn.unitPerMinute')" :error="errors.auth_fail_limit" :help="t('turn.authFailLimitHelp')" />
        <SettingField id="turn-block" v-model="form.block_minutes" type="number" min="1" :label="t('turn.blockMinutes')" :unit="t('settingsForm.minutes')" :error="errors.block_minutes" />
      </div>
    </SettingsSection>

    <SettingsSection :title="t('turn.secPolicy')" :description="t('turn.secPolicyDesc')" :icon="mdiRouterNetwork">
      <SwitchRow id="turn-private" v-model="form.allow_private" danger :label="t('turn.allowPrivate')" :description="t('turn.allowPrivateDesc')" />
      <p v-if="form.allow_private" class="text-xs rounded-lg bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-300 px-3 py-2">{{ t('turn.allowPrivateWarning') }}</p>
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
        <SettingField id="turn-allow" :label="t('turn.allowCidrs')" :error="errors.allow" :help="t('turn.allowCidrsHelp')">
          <textarea id="turn-allow" v-model="form.allowText" rows="4" placeholder="10.0.5.0/24" :class="['w-full rounded-lg border bg-white dark:bg-slate-800 px-3 py-2 text-sm font-mono focus:outline-none focus:ring-2 focus:ring-emerald-500/40', errors.allow ? 'border-red-500' : 'border-slate-300 dark:border-slate-600']" />
        </SettingField>
        <SettingField id="turn-deny" :label="t('turn.denyCidrs')" :error="errors.deny" :help="t('turn.denyCidrsHelp')">
          <textarea id="turn-deny" v-model="form.denyText" rows="4" placeholder="203.0.113.0/24" :class="['w-full rounded-lg border bg-white dark:bg-slate-800 px-3 py-2 text-sm font-mono focus:outline-none focus:ring-2 focus:ring-emerald-500/40', errors.deny ? 'border-red-500' : 'border-slate-300 dark:border-slate-600']" />
        </SettingField>
      </div>
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
      <div v-show="showAdvanced" class="mt-4 max-w-md">
        <SettingField id="turn-realm" v-model="form.realm" :label="t('turn.realm')" placeholder="redock" mono :help="t('turn.realmHelp')" />
      </div>
    </div>

    <SaveBar
      :dirty="dirty"
      :busy="busy"
      :errors="errorCount"
      :hint="props.running ? t('turn.saveHint') : ''"
      @save="save"
      @revert="revert"
    />
  </CardBox>
</template>
