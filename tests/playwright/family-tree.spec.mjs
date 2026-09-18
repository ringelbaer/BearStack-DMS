import { expect, test } from "@playwright/test";
import { startBearStack, stopBearStack, freePort } from "./server-fixture.mjs";
import http from "node:http";
import { mkdtemp, mkdir, writeFile, readFile, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

const model = "yunet-2023mar-sface-2021dec-v1";
const token = "bearstack-test-service-token-000000";
const png = await readFile(new URL("../../services/faces/tests/fixtures/astronaut.png", import.meta.url));
const photoPaths = ["a.png", "b.png", "c.png", "d.png", "e.png"];
let root, baseURL, app, service;

test.beforeAll(async () => {
  root = await mkdtemp(path.join(os.tmpdir(), "bearstack-faces-e2e-"));
  const photos = path.join(root, "photos");
  for (const name of photoPaths) {
    const file = path.join(photos, name);
    await mkdir(path.dirname(file), { recursive: true });
    await writeFile(file, png);
  }
  let calls=0;
  service=http.createServer((request,response)=>{
    if(request.headers.authorization!=="Bearer "+token){response.writeHead(401);response.end();return;}
    response.setHeader("Content-Type","application/json");
    if(request.url==="/health"){response.end(JSON.stringify({ready:true,protocol:1,model}));return;}
    request.resume();request.on("end",()=>{
      const landscape=false; const axis=calls++%5;
      const embedding=Array(128).fill(0);embedding[axis]=1;
      const bounds=landscape?{x:.3,y:.1,width:.4,height:.2}:{x:.35,y:.05,width:.25,height:.5};
      response.end(JSON.stringify({model,faces:[{...bounds,confidence:.99,embedding}]}));
    });
  });
  await new Promise(resolve=>service.listen(0,"127.0.0.1",resolve));
  const appPort=await freePort();baseURL=`http://127.0.0.1:${appPort}`;
  const config=path.join(root,"config.json");await writeFile(config,JSON.stringify({addr:`127.0.0.1:${appPort}`,data_dir:path.join(root,"data"),auth:{credentials:[{username:"admin",password:"secret",role:"admin"},{username:"manager",password:"secret",role:"photos_manager"},{username:"reader",password:"secret",role:"photos_read"}]},photos:{enabled:true,root_dir:photos,face_service_url:`http://127.0.0.1:${service.address().port}`,face_service_token:token}}));
  app = await startBearStack({ configPath: config, baseURL }, { username: "manager", password: "secret" });
});
test.afterAll(async ({}, testInfo) => {
  testInfo.setTimeout(75_000);
  await stopBearStack(app);
  if (service) await new Promise(resolve => service.close(resolve));
  if (root) await rm(root, { recursive: true, force: true });
});

test("family trees configure roots, deduplicate connections and support zoom, search and mobile", async ({ browser }) => {
  test.setTimeout(90_000);
  const context = await browser.newContext({ httpCredentials: { username: "manager", password: "secret" } });
  try {
    const page = await context.newPage(), errors = [];
    page.on("pageerror", error => errors.push(error.message));
    await page.goto(baseURL + "/login");
    await page.getByLabel("Benutzername").fill("manager");
    await page.locator('input[name="password"]').fill("secret");
    await page.getByRole("button", { name: "Anmelden", exact: true }).click();
    for (const file of photoPaths) expect((await context.request.get(baseURL + "/api/photos/v1/media/info?path=" + file)).ok()).toBe(true);
    expect((await context.request.post(baseURL + "/settings/photos/faces", { form: { enabled: "1", delay_millis: "100" }, headers: { Origin: baseURL } })).ok()).toBe(true);
    await expect.poll(async () => (await (await context.request.get(baseURL + "/settings/photos/faces?format=json")).json()).status.done, { timeout: 30000 }).toBe(5);
    const people = (await (await context.request.get(baseURL + "/photos/people?format=json")).json()).people.sort((a,b) => a.id-b.id);
    expect(people).toHaveLength(5);
    const names = ["Alice", "Bernd", "Clara", "Doris", "Emil"];
    for (let i=0; i<people.length; i++) expect((await context.request.post(baseURL + `/photos/people/${people[i].id}/rename`, { form: { name: names[i] }, headers: { Origin: baseURL } })).ok()).toBe(true);
    async function details(index, data) {
      const url = baseURL + `/photos/people/${people[index].id}/details`;
      const current = await (await context.request.get(url)).json();
      const response = await context.request.put(url, { data: { revision: current.revision, birth_date: current.birth_date, death_date: current.death_date, sibling_ids: current.siblings.map(p => p.id), marriages: current.marriages.map(m => ({ id: m.id, spouse_id: m.spouse.id, wedding_date: m.wedding_date, divorce_date: m.divorce_date })), ...data }, headers: { Origin: baseURL } });
      expect(response.ok(), await response.text()).toBe(true);
    }
    await details(0, { mother_id: people[1].id, birth_date: "1980-02-03" });
    await details(1, { sibling_ids: [people[2].id] });
    await details(2, { birth_date: "1950-04-05", marriages: [{ spouse_id: people[3].id, wedding_date: "1970-06-07", divorce_date: "1990-08-09" }] });
    await page.goto(baseURL + "/photos");
    await expect(page.locator('a[href="/photos/family-tree"]')).toHaveCount(0);
    await page.goto(baseURL + "/settings/photos/family-tree");
    for (const name of ["Alice", "Doris", "Emil", "Alice"]) {
      await page.getByRole("combobox").fill(name);
      await page.getByRole("option", { name: new RegExp("^" + name + " \\(#") }).click();
    }
    await expect(page.locator("[data-tree-roots] li")).toHaveCount(3);
    await page.getByRole("button", { name: "Auswahl speichern", exact: true }).click();
    await page.route("**/photos/family-tree?format=json", route => route.fulfill({ status: 503, json: { error: "Test: Laden unterbrochen" } }), { times: 1 });
    await page.getByRole("link", { name: "Stammbäume ansehen", exact: true }).click();
    await expect(page.locator("[data-tree-status]")).toHaveText("Test: Laden unterbrochen");
    await page.getByRole("button", { name: "Erneut laden", exact: true }).click();
    await expect(page.locator("[data-tree-status]")).toHaveText("2 Stammbäume · 4 Personen · 3 Verbindungen");
    await expect(page.locator("[data-tree-select] option")).toHaveCount(2);
    await expect(page.locator("[data-tree-inspector]")).toContainText("03.02.1980");
    await expect.poll(() => page.locator("[data-tree-inspector] > img").evaluate(img => img.naturalWidth)).toBeGreaterThan(0);
    await page.getByLabel("Person im Stammbaum suchen").fill("Clara");
    await page.locator("[data-tree-search-results]").getByRole("button", { name: "Clara", exact: true }).click();
    await expect(page.locator("[data-tree-inspector]")).toContainText("05.04.1950");
    await expect(page.locator("[data-tree-inspector]")).toContainText("Geschiedene Ehe");
    await expect(page.locator("[data-tree-inspector]")).toContainText("Heirat: 07.06.1970");
    await expect(page.locator("[data-tree-inspector]")).toContainText("Scheidung: 09.08.1990");
    await expect(page.locator("[data-tree-zoom]")).toHaveText("100 %");
    await page.getByRole("button", { name: "Vergrößern", exact: true }).click();
    await expect(page.locator("[data-tree-zoom]")).toHaveText("125 %");
    await page.locator("[data-tree-viewport]").focus();
    await page.keyboard.press("-");
    await expect(page.locator("[data-tree-zoom]")).toHaveText("100 %");
    const viewport = page.locator("[data-tree-viewport]");
    expect(await viewport.evaluate(v => v.scrollWidth > v.clientWidth)).toBe(true);
    await viewport.evaluate(v => { v.scrollLeft = v.scrollWidth; });
    expect(await viewport.evaluate(v => v.scrollLeft)).toBeGreaterThan(0);
    await page.getByRole("button", { name: "Alles anzeigen", exact: true }).click();
    for (const width of [1440, 390, 320]) {
      await page.setViewportSize({ width, height: 1000 });
      await page.getByRole("button", { name: "Alles anzeigen", exact: true }).click();
      expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(1);
      await page.evaluate(() => window.scrollTo(0, 0));
      await page.screenshot({ path: `/tmp/bearstack-family-tree-${width}.png`, fullPage: true });
    }
    await page.locator("[data-tree-select]").selectOption("1");
    await expect(page.locator("[data-tree-status]")).toHaveText("2 Stammbäume · 1 Person · 0 Verbindungen");
    await expect(page.locator("[data-tree-inspector] h2")).toHaveText("Emil");
    // A very large connected family stays navigable without thousands of DOM cards.
    const large = { trees: [{ id: 1, roots: [1], people: Array.from({length: 2000}, (_,i) => ({ id: i+1, name: "Familie " + (i+1), birth_date: "", death_date: "", face_id: people[0].preview_face_id || 1, tags: [] })), relations: Array.from({length:1999}, (_,i) => ({id:0, from:i+1, to:i+2, kind:"sibling"})) }] };
    await page.route("**/photos/family-tree?format=json", route => route.fulfill({json:large}));
    await page.goto(baseURL + "/photos/family-tree");
    await expect(page.locator("[data-tree-status]")).toContainText("2000 Personen");
    await expect(page.locator("[data-tree-person]")).toHaveCount(0);
    await page.getByLabel("Person im Stammbaum suchen").fill("Familie 2000");
    await page.getByLabel("Person im Stammbaum suchen").press("Enter");
    await expect(page.locator("[data-tree-inspector] h2")).toHaveText("Familie 2000");
    await expect(page.locator('[data-tree-person="2000"]')).toBeVisible();
    expect(await page.locator("[data-tree-person]").count()).toBeLessThan(10);
    await page.unroute("**/photos/family-tree?format=json");
    await page.goto(baseURL + "/settings/photos/family-tree");
    await page.getByRole("button", { name: "Auswahl leeren", exact: true }).click();
    await page.getByRole("button", { name: "Auswahl speichern", exact: true }).click();
    await expect(page.locator('a[href="/photos/family-tree"]')).toHaveCount(0);
    expect((await context.request.get(baseURL + "/photos/family-tree")).status()).toBe(404);
    expect(errors).toEqual([]);
  } finally { await context.close(); }
});
