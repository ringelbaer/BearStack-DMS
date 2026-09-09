import { expect, test } from "@playwright/test";
import { startBearStack, stopBearStack, freePort } from "./server-fixture.mjs";
import http from "node:http";
import { mkdtemp, mkdir, writeFile, readFile, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

let root, app, service, baseURL;
let embeddingIndex = 0, inferenceCalls = 0;
const model = "yunet-2023mar-sface-2021dec-v1";
test.beforeAll(async () => {
  root = await mkdtemp(path.join(os.tmpdir(), "bearstack-photo-face-info-"));
  const photos = path.join(root, "photos");
  await mkdir(photos);
  const png = await readFile(new URL("../../services/faces/tests/fixtures/astronaut.png", import.meta.url));
  await writeFile(path.join(photos, "a.png"), png);
  await writeFile(path.join(photos, "b.png"), png);
  await writeFile(path.join(photos, "c.png"), png);
  await writeFile(path.join(photos, "d.png"), png);
  await writeFile(path.join(photos, "e.png"), png);
  await writeFile(path.join(photos, "f.png"), png);
  service = http.createServer((request, response) => {
    request.resume();
    request.on("end", () => {
      response.setHeader("Content-Type", "application/json");
      inferenceCalls++;
      const embedding = Array(128).fill(0); embedding[embeddingIndex] = 1;
      response.end(JSON.stringify({ model, faces: [{ x: .3, y: .05, width: .3, height: .35, confidence: .99, embedding }] }));
    });
  });
  await new Promise(resolve => service.listen(0, "127.0.0.1", resolve));
  const port = await freePort(); baseURL = `http://127.0.0.1:${port}`;
  const configPath = path.join(root, "config.json");
  await writeFile(configPath, JSON.stringify({ addr: `127.0.0.1:${port}`, data_dir: path.join(root, "data"), auth: { credentials: [{ username: "editor", password: "secret", role: "photos_editor" }, { username: "reader", password: "secret", role: "photos_read" }] }, photos: { enabled: true, root_dir: photos, face_service_url: `http://127.0.0.1:${service.address().port}`, face_service_token: "photo-face-info-test-token-000000000" } }));
  app = await startBearStack({ configPath, baseURL }, { username: "editor", password: "secret" });
});

test("draw and name missing faces with mouse, touch and keyboard without inference", async ({ browser }) => {
  const context = await browser.newContext({ viewport: { width: 390, height: 844 }, hasTouch: true, httpCredentials: { username: "editor", password: "secret" } });
  const page = await context.newPage();
  const errors = []; page.on("pageerror", error => errors.push(error.message));
  const beforeCalls = inferenceCalls;
  await page.goto(baseURL + "/login");
  await page.getByLabel("Benutzername").fill("editor");
  await page.locator('input[name="password"]').fill("secret");
  await page.getByRole("button", { name: "Anmelden", exact: true }).click();
  await page.goto(baseURL + "/photos");
  const lightbox = page.locator("[data-photo-lightbox]");
  const drawing = page.locator("[data-face-drawing-dialog]");
  const draw = lightbox.getByRole("button", { name: "Gesicht einrahmen", exact: true });
  await page.locator('[data-photo-path="e.png"] .photo-card-button').click();
  await lightbox.locator("[data-photo-info-toggle]").press("Enter");
  await draw.click();
  await expect(drawing.locator("[data-face-drawing-status]")).toHaveText("Ziehe einen Rahmen um das Gesicht.");
  await expect(drawing.getByRole("button", { name: "Gesicht speichern" })).toBeDisabled();
  for (const viewport of [{ width: 1440, height: 1000 }, { width: 390, height: 844 }]) {
    await page.setViewportSize(viewport);
    const dialogBox = await drawing.boundingBox();
    const imageBox = await drawing.locator("[data-face-drawing-stage]").boundingBox();
    const footerBox = await drawing.locator("footer").boundingBox();
    expect(dialogBox.width).toBeGreaterThanOrEqual(viewport.width - 32);
    expect(dialogBox.height).toBeGreaterThanOrEqual(viewport.height - 32);
    expect(imageBox.width).toBeGreaterThan(viewport.width * .65);
    expect(imageBox.height).toBeGreaterThan(viewport.height * .6);
    expect(footerBox.y + footerBox.height).toBeLessThanOrEqual(viewport.height);
    await page.screenshot({ path: `/tmp/bearstack-face-drawing-large-${viewport.width}.png`, fullPage: true });
  }
  // Cancelling a draft must not leave an unnamed group behind.
  await drawing.getByRole("button", { name: "Rahmen mittig setzen" }).click();
  await drawing.locator("[data-face-drawing-box]").press("ArrowLeft");
  await drawing.locator("[data-face-drawing-box]").press("Shift+ArrowDown");
  await drawing.getByRole("button", { name: "Abbrechen" }).click();
  expect((await (await context.request.get(baseURL + "/photos/faces?path=e.png")).json()).photo.faces).toHaveLength(0);
  await draw.click();
  await expect(drawing.locator("[data-face-drawing-status]")).toHaveText("Ziehe einen Rahmen um das Gesicht.");
  const stage = drawing.locator("[data-face-drawing-stage]");
  const bounds = await stage.boundingBox();
  // Reverse-direction dragging also normalizes to a positive rectangle.
  await page.mouse.move(bounds.x + bounds.width * .6, bounds.y + bounds.height * .6);
  await page.mouse.down();
  await page.mouse.move(bounds.x + bounds.width * .3, bounds.y + bounds.height * .3, { steps: 5 });
  await page.mouse.up();
  await drawing.getByRole("combobox", { name: "Name", exact: true }).fill("Erika");
  await page.screenshot({ path: "/tmp/bearstack-face-drawing.png", fullPage: true });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await drawing.getByRole("button", { name: "Gesicht speichern" }).click();
  await expect(drawing).not.toBeVisible();
  await expect(lightbox.locator(".photo-info-face")).toContainText("Erika");
  const original = (await (await context.request.get(baseURL + "/photos/faces?path=e.png")).json()).photo.faces[0];
  expect(original.drawn).toBe(true);
  expect(original.width).toBeGreaterThan(0);
  await lightbox.locator("[data-photo-close]").press("Enter");
  await page.locator('[data-photo-path="f.png"] .photo-card-button').click();
  await lightbox.locator("[data-photo-info-toggle]").press("Enter");
  await draw.click();
  await expect(drawing.locator("[data-face-drawing-status]")).toHaveText("Ziehe einen Rahmen um das Gesicht.");
  const touchBounds = await stage.boundingBox();
  const client = await context.newCDPSession(page);
  await client.send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints: [{ x: touchBounds.x + touchBounds.width * .3, y: touchBounds.y + touchBounds.height * .3 }] });
  await client.send("Input.dispatchTouchEvent", { type: "touchMove", touchPoints: [{ x: touchBounds.x + touchBounds.width * .6, y: touchBounds.y + touchBounds.height * .6 }] });
  await client.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
  await drawing.getByRole("combobox", { name: "Name", exact: true }).fill("Erika");
  await drawing.getByRole("option").filter({ hasText: "Erika" }).first().click();
  await drawing.getByRole("button", { name: "Gesicht speichern" }).click();
  await expect(drawing).not.toBeVisible();
  await expect(lightbox.locator(".photo-info-face")).toContainText("Erika");
  const assigned = (await (await context.request.get(baseURL + "/photos/faces?path=f.png")).json()).photo.faces[0];
  expect(assigned.person_id).toBe(original.person_id);
  expect(inferenceCalls).toBe(beforeCalls);
  expect(errors).toEqual([]);
  await context.close();
});
test.afterAll(async ({}, info) => {
  info.setTimeout(75000);
  await stopBearStack(app);
  if (service) await new Promise(resolve => service.close(resolve));
  if (root) await rm(root, { recursive: true, force: true });
});

