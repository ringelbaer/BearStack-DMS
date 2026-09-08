import { expect, test } from "@playwright/test";
import { startBearStack, stopBearStack, freePort } from "./server-fixture.mjs";
import http from "node:http";
import { mkdtemp, mkdir, writeFile, readFile, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

const model = "yunet-2023mar-sface-2021dec-v1";
const token = "bearstack-group-photo-test-token-00000";
let root, baseURL, app, service;
test.beforeAll(async () => {
  root = await mkdtemp(path.join(os.tmpdir(), "bearstack-groups-e2e-"));
  const photos = path.join(root, "photos");
  await mkdir(photos);
  const png = await readFile(new URL("../../services/faces/tests/fixtures/astronaut.png", import.meta.url));
  for (const name of ["a", "b", "c", "d"]) await writeFile(path.join(photos, name + ".png"), png);
  let call = 0;
  service = http.createServer((request, response) => {
    if (request.headers.authorization !== "Bearer " + token) { response.writeHead(401); response.end(); return; }
    response.setHeader("Content-Type", "application/json");
    if (request.url === "/health") { response.end(JSON.stringify({ ready: true, protocol: 1, model })); return; }
    request.resume(); request.on("end", () => {
      const index = call++;
      const count = [5, 6, 7, 6][index] || 6;
      const offset = [40, 0, 10, 0][index] || 0;
      const faces = Array.from({ length: count }, (_, i) => {
        const embedding = Array(128).fill(0); embedding[offset + i] = 1;
        return { x: .05 + .23 * (i % 4), y: .06 + .36 * Math.floor(i / 4), width: .15, height: .22, confidence: .99, embedding };
      });
      response.end(JSON.stringify({ model, faces }));
    });
  });
  await new Promise(resolve => service.listen(0, "127.0.0.1", resolve));
  const port = await freePort(); baseURL = `http://127.0.0.1:${port}`;
  const configPath = path.join(root, "config.json");
  await writeFile(configPath, JSON.stringify({ addr: `127.0.0.1:${port}`, data_dir: path.join(root, "data"), auth: { credentials: [{ username: "manager", password: "secret", role: "photos_manager" }] }, photos: { enabled: true, root_dir: photos, face_service_url: `http://127.0.0.1:${service.address().port}`, face_service_token: token } }));
  app = await startBearStack({ configPath, baseURL }, { username: "manager", password: "secret" });
});
test.afterAll(async ({}, testInfo) => {
  testInfo.setTimeout(75_000);
  await stopBearStack(app);
  if (service) await new Promise(resolve => service.close(resolve));
  if (root) await rm(root, { recursive: true, force: true });
});

test("group photos: hover, whole-group naming, ignore, skip and retry", async ({ browser }) => {
  test.setTimeout(60_000);
  const context = await browser.newContext({ httpCredentials: { username: "manager", password: "secret" } });
  const page = await context.newPage();
  const errors = []; page.on("pageerror", error => errors.push(error.message));
  await page.goto(baseURL + "/login");
  await page.getByLabel("Benutzername").fill("manager"); await page.locator('input[name="password"]').fill("secret"); await page.getByRole("button", { name: "Anmelden" }).click();
  const enabled = await context.request.post(baseURL + "/settings/photos/faces", { form: { enabled: "1", delay_millis: "100" }, headers: { Origin: baseURL } });
  expect(enabled.ok()).toBe(true);
  await expect.poll(async () => (await (await context.request.get(baseURL + "/settings/photos/faces?format=json")).json()).status.done, { timeout: 20_000 }).toBe(4);
  await page.goto(baseURL + "/photos/people"); await page.getByRole("link", { name: "Gruppenbilder", exact: true }).click();
  const surface = page.locator("[data-group-photos]");
  const cards = page.locator("[data-group-face]");
  const image = page.locator("[data-group-image]");
  const box = page.locator("[data-group-box]");
  const threshold = page.locator('[data-group-filter] input[name="min"]');
  await expect(threshold).toHaveValue("5");
  await expect(surface).toHaveAttribute("data-path", "b.png"); await expect(cards).toHaveCount(6);
  await expect.poll(() => image.evaluate(img => img.complete && img.naturalWidth > 0)).toBe(true);
  for (const width of [1440, 390, 320]) {
    await page.setViewportSize({ width, height: 900 });
    await cards.first().locator("[data-group-highlight]").hover();
    await expect(box).toBeVisible();
    const result = await page.evaluate(() => {
      const card = document.querySelector("[data-group-face]");
      const image = document.querySelector("[data-group-image]");
      const rect = image.getBoundingClientRect(), box = document.querySelector("[data-group-box]").getBoundingClientRect();
      const factor = Math.min(rect.width / image.naturalWidth, rect.height / image.naturalHeight);
      const w = factor * image.naturalWidth, h = factor * image.naturalHeight;
      const grid = document.querySelector("[data-group-grid]").getBoundingClientRect();
      return { helpWidth: document.querySelector("#group-photo-filter-help").getBoundingClientRect().width, filterWidth: document.querySelector("[data-group-filter]").getBoundingClientRect().width, overflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
        dx: box.left - (rect.left + (rect.width - w) / 2 + Number(card.dataset.x) * w),
        dy: box.top - (rect.top + (rect.height - h) / 2 + Number(card.dataset.y) * h),
        dw: box.width - Number(card.dataset.width) * w, dh: box.height - Number(card.dataset.height) * h,
        photoRight: rect.right, photoBottom: rect.bottom, gridLeft: grid.left, gridTop: grid.top,
        squares: [...document.querySelectorAll('[data-group-face] img')].every(img => Math.abs(img.getBoundingClientRect().width - img.getBoundingClientRect().height) < 1) };
    });
    expect(result.overflow, `overflow at ${width}`).toBeLessThanOrEqual(1);
    for (const key of ["dx", "dy", "dw", "dh"]) expect(Math.abs(result[key]), `${key} at ${width}`).toBeLessThanOrEqual(1.5);
    expect(result.squares).toBe(true);
    expect(result.helpWidth).toBeGreaterThan(result.filterWidth - 45);
    if (width > 900) expect(result.gridLeft).toBeGreaterThan(result.photoRight);
    else expect(result.gridTop).toBeGreaterThan(result.photoBottom);
    await page.screenshot({ path: `/tmp/bearstack-group-photos-${width}.png`, fullPage: true });
  }
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.mouse.move(4, 4);
  await cards.nth(1).locator("[data-group-highlight]").focus();
  await expect(box).toBeVisible();
  await page.evaluate(() => { window.groupOriginal = document.querySelector("[data-group-image]"); window.groupThumb = document.querySelector("[data-group-face] img"); });
  const modal = page.locator("[data-person-dialog]");
  await cards.first().locator("[data-person-edit]").click();
  await modal.getByRole("combobox", { name: "Name", exact: true }).fill("Ada");
  await modal.getByRole("option", { name: /Neu anlegen:.*Ada/ }).click();
  await expect(modal).not.toBeVisible();
  await expect(surface).toHaveAttribute("data-path", "b.png");
  await expect(page.locator("[data-group-count]")).toContainText("5 unbearbeitete");
  await expect(cards.first().locator("[data-person-edit]")).toHaveCount(0);
  expect(await page.evaluate(() => window.groupOriginal === document.querySelector("[data-group-image]") && window.groupThumb === document.querySelector("[data-group-face] img"))).toBe(true);
  const other = await (await context.request.get(baseURL + "/photos/people/groups?format=json&path=d.png")).json();
  expect(other.photo.faces[0].name).toBe("Ada");
  await cards.nth(1).locator("[data-person-edit]").click();
  await modal.getByRole("combobox", { name: "Name", exact: true }).fill("Ada");
  await modal.getByRole("option", { name: /^Ada \(#/ }).click();
  await expect(modal).not.toBeVisible();
  await expect(page.locator("[data-group-count]")).toContainText("4 unbearbeitete");
  await page.route("**/photos/people/groups/ignore", route => route.fulfill({ status: 503, contentType: "application/json", body: JSON.stringify({ error: "Speichern fehlgeschlagen" }) }));
  await page.getByRole("button", { name: "Verbleibende ignorieren" }).click();
  await expect(page.locator("[data-people-status]")).toHaveText("Speichern fehlgeschlagen");
  await expect(surface).toHaveAttribute("data-path", "b.png");
  await page.unroute("**/photos/people/groups/ignore");
  const blockNext = async route => {
    const url = new URL(route.request().url());
    if (url.searchParams.get("after") === "b.png") await route.fulfill({ status: 503, contentType: "application/json", body: JSON.stringify({ error: "Nächstes Foto nicht erreichbar" }) });
    else await route.continue();
  };
  await page.route("**/photos/people/groups?*", blockNext);
  await page.getByRole("button", { name: "Verbleibende ignorieren" }).click();
  await expect(page.locator("[data-people-status]")).toContainText("Gesichter gespeichert");
  await expect(page.getByRole("button", { name: "Verbleibende ignorieren" })).toBeDisabled();
  await page.unroute("**/photos/people/groups?*", blockNext);
  await page.getByRole("button", { name: "Ansicht erneut laden" }).click();
  await expect(surface).toHaveAttribute("data-path", "c.png");
  const b = await (await context.request.get(baseURL + "/photos/people/groups?format=json&path=b.png")).json();
  expect(b.photo.faces.filter(face => face.ignored)).toHaveLength(4);
  expect(b.photo.faces.filter(face => face.name === "Ada" && !face.ignored)).toHaveLength(2);
  const d = await (await context.request.get(baseURL + "/photos/people/groups?format=json&path=d.png")).json();
  expect(d.photo.faces.filter(face => face.ignored)).toHaveLength(0);
  await page.getByRole("link", { name: "Überspringen / nächstes Foto" }).click();
  await expect(page.locator("[data-group-empty]")).toBeVisible();
  await page.getByRole("button", { name: "Durchlauf starten" }).click();
  await expect(surface).toHaveAttribute("data-path", "c.png");
  const c = await (await context.request.get(baseURL + "/photos/people/groups?format=json&path=c.png")).json();
  expect(c.photo.remaining).toBe(7);
  await context.request.post(baseURL + "/photos/people/" + c.photo.faces[0].person_id + "/rename", { form: { name: "Concurrent" }, headers: { Accept: "application/json", Origin: baseURL } });
  await page.getByRole("button", { name: "Verbleibende ignorieren" }).click();
  await expect(page.locator("[data-people-status]")).toContainText("inzwischen geändert");
  await expect(page.locator("[data-group-count]")).toContainText("6 unbearbeitete");
  await threshold.fill("4"); await page.getByRole("button", { name: "Durchlauf starten" }).click();
  await expect(surface).toHaveAttribute("data-path", "a.png");
  await page.goto(baseURL + "/photos/people/groups");
  await expect(threshold).toHaveValue("4"); await expect(surface).toHaveAttribute("data-path", "a.png");
  await page.route("**/photos/people/groups/image/*", route => route.fulfill({ status: 503, body: "preview unavailable" }));
  await page.reload();
  await expect(page.locator("[data-people-status]")).toContainText("Das Foto konnte nicht geladen werden");
  await page.unroute("**/photos/people/groups/image/*");
  await page.getByRole("button", { name: "Ansicht erneut laden" }).click();
  await expect.poll(() => image.evaluate(img => img.complete && img.naturalWidth > 0)).toBe(true);
  await expect(page.locator("[data-people-status]")).toBeEmpty();
  expect(errors).toEqual([]);

  const noJS = await browser.newContext({ javaScriptEnabled: false, storageState: await context.storageState(), httpCredentials: { username: "manager", password: "secret" } });
  const fallback = await noJS.newPage(); await fallback.goto(baseURL + "/photos/people/groups?min=4");
  await fallback.getByRole("button", { name: "Verbleibende ignorieren" }).click();
  await expect(fallback.locator("[data-group-photos]")).toHaveAttribute("data-path", "c.png");
  await noJS.close(); await context.close();
});
