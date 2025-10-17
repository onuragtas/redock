<template>
  <div class="space-y-8">
      <!-- Header -->
      <div class="bg-gradient-to-r from-green-600 via-teal-600 to-cyan-600 rounded-2xl p-8 text-white shadow-xl">
        <div class="flex flex-col lg:flex-row lg:items-center lg:justify-between">
          <div>
            <h1 class="text-3xl lg:text-4xl font-bold mb-2 flex items-center">
              <svg class="w-10 h-10 mr-4" fill="currentColor" viewBox="0 0 24 24">
                <path d="M20,19V7H4V19H20M20,3A2,2 0 0,1 22,5V19A2,2 0 0,1 20,21H4A2,2 0 0,1 2,19V5C2,3.89 2.9,3 4,3H20M13,17V15H18V17H13M9.58,13L5.57,9H8.4L11.7,12.3C12.09,12.69 12.09,13.33 11.7,13.72L8.42,17H5.59L9.58,13Z"/>
              </svg>
              Terminal Management
            </h1>
            <p class="text-green-100 text-lg">Create and manage container terminals</p>
          </div>
        </div>
      </div>

      <!-- Quick Actions -->
      <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <!-- Create Terminal -->
        <div class="bg-gray-900/50 backdrop-blur-sm border border-gray-700/50 rounded-2xl p-6 shadow-xl">
          <div class="flex items-center mb-4">
            <svg class="w-6 h-6 text-green-400 mr-3" fill="currentColor" viewBox="0 0 24 24">
              <path d="M12,2A10,10 0 0,1 22,12A10,10 0 0,1 12,22A10,10 0 0,1 2,12A10,10 0 0,1 12,2M13,7H11V11H7V13H11V17H13V13H17V11H13V7Z"/>
            </svg>
            <h3 class="text-xl font-semibold text-white">Create Terminal</h3>
          </div>

          <div class="space-y-4">
            <div>
              <label class="block text-sm font-medium text-gray-300 mb-2">Container ID</label>
              <input
                v-model="newTerminalContainer"
                type="text"
                placeholder="Enter container ID..."
                class="w-full px-4 py-3 bg-gray-800/50 border border-gray-600/50 rounded-xl text-white placeholder-gray-400 focus:ring-2 focus:ring-green-500 focus:border-green-500 transition-all"
              />
            </div>

            <div>
              <label class="block text-sm font-medium text-gray-300 mb-2">Terminal Name (Optional)</label>
              <input
                v-model="newTerminalName"
                type="text"
                placeholder="Custom terminal name..."
                class="w-full px-4 py-3 bg-gray-800/50 border border-gray-600/50 rounded-xl text-white placeholder-gray-400 focus:ring-2 focus:ring-green-500 focus:border-green-500 transition-all"
              />
            </div>

            <button
              @click="createTerminal"
              :disabled="isCreatingTerminal"
              class="w-full bg-gradient-to-r from-green-600 to-emerald-600 hover:from-green-700 hover:to-emerald-700 disabled:from-gray-600 disabled:to-gray-700 text-white font-semibold py-3 px-6 rounded-xl transition-all duration-200 disabled:cursor-not-allowed flex items-center justify-center"
            >
              <svg v-if="isCreatingTerminal" class="animate-spin -ml-1 mr-3 h-5 w-5 text-white" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
              {{ isCreatingTerminal ? 'Creating...' : 'Create Terminal' }}
            </button>
          </div>
        </div>

        <!-- Terminal Status -->
        <div class="bg-gray-900/50 backdrop-blur-sm border border-gray-700/50 rounded-2xl p-6 shadow-xl">
          <div class="flex items-center mb-4">
            <svg class="w-6 h-6 text-blue-400 mr-3" fill="currentColor" viewBox="0 0 24 24">
              <path d="M3,3V21H21V3H3M19,19H5V5H19V19M17,17H7V15H17V17M17,13H7V11H17V13M17,9H7V7H17V9Z"/>
            </svg>
            <h3 class="text-xl font-semibold text-white">Active Terminals</h3>
          </div>

          <div v-if="terminalStore.tabs.length === 0" class="text-center py-8">
            <svg class="w-16 h-16 text-gray-500 mx-auto mb-4" fill="currentColor" viewBox="0 0 24 24">
              <path d="M20,19V7H4V19H20M20,3A2,2 0 0,1 22,5V19A2,2 0 0,1 20,21H4A2,2 0 0,1 2,19V5C2,3.89 2.9,3 4,3H20"/>
            </svg>
            <p class="text-gray-400">No active terminals</p>
            <p class="text-gray-500 text-sm mt-1">Create a new terminal to get started</p>
          </div>

          <div v-else class="space-y-3">
            <div
              v-for="tab in terminalStore.tabs"
              :key="tab.id"
              class="flex items-center justify-between p-4 bg-gray-800/50 rounded-xl border border-gray-600/30"
            >
              <div class="flex items-center space-x-3">
                <div :class="[
                  'w-3 h-3 rounded-full',
                  tab.connected ? 'bg-green-400' : 'bg-red-400'
                ]"></div>
                <div>
                  <p class="text-white font-medium">{{ tab.name }}</p>
                  <p class="text-gray-400 text-sm">{{ tab.containerId }}</p>
                </div>
              </div>

              <div class="flex items-center space-x-2">
                <button
                  @click="switchToTerminal(tab.id)"
                  class="px-3 py-1.5 text-xs bg-blue-600 hover:bg-blue-700 text-white rounded-lg transition-colors"
                >
                  Switch
                </button>
                <button
                  @click="closeTerminal(tab.id)"
                  class="px-3 py-1.5 text-xs bg-red-600 hover:bg-red-700 text-white rounded-lg transition-colors"
                >
                  Close
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Terminal Status Help -->
      <div class="bg-gray-900/50 backdrop-blur-sm border border-gray-700/50 rounded-2xl p-6 shadow-xl">
        <div class="flex items-center mb-4">
          <svg class="w-6 h-6 text-blue-400 mr-3" fill="currentColor" viewBox="0 0 24 24">
            <path d="M11,9H13V7H11M12,20C7.59,20 4,16.41 4,12C4,7.59 7.59,4 12,4C16.41,4 20,7.59 20,12C20,16.41 16.41,20 12,20M12,2A10,10 0 0,0 2,12A10,10 0 0,0 12,22A10,10 0 0,0 22,12A10,10 0 0,0 12,2M11,17H13V11H11V17Z"/>
          </svg>
          <h3 class="text-xl font-semibold text-white">Terminal Information</h3>
        </div>
        
        <div class="space-y-4">
          <div class="bg-blue-500/10 border border-blue-500/20 rounded-lg p-4">
            <h4 class="text-blue-400 font-medium mb-2">Layout Terminal System</h4>
            <p class="text-gray-300 text-sm">
              Terminals are now managed globally and persist across page navigation. 
              You can access them from any page using the sidebar or the floating terminal panel at the bottom.
            </p>
          </div>
          
          <div class="bg-green-500/10 border border-green-500/20 rounded-lg p-4">
            <h4 class="text-green-400 font-medium mb-2">How to Use</h4>
            <ul class="text-gray-300 text-sm space-y-2">
              <li>• Create terminals above with container IDs</li>
              <li>• Access running terminals from the sidebar</li>
              <li>• Terminals remain active while navigating pages</li>
              <li>• Click terminal tabs in sidebar to switch between them</li>
            </ul>
          </div>
        </div>
      </div>
    </div>
