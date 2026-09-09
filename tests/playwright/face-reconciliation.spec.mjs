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

test("stored face reconciliation works offline with responsive merge review and stale-form recovery", async ({ browser }) => {
  test.setTimeout(90_000);
  const context = await browser.newContext({ httpCredentials: { username: "manager", password: "secret" } });
  try {
    const page = await context.newPage();
    const pageErrors = [];
    page.on("pageerror", error => pageErrors.push(error.message));
    const settingsURL = baseURL + "/settings/photos/faces";
    const suggestionsURL = baseURL + "/photos/people/merge-suggestions";
    const status = async () => {
      const response = await context.request.get(settingsURL + "?format=json&progress=1");
      expect(response.ok()).toBe(true);
      return response.json();
    };
    const suggestions = async () => {
      const response = await context.request.get(suggestionsURL + "?format=json");
      expect(response.ok()).toBe(true);
      expect(response.headers()["cache-control"]).toBe("private, no-store");
      return (await response.json()).suggestions;
    };
    const rename = async (id, name) => {
      const response = await context.request.post(`${baseURL}/photos/people/${id}/rename`, {
        form: { name }, headers: { Accept: "application/json", Origin: baseURL },
      });
      expect(response.ok(), await response.text()).toBe(true);
    };
    const waitForReconciliation = async () => {
      await expect.poll(async () => {
        const view = await status();
        return !view.reconciliation.pending && !view.reconciliation_running;
      }, { timeout: 20_000 }).toBe(true);
    };

    await page.goto(baseURL + "/login");
    await page.getByLabel("Benutzername").fill("manager");
    await page.locator('input[name="password"]').fill("secret");
    await page.getByRole("button", { name: "Anmelden", exact: true }).click();
    await page.goto(settingsURL);
    // Seed four independent groups before allowing stored-vector reconciliation.
    await page.locator('select[name="reconcile_enabled"]').selectOption("0");
    await page.getByRole("checkbox", { name: /^Gesichtserkennung aktivieren/ }).check();
    await page.getByRole("spinbutton", { name: /^Pause zwischen Bildern/ }).fill("100");
    await page.getByRole("button", { name: "Speichern", exact: true }).click();
    await expect.poll(async () => (await status()).status.done, { timeout: 20_000 }).toBe(4);
    await page.reload();
    await page.getByRole("checkbox", { name: /^Gesichtserkennung aktivieren/ }).uncheck();
    await page.getByRole("button", { name: "Speichern", exact: true }).click();
    await expect.poll(async () => (await status()).running).toBe(false);
    expect(inferenceCalls).toBe(4);
    const peopleResponse = await context.request.get(baseURL + "/photos/people?format=json&filter=all");
    const people = (await peopleResponse.json()).people.sort((a, b) => a.id - b.id);
    expect(people).toHaveLength(4);
    await rename(people[0].id, "Ada");
    await rename(people[2].id, "Grace");
    // Physically close the service: successful reconciliation must use saved vectors.
    await closeService();

    await page.getByRole("button", { name: "Zuordnungen erneut prüfen", exact: true }).click();
    await waitForReconciliation();
    await expect.poll(async () => (await suggestions()).length).toBe(2);
    await page.reload();
    await expect(page.locator("[data-face-reconciliation-state]")).toHaveText("Abgleich abgeschlossen");
    await expect(page.getByRole("checkbox", { name: /^Gesichtserkennung aktivieren/ })).not.toBeChecked();
    expect((await status()).settings.enabled).toBe(false);
    await page.getByRole("button", { name: "Abgleich pausieren", exact: true }).click();
    await expect(page.locator("[data-face-reconciliation-state]")).toHaveText("Pausiert");
    await page.getByRole("button", { name: "Abgleich fortsetzen", exact: true }).click();
    await waitForReconciliation();
    expect((await status()).settings.reconcile_enabled).toBe(true);

    for (const width of [320, 1440]) {
      await page.setViewportSize({ width, height: 900 });
      expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth), `settings overflow at ${width}px`).toBeLessThanOrEqual(1);
    }
    await page.getByRole("link", { name: "Zusammenführungsvorschläge", exact: true }).click();
    const cards = page.locator(".face-merge-suggestion");
    await expect(cards).toHaveCount(2);
    for (const width of [320, 1440]) {
      await page.setViewportSize({ width, height: 900 });
      for (const image of await cards.locator("img").all()) {
        await image.scrollIntoViewIfNeeded();
        await expect.poll(() => image.evaluate(img => img.complete && img.naturalWidth > 0)).toBe(true);
      }
      const layout = await page.evaluate(() => ({
        overflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
        contained: [...document.querySelectorAll(".face-merge-suggestion")].every(card => {
          const bounds = card.getBoundingClientRect();
          return [...card.querySelectorAll("button, img, a")].every(element => {
            const rect = element.getBoundingClientRect();
            return rect.left >= bounds.left - 1 && rect.right <= bounds.right + 1;
          });
        }),
      }));
      expect(layout.overflow, `merge review overflow at ${width}px`).toBeLessThanOrEqual(1);
      expect(layout.contained, `merge controls clipped at ${width}px`).toBe(true);
    }

    await cards.filter({ has: page.getByText("Ada", { exact: true }) }).getByRole("button", { name: "Zusammenführen", exact: true }).click();
    await expect(cards).toHaveCount(1);
    await expect(page.locator(".notice")).toHaveText("Personengruppen zusammengeführt.");
    const adaResponse = await context.request.get(`${baseURL}/photos/people/${people[0].id}?format=json`);
    const ada = await adaResponse.json();
    expect(ada.name).toBe("Ada");
    expect(ada.faces).toHaveLength(2);

    // Keep the displayed form while another request changes its target revision.
    const pause = await context.request.post(settingsURL + "/reconcile-pause", { headers: { Origin: baseURL } });
    expect(pause.ok()).toBe(true);
    await rename(people[2].id, "Grace geändert");
    const staleResponse = page.waitForResponse(response => response.request().method() === "POST" && response.url().endsWith("/reject"));
    await cards.getByRole("button", { name: "Getrennt lassen", exact: true }).click();
    const stale = await staleResponse;
    expect(stale.status()).toBe(409);
    expect(stale.headers()["content-type"]).toContain("text/html");
    await expect(page.getByRole("heading", { name: "Fehler", exact: true })).toBeVisible();
    await expect(page.getByRole("link", { name: "Zurück", exact: true })).toHaveAttribute("href", "/photos/people/merge-suggestions");
    await page.getByRole("link", { name: "Zurück", exact: true }).click();
    await expect(page.getByRole("heading", { name: "Ähnliche Personengruppen", exact: true })).toBeVisible();

    await page.goto(settingsURL);
    await page.getByRole("button", { name: "Zuordnungen erneut prüfen", exact: true }).click();
    await waitForReconciliation();
    await expect.poll(async () => (await suggestions()).length).toBe(1);
    await page.goto(suggestionsURL);
    await expect(cards).toContainText("Grace geändert");
    await cards.getByRole("button", { name: "Getrennt lassen", exact: true }).click();
    await expect(cards).toHaveCount(0);
    await expect(page.locator(".notice")).toHaveText("Die Gruppen bleiben getrennt.");
    // A new pass must respect the explicit rejection, even with the service off.
    await page.goto(settingsURL);
    await page.getByRole("button", { name: "Zuordnungen erneut prüfen", exact: true }).click();
    await waitForReconciliation();
    expect(await suggestions()).toEqual([]);
    const finalPeople = await context.request.get(baseURL + "/photos/people?format=json&filter=all");
    expect((await finalPeople.json()).people).toHaveLength(3);
    expect(inferenceCalls).toBe(4);
    expect(pageErrors).toEqual([]);
  } finally {
    await context.close();
  }
});
