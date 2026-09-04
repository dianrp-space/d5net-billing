import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  use: { baseURL: "http://127.0.0.1:8080" },
  projects: [{ name: "api", use: { ...devices["Desktop Chrome"] } }],
});
