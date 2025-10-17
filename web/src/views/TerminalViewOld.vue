<template>
  <LayoutAuthenticated>
    <div class="space-y-8 p-6">
      <!-- Header -->
      <div class="bg-gradient-to-r from-green-600 via-teal-600 to-cyan-600 rounded-2xl p-8 text-white shadow-xl">
        <div class="flex flex-col lg:flex-row lg:items-center lg:justify-between">
          <div>
            <h1 class="text-3xl lg:text-4xl font-bold mb-2 flex items-center">
              <svg class="w-10 h-10 mr-4" fill="currentColor" viewBox="0 0 24 24">
                <path d="M20,19V7H4V19H20M20,3A2,2 0 0,1 22,5V19A2,2 0 0,1 20,21H4A2,2 0 0,1 2,19V5C2,3.89 2.9,3 4,3H20M13,17V15H18V17H13M9.58,13L5.57,9H8.4L11.7,12.3C12.09,12.69 12.09,13.33 11.7,13.72L8.42,17H5.59L9.58,13Z"/>
              </svg>
              Terminal - {{ containerId || 'Container' }}
            </h1>
            <p class="text-green-100 text-lg">Interactive container terminal with saved commands</p>
          </div>
        </div>
      </div>

      <!-- Saved Commands Section -->
      <div v-if="savedCommands != null && savedCommands.length > 0" 
           class="bg-gray-900/50 backdrop-blur-sm border border-gray-700/50 rounded-2xl p-6 shadow-xl">
        <div class="flex items-center mb-4">
          <svg class="w-6 h-6 text-cyan-400 mr-3" fill="currentColor" viewBox="0 0 24 24">
            <path d="M3,3V21H21V3H3M19,19H5V5H19V19M17,17H7V15H17V17M17,13H7V11H17V13M17,9H7V7H17V9Z"/>
          </svg>
          <h3 class="text-xl font-semibold text-white">Saved Commands</h3>
        </div>
        
        <div class="bg-black/30 rounded-lg p-4 mb-4 max-h-48 overflow-y-auto">
          <div class="space-y-2">
            <div
              v-for="(command, index) in savedCommands"
              :key="index"
              :class="[
                'p-3 rounded-lg cursor-pointer transition-all duration-200 font-mono text-sm',
                selectedCommandIndex === index 
                  ? 'bg-cyan-500/30 border border-cyan-400/50 text-cyan-100' 
                  : 'bg-gray-800/50 hover:bg-gray-700/50 text-gray-300 hover:text-white border border-gray-600/30'
              ]"
              @click="selectCommand(index)"
            >
              <div class="flex items-center">
                <span class="text-cyan-400 mr-2">$</span>
                <span class="flex-1">{{ command.command }}</span>
                <svg v-if="selectedCommandIndex === index" class="w-4 h-4 text-cyan-400" fill="currentColor" viewBox="0 0 24 24">
                  <path d="M9,20.42L2.79,14.21L5.62,11.38L9,14.77L18.88,4.88L21.71,7.71L9,20.42Z"/>
                </svg>
              </div>
            </div>
          </div>
        </div>
        
        <BaseButton
          v-if="selectedCommandIndex !== null"
          type="button"
          color="primary"
          class="w-full bg-gradient-to-r from-cyan-500 to-teal-500 hover:from-cyan-600 hover:to-teal-600 text-white font-medium py-3 rounded-lg transition-all duration-200 shadow-lg"
          label="Execute Selected Command"
          @click="runSelectedCommand"
        />
      </div>

      <!-- PHP Debug Section -->
      <div v-if="containerId != ''" 
           class="bg-gray-900/50 backdrop-blur-sm border border-gray-700/50 rounded-2xl p-6 shadow-xl">
        <div class="flex items-center mb-4">
          <svg class="w-6 h-6 text-purple-400 mr-3" fill="currentColor" viewBox="0 0 24 24">
            <path d="M12,2A10,10 0 0,0 2,12A10,10 0 0,0 12,22A10,10 0 0,0 22,12A10,10 0 0,0 12,2M12,4A8,8 0 0,1 20,12A8,8 0 0,1 12,20A8,8 0 0,1 4,12A8,8 0 0,1 12,4M11,16.5L6.5,12L7.91,10.59L11,13.67L16.59,8.09L18,9.5L11,16.5Z"/>
          </svg>
          <h3 class="text-xl font-semibold text-white">PHP Debug Configuration</h3>
        </div>
        
        <div class="flex flex-col sm:flex-row gap-4">
          <div class="flex-1">
            <FormControl 
              v-model="domain" 
              type="input" 
              placeholder="Enter domain name for debugging" 
              class="bg-black/30 border-gray-600/50 text-white placeholder-gray-400 rounded-lg px-4 py-3"
            />
          </div>
          <BaseButton 
            type="submit" 
            color="info" 
            label="Enable Debug" 
            @click="enableDebugForDomain"
            class="bg-gradient-to-r from-purple-500 to-pink-500 hover:from-purple-600 hover:to-pink-600 text-white font-medium px-6 py-3 rounded-lg transition-all duration-200 shadow-lg whitespace-nowrap"
          />
        </div>
      </div>

      <!-- Terminal Section with Tabs -->
      <div class="bg-gray-900/50 backdrop-blur-sm border border-gray-700/50 rounded-2xl shadow-xl overflow-hidden">
        <!-- Tab Header -->
        <div class="flex items-center justify-between bg-gray-800/60 px-6 py-4 border-b border-gray-600/30">
          <div class="flex items-center">
            <svg class="w-6 h-6 text-green-400 mr-3" fill="currentColor" viewBox="0 0 24 24">
              <path d="M20,19V7H4V19H20M20,3A2,2 0 0,1 22,5V19A2,2 0 0,1 20,21H4A2,2 0 0,1 2,19V5C2,3.89 2.9,3 4,3H20M13,17V15H18V17H13M9.58,13L5.57,9H8.4L11.7,12.3C12.09,12.69 12.09,13.33 11.7,13.72L8.42,17H5.59L9.58,13Z"/>
            </svg>
            <h3 class="text-xl font-semibold text-white">Container Terminals</h3>
          </div>
          
          <!-- Add New Tab Button -->
          <button
            @click="addNewTab"
            class="flex items-center bg-gradient-to-r from-green-500 to-teal-500 hover:from-green-600 hover:to-teal-600 text-white px-4 py-2 rounded-lg transition-all duration-200 shadow-lg"
          >
            <svg class="w-5 h-5 mr-2" fill="currentColor" viewBox="0 0 24 24">
              <path d="M19,13H13V19H11V13H5V11H11V5H13V11H19V13Z"/>
            </svg>
            New Terminal
          </button>
        </div>

        <!-- Tab Navigation -->
        <div v-if="tabs.length > 0" class="flex bg-gray-800/40 px-6 overflow-x-auto">
          <div
            v-for="tab in tabs"
            :key="tab.id"
            :class="[
              'flex items-center min-w-0 px-4 py-3 cursor-pointer border-b-2 transition-all duration-200',
              activeTabId === tab.id 
                ? 'border-green-400 bg-gray-700/50 text-green-400' 
                : 'border-transparent hover:bg-gray-700/30 text-gray-300 hover:text-white'
            ]"
            @click="switchTab(tab.id)"
          >
            <svg class="w-4 h-4 mr-2 flex-shrink-0" fill="currentColor" viewBox="0 0 24 24">
              <path d="M20,19V7H4V19H20M20,3A2,2 0 0,1 22,5V19A2,2 0 0,1 20,21H4A2,2 0 0,1 2,19V5C2,3.89 2.9,3 4,3H20M13,17V15H18V17H13M9.58,13L5.57,9H8.4L11.7,12.3C12.09,12.69 12.09,13.33 11.7,13.72L8.42,17H5.59L9.58,13Z"/>
            </svg>
            <span class="truncate mr-2">{{ tab.name }}</span>
            <div v-if="tab.connected" class="w-2 h-2 bg-green-400 rounded-full flex-shrink-0 mr-2"></div>
            <div v-else class="w-2 h-2 bg-gray-500 rounded-full flex-shrink-0 mr-2"></div>
            <button
              v-if="tabs.length > 1"
              @click.stop="closeTab(tab.id)"
              class="ml-1 p-1 rounded hover:bg-red-500/20 text-gray-400 hover:text-red-400 transition-colors duration-200 flex-shrink-0"
            >
              <svg class="w-3 h-3" fill="currentColor" viewBox="0 0 24 24">
                <path d="M19,6.41L17.59,5L12,10.59L6.41,5L5,6.41L10.59,12L5,17.59L6.41,19L12,13.41L17.59,19L19,17.59L13.41,12L19,6.41Z"/>
              </svg>
            </button>
          </div>
        </div>

        <!-- Terminal Content -->
        <div class="p-6">
          <div v-if="tabs.length === 0" class="flex items-center justify-center h-96 text-gray-400">
            <div class="text-center">
              <svg class="w-16 h-16 mx-auto mb-4 opacity-50" fill="currentColor" viewBox="0 0 24 24">
                <path d="M20,19V7H4V19H20M20,3A2,2 0 0,1 22,5V19A2,2 0 0,1 20,21H4A2,2 0 0,1 2,19V5C2,3.89 2.9,3 4,3H20M13,17V15H18V17H13M9.58,13L5.57,9H8.4L11.7,12.3C12.09,12.69 12.09,13.33 11.7,13.72L8.42,17H5.59L9.58,13Z"/>
              </svg>
              <p class="text-lg mb-2">No Terminal Sessions</p>
              <p class="text-sm">Click "New Terminal" to start a new terminal session</p>
            </div>
          </div>

          <div v-else class="terminal-wrapper bg-black rounded-lg overflow-hidden shadow-inner">
            <div
              v-for="tab in tabs"
              :key="'terminal-' + tab.id"
              v-show="activeTabId === tab.id"
              :ref="'terminalContainer' + tab.id"
              class="terminal-container"
            ></div>
          </div>
        </div>
      </div>
    </div>
  </LayoutAuthenticated>
