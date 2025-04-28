<template>
  <LayoutAuthenticated>
    <SectionMain>
      <SectionTitleLineWithButton :icon="mdiChartTimelineVariant()" :title="containerId" main>
      </SectionTitleLineWithButton>

      <!-- Saved Commands List -->
      <div v-if="savedCommands != null && savedCommands.length > 0" class="saved-commands">
        <h3>Saved Commands</h3>
        <div class="scrollable-list">
          <ul>
            <li
              v-for="(command, index) in savedCommands"
              :key="index"
              :class="{ selected: selectedCommandIndex === index }"
              @click="selectCommand(index)"
            >
              {{ command.command }}
            </li>
          </ul>
        </div>
        <BaseButton
          v-if="selectedCommandIndex !== null"
          type="button"
          color="primary"
          class="mt-2"
          label="Run"
          @click="runSelectedCommand"
        />
      </div>

      <FormField v-if="containerId != ''" label="" help="">
        <FormControl v-model="domain" type="input" placeholder="domain" />
        <BaseButton type="submit" color="info" label="Enable Debug For Domain" @click="enableDebugForDomain" />
      </FormField>

      <div class="full-height">
        <div ref="terminalContainer" style="width: 100%; height: 100%;"></div>
      </div>
    </SectionMain>
  </LayoutAuthenticated>
</template>

<script>
import SectionMain from '@/components/SectionMain.vue';
import SectionTitleLineWithButton from '@/components/SectionTitleLineWithButton.vue';
import LayoutAuthenticated from '@/layouts/LayoutAuthenticated.vue';
import { mdiChartTimelineVariant } from '@mdi/js';
import FormControl from "@/components/FormControl.vue";
import BaseButton from "@/components/BaseButton.vue";
import FormField from "@/components/FormField.vue";
import ApiService from "@/services/ApiService";
import { Terminal } from '@xterm/xterm';
import { AttachAddon } from 'xterm-addon-attach';
import { FitAddon } from 'xterm-addon-fit';
import 'xterm/css/xterm.css';

export default {
  components: {
    LayoutAuthenticated,
    SectionTitleLineWithButton,
    SectionMain,
    FormControl,
    FormField,
    BaseButton,
  },
  data() {
    return {
      command: '',  // Kullanıcı komutu
      output: '',   // Komut çıktısı
      socket: null, // WebSocket nesnesi
      terminal: null, // Terminal nesnesi
      lastCommand: '',
      command: '',
      whoami: '',
      cardClass: '',
      containerId: '',
      domain: '',
      inputBuffer: '',
      isLastCommandFullScreen: false,
      fullScreenCommands: ['nano', 'htop', 'top'],
      savedCommands: [],
      selectedCommandIndex: null, // Seçilen komutun indeksi
    };
  },
  mounted() {
    this.getAllSavedCommands()
    this.terminal = new Terminal();
    const fitAddon = new FitAddon();
    this.terminal.loadAddon(fitAddon);
    this.terminal.open(this.$refs.terminalContainer);

    fitAddon.fit();

    this.containerId = this.$route.params.id;
    window.location.protocol + '//' + window.location.hostname + (window.location.port !== '' ? ':' + window.location.port : '')
    var url = 'ws' + '://' + window.location.hostname + ':6001/ws';
    if (!(this.containerId == undefined || this.containerId == '')) {
      url += '/' + this.containerId;
    }

    this.socket = new WebSocket(url);

    this.socket.onopen = () => {
      this.resizeTerminal();
      if (!(this.containerId == undefined || this.containerId == '')) {
        this.socket.send('docker exec -it ' + this.containerId + ' bash\n');
      }
    };
    window.addEventListener('resize', this.resizeTerminal);

    const attachAddon = new AttachAddon(this.socket);
    this.terminal.loadAddon(attachAddon);

  },
  beforeUnmount() {
    if (this.socket) {
      this.socket.close();
    }
    window.removeEventListener('resize', this.resizeTerminal);
  },
  methods: {
    enableDebugForDomain() {
      if (this.domain == '') {
        return;
      }
      this.socket.send('export PHP_IDE_CONFIG="serverName=' + this.domain + '"\n');
    },
    resizeTerminal() {
      const windowSize = { high: this.terminal.rows, width: this.terminal.cols };
      const blob = new Blob([JSON.stringify(windowSize)], { type: 'application/json' });
      this.socket.send(blob);
    },
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
      if (this.socket && command) {
        this.socket.send(command.command + '\n');
      }
    },
    selectCommand(index) {
      this.selectedCommandIndex = index;
    },
    runSelectedCommand() {
      if (this.selectedCommandIndex !== null) {
        const command = this.savedCommands[this.selectedCommandIndex];
        if (this.socket && command) {
          this.socket.send(command.command + '\n');
        }
      }
    },
  },
};
</script>

<style scoped>
.full-height {
  height: 60vh; /* Görünüm yüksekliğini kaplar */
  width: 100%; /* Görünüm genişliğini kaplar */
}

.saved-commands {
  margin-bottom: 20px;
}

.scrollable-list {
  max-height: 200px; /* Liste için maksimum yükseklik */
  overflow-y: auto; /* Dikey kaydırma */
  border: 1px solid #ccc;
  border-radius: 4px;
  padding: 10px;
}

.saved-commands ul {
  list-style-type: none;
  padding: 0;
  margin: 0;
}

.saved-commands li {
  padding: 8px 12px;
  cursor: pointer;
  border-radius: 4px;
  transition: background-color 0.2s;
}

.saved-commands li:hover {
  background-color: #f0f0f0;
  color: #000;
}

.saved-commands li.selected {
  background-color: #007bff;
  color: white;
}
</style>
