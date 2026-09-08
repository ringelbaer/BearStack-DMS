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

async function expectZoom(page, card) {
  await expect(card.locator("[data-group-highlight]")).toHaveAttribute("aria-pressed", "true");
  await expect(page.locator('[data-group-highlight][aria-pressed="true"]')).toHaveCount(1);
  await expect.poll(async () => card.evaluate(card => {
    const image = document.querySelector("[data-group-image]");
    const stage = document.querySelector("[data-group-stage]").getBoundingClientRect();
    const rect = image.getBoundingClientRect(), box = document.querySelector("[data-group-box]").getBoundingClientRect();
    const factor = Math.min(rect.width / image.naturalWidth, rect.height / image.naturalHeight);
    const w = factor * image.naturalWidth, h = factor * image.naturalHeight;
    const left = rect.left + (rect.width - w) / 2, top = rect.top + (rect.height - h) / 2;
    const baseFactor = Math.min(stage.width / image.naturalWidth, stage.height / image.naturalHeight);
    return Math.max(
      Math.abs(box.left - (left + Number(card.dataset.x) * w)),
      Math.abs(box.top - (top + Number(card.dataset.y) * h)),
      Math.abs(box.width - Number(card.dataset.width) * w),
      Math.abs(box.height - Number(card.dataset.height) * h),
      // One dimension of the doubled bounding box fills the stage.
      Math.min(Math.abs(2 * box.width - stage.width), Math.abs(2 * box.height - stage.height)),
      stage.left - box.left, box.right - stage.right, stage.top - box.top, box.bottom - stage.bottom,
      // Keep image edges inside the frame only when the whole dimension fits.
      w >= stage.width ? left - stage.left : Math.abs(left + w / 2 - (stage.left + stage.width / 2)),
      h >= stage.height ? top - stage.top : Math.abs(top + h / 2 - (stage.top + stage.height / 2)),
      w >= stage.width ? stage.right - (left + w) : 0,
      h >= stage.height ? stage.bottom - (top + h) : 0,
      factor > baseFactor ? 0 : 1000
    );
  })).toBeLessThanOrEqual(1.5);
}

