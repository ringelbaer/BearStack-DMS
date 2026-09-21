import { expect, test } from "@playwright/test";
import { startBearStack, stopBearStack, freePort } from "./server-fixture.mjs";
import http from "node:http";
import { mkdtemp, mkdir, writeFile, readFile, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

const model = "yunet-2023mar-sface-2021dec-v1";
const token = "bearstack-test-service-token-000000";
const png = await readFile(new URL("../../services/faces/tests/fixtures/astronaut.png", import.meta.url));
const photoPaths = ["root.png", ...Array.from({length: 9}, (_, i) => `20240102_Family_Trip/${i}.png`), "20240102_Family_Trip/Nested_Folder/child.png"];
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
      const landscape=false; calls++;
      const embedding=Array(128).fill(0);embedding[landscape?1:0]=1;
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

test("person folders format paths, preview eight faces and apply whole-folder actions", async ({ browser }) => {
  test.setTimeout(90_000);
  const context = await browser.newContext({ httpCredentials: { username: "manager", password: "secret" } });
  try {
    const page = await context.newPage(), errors = [];
    page.on("pageerror", error => errors.push(error.message));
    await page.goto(baseURL + "/login");
    await page.getByLabel("Benutzername").fill("manager");
    await page.locator('input[name="password"]').fill("secret");
    await page.getByRole("button", { name: "Anmelden", exact: true }).click();
    // Materialize every fixture photo; gallery folder previews index only a subset.
    for (const file of photoPaths) {
      expect((await context.request.get(baseURL + "/api/photos/v1/media/info?path=" + encodeURIComponent(file))).ok()).toBe(true);
    }
    const enabled = await context.request.post(baseURL + "/settings/photos/faces", { form: { enabled: "1", delay_millis: "100" }, headers: { Origin: baseURL } });
    expect(enabled.ok()).toBe(true);
    await expect.poll(async () => (await (await context.request.get(baseURL + "/settings/photos/faces?format=json")).json()).status.done, { timeout: 30000 }).toBe(11);
    const person = (await (await context.request.get(baseURL + "/photos/people?format=json")).json()).people[0];
    expect((await context.request.post(baseURL + `/photos/people/${person.id}/rename`, { form: { name: "Ada" }, headers: { Origin: baseURL } })).ok()).toBe(true);
    await page.goto(baseURL + `/photos/people/${person.id}`);
    await expect(page.locator("[data-person-summary]")).toBeHidden();
    await page.getByRole("button", { name: "Stammdaten", exact: true }).click();
    const details = page.locator("[data-person-details-dialog]");
    await details.locator('[name="birth_date"]').fill("1980-05-06");
    await details.getByRole("button", { name: "Stammdaten speichern" }).click();
    await expect(page.locator("[data-person-summary]")).toHaveText("Geboren: 06.05.1980");
    await page.getByLabel("Weitere Personenaktionen", { exact: true }).click();
    const folderLink = page.getByRole("link", { name: "Ordner-Pfade prüfen", exact: true });
    await expect(folderLink).not.toHaveClass(/secondary-button/);
    await expect(page.locator("[data-detail-display] > summary")).toHaveAttribute("aria-label", "Anzeigeeinstellungen");
    expect(await folderLink.evaluate(link => getComputedStyle(link).borderTopWidth)).toBe("0px");
    await folderLink.click();
    const folders = page.locator("[data-person-folder]");
    await expect(folders).toHaveCount(3);
    await expect(folders.nth(0).locator("h2")).toHaveText("Fotos");
    await expect(folders.nth(1).locator("h2")).toHaveText("Fotos / 02.01.2024 · Family Trip");
    await expect(folders.nth(2).locator("h2")).toHaveText("Fotos / 02.01.2024 · Family Trip / Nested Folder");
    await expect(folders.nth(1).locator("img")).toHaveCount(8);
    for (const width of [320, 1440]) {
      await page.setViewportSize({ width, height: 1000 });
      expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(1);
    }
    const assignment = page.locator("[data-person-dialog]");
    await expect(folders.getByRole("combobox")).toHaveCount(0);
    await expect(assignment).toHaveCount(1);
    for (const [index, display] of [[0, "Fotos"], [1, "Fotos / 02.01.2024 · Family Trip"], [2, "Fotos / 02.01.2024 · Family Trip / Nested Folder"]]) {
      const opener = folders.nth(index).getByRole("button", { name: /Gesichter zuordnen/ });
      await opener.click();
      await expect(assignment).toBeVisible();
      await expect(assignment.locator("[data-folder-dialog-path]")).toHaveText(display);
      if (index === 2) {
        await page.setViewportSize({ width: 320, height: 900 });
        const bounds = await assignment.boundingBox();
        expect(bounds.x).toBeGreaterThanOrEqual(0);
        expect(bounds.x + bounds.width).toBeLessThanOrEqual(320);
        expect(await assignment.evaluate(d => d.scrollWidth - d.clientWidth)).toBeLessThanOrEqual(1);
        await page.screenshot({ path: "/tmp/bearstack-folder-dialog-320.png", fullPage: true });
      }
      await assignment.getByRole("button", { name: "Abbrechen", exact: true }).focus();
      await page.keyboard.press("Escape");
      await expect(assignment).not.toBeVisible();
      await expect(opener).toBeFocused();
    }
    await folders.nth(1).getByRole("button", { name: /Gesichter zuordnen/ }).click();
    await assignment.getByRole("combobox").fill("Grace");
    await page.evaluate(() => { window.folderDocumentMarker = "original"; });
    let writes = 0, releaseWrite;
    const pendingWrite = new Promise(resolve => { releaseWrite = resolve; });
    await page.route(baseURL + `/photos/people/${person.id}/folder`, async route => {
      writes++;
      await route.fetch();
      await pendingWrite;
      await route.abort();
    }, {times:1});
    await assignment.getByRole("button", { name: /Gesichter zuordnen/ }).click();
    await expect.poll(() => writes).toBe(1);
    await assignment.locator("form").evaluate(form => form.requestSubmit());
    releaseWrite();
    await expect(assignment.locator("[data-person-dialog-status]")).toContainText("nicht bestätigt");
    await expect(assignment.locator("[data-person-submit]")).toBeDisabled();
    await assignment.getByRole("button", {name:"Abbrechen", exact:true}).click();
    await expect(folders).toHaveCount(3);
    await page.route(/\/folder\?.*format=fragment/, route => route.fulfill({status:503, body:"unavailable"}), {times:1});
    await page.getByRole("button", {name:"Ansicht aktualisieren", exact:true}).click();
    await expect(page.locator("[data-folder-status]")).toContainText("konnte nicht aktualisiert");
    await expect(folders).toHaveCount(3);
    await expect(folders.nth(0).getByRole("button", {name:/Gesichter zuordnen/})).toBeDisabled();
    // A stalled read times out without replacing the old list or repeating POST.
    await page.clock.install();
    let releaseRead, readStarted = false;
    const delayedRead = new Promise(resolve => { releaseRead = resolve; });
    await page.route(/\/folder\?.*format=fragment/, async route => {
      readStarted = true;
      await delayedRead;
      await route.fulfill({status:200, body:"late response"}).catch(() => {});
    }, {times:1});
    await page.getByRole("button", {name:"Ansicht aktualisieren", exact:true}).click();
    await expect.poll(() => readStarted).toBe(true);
    await page.clock.fastForward(30001);
    await expect(page.locator("[data-folder-status]")).toContainText("konnte nicht aktualisiert");
    releaseRead();
    await expect(folders).toHaveCount(3);
    await page.getByRole("button", {name:"Ansicht aktualisieren", exact:true}).click();
    await expect(folders).toHaveCount(2);
    expect(writes).toBe(1);
    expect(await page.evaluate(() => window.folderDocumentMarker)).toBe("original");
    await expect(folders.nth(0).locator("h2")).toHaveText("Fotos");
    await expect(folders.nth(1).locator("h2")).toHaveText("Fotos / 02.01.2024 · Family Trip / Nested Folder");
    let people = (await (await context.request.get(baseURL + "/photos/people?format=json")).json()).people;
    const grace = people.find(p => p.name === "Grace");
    expect(grace.count).toBe(9);
    expect(people.find(p => p.id === person.id).count).toBe(2);
    await page.goto(baseURL + `/photos/people/${grace.id}/folder`);
    await folders.getByRole("button", { name: /Gesichter zuordnen/ }).click();
    await assignment.getByRole("combobox").fill("Ada");
    await assignment.getByRole("option", { name: /„Ada“ als Ziel wählen/ }).click();
    await expect(assignment).toBeVisible();
    await assignment.getByRole("button", { name: "Gesichter zuordnen", exact: true }).click();
    await expect(folders).toHaveCount(0);
    await expect(page).toHaveURL(baseURL + `/photos/people/${grace.id}/folder`);
    await expect(page.getByRole("link", {name:"Zur Personenübersicht", exact:true})).toBeVisible();
    await page.goto(baseURL + `/photos/people/${person.id}/folder`);
    await expect(folders).toHaveCount(3);
    await page.evaluate(() => { window.folderDocumentMarker = "actions"; });
    // A real concurrent rename invalidates every old folder revision.
    expect((await context.request.post(baseURL + `/photos/people/${person.id}/rename`, {
      form:{name:"Ada aktualisiert"}, headers:{Origin:baseURL}
    })).ok()).toBe(true);
    await folders.nth(1).getByText("Weitere Ordneraktionen", {exact:true}).click();
    await folders.nth(1).getByRole("button", {name:"Pfad ausschließen und Zuordnungen auflösen", exact:true}).click();
    await page.locator("[data-app-dialog-confirm]").click();
    await expect(page.locator("[data-folder-status]")).toContainText("inzwischen geändert");
    await expect(folders).toHaveCount(3);
    await page.getByRole("button", {name:"Ansicht aktualisieren", exact:true}).click();
    await expect(page.locator("[data-folder-refresh]")).toBeHidden();
    await expect(page.locator("[data-folder-heading]")).toHaveText("Ordner: Ada aktualisiert");
    await folders.nth(1).getByText("Weitere Ordneraktionen", {exact:true}).click();
    await folders.nth(1).getByRole("button", { name: "Pfad ausschließen und Zuordnungen auflösen", exact: true }).click();
    await page.locator("[data-app-dialog-confirm]").click();
    await expect(folders.nth(1).getByRole("button", { name: "Pfad wieder freigeben" })).toBeVisible();
    await expect(folders.nth(1).locator("img")).toHaveCount(0);
    expect(await page.evaluate(() => window.folderDocumentMarker)).toBe("actions");
    await expect(folders.nth(1).locator("h2")).toHaveText("Fotos / 02.01.2024 · Family Trip");
    await page.route(/\/folder\?.*format=fragment/, route => route.fulfill({status:503, body:"unavailable"}), {times:1});
    await folders.nth(1).getByRole("button", { name: "Pfad wieder freigeben" }).click();
    await page.locator("[data-app-dialog-confirm]").click();
    await expect(page.locator("[data-folder-status]")).toContainText("Ordneraktion gespeichert, aber");
    await expect(folders).toHaveCount(3);
    await expect(folders.nth(1).getByRole("button", {name:"Pfad wieder freigeben"})).toBeDisabled();
    await page.getByRole("button", {name:"Ansicht aktualisieren", exact:true}).click();
    await expect(folders).toHaveCount(2);
    await folders.nth(1).getByText("Weitere Ordneraktionen",{exact:true}).click();
    await folders.nth(1).getByRole("button", { name: /Gesichter auf unbenannt setzen/ }).click();
    await page.locator("[data-app-dialog-confirm]").click();
    await expect(folders).toHaveCount(1);
    await folders.getByText("Weitere Ordneraktionen",{exact:true}).click();
    await folders.getByRole("button", { name: /Gesichter ignorieren/ }).click();
    await page.locator("[data-app-dialog-confirm]").click();
    await expect(folders).toHaveCount(0);
    await expect(page).toHaveURL(baseURL + `/photos/people/${person.id}/folder`);
    expect(await page.evaluate(() => window.folderDocumentMarker)).toBe("actions");
    expect(errors).toEqual([]);
  } finally { await context.close(); }
});

test("folder pagination keeps spaced controls on first, middle and last pages", async ({ browser }) => {
  test.setTimeout(90_000);
  const context = await browser.newContext({ httpCredentials: { username: "manager", password: "secret" } });
  try {
    // A real three-page result exercises every combination of navigation links.
    for (let i = 0; i < 81; i++) {
      const name = `Paging_${String(i).padStart(3, "0")}/photo.png`;
      const file = path.join(root, "photos", name);
      await mkdir(path.dirname(file), { recursive: true });
      await writeFile(file, png);
      expect((await context.request.get(baseURL + "/api/photos/v1/media/info?path=" + encodeURIComponent(name))).ok()).toBe(true);
    }
    expect((await context.request.post(baseURL + "/settings/photos/faces", { form: { enabled: "1", delay_millis: "100" }, headers: { Origin: baseURL } })).ok()).toBe(true);
    await expect.poll(async () => {
      const result = await (await context.request.get(baseURL + "/settings/photos/faces?format=json")).json();
      return result.status.done;
    }, { timeout: 45000 }).toBe(photoPaths.length + 81);
    const people = [];
    for (let current = 1, total = 1; current <= total; current++) {
      const result = await (await context.request.get(baseURL + `/photos/people?format=json&page=${current}`)).json();
      people.push(...result.people);
      total = result.total_pages;
    }
    const person = people[0];
    // Explicit assignment keeps pagination independent of recognition grouping.
    for (let start = 1; start < people.length; start += 60) {
      const sources = people.slice(start, start + 60);
      const data = new URLSearchParams({ target: String(person.id) });
      for (const source of sources.slice(1)) data.append("person_id", String(source.id));
      expect((await context.request.post(baseURL + `/photos/people/${sources[0].id}/merge`, {
        data: data.toString(), headers: { Origin: baseURL, Accept: "application/json", "Content-Type": "application/x-www-form-urlencoded" },
      })).ok()).toBe(true);
    }
    const page = await context.newPage();
    await page.goto(baseURL + "/login");
    await page.getByLabel("Benutzername").fill("manager");
    await page.locator('input[name="password"]').fill("secret");
    await page.getByRole("button", { name: "Anmelden", exact: true }).click();
    await page.goto(baseURL + `/photos/people/${person.id}/folder`);
    const navigation = page.getByRole("navigation", { name: "Ordnerseiten" });
    for (const current of [1, 2, 3]) {
      await expect(navigation.locator('[aria-current="page"]')).toHaveText(`Seite ${current}`);
      await expect(navigation.getByRole("link", { name: "Zurück", exact: true })).toHaveCount(current > 1 ? 1 : 0);
      await expect(navigation.getByRole("link", { name: "Weiter", exact: true })).toHaveCount(current < 3 ? 1 : 0);
      for (const width of [320, 640, 768, 1440]) {
        await page.setViewportSize({ width, height: 900 });
        const controls = await navigation.locator(":scope > *").all();
        const boxes = await Promise.all(controls.map(control => control.boundingBox()));
        for (let i = 1; i < boxes.length; i++) {
          expect(boxes[i].x - boxes[i - 1].x - boxes[i - 1].width).toBeGreaterThanOrEqual(11);
          expect(Math.abs(boxes[i].y + boxes[i].height / 2 - boxes[0].y - boxes[0].height / 2)).toBeLessThanOrEqual(1);
        }
        const last = boxes.at(-1);
        const bounds = await navigation.boundingBox();
        expect(Math.abs((boxes[0].x + last.x + last.width) / 2 - bounds.x - bounds.width / 2)).toBeLessThanOrEqual(1);
        expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(1);
      }
      if (current < 3) await navigation.getByRole("link", { name: "Weiter", exact: true }).click();
    }
    await navigation.getByRole("link", { name: "Zurück", exact: true }).click();
    await expect(navigation.locator('[aria-current="page"]')).toHaveText("Seite 2");
    const folders = page.locator("[data-person-folder]");
    const nextPage = await (await context.request.get(baseURL + `/photos/people/${person.id}/folder?format=json&page=3`)).json();
    const replacement = nextPage.folders[0];
    await page.evaluate(() => { window.folderDocumentMarker = "page two"; });
    await folders.first().getByText("Weitere Ordneraktionen", {exact:true}).click();
    await folders.first().getByRole("button", {name:/Gesichter ignorieren/}).click();
    await page.locator("[data-app-dialog-confirm]").click();
    await expect(folders.last().locator("h2")).toHaveText(replacement.display_path);
    await expect(folders).toHaveCount(40);
    await expect(navigation.locator('[aria-current="page"]')).toHaveText("Seite 2");
    expect(await page.evaluate(() => window.folderDocumentMarker)).toBe("page two");
    // Newly inserted rows use delegated actions and server-formatted dialog paths.
    const more = folders.last().locator(".people-menu");
    await more.locator("summary").click();
    await more.locator("summary").press("Escape");
    await expect(more).not.toHaveAttribute("open", "");
    await expect(more.locator("summary")).toBeFocused();
    await more.locator("summary").click();
    await page.locator("h1").click();
    await expect(more).not.toHaveAttribute("open", "");
    await folders.last().getByRole("button", {name:/Gesichter zuordnen/}).click();
    await expect(page.locator("[data-folder-dialog-path]")).toHaveText(replacement.display_path);
    await page.getByRole("button", {name:"Abbrechen", exact:true}).click();
  } finally { await context.close(); }
});
