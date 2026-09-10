import { expect, test } from "@playwright/test";
import { startBearStack, stopBearStack, freePort } from "./server-fixture.mjs";
import http from "node:http";
import { mkdtemp, mkdir, writeFile, readFile, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

let root, app, service, baseURL;
let embeddingIndex = 0, inferenceCalls = 0, detectionsPerPhoto = 1;
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
  await writeFile(path.join(photos, "g.png"), png);
  await writeFile(path.join(photos, "h.png"), png);
  await writeFile(path.join(photos, "i.png"), png);
  await writeFile(path.join(photos, "j.png"), png);
  await writeFile(path.join(photos, "k.png"), png);
  await writeFile(path.join(photos, "l.png"), png);
  for (const photo of ["m.png", "n.png", "o.png"]) await writeFile(path.join(photos, photo), png);
  service = http.createServer((request, response) => {
    request.resume();
    request.on("end", () => {
      response.setHeader("Content-Type", "application/json");
      inferenceCalls++;
      const embedding = Array(128).fill(0); embedding[embeddingIndex] = 1;
      response.end(JSON.stringify({ model, faces: Array.from({length:detectionsPerPhoto},(_,i)=> { const vector=embedding.slice(); if(i){vector[embeddingIndex]=0;vector[embeddingIndex+i]=1;} return { x: detectionsPerPhoto===1 ? .3 : .1+i*.3, y: .05, width: detectionsPerPhoto===1 ? .3 : .2, height: .35, confidence: .99, embedding:vector }; }) }));
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
    const input = drawing.getByRole("combobox", { name: "Name", exact: true });
    await input.focus();
    await input.scrollIntoViewIfNeeded();
    const inputBox = await input.boundingBox();
    const fieldsBox = await drawing.locator(".face-drawing-fields").boundingBox();
    expect(inputBox.x - fieldsBox.x).toBeGreaterThanOrEqual(4);
    expect(fieldsBox.x + fieldsBox.width - inputBox.x - inputBox.width).toBeGreaterThanOrEqual(4);
    await drawing.locator(".face-drawing-fields").screenshot({ path: `/tmp/bearstack-face-drawing-input-${viewport.width}.png` });
    await input.blur();
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
  const erika = drawing.getByRole("option").filter({ hasText: "Erika (#" });
  await expect(erika.locator(".person-picker-thumbnail")).toBeVisible();
  await erika.scrollIntoViewIfNeeded();
  const optionBounds = await erika.boundingBox();
  // A scroll gesture starting on a suggestion must not save the draft.
  const touchX = optionBounds.x + optionBounds.width / 2, touchY = optionBounds.y + optionBounds.height / 2;
  await client.send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints: [{ x: touchX, y: touchY }] });
  await client.send("Input.dispatchTouchEvent", { type: "touchMove", touchPoints: [{ x: touchX, y: touchY + 45 }] });
  await client.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
  expect((await (await context.request.get(baseURL + "/photos/faces?path=f.png")).json()).photo.faces).toHaveLength(0);
  await expect(drawing).toBeVisible();
  await client.detach();
  await erika.tap();
  await expect(drawing).not.toBeVisible();
  await expect(lightbox.locator(".photo-info-face")).toContainText("Erika");
  const assigned = (await (await context.request.get(baseURL + "/photos/faces?path=f.png")).json()).photo.faces[0];
  expect(assigned.person_id).toBe(original.person_id);
  expect(inferenceCalls).toBe(beforeCalls);
  expect(errors).toEqual([]);
  await context.close();
});

test("drawing shows labeled existing regions and submits picker choices only with a valid draft", async ({ browser }) => {
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, hasTouch: true, httpCredentials: { username: "editor", password: "secret" } });
  try {
    const post = async (route, form) => {
      const response = await context.request.post(baseURL + route, { form, headers: { Origin: baseURL, Accept: "application/json" } });
      expect(response.ok(), await response.text()).toBe(true);
      return response.json();
    };
    detectionsPerPhoto = 3; embeddingIndex = 20;
    const detected = (await post("/photos/faces/analyze", { path: "i.png" })).photo.faces;
    const name = 'Zoë <img src=x onerror="alert(1)">';
    await post(`/photos/people/${detected[0].person_id}/rename`, { name });
    await post("/photos/faces/edit", { action: "ignore", face_id: String(detected[1].id) });
    const calls = inferenceCalls;
    const page = await context.newPage();
    const errors = []; page.on("pageerror", error => errors.push(error.message));
    let writes = 0, faceReads = 0;
    page.on("request", request => {
      const pathname = new URL(request.url()).pathname;
      if (pathname === "/photos/faces/manual" && request.method() === "POST") writes++;
      if (pathname === "/photos/faces") faceReads++;
    });
    await page.goto(baseURL + "/photos");
    await page.locator('[data-photo-path="i.png"] .photo-card-button').click();
    const lightbox = page.locator("[data-photo-lightbox]");
    await lightbox.locator("[data-photo-info-toggle]").press("Enter");
    await expect(lightbox.locator(".photo-info-face")).toHaveCount(2);
    const reads = faceReads;
    const draw = lightbox.getByRole("button", { name: "Gesicht einrahmen", exact: true });
    const drawing = page.locator("[data-face-drawing-dialog]");
    const boxes = drawing.locator(".photo-face-box");
    const input = drawing.getByRole("combobox", { name: "Name", exact: true });
    await draw.click();
    await expect(drawing.locator("[data-face-drawing-status]")).toHaveText("Ziehe einen Rahmen um das Gesicht.");
    expect(faceReads).toBe(reads);
    await expect(boxes).toHaveCount(3);
    await expect(boxes.locator("span")).toHaveText([name, "Unbenannt (ignoriert)", "Unbenannt"]);
    await expect(boxes.locator("img")).toHaveCount(0);
    await expect(boxes.nth(1)).toHaveAttribute("data-ignored", "");
    for (const viewport of [{ width: 390, height: 844 }, { width: 1440, height: 1000 }]) {
      await page.setViewportSize(viewport);
      // The photo is square and letterboxed differently on mobile and desktop.
      await expect.poll(async () => {
        const stage = await drawing.locator("[data-face-drawing-stage]").boundingBox();
        const side = Math.min(stage.width, stage.height);
        const left = stage.x + (stage.width - side) / 2;
        const top = stage.y + (stage.height - side) / 2;
        const actual = await boxes.first().boundingBox();
        return Math.max(Math.abs(actual.x - left - detected[0].x * side), Math.abs(actual.y - top - detected[0].y * side),
          Math.abs(actual.width - detected[0].width * side), Math.abs(actual.height - detected[0].height * side));
      }).toBeLessThan(2);
      await expect(boxes.first().locator("span")).toBeVisible();
      const labelLayout = await boxes.first().evaluate(box => {
        const label = box.querySelector("span"), rect = label.getBoundingClientRect();
        return {
          outside: rect.bottom < box.getBoundingClientRect().top,
          fontSize: parseFloat(getComputedStyle(label).fontSize),
          unclipped: label.scrollWidth <= label.clientWidth,
          widerThanBox: rect.width > box.getBoundingClientRect().width
        };
      });
      expect(labelLayout.outside).toBe(true);
      expect(labelLayout.fontSize).toBeLessThan(12);
      expect(labelLayout.unclipped).toBe(true);
      if (viewport.width < 900) expect(labelLayout.widerThanBox).toBe(true);
      await page.screenshot({ path: `/tmp/bearstack-face-drawing-labels-${viewport.width}.png`, fullPage: true });
    }
    // Choosing before drawing cannot create a region or close the dialog.
    await input.fill("Zoë");
    const zoe = drawing.getByRole("option").filter({ hasText: "Zoë <img" }).first();
    await expect(zoe.locator(".person-picker-thumbnail")).toBeVisible();
    await zoe.tap();
    await expect(drawing.locator("[data-face-drawing-status]")).toContainText("zuerst einen Rahmen");
    expect(writes).toBe(0);
    await drawing.getByRole("button", { name: "Rahmen mittig setzen" }).click();
    await input.fill("Klara");
    await expect(drawing.getByRole("option", { name: "Neu anlegen: „Klara“", exact: true })).toBeVisible();
    await page.route("**/photos/faces/manual", route => route.fulfill({ status: 409, json: { error: "Changed" } }), { times: 1 });
    await input.press("ArrowDown");
    await input.press("Enter");
    await expect(drawing.locator("[data-face-drawing-status]")).toContainText("wurde geändert");
    await expect(drawing).toBeVisible();
    expect(writes).toBe(1);
    // Selecting the create option by keyboard confirms and closes after success.
    await input.click();
    await expect(drawing.getByRole("option", { name: "Neu anlegen: „Klara“", exact: true })).toBeVisible();
    await input.press("ArrowDown");
    await input.press("Enter");
    await expect(drawing).not.toBeVisible();
    expect(writes).toBe(2);
    await expect(lightbox.locator(".photo-info-face").filter({ hasText: "Klara" })).toHaveCount(1);
    await draw.click();
    await expect(drawing.locator("[data-face-drawing-status]")).toHaveText("Ziehe einen Rahmen um das Gesicht.");
    await expect(boxes).toHaveCount(4);
    await expect(boxes.filter({ hasText: "Klara" })).toBeVisible();
    await drawing.getByRole("button", { name: "Abbrechen" }).click();
    await lightbox.locator("[data-photo-close]").press("Enter");
    await page.locator('[data-photo-path="j.png"] .photo-card-button').click();
    await lightbox.locator("[data-photo-info-toggle]").press("Enter");
    await draw.click();
    await expect(drawing.locator("[data-face-drawing-status]")).toHaveText("Ziehe einen Rahmen um das Gesicht.");
    await expect(boxes).toHaveCount(0);
    await expect(input).toHaveValue("");
    await expect(drawing.locator("[data-face-drawing-box]")).toBeHidden();
    expect(inferenceCalls).toBe(calls);
    expect(errors).toEqual([]);
  } finally { detectionsPerPhoto = 1; embeddingIndex = 0; await context.close(); }
});
test.afterAll(async ({}, info) => {
  info.setTimeout(75000);
  await stopBearStack(app);
  if (service) await new Promise(resolve => service.close(resolve));
  if (root) await rm(root, { recursive: true, force: true });
});