</template>

<script>
import BaseButton from "@/components/BaseButton.vue";
import FormControl from "@/components/FormControl.vue";
import FormField from "@/components/FormField.vue";
import LayoutAuthenticated from '@/layouts/LayoutAuthenticated.vue';
import ApiService from "@/services/ApiService";
import { useTerminalStore } from '@/stores/terminalStore';
import { mdiChartTimelineVariant } from '@mdi/js';
import 'xterm/css/xterm.css';

export default {
  components: {
    LayoutAuthenticated,
    FormControl,
    FormField,
    BaseButton,
  },
  setup() {
    return {
      terminalStore: useTerminalStore()
    }
  },
  data() {
    return {
      containerId: '',
      domain: '',
      savedCommands: [],
      selectedCommandIndex: null,
      currentTerminal: null,
    };
  },
  mounted() {
    this.getAllSavedCommands()
    this.containerId = this.$route.params.id
    
    // Get or create terminal from store
    const terminalId = this.$route.params.id ? parseInt(this.$route.params.id) : null
    
    if (terminalId) {
      // Get existing terminal or create if not exists
      this.currentTerminal = this.terminalStore.getTerminalById(terminalId)
      if (!this.currentTerminal) {
        this.currentTerminal = this.terminalStore.createTerminal(this.containerId, `Terminal ${terminalId}`)
      }
      this.terminalStore.setActiveTerminal(terminalId)
    } else {
      // Create new terminal if no ID specified
      this.currentTerminal = this.terminalStore.createTerminal(this.containerId)
    }
    
    // Initialize terminal in next tick
    this.$nextTick(() => {
      this.initializeCurrentTerminal()
    })
    
    window.addEventListener('resize', this.resizeCurrentTerminal)
  },
  beforeUnmount() {
    // Close all terminal connections
    this.tabs.forEach(tab => {
      if (tab.socket) {
        tab.socket.close();
      }
      if (tab.terminal) {
        tab.terminal.dispose();
      }
    });
    window.removeEventListener('resize', this.resizeAllTerminals);
  },
  methods: {
    // Tab Management Methods
    addNewTab() {
      const tabId = this.nextTabId++;
      const tab = {
        id: tabId,
        name: `Terminal ${tabId}`,
        terminal: null,
        socket: null,
        fitAddon: null,
        connected: false
      };
      
      this.tabs.push(tab);
      this.activeTabId = tabId;
      
      // Wait for DOM update then initialize terminal
      this.$nextTick(() => {
        this.initializeTerminal(tab);
      });
    },
    
    switchTab(tabId) {
      this.activeTabId = tabId;
      
      // Resize active terminal after switch
      this.$nextTick(() => {
        const tab = this.tabs.find(t => t.id === tabId);
        if (tab && tab.fitAddon) {
          tab.fitAddon.fit();
        }
      });
    },
    
    closeTab(tabId) {
      const tabIndex = this.tabs.findIndex(t => t.id === tabId);
      if (tabIndex === -1) return;
      
      const tab = this.tabs[tabIndex];
      
      // Close connections
      if (tab.socket) {
        tab.socket.close();
      }
      if (tab.terminal) {
        tab.terminal.dispose();
      }
      
      // Remove tab
      this.tabs.splice(tabIndex, 1);
      
      // Switch to another tab if current was active
      if (this.activeTabId === tabId && this.tabs.length > 0) {
        this.activeTabId = this.tabs[Math.min(tabIndex, this.tabs.length - 1)].id;
      } else if (this.tabs.length === 0) {
        this.activeTabId = null;
      }
    },
    
    initializeTerminal(tab) {
      // Create terminal
      tab.terminal = new Terminal();
      tab.fitAddon = new FitAddon();
      tab.terminal.loadAddon(tab.fitAddon);
      
      // Get terminal container ref
      const containerRef = this.$refs[`terminalContainer${tab.id}`];
      if (!containerRef || !containerRef[0]) {
        console.error('Terminal container not found for tab', tab.id);
        return;
      }
      
      tab.terminal.open(containerRef[0]);
      tab.fitAddon.fit();
      
      // Create WebSocket connection
      let url = window.location.hostname + (window.location.port == '5173' ? ':6001' : (window.location.port !== '' ? ':' + window.location.port : ''));
      url = 'ws' + '://' + url + '/ws';
      if (!(this.containerId == undefined || this.containerId == '')) {
        url += '/' + this.containerId;
      }
      
      tab.socket = new WebSocket(url);
      
      tab.socket.onopen = () => {
        tab.connected = true;
        this.resizeTerminal(tab);
        if (!(this.containerId == undefined || this.containerId == '')) {
          tab.socket.send('docker exec -it ' + this.containerId + ' bash\n');
        }
      };
      
      tab.socket.onclose = () => {
        tab.connected = false;
      };
      
      tab.socket.onerror = () => {
        tab.connected = false;
      };
      
      // Attach terminal to WebSocket
      const attachAddon = new AttachAddon(tab.socket);
      tab.terminal.loadAddon(attachAddon);
    },
    
    resizeTerminal(tab) {
      if (tab && tab.terminal && tab.socket && tab.socket.readyState === WebSocket.OPEN) {
        const windowSize = { high: tab.terminal.rows, width: tab.terminal.cols };
        const blob = new Blob([JSON.stringify(windowSize)], { type: 'application/json' });
        tab.socket.send(blob);
      }
    },
    
    resizeAllTerminals() {
      this.tabs.forEach(tab => {
        if (tab.fitAddon) {
          tab.fitAddon.fit();
        }
        this.resizeTerminal(tab);
      });
    },

    // PHP Debug Method
    enableDebugForDomain() {
      if (this.domain == '') {
        return;
      }
      const activeTab = this.tabs.find(t => t.id === this.activeTabId);
      if (activeTab && activeTab.socket && activeTab.socket.readyState === WebSocket.OPEN) {
        activeTab.socket.send('export PHP_IDE_CONFIG="serverName=' + this.domain + '"\n');
      }
    },
    
    // Saved Commands Methods
    mdiChartTimelineVariant() {
      return mdiChartTimelineVariant;
    },
    getAllSavedCommands() {
      this.savedCommands = [];
      ApiService.getAllSavedCommands().then(value => {
        this.savedCommands = value.data.data
      })
    },
    runSavedCommand(command) {
      const activeTab = this.tabs.find(t => t.id === this.activeTabId);
      if (activeTab && activeTab.socket && activeTab.socket.readyState === WebSocket.OPEN && command) {
        activeTab.socket.send(command.command + '\n');
      }
    },
    selectCommand(index) {
      this.selectedCommandIndex = index;
    },
    runSelectedCommand() {
      if (this.selectedCommandIndex !== null) {
        const command = this.savedCommands[this.selectedCommandIndex];
        const activeTab = this.tabs.find(t => t.id === this.activeTabId);
        if (activeTab && activeTab.socket && activeTab.socket.readyState === WebSocket.OPEN && command) {
          activeTab.socket.send(command.command + '\n');
        }
      }
    },
  },
};
</script>

