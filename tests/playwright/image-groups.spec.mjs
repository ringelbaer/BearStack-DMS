import { expect, test } from "@playwright/test";
import { startBearStack, stopBearStack, freePort } from "./server-fixture.mjs";
import { mkdtemp, mkdir, writeFile, readFile, rm, stat } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

let root, baseURL, app;
const folder = "20240102_Family_Trip";
const paths = Array.from({ length: 8 }, (_, i) => `${folder}/${i + 1}.png`);
const original = await readFile(new URL("../../services/faces/tests/fixtures/astronaut.png", import.meta.url));

test.beforeAll(async () => {
  root = await mkdtemp(path.join(os.tmpdir(), "bearstack-image-groups-"));
  const photos = path.join(root, "photos"); await mkdir(path.join(photos, folder), { recursive: true });
  for (const name of paths) await writeFile(path.join(photos, name), original, { mode: 0o444 });
  const port = await freePort(); baseURL = `http://127.0.0.1:${port}`;
  const configPath = path.join(root, "config.json");
  await writeFile(configPath, JSON.stringify({ addr: `127.0.0.1:${port}`, data_dir: path.join(root, "data"), photos: { enabled: true, root_dir: photos }, auth: { credentials: [{ username: "editor", password: "secret", role: "photos_editor" }, { username: "reader", password: "secret", role: "photos_read" }] } }));
  app = await startBearStack({ configPath, baseURL }, { username: "editor", password: "secret" });
});

test("image groups fit narrow screens and readers can view without editing", async ({ browser }) => {
  const editor = await browser.newContext({ viewport: { width: 320, height: 780 }, hasTouch: true, isMobile: true });
  const reader = await browser.newContext();
  try {
    const page = await editor.newPage(); await login(page, "editor");
    await page.goto(`${baseURL}/photos?path=${encodeURIComponent(folder)}&sort=ascending_name`);
    const cards = page.locator(".photo-card"), mode = page.locator("[data-photo-mode-toggle]");
    await expect(cards).toHaveCount(8);
    await mode.click(); await page.locator("[data-photo-selection-mode]").click();
    await cards.first().locator(".photo-card-button").click();
    await mode.click();
    await expect(cards.locator("input:checked")).toHaveCount(0);
    await expect(page.locator("[data-photo-gallery]")).toHaveAttribute("data-selection-mode", "false");
    await expect(page.locator("[data-photo-selection-mode]")).toBeHidden();
    await mode.click();
    await cards.nth(6).locator('input[name="ids"]').check();
    await cards.nth(7).locator('input[name="ids"]').check();
    await page.locator("[data-image-group-create]").click();
    const dialog = page.locator("[data-image-group-dialog]");
    const box = await dialog.boundingBox(); expect(box.x).toBeGreaterThanOrEqual(0); expect(box.x + box.width).toBeLessThanOrEqual(320);
    await page.screenshot({ path: "/tmp/bearstack-image-group-dialog-mobile.png", fullPage: true, animations: "disabled" });
    await dialog.getByRole("button", { name: "Bildgruppe erstellen", exact: true }).click();
    await expect(page).toHaveURL(/\/photos\/image-groups\/\d+/);
    const groupURL = page.url();
    await mode.click();
    expect(Math.round((await mode.boundingBox()).width)).toBe(78);
    await expect(page.getByRole("button", { name: "Als Hauptbild verwenden", exact: true })).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.screenshot({ path: "/tmp/bearstack-image-group-mobile.png", fullPage: true, animations: "disabled" });
    const readPage = await reader.newPage(); await login(readPage, "reader"); await readPage.goto(groupURL);
    await expect(readPage.locator(".image-group-card")).toHaveCount(2);
    await expect(readPage.locator("[data-photo-mode-toggle]")).toHaveCount(0);
    await expect(readPage.getByRole("button", { name: "Als Hauptbild verwenden", exact: true })).toHaveCount(0);
    await readPage.locator(".image-group-card .person-photo-button").first().click();
    await expect(readPage.locator("[data-photo-lightbox]")).toBeVisible();
    await readPage.keyboard.press("ArrowRight");
    await expect(readPage.locator("[data-photo-lightbox] [data-photo-title]")).toHaveText("8.png");
    const g = await (await editor.request.get(groupURL+"?format=json")).json();
    const response = await editor.request.post(groupURL, { form: { action: "dissolve", revision: String(g.revision) }, headers: { Origin: baseURL, Accept: "application/json" } });
    expect(response.status()).toBe(200);
  } finally { await editor.close(); await reader.close(); }
});
test.afterAll(async ({}, info) => { info.setTimeout(75000); await stopBearStack(app); if (root) await rm(root, { recursive: true, force: true }); });
async function login(page, name) {
  await page.goto(baseURL + "/login"); await page.getByLabel("Benutzername").fill(name); await page.locator('input[name="password"]').fill("secret"); await page.getByRole("button", { name: "Anmelden", exact: true }).click();
}

