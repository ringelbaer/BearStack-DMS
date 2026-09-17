import { expect, test } from "@playwright/test";
import { startBearStack, stopBearStack, freePort } from "./server-fixture.mjs";
import http from "node:http";
import { mkdtemp, mkdir, writeFile, readFile, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

const directory = "2011/20111015-Silberhochzeit-Onkel-Hermann";
const paths = [directory + "/a.png", directory + "/Unterordner/b.png", directory + "-Nachbar/c.png"];
const action = "Alle ignorierten Gesichter zurücksetzen";
const endpoint = "/photos/faces/reset-ignored-directory";
const model = "yunet-2023mar-sface-2021dec-v1";
let root, app, service, baseURL, calls = 0;

test.beforeAll(async () => {
  root = await mkdtemp(path.join(os.tmpdir(), "bearstack-folder-reset-"));
  const photos = path.join(root, "photos");
  const png = await readFile(new URL("../../services/faces/tests/fixtures/astronaut.png", import.meta.url));
  for (const file of paths) {
    await mkdir(path.dirname(path.join(photos, file)), { recursive: true });
    await writeFile(path.join(photos, file), png);
  }
  service = http.createServer((request, response) => {
    request.resume(); request.on("end", () => {
      const axis = calls++ * 2;
      const faces = [0, 1].map(i => {
        const embedding = Array(128).fill(0); embedding[axis + i] = 1;
        return { x: .1 + i * .4, y: .1, width: .2, height: .2, confidence: .99, embedding };
      });
      response.setHeader("Content-Type", "application/json");
      response.end(JSON.stringify({ model, faces }));
    });
  });
  await new Promise(resolve => service.listen(0, "127.0.0.1", resolve));
  const port = await freePort(); baseURL = `http://127.0.0.1:${port}`;
  const configPath = path.join(root, "config.json");
  await writeFile(configPath, JSON.stringify({ addr: `127.0.0.1:${port}`, data_dir: path.join(root, "data"), auth: { credentials: [{ username: "editor", password: "secret", role: "photos_editor" }, { username: "reader", password: "secret", role: "photos_read" }] }, photos: { enabled: true, root_dir: photos, face_service_url: `http://127.0.0.1:${service.address().port}`, face_service_token: "folder-reset-test-service-token-000000" } }));
  app = await startBearStack({ configPath, baseURL }, { username: "editor", password: "secret" });
});

test.afterAll(async ({}, info) => {
  info.setTimeout(75_000);
  await stopBearStack(app);
  if (service) await new Promise(resolve => service.close(resolve));
  if (root) await rm(root, { recursive: true, force: true });
});

test("folder action confirms its recursive scope, keeps named faces and excludes neighbours", async ({ browser }) => {
  const context = await browser.newContext({ httpCredentials: { username: "editor", password: "secret" } });
  try {
    const page = await context.newPage(), errors = [];
    page.on("pageerror", error => errors.push(error.message));
    await page.goto(baseURL + "/login");
    await page.getByLabel("Benutzername").fill("editor");
    await page.locator('input[name="password"]').fill("secret");
    await page.getByRole("button", { name: "Anmelden", exact: true }).click();
    const post = async (route, form) => {
      const response = await context.request.post(baseURL + route, { form, headers: { Origin: baseURL, Accept: "application/json" } });
      expect(response.ok()).toBe(true); return response.json();
    };
    const faces = [];
    for (const file of paths) faces.push((await post("/photos/faces/analyze", { path: file })).photo.faces);
    const named = faces[0][1];
    await post(`/photos/people/${named.person_id}/rename`, { name: "Anna" });
    await post(`/photos/people/${named.person_id}/tags`, { tags: "familie" });
    for (const group of faces) await post("/photos/faces/edit", { action: "ignore", face_id: String(group[0].id) });
    for (const folder of ["", "2011", ".people/all"]) {
      await page.goto(baseURL + "/photos?path=" + encodeURIComponent(folder));
      await expect(page.getByRole("button", { name: action })).toHaveCount(0);
    }
    await page.goto(baseURL + "/photos?path=" + encodeURIComponent(directory) + "&type=video");
    const menu = page.locator(".photo-actions-menu");
    for (const width of [320, 390, 1440]) {
      await page.setViewportSize({ width, height: 900 });
      await menu.locator("summary").click();
      await expect(menu.getByRole("button", { name: action })).toBeVisible();
      const bounds = await menu.locator("nav").boundingBox();
      expect(bounds.x).toBeGreaterThanOrEqual(0);
      expect(bounds.x + bounds.width).toBeLessThanOrEqual(width);
      await page.screenshot({ path: `/tmp/bearstack-folder-reset-${width}.png`, fullPage: true });
      await menu.locator("summary").click();
    }
    let writes = 0;
    page.on("request", r => { if (r.method() === "POST" && r.url().endsWith(endpoint)) writes++; });
    await menu.locator("summary").click();
    await menu.getByRole("button", { name: action }).click();
    const dialog = page.locator("[data-app-dialog]");
    await expect(dialog).toContainText(directory);
    await expect(dialog).toContainText("Unterordnern");
    await expect(dialog).toContainText("Benannte Gesichter bleiben unverändert");
    await dialog.getByRole("button", { name: "Abbrechen", exact: true }).click();
    expect(writes).toBe(0);
    await menu.locator("summary").click();
    await menu.getByRole("button", { name: action }).click();
    await dialog.getByRole("button", { name: "Bestätigen", exact: true }).click();
    await expect(page).toHaveURL(/notice=/);
    await expect(page.locator("body")).toContainText("2 ignorierte Gesichter im Ordner und seinen Unterordnern zurückgesetzt");
    expect(writes).toBe(1);
    for (let i = 0; i < paths.length; i++) {
      const photo = (await (await context.request.get(baseURL + "/photos/faces?path=" + encodeURIComponent(paths[i]))).json()).photo;
      expect(photo.faces[0].ignored).toBe(i === 2);
      expect(photo.faces[0].name).toBe("");
    }
    const person = await (await context.request.get(baseURL + `/photos/people/${named.person_id}?format=json`)).json();
    expect(person.name).toBe("Anna"); expect(person.tags).toContain("familie");
    expect(calls).toBe(3);
    expect(errors).toEqual([]);
  } finally { await context.close(); }
});

test("readers cannot see or call the folder reset", async ({ browser }) => {
  const context = await browser.newContext({ httpCredentials: { username: "reader", password: "secret" } });
  try {
    const page = await context.newPage();
    await page.goto(baseURL + "/login");
    await page.getByLabel("Benutzername").fill("reader");
    await page.locator('input[name="password"]').fill("secret");
    await page.getByRole("button", { name: "Anmelden", exact: true }).click();
    await page.goto(baseURL + "/photos?path=" + encodeURIComponent(directory));
    await page.locator(".photo-actions-menu summary").click();
    await expect(page.getByRole("button", { name: action })).toHaveCount(0);
    const response = await context.request.post(baseURL + endpoint, { form: { path: directory }, headers: { Origin: baseURL, Accept: "application/json" } });
    expect(response.status()).toBe(403);
  } finally { await context.close(); }
});