test("recognize one photo and name faces inside its info panel", async ({ browser }) => {
  const context = await browser.newContext({ viewport: { width: 390, height: 844 } });
  const page = await context.newPage();
  const errors = []; page.on("pageerror", error => errors.push(error.message));
  await page.goto(baseURL + "/login");
  await page.getByLabel("Benutzername").fill("editor");
  await page.locator('input[name="password"]').fill("secret");
  await page.getByRole("button", { name: "Anmelden", exact: true }).click();
  await page.goto(baseURL + "/photos");
  await page.locator('[data-photo-path="a.png"] .photo-card-button').click();
  const lightbox = page.locator("[data-photo-lightbox]");
  await lightbox.locator("[data-photo-info-toggle]").press("Enter");
  await expect(lightbox.locator("[data-photo-face-status]")).toHaveText("Keine aktiven Gesichter gefunden.");
  const analyze = lightbox.getByRole("button", { name: "Gesichter erkennen und zuordnen" });
  const refresh = lightbox.getByRole("button", { name: "Gesichter aktualisieren", exact: true });
  const actions = lightbox.getByRole("group", { name: "Gesichtsfunktionen" });
  const controls = [actions.getByRole("img", { name: "Gesichter", exact: true }), analyze,
    actions.getByRole("button", { name: "Gesicht einrahmen", exact: true }), refresh];
  for (const width of [320, 1024, 390]) {
    await page.setViewportSize({ width, height: 844 });
    await actions.scrollIntoViewIfNeeded();
    let previous;
    for (const [index, control] of controls.entries()) {
      const box = await control.boundingBox();
      expect(box.height).toBe(44);
      if (index > 0) {
        expect(box.width).toBe(44);
        expect(box.x).toBeGreaterThanOrEqual(previous.x + previous.width);
        expect(box.y).toBeCloseTo(previous.y, 0);
        await expect(control).toHaveAttribute("title", /.+/);
      }
      expect(box.x + box.width).toBeLessThanOrEqual(width);
      previous = box;
    }
  }
  await actions.screenshot({ path: "/tmp/bearstack-face-action-row.png" });
  const callsBeforeRefresh = inferenceCalls;
  await refresh.click();
  await expect(lightbox.locator("[data-photo-face-status]")).toHaveText("Keine aktiven Gesichter gefunden.");
  expect(inferenceCalls).toBe(callsBeforeRefresh);
  await page.route("**/photos/faces/analyze", route => route.fulfill({ status: 502, json: { error: "Dienst nicht erreichbar" } }), { times: 1 });
  await analyze.click();
  await expect(lightbox.locator("[data-photo-face-status]")).toHaveText("Dienst nicht erreichbar");
  await expect(analyze).toBeEnabled();
  await analyze.click();
  await expect(lightbox.locator(".photo-info-face")).toHaveCount(1);
  await lightbox.locator("[data-person-edit]").click();
  const modal = page.locator("[data-person-dialog]");
  await expect(modal).toBeVisible();
  await expect(modal.locator("[data-person-preview-path]")).toHaveText("Fotos / a.png");
  await modal.getByRole("combobox", { name: "Name", exact: true }).fill("Daria");
  await expect(lightbox).toHaveClass(/info-open/);
  await modal.getByRole("button", { name: "Benennen", exact: true }).click();
  await expect(modal).not.toBeVisible();
  await expect(lightbox.locator(".photo-info-face")).toContainText("Daria");
  await expect(lightbox.locator("[data-photo-info-people]")).toHaveText("Daria");
  await lightbox.locator("[data-person-edit]").click();
  await expect(modal.locator("[data-person-dialog-ignore]")).toBeDisabled();
  await modal.locator("[data-person-dialog-cancel]").click();
  await expect(lightbox).toBeVisible();
  await page.screenshot({ path: "/tmp/bearstack-photo-face-info.png", fullPage: true });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await lightbox.locator("[data-photo-close]").press("Enter");
  await page.locator('[data-photo-path="b.png"] .photo-card-button').click();
  await lightbox.locator("[data-photo-info-toggle]").press("Enter");
  await expect(lightbox.locator("[data-photo-face-status]")).toHaveText("Keine aktiven Gesichter gefunden.");
  await analyze.click();
  await expect(lightbox.locator(".photo-info-face")).toContainText("Daria");
  // Reanalysis preserves explicit naming; no global face worker is needed.
  await analyze.click();
  await expect(lightbox.locator("[data-photo-face-status]")).toBeEmpty();
  await expect(lightbox.locator(".photo-info-face")).toContainText("Daria");
  expect(errors).toEqual([]);
  await context.close();
  const reader = await browser.newContext({ httpCredentials: { username: "reader", password: "secret" } });
  const readPage = await reader.newPage();
  await readPage.goto(baseURL + "/photos");
  await expect(readPage.locator("[data-photo-face-tools]")).toHaveCount(0);
  await reader.close();
});

