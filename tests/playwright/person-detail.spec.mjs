import { expect, test } from "@playwright/test";
import { startBearStack, stopBearStack, freePort } from "./server-fixture.mjs";
import http from "node:http";
import { mkdtemp, mkdir, writeFile, readFile, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

const model = "yunet-2023mar-sface-2021dec-v1";
const token = "bearstack-test-service-token-000000";
const png = await readFile(new URL("../../services/faces/tests/fixtures/astronaut.png", import.meta.url));
let root, baseURL, app, service;

test.beforeAll(async () => {
  root = await mkdtemp(path.join(os.tmpdir(), "bearstack-faces-e2e-"));
  const photos = path.join(root, "photos");
  for (const name of ["a", "b", "c"]) {
    const file = path.join(photos, "2002", "20021010-Assisi", "BILDER_LUKAS", name + "-sehr-langer-bilddateiname-ohne-kurze-abstaende-012345678901234567890123456789.png");
    await mkdir(path.dirname(file), { recursive: true });
    await writeFile(file, png);
  }
  let calls=0;
  service=http.createServer((request,response)=>{
    if(request.headers.authorization!=="Bearer "+token){response.writeHead(401);response.end();return;}
    response.setHeader("Content-Type","application/json");
    if(request.url==="/health"){response.end(JSON.stringify({ready:true,protocol:1,model}));return;}
    request.resume();request.on("end",()=>{
      const landscape=calls++===2;
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

test("person detail keeps actions compact and scopes face editing, selection and retries", async ({ browser }) => {
  const context = await browser.newContext({ httpCredentials: { username: "manager", password: "secret" } });
  try {
    const page = await context.newPage();
    const errors = []; page.on("pageerror", error => errors.push(error.message));
    await page.goto(baseURL + "/login");
    await page.getByLabel("Benutzername").fill("manager"); await page.locator('input[name="password"]').fill("secret");
    await page.getByRole("button", { name: "Anmelden", exact: true }).click();
    const enabled = await context.request.post(baseURL + "/settings/photos/faces", { form: { enabled: "1", delay_millis: "100" }, headers: { Origin: baseURL } });
    expect(enabled.ok()).toBe(true);
    await expect.poll(async () => (await (await context.request.get(baseURL + "/settings/photos/faces?format=json")).json()).status.done).toBe(3);
    const people = (await (await context.request.get(baseURL + "/photos/people?format=json")).json()).people;
    const person = people.find(p => p.count === 2);
    await page.goto(baseURL + "/photos/people/" + person.id);
    const cards = page.locator("[data-detail-face]");
    const dialog = page.locator("[data-person-dialog]");
    await expect(cards).toHaveCount(2);
    await expect(dialog).toHaveCount(1);
    await expect(page.locator(".people-pagination")).toBeHidden();
    for (const width of [320,390,480,640,1440]) {
      await page.setViewportSize({width,height:900});
      await page.screenshot({path:`/tmp/bearstack-person-detail-${width}.png`,fullPage:true});
      const geometry = await cards.first().boundingBox();
      expect(geometry.y).toBeLessThan(width <= 640 ? 290 : 360); expect(geometry.height).toBeLessThan(260);
      expect(await page.evaluate(() => document.documentElement.scrollWidth-document.documentElement.clientWidth)).toBeLessThanOrEqual(1);
    }
    for (const width of [320,390]) {
      await page.setViewportSize({width,height:900});
      const display = page.locator("[data-detail-display]");
      const help = page.locator(".person-detail-help");
      const more = page.locator(".person-detail-more");
      for (const [menu, panel] of [[display, ".people-display-options"], [help, ".person-detail-help-content"], [more, ":scope > div"]]) {
        await menu.locator("summary").click();
        await expect(menu).toHaveAttribute("open", "");
        const box = await menu.locator(panel).boundingBox();
        const firstCard = await cards.first().boundingBox();
        expect(box.width).toBeGreaterThan(width-60);
        expect(box.x).toBeGreaterThanOrEqual(0);
        expect(box.x+box.width).toBeLessThanOrEqual(width);
        expect(box.y+box.height).toBeLessThanOrEqual(firstCard.y);
        await page.screenshot({path:`/tmp/bearstack-person-detail-${width}-${panel.includes("help") ? "help" : panel.includes("options") ? "display" : "more"}.png`,fullPage:true});
        expect(await page.locator('details[name="person-detail-tools"][open]').count()).toBe(1);
      }
      await more.locator("summary").click();
    }
    await page.setViewportSize({width:1440,height:900});
    await page.getByLabel("Anzeigeeinstellungen", {exact:true}).click();
    await page.getByLabel("Thumbnailgröße", {exact:true}).selectOption("m");
    await page.getByLabel("Ordnerpfad anzeigen", {exact:true}).check();
    await expect(cards.first().locator(".person-detail-path")).toBeVisible();
    await page.reload();
    await expect(page.locator("[data-person-detail]")).toHaveAttribute("data-size","m");
    await expect(cards.first().locator(".person-detail-path")).toBeVisible();
    // The photo-info editor shares the dialog, but must save only once and
    // refresh the group behind the lightbox as well.
    const lightbox = page.locator("[data-photo-lightbox]");
    await cards.first().locator("[data-photo-item]").click();
    await lightbox.locator("[data-photo-info-toggle]").press("Enter");
    await lightbox.locator("[data-person-edit]").click();
    let renames = 0;
    page.on("request", request => { if (request.method() === "POST" && request.url().endsWith("/rename")) renames++; });
    await dialog.getByRole("combobox").fill("Testgruppe");
    await dialog.getByRole("button", {name:"Benennen",exact:true}).click();
    await expect(dialog).not.toBeVisible();
    await expect(lightbox.locator(".photo-info-face")).toContainText("Testgruppe");
    await expect(page.locator("[data-detail-title]")).toHaveText("Testgruppe");
    expect(renames).toBe(1);
    // Once named, corrections from this same info panel only move this face.
    await lightbox.locator("[data-person-edit]").click();
    await dialog.getByRole("combobox").fill("Einzelkorrektur");
    await dialog.getByRole("button", { name: "Gesicht benennen", exact: true }).click();
    await expect(dialog).not.toBeVisible();
    await expect(lightbox.locator(".photo-info-face")).toContainText("Einzelkorrektur");
    await expect(cards).toHaveCount(1);
    await expect(page.locator("[data-detail-title]")).toHaveText("Testgruppe");
    expect(renames).toBe(1);
    // Moving it back refreshes the target group's detail page without renaming it.
    await lightbox.locator("[data-person-edit]").click();
    await dialog.getByRole("combobox").fill("Testgruppe");
    await dialog.getByRole("option", { name: /^Testgruppe \(#/ }).click();
    await expect(dialog).not.toBeVisible();
    await expect(cards).toHaveCount(2);
    await expect(page.locator("[data-detail-title]")).toHaveText("Testgruppe");
    expect(renames).toBe(1);
    await lightbox.getByRole("button", {name:"Foto schließen",exact:true}).press("Enter");
    const secondID = await cards.nth(1).getAttribute("data-detail-face");
    await cards.nth(1).locator("[data-detail-edit-face]").click();
    await expect(dialog.locator("#person-dialog-title")).toHaveText("Gesicht benennen oder zuordnen");
    await expect(dialog.locator("[data-person-preview-image]")).toHaveAttribute("src",new RegExp("/"+secondID+"$"));
    await expect(dialog.locator("[data-person-preview-path]")).toContainText("10.10.2002 · Assisi");
    const match = page.waitForRequest(request => request.url().includes(`/faces/${secondID}/suggestions`));
    await dialog.getByRole("button", { name: "Ähnliche benannte Personen suchen" }).click(); await match;
    await dialog.getByRole("button", {name:"Abbrechen",exact:true}).click();
    const preserved = await cards.first().locator("img").elementHandle();
    await cards.nth(1).getByLabel("Auswählen", {exact:true}).check();
    await page.getByRole("button", {name:"Auswahl benennen / zuordnen",exact:true}).click();
    await dialog.getByRole("combobox").fill("Ada");
    await dialog.getByRole("button", {name:"Auswahl benennen",exact:true}).click();
    await expect(dialog).not.toBeVisible();
    await expect(cards).toHaveCount(1);
    expect(await preserved.evaluate(img=>img.isConnected)).toBe(true);
    await expect(page.getByRole("heading",{name:"Testgruppe",exact:true})).toBeVisible();
    const result = (await (await context.request.get(baseURL+"/photos/people?format=json")).json()).people;
    expect(result.find(p=>p.name==="Ada").count).toBe(1);
    // A lost response must lock the remaining card until the state is re-read.
    let writes=0;
    await page.route("**/photos/faces/edit", async route => { writes++; await route.fetch(); await route.abort(); });
    await cards.locator("[data-detail-ignore-face]").click();
    await expect(page.locator("[data-detail-retry]")).toBeVisible();
    await expect(cards.locator("[data-detail-ignore-face]")).toBeDisabled();
    await page.locator("[data-detail-retry]").click();
    await expect(cards).toHaveCount(0);
    expect(writes).toBe(1);
    await expect(page.locator("[data-detail-selection]")).toBeHidden();
    expect(errors).toEqual([]);
    const reader = await browser.newContext({ httpCredentials:{username:"reader",password:"secret"} });
    const readerPage=await reader.newPage();
    await readerPage.goto(baseURL + "/login");
    await readerPage.getByLabel("Benutzername").fill("reader");
    await readerPage.locator('input[name="password"]').fill("secret");
    await readerPage.getByRole("button", {name:"Anmelden",exact:true}).click();
    await readerPage.goto(baseURL+"/photos/people/"+result.find(p=>p.name==="Ada").id);
    await expect(readerPage.locator("[data-detail-edit-group], [data-detail-edit-face], [data-detail-ignore-face], [name=face_id], [data-person-dialog]")).toHaveCount(0);
    await expect(readerPage.locator("[data-photo-item]")).toHaveCount(1);
    await reader.close();
  } finally {await context.close();}
});