test("photo face overlay follows zoom and navigation and remembers the session toggle", async ({ browser }) => {
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, httpCredentials: { username: "editor", password: "secret" } });
  try {
    const post = async (route, form) => {
      const response = await context.request.post(baseURL + route, { form, headers: { Origin: baseURL, Accept: "application/json" } });
      expect(response.ok(), await response.text()).toBe(true);
      return response.json();
    };
    embeddingIndex = 30; detectionsPerPhoto = 3;
    const k = (await post("/photos/faces/analyze", { path: "k.png" })).photo;
    await post(`/photos/people/${k.faces[0].person_id}/rename`, { name: "Beate" });
    await post("/photos/faces/edit", { action: "ignore", face_id: String(k.faces[2].id) });
    embeddingIndex = 35; detectionsPerPhoto = 1;
    await post("/photos/faces/analyze", { path: "l.png" });
    const calls = inferenceCalls;
    const page = await context.newPage(), errors = [];
    page.on("pageerror", error => errors.push(error.message));
    let faceReads = 0;
    page.on("request", request => { if (new URL(request.url()).pathname === "/photos/faces") faceReads++; });
    const lightbox = page.locator("[data-photo-lightbox]");
    const overlay = lightbox.locator("[data-photo-face-overlay]");
    const boxes = overlay.locator(".photo-face-box");
    const toggle = lightbox.getByRole("button", { name: "Beschriftete Gesichtsrahmen anzeigen", exact: true });
    const open = async photo => {
      if (await lightbox.isVisible()) await lightbox.locator("[data-photo-close]").press("Enter");
      await page.locator(`[data-photo-path="${photo}"] .photo-card-button`).click();
      await expect.poll(() => lightbox.locator("[data-photo-image]").evaluate(img => img.complete && img.naturalWidth > 0)).toBe(true);
    };
    const aligned = async face => {
      await expect.poll(async () => {
        // The image's bounding rect includes its actual zoom and pan transform.
        const photo = await lightbox.locator("[data-photo-image]").boundingBox();
        const side = Math.min(photo.width, photo.height);
        const actual = await boxes.first().boundingBox();
        if (!actual) return Infinity;
        return Math.max(Math.abs(actual.x - photo.x - (photo.width - side) / 2 - face.x * side),
          Math.abs(actual.y - photo.y - (photo.height - side) / 2 - face.y * side), Math.abs(actual.width - face.width * side));
      }).toBeLessThan(2);
    };
    await page.goto(baseURL + "/photos");
    await open("k.png");
    await expect(overlay).toBeHidden();
    expect(faceReads).toBe(0);
    await lightbox.locator("[data-photo-info-toggle]").press("Enter");
    await expect(lightbox.locator(".photo-info-face")).toHaveCount(2);
    const loadedReads = faceReads;
    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-pressed", "true");
    await expect(boxes.locator("span")).toHaveText(["Beate", "Unbenannt", "Unbenannt (ignoriert)"]);
    expect(faceReads).toBe(loadedReads);
    await aligned(k.faces[0]);
    await lightbox.locator("[data-photo-info-close]").click();
    await expect(overlay).toBeVisible();
    await aligned(k.faces[0]);
    await lightbox.locator("[data-photo-zoom-in]").press("Enter");
    await expect(lightbox.locator("[data-photo-zoom-indicator]")).toHaveText("120%");
    await aligned(k.faces[0]);
    const stage = await lightbox.locator(".photo-lightbox-stage").boundingBox();
    await page.mouse.move(stage.x + stage.width / 2, stage.y + stage.height / 2);
    await page.mouse.down();
    await page.mouse.move(stage.x + stage.width / 2 + 35, stage.y + stage.height / 2 + 25, { steps: 4 });
    await page.mouse.up();
    await aligned(k.faces[0]);
    await lightbox.locator("[data-photo-zoom-reset]").press("Enter");
    await page.setViewportSize({ width: 390, height: 844 });
    await aligned(k.faces[0]);
    await page.screenshot({ path: "/tmp/bearstack-photo-face-overlay-mobile.png", fullPage: true });
    await page.setViewportSize({ width: 1440, height: 1000 });
    await lightbox.locator("[data-photo-info-toggle]").press("Enter");
    await lightbox.locator("[data-person-edit]").first().click();
    const naming = page.locator("[data-person-dialog]");
    await naming.getByRole("combobox", { name: "Name", exact: true }).fill("Beatrix");
    await naming.getByRole("button", { name: "Gesicht benennen", exact: true }).click();
    await expect(naming).not.toBeVisible();
    await expect(boxes.first()).toHaveText("Beatrix");
    await page.screenshot({ path: "/tmp/bearstack-photo-face-overlay-desktop.png", fullPage: true });
    // Reload and opening another image retain the toggle with the sidebar closed.
    await page.reload();
    await open("l.png");
    await expect(overlay).toBeVisible();
    await expect(boxes).toHaveCount(1);
    await expect(boxes.first()).toHaveText("Unbenannt");
    // A delayed face response must not be painted over the next photo.
    let release, requested = false;
    const pending = new Promise(resolve => { release = resolve; });
    await page.route("**/photos/faces?path=k.png", async route => {
      const response = await route.fetch(); requested = true;
      await pending; await route.fulfill({ response }).catch(() => {});
    });
    await open("k.png");
    await expect.poll(() => requested).toBe(true);
    await expect(overlay).toBeHidden();
    await open("j.png");
    release();
    await lightbox.locator("[data-photo-info-toggle]").press("Enter");
    await expect(lightbox.locator("[data-photo-face-status]")).toHaveText("Keine aktiven Gesichter gefunden.");
    await expect(overlay).toBeHidden();
    await expect(boxes).toHaveCount(0);
    await expect(toggle).toHaveAttribute("aria-pressed", "true");
    await page.unroute("**/photos/faces?path=k.png");
    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-pressed", "false");
    await page.reload();
    await open("k.png");
    await expect(overlay).toBeHidden();
    await lightbox.locator("[data-photo-info-toggle]").press("Enter");
    await expect(toggle).toHaveAttribute("aria-pressed", "false");
    // Storage restrictions leave the in-page session toggle fully usable.
    await page.addInitScript(() => Object.defineProperty(window, "sessionStorage", { get() { throw new DOMException("Blocked", "SecurityError"); } }));
    await page.reload();
    await open("k.png");
    await lightbox.locator("[data-photo-info-toggle]").press("Enter");
    await toggle.click();
    await expect(boxes).toHaveCount(3);
    await open("l.png");
    await expect(boxes).toHaveCount(1);
    expect(inferenceCalls).toBe(calls);
    expect(errors).toEqual([]);
  } finally { detectionsPerPhoto = 1; embeddingIndex = 0; await context.close(); }
});

