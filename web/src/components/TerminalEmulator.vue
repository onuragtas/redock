<template>
  <div ref="terminalContainer" style="height: 100%; width: 100%;"></div>
</template>

<script>
import { Terminal } from '@xterm/xterm';
import 'xterm/css/xterm.css';

export default {
  name: 'TerminalEmulator',
  data() {
    return {
      terminal: null, // Terminal nesnesi
    };
  },
  mounted() {
    // Terminali başlat
    this.terminal = new Terminal();
    this.terminal.open(this.$refs.terminalContainer);

    // Örnek veri ekleme
    this.terminal.write('docker exec -it container_name /bin/bash\r\n');

    // Kullanıcıdan veri alma (isteğe bağlı)
    this.terminal.onData(data => {
      this.terminal.write(`\r\nYou typed: ${data}`);
    });
  },
  beforeUnmount() {
    // Terminali temizle
    if (this.terminal) {
      this.terminal.dispose();
    }
  },
};
</script>

<style scoped>
/* Terminal boyutunu doldurması için gerekli CSS */
div {
  background-color: #1e1e1e; /* Terminal arka plan rengi */
}
</style>
