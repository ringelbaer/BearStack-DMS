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
    request.resume();request.on("end",()=>{const embedding=Array(128).fill(0);embedding[calls++===2?1:0]=1;response.end(JSON.stringify({model,faces:[{x:.1,y:.1,width:.5,height:.5,confidence:.99,embedding}]}));});
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

test("face settings fit the shared desktop and mobile layout", async ({ browser }) => {
  const context = await browser.newContext();
  const page = await context.newPage();
  await page.goto(baseURL + "/login");
  await page.getByLabel("Benutzername").fill("admin");
  await page.locator('input[name="password"]').fill("secret");
  await page.getByRole("button", { name: "Anmelden" }).click();
  await page.goto(baseURL + "/settings/photos/faces");
  await expect(page.getByRole("heading", { name: "Gesichtserkennung", exact: true })).toBeVisible();
  await expect(page.getByRole("navigation", { name: "Einstellungen", exact: true }).locator("a")).toHaveCount(6);
  for (const width of [1440, 1024, 390, 320]) {
    await page.setViewportSize({ width, height: 900 });
    const layout = await page.evaluate(() => {
      const navigation = document.querySelector(".settings-tab-list").getBoundingClientRect();
      const panel = document.querySelector(".face-settings-panel").getBoundingClientRect();
      const fields = [...document.querySelectorAll('.face-settings-panel input[type="number"], .face-settings-status > div, .face-settings-panel button')];
      return {
        overflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
        navigationRight: navigation.right,
        navigationBottom: navigation.bottom,
        panelLeft: panel.left,
        panelTop: panel.top,
        contained: fields.every(field => {
          const rect = field.getBoundingClientRect();
          return rect.left >= panel.left && rect.right <= panel.right + 1;
        }),
      };
    });
    expect(layout.overflow, `horizontal overflow at ${width}px`).toBeLessThanOrEqual(1);
    expect(layout.contained, `clipped controls at ${width}px`).toBe(true);
    if (width > 1050) expect(layout.panelLeft).toBeGreaterThanOrEqual(layout.navigationRight - 1);
    else expect(layout.panelTop).toBeGreaterThanOrEqual(layout.navigationBottom - 1);
    const enabled = await page.getByLabel("Gesichtserkennung aktivieren").boundingBox();
    const firstField = await page.getByLabel("Bilder pro Lauf", { exact: true }).boundingBox();
    expect(firstField.y).toBeGreaterThan(enabled.y + enabled.height);
    await page.screenshot({ path: `/tmp/bearstack-face-settings-${width}.png`, fullPage: true });
  }
  await expect(page.locator('input[name="confirm"]')).not.toBeChecked();
  await page.getByRole("button", { name: "Gesichtsdaten löschen", exact: true }).click();
  expect(await page.locator('input[name="confirm"]').evaluate(input => input.validity.valueMissing)).toBe(true);
  await context.close();
});

