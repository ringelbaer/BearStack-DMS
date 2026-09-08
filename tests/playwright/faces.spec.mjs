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
    const enabled = await page.getByRole("checkbox", { name: /^Gesichtserkennung aktivieren/ }).boundingBox();
    const firstField = await page.getByRole("spinbutton", { name: /^Bilder pro Lauf/ }).boundingBox();
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
  await expect(page.getByRole("checkbox", { name: /^Gesichtserkennung aktivieren/ })).not.toBeChecked();
  await expect(page.getByRole("spinbutton", { name: /^Referenzen pro Person/ })).toHaveValue("30");
  await page.getByRole("spinbutton", { name: /^Referenzen pro Person/ }).fill("50");
  await page.getByRole("checkbox", { name: /^Gesichtserkennung aktivieren/ }).check();
  await page.getByRole("spinbutton", { name: /^Pause zwischen Bildern/ }).fill("100");
  await page.getByRole("button",{name:"Speichern",exact:true}).click();
  await expect.poll(async()=>{const r=await context.request.get(baseURL+"/settings/photos/faces?format=json");return (await r.json()).status.done;}).toBe(3);
  await page.reload();
  await expect(page.getByRole("spinbutton", { name: /^Referenzen pro Person/ })).toHaveValue("50");
  const settingsResponse = await context.request.get(baseURL + "/settings/photos/faces?format=json");
  expect((await settingsResponse.json()).settings.reference_limit).toBe(50);
  await page.goto(baseURL+"/photos/people");await expect(page.locator("a.person-card")).toHaveCount(2);
  for (const width of [320, 768, 1440]) {
    await page.setViewportSize({ width, height: 900 });
    for (const preview of await page.locator("a.person-card img").all()) {
      await expect.poll(() => preview.evaluate(img => img.complete && img.naturalWidth > 0)).toBe(true);
      const result = await preview.evaluate(img => {
        const rect = img.getBoundingClientRect();
        const canvas = document.createElement("canvas");
        canvas.width = img.naturalWidth; canvas.height = img.naturalHeight;
        const context = canvas.getContext("2d");
        context.drawImage(img, 0, 0);
        return { width: rect.width, height: rect.height, fit: getComputedStyle(img).objectFit,
          padding: [...context.getImageData(2, 2, 1, 1).data] };
      });
      expect(Math.abs(result.width - result.height), `square tile at ${width}px`).toBeLessThanOrEqual(1);
      expect(result.fit).toBe("contain");
      for (const [index, value] of [17, 24, 32, 255].entries()) {
        expect(Math.abs(result.padding[index] - value), "neutral padding around rectangular crop").toBeLessThanOrEqual(5);
      }
    }
  }
  await page.screenshot({ path: "/tmp/bearstack-people-aspect-fit.png", fullPage: true });
  await page.locator("a.person-card").filter({hasText:"2 Fotos"}).click();
  await expect(page.getByRole("navigation", { name: "Personenseiten" })).toHaveText("Seite 1 von 1");
  await expect(page.getByRole("navigation", { name: "Personenseiten" }).getByRole("link")).toHaveCount(0);
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
  const favoriteButton = page.locator("button[data-face-favorite]").first();
  await expect(favoriteButton).toHaveAttribute("aria-pressed", "false");
  await page.evaluate(() => { window.favoritePageMarker = true; window.favoriteImage = document.querySelector(".person-photo-button img"); });
  await favoriteButton.click();
  await expect(favoriteButton).toHaveAttribute("aria-pressed", "true");
  expect(await page.evaluate(() => window.favoritePageMarker && window.favoriteImage === document.querySelector(".person-photo-button img"))).toBe(true);
  await expect(page.locator("[data-photo-lightbox]")).not.toBeVisible();
  await expect(page.getByLabel("Auswählen", { exact: true }).first()).not.toBeChecked();
  const favoriteID = await favoriteButton.getAttribute("data-face-favorite");
  const favoriteEndpoint = baseURL + "/api/photos/labeling/v1/faces/" + favoriteID + "/favorite";
  expect((await (await context.request.get(favoriteEndpoint)).json()).favorite).toBe(true);
  await page.reload();
  await expect(favoriteButton).toHaveAttribute("aria-pressed", "true");
  await page.route("**/api/photos/labeling/v1/faces/*/favorite", route => route.fulfill({ status: 503, contentType: "application/json", body: JSON.stringify({ error: "Speichern derzeit nicht möglich" }) }));
  await favoriteButton.click();
  await expect(page.locator("[data-face-favorite-status]")).toHaveText("Speichern derzeit nicht möglich");
  await expect(favoriteButton).toBeEnabled();
  await expect(favoriteButton).toHaveAttribute("aria-pressed", "true");
  await page.unroute("**/api/photos/labeling/v1/faces/*/favorite");
  await favoriteButton.focus();
  await page.keyboard.press("Enter");
  await expect(favoriteButton).toHaveAttribute("aria-pressed", "false");
  await page.screenshot({ path: "/tmp/bearstack-person-favorites.png", fullPage: true });
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
  await expect(page.locator("button[data-face-favorite]")).toHaveCount(2);
  await page.locator("button[data-face-favorite]").first().click();
  await expect(page.locator("button[data-face-favorite]").first()).toHaveAttribute("aria-pressed", "true");
  await page.locator("button[data-face-favorite]").first().click();
  await expect(page.locator("button[data-face-favorite]").first()).toHaveAttribute("aria-pressed", "false");
  const response=await context.request.get(baseURL+"/photos/frame/items?q=person%3AJuergen");expect((await response.json()).total).toBe(2);
  await page.goto(baseURL + "/photos/people");
  await expect(page.locator("a.person-card")).toHaveCount(2);
  const personDialog = page.locator("[data-person-dialog]");
  const dialogName = personDialog.getByRole("combobox", { name: "Name", exact: true });
  const namedCard = page.locator('.person-overview-card[data-person-name="Jürgen"]');
  await expect(namedCard.locator("[data-ignore-face]")).toBeHidden();
  await expect(namedCard.locator("[data-person-edit]")).toBeHidden();
  const unnamedCard = page.locator('.person-overview-card[data-person-name=""]');
  await expect(unnamedCard.locator("[data-ignore-face]")).toBeVisible();
  await expect(unnamedCard.locator("[data-person-edit]")).toBeVisible();
  async function selectPerson(name) {
    await page.getByRole("checkbox", { name: "Person auswählen: " + name, exact: true }).check();
  }
  async function renamePerson(id, name) {
    const result = await context.request.post(baseURL + "/photos/people/" + id + "/rename", {
      form: { name }, headers: { Accept: "application/json", Origin: baseURL },
    });
    expect(result.ok(), await result.text()).toBe(true);
  }
  const peopleFilter = page.getByRole("combobox", { name: "Personenfilter", exact: true });
  const filterName = page.locator('[data-people-filter] input[name="q"]');
  const unknownID = await unnamedCard.getAttribute("data-person-id");
  await expect(peopleFilter.locator("option")).toHaveCount(4);
  await peopleFilter.selectOption("unknown");
  await page.getByRole("button", { name: "Suchen", exact: true }).click();
  await expect(page.locator("a.person-card")).toHaveCount(1);
  await expect(peopleFilter).toHaveValue("unknown");
  await expect(namedCard).toHaveCount(0);
  await expect(unnamedCard.locator("strong")).toBeHidden();
  await expect(unnamedCard.locator("a.person-card")).toHaveAccessibleName("Person anzeigen: Unbenannt");
  await unnamedCard.locator("a.person-card").click();
  await expect(page.getByRole("link", { name: "Alle Personen", exact: true })).toHaveAttribute("href", /unknown=1/);
  await page.getByRole("link", { name: "Alle Personen", exact: true }).click();
  await expect(peopleFilter).toHaveValue("unknown");
  await page.goto(baseURL + "/photos/people");
  await expect(peopleFilter).toHaveValue("unknown");
  const displayMenu = page.locator("[data-people-display]");
  const imageBeforeSettings = await unnamedCard.locator("img").getAttribute("src");
  await displayMenu.locator("summary").click();
  await expect(displayMenu.getByLabel("Fotoanzahl anzeigen", { exact: true })).toBeChecked();
  await expect(displayMenu.getByLabel("Ordnername anzeigen", { exact: true })).not.toBeChecked();
  await displayMenu.getByLabel("Ordnername anzeigen", { exact: true }).check();
  await displayMenu.getByLabel("Fotoanzahl anzeigen", { exact: true }).uncheck();
  const thumbnailSize = displayMenu.getByRole("combobox", { name: "Thumbnailgröße", exact: true });
  for (const [size, pixels] of [["s", 160], ["m", 224], ["l", 320]]) {
    await thumbnailSize.selectOption(size);
    const actualWidth = await unnamedCard.locator("img").evaluate(img => img.getBoundingClientRect().width);
    expect(actualWidth).toBeLessThanOrEqual(pixels);
    expect(actualWidth).toBeGreaterThan(pixels - 20);
  }
  await displayMenu.press("Escape");
  await expect(unnamedCard.locator("[data-person-count]")).toBeHidden();
  await expect(unnamedCard.locator("[data-person-folder]")).toBeVisible();
  await expect(unnamedCard.locator("img")).toHaveAttribute("src", imageBeforeSettings);
  await page.reload();
  await expect(unnamedCard.locator("img")).toHaveCSS("width", "320px");
  await expect(unnamedCard.locator("[data-person-count]")).toBeHidden();
  await expect(unnamedCard.locator("[data-person-folder]")).toBeVisible();
  await unnamedCard.locator("[data-person-edit]").click();
  const modalImage = personDialog.locator("[data-person-preview-image]");
  await expect.poll(() => modalImage.evaluate(img => img.complete && img.naturalWidth > 0)).toBe(true);
  await expect(personDialog.locator("[data-person-preview-box]")).toBeVisible();
  expect(await personDialog.evaluate(dialog => dialog.querySelector("[data-person-preview]").getBoundingClientRect().right <= dialog.querySelector(".person-dialog-fields").getBoundingClientRect().left)).toBe(true);
  await page.screenshot({ path: "/tmp/bearstack-people-modal-preview.png", fullPage: true });
  await dialogName.fill("Filter-Test");
  await personDialog.getByRole("option", { name: /Neu anlegen:.*Filter-Test/ }).click();
  await expect(personDialog).not.toBeVisible();
  await expect(page.locator("a.person-card")).toHaveCount(0);
  await expect(peopleFilter).toHaveValue("unknown");
  await renamePerson(unknownID, "");
  await page.reload();
  await expect(page.locator("a.person-card")).toHaveCount(1);
  await expect(unnamedCard.locator("strong")).toBeHidden();
  await expect(unnamedCard.locator("[data-person-count]")).toBeHidden();
  await displayMenu.locator("summary").click();
  await displayMenu.getByLabel("Fotoanzahl anzeigen", { exact: true }).check();
  await displayMenu.getByLabel("Ordnername anzeigen", { exact: true }).uncheck();
  await thumbnailSize.selectOption("s");
  await displayMenu.press("Escape");
  await peopleFilter.selectOption("known");
  await filterName.fill("Jürgen");
  await page.getByRole("button", { name: "Suchen", exact: true }).click();
  for (const width of [320, 1440]) {
    await page.setViewportSize({ width, height: 900 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(1);
  }
  await page.getByRole("link", { name: "Alle Filter aufheben" }).click();
  await expect(filterName).toHaveValue("");
  await expect(peopleFilter).toHaveValue("all");
  await expect(page.locator("a.person-card")).toHaveCount(2);
  await expect(page.locator(".people-pagination-current")).toHaveText("Seite 1 von 1");
  await page.goto(baseURL + "/photos/people");
  await expect(peopleFilter).toHaveValue("all");
  await expect(page.locator("a.person-card")).toHaveCount(2);
  const noJS = await browser.newContext({ javaScriptEnabled: false, storageState: await context.storageState(), httpCredentials: { username: "manager", password: "secret" } });
  const plainPage = await noJS.newPage();
  await plainPage.goto(baseURL + "/photos/people?unknown=1&known=1&ignored=1&q=NoMatch");
  await expect(plainPage.locator("a.person-card")).toHaveCount(0);
  await plainPage.getByRole("link", { name: "Alle Filter aufheben" }).click();
  await expect(plainPage.locator("a.person-card")).toHaveCount(2);
  await expect(plainPage.getByRole("combobox", { name: "Personenfilter", exact: true })).toHaveValue("all");
  await noJS.close();
  await selectPerson("Jürgen");
  const editJuergen = page.locator("[data-people-edit-button]");
  await page.evaluate(() => { window.modalPageMarker = "unchanged"; });
  await editJuergen.click();
  await expect(dialogName).toHaveValue("Jürgen");
  await dialogName.fill("");
  await expect(personDialog.locator("[data-person-feedback]")).toHaveText("Keine passende Person gefunden.");
  await expect(personDialog.getByRole("option", { name: /Unbenannt/ })).toHaveCount(0);
  await expect(personDialog.getByRole("option")).toHaveCount(0);

  await personDialog.getByRole("button", { name: "Abbrechen" }).click();
  await expect(personDialog).not.toBeVisible();
  await expect(editJuergen).toBeFocused();
  await editJuergen.click();
  await dialogName.fill("Jürgen Neu");
  await expect(personDialog.getByRole("option", { name: /Neu anlegen:/ })).toBeVisible();
  await page.route("**/photos/people/*/rename", route => route.fulfill({ status: 503, body: "Unavailable" }));
  await personDialog.getByRole("option", { name: /Neu anlegen:/ }).click();
  await expect(personDialog.locator("[data-person-dialog-status]")).toContainText("HTTP 503");
  await expect(dialogName).toHaveValue("Jürgen Neu");
  await page.unroute("**/photos/people/*/rename");
  await personDialog.getByRole("button", { name: "Benennen", exact: true }).click();
  await expect(personDialog).not.toBeVisible();
  await expect(page.locator("a.person-card").filter({ hasText: "Jürgen Neu" })).toHaveCount(1);
  await expect(page.getByRole("navigation", { name: "Personenseiten" })).toHaveText("Seite 1 von 1");
  expect(await page.evaluate(() => window.modalPageMarker)).toBe("unchanged");
  await expect(page.getByRole("button", { name: "Benennen oder zuordnen: Jürgen Neu", exact: true })).toHaveCount(0);
  await selectPerson("Jürgen Neu");
  await page.locator("[data-people-edit-button]").click();
  await dialogName.fill("Jürgen");
  await personDialog.getByRole("button", { name: "Benennen", exact: true }).click();
  await expect(personDialog).not.toBeVisible();
  await peopleFilter.selectOption("known");
  await page.getByRole("button", { name: "Suchen", exact: true }).click();
  await expect(peopleFilter).toHaveValue("known");
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
  const mergeForm = page.locator("form[data-person-create]");
  const personSearch = mergeForm.getByRole("combobox", { name: "Name", exact: true });
  const personOptions = mergeForm.getByRole("option").filter({ hasNotText: "Neu anlegen:" });
  await expect(mergeForm.locator("select")).toHaveCount(0);
  await personSearch.fill("Marie");
  await expect(mergeForm.getByRole("option", { name: /Neu anlegen:/ })).toBeVisible();
  await expect(personOptions).toHaveCount(0);
  await mergeForm.getByRole("option", { name: /Neu anlegen:/ }).click();
  await expect(personSearch).toHaveValue("Marie");
  await expect(mergeForm.getByRole("button", { name: "Benennen", exact: true })).toBeVisible();
  await personSearch.fill("nicht-vorhandene-person");
  await expect(mergeForm.getByRole("option", { name: /Neu anlegen:/ })).toBeVisible();
  expect(await personSearch.evaluate(input => input.checkValidity())).toBe(true);
  await expect(mergeForm).toHaveAttribute("action", /\/rename$/);
  await mergeForm.getByRole("option", { name: /Neu anlegen:/ }).click();
  await mergeForm.getByRole("button", { name: "Benennen", exact: true }).click();
  await expect(page.getByRole("heading", { name: "nicht-vorhandene-person", exact: true })).toBeVisible();
  await expect(page.locator(".people-management input:not([type=hidden])")).toHaveCount(1);
  await page.route("**/photos/people?format=suggestions&q=Fehler", route => route.fulfill({ status: 503, body: "Unavailable" }));
  await personSearch.fill("Fehler");
  await expect(mergeForm.locator("[data-person-feedback]")).toContainText("Personen konnten nicht geladen werden.");
  await page.unroute("**/photos/people?format=suggestions&q=Fehler");
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
  await page.route("**/photos/people?format=suggestions&q=Veraltet", async route => {
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
  await page.unroute("**/photos/people?format=suggestions&q=Veraltet");
  await expect(personOptions.first()).toContainText("Jürgen");
  // Editing a chosen label must discard its ID; mouse selection restores it.
  await personSearch.fill("Jü");
  await expect(mergeForm.locator("[data-person-target]")).toHaveValue("");
  expect(await personSearch.evaluate(input => input.checkValidity())).toBe(true);
  await expect(mergeForm).toHaveAttribute("action", /\/rename$/);
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
  await expect(page.locator("[data-ignore-face]")).toBeHidden();
  await expect(page.locator("[data-person-edit]")).toBeHidden();
  const ignorePersonID = await page.locator(".person-overview-card").getAttribute("data-person-id");
  // The overview's ignore action belongs only to unnamed groups.
  await renamePerson(ignorePersonID, "");
  await page.goto(baseURL + "/photos/people?q=&known=0&page=1");
  const ignoreCard = page.locator(`[data-person-id="${ignorePersonID}"]`);
  await page.evaluate(() => { window.peoplePageMarker = "unchanged"; });
  const ignore = ignoreCard.locator("[data-ignore-face]");
  const oldFace = await ignore.getAttribute("data-ignore-face");
  await page.route("**/photos/faces/edit", route => route.fulfill({ status: 503, body: "Unavailable" }));
  await ignore.click();
  await expect(page.locator("[data-people-status]")).toContainText("HTTP 503");
  await expect(ignore).toBeEnabled();
  await expect(ignoreCard).toContainText("2 Fotos");
  await page.unroute("**/photos/faces/edit");
  const ignoredResponse = page.waitForResponse(response => response.url().endsWith("/photos/faces/edit") && response.request().method() === "POST");
  await ignore.click();
  expect(await (await ignoredResponse).json()).toEqual({ ok: true });
  await expect(page.locator("[data-people-status]")).toHaveText("Gesicht ignoriert.");
  await expect(ignore).not.toHaveAttribute("data-ignore-face", oldFace);
  await expect(peopleFilter).toHaveValue("all");
  await expect(page).toHaveURL(baseURL + "/photos/people?q=&known=0&page=1");
  expect(await page.evaluate(() => window.peoplePageMarker)).toBe("unchanged");
  await expect(ignoreCard).toContainText("1 Foto");
  // Naming through AJAX hides both controls on the existing card immediately.
  await ignoreCard.locator("[data-person-edit]").click();
  await dialogName.fill("Jürgen");
  await personDialog.getByRole("button", { name: "Benennen", exact: true }).click();
  await expect(personDialog).not.toBeVisible();
  await expect(ignoreCard.locator("[data-ignore-face]")).toBeHidden();
  await expect(ignoreCard.locator("[data-person-edit]")).toBeHidden();
  expect(await page.evaluate(() => window.peoplePageMarker)).toBe("unchanged");
  await page.goto(baseURL + "/photos/people?q=J%C3%BCrgen&known=1");
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
  await expect(readerPage.locator("[data-person-edit], [data-person-dialog]")).toHaveCount(0);
  await expect(readerPage.locator("[data-person-select]")).toHaveCount(0);
  expect((await reader.request.post(baseURL + "/photos/people/1/merge", { form: { target: "2" }, headers: { Origin: baseURL } })).status()).toBe(403);
  await reader.close();
  await page.goto(baseURL+"/settings/photos/faces");await page.getByRole("button",{name:"Pausieren",exact:true}).click();await expect(page.getByRole("checkbox", { name: /^Gesichtserkennung aktivieren/ })).not.toBeChecked();
  await page.goto(baseURL + "/photos/people?q=J%C3%BCrgen&known=1");
  await expect(page.locator("[data-ignore-face]")).toBeHidden();
  await page.locator("a.person-card").click();
  await page.getByLabel("Auswählen", { exact: true }).check();
  await page.getByRole("button", { name: "Auswahl ignorieren", exact: true }).click();
  await page.goto(baseURL + "/photos/people?q=J%C3%BCrgen&known=1");
  await expect(page.locator("a.person-card")).toHaveCount(0);
  await expect(page.locator("[data-people-overview]")).toContainText("Keine Personen für diesen Filter gefunden.");
  // Restore the remaining named group, then merge from the overview via Ajax.
  const restore = await context.request.post(baseURL + "/photos/faces/edit", {
    form: { face_id: oldFace, action: "move", name: "Merge-Ziel" },
    headers: { Accept: "application/json", Origin: baseURL }
  });
  expect(restore.ok(), await restore.text()).toBe(true);
  // Explicitly clear the remembered name/known filters for the merge scenario.
  await page.goto(baseURL + "/photos/people?q=&known=0&page=1");
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
  // Split one face again, then assign its entire group through the overview dialog.
  const split = await context.request.post(baseURL + "/photos/faces/edit", {
    form: { face_id: oldFace, action: "move", name: "Dialog-Quelle" },
    headers: { Accept: "application/json", Origin: baseURL }
  });
  expect(split.ok()).toBe(true);
  await page.goto(baseURL + "/photos/people?q=&known=1&page=1");
  await page.evaluate(() => { window.modalPageMarker = "assign"; });
  await selectPerson("Dialog-Quelle");
  await page.locator("[data-people-edit-button]").click();
  await dialogName.fill("Merge-Ziel");
  await expect(personDialog.getByRole("option").filter({ hasNotText: "Neu anlegen:" })).toHaveCount(1);
  const suggestedPortrait = personDialog.getByRole("option", { name: /^Merge-Ziel \(#/ }).locator("img");
  await expect(suggestedPortrait).toHaveAttribute("src", await page.locator('.person-overview-card[data-person-name="Merge-Ziel"] .person-card img').getAttribute("src"));
  await expect.poll(() => suggestedPortrait.evaluate(img => img.complete && img.naturalWidth > 0)).toBe(true);
  // A failed image keeps the row and its keyboard selection usable.
  await page.route("**/photos/faces/*/thumbnail", route => route.fulfill({ status: 404, body: "unavailable" }));
  await dialogName.fill("Merge-Zie");
  await expect(suggestedPortrait).toHaveCSS("visibility", "hidden");
  await expect(personDialog.getByRole("option", { name: /^Merge-Ziel \(#/ })).toBeVisible();
  await page.unroute("**/photos/faces/*/thumbnail");
  await dialogName.press("ArrowDown");
  await page.screenshot({ path: "/tmp/bearstack-people-modal-mobile.png", fullPage: true });
  expect(await personDialog.evaluate(dialog => dialog.scrollWidth <= dialog.clientWidth + 1)).toBe(true);
  await dialogName.press("Enter");
  await expect(personDialog).not.toBeVisible();
  await expect(page.locator("a.person-card")).toHaveCount(1);
  await expect(page.locator("a.person-card")).toContainText("Merge-Ziel");
  await expect(page.locator("a.person-card")).toContainText("2 Fotos");
  await expect(peopleFilter).toHaveValue("known");
  expect(await page.evaluate(() => window.modalPageMarker)).toBe("assign");
  // Name several selected groups atomically, then assign them to a selected target.
  async function splitForBulk() {
    const result = await context.request.post(baseURL + "/photos/faces/edit", {
      form: { face_id: oldFace, action: "move", name: "Bulk-Quelle" },
      headers: { Accept: "application/json", Origin: baseURL }
    });
    expect(result.ok()).toBe(true);
    await page.goto(baseURL + "/photos/people?q=&known=1&page=1");
    await page.evaluate(() => { window.modalPageMarker = "bulk"; });
    for (const checkbox of await page.locator("[data-person-select]").all()) await checkbox.check();
    await page.locator("[data-people-edit-button]").click();
    await expect(personDialog).toContainText("2 Gruppen benennen oder zuordnen");
  }
  await splitForBulk();
  expect(await dialogName.evaluate(input => input.checkValidity())).toBe(false);
  await dialogName.fill("Gemeinsamer Name");
  await page.route("**/photos/people/*/merge", route => route.fulfill({ status: 503, body: "Unavailable" }));
  await personDialog.getByRole("button", { name: "Benennen und zusammenführen", exact: true }).click();
  await expect(personDialog.locator("[data-person-dialog-status]")).toContainText("HTTP 503");
  await expect(page.locator("[data-person-select]:checked")).toHaveCount(2);
  await page.unroute("**/photos/people/*/merge");
  await personDialog.getByRole("button", { name: "Benennen und zusammenführen", exact: true }).click();
  await expect(personDialog).not.toBeVisible();
  await expect(page.locator("a.person-card")).toHaveCount(1);
  await expect(page.locator("a.person-card")).toContainText("Gemeinsamer Name");
  await expect(page.locator("[data-person-select]:checked")).toHaveCount(0);
  expect(await page.evaluate(() => window.modalPageMarker)).toBe("bulk");
  await splitForBulk();
  await dialogName.fill("Gemeinsamer Name");
  await expect(personDialog.getByRole("option").filter({ hasNotText: "Neu anlegen:" })).toHaveCount(1);
  await page.screenshot({ path: "/tmp/bearstack-people-bulk-modal.png", fullPage: true });
  await personDialog.getByRole("option").filter({ hasNotText: "Neu anlegen:" }).click();
  await expect(personDialog).not.toBeVisible();
  await expect(page.locator("a.person-card")).toHaveCount(1);
  await expect(page.locator("a.person-card")).toContainText("Gemeinsamer Name");
  await expect(page.locator("a.person-card")).toContainText("2 Fotos");
  await expect(page.locator("[data-person-edit]")).toBeHidden();
  await expect(page.locator("[data-ignore-face]")).toBeHidden();
  expect(await page.evaluate(() => window.modalPageMarker)).toBe("bulk");
  // A saved mutation must not be retried when only refreshing the list fails.
  await selectPerson("Gemeinsamer Name");
  await page.locator("[data-people-edit-button]").click();
  await dialogName.fill("Dialog-Gespeichert");
  await dialogName.press("Tab");
  await page.route("**/photos/people?**", route => route.fulfill({ status: 503, body: "Unavailable" }));
  await personDialog.getByRole("button", { name: "Benennen", exact: true }).click();
  await expect(personDialog.locator("[data-person-dialog-status]")).toContainText("Gespeichert, aber");
  await expect(personDialog.getByRole("button", { name: "Benennen", exact: true })).toBeDisabled();
  await personDialog.getByRole("button", { name: "Abbrechen" }).click();
  await expect(personDialog).not.toBeVisible();
  await page.unroute("**/photos/people?**");
  await context.close();
});