test("image groups preserve originals and use one primary in gallery, frame and slideshow", async ({ browser }) => {
  const context = await browser.newContext();
  try {
    const page = await context.newPage(); const errors = []; page.on("pageerror", e => errors.push(e.message));
    const before = await Promise.all(paths.map(p => stat(path.join(root, "photos", p))));
    await login(page, "editor");
    const gallery = `${baseURL}/photos?path=${encodeURIComponent(folder)}&sort=ascending_name`;
    await page.goto(gallery);
    const cards = page.locator(".photo-card");
    await expect(cards).toHaveCount(8);
    await expect(page.locator("[data-photo-selection-mode]")).toBeHidden();
    await expect(cards.first().locator('input[name="ids"]')).toBeHidden();
    await expect(page.locator("[data-image-group-create]")).toBeHidden();
    await page.locator("[data-photo-mode-toggle]").click();
    await expect(cards.first().locator('input[name="ids"]')).toBeVisible();
    await cards.first().locator(".photo-card-button").click();
    await expect(page.locator("[data-photo-lightbox]")).toBeVisible();
    await page.keyboard.press("Escape");
    await page.locator("[data-photo-selection-mode]").click();
    await cards.first().locator(".photo-card-button").click();
    await cards.nth(2).locator(".photo-card-button").click({ modifiers: ["Shift"] });
    await expect(cards.locator("input:checked")).toHaveCount(3);
    await expect(page.locator("[data-photo-lightbox]")).not.toBeVisible();
    await page.locator("[data-image-group-create]").click();
    const dialog = page.locator("[data-image-group-dialog]");
    await expect(dialog).toContainText("3 ausgewählte Bilder");
    await expect(dialog.locator("option").first()).toHaveText("Fotos / 02.01.2024 · Family Trip / 1.png");
    await dialog.getByLabel("Hauptbild", { exact: true }).selectOption(paths[1]);
    await dialog.getByRole("button", { name: "Bildgruppe erstellen", exact: true }).click();
    await expect(page).toHaveURL(/\/photos\/image-groups\/\d+/);
    const groupURL = page.url(), id = Number(new URL(groupURL).pathname.split("/").at(-1));
    await expect(page.locator(".image-group-card")).toHaveCount(3);
    await expect(page.locator(".image-group-card").filter({ has: page.locator(".image-group-primary") })).toContainText("2.png");
    await page.goto(gallery);
    await expect(cards).toHaveCount(6);
    await expect(cards.locator("[data-image-group-link]")).toHaveCount(1);
    await expect(cards.locator("[data-image-group-link]")).toHaveAttribute("href", `/photos/image-groups/${id}`);
    const frame = await (await context.request.get(baseURL + `/photos/frame/items?path=${encodeURIComponent(folder)}&sort=ascending_name`)).json();
    expect(frame.total).toBe(6); expect(frame.media.map(m => m.path)).toEqual(paths.filter(p => ![paths[0], paths[2]].includes(p)));
    await cards.first().locator(".photo-card-button").click();
    await page.mouse.move(100, 20);
    await expect(page.locator("[data-photo-image-group]")).toBeVisible();
    await expect(page.locator("[data-photo-lightbox] [data-photo-title]")).toHaveText("2.png");
    await page.locator("[data-photo-next]").click();
    await expect(page.locator("[data-photo-lightbox] [data-photo-title]")).toHaveText("4.png");
    await page.keyboard.press("Escape");
    await cards.locator("[data-image-group-link]").click();
    await expect(page).toHaveURL(groupURL);
    await expect(page.getByRole("button", { name: "Als Hauptbild verwenden", exact: true }).first()).toBeHidden();
    await page.locator("[data-photo-mode-toggle]").click();
    await page.screenshot({ path: "/tmp/bearstack-image-group-desktop.png", fullPage: true, animations: "disabled" });
    const snapshot = await (await context.request.get(groupURL + "?format=json")).json();
    await page.locator(".image-group-card").filter({ hasText: "3.png" }).getByRole("button", { name: "Als Hauptbild verwenden", exact: true }).click();
    await expect(page.locator(".image-group-card").filter({ has: page.locator(".image-group-primary") })).toContainText("3.png");
    const stale = await context.request.post(groupURL, { form: { action: "dissolve", revision: String(snapshot.revision) }, headers: { Origin: baseURL, Accept: "application/json" } });
    expect(stale.status()).toBe(409);
    await page.locator("[data-photo-mode-toggle]").click();
    await page.locator(".image-group-card").filter({ hasText: "1.png" }).getByRole("button", { name: "Dieses Bild aus Gruppe entfernen", exact: true }).click();
    await page.getByRole("dialog").filter({ hasText: "Bildgruppe ändern" }).getByRole("button", { name: /Bestätigen|OK|Fortfahren/ }).click();
    await expect(page.locator(".image-group-card")).toHaveCount(2);
    await page.locator("[data-photo-mode-toggle]").click();
    await page.getByRole("button", { name: "Gesamte Bildgruppe auflösen", exact: true }).click();
    await page.getByRole("dialog").filter({ hasText: "Bildgruppe ändern" }).getByRole("button", { name: /Bestätigen|OK|Fortfahren/ }).click();
    await expect(page).toHaveURL(/\/photos\?notice=/);
    await page.goto(gallery); await expect(cards).toHaveCount(8);
    await expect(cards.locator("[data-image-group-link]")).toHaveCount(0);
    for (let i = 0; i < paths.length; i++) {
      const name = path.join(root, "photos", paths[i]);
      expect(await readFile(name)).toEqual(original); const after = await stat(name);
      expect(after.mtimeMs).toBe(before[i].mtimeMs); expect(after.mode).toBe(before[i].mode);
    }
    expect(errors).toEqual([]);
  } finally { await context.close(); }
});
