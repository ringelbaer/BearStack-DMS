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

async function adminPage(browser, options = {}) {
  const context = await browser.newContext(options);
  const page = await context.newPage();
  // Other tests deliberately persist a different home page in the shared fixture.
  await page.goto(baseURL + "/login?return=%2Fdocuments");
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

  test(`system menu icons and footer links work without JavaScript (${theme})`, async ({ browser }) => {
    const { context, page } = await adminPage(browser, { javaScriptEnabled: false });
    try {
      await page.goto(baseURL + "/settings/general");
      await page.locator(`input[name="theme_mode"][value="${theme}"]`).check();
      await page.getByRole("button", { name: "Speichern", exact: true }).click();

      const menu = page.getByRole("navigation", { name: "Systemmenü", exact: true });
      const footer = page.locator(".app-footer");
      for (const width of [1440, 768, 390, 320]) {
        await page.setViewportSize({ width, height: 844 });
        await page.goto(baseURL + "/help");
        await page.getByLabel("Systemmenü öffnen", { exact: true }).click();
        await expect(menu.getByRole("link", { name: "API", exact: true })).toHaveCount(0);
        await expect(menu.getByRole("link", { name: "Log", exact: true })).toHaveCount(0);
        await expect(footer.getByRole("link", { name: "API", exact: true })).toHaveAttribute("href", "/api");
        await expect(footer.getByRole("link", { name: "Log", exact: true })).toHaveAttribute("href", "/log");
        await expect(menu.locator(".system-menu-icon svg")).toHaveCount(3);
        const layout = await page.evaluate(() => {
          const bounds = document.querySelector(".system-menu-list").getBoundingClientRect();
          const icons = [...document.querySelectorAll(".system-menu-icon")].map((element) => element.getBoundingClientRect());
          const footerItems = [...document.querySelector(".app-footer").children].map((element) => element.getBoundingClientRect());
          return {
            overflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
            contained: icons.every((rect) => rect.left >= bounds.left && rect.right <= bounds.right),
            touchTargets: icons.every((rect) => rect.width >= 44 && rect.height >= 44),
            iconRow: icons.every((rect) => Math.abs(rect.top - icons[0].top) < 1),
            footerRow: footerItems.every((rect) => Math.abs(rect.top + rect.height / 2 - footerItems[0].top - footerItems[0].height / 2) < 1),
          };
        });
        expect(layout, `${theme} at ${width}px`).toEqual({ overflow: 0, contained: true, touchTargets: true, iconRow: true, footerRow: true });
        if (width === 390 || width === 1440) {
          await page.screenshot({ path: `/tmp/bearstack-menu-${theme}-${width}.png` });
          await footer.screenshot({ path: `/tmp/bearstack-footer-${theme}-${width}.png` });
        }
      }

      await footer.getByRole("link", { name: "API", exact: true }).click();
      await expect(page).toHaveURL(baseURL + "/api");
      await expect(footer.getByRole("link", { name: "API", exact: true })).toHaveAttribute("aria-current", "page");
      await footer.getByRole("link", { name: "Log", exact: true }).click();
      await expect(page).toHaveURL(baseURL + "/log");
      await expect(footer.getByRole("link", { name: "Log", exact: true })).toHaveAttribute("aria-current", "page");
      await page.getByLabel("Systemmenü öffnen", { exact: true }).click();
      await menu.getByRole("link", { name: "Einstellungen", exact: true }).click();
      await expect(page).toHaveURL(baseURL + "/settings/general");
      await page.getByLabel("Systemmenü öffnen", { exact: true }).click();
      await expect(menu.getByRole("link", { name: "Einstellungen", exact: true })).toHaveAttribute("aria-current", "page");
      await menu.getByRole("link", { name: "Konto · admin", exact: true }).click();
      await expect(page).toHaveURL(baseURL + "/account");
      await page.getByLabel("Systemmenü öffnen", { exact: true }).click();
      await expect(menu.getByRole("link", { name: "Konto · admin", exact: true })).toHaveAttribute("aria-current", "page");
      await menu.getByRole("button", { name: "Logout", exact: true }).click();
      await expect(page).toHaveURL(/\/login/);
      await expect(page.getByLabel("Benutzername")).toBeVisible();
    } finally {
      await context.close();
    }
  });
}

test("system menu icons and footer links are reachable with the keyboard", async ({ browser }) => {
  const { context, page } = await adminPage(browser);
  try {
    await page.goto(baseURL + "/help");
    const toggle = page.getByLabel("Systemmenü öffnen", { exact: true });
    const menu = page.getByRole("navigation", { name: "Systemmenü", exact: true });
    await toggle.focus();
    await page.keyboard.press("Enter");
    await expect(menu).toBeVisible();
    const links = menu.locator("a, button");
    for (let i = 0; i < await links.count(); i += 1) {
      await page.keyboard.press("Tab");
      await expect(links.nth(i)).toBeFocused();
    }
    await expect(menu.getByRole("button", { name: "Logout", exact: true })).toBeFocused();
    await page.keyboard.press("Shift+Tab");
    await expect(menu.getByRole("link", { name: "Konto · admin", exact: true })).toBeFocused();
    await page.keyboard.press("Shift+Tab");
    await expect(menu.getByRole("link", { name: "Einstellungen", exact: true })).toBeFocused();
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(baseURL + "/settings/general");
    const api = page.locator(".app-footer").getByRole("link", { name: "API", exact: true });
    await api.focus();
    await page.keyboard.press("Tab");
    await expect(page.locator(".app-footer").getByRole("link", { name: "Log", exact: true })).toBeFocused();
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(baseURL + "/log");
  } finally {
    await context.close();
  }
});

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

test("face expert settings expand, validate and persist on mobile", async ({ browser }) => {
  const { context, page } = await adminPage(browser, { viewport: { width: 390, height: 844 } });
  try {
    await page.goto(baseURL + "/settings/photos/faces");
    const expert = page.locator("details.face-settings-expert");
    const similarity = expert.locator('[name="suggestion_similarity"]');
    await expect(similarity).not.toBeVisible();
    await expert.locator("summary").click();
    await expect(similarity).toBeVisible();
    await expect(expert.locator('input[type="number"]')).toHaveCount(6);
    await similarity.fill("0.71");
    expect(await similarity.evaluate(input => input.checkValidity())).toBe(false);
    await similarity.fill("0.6");
    await expert.locator('[name="suggestion_margin"]').fill("0.1");
    for (const width of [320, 390, 1440]) {
      await page.setViewportSize({ width, height: 844 });
      expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(1);
    }
    await page.getByRole("button", { name: "Speichern", exact: true }).click();
    await expect(page.locator(".notice").filter({ hasText: "Einstellungen gespeichert." })).toBeVisible();
    await expert.locator("summary").click();
    await expect(similarity).toHaveValue("0.6");
    await expect(expert.locator('[name="suggestion_margin"]')).toHaveValue("0.1");
    await expect(expert.locator('[name="assignment_similarity"]')).toHaveValue("0.55");
    await expect(expert.locator('[name="reconcile_similarity"]')).toHaveValue("0.62");
  } finally {
    await context.close();
  }
});
