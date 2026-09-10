import { expect, test } from "@playwright/test";
import { startBearStack, stopBearStack, freePort } from "./server-fixture.mjs";
import http from "node:http";
import { mkdtemp, mkdir, writeFile, readFile, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

const model = "yunet-2023mar-sface-2021dec-v1";
const token = "bearstack-reconciliation-test-token-000000";
const portrait = await readFile(new URL("../../services/faces/tests/fixtures/astronaut.png", import.meta.url));
let root, baseURL, app, service;
let inferenceCalls = 0;

async function closeService() {
  if (!service?.listening) return;
  await new Promise((resolve, reject) => {
    service.close(error => error ? reject(error) : resolve());
    service.closeAllConnections();
  });
}

test.beforeAll(async () => {
  root = await mkdtemp(path.join(os.tmpdir(), "bearstack-reconciliation-e2e-"));
  const photos = path.join(root, "photos");
  await mkdir(photos);
  for (const name of ["a", "b", "c", "d"]) await writeFile(path.join(photos, `${name}.png`), portrait);
  service = http.createServer((request, response) => {
    if (request.headers.authorization !== `Bearer ${token}`) {
      response.writeHead(401).end();
      return;
    }
    response.setHeader("Content-Type", "application/json");
    if (request.url === "/health") {
      response.end(JSON.stringify({ ready: true, protocol: 1, model }));
      return;
    }
    request.resume();
    request.on("end", () => {
      const index = inferenceCalls++;
      if (index >= 4) {
        response.writeHead(503).end(JSON.stringify({ error: "Unexpected repeated inference" }));
        return;
      }
      const embedding = Array(128).fill(0);
      const axis = index < 2 ? 0 : 2;
      embedding[axis] = index % 2 ? .52 : 1;
      if (index % 2) embedding[axis + 1] = Math.sqrt(1 - .52 * .52);
      response.end(JSON.stringify({ model, faces: [{ x: .3, y: .1, width: .3, height: .35, confidence: .99, embedding,
        quality: { face_pixels: 153.6, sharpness: 100, reference_eligible: true } }] }));
    });
  });
  await new Promise((resolve, reject) => {
    service.once("error", reject);
    service.listen(0, "127.0.0.1", resolve);
  });
  const port = await freePort();
  baseURL = `http://127.0.0.1:${port}`;
  const configPath = path.join(root, "config.json");
  await writeFile(configPath, JSON.stringify({
    addr: `127.0.0.1:${port}`, data_dir: path.join(root, "data"),
    auth: { credentials: [{ username: "manager", password: "secret", role: "photos_manager" }] },
    photos: { enabled: true, root_dir: photos, face_service_url: `http://127.0.0.1:${service.address().port}`, face_service_token: token },
  }));
  app = await startBearStack({ configPath, baseURL }, { username: "manager", password: "secret" });
});

test.afterAll(async ({}, testInfo) => {
  testInfo.setTimeout(75_000);
  try {
    await stopBearStack(app);
  } finally {
    try {
      await closeService();
    } finally {
      if (root) await rm(root, { recursive: true, force: true });
    }
  }
});

test("pencil names or assigns two unnamed groups atomically", async ({ browser }) => {
  test.setTimeout(90_000);
  const context = await browser.newContext({ httpCredentials: { username: "manager", password: "secret" } });
  try {
    const page = await context.newPage();
    const errors = [];
    page.on("pageerror", error => errors.push(error.message));
    const settingsURL = baseURL + "/settings/photos/faces";
    const status = async () => (await context.request.get(settingsURL + "?format=json&progress=1")).json();
    await page.goto(baseURL + "/login");
    await page.getByLabel("Benutzername").fill("manager");
    await page.locator('input[name="password"]').fill("secret");
    await page.getByRole("button", { name: "Anmelden", exact: true }).click();
    await page.goto(settingsURL);
    await page.locator('select[name="reconcile_enabled"]').selectOption("0");
    await page.getByRole("checkbox", { name: /^Gesichtserkennung aktivieren/ }).check();
    await page.getByRole("spinbutton", { name: /^Pause zwischen Bildern/ }).fill("100");
    await page.getByRole("button", { name: "Speichern", exact: true }).click();
    await expect.poll(async () => (await status()).status.done, { timeout: 20_000 }).toBe(4);
    await page.reload();
    await page.getByRole("checkbox", { name: /^Gesichtserkennung aktivieren/ }).uncheck();
    await page.getByRole("button", { name: "Speichern", exact: true }).click();
    await expect.poll(async () => (await status()).running).toBe(false);
    await page.getByRole("button", { name: "Zuordnungen erneut prüfen", exact: true }).click();
    await expect.poll(async () => {
      const s = await status(); return !s.reconciliation.pending && !s.reconciliation_running;
    }, { timeout: 20_000 }).toBe(true);
    await page.goto(baseURL + "/photos/people/merge-suggestions");
    const cards = page.locator(".face-merge-suggestion");
    const pencil = page.getByRole("button", { name: "Zusammenführen und benennen/zuordnen", exact: true });
    const dialog = page.getByRole("dialog");
    await expect(cards).toHaveCount(2);
    await expect(pencil).toHaveCount(2);
    for (const width of [320,1440]) {
      await page.setViewportSize({ width, height: 900 });
      await pencil.first().click();
      await expect(dialog.getByRole("combobox")).toBeEnabled();
      expect(await dialog.evaluate(el => el.scrollWidth - el.clientWidth)).toBeLessThanOrEqual(1);
      await dialog.getByRole("button", { name: "Abbrechen", exact: true }).click();
      await expect(cards).toHaveCount(2);
    }
    await pencil.first().click();
    await dialog.getByRole("combobox").fill("Ada");
    await dialog.getByRole("button", { name: "Benennen und zusammenführen", exact: true }).click();
    await expect(dialog).not.toBeVisible();
    await expect(cards).toHaveCount(1);
    await expect(page.locator("[data-merge-suggestions]")).toHaveAttribute("aria-busy", "false");
    const people = async () => (await (await context.request.get(baseURL + "/photos/people?format=json&filter=all")).json()).people;
    expect((await people()).filter(p => p.name === "Ada")).toHaveLength(1);
    // A typed duplicate does not merge first; choosing an existing person assigns both groups.
    await pencil.click();
    await dialog.getByRole("combobox").fill("Ada");
    await dialog.getByRole("button", { name: "Benennen und zusammenführen", exact: true }).click();
    await expect(dialog.locator("[data-person-dialog-status]")).toContainText("Dieser Name existiert bereits");
    await expect(cards).toHaveCount(1);
    expect(await people()).toHaveLength(3);
    await dialog.getByRole("combobox").click();
    await dialog.getByRole("option").filter({ hasText: "Ada (#" }).click();
    await expect(dialog).not.toBeVisible();
    await expect(cards).toHaveCount(0);
    const all = await people();
    expect(all).toHaveLength(1);
    expect(all[0].name).toBe("Ada");
    const detail = await (await context.request.get(`${baseURL}/photos/people/${all[0].id}?format=json`)).json();
    expect(detail.faces).toHaveLength(4);
    expect(errors).toEqual([]);
  } finally { await context.close(); }
});
