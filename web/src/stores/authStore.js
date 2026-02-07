import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

export const useAuthStore = defineStore('auth', () => {
  const userInfo = ref({
    id: null,
    email: '',
    user_role: 'user',
    allowed_menus: []
  })

  const isAdmin = computed(() => userInfo.value.user_role === 'admin')

  function setUser(data) {
    if (!data) {
      userInfo.value = { id: null, email: '', user_role: 'user', allowed_menus: [] }
      return
    }
    userInfo.value = {
      id: data.id,
      email: data.email || userInfo.value.email,
      user_role: data.user_role || 'user',
      allowed_menus: Array.isArray(data.allowed_menus) ? data.allowed_menus : []
    }
  }

  function clearUser() {
    setUser(null)
  }

  function canSeeMenu(path) {
    const allowed = userInfo.value.allowed_menus || []
    return allowed.includes(path)
  }

  return {
    userInfo: computed(() => userInfo.value),
    isAdmin,
    setUser,
    clearUser,
    canSeeMenu
  }
})
