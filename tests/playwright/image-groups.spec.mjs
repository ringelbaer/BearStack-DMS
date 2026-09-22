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
    await expect.poll(() => dialog.locator("[data-image-group-preview] img").evaluate(img => img.complete && img.naturalWidth > 0)).toBe(true);
    await expect(dialog.getByRole("button", { name: "Bildgruppe erstellen", exact: true })).toBeInViewport();
    await page.screenshot({ path: "/tmp/bearstack-image-group-dialog-mobile.png", animations: "disabled" });
    await dialog.getByRole("button", { name: "Bildgruppe erstellen", exact: true }).click();
    await expect.poll(() => new URL(page.url()).pathname).toBe("/photos");
    await expect(cards).toHaveCount(7);
    await cards.locator("[data-image-group-link]").click();
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

async function createGroup(request, members, primary = members[0]) {
  const body = new URLSearchParams({ primary }); members.forEach(p => body.append("ids", p));
  const response = await request.post(baseURL + "/photos/image-groups", { data: body.toString(), headers: { "Content-Type": "application/x-www-form-urlencoded", Accept: "application/json", Origin: baseURL } });
  expect(response.status()).toBe(201);
  return (await response.json()).url;
}
async function dissolveGroup(request, url) {
  const group = await (await request.get(baseURL + url + "?format=json")).json();
  const response = await request.post(baseURL + url, { form: { action: "dissolve", revision: String(group.revision) }, headers: { Origin: baseURL, Accept: "application/json" } });
  expect(response.status()).toBe(200);
  return (await response.json()).url;
}

test("selection keeps group badges and adds images to exactly one existing group", async ({ browser }) => {
  const context = await browser.newContext();
  try {
    const page = await context.newPage(); await login(page, "editor");
    const groupURL = await createGroup(context.request, paths.slice(0, 2));
    const otherURL = await createGroup(context.request, paths.slice(4, 6));
    const before = await (await context.request.get(baseURL + groupURL + "?format=json")).json();
    const gallery = `${baseURL}/photos?path=${encodeURIComponent(folder)}&sort=ascending_name&type=image`;
    await page.goto(gallery); await page.locator("[data-photo-mode-toggle]").click();
    await page.locator("[data-photo-selection-mode]").click();
    const card = p => page.locator(".photo-card").filter({ has: page.locator(`input[value="${p}"]`) });
    await expect(page.locator("[data-image-group-link]")).toHaveCount(2);
    for (const badge of await page.locator("[data-image-group-link]").all()) await expect(badge).toBeVisible();
    await card(paths[0]).locator(".photo-card-button").click();
    await card(paths[4]).locator(".photo-card-button").click();
    await card(paths[2]).locator(".photo-card-button").click();
    await expect(page.locator("[data-image-group-create]")).toBeDisabled();
    await expect(page.locator("[data-image-group-hint]")).toContainText("höchstens eine Bildgruppe");
    await card(paths[4]).locator(".photo-card-button").click();
    await expect(card(paths[0]).locator("[data-image-group-link]")).toBeVisible();
    await page.getByRole("button", { name: "Ausgewählte Bilder zur Bildgruppe hinzufügen …", exact: true }).click();
    const dialog = page.locator("[data-image-group-dialog]");
    await expect(dialog).toContainText("Bisheriges Hauptbild bleibt erhalten");
    await expect(dialog.locator(".image-group-choice")).toHaveCount(1);
    await expect(dialog.getByRole("radio")).toHaveCount(0);
    await expect(dialog.locator("input[name=ids]")).toHaveValue(paths[2]);
    await expect(dialog.locator("[data-image-group-preview-path]")).toContainText("1.png");
    const submit = dialog.getByRole("button", { name: "Zur Bildgruppe hinzufügen", exact: true });
    await expect(submit).toBeEnabled(); await submit.click();
    await expect(page).toHaveURL(new RegExp(groupURL + "\\?notice="));
    await expect(page.locator(".image-group-card")).toHaveCount(3);
    const after = await (await context.request.get(baseURL + groupURL + "?format=json")).json();
    expect(after.members.find(m => m.primary).entity_id).toBe(before.members.find(m => m.primary).entity_id);
    expect(after.members.map(m => m.path)).toEqual(paths.slice(0, 3));
    await page.goto(gallery); await expect(page.locator(".photo-card")).toHaveCount(5);
    await expect(card(paths[2])).toHaveCount(0);
    const destination = await dissolveGroup(context.request, groupURL);
    expect(new URL(destination, baseURL).searchParams.get("path")).toBe(folder);
    await dissolveGroup(context.request, otherURL);
  } finally { await context.close(); }
});