<style scoped>
.terminal-wrapper {
  position: relative;
  height: 60vh;
  min-height: 500px;
}

.terminal-container {
  width: 100%;
  height: 100%;
  font-family: 'JetBrains Mono', Monaco, 'Courier New', monospace;
}

/* Xterm.js terminal styling */
:deep(.xterm) {
  height: 100% !important;
  padding: 15px;
}

:deep(.xterm .xterm-viewport) {
  overflow-y: auto;
}

:deep(.xterm .xterm-screen) {
  padding: 10px;
}

/* Custom scrollbar for terminal */
:deep(.xterm .xterm-viewport::-webkit-scrollbar) {
  width: 8px;
}

:deep(.xterm .xterm-viewport::-webkit-scrollbar-track) {
  background: #1a1a1a;
}

:deep(.xterm .xterm-viewport::-webkit-scrollbar-thumb) {
  background: #404040;
  border-radius: 4px;
}

:deep(.xterm .xterm-viewport::-webkit-scrollbar-thumb:hover) {
  background: #505050;
}

/* Custom form control styling */
:deep(.form-control) {
  background: rgba(0, 0, 0, 0.3) !important;
  border: 1px solid rgba(107, 114, 128, 0.5) !important;
  color: white !important;
  border-radius: 0.5rem !important;
  padding: 0.75rem 1rem !important;
}

:deep(.form-control::placeholder) {
  color: rgba(156, 163, 175, 1) !important;
}

:deep(.form-control:focus) {
  border-color: rgba(34, 197, 94, 0.5) !important;
  box-shadow: 0 0 0 3px rgba(34, 197, 94, 0.1) !important;
}

/* Button styling */
:deep(.btn) {
  border-radius: 0.5rem !important;
  font-weight: 500 !important;
  transition: all 0.2s ease-in-out !important;
}

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

/* Gradient text effect */
.bg-gradient-to-r {
  background-image: linear-gradient(to right, var(--tw-gradient-stops));
}

/* Smooth transitions */
* {
  transition: all 0.2s ease-in-out;
}

/* Responsive design improvements */
@media (max-width: 768px) {
  .terminal-wrapper {
    height: 50vh;
    min-height: 400px;
  }
  
  :deep(.xterm) {
    padding: 10px;
  }
}
</style>