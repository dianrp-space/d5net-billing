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
      "/api": { target: "http://127.0.0.1:8087", changeOrigin: true },
      "/uploads": { target: "http://127.0.0.1:8087", changeOrigin: true },
      "/events": { target: "http://127.0.0.1:8087", changeOrigin: true },
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
        manualChunks(id) {
          if (!id.includes("node_modules")) return;
          if (id.includes("echarts")) return "charts";
          if (id.includes("react-dom") || id.includes("scheduler") || id.includes("/react/")) return "vendor";
          if (id.includes("@radix-ui")) return "radix";
          if (id.includes("@tanstack")) return "tanstack";
          if (id.includes("lucide-react")) return "icons";
          if (id.includes("sweetalert2")) return "sweetalert";
          if (id.includes("leaflet")) return "leaflet";
          return "vendor";
        },
      },
    },
  },
});
