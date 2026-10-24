import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// 生产构建产物直接落 web/dist，由 make web 同步进 internal/webui/dist 后 go:embed。
// 开发期用 Vite 代理 /api → 本地 norvels serve，避免跨域与鉴权头问题。
export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    strictPort: true,
    proxy: {
      "/api": {
        target: process.env.NORVELS_API ?? "http://127.0.0.1:8080",
        changeOrigin: false,
        headers: process.env.NORVELS_AUTH_TOKEN
          ? { Authorization: `Bearer ${process.env.NORVELS_AUTH_TOKEN}` }
          : undefined,
      },
    },
  },
});
