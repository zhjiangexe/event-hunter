import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
    setupFiles: "./src/test/setup.ts",
    css: true,
  },
  server: {
    port: Number(process.env.EVENT_HUNTER_FRONTEND_PORT ?? 28334),
    proxy: {
      "/api": {
        target: process.env.VITE_DEV_API_URL ?? "http://localhost:28333",
        changeOrigin: true,
      },
      "/scenario-api": {
        target: process.env.VITE_DEV_EVENT_LAB_URL ?? "http://localhost:28343",
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/scenario-api/, ""),
      },
    },
  },
});
