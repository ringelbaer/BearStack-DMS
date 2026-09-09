import { expect, test } from "@playwright/test";
import { startBearStack, stopBearStack, freePort } from "./server-fixture.mjs";
import http from "node:http";
import { mkdtemp, mkdir, writeFile, readFile, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

let root, app, service, baseURL;
const model = "yunet-2023mar-sface-2021dec-v1";
test.beforeAll(async () => {
  root = await mkdtemp(path.join(os.tmpdir(), "bearstack-photo-face-info-"));
  const photos = path.join(root, "photos");
  await mkdir(photos);
  const png = await readFile(new URL("../../services/faces/tests/fixtures/astronaut.png", import.meta.url));
  await writeFile(path.join(photos, "a.png"), png);
  await writeFile(path.join(photos, "b.png"), png);
  service = http.createServer((request, response) => {
    request.resume();
    request.on("end", () => {
      response.setHeader("Content-Type", "application/json");
      const embedding = Array(128).fill(0); embedding[0] = 1;
      response.end(JSON.stringify({ model, faces: [{ x: .3, y: .05, width: .3, height: .35, confidence: .99, embedding }] }));
    });
  });
  await new Promise(resolve => service.listen(0, "127.0.0.1", resolve));
  const port = await freePort(); baseURL = `http://127.0.0.1:${port}`;
  const configPath = path.join(root, "config.json");
  await writeFile(configPath, JSON.stringify({ addr: `127.0.0.1:${port}`, data_dir: path.join(root, "data"), auth: { credentials: [{ username: "editor", password: "secret", role: "photos_editor" }, { username: "reader", password: "secret", role: "photos_read" }] }, photos: { enabled: true, root_dir: photos, face_service_url: `http://127.0.0.1:${service.address().port}`, face_service_token: "photo-face-info-test-token-000000000" } }));
  app = await startBearStack({ configPath, baseURL }, { username: "editor", password: "secret" });
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
  await expect(lightbox.locator("[data-photo-face-status]")).toContainText("1 Gesicht.");
  await expect(lightbox.locator(".photo-info-face")).toContainText("Daria");
  expect(errors).toEqual([]);
  await context.close();
  const reader = await browser.newContext({ httpCredentials: { username: "reader", password: "secret" } });
  const readPage = await reader.newPage();
  await readPage.goto(baseURL + "/photos");
  await expect(readPage.locator("[data-photo-face-tools]")).toHaveCount(0);
  await reader.close();
});