test("visual picker supports keyboard and cancel; adding waits for a valid group revision", async ({ browser }) => {
  const context = await browser.newContext();
  try {
    const page = await context.newPage(); await login(page, "editor");
    const gallery = `${baseURL}/photos?path=${encodeURIComponent(folder)}&sort=ascending_name`;
    await page.goto(gallery); await page.locator("[data-photo-mode-toggle]").click();
    const cards = page.locator(".photo-card");
    await cards.nth(0).locator('input[name="ids"]').check(); await cards.nth(1).locator('input[name="ids"]').check();
    await page.locator("[data-image-group-create]").click();
    const dialog = page.locator("[data-image-group-dialog]");
    await expect(dialog.getByRole("radio").first()).toBeFocused();
    await page.keyboard.press("ArrowRight");
    await expect(dialog.getByRole("radio").nth(1)).toBeChecked();
    await expect(dialog.locator("[data-image-group-preview-path]")).toContainText("2.png");
    await page.keyboard.press("Escape");
    await expect(dialog).not.toBeVisible();
    await expect(page.locator("[data-image-group-create]")).toBeFocused();
    await expect(dialog.locator("img")).toHaveCount(0);
    const groupURL = await createGroup(context.request, paths.slice(0,2));
    await page.goto(gallery); await page.locator("[data-photo-mode-toggle]").click();
    await cards.first().locator('input[name="ids"]').check(); await cards.nth(1).locator('input[name="ids"]').check();
    await page.route("**" + groupURL + "?format=json", route => route.fulfill({ status: 503, body: "Unavailable" }));
    await page.locator("[data-image-group-create]").click();
    await expect(dialog.locator("[data-image-group-form-status]")).toContainText("konnte nicht geladen");
    await expect(dialog.locator("[data-image-group-submit]")).toBeDisabled();
    await dialog.getByRole("button", { name: "Abbrechen", exact: true }).click();
    await page.unroute("**" + groupURL + "?format=json");
    await page.locator("[data-image-group-create]").click();
    await expect(dialog.locator("[data-image-group-submit]")).toBeEnabled();
    await expect(dialog.locator("input[name=revision]")).toHaveCount(1);
    await dialog.getByRole("button", { name: "Abbrechen", exact: true }).click();
    const group = await (await context.request.get(baseURL + groupURL + "?format=json")).json(); expect(group.members).toHaveLength(2);
    await dissolveGroup(context.request, groupURL);
  } finally { await context.close(); }
});

test("image groups preserve originals and use one primary in gallery, frame and slideshow", async ({ browser }) => {
  const context = await browser.newContext();
  try {
    const page = await context.newPage(); const errors = []; page.on("pageerror", e => errors.push(e.message));
    const before = await Promise.all(paths.map(p => stat(path.join(root, "photos", p))));
    await login(page, "editor");
    const gallery = `${baseURL}/photos?path=${encodeURIComponent(folder)}&sort=ascending_name&type=image`;
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
    await expect(dialog.getByRole("radio")).toHaveCount(3);
    await expect(dialog.locator("select")).toHaveCount(0);
    await expect(dialog.getByRole("radio").first()).toHaveAccessibleName("Fotos / 02.01.2024 · Family Trip / 1.png");
    await expect.poll(() => dialog.locator("[data-image-group-preview] img").evaluate(img => img.complete && img.naturalWidth > 0)).toBe(true);
    await dialog.locator(".image-group-choice").nth(1).locator("img").click();
    await expect(dialog.getByRole("radio").nth(1)).toBeChecked();
    await expect(dialog.locator("[data-image-group-preview] img")).toHaveAttribute("alt", "Hauptbild: Fotos / 02.01.2024 · Family Trip / 2.png");
    await expect.poll(() => dialog.locator("[data-image-group-preview] img").evaluate(img => img.complete && img.naturalWidth > 0)).toBe(true);
    await page.screenshot({ path: "/tmp/bearstack-image-picker-desktop.png", animations: "disabled" });
    await dialog.getByRole("button", { name: "Bildgruppe erstellen", exact: true }).click();
    await expect.poll(() => new URL(page.url()).pathname).toBe("/photos");
    expect(new URL(page.url()).searchParams.get("path")).toBe(folder);
    expect(new URL(page.url()).searchParams.get("sort")).toBe("ascending_name");
    expect(new URL(page.url()).searchParams.get("type")).toBe("image");
    await expect(cards).toHaveCount(6);
    await expect(cards.locator("[data-image-group-link]")).toHaveCount(1);
    const groupURL = new URL(await cards.locator("[data-image-group-link]").getAttribute("href"), baseURL).href;
    await expect(cards.first()).toHaveAttribute("data-photo-path", paths[1]);
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
    await expect(page.locator(".image-group-card")).toHaveCount(3);
    await expect(page.locator(".image-group-card").filter({ has: page.locator(".image-group-primary") })).toContainText("2.png");
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
    await expect.poll(() => new URL(page.url()).searchParams.get("path")).toBe(folder);
    await expect(cards).toHaveCount(8);
    await expect(cards.locator("[data-image-group-link]")).toHaveCount(0);
    for (let i = 0; i < paths.length; i++) {
      const name = path.join(root, "photos", paths[i]);
      expect(await readFile(name)).toEqual(original); const after = await stat(name);
      expect(after.mtimeMs).toBe(before[i].mtimeMs); expect(after.mode).toBe(before[i].mode);
    }
    expect(errors).toEqual([]);
  } finally { await context.close(); }
});
