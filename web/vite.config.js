import { fileURLToPath, URL } from "node:url";

import vue from "@vitejs/plugin-vue";
import { defineConfig } from "vite";

// https://vitejs.dev/config/
export default defineConfig({
  base: "/",
  plugins: [vue()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  // brotli-dec-wasm ships its .wasm file alongside a glue module that
  // references it via a relative URL; Vite's dev-time esbuild dependency
  // pre-bundling doesn't copy that sibling .wasm into node_modules/.vite/deps,
  // causing a 404 at runtime in dev mode only (production's Rollup build
  // handles it correctly on its own). Excluding it from pre-bundling makes
  // the dev server load it as a native ES module instead, which resolves
  // the .wasm URL correctly.
  optimizeDeps: {
    exclude: ["brotli-dec-wasm"],
  },
});
