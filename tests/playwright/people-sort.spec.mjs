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
  await page.getByRole("link", { name: "← Alle Personen", exact: true }).click();
  await expect(page).toHaveURL(/page=2.*sort=date_desc|sort=date_desc.*page=2/);
  await page.goto(baseURL + "/photos/people");
  await expect(page).toHaveURL(/page=2.*sort=date_desc|sort=date_desc.*page=2/);
  await expect(page.getByRole("link", { name: "Alle Filter aufheben" })).toHaveCount(0);
  await page.getByRole("combobox", { name: "Sortieren", exact: true }).selectOption("name_asc");
  await expect(page.getByRole("combobox", { name: "Sortieren", exact: true })).toHaveValue("name_asc");
  await expect(page.locator(".people-pagination-current")).toHaveText("Seite 1 von 2");
  for (const width of [320, 390, 640, 1440]) {
    await page.setViewportSize({ width, height: 800 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBeLessThanOrEqual(1);
    const filter = await page.locator("[data-people-filter]").boundingBox();
    // Allow the filter group's additional border and padding on narrow screens.
    expect(filter.y + filter.height).toBeLessThan(630);
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

for (const javaScriptEnabled of [true, false]) {
  test(`person filters switch directly and discard irrelevant search, JavaScript=${javaScriptEnabled}`, async ({ browser }) => {
    const context = await browser.newContext({ javaScriptEnabled }); const page = await context.newPage();
    await login(page);
    await page.goto(baseURL + "/photos/people?filter=all&q=Zoe&sort=count_desc");
    await expect(page.locator("a.person-card")).toHaveCount(1);
    const filters = page.getByRole("navigation", { name: "Personenfilter", exact: true });
    for (const [name, value] of [["Unbenannt", "unknown"], ["Ignoriert", "ignored"], ["Benannt", "known"], ["Alle", "all"]]) {
      await filters.getByRole("link", { name, exact: true }).click();
      await expect(page).toHaveURL(new RegExp(`filter=${value}`));
      await expect(filters.locator('[aria-current="page"]')).toHaveText(name);
      await expect(page.getByRole("combobox", { name: "Sortieren", exact: true })).toHaveValue("count_desc");
      const hasSearch = value === "known" || value === "all";
      await expect(page.locator('[data-people-filter] input[name="q"]')).toHaveCount(hasSearch ? 1 : 0);
      await expect(page.getByRole("button", { name: "Suchen", exact: true })).toHaveCount(hasSearch ? 1 : 0);
      if (!hasSearch) expect(new URL(page.url()).searchParams.has("q")).toBe(false);
      if (value === "unknown") {
        await expect(page.locator("a.person-card")).toHaveCount(60);
        await expect(page.locator(".people-pagination-current")).toHaveText("Seite 1 von 2");
      }
      if (value === "known") await expect(page.locator("a.person-card")).toHaveCount(1);
    }
    await page.goto(baseURL + "/photos/people?page=2&filter=unknown&sort=date_desc");
    for (const width of [320, 390, 640, 1440]) {
      await page.setViewportSize({ width, height: 800 });
      const layout = await page.locator(".people-pagination").evaluate(nav => {
        const controls = [...nav.children].map(el => el.getBoundingClientRect());
        return { height: nav.getBoundingClientRect().height, tops: controls.map(rect => Math.round(rect.top)), widths: controls.filter((_, i) => i !== 2).map(rect => rect.width), overflow: document.documentElement.scrollWidth - innerWidth };
      });
      expect(layout.overflow).toBeLessThanOrEqual(1);
      if (width <= 640) {
        expect(layout.height).toBeLessThanOrEqual(48);
        expect(new Set(layout.tops).size).toBe(1);
        expect(layout.widths.every(width => width >= 44)).toBe(true);
      }
      await expect(page.getByRole("link", { name: "Erste Seite", exact: true })).toHaveAttribute("href", /sort=date_desc/);
    }
    if (javaScriptEnabled) {
      for (const theme of ["design2", "default"]) {
        await page.evaluate(theme => { document.documentElement.dataset.theme = theme; }, theme);
        const colors = await filters.evaluate(nav => {
          const active = getComputedStyle(nav.querySelector('[aria-current="page"]'));
          const other = getComputedStyle(nav.querySelector('a:not([aria-current])'));
          return { active: active.backgroundColor, other: other.backgroundColor };
        });
        expect(colors.active).not.toBe(colors.other);
      }
      await page.setViewportSize({ width: 390, height: 800 });
      await page.screenshot({ path: "/tmp/bearstack-people-filters-pagination.png", fullPage: true });
      await page.getByLabel("Anzeigeeinstellungen", { exact: true }).click();
      await page.locator(".people-display-options").screenshot({ path: "/tmp/bearstack-people-display-menu.png" });
      await page.keyboard.press("Escape");
    }
    await page.getByRole("link", { name: "Ignoriert", exact: true }).click();
    await page.getByRole("combobox", { name: "Sortieren", exact: true }).selectOption("folder_asc");
    if (!javaScriptEnabled) await page.getByRole("button", { name: "Sortieren", exact: true }).click();
    await expect(page).toHaveURL(/sort=folder_asc/);
    await expect(filters.locator('[aria-current="page"]')).toHaveText("Ignoriert");
    await context.close();
  });
}

test("favorite portraits appear in overview and naming choices without changing the search source", async ({ browser }) => {
  const context = await browser.newContext(); const page = await context.newPage();
  await login(page);
  const named = (await (await context.request.get(baseURL + "/photos/people?format=json&filter=known&q=Zoe")).json()).people[0];
  const detail = (await (await context.request.get(baseURL + `/photos/people/${named.id}?format=json`)).json()).faces;
  const favorite = detail.find(face => face.id !== named.face_id).id;
  const response = await context.request.post(baseURL + `/photos/faces/${favorite}/favorite`, {
    form: { person_id: String(named.id), favorite: "1" }, headers: { Origin: baseURL, Accept: "application/json" }
  });
  expect(response.ok()).toBe(true);
  await page.goto(baseURL + "/photos/people?filter=known&q=Zoe");
  const card = page.locator(`.person-overview-card[data-person-id="${named.id}"]`);
  await expect(card.locator("img")).toHaveAttribute("src", `/photos/faces/${favorite}/thumbnail`);
  await expect(card).toHaveAttribute("data-search-face-id", String(named.face_id));
  await card.locator("[data-person-select]").check();
  await page.locator("[data-people-edit-button]").click();
  const dialog = page.locator("[data-person-dialog]");
  const search = page.waitForRequest(request => request.url().endsWith(`/photos/faces/${named.face_id}/suggestions`));
  await dialog.getByRole("button", { name: "Ähnliche benannte Personen suchen" }).click(); await search;
  await dialog.getByRole("button", { name: "Abbrechen", exact: true }).click();
  await page.goto(baseURL + "/photos/people?filter=unknown");
  await page.locator(".person-overview-card [data-person-edit]").first().click();
  await dialog.getByRole("combobox").fill("Zoe");
  await expect(dialog.getByRole("option", { name: /^Zoe/ }).locator("img")).toHaveAttribute("src", `/photos/faces/${favorite}/thumbnail`);
  await dialog.getByRole("button", { name: "Abbrechen", exact: true }).click();
  await context.close();
});

for (const javaScriptEnabled of [true, false]) {
  test(`search reset clears the query and preserves named filter, JavaScript=${javaScriptEnabled}`, async ({ browser }) => {
    const context = await browser.newContext({ javaScriptEnabled }); const page = await context.newPage();
    await login(page);
    await page.goto(baseURL + "/photos/people?known=1&q=missing&page=2&sort=count_desc");
    await expect(page.locator("a.person-card")).toHaveCount(0);
    await page.getByRole("link", { name: "Personensuche zurücksetzen" }).click();
    await expect(page.locator('input[name="q"]')).toHaveValue("");
    await expect(page.locator('input[name="filter"]')).toHaveValue("known");
    await expect(page.locator("a.person-card")).toHaveCount(1);
    await expect(page.getByRole("combobox", { name: "Sortieren", exact: true })).toHaveValue("count_desc");
    await context.close();
  });
}

test("tag a person and browse the virtual folders into the dated photo gallery", async ({ browser }) => {
  const context = await browser.newContext(); const page = await context.newPage();
  async function checkPhotoHead() {
    for (const width of [320, 390, 480, 640, 1440]) {
      await page.setViewportSize({ width, height: 800 });
      const geometry = await page.evaluate(() => {
        const heading = document.querySelector(".photo-page > .page-head h1");
        const boxes = [...document.querySelectorAll(".photo-page > .page-head h1, .photo-page > .page-head .page-actions > *")].map(el => el.getBoundingClientRect()).filter(r => r.width && r.height);
        return { overflow: document.documentElement.scrollWidth - innerWidth,
          titleClipped: heading.scrollWidth - heading.clientWidth,
          overlaps: boxes.some((a,i) => boxes.slice(i+1).some(b => a.x < b.right && a.right > b.x && a.y < b.bottom && a.bottom > b.y)) };
      });
      expect(geometry.overflow).toBeLessThanOrEqual(1);
      expect(geometry.titleClipped).toBeLessThanOrEqual(1);
      expect(geometry.overlaps).toBe(false);
    }
  }
  await login(page);
  await page.goto(baseURL + "/photos/people?known=1&q=Zoe");
  await page.locator("a.person-card").click();
  await page.getByRole("button", { name: "Personen-Tags bearbeiten" }).click();
  const dialog = page.locator("[data-tag-select-modal]");
  await dialog.locator("[data-tag-select-search]").fill("familie");
  await dialog.getByRole("button", { name: "Tag anlegen", exact: true }).click();
  await dialog.getByRole("button", { name: "Übernehmen", exact: true }).click();
  await expect(dialog).not.toBeVisible();
  await expect(page.getByRole("button", { name: "Personen-Tags bearbeiten" })).toContainText("familie");
  await page.goto(baseURL + "/photos");
  await checkPhotoHead();
  const people = page.locator(".photo-folder-link").filter({ hasText: "Personen" }).first();
  await expect(people.locator("img")).toHaveCount(8);
  await people.click();
  await expect(page.getByRole("link", { name: /^Alle \d+ Personen/ })).toHaveCount(1);
  await page.locator(".photo-folder-link").filter({ hasText: "familie" }).click();
  await expect(page.locator(".photo-folder-link")).toHaveCount(1);
  await page.locator(".photo-folder-link").filter({ hasText: "Zoe" }).click();
  await expect(page.locator(".photo-card")).toHaveCount(2);
  await expect(page.locator(".photo-date-group")).toHaveCount(2);
  await page.locator("[data-photo-sort-menu] summary").click();
  await page.getByRole("link", { name: "Datum absteigend", exact: true }).click();
  await expect(page.locator(".photo-card").first()).toHaveAttribute("data-photo-path", "B/photo.png");
  await checkPhotoHead();
  await page.setViewportSize({ width: 390, height: 800 });
  await page.screenshot({ path: "/tmp/bearstack-people-gallery-mobile.png", fullPage: true });
  await context.close();
});

test("named groups can choose, follow and clear existing parents", async ({ browser }) => {
  const context = await browser.newContext(); const page = await context.newPage();
  await login(page);
  const groups = (await (await context.request.get(baseURL + "/photos/people?format=json")).json()).people;
  const child = (await (await context.request.get(baseURL + "/photos/people?format=json&known=1&q=Zoe")).json()).people[0];
  const parents = groups.filter(p => p.id !== child.id).slice(0,2);
  for (const [i,parent] of parents.entries()) {
    const response = await context.request.post(`${baseURL}/photos/people/${parent.id}/rename`, { form: {name: i === 0 ? "Mutter Beispiel" : "Vater Beispiel"}, headers: {Origin: baseURL, Accept: "application/json"} });
    expect(response.ok()).toBe(true);
  }
  await page.goto(`${baseURL}/photos/people/${child.id}`);
  await page.locator(".person-parents summary").click();
  await page.getByRole("combobox",{name:"Mutter",exact:true}).fill("Mutter Beispiel");
  await page.getByRole("option",{name:/^Mutter Beispiel/}).click();
  await page.getByRole("combobox",{name:"Vater",exact:true}).fill("Vater Beispiel");
  await page.getByRole("option",{name:/^Vater Beispiel/}).click();
  await page.getByRole("button",{name:"Eltern speichern",exact:true}).click();
  await expect(page.locator(".notice")).toContainText("Eltern gespeichert");
  await page.locator(".person-parents summary").click();
  await expect(page.locator(".person-parents").getByRole("link",{name:"Mutter Beispiel",exact:true})).toHaveAttribute("href",`/photos/people/${parents[0].id}`);
  await page.setViewportSize({width:390,height:800});
  const box = await page.locator(".person-parents-panel").boundingBox();
  expect(box.x).toBeGreaterThanOrEqual(0); expect(box.x+box.width).toBeLessThanOrEqual(390);
  await page.getByRole("combobox",{name:"Mutter",exact:true}).fill("");
  await page.getByRole("combobox",{name:"Vater",exact:true}).fill("");
  await page.getByRole("button",{name:"Eltern speichern",exact:true}).click();
  const result = await (await context.request.get(`${baseURL}/photos/people/${child.id}/parents`)).json();
  expect(result).toEqual({});
  await context.close();
});

test("folder people view keeps portraits, photos and navigation inside the folder", async ({ browser }) => {
  const context = await browser.newContext(); const page = await context.newPage();
  await login(page);
  await page.goto(baseURL + "/photos?path=B");
  await page.locator(".photo-actions-menu > summary").click();
  await page.getByRole("link",{name:"Personen im Ordner",exact:true}).click();
  await expect(page.locator(".photo-folder-link")).toHaveCount(1);
  await expect(page.locator(".photo-folder-link")).toContainText("Zoe");
  await page.locator(".photo-folder-link").click();
  await expect(page.locator(".photo-card")).toHaveCount(1);
  await expect(page.locator(".photo-card")).toHaveAttribute("data-photo-path","B/photo.png");
  await expect(page.getByRole("navigation",{name:"Fotopfad"}).getByRole("link",{name:"B",exact:true})).toHaveAttribute("href","/photos?path=B");
  await page.setViewportSize({width:390,height:800});
  expect(await page.evaluate(() => document.documentElement.scrollWidth-innerWidth)).toBeLessThanOrEqual(1);
  await page.getByRole("navigation",{name:"Fotopfad"}).getByRole("link",{name:"Personen im Ordner",exact:true}).click();
  await expect(page.locator(".photo-folder-link")).toHaveCount(1);
  await page.getByRole("navigation",{name:"Fotopfad"}).getByRole("link",{name:"B",exact:true}).click();
  await expect(page).toHaveURL(baseURL+"/photos?path=B");
  await context.close();
});
