<script setup>
import { onMounted, ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useToast } from 'vue-toastification'
import {
  mdiCubeOutline, mdiRefresh, mdiPlay, mdiStop, mdiRestart,
  mdiSourceRepository, mdiPlus, mdiDelete, mdiSync, mdiTune, mdiContentSave,
  mdiPencil, mdiToggleSwitch, mdiToggleSwitchOff
} from '@mdi/js'
import ApiService from '@/services/ApiService'
import CardBox from '@/components/CardBox.vue'
import CardBoxModal from '@/components/CardBoxModal.vue'
import BaseButton from '@/components/BaseButton.vue'
import BaseIcon from '@/components/BaseIcon.vue'
import FormField from '@/components/FormField.vue'
import FormControl from '@/components/FormControl.vue'
import SectionTitleLineWithButton from '@/components/SectionTitleLineWithButton.vue'

const { t } = useI18n()
const toast = useToast()

const loading = ref(false)
const services = ref([])
const statusMap = ref({})
const syncErrors = ref({})
const repositories = ref([])
const search = ref('')
const busy = ref('') // service name currently acting on

const isAddRepoModal = ref(false)
const isAddServiceModal = ref(false)
const editRepoName = ref('') // non-empty → repo modal is in edit mode
const editServiceName = ref('') // non-empty → service modal is in edit mode
const newRepo = ref({ name: '', kind: 'compose-url', location: '', compose: '', ref: '', file: null })
const blankService = () => ({
  source_type: 'image', dockerfile: '', files: [],
  name: '', image: '', container_name: '', restart: 'unless-stopped',
  command: '', entrypoint: '', tty: false, stdin_open: false,
  working_dir: '', user: '', hostname: '', labels: '',
  ports: '', expose: '', static_ip: '', aliases: '', depends_on: '', extra_hosts: '', dns: '',
  volumes: '', tmpfs: '', read_only: false, shm_size_mb: 0,
  privileged: false, memory_mb: 0, cpus: 0, pids_limit: 0,
  env: '',
  hc_test: '', hc_interval: '', hc_timeout: '', hc_retries: 0, hc_start_period: ''
})
const newService = ref(blankService())

const repoFilter = ref('')
const catalogRepos = computed(() => {
  const set = new Set()
  for (const s of services.value) if (s.repo) set.add(s.repo)
  return [...set].sort()
})
const filtered = computed(() => {
  const q = search.value.toLowerCase()
  return services.value
    .filter((s) => !repoFilter.value || s.repo === repoFilter.value)
    .filter((s) => !q || (s.name || '').toLowerCase().includes(q) || (s.category || '').includes(q) || (s.repo || '').toLowerCase().includes(q))
    .slice()
    .sort((a, b) => (a.name || '').localeCompare(b.name || ''))
})

const statusFor = (name) => statusMap.value[name] || { active: false, running: false, exists: false }

// The API always returns HTTP 200 with an {error, msg} envelope, so axios never
// rejects on a logical error — inspect the body and surface it.
const apiOk = (res) => {
  if (res?.data?.error) {
    toast.error(res.data.msg || t('stacks.actionFailed'))
    return false
  }
  return true
}

const loadCatalog = async () => {
  loading.value = true
  try {
    const res = await ApiService.getStacksCatalog()
    services.value = res.data?.data?.services || []
    syncErrors.value = res.data?.data?.sync_errors || {}
    await loadStatus()
  } catch (e) {
    toast.error(e.response?.data?.msg || t('stacks.loadFailed'))
  } finally {
    loading.value = false
  }
}

const loadStatus = async () => {
  try {
    const res = await ApiService.getStacksStatus()
    const map = {}
    for (const s of res.data?.data?.status || []) map[s.name] = s
    statusMap.value = map
  } catch (e) {
    /* status optional */
  }
}

const loadRepositories = async () => {
  try {
    const res = await ApiService.getStacksRepositories()
    repositories.value = res.data?.data?.repositories || []
  } catch (e) {
    /* ignore */
  }
}

const up = async (name) => {
  busy.value = name
  try {
    const res = await ApiService.stacksUp([name])
    if (!apiOk(res)) return
    toast.success(t('stacks.started', { name }))
    await loadStatus()
  } catch (e) {
    toast.error(e.response?.data?.msg || e.message || t('stacks.actionFailed'))
  } finally {
    busy.value = ''
  }
}

const down = async (name) => {
  busy.value = name
  try {
    const res = await ApiService.stacksDown(name)
    if (!apiOk(res)) return
    toast.success(t('stacks.stopped', { name }))
    await loadStatus()
  } catch (e) {
    toast.error(e.response?.data?.msg || e.message || t('stacks.actionFailed'))
  } finally {
    busy.value = ''
  }
}

