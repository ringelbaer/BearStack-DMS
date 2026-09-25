import { expect, test } from "@playwright/test";
import { mkdtemp, writeFile, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { startBearStack, stopBearStack, freePort } from "./server-fixture.mjs";

let root, baseURL, app;
test.beforeAll(async () => {
  root = await mkdtemp(path.join(os.tmpdir(), "bearstack-document-links-"));
  const port = await freePort(); baseURL = `http://127.0.0.1:${port}`;
  const configPath = path.join(root, "config.json");
  await writeFile(configPath, JSON.stringify({ addr: `127.0.0.1:${port}`, data_dir: path.join(root, "data"), auth: { credentials: [{ username: "editor", password: "secret", role: "documents_editor" }, { username: "reader", password: "secret", role: "documents_read" }] } }));
  app = await startBearStack({ configPath, baseURL }, { username: "editor", password: "secret" });
});
test.afterAll(async ({}, info) => { info.setTimeout(75000); await stopBearStack(app); if (root) await rm(root, { recursive: true, force: true }); });

for (const width of [1440, 320]) {
  test(`document link choices, cancellation and stale selections at ${width}px`, async ({ browser }) => {
    const context = await browser.newContext({ viewport: { width, height: 900 }, httpCredentials: { username: "editor", password: "secret", send: "always" } });
    try {
      const page = await context.newPage();
      const prefix = `links-${width}-`;
      for (let i = 0; i < 5; i++) {
        const response = await context.request.post(baseURL + "/upload", { headers: { Accept: "application/json", Origin: baseURL }, multipart: { files: { name: `${prefix}${i}.txt`, mimeType: "text/plain", buffer: Buffer.from(`${prefix}${i}`) } } });
        expect(response.status()).toBe(201);
      }
      await page.goto(baseURL + "/?q=" + prefix);
      const boxes = page.locator('input[name="ids"]');
      await expect(boxes).toHaveCount(5);
      const ids = await boxes.evaluateAll(items => items.map(item => item.value));
      const post = (selection, mode) => context.request.post(baseURL + "/documents/link", { headers: { Origin: baseURL, "Content-Type": "application/x-www-form-urlencoded" }, data: new URLSearchParams([...selection.map(id => ["ids", id]), ["link_mode", mode]]).toString(), maxRedirects: 0 });
      expect((await post([ids[0], ids[1]], "new")).status()).toBe(303);
      await page.reload();
      const openLink = async () => {
        if (!await page.locator("[data-link-submit]").isVisible()) await page.locator("[data-document-batch-menu-toggle]").click();
        await page.locator("[data-link-submit]").click();
      };
      const selectPair = async (...selected) => {
        for (const id of selected) await page.locator(`input[name="ids"][value="${id}"]`).check();
        await openLink();
      };
      const dialog = page.locator("[data-document-link-dialog]");
      await selectPair(ids[0], ids[2]);
      await expect(dialog).toBeVisible();
      const box = await dialog.boundingBox(); expect(box.x).toBeGreaterThanOrEqual(0); expect(box.x + box.width).toBeLessThanOrEqual(width);
      await dialog.getByRole("button", { name: "Abbrechen" }).click();
      await expect(dialog).toBeHidden();
      await expect(page.locator(`input[value="${ids[2]}"][name="ids"]`)).toHaveAttribute("data-linked-count", "0");
      await openLink(); await page.keyboard.press("Escape"); await expect(dialog).toBeHidden();
      await openLink();
      await dialog.getByLabel("Neue Verknüpfung erstellen").check();
      await dialog.locator("[data-document-link-confirm]").click();
      await expect(page.locator(`input[value="${ids[2]}"][name="ids"]`)).toHaveAttribute("data-linked-count", "1");
      await expect(page.locator(`input[value="${ids[1]}"][name="ids"]`)).toHaveAttribute("data-linked-count", "1");
      await selectPair(ids[0], ids[3], ids[4]);
      await expect(dialog.getByLabel("Zur vorhandenen Verknüpfung hinzufügen")).toBeChecked();
      await dialog.locator("[data-document-link-confirm]").click();
      await expect(page.locator(`input[value="${ids[3]}"][name="ids"]`)).toHaveAttribute("data-linked-count", "4");
      await expect(page.locator(`input[value="${ids[1]}"][name="ids"]`)).toHaveAttribute("data-linked-count", "3");
      expect((await post([ids[0], ids[3]], "extend")).status()).toBe(409);
      expect((await post([ids[0], ids[3]], "invalid")).status()).toBe(400);
      await page.goto(baseURL + `/documents/${ids[3]}`);
      await expect(page.locator(".linked-document-item")).toHaveCount(4);
    } finally { await context.close(); }
  });
}

test("readers cannot link documents", async ({ browser }) => {
  const context = await browser.newContext({ httpCredentials: { username: "reader", password: "secret", send: "always" } });
  try {
    const page = await context.newPage(); await page.goto(baseURL);
    await expect(page.locator("[data-link-submit]")).toHaveCount(0);
    await expect(page.locator("[data-document-link-dialog]")).toHaveCount(0);
    const response = await context.request.post(baseURL + "/documents/link", { headers: { Origin: baseURL }, form: { ids: "1", link_mode: "extend" }, maxRedirects: 0 });
    expect(response.status()).toBe(403);
  } finally { await context.close(); }
});
