import { expect, test } from "@playwright/test";
import { startBearStack, stopBearStack, freePort } from "./server-fixture.mjs";
import http from "node:http";
import { mkdtemp, mkdir, writeFile, readFile, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

const model = "yunet-2023mar-sface-2021dec-v1";
const token = "bearstack-face-chain-test-token-000000";
const portrait = await readFile(new URL("../../services/faces/tests/fixtures/astronaut.png", import.meta.url));
let root, baseURL, app, service, calls = 0;
test.beforeAll(async () => {
  root = await mkdtemp(path.join(os.tmpdir(), "bearstack-face-chains-"));
  const photos = path.join(root, "photos");
  for (const name of ["A", "B", "C", "D", "Named"]) {
    await mkdir(path.join(photos, name), { recursive: true });
    await writeFile(path.join(photos, name, "portrait.png"), portrait);
  }
  service = http.createServer((request, response) => {
    if (request.headers.authorization !== `Bearer ${token}`) { response.writeHead(401).end(); return; }
    response.setHeader("Content-Type", "application/json");
    if (request.url === "/health") { response.end(JSON.stringify({ ready: true, protocol: 1, model })); return; }
    request.resume();
    request.on("end", () => {
      const i = calls++, axis = Math.floor(i / 2) * 2, embedding = Array(128).fill(0);
      if (i >= 5) { response.writeHead(503).end(); return; }
      embedding[axis] = i % 2 ? .52 : 1;
      if (i % 2) embedding[axis + 1] = Math.sqrt(1 - .52 ** 2);
      response.end(JSON.stringify({ model, faces: [{ x: .3, y: .1, width: .3, height: .35, confidence: .99, embedding,
        quality: { face_pixels: 153.6, sharpness: 100, reference_eligible: true } }] }));
    });
  });
  await new Promise(resolve => service.listen(0, "127.0.0.1", resolve));
  const port = await freePort(); baseURL = `http://127.0.0.1:${port}`;
  const configPath = path.join(root, "config.json");
  await writeFile(configPath, JSON.stringify({ addr: `127.0.0.1:${port}`, data_dir: path.join(root, "data"),
    auth: { credentials: [{ username: "manager", password: "secret", role: "photos_manager" }] },
    photos: { enabled: true, root_dir: photos, face_service_url: `http://127.0.0.1:${service.address().port}`, face_service_token: token } }));
  app = await startBearStack({ configPath, baseURL }, { username: "manager", password: "secret" });
});
test.afterAll(async ({}, info) => {
  info.setTimeout(75_000);
  try { await stopBearStack(app); }
  finally {
    if (service?.listening) await new Promise(resolve => { service.close(resolve); service.closeAllConnections(); });
    if (root) await rm(root, { recursive: true, force: true });
  }
});
async function login(page) {
  await page.goto(baseURL + "/login");
  await page.getByLabel("Benutzername").fill("manager");
  await page.locator('input[name="password"]').fill("secret");
  await page.getByRole("button", { name: "Anmelden", exact: true }).click();
  await expect(page).not.toHaveURL(/\/login$/);
}

test("review, skip, restart, assign selected faces to existing and new names", async ({ browser }) => {
  test.setTimeout(90_000);
  const context = await browser.newContext({ httpCredentials: { username: "manager", password: "secret" } });
  try {
    const page = await context.newPage(), errors = [];
    page.on("pageerror", error => errors.push(error.message));
    await login(page);
    const settings = baseURL + "/settings/photos/faces";
    await page.goto(settings);
    await page.locator('select[name="reconcile_enabled"]').selectOption("0");
    await page.getByRole("checkbox", { name: /^Gesichtserkennung aktivieren/ }).check();
    await page.getByRole("spinbutton", { name: /^Pause zwischen Bildern/ }).fill("100");
    await page.getByRole("button", { name: "Speichern", exact: true }).click();
    await expect.poll(async () => (await (await context.request.get(settings + "?format=json&progress=1")).json()).status.done, { timeout: 20_000 }).toBe(5);
    await page.reload();
    await page.getByRole("checkbox", { name: /^Gesichtserkennung aktivieren/ }).uncheck();
    await page.getByRole("button", { name: "Speichern", exact: true }).click();
    const people = (await (await context.request.get(baseURL + "/photos/people?format=json&filter=all")).json()).people.sort((a, b) => a.id - b.id);
    expect(people).toHaveLength(5);
    const rename = await context.request.post(`${baseURL}/photos/people/${people[4].id}/rename`, { form: { name: "Ada" }, headers: { Accept: "application/json", Origin: baseURL } });
    expect(rename.ok()).toBe(true);
    let searches = 0;
    page.on("request", request => { if (request.url().endsWith("/chains/search")) searches++; });
    await page.goto(baseURL + "/photos/people/merge-suggestions");
    expect(searches).toBe(0);
    await page.getByRole("link", { name: "Gesichtsketten prüfen", exact: true }).click();
    const cards = page.locator("[data-chain-grid] article"), boxes = cards.getByRole("checkbox");
    await expect(cards).toHaveCount(2);
    const similarity = page.getByRole("spinbutton", { name: "Mindestähnlichkeit", exact: true });
    await expect(similarity).toHaveValue("0.45");
    await similarity.fill("0.8");
    await page.getByRole("button", { name: "Durchlauf neu starten" }).click();
    await expect(page.locator("[data-chain-status]")).toContainText("Keine weiteren Gesichtsketten");
    await expect(page.locator("[data-chain-content]")).toBeHidden();
    await similarity.fill("0.5");
    await page.getByRole("button", { name: "Durchlauf neu starten" }).click();
    await expect(cards).toHaveCount(2);
    await expect(page.locator("[data-chain-summary]")).toContainText("Mindestähnlichkeit 0,5");
    await expect(boxes.nth(0)).toBeChecked(); await expect(boxes.nth(1)).toBeChecked();
    expect(await cards.locator("strong").allTextContents()).toEqual(["A", "B"]);
    await page.getByRole("button", { name: "Kette überspringen", exact: true }).click();
    await expect(cards.locator("strong").first()).toHaveText("C");
    await page.getByRole("spinbutton", { name: "Maximale Sprünge" }).fill("1");
    const search = page.waitForRequest(request => request.url().endsWith("/chains/search"));
    await page.getByRole("button", { name: "Durchlauf neu starten" }).click();
    expect((await search).postDataJSON().hops).toBe(1);
    expect((await search).postDataJSON()).toMatchObject({ similarity: .5, after: 0, excluded_groups: [] });
    await expect(cards.locator("strong").first()).toHaveText("A");
    for (const width of [320, 390, 1440]) {
      await page.setViewportSize({ width, height: 900 });
      expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(1);
      const label = await cards.first().locator("label").boundingBox(), image = await cards.first().locator("img").boundingBox();
      expect(label.height).toBeGreaterThanOrEqual(44);
      expect(image.y).toBeGreaterThanOrEqual(label.y + label.height);
    }
    await page.screenshot({ path: test.info().outputPath("face-chains.png"), fullPage: true });
    await boxes.first().uncheck();
    await expect(page.locator("[data-chain-selected]")).toContainText("1 Gesichter");
    await page.getByRole("button", { name: "Auswahl zuordnen", exact: true }).click();
    const dialog = page.getByRole("dialog");
    await dialog.getByRole("combobox").fill("Ada");
    await dialog.getByRole("option", { name: /^Ada \(#/ }).click();
    await expect(dialog).not.toBeVisible();
    await expect(cards.locator("strong").first()).toHaveText("C");
    const ada = await (await context.request.get(`${baseURL}/photos/people/${people[4].id}?format=json`)).json();
    expect(ada.name).toBe("Ada"); expect(ada.faces).toHaveLength(2);
    const left = await (await context.request.get(`${baseURL}/photos/people/${people[0].id}?format=json`)).json();
    expect(left.name || "").toBe(""); expect(left.faces).toHaveLength(1);
    await page.getByRole("button", { name: "Auswahl zuordnen", exact: true }).click();
    await dialog.getByRole("combobox").fill("Neue Person");
    await dialog.getByRole("option", { name: /Neu anlegen/ }).click();
    await expect(dialog).not.toBeVisible();
    await expect(page.locator("[data-chain-status]")).toContainText("Keine weiteren Gesichtsketten");
    const defaults = await context.request.post(baseURL + "/photos/people/chains/search", { data: { hops: 1 }, headers: { Origin: baseURL } });
    expect((await defaults.json()).similarity).toBe(.45);
    expect(calls).toBe(5); expect(errors).toEqual([]);
  } finally { await context.close(); }
});

test("cross-page exclusions persist and a lost write response is recovered after reload", async ({ browser }) => {
  const context = await browser.newContext({ httpCredentials: { username: "manager", password: "secret" } });
  try {
    const page = await context.newPage(), errors = [];
    page.on("pageerror", error => errors.push(error.message));
    await login(page);
    const dataset = "12345678901234567890123456789012", groups = [{ id: 1, revision: 1, depth: 0 }, { id: 2, revision: 1, depth: 1 }];
    let searchCount = 0, payload, receipt, assigned = false;
    await page.route("**/photos/people/chains/search", route => {
      searchCount++;
      const { similarity } = route.request().postDataJSON();
      expect(similarity).toBe(searchCount === 1 ? .45 : .6);
      return route.fulfill({ json: { dataset, groups: assigned ? [] : groups, after: assigned ? 2 : 1, has_more: !assigned, similarity } });
    });
    await page.route("**/photos/people/chains/faces", route => {
      const { page: number } = route.request().postDataJSON();
      const faces = Array.from({ length: number === 1 ? 60 : 4 }, (_, i) => ({ id: (number - 1) * 60 + i + 1, person_id: number,
        folder_name: "Ordner " + number, display_path: `Ordner ${number}/Foto ${i}.jpg`, x: .3, y: .1, width: .3, height: .35 }));
      return route.fulfill({ json: { faces, page: number, pages: 2, total: 64 } });
    });
    await page.route("**/photos/faces/*/thumbnail", route => route.fulfill({ body: portrait, contentType: "image/png" }));
    await page.route("**/photos/people/chains/assign", async route => {
      payload = JSON.parse(new URLSearchParams(route.request().postData()).get("payload"));
      receipt = { operation_id: payload.operation_id, action: "assign_chain", source_id: 1, target_id: 99, faces: 64 - payload.excluded_faces.length };
      assigned = true;
      await route.abort();
    });
    await page.route("**/api/photos/labeling/v1/actions/*", route => route.fulfill({ json: receipt }));
    await page.goto(baseURL + "/photos/people/chains");
    const boxes = page.locator("[data-chain-grid] input[type=checkbox]");
    await expect(boxes).toHaveCount(60);
    await page.getByRole("spinbutton", { name: "Mindestähnlichkeit", exact: true }).fill("0.6");
    await page.getByRole("spinbutton", { name: "Maximale Sprünge" }).fill("3");
    await page.getByRole("button", { name: "Durchlauf neu starten" }).click();
    await expect(page.locator("[data-chain-summary]")).toContainText("Mindestähnlichkeit 0,6");
    await expect(page.locator("[data-chain-selected]")).toContainText("64 Gesichter");
    await boxes.first().uncheck();
    await page.getByRole("button", { name: "Weiter", exact: true }).click();
    await expect(boxes).toHaveCount(4); await expect(boxes.last()).toBeChecked();
    await boxes.last().uncheck();
    await page.getByRole("button", { name: "Zurück", exact: true }).click();
    await expect(boxes).toHaveCount(60); await expect(boxes.first()).not.toBeChecked();
    await expect(page.locator("[data-chain-selected]")).toContainText("62 Gesichter");
    await page.getByRole("button", { name: "Seite abwählen", exact: true }).click();
    await expect(page.locator("[data-chain-selected]")).toContainText("3 Gesichter");
    await page.getByRole("button", { name: "Auswahl zuordnen", exact: true }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog.locator("[data-person-preview-path]")).toHaveText("Ordner 2/Foto 0.jpg");
    await dialog.getByRole("combobox").fill("Testperson");
    await dialog.getByRole("option", { name: /Neu anlegen/ }).click();
    await expect(page.locator("[data-chain-status]")).toContainText("Speicherung nicht bestätigt");
    expect(new Set(payload.excluded_faces)).toEqual(new Set([...Array.from({ length: 60 }, (_, i) => i + 1), 64]));
    await dialog.getByRole("button", { name: "Abbrechen", exact: true }).click();
    await expect(page.locator("[data-chain-skip]")).toBeDisabled();
    await page.reload();
    expect(searchCount).toBe(2);
    await expect(page.getByRole("spinbutton", { name: "Mindestähnlichkeit", exact: true })).toHaveValue("0.6");
    await expect(page.getByRole("spinbutton", { name: "Maximale Sprünge" })).toHaveValue("3");
    await page.getByRole("button", { name: "Speicherung prüfen", exact: true }).click();
    await expect(page.locator("[data-chain-status]")).toContainText("Keine weiteren Gesichtsketten");
    expect(searchCount).toBe(3); expect(errors).toEqual([]);
  } finally { await context.close(); }
});

test("conflicts require a fresh review and an uncommitted request retries the same operation", async ({ browser }) => {
  const context = await browser.newContext({ httpCredentials: { username: "manager", password: "secret" } });
  try {
    const page = await context.newPage();
    await login(page);
    let searches = 0, writes = 0, retryPayload, done = false;
    const dataset = "12345678901234567890123456789012";
    await page.route("**/photos/people/chains/search", route => {
      searches++;
      return route.fulfill({ json: { dataset, groups: done ? [] : [{ id: 1, revision: searches, depth: 0 }, { id: 2, revision: searches, depth: 1 }], after: 1, has_more: !done, similarity: .45 } });
    });
    await page.route("**/photos/people/chains/faces", route => route.fulfill({ json: { page: 1, pages: 1, total: 2,
      faces: [1, 2].map(id => ({ id, person_id: id, folder_name: "Ordner", display_path: "Ordner/Foto.jpg", x: .3, y: .1, width: .3, height: .35 })) } }));
    await page.route("**/photos/faces/*/thumbnail", route => route.fulfill({ body: portrait, contentType: "image/png" }));
    await page.route("**/photos/people/chains/assign", async route => {
      writes++;
      if (writes === 1) { await route.fulfill({ status: 409, json: { error: "Gruppen geändert", code: "conflict" } }); return; }
      if (writes === 2) {
        retryPayload = JSON.parse(new URLSearchParams(route.request().postData()).get("payload"));
        await route.abort(); return;
      }
      expect(route.request().postDataJSON()).toEqual(retryPayload);
      done = true;
      await route.fulfill({ json: { ok: true, receipt: { operation_id: retryPayload.operation_id, action: "assign_chain", source_id: 1, target_id: 99, faces: 2 } } });
    });
    await page.route("**/api/photos/labeling/v1/actions/*", route => route.fulfill({ status: 404, json: { error: "Not committed", code: "not_found" } }));
    await page.goto(baseURL + "/photos/people/chains");
    const dialog = page.getByRole("dialog");
    for (const attempt of [1, 2]) {
      await expect(page.locator("[data-chain-grid] article")).toHaveCount(2);
      await page.getByRole("button", { name: "Auswahl zuordnen", exact: true }).click();
      await dialog.getByRole("combobox").fill("Retryperson");
      await dialog.getByRole("option", { name: /Neu anlegen/ }).click();
      await expect(page.locator("[data-chain-status]")).toContainText(attempt === 1 ? "nicht gespeichert" : "Speicherung nicht bestätigt");
      await dialog.getByRole("button", { name: "Abbrechen", exact: true }).click();
      if (attempt === 1) await page.getByRole("button", { name: "Erneut versuchen", exact: true }).click();
    }
    expect(searches).toBe(2);
    await page.getByRole("button", { name: "Speicherung prüfen", exact: true }).click();
    await expect(page.locator("[data-chain-status]")).toContainText("Keine weiteren Gesichtsketten");
    expect(writes).toBe(3); expect(searches).toBe(3);
  } finally { await context.close(); }
});