const restart = async (name) => {
  busy.value = name
  try {
    const res = await ApiService.stacksRestart(name)
    if (!apiOk(res)) return
    toast.success(t('stacks.restarted', { name }))
    await loadStatus()
  } catch (e) {
    toast.error(e.response?.data?.msg || e.message || t('stacks.actionFailed'))
  } finally {
    busy.value = ''
  }
}

const syncRepos = async () => {
  loading.value = true
  try {
    const res = await ApiService.syncStacksRepositories()
    if (!apiOk(res)) return
    toast.success(t('stacks.synced'))
    await Promise.all([loadCatalog(), loadEnv()])
  } catch (e) {
    toast.error(e.response?.data?.msg || t('stacks.actionFailed'))
  } finally {
    loading.value = false
  }
}

const openAddRepo = () => {
  editRepoName.value = ''
  newRepo.value = { name: '', kind: 'compose-url', location: '', compose: '', ref: '', file: null }
  isAddRepoModal.value = true
}

const openEditRepo = (repo) => {
  editRepoName.value = repo.name
  newRepo.value = {
    name: repo.name, kind: repo.kind, location: repo.location,
    compose: repo.compose || '', ref: '', file: null
  }
  isAddRepoModal.value = true
}

const submitRepo = async () => {
  const r = newRepo.value
  if (!r.name) {
    toast.error(t('stacks.repoFieldsRequired'))
    return
  }
  try {
    let res
    if (editRepoName.value) {
      // Edit mode: zip re-upload is not supported; source is kind/location/compose.
      if (r.kind !== 'upload' && !r.location) {
        toast.error(t('stacks.repoFieldsRequired'))
        return
      }
      res = await ApiService.updateStacksRepository(editRepoName.value, r)
    } else if (r.kind === 'upload') {
      if (!r.file) {
        toast.error(t('stacks.zipRequired'))
        return
      }
      res = await ApiService.uploadStacksRepository(r.name, r.compose, r.file)
    } else {
      if (!r.location) {
        toast.error(t('stacks.repoFieldsRequired'))
        return
      }
      res = await ApiService.addStacksRepository(r)
    }
    if (!apiOk(res)) return
    toast.success(editRepoName.value ? t('stacks.repoUpdated') : t('stacks.repoAdded'))
    isAddRepoModal.value = false
    editRepoName.value = ''
    newRepo.value = { name: '', kind: 'compose-url', location: '', compose: '', ref: '', file: null }
    await loadRepositories()
    await Promise.all([loadCatalog(), loadEnv()])
  } catch (e) {
    toast.error(e.response?.data?.msg || t('stacks.actionFailed'))
  }
}

const toggleRepo = async (repo) => {
  try {
    const res = await ApiService.toggleStacksRepository(repo.name, !repo.enabled)
    if (!apiOk(res)) return
    await loadRepositories()
    await Promise.all([loadCatalog(), loadEnv()])
  } catch (e) {
    toast.error(e.response?.data?.msg || t('stacks.actionFailed'))
  }
}

const removeRepo = async (name) => {
  try {
    const res = await ApiService.removeStacksRepository(name)
    if (!apiOk(res)) return
    toast.success(t('stacks.repoRemoved'))
    await loadRepositories()
    await Promise.all([loadCatalog(), loadEnv()])
  } catch (e) {
    toast.error(e.response?.data?.msg || t('stacks.actionFailed'))
  }
}

const parsePorts = (text) =>
  (text || '')
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean)
    .map((l) => {
      const [host, container] = l.split(':')
      return { host: (host || '').trim(), container: (container || host || '').trim() }
    })

const parseEnv = (text) => {
  const out = {}
  for (const line of (text || '').split('\n')) {
    const trimmed = line.trim()
    if (!trimmed) continue
    const i = trimmed.indexOf('=')
    if (i > 0) out[trimmed.slice(0, i).trim()] = trimmed.slice(i + 1).trim()
  }
  return out
}

const splitLines = (text) => (text || '').split('\n').map((l) => l.trim()).filter(Boolean)
const splitComma = (text) => (text || '').split(',').map((l) => l.trim()).filter(Boolean)
const splitSpace = (text) => (text || '').trim().split(/\s+/).filter(Boolean)

// Volume lines: "src:dst" or "src:dst:ro". kind 1 (named) if src is a bare name.
const parseVolumes = (text) =>
  splitLines(text).map((l) => {
    const parts = l.split(':')
    const source = (parts[0] || '').trim()
    const target = (parts[1] || '').trim()
    const ro = (parts[2] || '').trim() === 'ro'
    const named = !!source && !/[/.$]/.test(source)
    return { kind: named ? 1 : 0, source, target, read_only: ro }
  })