test("info-panel modal ignores only the selected face and handles stale revisions", async ({ browser }) => {
  embeddingIndex = 1;
  const context = await browser.newContext({ viewport: { width: 390, height: 844 }, httpCredentials: { username: "editor", password: "secret" } });
  const page = await context.newPage();
  let navigations = 0;
  page.on("framenavigated", frame => { if (frame === page.mainFrame()) navigations++; });
  const facesByPath = {};
  for (const photo of ["c.png", "d.png"]) {
    const response = await context.request.post(baseURL + "/photos/faces/analyze", { form: { path: photo }, headers: { Origin: baseURL } });
    expect(response.ok(), await response.text()).toBe(true);
    facesByPath[photo] = (await response.json()).photo.faces;
  }
  expect(facesByPath["c.png"][0].person_id).toBe(facesByPath["d.png"][0].person_id);
  await page.goto(baseURL + "/photos");
  await page.locator('[data-photo-path="c.png"] .photo-card-button').click();
  const lightbox = page.locator("[data-photo-lightbox]");
  await lightbox.locator("[data-photo-info-toggle]").press("Enter");
  await expect(lightbox.locator(".photo-info-face")).toContainText("Unbenannt");
  await lightbox.locator("[data-person-edit]").click();
  const modal = page.locator("[data-person-dialog]");
  const ignore = modal.locator("[data-person-dialog-ignore]");
  await expect(ignore).toBeEnabled();
  await page.route("**/photos/people/groups/ignore", route => route.fulfill({ status: 409, json: { error: "Changed", code: "conflict" } }), { times: 1 });
  await ignore.click();
  await expect(modal.locator("[data-person-dialog-status]")).toContainText("inzwischen geändert");
  await expect(ignore).toBeDisabled();
  await modal.locator("[data-person-dialog-cancel]").click();
  await lightbox.locator("[data-person-edit]").click();
  const beforeIgnore = navigations;
  await ignore.click();
  await expect(modal).not.toBeVisible();
  await expect(lightbox.locator(".photo-info-face")).toHaveCount(0);
  await expect(lightbox.locator("[data-photo-face-status]")).toHaveText("Gesicht ignoriert.");
  await expect(lightbox).toBeVisible();
  expect(navigations).toBe(beforeIgnore);
  for (const photo of ["c.png", "d.png"]) {
    const response = await context.request.get(baseURL + "/photos/faces", { params: { path: photo } });
    expect((await response.json()).photo.faces[0].ignored).toBe(photo === "c.png");
  }
  await context.close();
});
