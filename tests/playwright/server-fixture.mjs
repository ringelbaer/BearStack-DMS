import { spawn } from "node:child_process";
import net from "node:net";
import path from "node:path";
import { fileURLToPath } from "node:url";

export const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");

export async function startBearStack(fixture, credential = null, extraEnv = {}) {
  if (!process.env.BEARSTACK_E2E_BINARY) throw new Error("Playwright global setup did not build BearStack");
  const child = spawn(process.env.BEARSTACK_E2E_BINARY, [], {
    cwd: repoRoot,
    env: { ...process.env, ...extraEnv, BEARSTACK_CONFIG: fixture.configPath },
    stdio: ["ignore", "pipe", "pipe"],
  });
  const state = { child, output: "" };
  state.done = new Promise((resolve) => {
    child.once("error", (error) => { state.error = error; resolve(); });
    child.once("close", (code, signal) => { state.exited = { code, signal }; resolve(); });
  });
  const collect = (chunk) => { state.output = (state.output + chunk.toString()).slice(-128 * 1024); };
  child.stdout.on("data", collect);
  child.stderr.on("data", collect);
  try {
    await waitForHealth(fixture.baseURL, state, credential);
    return state;
  } catch (error) {
    try {
      await stopBearStack(state);
    } catch (cleanupError) {
      throw new AggregateError([error, cleanupError], "BearStack startup and cleanup failed");
    }
    throw error;
  }
}

async function exitedWithin(state, milliseconds) {
  let timer;
  try {
    return await Promise.race([
      state.done.then(() => true),
      new Promise((resolve) => { timer = setTimeout(() => resolve(false), milliseconds); }),
    ]);
  } finally {
    clearTimeout(timer);
  }
}

export async function stopBearStack(state) {
  if (!state?.child || state.exited || state.error) return;
  state.child.kill("SIGTERM");
  // Allow the application's 60-second drain, then reap before fixture removal.
  if (await exitedWithin(state, 65_000)) {
    if (state.exited?.code !== 0) throw new Error(`BearStack shutdown failed: ${JSON.stringify(state.exited)}\n${state.output}`);
    return;
  }
  state.child.kill("SIGKILL");
  await exitedWithin(state, 5_000);
  throw new Error(`BearStack exceeded its shutdown timeout\n${state.output}`);
}

async function waitForHealth(baseURL, state, credential) {
  const headers = credential ? { authorization: `Basic ${Buffer.from(`${credential.username}:${credential.password}`).toString("base64")}` } : {};
  const deadline = Date.now() + 30_000;
  while (Date.now() < deadline) {
    if (state.error || state.exited) throw new Error(`BearStack failed to start: ${state.error || JSON.stringify(state.exited)}\n${state.output}`);
    try {
      const response = await fetch(`${baseURL}/healthz`, { headers, signal: AbortSignal.timeout(1_000) });
      await response.body?.cancel();
      if (response.ok) return;
    } catch { /* Retry while starting. */ }
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
  throw new Error(`BearStack did not become healthy\n${state.output}`);
}

export async function freePort() {
  return await new Promise((resolve, reject) => {
    const listener = net.createServer();
    listener.once("error", reject);
    listener.listen(0, "127.0.0.1", () => {
      const port = listener.address().port;
      listener.close((error) => error ? reject(error) : resolve(port));
    });
  });
}
