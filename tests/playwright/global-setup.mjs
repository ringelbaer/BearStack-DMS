import { execFile } from "node:child_process";
import { mkdtemp, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { promisify } from "node:util";
import { repoRoot } from "./server-fixture.mjs";

export default async function globalSetup() {
  const directory = await mkdtemp(path.join(os.tmpdir(), "bearstack-e2e-build-"));
  const binary = path.join(directory, process.platform === "win32" ? "bearstack.exe" : "bearstack");
  try {
    await promisify(execFile)(process.env.GO || "go", ["build", "-o", binary, "./cmd/bearstack"], {
      cwd: repoRoot,
      env: { ...process.env, GOCACHE: process.env.GOCACHE || path.join(os.tmpdir(), "bearstack-playwright-go-cache") },
      timeout: 180_000,
      maxBuffer: 4 * 1024 * 1024,
    });
    process.env.BEARSTACK_E2E_BINARY = binary;
  } catch (error) {
    await rm(directory, { recursive: true, force: true });
    throw error;
  }
  return async () => rm(directory, { recursive: true, force: true });
}
