import { expect, test } from "@playwright/test";
import { startBearStack, stopBearStack, freePort } from "./server-fixture.mjs";
import http from "node:http";
import { mkdtemp, mkdir, writeFile, readFile, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

const model = "yunet-2023mar-sface-2021dec-v1";
const token = "bearstack-reconciliation-test-token-000000";
const portrait = await readFile(new URL("../../services/faces/tests/fixtures/astronaut.png", import.meta.url));
let root, baseURL, app, service;
let inferenceCalls = 0;

async function closeService() {
  if (!service?.listening) return;
  await new Promise((resolve, reject) => {
    service.close(error => error ? reject(error) : resolve());
    service.closeAllConnections();
  });
}

test.beforeAll(async () => {
  root = await mkdtemp(path.join(os.tmpdir(), "bearstack-reconciliation-e2e-"));
  const photos = path.join(root, "photos");
  await mkdir(photos);
  for (const name of ["a", "b", "c", "d"]) await writeFile(path.join(photos, `${name}.png`), portrait);
  service = http.createServer((request, response) => {
    if (request.headers.authorization !== `Bearer ${token}`) {
      response.writeHead(401).end();
      return;
    }
    response.setHeader("Content-Type", "application/json");
    if (request.url === "/health") {
      response.end(JSON.stringify({ ready: true, protocol: 1, model }));
      return;
    }
    request.resume();
    request.on("end", () => {
      const index = inferenceCalls++;
      if (index >= 4) {
        response.writeHead(503).end(JSON.stringify({ error: "Unexpected repeated inference" }));
        return;
      }
      const embedding = Array(128).fill(0);
      const axis = index < 2 ? 0 : 2;
      embedding[axis] = index % 2 ? .52 : 1;
      if (index % 2) embedding[axis + 1] = Math.sqrt(1 - .52 * .52);
      response.end(JSON.stringify({ model, faces: [{ x: .3, y: .1, width: .3, height: .35, confidence: .99, embedding,
        quality: { face_pixels: 153.6, sharpness: 100, reference_eligible: true } }] }));
    });
  });
  await new Promise((resolve, reject) => {
    service.once("error", reject);
    service.listen(0, "127.0.0.1", resolve);
  });
  const port = await freePort();
  baseURL = `http://127.0.0.1:${port}`;
  const configPath = path.join(root, "config.json");
  await writeFile(configPath, JSON.stringify({
    addr: `127.0.0.1:${port}`, data_dir: path.join(root, "data"),
    auth: { credentials: [{ username: "manager", password: "secret", role: "photos_manager" }] },
    photos: { enabled: true, root_dir: photos, face_service_url: `http://127.0.0.1:${service.address().port}`, face_service_token: token },
  }));
  app = await startBearStack({ configPath, baseURL }, { username: "manager", password: "secret" });
});

test.afterAll(async ({}, testInfo) => {
  testInfo.setTimeout(75_000);
  try {
    await stopBearStack(app);
  } finally {
    try {
      await closeService();
    } finally {
      if (root) await rm(root, { recursive: true, force: true });
    }
  }
});

test("individual actions affect only unnamed sides and retain the pair until hidden", async ({ browser }) => {
  test.setTimeout(90_000);
  const context = await browser.newContext({ httpCredentials: { username: "manager", password: "secret" } });
  try {
    const page = await context.newPage();
    const errors = []; page.on("pageerror", error => errors.push(error.message));
    const settingsURL = baseURL + "/settings/photos/faces";
    const status = async () => (await context.request.get(settingsURL + "?format=json&progress=1")).json();
    await page.goto(baseURL + "/login");
    await page.getByLabel("Benutzername").fill("manager");
    await page.locator('input[name="password"]').fill("secret");
    await page.getByRole("button", { name: "Anmelden", exact: true }).click();
    await page.goto(settingsURL);
    await page.locator('select[name="reconcile_enabled"]').selectOption("0");
    await page.getByRole("checkbox", { name: /^Gesichtserkennung aktivieren/ }).check();
    await page.getByRole("spinbutton", { name: /^Pause zwischen Bildern/ }).fill("100");
    await page.getByRole("button", { name: "Speichern", exact: true }).click();
    await expect.poll(async () => (await status()).status.done, { timeout: 20_000 }).toBe(4);
    await page.reload();
    await page.getByRole("checkbox", { name: /^Gesichtserkennung aktivieren/ }).uncheck();
    await page.getByRole("button", { name: "Speichern", exact: true }).click();
    const people = (await (await context.request.get(baseURL + "/photos/people?format=json&filter=all")).json()).people.sort((a,b) => a.id-b.id);
    expect(people).toHaveLength(4);
    expect((await context.request.post(`${baseURL}/photos/people/${people[2].id}/rename`, { headers: { Origin: baseURL }, form: { name: "Existing" } })).ok()).toBe(true);
    await page.getByRole("button", { name: "Zuordnungen erneut prüfen", exact: true }).click();
    await expect.poll(async () => { const s=await status(); return !s.reconciliation.pending && !s.reconciliation_running; }, { timeout: 20_000 }).toBe(true);
    await page.goto(baseURL + "/photos/people/merge-suggestions");
    const cards = page.locator(".face-merge-suggestion");
    await expect(cards).toHaveCount(2);
    const first=cards.filter({ hasNotText: "Existing" }), second=cards.filter({ hasText: "Existing" });
    await expect(first.locator("[data-merge-ignore]")).toHaveCount(2);
    await expect(second.locator("[data-merge-ignore]")).toHaveCount(1);
    await expect(second.locator('[data-side-name="Existing"] button')).toHaveCount(0);
    for (const width of [320,390,1440]) {
      await page.setViewportSize({ width, height: 900 });
      for (const button of await cards.locator("button:visible").all()) {
        const box=await button.boundingBox(); expect(box.height).toBeGreaterThanOrEqual(44);
        const bounds=await button.locator("xpath=ancestor::section[1]").boundingBox();
        expect(box.x).toBeGreaterThanOrEqual(bounds.x-1); expect(box.x+box.width).toBeLessThanOrEqual(bounds.x+bounds.width+1);
      }
      for (const button of await cards.locator("[data-merge-ignore]").all()) {
        expect(await button.evaluate(element => {
          const text = document.createRange(); text.selectNodeContents(element);
          const style = getComputedStyle(element);
          return element.clientWidth - parseFloat(style.paddingLeft) - parseFloat(style.paddingRight) - text.getBoundingClientRect().width;
        })).toBeGreaterThanOrEqual(-1);
      }
      expect(await page.evaluate(() => document.documentElement.scrollWidth-document.documentElement.clientWidth)).toBeLessThanOrEqual(1);
      if (width === 320) await page.screenshot({path:"/private/tmp/bearstack-merge-sides-320.png",fullPage:true});
    }
    await page.evaluate(() => { window.mergeSideMarker="same document"; });
    const firstSide=first.locator("[data-merge-side]").first(), otherSide=first.locator("[data-merge-side]").last();
    const ignoredID=Number(await firstSide.getAttribute("data-merge-side")), otherID=Number(await otherSide.getAttribute("data-merge-side"));
    const detail=async id => (await context.request.get(`${baseURL}/api/photos/labeling/v1/people/${id}`)).json();
    const untouched=await detail(otherID);
    const dialog=page.getByRole("dialog");
    await otherSide.locator("[data-merge-side-name]").click();
    await expect(dialog.getByRole("combobox")).toBeEnabled();
    await dialog.getByRole("button", {name:"Abbrechen",exact:true}).click();
    await expect(first.locator("[data-merge-dismiss]")).toBeHidden();
    // The server commits, but the response is lost. Reloading resolves its receipt,
    // retains this pair, and allows editing the untouched other side.
    let ignoreWrites=0;
    await page.route("**/api/photos/labeling/v1/people/*/actions", async route => {
      if(route.request().postDataJSON().action!=="ignore") return route.continue();
      ignoreWrites++;
      expect((await route.fetch()).status()).toBe(200);
      await route.abort();
    });
    await firstSide.locator("[data-merge-ignore]").click();
    await expect(page.locator("[data-merge-status]")).toContainText("konnte nicht bestätigt werden");
    await expect(otherSide.locator("[data-merge-ignore]")).toBeDisabled();
    await page.getByRole("button", {name:"Vorschläge aktualisieren",exact:true}).click();
    await expect(first.locator("[data-merge-dismiss]")).toBeVisible();
    await expect(first.locator(".face-merge-accept")).toBeHidden();
    await expect(first.locator(".face-merge-reject")).toBeHidden();
    await expect(otherSide.locator("[data-merge-ignore]")).toBeEnabled();
    await page.screenshot({path:"/private/tmp/bearstack-merge-sides-edited.png",fullPage:true});
    expect(ignoreWrites).toBe(1);
    expect(await detail(otherID)).toEqual(untouched);
    expect((await context.request.get(`${baseURL}/api/photos/labeling/v1/people/${ignoredID}`)).status()).toBe(404);
    await otherSide.locator("[data-merge-side-name]").click();
    await dialog.getByRole("combobox").fill("Ada");
    await dialog.getByRole("button", {name:"Benennen",exact:true}).click();
    await expect(dialog).not.toBeVisible();
    await expect(cards).toHaveCount(2);
    expect((await detail(otherID)).name).toBe("Ada");
    await expect(otherSide.locator("[data-merge-side-name]")).toBeHidden();
    await first.getByRole("button", {name:"Ausblenden",exact:true}).click();
    await expect(cards).toHaveCount(1);
    const remaining=second.locator('[data-side-name=""]');
    await remaining.locator("[data-merge-side-name]").click();
    await dialog.getByRole("combobox").fill("Ada");
    await dialog.getByRole("option").filter({hasText:"Ada (#"}).click();
    await expect(dialog).not.toBeVisible();
    await expect(second.getByRole("button", {name:"Ausblenden",exact:true})).toBeVisible();
    await expect(second.locator(".face-merge-reject")).toBeHidden();
    expect((await detail(otherID)).count).toBe(2);
    await second.getByRole("button", {name:"Ausblenden",exact:true}).click();
    await expect(cards).toHaveCount(0);
    expect(await page.evaluate(() => window.mergeSideMarker)).toBe("same document");
    expect(inferenceCalls).toBe(4);
    expect(errors).toEqual([]);
  } finally { await context.close(); }
});

test("refresh retains an edited pair once even if matching recreated it in reverse", async ({ browser }) => {
  const context=await browser.newContext();
  const html=pairs => `<!doctype html><p data-merge-status hidden></p><button data-merge-refresh hidden>Vorschläge aktualisieren</button>
    <div data-merge-suggestions>${pairs.map(([id,source,target]) => `<section data-merge-id="${id}" data-source-id="${source}" data-target-id="${target}">
      ${[source,target].map(person => `<div data-merge-side="${person}" data-side-revision="1" data-side-name=""><strong>Unbenannt</strong><div class="face-merge-side-actions"><button data-merge-ignore>Ignorieren</button></div><p data-merge-side-status hidden></p></div>`).join("")}
      <form action="/photos/people/merge-suggestions/${id}/reject"><input name="source_revision" value="1"><input name="target_revision" value="1"><button>Getrennt lassen</button></form>
      <button data-merge-dismiss hidden>Ausblenden</button></section>`).join("")}</div><p data-merge-hint></p><script src="/static/app-face-merges.js" defer></script>`;
  try {
    const page=await context.newPage();
    const errors=[];page.on("pageerror",error => errors.push(error.message));
    await page.route("**/photos/people/merge-suggestions",route => route.fulfill({contentType:"text/html",body:html(route.request().isNavigationRequest() ? [[1,3,4],[2,5,6]] : [[99,4,3]])}));
    await page.route("**/api/photos/labeling/v1/session",route => route.fulfill({json:{dataset:"dataset"}}));
    await page.route("**/api/photos/labeling/v1/people/*/actions",route => {
      const body=route.request().postDataJSON();
      return route.fulfill({json:{operation_id:body.operation_id,action:body.action,source_id:3}});
    });
    await page.route("**/merge-suggestions/2/reject",route => route.fulfill({json:{ok:true}}));
    await page.goto(baseURL+"/photos/people/merge-suggestions");
    const first=page.locator('[data-merge-id="1"]');
    await first.locator('[data-merge-side="3"] button').click();
    await expect(first.getByRole("button",{name:"Ausblenden"})).toBeVisible();
    await page.locator('[data-merge-id="2"]').getByRole("button",{name:"Getrennt lassen"}).click();
    await expect(page.locator("[data-merge-suggestions]")).toHaveAttribute("aria-busy","false");
    await expect(page.locator("[data-merge-id]")).toHaveCount(1);
    await expect(first).toBeVisible();
    await expect(first.locator('[data-merge-side="4"] button')).toBeEnabled();
    await first.getByRole("button",{name:"Ausblenden"}).click();
    await expect(page.locator("[data-merge-suggestions]")).toHaveAttribute("aria-busy","false");
    await expect(page.locator("[data-merge-id]")).toHaveCount(0);
    expect(errors).toEqual([]);
  } finally {await context.close();}
});