test("renaming an already named face in photo info never changes its siblings", async ({ browser }) => {
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, httpCredentials: { username: "editor", password: "secret" } });
  try {
    const post = async (route, form) => {
      const response = await context.request.post(baseURL + route, { form, headers: { Origin: baseURL, Accept: "application/json" } });
      expect(response.ok(), await response.text()).toBe(true);
      return response.json();
    };
    const get = async photo => (await (await context.request.get(baseURL + "/photos/faces", { params: { path: photo } })).json()).photo;
    detectionsPerPhoto = 3; embeddingIndex = 40;
    for (const photo of ["m.png", "n.png", "o.png"]) await post("/photos/faces/analyze", { path: photo });
    const original = await get("m.png");
    const source = original.faces[0].person_id, target = original.faces[2].person_id;
    const selected = original.faces[1].id;
    await post(`/photos/people/${source}/rename`, { name: "Ursula" });
    await post(`/photos/people/${target}/rename`, { name: "Vera" });
    // Two detections of the same named person in this photo must remain distinct.
    await post("/photos/faces/edit", { action: "move", face_id: String(selected), target: String(source) });
    const untouched = new Map();
    for (const photo of ["m.png", "n.png", "o.png"]) {
      untouched.set(photo, (await get(photo)).faces.filter(face => face.id !== selected));
    }
    const unchangedSiblings = async () => {
      for (const [photo, faces] of untouched) {
        expect((await get(photo)).faces.filter(face => face.id !== selected)).toEqual(faces);
      }
    };
    const calls = inferenceCalls, page = await context.newPage(), errors = [], writes = [];
    page.on("pageerror", error => errors.push(error.message));
    page.on("request", request => {
      const path = new URL(request.url()).pathname;
      if (request.method() === "POST" && /^\/photos\/(faces|people)\//.test(path)) writes.push({ path, body: new URLSearchParams(request.postData()) });
    });
    await page.goto(baseURL + "/photos");
    await page.locator('[data-photo-path="m.png"] .photo-card-button').click();
    const lightbox = page.locator("[data-photo-lightbox]"), modal = page.locator("[data-person-dialog]");
    await lightbox.locator("[data-photo-info-toggle]").press("Enter");
    await lightbox.locator("[data-photo-face-toggle]").click();
    const input = modal.getByRole("combobox", { name: "Name", exact: true });
    const editSelected = () => lightbox.locator(`[data-face-id="${selected}"] [data-person-edit]`).click();
    await editSelected();
    await expect(modal.locator("#person-dialog-title")).toHaveText("Gesicht in diesem Foto benennen oder zuordnen");
    await expect(modal.locator("#overview-person-hint")).toContainText("Nur dieses Gesicht");
    await expect(modal.locator("[data-person-preview-image]")).toHaveAttribute("src", new RegExp(`/${selected}$`));
    await expect(input).toHaveValue("Ursula");
    // Keeping the name does not split off a duplicate person.
    await modal.getByRole("button", { name: "Gesicht benennen", exact: true }).click();
    await expect(modal).not.toBeVisible();
    expect((await get("m.png")).faces.find(face => face.id === selected).person_id).toBe(source);
    await unchangedSiblings();
    // Failure and retry keep the single-face scope; there is no group fallback.
    await editSelected();
    await input.fill("Ute");
    await page.route("**/photos/faces/edit", route => route.fulfill({ status: 503, json: { error: "Try again" } }), { times: 1 });
    await modal.getByRole("option", { name: "Neu anlegen: „Ute“", exact: true }).click();
    await expect(modal.locator("[data-person-dialog-status]")).toContainText("HTTP 503");
    await unchangedSiblings();
    await modal.getByRole("button", { name: "Gesicht benennen", exact: true }).click();
    await expect(modal).not.toBeVisible();
    const renamed = (await get("m.png")).faces.find(face => face.id === selected);
    expect(renamed.name).toBe("Ute");
    expect(renamed.person_id).not.toBe(source);
    await unchangedSiblings();
    await expect(lightbox.locator("[data-photo-face-overlay] .photo-face-box > span")).toHaveText(["Ursula", "Ute", "Vera"]);
    // Choosing an existing person by keyboard also moves only the clicked face.
    await editSelected();
    await input.fill("Vera");
    await expect(modal.getByRole("option", { name: /^Vera \(#/ })).toBeVisible();
    await input.press("ArrowDown"); await input.press("Enter");
    await expect(modal).not.toBeVisible();
    expect((await get("m.png")).faces.find(face => face.id === selected)).toMatchObject({ name: "Vera", person_id: target });
    await unchangedSiblings();
    expect(writes).toHaveLength(4);
    for (const write of writes) {
      expect(write.path).toBe("/photos/faces/edit");
      expect(write.body.getAll("face_id")).toEqual([String(selected)]);
      expect(write.body.get("action")).toBe("move");
    }
    // Opening an unnamed group afterwards resets the dialog's scope correctly.
    await lightbox.locator("[data-photo-close]").press("Enter");
    await page.locator('[data-photo-path="n.png"] .photo-card-button').click();
    await lightbox.locator("[data-photo-info-toggle]").press("Enter");
    const unnamed = (await get("n.png")).faces[1];
    await lightbox.locator(`[data-face-id="${unnamed.id}"] [data-person-edit]`).click();
    await expect(modal.locator("#overview-person-hint")).toContainText("gesamte Gruppe");
    await input.fill("Waltraud");
    await modal.getByRole("button", { name: "Benennen", exact: true }).click();
    await expect(modal).not.toBeVisible();
    expect(writes.at(-1).path).toBe(`/photos/people/${unnamed.person_id}/rename`);
    for (const photo of ["n.png", "o.png"]) {
      const faces = (await get(photo)).faces;
      expect(faces[0]).toMatchObject({ person_id: source, name: "Ursula" });
      expect(faces[1]).toMatchObject({ person_id: unnamed.person_id, name: "Waltraud" });
      expect(faces[2]).toMatchObject({ person_id: target, name: "Vera" });
    }
    expect(inferenceCalls).toBe(calls);
    expect(errors).toEqual([]);
  } finally { detectionsPerPhoto = 1; embeddingIndex = 0; await context.close(); }
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
    actions.getByRole("button", { name: "Gesicht einrahmen", exact: true }), actions.getByRole("button", { name: "Beschriftete Gesichtsrahmen anzeigen", exact: true }), actions.getByRole("button", { name: "Alle ignorierten Gesichter dieses Fotos wiederherstellen", exact:true }), refresh];
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
  await expect(readPage.locator("[data-photo-face-overlay]")).toHaveCount(0);
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

test("info bar unignores only this photo, preserving names and handling stale or lost responses", async ({browser}) => {
  const context=await browser.newContext({httpCredentials:{username:"editor",password:"secret"},viewport:{width:390,height:844}});
  try {
    detectionsPerPhoto=3;embeddingIndex=10;
    const post=async(route,form)=>{
      const response=await context.request.post(baseURL+route,{form,headers:{Origin:baseURL,Accept:"application/json"}});
      expect(response.ok()).toBe(true);return response.json();
    };
    const get=async(path)=>(await(await context.request.get(baseURL+"/photos/faces?path="+path)).json()).photo;
    await post("/photos/faces/analyze",{path:"g.png"});await post("/photos/faces/analyze",{path:"h.png"});
    const g=await get("g.png"),h=await get("h.png");
    await post(`/photos/people/${g.faces[0].person_id}/rename`,{name:"Anna"});
    const ignore=async(id)=>post("/photos/faces/edit",{action:"ignore",face_id:String(id)});
    await ignore(g.faces[0].id);await ignore(g.faces[1].id);await ignore(h.faces[0].id);
    const calls=inferenceCalls;
    const page=await context.newPage(),errors=[];page.on("pageerror",e=>errors.push(e.message));
    await page.goto(baseURL+"/photos");await page.locator('[data-photo-path="g.png"] .photo-card-button').click();
    const lightbox=page.locator("[data-photo-lightbox]");
    await lightbox.locator("[data-photo-info-toggle]").press("Enter");
    const restore=lightbox.getByRole("button",{name:"Alle ignorierten Gesichter dieses Fotos wiederherstellen",exact:true});
    const refresh=lightbox.getByRole("button",{name:"Gesichter aktualisieren",exact:true});
    await expect(restore).toBeEnabled();await expect(lightbox.locator(".photo-info-face")).toHaveCount(1);
    const originalImage=await lightbox.locator("[data-photo-image]").elementHandle();
    await lightbox.locator(".photo-face-actions").screenshot({path:"/tmp/bearstack-photo-unignore-button.png"});
    await restore.click();
    await expect(lightbox.locator("[data-photo-face-status]")).toHaveText("2 Gesichter wiederhergestellt.");
    await expect(lightbox.locator(".photo-info-face")).toHaveCount(3);await expect(restore).toBeDisabled();
    expect(await originalImage.evaluate(img=>img.isConnected)).toBe(true);
    expect((await get("g.png")).faces[0]).toMatchObject({ignored:false,name:"Anna",person_id:g.faces[0].person_id});
    expect((await get("h.png")).faces[0].ignored).toBe(true);
    // A late ignore cannot be silently included in the previous snapshot.
    await ignore(g.faces[0].id);await refresh.click();await expect(restore).toBeEnabled();
    await ignore(g.faces[1].id);
    await restore.click();await expect(lightbox.locator("[data-photo-face-status]")).toContainText("inzwischen geändert");
    await expect(restore).toBeDisabled();
    expect((await get("g.png")).faces.filter(f=>f.ignored)).toHaveLength(2);
    await refresh.click();await expect(restore).toBeEnabled();
    // The server committed, but the response was lost. Refresh never repeats POST.
    let writes=0;
    await page.route("**/photos/faces/unignore",async route=>{writes++;await route.fetch();await route.abort();});
    await restore.click();await expect(lightbox.locator("[data-photo-face-status]")).toContainText("zuerst die Gesichter aktualisieren");
    await expect(restore).toBeDisabled();await expect(refresh).toBeEnabled();
    await refresh.click();await expect(lightbox.locator(".photo-info-face")).toHaveCount(3);
    await expect(restore).toBeDisabled();expect(writes).toBe(1);
    await page.unroute("**/photos/faces/unignore");
    // Late responses belong to their original photo, never the next one.
    await ignore(g.faces[0].id);await refresh.click();await expect(restore).toBeEnabled();
    let release;const blocked=new Promise(resolve=>{release=resolve;});let posted=false;
    await page.route("**/photos/faces/unignore",async route=>{const response=await route.fetch();posted=true;await blocked;await route.fulfill({response}).catch(()=>{});});
    await restore.click();await expect.poll(()=>posted).toBe(true);
    await lightbox.getByRole("button",{name:"Foto schließen",exact:true}).press("Enter");
    await page.locator('[data-photo-path="h.png"] .photo-card-button').click();
    if(!await lightbox.locator("[data-photo-face-tools]").isVisible()) await lightbox.locator("[data-photo-info-toggle]").press("Enter");
    await expect(restore).toBeEnabled();release();
    await expect(lightbox.locator(".photo-info-face")).toHaveCount(2);
    expect((await get("h.png")).faces[0].ignored).toBe(true);expect(inferenceCalls).toBe(calls);
    expect(errors).toEqual([]);
  } finally {detectionsPerPhoto=1;await context.close();}
});