</template>

<script>

import { useTerminalStore } from '@/stores/terminalStore';
import { onMounted, ref } from 'vue';
import { useRoute } from 'vue-router';

export default {
  components: {
  },
  setup() {
    const route = useRoute()
    const terminalStore = useTerminalStore()
    
    // Reactive variables
    const newTerminalContainer = ref(route.params.id)
    const newTerminalName = ref('')

    const domain = ref('')
    const isCreatingTerminal = ref(false)
    
    // Methods
    const createTerminal = async () => {
      // Prevent double clicking
      if (isCreatingTerminal.value) {
        return
      }
      
      isCreatingTerminal.value = true
      
      try {
        const containerId = newTerminalContainer.value.trim() || null
        const name = newTerminalName.value.trim() || (containerId ? `Terminal ${containerId}` : 'New Terminal')
        
        // Use parent layout's createNewTerminal function
        // We need to emit an event or call the parent function
        // For now, let's use a direct approach through window or global event
        window.dispatchEvent(new CustomEvent('create-terminal', {
          detail: { containerId, name }
        }))
        
        // Clear form
        newTerminalContainer.value = ''
        newTerminalName.value = ''
      } finally {
        // Reset flag after 1 second
        setTimeout(() => {
          isCreatingTerminal.value = false
        }, 1000)
      }
    }
    
    const switchToTerminal = (tabId) => {
      terminalStore.setActiveTab(tabId)
      terminalStore.showTerminal()
    }
    
    const closeTerminal = (tabId) => {
      terminalStore.removeTab(tabId)
    }
    
    // Lifecycle
    onMounted(() => {
      
      // Auto-create terminal after terminal store is initialized
      // const containerId = route.params.id
      // if (containerId && containerId.trim()) {
      //   // Route'tan container ID var, bunu kullan
      //   newTerminalContainer.value = containerId
      //   // Terminal store hazır olduktan sonra oluştur
      //   setTimeout(() => {
      //     createTerminal()
      //   }, 100)
      // } else {
      //   // Route'ta container ID yok, boş terminal oluştur
      //   setTimeout(() => {
      //     window.dispatchEvent(new CustomEvent('create-terminal', {
      //       detail: { containerId: null, name: 'New Terminal' }
      //     }))
      //   }, 100)
      // }
    })
    
    return {
      terminalStore,
      newTerminalContainer,
      newTerminalName,
      domain,
      isCreatingTerminal,
      createTerminal,
      switchToTerminal,
      closeTerminal
    }
  }
};
</script>

<style scoped>
/* Animation for connection indicator */
@keyframes pulse {
  0%, 100% {
    opacity: 1;
  }
  50% {
    opacity: 0.5;
  }
}

.animate-pulse {
  animation: pulse 2s cubic-bezier(0.4, 0, 0.6, 1) infinite;
}

/* Glass morphism effect */
.bg-gray-900\/50 {
  background: rgba(17, 24, 39, 0.5);
  backdrop-filter: blur(10px);
}

/* Smooth transitions */
.transition-all {
  transition: all 0.2s ease-in-out;
}

/* Responsive design improvements */
@media (max-width: 768px) {
  .grid-cols-1 {
    grid-template-columns: repeat(1, minmax(0, 1fr));
  }
}
</style>