test("group photos: hover, zoom, whole-group naming, ignore, skip and retry", async ({ browser }) => {
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
  await expect(page.locator('script[src*="/static/app-person-dialog.js"]')).toHaveCount(1);
  await expect(page.locator('script[src*="/static/app-people.js"]')).toHaveCount(0);
  const surface = page.locator("[data-group-photos]");
  const cards = page.locator("[data-group-face]");
  const image = page.locator("[data-group-image]");
  const box = page.locator("[data-group-box]");
  const threshold = page.locator('[data-group-filter] input[name="min"]');
  await expect(threshold).toHaveValue("5");
  await expect(surface).toHaveAttribute("data-path", "b.png"); await expect(cards).toHaveCount(6);
  await expect.poll(() => image.evaluate(img => img.complete && img.naturalWidth > 0)).toBe(true);
  const zoomRequests = [];
  page.on("request", request => { if (request.url().includes("/photos/people/groups")) zoomRequests.push(request.url()); });
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
    const preview = cards.first().locator("[data-group-highlight]");
    await preview.click();
    await expectZoom(page, cards.first());
    await cards.nth(3).locator("[data-group-highlight]").hover();
    await expectZoom(page, cards.first());
    await cards.nth(3).locator("[data-group-highlight]").click();
    await expectZoom(page, cards.nth(3));
    await expect(preview).toHaveAttribute("aria-pressed", "false");
    await cards.nth(3).locator("[data-group-highlight]").click();
    await expect(image).toHaveCSS("transform", "none");
    await expect(page.locator('[data-group-highlight][aria-pressed="true"]')).toHaveCount(0);
  }
  expect(zoomRequests).toEqual([]);
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.mouse.move(4, 4);
  await cards.nth(1).locator("[data-group-highlight]").focus();
  await expect(box).toBeVisible();
  await page.keyboard.press("Enter");
  await expectZoom(page, cards.nth(1));
  await page.setViewportSize({ width: 390, height: 900 });
  await expectZoom(page, cards.nth(1));
  await page.keyboard.press("Space");
  await expect(image).toHaveCSS("transform", "none");
  await page.setViewportSize({ width: 1440, height: 900 });
  // Natural dimensions already include the decoded photo's orientation.
  const originalSource = await image.getAttribute("src");
  for (const [width, height] of [[800, 400], [400, 800]]) {
    await image.evaluate((img, dimensions) => { img.src = "data:image/svg+xml," + encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" width="${dimensions[0]}" height="${dimensions[1]}"><rect width="100%" height="100%" fill="gray"/></svg>`); }, [width, height]);
    await expect.poll(() => image.evaluate(img => img.complete && img.naturalWidth)).toBe(width);
    await cards.first().locator("[data-group-highlight]").click();
    await expectZoom(page, cards.first());
    await page.setViewportSize({ width: 320, height: 900 });
    await expectZoom(page, cards.first());
    await cards.first().locator("[data-group-highlight]").click();
    await expect(image).toHaveCSS("transform", "none");
    await page.setViewportSize({ width: 1440, height: 900 });
  }
  await image.evaluate((img, src) => { img.src = src; }, originalSource);
  await expect.poll(() => image.evaluate(img => img.complete && img.naturalWidth > 0 && img.naturalWidth === img.naturalHeight)).toBe(true);
  // Small faces amplify layout rounding errors, especially at fractional grid widths.
  const originalBounds = await cards.first().evaluate(card => ({ x: card.dataset.x, y: card.dataset.y, width: card.dataset.width, height: card.dataset.height }));
  await cards.first().evaluate(card => { Object.assign(card.dataset, { x: ".4", y: ".4", width: ".005", height: ".007" }); });
  await page.setViewportSize({ width: 1437, height: 900 });
  await cards.first().locator("[data-group-highlight]").click();
  await expectZoom(page, cards.first());
  await cards.first().locator("[data-group-highlight]").click();
  await cards.first().evaluate((card, bounds) => { Object.assign(card.dataset, bounds); }, originalBounds);
  await page.setViewportSize({ width: 1440, height: 900 });
  // Invalid and entirely out-of-photo regions cannot activate a zoom.
  const originalX = await cards.first().getAttribute("data-x");
  for (const x of ["NaN", "2"]) {
    await cards.first().evaluate((card, value) => { card.dataset.x = value; }, x);
    await cards.first().locator("[data-group-highlight]").click();
    await expect(image).toHaveCSS("transform", "none");
    await expect(cards.first().locator("[data-group-highlight]")).toHaveAttribute("aria-pressed", "false");
  }
  await cards.first().evaluate((card, value) => { card.dataset.x = value; }, originalX);
  await cards.first().locator("[data-group-highlight]").click();
  await expectZoom(page, cards.first());
  await page.evaluate(() => { window.groupOriginal = document.querySelector("[data-group-image]"); window.groupThumb = document.querySelector("[data-group-face] img"); });
  const modal = page.locator("[data-person-dialog]");
  const modalSource = page.locator("[data-group-face][data-person-dialog-source]");
  const repeatedPerson = cards.nth(2);
  const originalPersonID = await repeatedPerson.getAttribute("data-person-id");
  await repeatedPerson.evaluate((card, id) => { card.dataset.personId = id; }, await cards.first().getAttribute("data-person-id"));
  for (const closing of ["cancel", "escape"]) {
    await repeatedPerson.locator("[data-person-edit]").click();
    await expect(modal).toBeVisible();
    // Repeated detections share a person ID, but only the clicked thumbnail is marked.
    await expect(modalSource).toHaveCount(1);
    await expect(modalSource).toHaveAttribute("data-group-face", await repeatedPerson.getAttribute("data-group-face"));
    await expect(repeatedPerson.locator("[data-group-highlight]")).toHaveCSS("outline-style", "solid");
    expect(await repeatedPerson.locator("[data-group-highlight]").evaluate(preview => parseFloat(getComputedStyle(preview).outlineWidth))).toBeGreaterThan(0);
    await expectZoom(page, cards.first());
    if (closing === "cancel") await modal.getByRole("button", { name: "Abbrechen" }).click();
    else {
      const name = modal.getByRole("combobox", { name: "Name", exact: true });
      await expect(name).toHaveAttribute("aria-expanded", "true");
      await name.press("Escape");
      await expect(modal).toBeVisible();
      await expect(modalSource).toHaveCount(1);
      await name.press("Escape");
    }
    await expect(modal).not.toBeVisible();
    await expect(modalSource).toHaveCount(0);
    await expect(repeatedPerson.locator("[data-group-highlight]")).toHaveCSS("outline-style", "none");
  }
  await repeatedPerson.evaluate((card, id) => { card.dataset.personId = id; }, originalPersonID);
  await cards.first().locator("[data-person-edit]").click();
  await expect(modalSource).toHaveCount(1);
  await expect(modalSource).toHaveAttribute("data-group-face", await cards.first().getAttribute("data-group-face"));
  await modal.getByRole("combobox", { name: "Name", exact: true }).fill("Ada");
  await expect(modal.getByRole("option", { name: /Neu anlegen:.*Ada/ }).locator("img")).toHaveCount(0);
  await page.route("**/photos/people/*/rename", route => route.fulfill({ status: 503, body: "Unavailable" }));
  await modal.getByRole("option", { name: /Neu anlegen:.*Ada/ }).click();
  await expect(modal.locator("[data-person-dialog-status]")).toContainText("HTTP 503");
  await expect(modalSource).toHaveCount(1);
  await expect(modalSource).toHaveAttribute("data-group-face", await cards.first().getAttribute("data-group-face"));
  await page.unroute("**/photos/people/*/rename");
  await modal.getByRole("button", { name: "Benennen", exact: true }).click();
  await expect(modal).not.toBeVisible();
  await expect(modalSource).toHaveCount(0);
  await expect(surface).toHaveAttribute("data-path", "b.png");
  await expect(page.locator("[data-group-count]")).toContainText("5 unbearbeitete");
  await expect(cards.first().locator("[data-person-edit]")).toHaveCount(0);
  await expectZoom(page, cards.first());
  await expect(cards.first().locator("[data-group-highlight]")).toHaveAccessibleName("Gesicht im Foto vergrößern: Ada");
  expect(await page.evaluate(() => window.groupOriginal === document.querySelector("[data-group-image]") && window.groupThumb === document.querySelector("[data-group-face] img"))).toBe(true);
  const other = await (await context.request.get(baseURL + "/photos/people/groups?format=json&path=d.png")).json();
  expect(other.photo.faces[0].name).toBe("Ada");
  await cards.nth(1).locator("[data-person-edit]").click();
  await expect(modalSource).toHaveCount(1);
  await expect(modalSource).toHaveAttribute("data-group-face", await cards.nth(1).getAttribute("data-group-face"));
  await modal.getByRole("combobox", { name: "Name", exact: true }).fill("Ada");
  const adaOption = modal.getByRole("option", { name: /^Ada \(#/ });
  const adaThumbnail = adaOption.locator("img");
  await expect(adaThumbnail).toHaveAttribute("src", await cards.first().locator("[data-group-highlight] img").getAttribute("src"));
  await expect(adaThumbnail).toHaveAttribute("alt", "");
  await expect(adaThumbnail).toHaveAttribute("loading", "lazy");
  await expect.poll(() => adaThumbnail.evaluate(img => img.complete && img.naturalWidth > 0)).toBe(true);
  for (const width of [320, 1440]) {
    await page.setViewportSize({ width, height: 900 });
    expect(await adaThumbnail.evaluate(img => {
      const rect = img.getBoundingClientRect(), option = img.closest('[role="option"]').getBoundingClientRect();
      return Math.abs(rect.width - 40) < 1 && Math.abs(rect.height - 40) < 1 && rect.left >= option.left && rect.right <= option.right;
    })).toBe(true);
    expect(await modal.evaluate(dialog => dialog.scrollWidth <= dialog.clientWidth + 1)).toBe(true);
  }
  await page.screenshot({ path: "/tmp/bearstack-group-person-thumbnail.png", fullPage: true });
  // Clicking the image chooses the same person as clicking the name.
  await adaThumbnail.click();
  await expect(modal).not.toBeVisible();
  await expect(modalSource).toHaveCount(0);
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
  await expect(image).toHaveCSS("transform", "none");
  await expect(page.locator('[data-group-highlight][aria-pressed="true"]')).toHaveCount(0);
  const b = await (await context.request.get(baseURL + "/photos/people/groups?format=json&path=b.png")).json();
  expect(b.photo.faces.filter(face => face.ignored)).toHaveLength(4);
  expect(b.photo.faces.filter(face => face.name === "Ada" && !face.ignored)).toHaveLength(2);
  const d = await (await context.request.get(baseURL + "/photos/people/groups?format=json&path=d.png")).json();
  expect(d.photo.faces.filter(face => face.ignored)).toHaveLength(0);
  await expect.poll(() => image.evaluate(img => img.complete && img.naturalWidth > 0)).toBe(true);
  await cards.first().locator("[data-group-highlight]").click();
  await expectZoom(page, cards.first());
  await page.getByRole("link", { name: "Überspringen / nächstes Foto" }).click();
  await expect(page.locator("[data-group-empty]")).toBeVisible();
  await expect(image).toHaveCSS("transform", "none");
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
  await cards.first().locator("[data-group-highlight]").click();
  await expect(image).toHaveCSS("transform", "none");
  await expect(cards.first().locator("[data-group-highlight]")).toHaveAttribute("aria-pressed", "false");
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
