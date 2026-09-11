import { expect, test } from "@playwright/test";
import { startBearStack, stopBearStack, freePort } from "./server-fixture.mjs";
import http from "node:http";
import { mkdtemp, mkdir, writeFile, readFile, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { execFile } from "node:child_process";
import { promisify } from "node:util";

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
  root = await mkdtemp(path.join(os.tmpdir(), "bearstack-merge-confirmation-e2e-"));
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
    photos: { enabled: true, root_dir: photos, db_path: path.join(root, "photos.db"), face_service_url: `http://127.0.0.1:${service.address().port}`, face_service_token: token },
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

test("named groups require confirmation, including equal names and forms without JavaScript", async ({ browser }) => {
  test.setTimeout(90_000);
  const context = await browser.newContext({ httpCredentials: { username: "manager", password: "secret" } });
  let noScript;
  try {
    const page = await context.newPage();
    const errors = [], writes = [];
    page.on("pageerror", error => errors.push(error.message));
    page.on("request", request => { if (request.method() === "POST" && /merge-suggestions\/\d+\//.test(request.url())) writes.push(request); });
    const settingsURL = baseURL + "/settings/photos/faces", suggestionsURL = baseURL + "/photos/people/merge-suggestions";
    const status = async () => (await (await context.request.get(settingsURL + "?format=json&progress=1")).json());
    const rename = async (id, name) => {
      const response = await context.request.post(`${baseURL}/photos/people/${id}/rename`, {
        form: { name }, headers: { Accept: "application/json", Origin: baseURL },
      });
      expect(response.ok(), await response.text()).toBe(true);
    };
    const login = async target => {
      await target.goto(baseURL + "/login");
      await target.getByLabel("Benutzername").fill("manager");
      await target.locator('input[name="password"]').fill("secret");
      await target.getByRole("button", { name: "Anmelden", exact: true }).click();
    };
    await login(page);
    await page.goto(settingsURL);
    await page.locator('select[name="reconcile_enabled"]').selectOption("0");
    await page.getByRole("checkbox", { name: /^Gesichtserkennung aktivieren/ }).check();
    await page.getByRole("spinbutton", { name: /^Pause zwischen Bildern/ }).fill("100");
    await page.getByRole("button", { name: "Speichern", exact: true }).click();
    await expect.poll(async () => (await status()).status.done, { timeout: 20_000 }).toBe(4);
    await page.reload();
    await page.getByRole("checkbox", { name: /^Gesichtserkennung aktivieren/ }).uncheck();
    await page.getByRole("button", { name: "Speichern", exact: true }).click();
    await expect.poll(async () => (await status()).running).toBe(false);
    await closeService();
    const people = (await (await context.request.get(baseURL + "/photos/people?format=json&filter=all")).json()).people.sort((a, b) => a.id - b.id);
    for (const [index, name] of ['Ada <img src=x onerror="alert(1)">', 'Grace & Co.', 'Alex', 'Alex'].entries()) await rename(people[index].id, name);
    // The matcher currently proposes unnamed sources. Seed valid named pairs in
    // this isolated fixture to exercise the UI safeguard and real mutation API.
    const seedSuggestions = async () => promisify(execFile)(process.env.PYTHON || "python3", ["-c", `
import json, sqlite3, sys
with sqlite3.connect(sys.argv[1]) as db:
    people = json.loads(sys.argv[2])
    for source, target in zip(people[::2], people[1::2]):
        db.execute("""INSERT INTO photo_face_merge_suggestions
            (source_id,target_id,source_revision,target_revision,source_face_id,target_face_id,score,model)
            SELECT s.person_id,t.person_id,s.revision,t.revision,
                (SELECT min(id) FROM photo_faces WHERE person_id=s.person_id),
                (SELECT min(id) FROM photo_faces WHERE person_id=t.person_id),.52,state.model
            FROM photo_person_revisions s,photo_person_revisions t,photo_face_state state
            WHERE s.person_id=? AND t.person_id=? AND state.id=1
            ON CONFLICT(source_id,target_id) DO NOTHING""", (source,target))
`, path.join(root, "photos.db"), JSON.stringify(people.map(person => person.id))]);
    await seedSuggestions();
    await page.goto(suggestionsURL);
    const cards = page.locator("[data-merge-id]"), dialog = page.locator("[data-app-dialog]");
    await expect(cards).toHaveCount(2);
    const first = cards.filter({ hasText: "Grace & Co." }), equal = cards.filter({ hasText: "Alex" });
    const accept = first.getByRole("button", { name: "Zusammenführen", exact: true });
    const pair = await first.evaluate(card => ({ source: Number(card.dataset.sourceId), target: Number(card.dataset.targetId),
      sourceName: card.querySelector('[data-merge-side]').dataset.sideName,
      targetName: card.querySelectorAll('[data-merge-side]')[1].dataset.sideName }));
    await page.setViewportSize({ width: 320, height: 800 });
    await accept.click();
    await expect(dialog).toBeVisible();
    await expect(dialog).toContainText("Benannte Gruppen zusammenführen?");
    await expect(dialog).toContainText(`Die Gruppen „${pair.sourceName}“ und „${pair.targetName}“ sind bereits benannt.`);
    await expect(dialog).toContainText(`Der Name „${pair.targetName}“ bleibt erhalten.`);
    await expect(dialog.locator("img")).toHaveCount(0);
    expect(writes).toHaveLength(0);
    await dialog.getByRole("button", { name: "Abbrechen", exact: true }).click();
    await expect(accept).toBeEnabled();
    await expect(accept).toBeFocused();
    await expect(cards).toHaveCount(2);
    expect(writes).toHaveLength(0);
    await accept.click();
    await page.keyboard.press("Escape");
    await expect(dialog).toBeHidden();
    await expect(accept).toBeEnabled();
    expect(writes).toHaveLength(0);
    await equal.getByRole("button", { name: "Zusammenführen", exact: true }).click();
    await expect(dialog).toContainText("Die Gruppen „Alex“ und „Alex“ sind bereits benannt.");
    await dialog.getByRole("button", { name: "Abbrechen", exact: true }).click();
    expect(writes).toHaveLength(0);

    // Native HTML validation must also require consent; rejection bypasses it.
    noScript = await browser.newContext({ javaScriptEnabled: false, viewport: { width: 320, height: 800 } });
    const plain = await noScript.newPage(), plainWrites = [];
    await login(plain);
    await plain.route("**/merge-suggestions/*/*", route => {
      plainWrites.push(route.request()); return route.fulfill({ contentType: "text/html", body: "Formular empfangen" });
    });
    await plain.goto(suggestionsURL);
    let plainCard = plain.locator("[data-merge-id]").filter({ hasText: "Alex" });
    await plainCard.getByRole("button", { name: "Zusammenführen", exact: true }).click();
    expect(plainWrites).toHaveLength(0);
    await expect(plainCard.getByRole("checkbox")).toBeFocused();
    await plainCard.getByRole("checkbox").check();
    await plainCard.getByRole("button", { name: "Zusammenführen", exact: true }).click();
    await expect(plain.locator("body")).toHaveText("Formular empfangen");
    expect(plainWrites).toHaveLength(1); expect(plainWrites[0].url()).toMatch(/\/accept$/);
    await plain.goto(suggestionsURL);
    plainCard = plain.locator("[data-merge-id]").filter({ hasText: "Alex" });
    await expect(plainCard.getByRole("checkbox")).not.toBeChecked();
    await plainCard.getByRole("button", { name: "Getrennt lassen", exact: true }).click();
    await expect(plain.locator("body")).toHaveText("Formular empfangen");
    expect(plainWrites).toHaveLength(2); expect(plainWrites[1].url()).toMatch(/\/reject$/);

    // A change while confirming must still fail the original revision check.
    await accept.click();
    expect((await context.request.post(settingsURL + "/reconcile-pause", { headers: { Origin: baseURL } })).ok()).toBe(true);
    await rename(pair.target, "Neuer Zielname");
    await seedSuggestions();
    const stale = page.waitForResponse(response => response.request().method() === "POST" && response.url().endsWith("/accept"));
    await dialog.getByRole("button", { name: "Bestätigen", exact: true }).click();
    expect((await stale).status()).toBe(409);
    await expect(page.locator("[data-merge-status]")).toContainText("Personengruppen haben sich geändert");
    expect(writes).toHaveLength(1);
    await expect(page.locator("[data-merge-suggestions]")).toHaveAttribute("aria-busy", "false");
    await cards.filter({ hasText: "Neuer Zielname" }).getByRole("button", { name: "Zusammenführen", exact: true }).click();
    await expect(dialog).toContainText("Neuer Zielname");
    expect(writes).toHaveLength(1);
    const retained = await cards.filter({ hasText: "Neuer Zielname" }).locator("[data-merge-side]").last().getAttribute("data-side-name");
    await dialog.getByRole("button", { name: "Bestätigen", exact: true }).click();
    await expect(cards).toHaveCount(1);
    expect(writes).toHaveLength(2);
    const merged = (await (await context.request.get(baseURL + "/photos/people?format=json&filter=all")).json()).people;
    expect(merged.find(person => person.name === retained).count).toBe(2);
    await cards.getByRole("button", { name: "Getrennt lassen", exact: true }).click();
    await expect(cards).toHaveCount(0);
    await expect(dialog).toBeHidden();
    expect(writes).toHaveLength(3); expect(writes[2].url()).toMatch(/\/reject$/);
    expect(inferenceCalls).toBe(4); expect(errors).toEqual([]);
  } finally { await noScript?.close(); await context.close(); }
});
