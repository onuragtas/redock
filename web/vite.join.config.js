import { fileURLToPath, URL } from "node:url";

import vue from "@vitejs/plugin-vue";
import { defineConfig } from "vite";

// The public room-join page. It is built separately into dist/rtc-join with
// relative asset paths, because the SFU serves it under /redock/ (same origin
// as Galène's WebSocket), not from the dashboard's root.
export default defineConfig({
  base: "./",
  publicDir: false, // the dashboard's public files don't belong on the SFU
  plugins: [vue()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  build: {
    outDir: "dist/rtc-join",
    emptyOutDir: true,
    rollupOptions: {
      input: fileURLToPath(new URL("./join.html", import.meta.url)),
    },
  },
});
