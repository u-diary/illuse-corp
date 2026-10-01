import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// 独自ドメイン（illuse-corp.u-diary.art）で配信するので base は "/" のまま。
export default defineConfig({
  plugins: [react()],
  server: { port: 5173, strictPort: true },
});
