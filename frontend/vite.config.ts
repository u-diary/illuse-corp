import { resolve } from "node:path";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// 独自ドメイン（illuse-corp.u-diary.art）で配信するので base は "/" のまま。
// onboard は "/"、fire は "/fire/" に配置する。
export default defineConfig({
  plugins: [react()],
  server: { port: 5173, strictPort: true },
  build: {
    rollupOptions: {
      input: {
        onboard: resolve(import.meta.dirname, "index.html"),
        fire: resolve(import.meta.dirname, "fire/index.html"),
      },
    },
  },
});
