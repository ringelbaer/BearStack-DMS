import { expect, test } from "@playwright/test";
import { startBearStack, stopBearStack, freePort } from "./server-fixture.mjs";
import http from "node:http";
import { mkdtemp, mkdir, writeFile, readFile, utimes, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

const model = "yunet-2023mar-sface-2021dec-v1";
const token = "bearstack-people-sort-test-token-00000";
let root, baseURL, app, service;
test.beforeAll(async ({ browser }) => {
  root = await mkdtemp(path.join(os.tmpdir(), "bearstack-people-sort-"));
  const photos = path.join(root, "photos");
  const png = await readFile(new URL("../../services/faces/tests/fixtures/astronaut.png", import.meta.url));
  for (const [folder, year] of [["A", 2020], ["B", 2024], ["C", 2022]]) {
    const file = path.join(photos, folder, "photo.png");
    await mkdir(path.dirname(file), { recursive: true }); await writeFile(file, png);
    await utimes(file, new Date(`${year}-01-01T00:00:00Z`), new Date(`${year}-01-01T00:00:00Z`));
  }
  let call = 0;
  service = http.createServer((request, response) => {
    if (request.headers.authorization !== "Bearer " + token) { response.writeHead(401); response.end(); return; }
    response.setHeader("Content-Type", "application/json");
    if (request.url === "/health") { response.end(JSON.stringify({ ready: true, protocol: 1, model })); return; }
    request.resume(); request.on("end", () => {
      const index = call++;
      const axes = index === 0 ? Array.from({ length: 64 }, (_, i) => i) : [index === 1 ? 0 : 64];
      const faces = axes.map((axis, i) => {
        const embedding = Array(128).fill(0); embedding[axis] = 1;
        return { x: .01 + .12 * (i % 8), y: .01 + .12 * Math.floor(i / 8), width: .07, height: .1, confidence: .99, embedding };
      });
      response.end(JSON.stringify({ model, faces }));
    });
  });
  await new Promise(resolve => service.listen(0, "127.0.0.1", resolve));
  const port = await freePort(); baseURL = `http://127.0.0.1:${port}`;
  const configPath = path.join(root, "config.json");
  await writeFile(configPath, JSON.stringify({ addr: `127.0.0.1:${port}`, data_dir: path.join(root, "data"), auth: { credentials: [{ username: "manager", password: "secret", role: "photos_manager" }] }, photos: { enabled: true, root_dir: photos, face_service_url: `http://127.0.0.1:${service.address().port}`, face_service_token: token } }));
  app = await startBearStack({ configPath, baseURL }, { username: "manager", password: "secret" });
  const context = await browser.newContext({ httpCredentials: { username: "manager", password: "secret" } });
  const enabled = await context.request.post(baseURL + "/settings/photos/faces", { form: { enabled: "1", delay_millis: "100" }, headers: { Origin: baseURL } });
  expect(enabled.ok()).toBe(true);
  await expect.poll(async () => (await (await context.request.get(baseURL + "/settings/photos/faces?format=json")).json()).status.done, { timeout: 20000 }).toBe(3);
  const page = await (await context.request.get(baseURL + "/photos/people?format=json&sort=count_desc")).json();
  expect(page.total_pages).toBe(2); expect(page.people[0].count).toBe(2);
  const renamed = await context.request.post(baseURL + `/photos/people/${page.people[0].id}/rename`, { form: { name: "Zoe" }, headers: { Origin: baseURL, Accept: "application/json" } });
  expect(renamed.ok()).toBe(true);
  await context.close();
});
test.afterAll(async ({}, info) => {
  info.setTimeout(75000); await stopBearStack(app);
  if (service) await new Promise(resolve => service.close(resolve));
  if (root) await rm(root, { recursive: true, force: true });
});
async function login(page) {
  await page.goto(baseURL + "/login?next=%2Fphotos%2Fpeople%3Fpage%3D1");
  await page.getByLabel("Benutzername").fill("manager");
  await page.locator('input[name="password"]').fill("secret");
  await page.getByRole("button", { name: "Anmelden" }).click();
}

test("sorting resets the page, persists through details and matches global server order", async ({ browser }) => {
  const context = await browser.newContext(); const page = await context.newPage();
  await login(page);
  await page.goto(baseURL + "/photos/people?page=2&filter=all&sort=name_asc");
  for (const sort of ["count_desc", "count_asc", "folder_desc", "folder_asc", "date_desc", "date_asc", "name_desc", "name_asc"]) {
    await page.getByRole("combobox", { name: "Sortieren", exact: true }).selectOption(sort);
    await expect(page).toHaveURL(new RegExp(`sort=${sort}`));
    await expect(page.locator(".people-pagination-current")).toHaveText("Seite 1 von 2");
    const result = await (await context.request.get(page.url() + "&format=json")).json();
    await expect.poll(() => page.locator("[data-person-id].person-overview-card").evaluateAll(cards => cards.map(card => Number(card.dataset.personId))))
      .toEqual(result.people.map(person => person.id));
    await expect(page.locator('.people-pagination a[rel="next"]')).toHaveAttribute("href", new RegExp(`sort=${sort}`));
  }
  await page.getByRole("combobox", { name: "Sortieren", exact: true }).selectOption("date_desc");
  await page.locator('.people-pagination a[rel="next"]').click();
  await expect(page.getByRole("combobox", { name: "Sortieren", exact: true })).toHaveValue("date_desc");
  await page.locator("a.person-card").first().click();
  await page.getByRole("link", { name: "Alle Personen", exact: true }).click();
  await expect(page).toHaveURL(/page=2.*sort=date_desc|sort=date_desc.*page=2/);
  await page.goto(baseURL + "/photos/people");
  await expect(page).toHaveURL(/page=2.*sort=date_desc|sort=date_desc.*page=2/);
  await page.getByRole("link", { name: "Alle Filter aufheben" }).click();
  await expect(page.getByRole("combobox", { name: "Sortieren", exact: true })).toHaveValue("name_asc");
  await expect(page.locator(".people-pagination-current")).toHaveText("Seite 1 von 2");
  for (const width of [320, 390, 640, 1440]) {
    await page.setViewportSize({ width, height: 800 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBeLessThanOrEqual(1);
    const filter = await page.locator("[data-people-filter]").boundingBox();
    expect(filter.y + filter.height).toBeLessThan(620);
  }
  await page.setViewportSize({ width: 390, height: 800 });
  await page.screenshot({ path: "/tmp/bearstack-people-sort-mobile.png", fullPage: true });
  await context.close();
});

test("sorting and pagination also work without JavaScript", async ({ browser }) => {
  const context = await browser.newContext({ javaScriptEnabled: false }); const page = await context.newPage();
  await login(page);
  await page.goto(baseURL + "/photos/people?page=2");
  await page.getByRole("combobox", { name: "Sortieren", exact: true }).selectOption("count_desc");
  await page.getByRole("button", { name: "Suchen", exact: true }).click();
  await expect(page.locator("a.person-card").first()).toContainText("Zoe");
  await expect(page.locator("a.person-card").first()).toContainText("2 Fotos");
  await page.locator('.people-pagination a[rel="next"]').click();
  await expect(page).toHaveURL(/sort=count_desc/);
  await expect(page.getByRole("combobox", { name: "Sortieren", exact: true })).toHaveValue("count_desc");
  await context.close();
});