const submitService = async () => {
  const s = newService.value
  if (!s.name) {
    toast.error(t('stacks.serviceFieldsRequired'))
    return
  }
  if (s.source_type === 'dockerfile' && !s.dockerfile) {
    toast.error(t('stacks.serviceFieldsRequired'))
    return
  }
  if (s.source_type === 'image' && !s.image) {
    toast.error(t('stacks.serviceFieldsRequired'))
    return
  }
  const spec = {
    name: s.name,
    image: s.source_type === 'image' ? s.image : '',
    container_name: s.container_name || undefined,
    restart: s.restart || undefined,
    command: splitSpace(s.command),
    entrypoint: splitSpace(s.entrypoint),
    tty: s.tty,
    stdin_open: s.stdin_open,
    working_dir: s.working_dir || undefined,
    user: s.user || undefined,
    hostname: s.hostname || undefined,
    labels: parseEnv(s.labels),
    ports: parsePorts(s.ports),
    static_ip: s.static_ip || undefined,
    aliases: splitComma(s.aliases),
    depends_on: splitComma(s.depends_on),
    extra_hosts: splitLines(s.extra_hosts),
    dns: splitComma(s.dns),
    volumes: parseVolumes(s.volumes),
    tmpfs: splitLines(s.tmpfs),
    read_only: s.read_only,
    shm_size: Math.round(Number(s.shm_size_mb || 0) * 1024 * 1024),
    privileged: s.privileged,
    memory: Math.round(Number(s.memory_mb || 0) * 1024 * 1024),
    nano_cpus: Math.round(Number(s.cpus || 0) * 1e9),
    pids_limit: Number(s.pids_limit || 0),
    env: parseEnv(s.env)
  }
  if (s.hc_test) {
    spec.healthcheck = {
      test: ['CMD-SHELL', s.hc_test],
      interval: s.hc_interval || undefined,
      timeout: s.hc_timeout || undefined,
      retries: Number(s.hc_retries || 0),
      start_period: s.hc_start_period || undefined
    }
  }
  if (s.source_type === 'dockerfile') {
    spec.dockerfile = s.dockerfile
    spec.files = (s.files || []).filter((f) => f.path)
  }
  try {
    const res = editServiceName.value
      ? await ApiService.updateStacksService(editServiceName.value, spec)
      : await ApiService.addStacksService(spec)
    if (!apiOk(res)) return
    toast.success(editServiceName.value ? t('stacks.serviceUpdated') : t('stacks.serviceAdded'))
    isAddServiceModal.value = false
    editServiceName.value = ''
    newService.value = blankService()
    await Promise.all([loadCatalog(), loadEnv()])
  } catch (e) {
    toast.error(e.response?.data?.msg || t('stacks.actionFailed'))
  }
}

const openAddService = () => {
  editServiceName.value = ''
  newService.value = blankService()
  isAddServiceModal.value = true
}

// volumesToText / portsToText reverse the structured spec back to the textarea form.
const portsToText = (ports) => (ports || []).map((p) => `${p.host}:${p.container}`).join('\n')
const volumesToText = (vols) =>
  (vols || []).map((v) => `${v.source}:${v.target}${v.read_only ? ':ro' : ''}`).join('\n')
const mapToText = (m) => Object.entries(m || {}).map(([k, v]) => `${k}=${v}`).join('\n')

const openEditService = (svc) => {
  editServiceName.value = svc.name
  const hc = svc.healthcheck || {}
  newService.value = {
    source_type: svc.build ? 'dockerfile' : 'image',
    dockerfile: '', files: [],
    name: svc.name, image: svc.image || '', container_name: svc.container_name || '',
    restart: svc.restart || 'unless-stopped',
    command: (svc.command || []).join(' '), entrypoint: (svc.entrypoint || []).join(' '),
    tty: !!svc.tty, stdin_open: !!svc.stdin_open,
    working_dir: svc.working_dir || '', user: svc.user || '', hostname: svc.hostname || '',
    labels: mapToText(svc.labels),
    ports: portsToText(svc.ports), expose: '',
    static_ip: svc.static_ip || '', aliases: (svc.aliases || []).join(', '),
    depends_on: (svc.depends_on || []).join(', '),
    extra_hosts: (svc.extra_hosts || []).join('\n'), dns: (svc.dns || []).join(', '),
    volumes: volumesToText(svc.volumes), tmpfs: (svc.tmpfs || []).join('\n'),
    read_only: !!svc.read_only, shm_size_mb: Math.round((svc.shm_size || 0) / 1048576),
    privileged: !!svc.privileged, memory_mb: Math.round((svc.memory || 0) / 1048576),
    cpus: (svc.nano_cpus || 0) / 1e9, pids_limit: svc.pids_limit || 0,
    env: mapToText(svc.env),
    hc_test: (hc.test || []).slice(1).join(' '), hc_interval: hc.interval || '',
    hc_timeout: hc.timeout || '', hc_retries: hc.retries || 0, hc_start_period: hc.start_period || ''
  }
  isAddServiceModal.value = true
}

const removeService = async (name) => {
  if (!window.confirm(t('stacks.removeServiceConfirm', { name }))) return
  try {
    const res = await ApiService.removeStacksService(name)
    if (!apiOk(res)) return
    toast.success(t('stacks.serviceRemoved'))
    await Promise.all([loadCatalog(), loadEnv()])
  } catch (e) {
    toast.error(e.response?.data?.msg || t('stacks.actionFailed'))
  }
}

