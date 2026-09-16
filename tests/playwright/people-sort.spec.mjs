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
  await writeFile(configPath, JSON.stringify({ addr: `127.0.0.1:${port}`, data_dir: path.join(root, "data"), auth: { credentials: [{ username: "manager", password: "secret", role: "photos_manager" }, { username: "reader", password: "secret", role: "photos_read" }] }, photos: { enabled: true, root_dir: photos, face_service_url: `http://127.0.0.1:${service.address().port}`, face_service_token: token } }));
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
async function login(page, username = "manager") {
  await page.goto(baseURL + "/login?next=%2Fphotos%2Fpeople%3Fpage%3D1");
  await page.getByLabel("Benutzername").fill(username);
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
  await page.getByLabel("Weitere Personenaktionen", {exact:true}).click();
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
  await expect(people.locator("img")).toHaveCount(1);
  await people.click();
  await expect(page.getByRole("link", { name: /^Alle 1 Person/ })).toHaveCount(1);
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
  await page.getByLabel("Weitere Personenaktionen", {exact:true}).click();
  await page.getByRole("button",{name:"Stammdaten",exact:true}).click();
  await expect(page.locator("[data-person-details-fields]")).toBeVisible();
  await page.getByRole("combobox",{name:"Mutter",exact:true}).fill("Mutter Beispiel");
  await page.getByRole("option",{name:/^Mutter Beispiel/}).click();
  await page.getByRole("combobox",{name:"Vater",exact:true}).fill("Vater Beispiel");
  await page.getByRole("option",{name:/^Vater Beispiel/}).click();
  await page.getByRole("button",{name:"Stammdaten speichern",exact:true}).click();
  await expect(page.locator("[data-person-details-dialog]")).not.toBeVisible();
  await expect(page.locator("[data-detail-status]")).toContainText("Stammdaten gespeichert");
  await page.getByLabel("Weitere Personenaktionen", {exact:true}).click();
  await page.getByRole("button",{name:"Stammdaten",exact:true}).click();
  await expect(page.locator("[data-person-details-fields]")).toBeVisible();
  await expect(page.getByRole("combobox",{name:"Mutter",exact:true})).toHaveValue(`Mutter Beispiel (#${parents[0].id})`);
  await page.setViewportSize({width:390,height:800});
  const box = await page.locator("[data-person-details-dialog]").boundingBox();
  expect(box.x).toBeGreaterThanOrEqual(0); expect(box.x+box.width).toBeLessThanOrEqual(390);
  await page.getByRole("button",{name:"Mutter entfernen",exact:true}).click();
  await expect(page.getByRole("combobox",{name:"Mutter",exact:true})).toHaveValue("");
  await expect(page.getByRole("combobox",{name:"Mutter",exact:true})).toHaveAttribute("aria-expanded","false");
  await page.getByRole("button",{name:"Vater entfernen",exact:true}).click();
  await expect(page.getByRole("combobox",{name:"Vater",exact:true})).toHaveValue("");
  await page.getByRole("button",{name:"Stammdaten speichern",exact:true}).click();
  await expect(page.locator("[data-person-details-dialog]")).not.toBeVisible();
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

test("selected people receive additive tags through the batch dialog with cancel and retry", async ({ browser }) => {
  const context = await browser.newContext();
  const page = await context.newPage();
  const errors=[];page.on("pageerror",error=>errors.push(error.message));
  await login(page);
  await page.goto(baseURL + "/photos/people?filter=all&sort=count_desc&page=1");
  const cards=page.locator(".person-overview-card");
  const ids=await cards.evaluateAll(items=>items.slice(0,3).map(item=>item.dataset.personId));
  const originals=[];
  for (const id of ids) originals.push((await (await context.request.get(baseURL+`/photos/people/${id}?format=json`)).json()).tags || []);
  const tagsOf=async id=>(await (await context.request.get(baseURL+`/photos/people/${id}?format=json`)).json()).tags || [];
  try {
    const seed=new URLSearchParams();[...originals[0],"batch-vorher"].forEach(tag=>seed.append("tags",tag));
    const seeded=await context.request.post(baseURL+`/photos/people/${ids[0]}/tags`,{data:seed.toString(),headers:{"Content-Type":"application/x-www-form-urlencoded",Origin:baseURL}});
    expect(seeded.ok()).toBe(true);
    const previousFirst=[...new Set([...originals[0],"batch-vorher"])].sort();
    const action=page.getByRole("button",{name:"Personen-Tags ergänzen",exact:true});
    await expect(action).toBeHidden();
    for(const id of ids.slice(0,2)) await page.locator(`[data-person-select][value="${id}"]`).check();
    await page.setViewportSize({width:390,height:844});
    await action.click();
    const dialog=page.locator("[data-tag-select-modal]");
    await dialog.locator("[data-tag-select-search]").fill("batch-abgebrochen");
    await dialog.getByRole("button",{name:"Tag anlegen",exact:true}).click();
    await page.keyboard.press("Escape");
    expect(await tagsOf(ids[0])).toEqual(previousFirst);
    await action.click();
    await dialog.locator("[data-tag-select-search]").fill("batch-familie");
    await dialog.getByRole("button",{name:"Tag anlegen",exact:true}).click();
    let attempts=0;
    await page.route("**/photos/people/tags/add",async route=>{
      const body=new URLSearchParams(route.request().postData());
      expect(body.getAll("ids")).toEqual(ids.slice(0,2));
      expect(body.getAll("tags")).toEqual(["batch-familie"]);
      if(++attempts===1) await route.fulfill({status:500,contentType:"application/json",body:'{"error":"fixture"}'});
      else await route.continue();
    });
    await dialog.getByRole("button",{name:"Übernehmen",exact:true}).click();
    await expect(dialog.locator("[data-tag-select-error]")).toBeVisible();
    expect(await tagsOf(ids[1])).toEqual(originals[1]);
    await dialog.getByRole("button",{name:"Übernehmen",exact:true}).click();
    await expect(dialog).not.toBeVisible();
    await expect.poll(()=>tagsOf(ids[0])).toEqual([...new Set([...previousFirst,"batch-familie"])].sort());
    expect(await tagsOf(ids[1])).toEqual([...new Set([...originals[1],"batch-familie"])].sort());
    expect(await tagsOf(ids[2])).toEqual(originals[2]);
    expect(new URL(page.url()).searchParams.get("sort")).toBe("count_desc");
    await expect(page.locator(".notice")).toHaveText("Personen-Tags für die Auswahl ergänzt.");
    expect(attempts).toBe(2);expect(errors).toEqual([]);
    expect(await page.evaluate(()=>document.documentElement.scrollWidth-innerWidth)).toBeLessThanOrEqual(1);
  } finally {
    for(let i=0;i<2;i++) {
      const form=new URLSearchParams();originals[i].forEach(tag=>form.append("tags",tag));
      await context.request.post(baseURL+`/photos/people/${ids[i]}/tags`,{data:form.toString(),headers:{"Content-Type":"application/x-www-form-urlencoded",Origin:baseURL}}).catch(()=>{});
    }
    await context.close();
  }
});

test("person records support multiple relatives, reciprocal marriages, cancellation and concurrent edits", async ({ browser }) => {
  const context=await browser.newContext();const page=await context.newPage();await login(page);
  const errors=[];page.on("pageerror",error=>errors.push(error.message));
  const persons=(await (await context.request.get(baseURL+"/photos/people?format=json&sort=count_desc")).json()).people.slice(0,5);
  const names=["Profil Alpha","Profil Beta","Profil Gamma","Profil Delta","Profil Epsilon"];
  for(let i=0;i<5;i++) expect((await context.request.post(baseURL+`/photos/people/${persons[i].id}/rename`,{form:{name:names[i]},headers:{Origin:baseURL,Accept:"application/json"}})).ok()).toBe(true);
  const endpoint=baseURL+`/photos/people/${persons[0].id}/details`;
  const details=async id=>(await (await context.request.get(baseURL+`/photos/people/${id}/details`)).json());
  const open=async()=>{await page.getByLabel("Weitere Personenaktionen",{exact:true}).click();await page.getByRole("button",{name:"Stammdaten",exact:true}).click();await expect(page.locator("[data-person-details-fields]")).toBeVisible();};
  const dialog=page.locator("[data-person-details-dialog]");
  const choose=async(row,name)=>{await row.locator("[data-person-search]").fill(name);await row.getByRole("option").filter({hasText:name}).click();};
  try {
    await page.goto(baseURL+`/photos/people/${persons[0].id}`);await open();
    await dialog.getByLabel("Geburtsdatum",{exact:true}).fill("1960-02-29");
    await expect(dialog.getByLabel("Sterbedatum",{exact:true})).toBeHidden();
    await dialog.getByRole("button",{name:"Sterbedatum ergänzen",exact:true}).click();
    await expect(dialog.getByLabel("Sterbedatum",{exact:true})).toBeFocused();
    await dialog.getByLabel("Sterbedatum",{exact:true}).fill("2020-01-01");
    for(const name of names.slice(1,3)) {await dialog.getByRole("button",{name:"Geschwister hinzufügen",exact:true}).click();await choose(dialog.locator('[data-relation="sibling"]').last(),name);}
    for(const [index,name] of names.slice(3).entries()) {
      await dialog.getByRole("button",{name:"Ehe hinzufügen",exact:true}).click();const row=dialog.locator('[data-relation="marriage"]').last();await choose(row,name);
      await row.getByLabel("Hochzeitsdatum",{exact:true}).fill(index===0?"1980-06-15":"2000-07-20");
      await expect(row.getByLabel("Scheidungsdatum",{exact:true})).toBeHidden();
      if(index===0) {
        await row.getByRole("button",{name:"Scheidungsdatum ergänzen",exact:true}).click();
        await row.getByLabel("Scheidungsdatum",{exact:true}).fill("1995-02-01");
      }
    }
    await expect(dialog.getByRole("heading",{name:"Geschwister",exact:true})).toHaveCount(1);
    await expect(dialog.locator("legend")).toHaveCount(0);
    for(const width of [320,390,1440]) {
      await page.setViewportSize({width,height:900});
      const box=await dialog.boundingBox();expect(box.x).toBeGreaterThanOrEqual(0);expect(box.x+box.width).toBeLessThanOrEqual(width);
      expect(await dialog.evaluate(el=>el.scrollWidth-el.clientWidth)).toBeLessThanOrEqual(1);
      const row=dialog.locator('[data-relation="sibling"]').first();
      const input=await row.locator('[data-person-search]').boundingBox(), remove=await row.getByRole('button',{name:'Geschwisterzuordnung entfernen'}).boundingBox();
      expect(Math.abs(input.y-remove.y)).toBeLessThanOrEqual(1);expect(input.height).toBe(remove.height);
      const marriage=dialog.locator('[data-relation="marriage"]').last();
      const wedding=await marriage.getByLabel('Hochzeitsdatum',{exact:true}).boundingBox(), plus=await marriage.getByRole('button',{name:'Scheidungsdatum ergänzen'}).boundingBox();
      expect(Math.abs(wedding.y+wedding.height-plus.y-plus.height)).toBeLessThanOrEqual(1);
    }
    await page.screenshot({path:"/tmp/bearstack-person-records.png",fullPage:true});
    await dialog.getByRole("button",{name:"Stammdaten speichern",exact:true}).click();await expect(dialog).not.toBeVisible();
    let saved=await details(persons[0].id);expect(saved.birth_date).toBe("1960-02-29");expect(saved.death_date).toBe("2020-01-01");expect(saved.siblings).toHaveLength(2);expect(saved.marriages).toHaveLength(2);
    expect((await details(persons[1].id)).siblings[0].id).toBe(persons[0].id);expect((await details(persons[3].id)).marriages[0].spouse.id).toBe(persons[0].id);
    await open();
    await expect(dialog.getByLabel("Sterbedatum",{exact:true})).toBeVisible();
    await expect(dialog.locator('[data-relation="marriage"]').first().getByLabel("Scheidungsdatum",{exact:true})).toBeVisible();
    await expect(dialog.locator('[data-relation="marriage"]').last().getByLabel("Scheidungsdatum",{exact:true})).toBeHidden();
    await dialog.getByLabel("Geburtsdatum",{exact:true}).fill("1961-01-01");await page.keyboard.press("Escape");expect((await details(persons[0].id)).birth_date).toBe("1960-02-29");
    await expect(page.getByLabel("Weitere Personenaktionen",{exact:true})).toBeFocused();
    await open();
    const change={revision:saved.revision,birth_date:"1962-01-01",death_date:saved.death_date,sibling_ids:saved.siblings.map(p=>p.id),marriages:saved.marriages.map(m=>({id:m.id,spouse_id:m.spouse.id,wedding_date:m.wedding_date,divorce_date:m.divorce_date}))};
    expect((await context.request.put(endpoint,{data:change,headers:{Origin:baseURL}})).ok()).toBe(true);
    await dialog.getByRole("button",{name:"Stammdaten speichern",exact:true}).click();await expect(dialog.locator("[data-person-details-status]")).toContainText("zwischenzeitlich geändert");
    await dialog.getByRole("button",{name:"Stammdaten neu laden",exact:true}).click();await expect(dialog.getByLabel("Geburtsdatum",{exact:true})).toHaveValue("1962-01-01");
    const removedSibling=Number(await dialog.locator('[data-relation="sibling"] [data-person-target]').first().inputValue());
    await dialog.getByRole("button",{name:"Geschwisterzuordnung entfernen",exact:true}).first().click();
    await dialog.getByRole("button",{name:"Ehe entfernen",exact:true}).first().click();
    await dialog.getByRole("button",{name:"Stammdaten speichern",exact:true}).click();await expect(dialog).not.toBeVisible();
    expect((await details(removedSibling)).siblings).toHaveLength(0);expect((await details(persons[3].id)).marriages).toHaveLength(0);
    const reader=await browser.newContext({httpCredentials:{username:"reader",password:"secret"}});const readerPage=await reader.newPage();
    await login(readerPage,"reader");
    await readerPage.goto(baseURL+`/photos/people/${persons[0].id}`);await readerPage.getByLabel("Weitere Personenaktionen",{exact:true}).click();await readerPage.getByRole("button",{name:"Stammdaten",exact:true}).click();
    await expect(readerPage.getByLabel("Geburtsdatum",{exact:true})).toHaveValue("1962-01-01");await expect(readerPage.locator("[data-person-details-save]")).toHaveCount(0);
    await expect(readerPage.getByLabel("Sterbedatum",{exact:true})).toHaveValue("2020-01-01");
    await expect(readerPage.getByLabel("Scheidungsdatum",{exact:true})).toBeHidden();
    await expect(readerPage.getByRole("button",{name:"Scheidungsdatum ergänzen",exact:true})).toHaveCount(0);await expect(readerPage.getByRole("link",{name:"Profil Epsilon",exact:true})).toBeVisible();await reader.close();
    expect(errors).toEqual([]);
  } finally {
    const current=await details(persons[0].id).catch(()=>null);
    if(current) await context.request.put(endpoint,{data:{revision:current.revision,birth_date:"",death_date:"",sibling_ids:[],marriages:[]},headers:{Origin:baseURL}}).catch(()=>{});
    for(const person of persons) await context.request.post(baseURL+`/photos/people/${person.id}/rename`,{form:{name:person.name||""},headers:{Origin:baseURL,Accept:"application/json"}}).catch(()=>{});
    await context.close();
  }
});
