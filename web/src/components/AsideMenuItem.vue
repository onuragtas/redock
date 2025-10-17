<script setup>
import { getButtonColor } from '@/colors.js'
import AsideMenuList from '@/components/AsideMenuList.vue'
import BaseIcon from '@/components/BaseIcon.vue'
import { mdiChevronDown, mdiChevronRight } from '@mdi/js'
import { computed, ref } from 'vue'
import { RouterLink } from 'vue-router'

const props = defineProps({
  item: {
    type: Object,
    required: true
  },
  isDropdownList: Boolean
})

const emit = defineEmits(['menu-click'])

const hasColor = computed(() => props.item && props.item.color)
const isDropdownActive = ref(false)
const hasDropdown = computed(() => !!props.item.menu)

const componentClass = computed(() => [
  'group flex items-center px-3 py-2.5 text-sm font-medium rounded-xl transition-all duration-200',
  props.isDropdownList 
    ? 'ml-4 text-gray-400 hover:text-white hover:bg-gray-800/50' 
    : hasColor.value && props.item.color === 'danger'
      ? 'text-red-400 hover:text-white hover:bg-red-600/20 border border-red-600/20'
      : 'text-gray-300 hover:text-white hover:bg-gray-700/50'
])

const iconClass = computed(() => [
  'flex-shrink-0 mr-3',
  props.isDropdownList ? 'w-4 h-4' : 'w-5 h-5'
])

const menuClick = (event) => {
  emit('menu-click', event, props.item)

  if (hasDropdown.value) {
    isDropdownActive.value = !isDropdownActive.value
  }
}
</script>

<template>
  <li class="mb-1">
    <component
      :is="item.to ? RouterLink : 'a'"
      v-slot="vSlot"
      :to="item.to ?? null"
      :href="item.href ?? null"
      :target="item.target ?? null"
      :class="[
        componentClass,
        vSlot?.isExactActive 
          ? 'bg-gradient-to-r from-blue-600 to-purple-600 text-white shadow-lg' 
          : ''
      ]"
      @click="menuClick"
    >
      <BaseIcon
        v-if="item.icon"
        :path="item.icon"
        :class="[
          iconClass,
          vSlot?.isExactActive ? 'text-white' : ''
        ]"
      />
      
      <span class="flex-1 truncate">{{ item.label }}</span>
      
      <BaseIcon
        v-if="hasDropdown"
        :path="isDropdownActive ? mdiChevronDown : mdiChevronRight"
        class="w-4 h-4 ml-2 transition-transform duration-200"
        :class="{ 'transform rotate-90': isDropdownActive }"
      />
    </component>

    <!-- Dropdown Menu -->
    <transition
      enter-active-class="transition duration-200 ease-out"
      enter-from-class="transform scale-95 opacity-0"
      enter-to-class="transform scale-100 opacity-100"
      leave-active-class="transition duration-75 ease-in"
      leave-from-class="transform scale-100 opacity-100"
      leave-to-class="transform scale-95 opacity-0"
    >
      <div v-show="isDropdownActive && hasDropdown" class="mt-1 ml-2 border-l border-gray-700/50 pl-2">
        <AsideMenuList
          :menu="item.menu"
          is-dropdown-list
          @menu-click="emit('menu-click', $event)"
        />
      </div>
    </transition>
  </li>
</template>

<style scoped>
/* Active link glow effect */
.router-link-exact-active {
  position: relative;
}

.router-link-exact-active::before {
  content: '';
  position: absolute;
  inset: -2px;
  background: linear-gradient(45deg, #3b82f6, #8b5cf6);
  border-radius: 12px;
  z-index: -1;
  filter: blur(4px);
  opacity: 0.6;
}

/* Hover animation */
.group:hover .flex-shrink-0 {
  transform: scale(1.1);
  transition: transform 0.2s ease;
}
</style>