const envVars = ref([])
const envSearch = ref('')
const envRepoFilter = ref('')
const envRepos = computed(() => {
  const set = new Set()
  for (const v of envVars.value) for (const r of v.repos || []) set.add(r)
  return [...set].sort()
})
const filteredEnv = computed(() => {
  const q = envSearch.value.toLowerCase()
  return envVars.value
    .filter((v) => !envRepoFilter.value || (v.repos || []).includes(envRepoFilter.value))
    .filter((v) => !q || v.key.toLowerCase().includes(q))
})
const loadEnv = async () => {
  try {
    const res = await ApiService.getStacksEnv()
    envVars.value = res.data?.data?.vars || []
  } catch (e) {
    /* ignore */
  }
}
const addEnvVar = () => envVars.value.unshift({ key: '', value: '', default: '', overridden: true, isNew: true })
const resetEnvVar = (v) => { v.value = v.default }
const saveEnv = async () => {
  const map = {}
  for (const v of envVars.value) {
    if (v.key) map[v.key] = v.value
  }
  try {
    const res = await ApiService.saveStacksEnv(map)
    if (!apiOk(res)) return
    toast.success(t('stacks.envSaved'))
    await Promise.all([loadEnv(), loadCatalog()])
  } catch (e) {
    toast.error(e.response?.data?.msg || e.message || t('stacks.actionFailed'))
  }
}

const refreshAll = async () => {
  await Promise.all([loadRepositories(), loadCatalog(), loadEnv()])
}

onMounted(refreshAll)
</script>

