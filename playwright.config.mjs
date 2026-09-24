import { defineConfig } from "@playwright/test";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const configDir = dirname(fileURLToPath(import.meta.url));
const browserName = process.env.PLAYWRIGHT_BROWSER || "chromium";

export default defineConfig({
  testDir: join(configDir, "tests/playwright"),
  globalSetup: join(configDir, "tests/playwright/global-setup.mjs"),
  outputDir: join(configDir, "test-results"),
  timeout: 30_000,
  expect: {
    timeout: 5_000,
  },
  fullyParallel: false,
  workers: 1,
  use: {
    browserName,
    channel: browserName === "chromium" ? (process.env.PLAYWRIGHT_BROWSER_CHANNEL || "chrome") : undefined,
    trace: "retain-on-failure",
  },
});
