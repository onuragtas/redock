<template>
  <LayoutAuthenticated>
    <SectionMain>
      <SectionTitleLineWithButton :icon="mdiChartTimelineVariant()" :title="containerId" main>
      </SectionTitleLineWithButton>
      <div ref="terminalContainer"></div>
    </SectionMain>
  </LayoutAuthenticated>
</template>

<script>
import SectionMain from '@/components/SectionMain.vue';
import SectionTitleLineWithButton from '@/components/SectionTitleLineWithButton.vue';
import LayoutAuthenticated from '@/layouts/LayoutAuthenticated.vue';
import { mdiChartTimelineVariant } from '@mdi/js';
import { Terminal } from '@xterm/xterm';
import { AttachAddon } from 'xterm-addon-attach';
import { FitAddon } from 'xterm-addon-fit';
import 'xterm/css/xterm.css';

export default {
  components: {
    LayoutAuthenticated,
    SectionTitleLineWithButton,
    SectionMain,
  },
  data() {
    return {
      command: '',  // Kullanıcı komutu
      output: '',   // Komut çıktısı
      socket: null, // WebSocket nesnesi
      terminal: null, // Terminal nesnesi
      lastCommand: '',
      whoami: '',
      cardClass: '',
      containerId: '',
      inputBuffer: '',
      isLastCommandFullScreen: false,
      fullScreenCommands: ['nano', 'htop', 'top']
    };
  },
  mounted() {

    this.terminal = new Terminal();
    const fitAddon = new FitAddon();
    this.terminal.loadAddon(fitAddon);
    this.terminal.open(this.$refs.terminalContainer);
    fitAddon.fit();

    this.containerId = this.$route.params.id;
    window.location.protocol + '//' + window.location.hostname + ':6001'
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
    resizeTerminal() {
      const windowSize = { high: this.terminal.rows, width: this.terminal.cols };
      const blob = new Blob([JSON.stringify(windowSize)], { type: 'application/json' });
      this.socket.send(blob);
    },
    mdiChartTimelineVariant() {
      return mdiChartTimelineVariant;
    }
  },
};
</script>


<style scoped>
/* html ve body'ye yüksekliği 100% yapıyoruz */
html,
body {
  margin: 0;
  padding: 0;
  height: 500px;
  width: 100%;
}

/* Terminali kapsayan div'in genişlik ve yüksekliğini yüzde 100 yapıyoruz */
div {
  height: 500px;
  width: 100%;
  background-color: #1e1e1e;
  display: flex;
  flex-direction: column;
}

/* Terminalin genişliğini ve yüksekliğini ayarlıyoruz */
.xterm {
  height: 500px;
  width: 100%;
  flex-grow: 1;
}
</style>
