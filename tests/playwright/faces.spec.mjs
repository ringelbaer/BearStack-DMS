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

test("people navigation and filters leave room for results on mobile", async ({ browser }) => {
  const context = await browser.newContext();
  const page = await context.newPage();
  await page.goto(baseURL + "/login");
  await page.getByLabel("Benutzername").fill("admin");
  await page.locator('input[name="password"]').fill("secret");
  await page.getByRole("button", { name: "Anmelden" }).click();
  await page.goto(baseURL + "/photos/people?filter=unknown");
  for (const width of [320, 390, 640, 1440]) {
    await page.setViewportSize({ width, height: 800 });
    const layout = await page.evaluate(() => {
      const links = [...document.querySelectorAll(".people-page-head .page-actions a")].map(el => el.getBoundingClientRect());
      const filter = document.querySelector("[data-people-filter]").getBoundingClientRect();
      const menu = document.querySelector("[data-people-display] summary").getBoundingClientRect();
      return {
        overflow: document.documentElement.scrollWidth - innerWidth,
        paired: links[0].top === links[1].top && links[2].top === links[3].top,
        filterBottom: filter.bottom,
        menuInside: menu.top >= filter.top && menu.bottom <= filter.bottom,
        touchTargets: links.every(rect => rect.height >= 44) && menu.height >= 44,
      };
    });
    expect(layout.overflow).toBeLessThanOrEqual(1);
    expect(layout.menuInside).toBe(true);
    if (width <= 640) {
      expect(layout.paired).toBe(true);
      expect(layout.touchTargets).toBe(true);
      expect(layout.filterBottom).toBeLessThan(620);
    }
    await page.getByLabel("Anzeigeeinstellungen", { exact: true }).click();
    await page.getByLabel("Ordnername anzeigen", { exact: true }).check();
    await expect(page.locator("[data-people-view]")).toHaveAttribute("data-show-folders", "true");
    const panel = await page.locator(".people-display-options").boundingBox();
    expect(panel.x).toBeGreaterThanOrEqual(0);
    expect(panel.x + panel.width).toBeLessThanOrEqual(width);
    const controls = await page.locator(".people-display-options").evaluate(panel => {
      const labels = [...panel.querySelectorAll("label")];
      return {
        checkboxes: [...panel.querySelectorAll('input[type="checkbox"]')].map(input => input.getBoundingClientRect().width),
        oneLine: labels.slice(0, 2).every(label => label.getBoundingClientRect().height <= 34),
        contained: [...panel.querySelectorAll("input, select")].every(input => input.getBoundingClientRect().right <= panel.getBoundingClientRect().right),
      };
    });
    expect(controls.checkboxes).toEqual([18, 18]);
    expect(controls.oneLine).toBe(true);
    expect(controls.contained).toBe(true);
    const icon = await page.locator("[data-people-display] summary svg").boundingBox();
    const button = await page.locator("[data-people-display] summary").boundingBox();
    expect(Math.abs(icon.y + icon.height / 2 - button.y - button.height / 2)).toBeLessThan(1);
    await page.keyboard.press("Escape");
  }
  await page.setViewportSize({ width: 390, height: 800 });
  await page.screenshot({ path: "/tmp/bearstack-people-mobile-fixed.png", fullPage: true });
  await expect(page.locator('.people-filter-tabs [aria-current="page"]')).toHaveText("Unbenannt");
  await expect(page.locator('[data-people-filter] input[name="q"]')).toHaveCount(0);
  await context.close();
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
  test.setTimeout(60_000);
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
  await expect(page.locator('[data-face-count="recognition_matched"]')).toHaveText("1");
  await expect(page.locator('[data-face-count="recognition_new"]')).toHaveText("2");
  await expect(page.locator('[data-face-count="recognition_unknown"]')).toHaveText("0");
  await expect(page.locator('[data-face-match-percent]')).toHaveText("33.3");
  await page.locator('[data-face-count="recognition_matched"]').evaluate(el => { el.textContent = "outdated"; });
  await expect(page.locator('[data-face-count="recognition_matched"]')).toHaveText("1", { timeout: 12000 });
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
  await expect(page.locator(".people-pagination")).toBeHidden();
  await expect(page.locator(".people-pagination a")).toHaveCount(0);
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
            getComputedStyle(caption).textOverflow === "ellipsis" && button.scrollWidth <= button.clientWidth + 1 &&
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
  await page.getByRole("button", { name: "Person benennen / zuordnen", exact: true }).click();
  await page.getByRole("combobox",{name:"Name",exact:true}).fill("Jürgen");await page.getByRole("button",{name:"Benennen",exact:true}).click();await expect(page.getByRole("heading",{name:"Jürgen",exact:true})).toBeVisible();
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
  const peopleFilter = page.locator('[data-people-filter] input[name="filter"]');
  const filterName = page.locator('[data-people-filter] input[name="q"]');
  const unknownID = await unnamedCard.getAttribute("data-person-id");
  await expect(page.getByRole("navigation", { name: "Personenfilter" }).getByRole("link")).toHaveCount(4);
  await page.getByRole("link", { name: "Unbenannt", exact: true }).click();
  await expect(page.locator("a.person-card")).toHaveCount(1);
  await expect(peopleFilter).toHaveValue("unknown");
  await expect(namedCard).toHaveCount(0);
  await expect(unnamedCard.locator("strong")).toBeHidden();
  await expect(unnamedCard.locator("a.person-card")).toHaveAccessibleName("Person anzeigen: Unbenannt");
  await unnamedCard.locator("a.person-card").click();
  await expect(page.getByRole("link", { name: "← Alle Personen", exact: true })).toHaveAttribute("href", /unknown=1/);
  await page.getByRole("link", { name: "← Alle Personen", exact: true }).click();
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
  await page.getByRole("navigation", { name: "Personenfilter", exact: true }).getByRole("link", { name: "Benannt", exact: true }).click();
  await filterName.fill("Jürgen");
  await page.getByRole("button", { name: "Suchen", exact: true }).click();
  for (const width of [320, 1440]) {
    await page.setViewportSize({ width, height: 900 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(1);
  }
  await filterName.fill("");
  await page.getByRole("button", { name: "Suchen", exact: true }).click();
  await page.getByRole("navigation", { name: "Personenfilter", exact: true }).getByRole("link", { name: "Alle", exact: true }).click();
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
  await plainPage.getByRole("link", { name: "Unbenannt", exact: true }).click();
  await plainPage.getByRole("navigation", { name: "Personenfilter", exact: true }).getByRole("link", { name: "Alle", exact: true }).click();
  await expect(plainPage.locator("a.person-card")).toHaveCount(2);
  await expect(plainPage.locator('[data-people-filter] input[name="filter"]')).toHaveValue("all");
  await noJS.close();
  await selectPerson("Jürgen");
  const editJuergen = page.locator("[data-people-edit-button]");
  await page.evaluate(() => { window.modalPageMarker = "unchanged"; });
  await editJuergen.click();
  await expect(dialogName).toHaveValue("Jürgen");
  await expect(personDialog.getByRole("button", { name: "Ignorieren", exact: true })).toBeDisabled();
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
  await expect(page.locator(".people-pagination")).toContainText("Seite 1 von 1");
  expect(await page.evaluate(() => window.modalPageMarker)).toBe("unchanged");
  await expect(page.getByRole("button", { name: "Benennen oder zuordnen: Jürgen Neu", exact: true })).toHaveCount(0);
  await selectPerson("Jürgen Neu");
  await page.locator("[data-people-edit-button]").click();
  await dialogName.fill("Jürgen");
  await personDialog.getByRole("button", { name: "Benennen", exact: true }).click();
  await expect(personDialog).not.toBeVisible();
  await page.getByRole("navigation", { name: "Personenfilter", exact: true }).getByRole("link", { name: "Benannt", exact: true }).click();
  await expect(peopleFilter).toHaveValue("known");
  await expect(page.locator("a.person-card")).toHaveCount(1);
  await expect(page.locator("a.person-card")).toContainText("Jürgen");
  const knownPage = await context.request.get(baseURL + "/photos/people?format=json&known=1");
  expect((await knownPage.json()).known_only).toBe(true);
  await page.locator("a.person-card").click();

  await expect(page.locator(".person-detail-selection")).toBeHidden();
  await page.getByLabel("Auswählen", { exact: true }).first().check();
  await expect(page.locator("[data-detail-selection-count]")).toHaveText("1 ausgewählt");
  await page.getByRole("button", { name: "Auswahl benennen / zuordnen", exact: true }).click();
  const detailDialog = page.locator("[data-person-dialog]");
  await expect(detailDialog.locator("#person-dialog-title")).toHaveText("Gesicht benennen oder zuordnen");
  await detailDialog.getByRole("combobox").fill("Marie");
  await detailDialog.getByRole("button", { name: "Auswahl benennen", exact: true }).click();
  await expect(detailDialog).not.toBeVisible();
  await expect(page.getByLabel("Auswählen", { exact: true })).toHaveCount(1);
  await page.getByRole("link", { name: "← Alle Personen", exact: true }).click();
  await page.locator("a.person-card").filter({ hasText: "Marie" }).click();
  await page.getByRole("button", { name: "Person benennen / zuordnen", exact: true }).click();
  const personSearch = detailDialog.getByRole("combobox", { name: "Name", exact: true });
  await personSearch.fill("Marie");
  await expect(detailDialog.getByRole("option").filter({ hasNotText: "Neu anlegen:" })).toHaveCount(0);
  await personSearch.fill("nicht-vorhandene-person");
  await detailDialog.getByRole("button", { name: "Benennen", exact: true }).click();
  await expect(detailDialog).not.toBeVisible();
  await expect(page.getByRole("heading", { name: "nicht-vorhandene-person", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Person benennen / zuordnen", exact: true }).click();
  await page.route("**/photos/people?format=suggestions&q=Fehler", route => route.fulfill({ status: 503, body: "Unavailable" }));
  await personSearch.fill("Fehler");
  await expect(detailDialog.locator("[data-person-feedback]")).toContainText("Personen konnten nicht geladen werden");
  await page.unroute("**/photos/people?format=suggestions&q=Fehler");
  await personSearch.fill("Jür");
  await expect(detailDialog.getByRole("option").filter({ hasNotText: "Neu anlegen:" })).toHaveCount(1);
  await personSearch.press("ArrowDown");
  await personSearch.press("Enter");
  await expect(detailDialog).not.toBeVisible();
  await expect(page.getByRole("heading", { name: "Jürgen", exact: true })).toBeVisible();
  await expect(page.getByLabel("Auswählen", { exact: true })).toHaveCount(2);
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
  const faceMatch = personDialog.getByRole("button", { name: "Ähnliche benannte Personen suchen" });
  const matchURL = "**/photos/faces/*/suggestions";
  await page.route(matchURL, route => route.fulfill({ status: 503, json: { error: "Unavailable" } }), { times: 1 });
  await faceMatch.click();
  await expect(personDialog.locator("[data-person-feedback]")).toContainText("Gesichtsabgleich fehlgeschlagen");
  await expect(faceMatch).toBeEnabled();
  await faceMatch.focus();
  await faceMatch.press("Enter");
  // These fixture faces are deliberately dissimilar; the real matcher returns no candidates.
  await expect(personDialog.locator("[data-person-feedback]")).toContainText("Keine ähnlichen benannten Personen");
  const targetCard = page.locator('.person-overview-card[data-person-name="Merge-Ziel"]');
  const matchedPerson = { id: Number(await targetCard.getAttribute("data-person-id")), face_id: Number(await targetCard.getAttribute("data-face-id")), name: "Merge-Ziel", count: 1 };
  // Keep a real ReadableStream open: the first candidate must appear before EOF.
  await page.evaluate(() => {
    window.originalMatchFetch = window.fetch;
    window.fetch = function (url, options) {
      if (!String(url).match(/\/photos\/faces\/\d+\/suggestions$/)) return window.originalMatchFetch(url, options);
      const stream = new ReadableStream({
        start(controller) { window.matchStream = controller; },
        cancel() { window.matchStreamCancelled = true; }
      });
      options.signal.addEventListener("abort", () => { try { window.matchStream.error(new DOMException("Aborted", "AbortError")); } catch (_) {} });
      return Promise.resolve(new Response(stream, { headers: { "Content-Type": "application/x-ndjson" } }));
    };
  });
  await faceMatch.click();
  await expect(faceMatch).toHaveAttribute("aria-busy", "true");
  const firstUpdate = JSON.stringify({ people: [matchedPerson], has_next: false, done: false }) + "\n";
  await page.evaluate(line => { const bytes = new TextEncoder().encode(line); window.matchStream.enqueue(bytes.slice(0, 9)); window.matchStream.enqueue(bytes.slice(9)); }, firstUpdate);
  await expect(personDialog.getByRole("option", { name: /^Merge-Ziel \(#/ })).toBeVisible();
  await expect(personDialog.locator("[data-person-feedback]")).toContainText("Abgleich läuft");
  await expect(faceMatch).toHaveAttribute("aria-busy", "true");
  await dialogName.press("ArrowDown");
  const firstOption = await personDialog.getByRole("option", { name: /^Merge-Ziel \(#/ }).elementHandle();
  await page.evaluate(person => {
    window.matchStream.enqueue(new TextEncoder().encode(JSON.stringify({ people: [person], has_next: false, done: true }) + "\n"));
  }, matchedPerson);
  await expect(faceMatch).not.toHaveAttribute("aria-busy", "true");
  expect(await firstOption.evaluate(node => node.isConnected && node.getAttribute("aria-selected") === "true")).toBe(true);
  await page.evaluate(() => { window.fetch = window.originalMatchFetch; });
  await expect(personDialog.getByRole("option").filter({ hasNotText: "Neu anlegen:" })).toHaveCount(1);
  const suggestedPortrait = personDialog.getByRole("option", { name: /^Merge-Ziel \(#/ }).locator("img");
  await expect(suggestedPortrait).toHaveAttribute("src", await page.locator('.person-overview-card[data-person-name="Merge-Ziel"] .person-card img').getAttribute("src"));
  await expect.poll(() => suggestedPortrait.evaluate(img => img.complete && img.naturalWidth > 0)).toBe(true);
  await expect(personDialog.getByRole("option", { name: /Neu anlegen:/ })).toHaveCount(0);
  const nameBounds = await dialogName.boundingBox(), matchBounds = await faceMatch.boundingBox();
  expect(matchBounds.x).toBeGreaterThanOrEqual(nameBounds.x + nameBounds.width);
  expect(matchBounds.width).toBeGreaterThanOrEqual(44);
  // A late live-match response cannot overwrite a newly typed name.
  let releaseMatch, startedMatch, finishedMatch;
  const matchingFinished = new Promise(resolve => { finishedMatch = resolve; });
  const matchingStarted = new Promise(resolve => { startedMatch = resolve; });
  await page.route(matchURL, async route => {
    startedMatch();
    await new Promise(resolve => { releaseMatch = resolve; });
    try {
      await route.fulfill({ json: { people: [{ id: 99999, name: "Veralteter Abgleich", count: 1 }], has_next: false } });
    } finally { finishedMatch(); }
  });
  await faceMatch.click();
  await matchingStarted;
  await dialogName.fill("Merge-Ziel");
  await expect(personDialog.getByRole("option", { name: /^Merge-Ziel \(#/ })).toBeVisible();
  releaseMatch();
  await matchingFinished;
  await page.unroute(matchURL);
  await expect(personDialog.getByRole("option", { name: /Veralteter Abgleich/ })).toHaveCount(0);
  // A failed image keeps the row and its keyboard selection usable.
  // The portrait was already decoded above. A distinct URL guarantees a new
  // request instead of reusing the browser's in-memory image resource.
  const failedPortraitURL = new URL(await suggestedPortrait.getAttribute("src"), baseURL);
  failedPortraitURL.searchParams.set("test-missing-preview", "1");
  await page.route(failedPortraitURL.href, route => route.fulfill({ status: 404, body: "unavailable" }));
  const failedPortraitResponse = page.waitForResponse(response => response.url() === failedPortraitURL.href && response.status() === 404);
  await suggestedPortrait.evaluate((img, url) => { img.src = url; }, failedPortraitURL.href);
  await failedPortraitResponse;
  await expect(suggestedPortrait).toHaveCSS("visibility", "hidden");
  await expect(personDialog.getByRole("option", { name: /^Merge-Ziel \(#/ })).toBeVisible();
  await page.unroute(failedPortraitURL.href);
  await dialogName.press("ArrowDown");
  const caption = personDialog.locator("[data-person-preview-path]");
  await expect(caption).toHaveText(await page.locator('.person-overview-card[data-person-name="Dialog-Quelle"]').getAttribute("data-display-path"));
  await expect(caption).toContainText("Fotos / 2002 / 10.10.2002 · Assisi / BILDER LUKAS /");
  const previewBounds = await personDialog.locator("[data-person-preview]").boundingBox();
  const captionBounds = await caption.boundingBox();
  expect(captionBounds.y).toBeGreaterThanOrEqual(previewBounds.y + previewBounds.height);
  expect(await caption.evaluate(el => el.scrollWidth <= el.clientWidth + 1)).toBe(true);
  const dialogBounds = await personDialog.boundingBox();
  for (const button of await personDialog.locator("footer button").all()) {
    const bounds = await button.boundingBox();
    expect(bounds.y + bounds.height).toBeLessThanOrEqual(dialogBounds.y + dialogBounds.height);
    expect(bounds.y + bounds.height).toBeLessThanOrEqual(page.viewportSize().height);
  }
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
  // Ignoring uses the shown portrait and never submits the entered name.
  await page.goto(baseURL + "/photos/people?filter=all&page=1");
  const remainingPerson = await page.locator("[data-person-id]").first().getAttribute("data-person-id");
  await renamePerson(remainingPerson, "");
  await page.goto(baseURL + "/photos/people?filter=unknown&page=1");
  const portraitID = await page.locator("[data-people-overview] [data-face-id]").first().getAttribute("data-face-id");
  await page.locator("[data-person-edit]").first().click();
  const modalIgnore = personDialog.getByRole("button", { name: "Ignorieren", exact: true });
  await expect(modalIgnore).toBeEnabled();
  for (const width of [320, 1440]) {
    await page.setViewportSize({ width, height: 900 });
    const left = await modalIgnore.boundingBox();
    const right = await personDialog.getByRole("button", { name: "Abbrechen", exact: true }).boundingBox();
    expect(left.x + left.width).toBeLessThanOrEqual(right.x);
    expect(Math.abs(left.y - right.y)).toBeLessThan(1);
    expect(await personDialog.evaluate(dialog => dialog.scrollWidth <= dialog.clientWidth + 1)).toBe(true);
  }
  await page.screenshot({ path: "/tmp/bearstack-modal-ignore.png", fullPage: true });
  await dialogName.fill("Nicht als Namen speichern");
  await page.route("**/photos/faces/edit", route => route.fulfill({ status: 503, body: "Unavailable" }));
  await modalIgnore.click();
  await expect(personDialog.locator("[data-person-dialog-status]")).toContainText("HTTP 503");
  await expect(modalIgnore).toBeEnabled();
  await expect(personDialog).toBeVisible();
  await page.unroute("**/photos/faces/edit");
  const ignoredFromDialog = page.waitForRequest(request => request.method() === "POST" && request.url().endsWith("/photos/faces/edit"));
  await modalIgnore.click();
  const ignoreBody = new URLSearchParams((await ignoredFromDialog).postData());
  expect(ignoreBody.getAll("face_id")).toEqual([portraitID]);
  expect(ignoreBody.get("action")).toBe("ignore");
  expect(ignoreBody.has("name")).toBe(false);
  await expect(personDialog).not.toBeVisible();
  await expect(page.locator("a.person-card")).toHaveCount(1);
  await expect(page.locator("[data-people-overview] [data-person-count]")).toHaveText("1 Foto");
  await expect(page.locator("[data-person-id]").first()).toHaveAttribute("data-person-name", "");
  // Restore the ignored detection as another group, then ignore both portraits.
  const restored = await context.request.post(baseURL + "/photos/faces/edit", { form: { face_id: portraitID, action: "move" }, headers: { Accept: "application/json", Origin: baseURL } });
  expect(restored.ok()).toBe(true);
  await page.reload();
  await expect(page.locator("[data-person-select]")).toHaveCount(2);
  for (const checkbox of await page.locator("[data-person-select]").all()) await checkbox.check();
  await page.locator("[data-people-edit-button]").click();
  await expect(dialogName).toHaveValue("");
  await expect(modalIgnore).toBeEnabled();
  await page.route("**/photos/people?**", route => route.fulfill({ status: 503, body: "Unavailable" }));
  await modalIgnore.click();
  await expect(personDialog.locator("[data-person-dialog-status]")).toContainText("Gespeichert, aber");
  await expect(modalIgnore).toBeDisabled();
  await expect(personDialog.locator("[data-person-submit]")).toBeDisabled();
  await personDialog.getByRole("button", { name: "Abbrechen", exact: true }).click();
  await page.unroute("**/photos/people?**");
  await page.reload();
  await expect(page.locator("a.person-card")).toHaveCount(0);
  await context.close();
});
