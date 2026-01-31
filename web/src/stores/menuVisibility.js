import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

const STORAGE_KEY = 'redock-menu-visibility'

const DEFAULT_DISABLED = ['/dns-server', '/vpn-server', '/email-server', '/cloudflare']

function loadStored() {
  if (typeof localStorage === 'undefined') return {}
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return {}
    const parsed = JSON.parse(raw)
    return typeof parsed === 'object' && parsed !== null ? parsed : {}
  } catch {
    return {}
  }
}

function saveStored(visibility) {
  if (typeof localStorage === 'undefined') return
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(visibility))
  } catch {}
}

function getDefaultEnabled(path) {
  return !DEFAULT_DISABLED.includes(path)
}

export const useMenuVisibilityStore = defineStore('menuVisibility', () => {
  const visibility = ref(loadStored())

  function isEnabled(path) {
    if (path in visibility.value) return visibility.value[path] === true
    return getDefaultEnabled(path)
  }

  function setEnabled(path, enabled) {
    const next = { ...visibility.value }
    next[path] = !!enabled
    visibility.value = next
    saveStored(next)
  }

  function toggle(path) {
    setEnabled(path, !isEnabled(path))
  }

  function getVisibilityForItems(items) {
    const out = {}
    items.forEach((item) => {
      out[item.path] = isEnabled(item.path)
    })
    return out
  }

  return {
    visibility: computed(() => visibility.value),
    isEnabled,
    setEnabled,
    toggle,
    getVisibilityForItems
  }
})