<template>
  <div class="space-y-6">
    <!-- Header -->
    <div class="bg-gradient-to-r from-slate-700 to-slate-900 rounded-xl p-6 text-white">
      <div class="flex flex-col lg:flex-row lg:items-center lg:justify-between gap-4">
        <div class="flex items-center gap-4">
          <div class="w-12 h-12 bg-white/15 rounded-xl flex items-center justify-center">
            <BaseIcon :path="mdiCubeOutline" size="24" class="text-white" />
          </div>
          <div>
            <h1 class="text-2xl lg:text-3xl font-bold">{{ t('stacks.title') }}</h1>
            <p class="text-slate-300">{{ t('stacks.subtitle') }}</p>
          </div>
        </div>
        <BaseButton :icon="mdiRefresh" :label="t('common.refresh')" color="white" outline :loading="loading" @click="refreshAll" />
      </div>
    </div>

    <!-- Import warnings (services that couldn't be fetched/imported) -->
    <div v-if="Object.keys(syncErrors).length" class="rounded-lg bg-orange-50 dark:bg-orange-900/20 border border-orange-200 dark:border-orange-800 p-4">
      <div class="flex items-center gap-2 text-sm font-medium text-orange-800 dark:text-orange-200 mb-2">
        ⚠️ {{ t('stacks.importWarnings') }}
      </div>
      <ul class="space-y-1 text-xs text-orange-700 dark:text-orange-300 font-mono">
        <li v-for="(msg, key) in syncErrors" :key="key">
          <span class="font-semibold">{{ key.replace('service:', '') }}</span>: {{ msg }}
        </li>
      </ul>
    </div>

    <!-- Repositories -->
    <CardBox>
      <SectionTitleLineWithButton :icon="mdiSourceRepository" :title="t('stacks.repositories')" main>
        <div class="flex gap-2">
          <BaseButton :icon="mdiSync" :label="t('stacks.sync')" color="info" outline :loading="loading" @click="syncRepos" />
          <BaseButton :icon="mdiPlus" :label="t('stacks.addRepo')" color="success" @click="openAddRepo" />
        </div>
      </SectionTitleLineWithButton>

      <div v-if="repositories.length === 0" class="text-sm text-slate-500 dark:text-slate-400 py-4">
        {{ t('stacks.noRepos') }}
      </div>
      <div v-else class="space-y-2">
        <div
          v-for="repo in repositories"
          :key="repo.name"
          class="flex items-center justify-between gap-3 p-3 rounded-lg border border-slate-200 dark:border-slate-700"
          :class="{ 'opacity-50': !repo.enabled }"
        >
          <div class="min-w-0">
            <div class="flex items-center gap-2">
              <span class="font-medium">{{ repo.name }}</span>
              <span class="text-xs px-2 py-0.5 rounded-full bg-slate-100 dark:bg-slate-700 text-slate-600 dark:text-slate-300">{{ repo.kind }}</span>
              <span v-if="!repo.enabled" class="text-xs px-2 py-0.5 rounded-full bg-amber-100 dark:bg-amber-900/40 text-amber-700 dark:text-amber-300">{{ t('stacks.disabled') }}</span>
              <span v-if="syncErrors[repo.name]" class="text-xs text-red-500" :title="syncErrors[repo.name]">⚠️</span>
            </div>
            <div class="text-xs text-slate-500 dark:text-slate-400 font-mono truncate">{{ repo.location }}</div>
          </div>
          <div class="flex items-center gap-2 shrink-0">
            <BaseButton
              :icon="repo.enabled ? mdiToggleSwitch : mdiToggleSwitchOff"
              :color="repo.enabled ? 'success' : 'lightDark'"
              small
              outline
              :title="repo.enabled ? t('stacks.disable') : t('stacks.enable')"
              @click="toggleRepo(repo)"
            />
            <BaseButton
              v-if="repo.name !== 'default'"
              :icon="mdiPencil"
              color="info"
              small
              outline
              :title="t('common.edit')"
              @click="openEditRepo(repo)"
            />
            <BaseButton
              v-if="repo.name !== 'default'"
              :icon="mdiDelete"
              color="danger"
              small
              outline
              :title="t('common.delete')"
              @click="removeRepo(repo.name)"
            />
          </div>
        </div>
      </div>
    </CardBox>

    <!-- Environment -->
    <CardBox>
      <SectionTitleLineWithButton :icon="mdiTune" :title="t('stacks.environment')" main>
        <div class="flex gap-2 items-center">
          <select v-model="envRepoFilter" class="rounded-lg border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-800 px-3 py-2 text-sm">
            <option value="">{{ t('stacks.allRepos') }}</option>
            <option v-for="r in envRepos" :key="r" :value="r">{{ r }}</option>
          </select>
          <FormControl v-model="envSearch" :placeholder="t('stacks.envSearch')" class="w-44" />
          <BaseButton :icon="mdiPlus" :label="t('stacks.envAdd')" color="lightDark" outline @click="addEnvVar" />
          <BaseButton :icon="mdiContentSave" :label="t('common.save')" color="success" @click="saveEnv" />
        </div>
      </SectionTitleLineWithButton>
      <p class="text-sm text-slate-500 dark:text-slate-400 mb-3">{{ t('stacks.environmentHint') }}</p>

      <div v-if="filteredEnv.length === 0" class="text-sm text-slate-500 dark:text-slate-400 py-3">{{ t('stacks.envEmpty') }}</div>
      <div v-else class="max-h-96 overflow-y-auto space-y-1.5">
        <div v-for="(v, i) in filteredEnv" :key="v.key + i" class="flex items-start gap-2">
          <div class="w-1/3 min-w-0">
            <FormControl v-if="v.isNew" v-model="v.key" placeholder="KEY" class="font-mono text-xs" />
            <template v-else>
              <div class="font-mono text-xs truncate flex items-center gap-1" :title="v.key">
                {{ v.key }}
                <span v-if="v.overridden" class="text-[10px] px-1 rounded bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-300">{{ t('stacks.envOverride') }}</span>
              </div>
              <div class="flex flex-wrap items-center gap-1 mt-0.5">
                <span v-if="v.repo" class="text-[10px] px-1 rounded bg-slate-100 dark:bg-slate-700 text-slate-500 dark:text-slate-400">{{ v.repo }}</span>
                <span
                  v-if="v.services && v.services.length"
                  class="text-[10px] px-1 rounded bg-blue-100 dark:bg-blue-900/30 text-blue-600 dark:text-blue-300 cursor-help"
                  :title="v.services.join(', ')"
                >{{ t('stacks.envUsedBy', { n: v.services.length }) }}</span>
              </div>
            </template>
          </div>
          <FormControl v-model="v.value" class="flex-1 font-mono text-xs" :placeholder="v.default" />
          <BaseButton
            v-if="!v.isNew && v.default !== undefined && v.value !== v.default"
            :icon="mdiRestart"
            color="lightDark"
            small
            outline
            :title="t('stacks.envReset')"
            @click="resetEnvVar(v)"
          />
        </div>
      </div>
    </CardBox>

    <!-- Catalog -->
    <CardBox>
      <SectionTitleLineWithButton :icon="mdiCubeOutline" :title="t('stacks.catalog')" main>
        <div class="flex gap-2 items-center">
          <select v-model="repoFilter" class="rounded-lg border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-800 px-3 py-2 text-sm">
            <option value="">{{ t('stacks.allRepos') }}</option>
            <option v-for="r in catalogRepos" :key="r" :value="r">{{ r }}</option>
          </select>
          <FormControl v-model="search" :placeholder="t('stacks.searchServices')" class="w-48" />
          <BaseButton :icon="mdiPlus" :label="t('stacks.addService')" color="success" @click="openAddService" />
        </div>
      </SectionTitleLineWithButton>

      <div v-if="loading" class="py-10 text-center text-slate-500">{{ t('common.loading') }}</div>
      <div v-else-if="filtered.length === 0" class="py-10 text-center text-slate-500">{{ t('stacks.noServices') }}</div>
      <div v-else class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead>
            <tr class="border-b border-slate-200 dark:border-slate-700 text-left text-slate-500">
              <th class="py-2 px-3">{{ t('common.name') }}</th>
              <th class="py-2 px-3">{{ t('stacks.repoColumn') }}</th>
              <th class="py-2 px-3">{{ t('stacks.source') }}</th>
              <th class="py-2 px-3">{{ t('common.status') }}</th>
              <th class="py-2 px-3 text-right">{{ t('common.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="s in filtered" :key="s.name" class="border-b border-slate-100 dark:border-slate-800">
              <td class="py-2 px-3">
                <div class="font-medium flex items-center gap-2">
                  {{ s.name }}
                  <span
                    v-if="s.import_error"
                    class="text-xs px-1.5 py-0.5 rounded bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-300"
                    :title="s.import_error"
                  >{{ t('stacks.notImportable') }}</span>
                </div>
                <div class="text-xs text-slate-400">{{ s.category }}</div>
              </td>
              <td class="py-2 px-3">
                <span class="text-xs px-2 py-0.5 rounded bg-slate-100 dark:bg-slate-700 text-slate-600 dark:text-slate-300">{{ s.repo || '—' }}</span>
              </td>
              <td class="py-2 px-3">
                <span v-if="s.build" class="text-xs px-2 py-0.5 rounded bg-indigo-100 dark:bg-indigo-900/30 text-indigo-700 dark:text-indigo-300">{{ t('stacks.build') }}</span>
                <span v-else class="text-xs font-mono text-slate-500 dark:text-slate-400">{{ s.image }}</span>
              </td>
              <td class="py-2 px-3">
                <span
                  v-if="statusFor(s.name).running"
                  class="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-300"
                >● {{ t('common.running') }}</span>
                <span
                  v-else-if="statusFor(s.name).exists"
                  class="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full bg-yellow-100 dark:bg-yellow-900/30 text-yellow-700 dark:text-yellow-300"
                >● {{ statusFor(s.name).state || t('common.stopped') }}</span>
                <span v-else class="text-xs text-slate-400">{{ t('stacks.notStarted') }}</span>
              </td>
              <td class="py-2 px-3">
                <div class="flex justify-end gap-1">
                  <BaseButton :icon="mdiPlay" color="success" small :disabled="!!s.import_error" :loading="busy === s.name" :title="t('stacks.up')" @click="up(s.name)" />
                  <BaseButton :icon="mdiRestart" color="info" small :title="t('common.restart')" @click="restart(s.name)" />
                  <BaseButton :icon="mdiStop" color="danger" small :title="t('stacks.down')" @click="down(s.name)" />
                  <BaseButton v-if="s.repo === 'custom'" :icon="mdiPencil" color="lightDark" small outline :title="t('common.edit')" @click="openEditService(s)" />
                  <BaseButton v-if="s.repo === 'custom'" :icon="mdiDelete" color="danger" small outline :title="t('common.delete')" @click="removeService(s.name)" />
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </CardBox>

    <!-- Add / Edit Repository modal -->
    <CardBoxModal v-model="isAddRepoModal" :title="editRepoName ? t('stacks.editRepo') : t('stacks.addRepo')" :button-label="editRepoName ? t('common.save') : t('common.add')" has-cancel @confirm="submitRepo">
      <FormField :label="t('common.name')">
        <FormControl v-model="newRepo.name" placeholder="my-stack" :disabled="!!editRepoName" />
      </FormField>
      <FormField :label="t('stacks.repoKind')">
        <select v-model="newRepo.kind" :disabled="!!editRepoName && newRepo.kind === 'upload'" class="w-full rounded-lg border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-800 px-3 py-2 text-sm">
          <option value="compose-url">{{ t('stacks.kindComposeUrl') }}</option>
          <option value="git">{{ t('stacks.kindGit') }}</option>
          <option v-if="!editRepoName" value="upload">{{ t('stacks.kindUpload') }}</option>
          <option value="local">{{ t('stacks.kindLocal') }}</option>
        </select>
      </FormField>
      <FormField v-if="newRepo.kind === 'upload'" :label="t('stacks.zipFile')" :help="t('stacks.zipHelp')">
        <input type="file" accept=".zip" class="w-full text-sm" @change="newRepo.file = $event.target.files[0]" />
      </FormField>
      <FormField v-else :label="t('stacks.location')" :help="t('stacks.locationHelp')">
        <FormControl v-model="newRepo.location" :placeholder="newRepo.kind === 'git' ? 'https://github.com/acme/stack.git' : (newRepo.kind === 'local' ? '/path/to/stack' : 'https://.../docker-compose.yml')" />
      </FormField>
      <FormField v-if="newRepo.kind === 'git'" :label="t('stacks.ref')" :help="t('stacks.refHelp')">
        <FormControl v-model="newRepo.ref" placeholder="main" />
      </FormField>
      <FormField :label="t('stacks.composeName')" :help="t('stacks.composeNameHelp')">
        <FormControl v-model="newRepo.compose" placeholder="docker-compose.yml, docker-compose.override.yml" />
      </FormField>
    </CardBoxModal>

    <!-- Add / Edit Service modal -->
    <CardBoxModal v-model="isAddServiceModal" :title="editServiceName ? t('stacks.editService') : t('stacks.addService')" :button-label="editServiceName ? t('common.save') : t('common.add')" has-cancel @confirm="submitService">
      <p class="text-sm text-slate-500 dark:text-slate-400 mb-3">{{ t('stacks.addServiceHint') }}</p>

      <!-- Basic -->
      <div class="grid grid-cols-2 gap-3">
        <FormField :label="t('common.name')">
          <FormControl v-model="newService.name" placeholder="my-redis" :disabled="!!editServiceName" />
        </FormField>
        <FormField :label="t('stacks.sourceType')">
          <select v-model="newService.source_type" :disabled="!!editServiceName" class="w-full rounded-lg border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-800 px-3 py-2 text-sm">
            <option value="image">{{ t('stacks.srcImage') }}</option>
            <option value="dockerfile">{{ t('stacks.srcDockerfile') }}</option>
          </select>
        </FormField>
      </div>
      <FormField v-if="newService.source_type === 'image'" :label="t('stacks.image')">
        <FormControl v-model="newService.image" placeholder="redis:alpine" />
      </FormField>
      <template v-else>
        <FormField :label="t('stacks.dockerfile')" :help="t('stacks.dockerfileHelp')">
          <FormControl v-model="newService.dockerfile" type="textarea" :rows="5" placeholder="FROM php:8.2-fpm&#10;RUN ..." />
        </FormField>
        <div class="rounded-lg border border-slate-200 dark:border-slate-700 p-2">
          <div class="flex items-center justify-between mb-2">
            <span class="text-sm font-medium">{{ t('stacks.extraFiles') }}</span>
            <BaseButton :icon="mdiPlus" :label="t('stacks.addFile')" color="lightDark" small outline @click="newService.files.push({ path: '', content: '' })" />
          </div>
          <p class="text-xs text-slate-500 dark:text-slate-400 mb-2">{{ t('stacks.extraFilesHelp') }}</p>
          <div v-for="(f, i) in newService.files" :key="i" class="mb-2 space-y-1">
            <div class="flex gap-2">
              <FormControl v-model="f.path" :placeholder="t('stacks.filePath')" class="flex-1" />
              <BaseButton :icon="mdiDelete" color="danger" small outline @click="newService.files.splice(i, 1)" />
            </div>
            <FormControl v-model="f.content" type="textarea" :rows="2" :placeholder="t('stacks.fileContent')" />
          </div>
        </div>
      </template>
      <div class="grid grid-cols-2 gap-3">
        <FormField :label="t('stacks.containerName')">
          <FormControl v-model="newService.container_name" :placeholder="newService.name || 'my-redis'" />
        </FormField>
        <FormField :label="t('stacks.restart')">
          <select v-model="newService.restart" class="w-full rounded-lg border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-800 px-3 py-2 text-sm">
            <option value="unless-stopped">unless-stopped</option>
            <option value="always">always</option>
            <option value="on-failure">on-failure</option>
            <option value="no">no</option>
          </select>
        </FormField>
      </div>
      <FormField :label="t('stacks.ports')" :help="t('stacks.portsHelp')">
        <FormControl v-model="newService.ports" type="textarea" :rows="2" placeholder="6379:6379" />
      </FormField>
      <FormField :label="t('stacks.envVars')" :help="t('stacks.envHelp')">
        <FormControl v-model="newService.env" type="textarea" :rows="2" placeholder="KEY=value" />
      </FormField>
      <FormField :label="t('stacks.volumes')" :help="t('stacks.volumesHelp')">
        <FormControl v-model="newService.volumes" type="textarea" :rows="2" placeholder="/host/path:/in/container&#10;myvol:/data:ro" />
      </FormField>

      <!-- Command / runtime -->
      <details class="mt-2 border-t border-slate-200 dark:border-slate-700 pt-2">
        <summary class="cursor-pointer text-sm font-medium text-slate-600 dark:text-slate-300">{{ t('stacks.grpCommand') }}</summary>
        <div class="mt-3 space-y-3">
          <FormField :label="t('stacks.command')"><FormControl v-model="newService.command" placeholder="redis-server --appendonly yes" /></FormField>
          <FormField :label="t('stacks.entrypoint')"><FormControl v-model="newService.entrypoint" placeholder="/entrypoint.sh" /></FormField>
          <div class="grid grid-cols-2 gap-3">
            <FormField :label="t('stacks.workingDir')"><FormControl v-model="newService.working_dir" placeholder="/app" /></FormField>
            <FormField :label="t('stacks.user')"><FormControl v-model="newService.user" placeholder="1000:1000" /></FormField>
          </div>
          <FormField :label="t('stacks.hostname')"><FormControl v-model="newService.hostname" /></FormField>
          <FormField :label="t('stacks.labels')" :help="t('stacks.envHelp')"><FormControl v-model="newService.labels" type="textarea" :rows="2" placeholder="com.example.key=value" /></FormField>
          <div class="flex gap-6">
            <label class="flex items-center gap-2 text-sm cursor-pointer"><input v-model="newService.tty" type="checkbox" /> tty</label>
            <label class="flex items-center gap-2 text-sm cursor-pointer"><input v-model="newService.stdin_open" type="checkbox" /> stdin_open (-i)</label>
          </div>
        </div>
      </details>

      <!-- Networking -->
      <details class="mt-2 border-t border-slate-200 dark:border-slate-700 pt-2">
        <summary class="cursor-pointer text-sm font-medium text-slate-600 dark:text-slate-300">{{ t('stacks.grpNetwork') }}</summary>
        <div class="mt-3 space-y-3">
          <div class="grid grid-cols-2 gap-3">
            <FormField :label="t('stacks.staticIp')"><FormControl v-model="newService.static_ip" placeholder="172.28.1.50" /></FormField>
            <FormField :label="t('stacks.aliases')" :help="t('stacks.commaHelp')"><FormControl v-model="newService.aliases" placeholder="redis, cache" /></FormField>
          </div>
          <FormField :label="t('stacks.dependsOn')" :help="t('stacks.commaHelp')"><FormControl v-model="newService.depends_on" placeholder="db, redis" /></FormField>
          <FormField :label="t('stacks.extraHosts')" :help="t('stacks.extraHostsHelp')"><FormControl v-model="newService.extra_hosts" type="textarea" :rows="2" placeholder="db.local:172.28.1.3" /></FormField>
          <FormField :label="t('stacks.dns')" :help="t('stacks.commaHelp')"><FormControl v-model="newService.dns" placeholder="1.1.1.1, 8.8.8.8" /></FormField>
        </div>
      </details>

      <!-- Resources / storage -->
      <details class="mt-2 border-t border-slate-200 dark:border-slate-700 pt-2">
        <summary class="cursor-pointer text-sm font-medium text-slate-600 dark:text-slate-300">{{ t('stacks.grpResources') }}</summary>
        <div class="mt-3 space-y-3">
          <div class="grid grid-cols-3 gap-3">
            <FormField :label="t('stacks.memoryMb')"><FormControl v-model="newService.memory_mb" type="number" /></FormField>
            <FormField :label="t('stacks.cpus')"><FormControl v-model="newService.cpus" type="number" /></FormField>
            <FormField :label="t('stacks.pidsLimit')"><FormControl v-model="newService.pids_limit" type="number" /></FormField>
          </div>
          <FormField :label="t('stacks.tmpfs')" :help="t('stacks.linesHelp')"><FormControl v-model="newService.tmpfs" type="textarea" :rows="2" placeholder="/tmp" /></FormField>
          <div class="grid grid-cols-2 gap-3">
            <FormField :label="t('stacks.shmSizeMb')"><FormControl v-model="newService.shm_size_mb" type="number" /></FormField>
            <div class="flex items-end gap-6 pb-2">
              <label class="flex items-center gap-2 text-sm cursor-pointer"><input v-model="newService.read_only" type="checkbox" /> read_only</label>
              <label class="flex items-center gap-2 text-sm cursor-pointer"><input v-model="newService.privileged" type="checkbox" /> privileged</label>
            </div>
          </div>
        </div>
      </details>

      <!-- Healthcheck -->
      <details class="mt-2 border-t border-slate-200 dark:border-slate-700 pt-2">
        <summary class="cursor-pointer text-sm font-medium text-slate-600 dark:text-slate-300">{{ t('stacks.grpHealthcheck') }}</summary>
        <div class="mt-3 space-y-3">
          <FormField :label="t('stacks.hcTest')" :help="t('stacks.hcTestHelp')"><FormControl v-model="newService.hc_test" placeholder="curl -f http://localhost || exit 1" /></FormField>
          <div class="grid grid-cols-2 gap-3">
            <FormField :label="t('stacks.hcInterval')"><FormControl v-model="newService.hc_interval" placeholder="30s" /></FormField>
            <FormField :label="t('stacks.hcTimeout')"><FormControl v-model="newService.hc_timeout" placeholder="5s" /></FormField>
          </div>
          <div class="grid grid-cols-2 gap-3">
            <FormField :label="t('stacks.hcRetries')"><FormControl v-model="newService.hc_retries" type="number" /></FormField>
            <FormField :label="t('stacks.hcStartPeriod')"><FormControl v-model="newService.hc_start_period" placeholder="10s" /></FormField>
          </div>
        </div>
      </details>
    </CardBoxModal>
  </div>
</template>
