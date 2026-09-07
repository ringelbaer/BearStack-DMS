import { expect, test } from "@playwright/test";
import { mkdtemp, mkdir, writeFile, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { startBearStack, stopBearStack, freePort } from "./server-fixture.mjs";

let root, baseURL, app;
const credentials = { username: "admin", password: "secret" };

test.beforeAll(async () => {
  root = await mkdtemp(path.join(os.tmpdir(), "bearstack-settings-e2e-"));
  const photos = path.join(root, "photos");
  await mkdir(photos);
  const port = await freePort();
  baseURL = `http://127.0.0.1:${port}`;
  const configPath = path.join(root, "config.json");
  await writeFile(configPath, JSON.stringify({
    addr: `127.0.0.1:${port}`,
    data_dir: path.join(root, "data"),
    auth: { credentials: [{ ...credentials, role: "admin" }] },
    photos: { enabled: true, root_dir: photos },
  }));
  app = await startBearStack({ configPath, baseURL }, credentials);
});

test.afterAll(async ({}, testInfo) => {
  testInfo.setTimeout(75_000);
  await stopBearStack(app);
  if (root) await rm(root, { recursive: true, force: true });
});

async function adminPage(browser) {
  const context = await browser.newContext();
  const page = await context.newPage();
  await page.goto(baseURL + "/login");
  await page.getByLabel("Benutzername").fill(credentials.username);
  await page.locator('input[name="password"]').fill(credentials.password);
  await page.getByRole("button", { name: "Anmelden" }).click();
  await page.waitForURL("**/documents");
  return { context, page };
}

for (const theme of ["default", "design2"]) {
  test(`settings controls and navigation fit desktop and mobile (${theme})`, async ({ browser }) => {
    const { context, page } = await adminPage(browser);
    await page.goto(baseURL + "/settings");
    const navigation = page.getByRole("navigation", { name: "Einstellungen", exact: true });
    await expect(navigation.locator("a")).toHaveCount(6);
    await navigation.getByRole("link", { name: "Allgemein", exact: true }).click();
    await page.locator(`input[name="theme_mode"][value="${theme}"]`).check();
    await page.getByRole("button", { name: "Speichern", exact: true }).click();
    await expect(page.locator("html")).toHaveAttribute("data-theme", theme);

    for (const route of ["general", "", "photos", "mail-import"]) {
      await page.goto(baseURL + "/settings" + (route ? "/" + route : ""));
      for (const width of [320, 390, 768, 1024, 1440]) {
        await page.setViewportSize({ width, height: 844 });
        const layout = await page.evaluate(() => {
          const nav = document.querySelector(".settings-tab-list");
          const panel = document.querySelector(".settings-tab-panel");
          const problems = [];
          for (const container of [nav, panel]) {
            const bounds = container.getBoundingClientRect();
            for (const element of container.querySelectorAll("a, fieldset, input:not([type=hidden]), select, textarea, button, label, .checkbox-help-label, .theme-choice-copy")) {
              const rect = element.getBoundingClientRect();
              if (!rect.width) continue;
              if (rect.left < bounds.left - 1 || rect.right > bounds.right + 1) {
                problems.push(`${element.tagName} ${element.getAttribute("name") || element.textContent.trim().slice(0, 60)} outside container`);
              }
              if (element.matches("fieldset, label, .theme-choice-copy") && element.scrollWidth > element.clientWidth + 1) {
                problems.push(`${element.tagName} ${element.textContent.trim().slice(0, 60)} clips content`);
              }
            }
          }
          return {
            overflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
            problems,
          };
        });
        expect(layout.overflow, `${route || "documents"} at ${width}px`).toBeLessThanOrEqual(1);
        expect(layout.problems, `${route || "documents"} at ${width}px`).toEqual([]);
        if (width === 390 && ["general", "photos"].includes(route)) {
          await page.screenshot({ path: `/tmp/bearstack-settings-${route}-${theme}-mobile.png`, fullPage: true });
        }
      }
    }
    await context.close();
  });
}

test("general and document forms preserve each other's settings", async ({ browser }) => {
  const { context, page } = await adminPage(browser);
  await page.goto(baseURL + "/settings");
  await expect(page.locator('input[name="app_name"]')).toHaveCount(0);
  await page.locator('input[name="desktop_preview_mode"][value="inline"]').check();
  await page.locator('input[name="folder_tag_min_documents"]').fill("13");
  await page.locator('input[name="document_cloud_enabled"]').check();
  await page.getByRole("button", { name: "Speichern", exact: true }).click();

  await page.goto(baseURL + "/settings/general");
  await expect(page.locator('[name="trash_retention_days"]')).toHaveCount(0);
  await page.getByLabel("Name", { exact: true }).fill("Familienarchiv");
  await page.locator('input[name="home_page"][value="cloud"]').check();
  await page.locator('input[name="tag_display_mode"][value="strtoupper"]').check();
  await page.getByRole("button", { name: "Speichern", exact: true }).click();
  await expect(page).toHaveURL(/\/settings\/general\?notice=/);
  await expect(page.getByRole("link", { name: "Allgemein", exact: true })).toHaveAttribute("aria-current", "page");

  await page.goto(baseURL + "/settings");
  await expect(page.locator('input[name="desktop_preview_mode"][value="inline"]')).toBeChecked();
  await expect(page.locator('input[name="folder_tag_min_documents"]')).toHaveValue("13");
  await expect(page.locator('input[name="document_cloud_enabled"]')).toBeChecked();
  await page.locator('input[name="folder_tag_min_documents"]').fill("17");
  await page.getByRole("button", { name: "Speichern", exact: true }).click();
  await page.goto(baseURL + "/settings/general");
  await expect(page.getByLabel("Name", { exact: true })).toHaveValue("Familienarchiv");
  await expect(page.locator('input[name="home_page"][value="cloud"]')).toBeChecked();
  await expect(page.locator('input[name="tag_display_mode"][value="strtoupper"]')).toBeChecked();
  await context.close();
});
