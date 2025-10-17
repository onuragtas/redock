<script setup>
import BaseIcon from '@/components/BaseIcon.vue'
import NavBarItemPlain from '@/components/NavBarItemPlain.vue'
import NavBarMenuList from '@/components/NavBarMenuList.vue'
import { containerMaxW } from '@/config.js'
import { mdiAccount, mdiBell, mdiClose, mdiDotsVertical } from '@mdi/js'
import { ref } from 'vue'

const props = defineProps({
  menu: {
    type: Array,
    required: true
  },
  username: {
    type: String,
    default: '-'
  }
})

const { username } = props

const emit = defineEmits(['menu-click'])

const menuClick = (event, item) => {
  emit('menu-click', event, item)
}

const isMenuNavBarActive = ref(false)
</script>

<template>
  <nav class="top-0 inset-x-0 fixed h-16 z-30 w-screen lg:w-auto backdrop-blur-xl bg-gray-900/80 border-b border-gray-700/50 shadow-xl">
    <div class="flex lg:items-stretch px-6" :class="containerMaxW">
      <!-- Left side - Main content slot -->
      <div class="flex flex-1 items-center h-16">
        <slot />
      </div>

      <!-- Right side - User menu and notifications -->
      <div class="flex items-center space-x-4">
        <!-- Notifications -->
        <div class="relative">
          <button class="p-2 text-gray-400 hover:text-white hover:bg-gray-700/50 rounded-lg transition-colors">
            <BaseIcon :path="mdiBell" size="20" />
            <!-- Notification badge -->
            <span class="absolute -top-1 -right-1 w-3 h-3 bg-red-500 rounded-full border-2 border-gray-900"></span>
          </button>
        </div>

        <!-- User Profile -->
        <div class="relative">
          <button class="flex items-center space-x-2 p-2 text-gray-400 hover:text-white hover:bg-gray-700/50 rounded-lg transition-colors">
            <div class="w-8 h-8 bg-gradient-to-br from-blue-500 to-purple-600 rounded-full flex items-center justify-center">
              <BaseIcon :path="mdiAccount" size="16" class="text-white" />
            </div>
            <span class="hidden md:block text-sm font-medium">{{ username }}</span>
          </button>
        </div>

        <!-- Mobile menu toggle -->
        <NavBarItemPlain 
          class="lg:hidden"
          @click.prevent="isMenuNavBarActive = !isMenuNavBarActive"
        >
          <div class="p-2 text-gray-400 hover:text-white hover:bg-gray-700/50 rounded-lg transition-colors">
            <BaseIcon :path="isMenuNavBarActive ? mdiClose : mdiDotsVertical" size="20" />
          </div>
        </NavBarItemPlain>
      </div>

      <!-- Mobile dropdown menu -->
      <div
        class="absolute w-screen top-16 left-0 bg-gray-900/95 backdrop-blur-xl shadow-2xl border-t border-gray-700/50 lg:hidden"
        :class="[isMenuNavBarActive ? 'block' : 'hidden']"
      >
        <div class="max-h-screen-menu overflow-y-auto">
          <NavBarMenuList :menu="menu" @menu-click="menuClick" />
        </div>
      </div>
    </div>
  </nav>
</template>

<style scoped>
/* Smooth transitions */
.transition-colors {
  transition: background-color 0.2s ease-in-out, color 0.2s ease-in-out;
}

/* Glass morphism effect */
nav {
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
}

/* Mobile menu animation */
.lg\:hidden > div {
  animation: slideDown 0.3s ease-out;
}

@keyframes slideDown {
  from {
    opacity: 0;
    transform: translateY(-10px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}
</style>