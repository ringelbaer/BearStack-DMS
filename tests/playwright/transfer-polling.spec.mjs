import { expect, test } from "@playwright/test";
import { readFile } from "node:fs/promises";

const script = await readFile(new URL("../../internal/server/static/app-transfers.js", import.meta.url), "utf8");
const html = `<main data-transfer-page="jobs">
  <p data-transfer-status></p><select data-transfer-filter><option value="">Alle</option></select>
  <div data-transfer-jobs></div><button data-transfer-prev>Zurück</button>
  <span data-transfer-page-number></span><button data-transfer-next>Weiter</button></main>`;
const job = () => ({ id: "j", source_name: "Fotos", connection_name: "Cloud", provider: "test",
  base: { name: "Archiv" }, target: "Reise", state: "running", total: 10, bytes: 100,
  missing: 10, missing_bytes: 100, existing: 0, conflicts: 0, failed: 0, done: 0,
  uploaded_bytes: 0, in_flight_bytes: 1 });

test("transfer polling preserves unchanged cards, backs off when idle and resumes on visibility", async ({ page }) => {
  const current = job();
  let requests = 0, fail = false;
  await page.clock.install();
  await page.route("http://bearstack.test/**", async route => {
    const pathname = new URL(route.request().url()).pathname;
    if (pathname.endsWith("/jobs")) {
      requests++;
      await route.fulfill({ status: fail ? 503 : 200, json: fail ? { error: "offline" } : [current] });
    } else if (pathname.includes("/api/")) {
      await route.fulfill({ json: [] });
    } else {
      await route.fulfill({ contentType: "text/html", body: html });
    }
  });
  await page.goto("http://bearstack.test/");
  await page.addScriptTag({ content: script });
  await expect(page.locator(".transfer-summary")).toHaveCount(1);
  await page.evaluate(() => { window.originalSummary = document.querySelector(".transfer-summary"); });
  await page.clock.runFor(2000);
  await expect.poll(() => requests).toBe(2);
  // Await the response and finally block before advancing the next timer.
  await expect(page.locator("[data-transfer-page-number]")).toHaveText("Seite 1");
  expect(await page.evaluate(() => window.originalSummary === document.querySelector(".transfer-summary"))).toBe(true);
  current.in_flight_bytes = 20;
  await page.clock.runFor(2000);
  await expect(page.locator("progress")).toHaveAttribute("value", "20");
  current.state = "complete";
  await page.clock.runFor(2000);
  await expect(page.locator('[data-state="complete"]')).toBeVisible();
  const idleRequests = requests;
  await page.clock.runFor(14000);
  expect(requests).toBe(idleRequests);
  await page.clock.runFor(1000);
  await expect.poll(() => requests).toBe(idleRequests + 1);
  await page.evaluate(() => {
    window.hiddenForTest = true;
    Object.defineProperty(document, "hidden", { get: () => window.hiddenForTest });
    document.dispatchEvent(new Event("visibilitychange"));
  });
  await page.clock.runFor(60000);
  expect(requests).toBe(idleRequests + 1);
  current.state = "running";
  await page.evaluate(() => {
    window.hiddenForTest = false;
    document.dispatchEvent(new Event("visibilitychange"));
  });
  await expect(page.locator('[data-state="running"]')).toBeVisible();
  fail = true;
  await page.clock.runFor(2000);
  await expect(page.locator("[data-transfer-status]")).toHaveText("offline");
  const failedRequests = requests;
  await page.clock.runFor(4000);
  expect(requests).toBe(failedRequests);
  fail = false;
  await page.clock.runFor(1000);
  await expect.poll(() => requests).toBe(failedRequests + 1);
  await expect(page.locator("[data-transfer-status]")).toBeEmpty();
});

test("transfer filter changes while loading discard the old response", async ({ page }) => {
  let release;
  const pending = new Promise(resolve => { release = resolve; });
  let requests = 0;
  await page.route("http://bearstack.test/**", async route => {
    const url = new URL(route.request().url());
    if (url.pathname.endsWith("/jobs")) {
      requests++;
      if (requests === 1) await pending;
      const current = job();
      current.source_name = url.searchParams.get("connection") === "new" ? "Neue Auswahl" : "Alte Auswahl";
      await route.fulfill({ json: [current] });
    } else if (url.pathname.endsWith("/connections")) {
      await route.fulfill({ json: [{ id: "new", name: "Andere Verbindung" }] });
    } else if (url.pathname.includes("/api/")) {
      await route.fulfill({ json: [] });
    } else {
      await route.fulfill({ contentType: "text/html", body: html });
    }
  });
  await page.goto("http://bearstack.test/");
  await page.addScriptTag({ content: script });
  await expect.poll(() => requests).toBe(1);
  await page.locator("select").selectOption("new");
  release();
  await expect(page.locator("h3")).toHaveText("Neue Auswahl");
  expect(requests).toBe(2);
});
