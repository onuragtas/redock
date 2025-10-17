import { defineStore } from 'pinia'

export const useTerminalStore = defineStore('terminal', {
  state: () => ({
    tabs: [],
    activeTabId: null,
    nextTabId: 1,
    isTerminalVisible: false,
    terminalHeight: 400,
    keepTerminalOpen: false // Terminal'in sayfa değişikliklerinde açık kalması için
  }),

  getters: {
    getTabsForContainer: (state) => (containerId) => {
      return state.tabs.filter(tab => tab.containerId === containerId)
    },
    
    getActiveTab: (state) => {
      return state.tabs.find(tab => tab.id === state.activeTabId)
    },
    
    hasAnyTabs: (state) => {
      return state.tabs.length > 0
    }
  },

  actions: {
    addTab(containerId, name = null) {
      const tabId = this.nextTabId++
      const tab = {
        id: tabId,
        name: name || `Terminal ${tabId}`,
        containerId: containerId,
        terminal: null,
        socket: null,
        fitAddon: null,
        connected: false,
        active: false
      }
      
      this.tabs.push(tab)
      this.setActiveTab(tabId)
      this.showTerminal()
      
      return tab
    },

    removeTab(tabId) {
      const tabIndex = this.tabs.findIndex(t => t.id === tabId)
      if (tabIndex === -1) return null
      
      const tab = this.tabs[tabIndex]
      
      // Close connections
      if (tab.socket) {
        tab.socket.close()
      }
      if (tab.terminal) {
        tab.terminal.dispose()
      }
      
      // Remove tab
      this.tabs.splice(tabIndex, 1)
      
      // Update active tab if necessary
      if (this.activeTabId === tabId && this.tabs.length > 0) {
        this.activeTabId = this.tabs[Math.min(tabIndex, this.tabs.length - 1)].id
      } else if (this.tabs.length === 0) {
        this.activeTabId = null
        this.hideTerminal()
      }
      
      return tab
    },

    setActiveTab(tabId) {
      // Reset all tabs active state
      this.tabs.forEach(tab => {
        tab.active = tab.id === tabId
      })
      
      this.activeTabId = tabId
    },

    fitActiveTerminal() {
      const activeTab = this.getActiveTab
      if (activeTab?.fitAddon?.fit) {
        try {
          activeTab.fitAddon.fit()
          return true
        } catch (error) {
          console.warn('Fit error:', error)
          return false
        }
      }
      return false
    },

    updateTabConnection(tabId, connected) {
      const tab = this.tabs.find(t => t.id === tabId)
      if (tab) {
        tab.connected = connected
      }
    },

    setTabTerminal(tabId, terminal, socket, fitAddon) {
      const tab = this.tabs.find(t => t.id === tabId)
      if (tab) {
        tab.terminal = terminal
        tab.socket = socket
        tab.fitAddon = fitAddon
      }
    },

    getTab(tabId) {
      return this.tabs.find(t => t.id === tabId)
    },

    showTerminal() {
      this.isTerminalVisible = true
    },

    hideTerminal() {
      this.isTerminalVisible = false
    },

    toggleTerminal() {
      this.isTerminalVisible = !this.isTerminalVisible
    },

    setTerminalHeight(height) {
      this.terminalHeight = Math.max(200, Math.min(800, height))
    },

    setKeepTerminalOpen(keep) {
      this.keepTerminalOpen = keep
      if (keep && this.hasAnyTabs) {
        this.isTerminalVisible = true
      }
    },

    // Terminal'i sadece kullanıcı manuel olarak kapatmışsa gizle
    conditionalHideTerminal() {
      if (!this.keepTerminalOpen) {
        this.isTerminalVisible = false
      }
    }
  }
})
