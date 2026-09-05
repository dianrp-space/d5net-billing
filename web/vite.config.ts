/// <reference types="vitest" />
import path from "node:path";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    host: "0.0.0.0",
    port: 5173,
    strictPort: true,
    proxy: {
      "/api": { target: "http://127.0.0.1:8080", changeOrigin: true },
      "/uploads": { target: "http://127.0.0.1:8080", changeOrigin: true },
      "/events": { target: "http://127.0.0.1:8080", changeOrigin: true },
    },
  },
  test: {
    environment: "jsdom",
    include: ["src/**/*.test.ts"],
  },
  build: {
    outDir: "dist",
    sourcemap: false,
    rollupOptions: {
      output: {
        manualChunks: {
          vendor: ["react", "react-dom"],
          charts: ["echarts", "echarts-for-react"],
        },
      },
    },
  },
});
