<template>
  <LayoutAuthenticated>
    <SectionMain>
      <SectionTitleLineWithButton :icon="mdiChartTimelineVariant()" :title="containerId" main>
      </SectionTitleLineWithButton>
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
      whoami: '',
      cardClass: '',
      containerId: '',
      domain: '',
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
    }
  },
};
</script>


<style scoped>
.full-height {
  height: 60vh; /* Görünüm yüksekliğini kaplar */
  width: 100%; /* Görünüm genişliğini kaplar */
}
</style>