test("face recognition: enable, name, move, merge, ignore and search",async({browser})=>{
  const context=await browser.newContext({httpCredentials:{username:"manager",password:"secret"}});const page=await context.newPage();
  await page.goto(baseURL+"/login");
  await page.getByLabel("Benutzername").fill("manager");await page.locator('input[name="password"]').fill("secret");await page.getByRole("button",{name:"Anmelden"}).click();
  await page.goto(baseURL+"/settings/photos/faces");
  await expect(page.getByLabel("Gesichtserkennung aktivieren")).not.toBeChecked();
  await page.getByLabel("Gesichtserkennung aktivieren").check();
  await page.getByLabel("Pause zwischen Bildern (ms)").fill("100");
  await page.getByRole("button",{name:"Speichern",exact:true}).click();
  await expect.poll(async()=>{const r=await context.request.get(baseURL+"/settings/photos/faces?format=json");return (await r.json()).status.done;}).toBe(3);
  await page.goto(baseURL+"/photos/people");await expect(page.locator("a.person-card")).toHaveCount(2);
  await page.locator("a.person-card").filter({hasText:"2 Fotos"}).click();
  for (const width of [320, 390, 640, 960, 1440]) {
    await page.setViewportSize({ width, height: 900 });
    const layout = await page.evaluate(() => {
      const forms = [...document.querySelectorAll(".people-form")];
      return {
        overflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
        cardsFit: [...document.querySelectorAll(".person-photo-button")].every(button => {
          const card = button.closest(".person-card").getBoundingClientRect();
          const caption = button.querySelector("span");
          const text = caption.getBoundingClientRect();
          const image = button.querySelector("img").getBoundingClientRect();
          return text.left >= card.left && text.right <= card.right &&
            caption.scrollWidth <= caption.clientWidth + 1 && button.scrollWidth <= button.clientWidth + 1 &&
            text.top >= image.bottom && Math.abs(image.width - image.height) <= 1;
        }),
        fits: forms.every(form => {
          const bounds = form.getBoundingClientRect();
          const children = [...form.children];
          return [...form.querySelectorAll("input:not([type=hidden]), select, button")].every(control => {
            const rect = control.getBoundingClientRect();
            return rect.width >= 180 && rect.left >= bounds.left && rect.right <= bounds.right + 1 &&
              (control.tagName !== "BUTTON" || control.scrollWidth <= control.clientWidth + 1);
          }) && children.every((child, index) => {
            if (window.innerWidth > 640 || !index) return true;
            return child.getBoundingClientRect().top >= children[index - 1].getBoundingClientRect().bottom + 5;
          });
        }),
      };
    });
    expect(layout.overflow, `page overflow at ${width}px`).toBeLessThanOrEqual(1);
    expect(layout.fits, `person form controls at ${width}px`).toBe(true);
    expect(layout.cardsFit, `long photo paths and square thumbnails at ${width}px`).toBe(true);
  }
  await page.setViewportSize({ width: 1280, height: 900 });
  const personURL = page.url();
  const lightbox = page.locator("[data-photo-lightbox]");
  const photoButtons = page.locator(".person-photo-button");
  await expect(photoButtons).toHaveCount(2);
  await photoButtons.first().hover();
  await expect(photoButtons.first()).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
  await page.screenshot({ path: "/tmp/bearstack-person-layout-fixed.png", fullPage: true });
  await photoButtons.first().locator("img").click();
  await expect(lightbox).toBeVisible();
  await expect.poll(() => lightbox.locator("[data-photo-image]").evaluate(img => img.complete && img.naturalWidth > 0)).toBe(true);
  await expect(page).toHaveURL(personURL);
  const firstTitle = await lightbox.locator("[data-photo-title]").textContent();
  await page.mouse.move(400, 8);
  await lightbox.getByRole("button", { name: "Nächstes Foto", exact: true }).click();
  await expect(lightbox.locator("[data-photo-title]")).not.toHaveText(firstTitle);
  await page.keyboard.press("Escape");
  await expect(lightbox).not.toBeVisible();
  await photoButtons.first().focus();
  await page.keyboard.press("Enter");
  await expect(lightbox).toBeVisible();
  await page.mouse.move(400, 8);
  await lightbox.getByRole("button", { name: "Foto schließen", exact: true }).click();
  await page.getByLabel("Auswählen", { exact: true }).first().check();
  await expect(lightbox).not.toBeVisible();
  await page.getByLabel("Auswählen", { exact: true }).first().uncheck();
  await page.getByLabel("Name",{exact:true}).fill("Jürgen");await page.getByRole("button",{name:"Benennen",exact:true}).click();await expect(page.getByRole("heading",{name:"Jürgen",exact:true})).toBeVisible();
  const response=await context.request.get(baseURL+"/photos/frame/items?q=person%3AJuergen");expect((await response.json()).total).toBe(2);
  await page.goto(baseURL + "/photos/people");
  await expect(page.locator("a.person-card")).toHaveCount(2);
  await page.getByLabel("Nur bekannte Personen", { exact: true }).check();
  await page.getByRole("button", { name: "Suchen", exact: true }).click();
  await expect(page.getByLabel("Nur bekannte Personen", { exact: true })).toBeChecked();
  await expect(page.locator("a.person-card")).toHaveCount(1);
  await expect(page.locator("a.person-card")).toContainText("Jürgen");
  const knownPage = await context.request.get(baseURL + "/photos/people?format=json&known=1");
  expect((await knownPage.json()).known_only).toBe(true);
  await page.locator("a.person-card").click();

  const moveForm = page.locator('form[action="/photos/faces/edit"]');
  const moveSearch = moveForm.getByRole("combobox", { name: "Zielperson suchen" });
  await moveSearch.fill("Jür");
  await expect(moveForm.getByRole("option")).toHaveCount(1);
  await moveForm.getByRole("option").click();
  await expect(moveForm.locator("[data-person-target]")).not.toHaveValue("0");
  await moveSearch.fill("");
  await expect(moveForm.locator("[data-person-target]")).toHaveValue("0");
  expect(await moveSearch.evaluate(input => input.checkValidity())).toBe(true);
  await moveSearch.press("Tab");
  await page.getByLabel("Auswählen",{exact:true}).first().check();await page.getByLabel("Name der neuen Person").fill("Marie");await page.getByRole("button",{name:"Auswahl verschieben"}).click();
  await page.locator("a.person-card").filter({hasText:"Marie"}).click();
  const mergeForm = page.locator('form[action$="/merge"]');
  const personSearch = mergeForm.getByRole("combobox", { name: "Zielperson suchen" });
  const personOptions = mergeForm.getByRole("option");
  await expect(mergeForm.locator("select")).toHaveCount(0);
  await personSearch.fill("Marie");
  await expect(mergeForm.locator("[data-person-feedback]")).toHaveText("Keine passende Person gefunden.");
  await expect(personOptions).toHaveCount(0);
  await personSearch.fill("nicht-vorhandene-person");
  await expect(mergeForm.locator("[data-person-feedback]")).toHaveText("Keine passende Person gefunden.");
  expect(await personSearch.evaluate(input => input.checkValidity())).toBe(false);
  await page.route("**/photos/people?format=json&q=Fehler", route => route.fulfill({ status: 503, body: "Unavailable" }));
  await personSearch.fill("Fehler");
  await expect(mergeForm.locator("[data-person-feedback]")).toContainText("Personen konnten nicht geladen werden.");
  await page.unroute("**/photos/people?format=json&q=Fehler");
  await personSearch.fill("Jür");
  await expect(personOptions).toHaveCount(1);
  await expect(personOptions.first()).toContainText("Jürgen");
  for (const width of [320, 1280]) {
    await page.setViewportSize({ width, height: 900 });
    const visible = await mergeForm.locator("[data-person-popup]").evaluate(popup => {
      const rect = popup.getBoundingClientRect();
      const status = popup.querySelector("[role=status]").getBoundingClientRect();
      return rect.left >= 0 && rect.right <= window.innerWidth &&
        popup.contains(document.elementFromPoint(status.left + status.width / 2, status.top + status.height / 2));
    });
    expect(visible, "suggestions must overlay the following form at " + width + "px").toBe(true);
  }
  await page.screenshot({ path: "/tmp/bearstack-person-autocomplete.png", fullPage: true });
  await personSearch.press("ArrowDown");
  await expect(personOptions.first()).toHaveAttribute("aria-selected", "true");
  await personSearch.press("Enter");
  await expect(personSearch).toHaveValue(/Jürgen \(#\d+\)/);
  await expect(personSearch).toHaveAttribute("aria-expanded", "false");
  await expect(mergeForm.locator("[data-person-target]")).not.toHaveValue("");
  expect(await personSearch.evaluate(input => input.checkValidity())).toBe(true);
  // A response to an obsolete query must not replace the current suggestions.
  let releaseStale, markStarted;
  const started = new Promise(resolve => { markStarted = resolve; });
  let markFinished;
  const finished = new Promise(resolve => { markFinished = resolve; });
  await page.route("**/photos/people?format=json&q=Veraltet", async route => {
    markStarted();
    await new Promise(resolve => { releaseStale = resolve; });
    try {
      await route.fulfill({ json: { people: [{ id: 99999, name: "Veraltete Antwort", count: 1 }] } });
    } finally {
      markFinished();
    }
  });
  await personSearch.fill("Veraltet");
  await started;
  await personSearch.fill("Jür");
  await expect(personOptions).toHaveCount(1);
  await expect(personOptions.first()).toContainText("Jürgen");
  releaseStale();
  await finished;
  await page.unroute("**/photos/people?format=json&q=Veraltet");
  await expect(personOptions.first()).toContainText("Jürgen");
  // Editing a chosen label must discard its ID; mouse selection restores it.
  await personSearch.fill("Jü");
  await expect(mergeForm.locator("[data-person-target]")).toHaveValue("");
  expect(await personSearch.evaluate(input => input.checkValidity())).toBe(false);
  await expect(personOptions).toHaveCount(1);
  await personSearch.press("Escape");
  await expect(personSearch).toHaveAttribute("aria-expanded", "false");
  await personSearch.press("ArrowDown");
  await expect(personOptions).toHaveCount(1);
  await personOptions.first().click();
  await expect(personSearch).toHaveAttribute("aria-expanded", "false");
  await expect(mergeForm.locator("[data-person-target]")).not.toHaveValue("");
  await page.getByRole("button",{name:"Gruppen zusammenführen"}).click();await expect(page.getByLabel("Auswählen",{exact:true})).toHaveCount(2);
  await page.goto(baseURL + "/photos/people?q=J%C3%BCrgen&known=1");
  await page.evaluate(() => { window.peoplePageMarker = "unchanged"; });
  const ignore = page.locator("[data-ignore-face]").first();
  const oldFace = await ignore.getAttribute("data-ignore-face");
  await page.route("**/photos/faces/edit", route => route.fulfill({ status: 503, body: "Unavailable" }));
  await ignore.click();
  await expect(page.locator("[data-people-status]")).toContainText("HTTP 503");
  await expect(ignore).toBeEnabled();
  await expect(page.locator("a.person-card")).toContainText("2 Fotos");
  await page.unroute("**/photos/faces/edit");
  const ignoredResponse = page.waitForResponse(response => response.url().endsWith("/photos/faces/edit") && response.request().method() === "POST");
  await ignore.click();
  expect(await (await ignoredResponse).json()).toEqual({ ok: true });
  await expect(page.locator("[data-people-status]")).toHaveText("Gesicht ignoriert.");
  await expect(page.locator("[data-ignore-face]")).not.toHaveAttribute("data-ignore-face", oldFace);
  await expect(page.getByLabel("Nur bekannte Personen", { exact: true })).toBeChecked();
  await expect(page).toHaveURL(baseURL + "/photos/people?q=J%C3%BCrgen&known=1");
  expect(await page.evaluate(() => window.peoplePageMarker)).toBe("unchanged");
  await expect(page.locator("a.person-card").filter({hasText:"Jürgen"})).toContainText("1 Foto");
  await page.screenshot({path:"/tmp/bearstack-people.png",fullPage:true});
  await page.setViewportSize({width:390,height:844});
  await expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth-document.documentElement.clientWidth)).toBeLessThanOrEqual(1);
  await page.locator("a.person-card").filter({hasText:"Jürgen"}).click();
  await expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth-document.documentElement.clientWidth)).toBeLessThanOrEqual(1);
  await page.screenshot({path:"/tmp/bearstack-person-mobile.png",fullPage:true});
  const reader=await browser.newContext({httpCredentials:{username:"reader",password:"secret"}});const denied=await reader.request.post(baseURL+"/photos/faces/edit",{form:{face_id:"1",action:"ignore"}});expect(denied.status()).toBe(403);const readerPage = await reader.newPage();
  await readerPage.goto(page.url());
  await expect(readerPage.getByLabel("Auswählen", { exact: true })).toHaveCount(0);
  await readerPage.locator(".person-photo-button span").first().click();
  await expect(readerPage.locator("[data-photo-lightbox]")).toBeVisible();
  await expect.poll(() => readerPage.locator("[data-photo-image]").evaluate(img => img.complete && img.naturalWidth > 0)).toBe(true);
  await readerPage.goto(baseURL + "/photos/people");
  await expect(readerPage.locator("[data-ignore-face]")).toHaveCount(0);
  await expect(readerPage.locator("[data-person-select]")).toHaveCount(0);
  expect((await reader.request.post(baseURL + "/photos/people/1/merge", { form: { target: "2" }, headers: { Origin: baseURL } })).status()).toBe(403);
  await reader.close();
  await page.goto(baseURL+"/settings/photos/faces");await page.getByRole("button",{name:"Pausieren",exact:true}).click();await expect(page.getByLabel("Gesichtserkennung aktivieren")).not.toBeChecked();
  await page.goto(baseURL + "/photos/people?q=J%C3%BCrgen&known=1");
  await page.evaluate(() => { window.peoplePageMarker = "last-face"; });
  await page.locator("[data-ignore-face]").click();
  await expect(page.locator("[data-people-status]")).toHaveText("Gesicht ignoriert.");
  await expect(page.locator("a.person-card")).toHaveCount(0);
  await expect(page.locator("[data-people-overview]")).toContainText("Keine weiteren Personen gefunden.");
  expect(await page.evaluate(() => window.peoplePageMarker)).toBe("last-face");
  // Restore the remaining named group, then merge from the overview via Ajax.
  const restore = await context.request.post(baseURL + "/photos/faces/edit", {
    form: { face_id: oldFace, action: "move", name: "Merge-Ziel" },
    headers: { Accept: "application/json", Origin: baseURL }
  });
  expect(restore.ok(), await restore.text()).toBe(true);
  await page.goto(baseURL + "/photos/people");
  await page.evaluate(() => { window.peoplePageMarker = "merge"; });
  const mergeButton = page.locator("[data-people-merge-button]");
  await expect(mergeButton).toBeHidden();
  await page.locator(".person-overview-card").filter({ hasText: "Merge-Ziel" }).locator("[data-person-select]").check();
  await expect(mergeButton).toBeHidden();
  await page.locator(".person-overview-card").filter({ hasNotText: "Merge-Ziel" }).first().locator("[data-person-select]").check();
  await expect(mergeButton).toBeVisible();
  await expect(page.locator("[data-people-merge-target]")).toContainText("Ziel: Merge-Ziel");
  const mergeBounds = await page.locator("[data-people-merge]").boundingBox();
  expect(mergeBounds.x).toBeGreaterThanOrEqual(0);
  expect(mergeBounds.x + mergeBounds.width).toBeLessThanOrEqual(390);
  expect(mergeBounds.y + mergeBounds.height).toBeLessThanOrEqual(844);
  const second = page.locator(".person-overview-card").filter({ hasNotText: "Merge-Ziel" }).first().locator("[data-person-select]");
  await second.uncheck();
  await expect(mergeButton).toBeHidden();
  await second.check();
  await page.route("**/photos/people/*/merge", route => route.fulfill({ status: 503, body: "Unavailable" }));
  await mergeButton.click();
  await expect(page.locator("[data-people-status]")).toContainText("HTTP 503");
  await expect(mergeButton).toBeEnabled();
  await expect(page.locator("[data-person-select]:checked")).toHaveCount(2);
  await page.unroute("**/photos/people/*/merge");
  await mergeButton.click();
  await expect(page.locator("[data-people-status]")).toHaveText("Personen zusammengeführt.");
  await expect(mergeButton).toBeHidden();
  await expect(page.locator("a.person-card")).toHaveCount(1);
  await expect(page.locator("a.person-card")).toContainText("Merge-Ziel");
  await expect(page.locator("a.person-card")).toContainText("2 Fotos");
  expect(await page.evaluate(() => window.peoplePageMarker)).toBe("merge");
  await context.close();
});
