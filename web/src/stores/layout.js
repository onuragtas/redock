import { defineStore } from 'pinia'
import { ref } from 'vue'

const STORAGE_KEY = 'redock-sidebar-collapsed'

function getStored() {
  if (typeof localStorage === 'undefined') return false
  try {
    const v = localStorage.getItem(STORAGE_KEY)
    return v === '1' || v === 'true'
  } catch {
    return false
  }
}

function setStored(collapsed) {
  if (typeof localStorage === 'undefined') return
  try {
    localStorage.setItem(STORAGE_KEY, collapsed ? '1' : '0')
  } catch {}
}

export const useLayoutStore = defineStore('layout', () => {
  const sidebarCollapsed = ref(getStored())

  function setSidebarCollapsed(value) {
    sidebarCollapsed.value = !!value
    setStored(sidebarCollapsed.value)
  }

  function toggleSidebarCollapsed() {
    sidebarCollapsed.value = !sidebarCollapsed.value
    setStored(sidebarCollapsed.value)
  }

  return {
    sidebarCollapsed,
    setSidebarCollapsed,
    toggleSidebarCollapsed
  }
})
