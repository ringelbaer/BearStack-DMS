import { defineConfig } from "@playwright/test";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const configDir = dirname(fileURLToPath(import.meta.url));

export default defineConfig({
  testDir: join(configDir, "tests/playwright"),
  outputDir: join(configDir, "test-results"),
  timeout: 30_000,
  expect: {
    timeout: 5_000,
  },
  fullyParallel: false,
  workers: 1,
  use: {
    channel: process.env.PLAYWRIGHT_BROWSER_CHANNEL || "chrome",
    trace: "retain-on-failure",
  },
});
