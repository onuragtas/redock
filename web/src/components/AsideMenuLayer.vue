<script setup>
import AsideMenuItem from '@/components/AsideMenuItem.vue'
import AsideMenuList from '@/components/AsideMenuList.vue'
import BaseIcon from '@/components/BaseIcon.vue'
import { mdiClose, mdiLogout, mdiDocker } from '@mdi/js'
import { computed } from 'vue'

defineProps({
  menu: {
    type: Array,
    required: true
  }
})

const emit = defineEmits(['menu-click', 'aside-lg-close-click'])

const logoutItem = computed(() => ({
  label: 'Logout',
  icon: mdiLogout,
  color: 'danger',
  isLogout: true
}))

const menuClick = (event, item) => {
  emit('menu-click', event, item)
}

const asideLgCloseClick = (event) => {
  emit('aside-lg-close-click', event)
}
</script>

<template>
  <aside
    id="aside"
    class="lg:py-4 lg:pl-4 w-64 fixed flex z-40 top-0 h-screen transition-all duration-300 overflow-hidden"
  >
    <div class="bg-gray-900/95 backdrop-blur-xl border-r border-gray-700/50 shadow-2xl lg:rounded-2xl flex-1 flex flex-col overflow-hidden">
      <!-- Brand Header -->
      <div class="flex flex-row h-16 items-center justify-between px-6 border-b border-gray-700/50">
        <div class="flex items-center space-x-3">
          <div class="w-8 h-8 bg-gradient-to-br from-blue-500 to-purple-600 rounded-lg flex items-center justify-center">
            <BaseIcon :path="mdiDocker" size="20" class="text-white" />
          </div>
          <div>
            <h1 class="text-lg font-bold text-white">Redock</h1>
            <p class="text-xs text-gray-400">Container Manager</p>
          </div>
        </div>
        
        <button 
          class="hidden lg:inline-flex xl:hidden p-2 hover:bg-gray-800 rounded-lg transition-colors" 
          @click.prevent="asideLgCloseClick"
        >
          <BaseIcon :path="mdiClose" size="20" class="text-gray-400" />
        </button>
      </div>

      <!-- Navigation Menu -->
      <div class="flex-1 overflow-y-auto overflow-x-hidden p-4">
        <AsideMenuList :menu="menu" @menu-click="menuClick" />
      </div>

      <!-- Footer with Logout -->
      <div class="p-4 border-t border-gray-700/50">
        <AsideMenuItem :item="logoutItem" @menu-click="menuClick" />
      </div>
    </div>
  </aside>
</template>

<style scoped>
/* Custom scrollbar for sidebar */
::-webkit-scrollbar {
  width: 4px;
}

::-webkit-scrollbar-track {
  background: transparent;
}

::-webkit-scrollbar-thumb {
  background: rgba(107, 114, 128, 0.3);
  border-radius: 2px;
}

::-webkit-scrollbar-thumb:hover {
  background: rgba(107, 114, 128, 0.5);
}

/* Smooth transitions */
.transition-all {
  transition-property: all;
  transition-timing-function: cubic-bezier(0.4, 0, 0.2, 1);
}
</style>