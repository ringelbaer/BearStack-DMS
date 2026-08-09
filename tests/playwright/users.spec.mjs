import { expect, test } from "@playwright/test";
import { spawn } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import net from "node:net";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const testDir = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(testDir, "../..");

const bootstrapAdmin = "local-admin";
const bootstrapPassword = "Bootstrap-Admin!2026";
const configAdmin = "config-admin";
const configPassword = "Config-Admin!2026";
const managedUser = "managed-reader";
const managedPassword = "Managed-Reader!2026";
const managedSelfPassword = "Managed-Self!2026";
const managedResetPassword = "Managed-Reset!2026";

test("first local administrator can be bootstrapped on loopback", async ({ browser }) => {
  test.setTimeout(60_000);
  const fixture = await createUsersFixture();
  const server = await startBearStack(fixture);
  const context = await browser.newContext();
  const page = await context.newPage();
  try {
    await page.goto(`${fixture.baseURL}/settings/users`);
    await expect(page.getByRole("heading", { name: "Noch keine Nutzer" })).toBeVisible();
    await expect(page.getByText("Es gibt noch kein lokales Administratorkonto")).toBeVisible();

    await page.getByRole("link", { name: "Nutzer anlegen" }).click();
    const form = page.locator('form[action="/settings/users"]');
    await expect(form.getByText("Das erste lokale Konto wird als aktiver Administrator")).toBeVisible();
    await expect(form.locator('input[name="current_password"]')).toHaveCount(0);
    await form.locator('input[name="username"]').fill(bootstrapAdmin);
    await form.locator('input[name="new_password"]').fill(bootstrapPassword);
    await form.locator('input[name="new_password_confirmation"]').fill(bootstrapPassword);

    await Promise.all([
      page.waitForURL((url) => url.pathname === "/settings/users" && url.searchParams.has("notice")),
      form.getByRole("button", { name: "Nutzer anlegen" }).click(),
    ]);
    await expect(page.getByText(`Nutzer ${bootstrapAdmin} wurde angelegt.`)).toBeVisible();
    const card = userCard(page, bootstrapAdmin);
    await expect(card).toContainText("Administrator");
    await expect(card).toContainText("Sie");

    await page.goto(`${fixture.baseURL}/account`);
    await expect(page.getByRole("heading", { name: "Mein Konto" })).toBeVisible();
    await expect(page.locator(".account-summary")).toContainText(bootstrapAdmin);
    await expect(page.locator('form[action="/account/password"]')).toBeVisible();
  } finally {
    await context.close();
    await stopBearStack(server);
    await rm(fixture.root, { recursive: true, force: true });
  }
});

test.describe.serial("hybrid user management", () => {
  let fixture;
  let server;

  test.beforeAll(async () => {
    fixture = await createUsersFixture([
      { username: configAdmin, password: configPassword, role: "admin" },
    ]);
    server = await startBearStack(fixture, { username: configAdmin, password: configPassword });
  });

  test.afterAll(async () => {
    await stopBearStack(server);
    if (fixture?.root) {
      await rm(fixture.root, { recursive: true, force: true });
    }
  });

  test("configuration account is visible but read-only", async ({ browser }) => {
    const { context, page } = await authenticatedPage(browser, fixture, configAdmin, configPassword);
    try {
      await page.goto(`${fixture.baseURL}/settings/users`);
      const card = userCard(page, configAdmin);
      await expect(card).toContainText("Konfiguration");
      await expect(card).toContainText("Durch Konfiguration verwaltet");
      await expect(card.getByRole("link", { name: "Bearbeiten" })).toHaveCount(0);

      await page.goto(`${fixture.baseURL}/account`);
      await expect(page.locator(".account-summary")).toContainText("Konfiguration");
      await expect(page.locator('form[action="/account/password"]')).toHaveCount(0);
      await expect(page.getByText("Dieses Konto wird durch die BearStack-Konfiguration verwaltet")).toBeVisible();
    } finally {
      await context.close();
    }
  });

  test("administrator creates a database user with role and extra permission", async ({ browser }) => {
    const { context, page } = await authenticatedPage(browser, fixture, configAdmin, configPassword);
    try {
      await page.goto(`${fixture.baseURL}/settings/users/new`);
      const form = page.locator('form[action="/settings/users"]');
      await form.locator('input[name="username"]').fill(managedUser);
      await form.locator('input[name="new_password"]').fill(managedPassword);
      await form.locator('input[name="new_password_confirmation"]').fill(managedPassword);
      await form.locator('select[name="role"]').selectOption("documents_read");
      await form.locator('input[name="permissions"][value="documents.upload"]').check();
      await form.locator('input[name="current_password"]').fill(configPassword);

      await Promise.all([
        page.waitForURL((url) => url.pathname === "/settings/users" && url.searchParams.has("notice")),
        form.getByRole("button", { name: "Nutzer anlegen" }).click(),
      ]);
      await expect(page.getByText(`Nutzer ${managedUser} wurde angelegt.`)).toBeVisible();
      const card = userCard(page, managedUser);
      await expect(card).toContainText("Dokumente lesen");
      await expect(card).toContainText("Dokumente hochladen");
      await expect(card).toContainText("BearStack-Datenbank");
    } finally {
      await context.close();
    }
  });

  test("database user logs in and changes the own password", async ({ browser }) => {
    const { context, page } = await authenticatedPage(browser, fixture, managedUser, managedPassword);
    try {
      await page.goto(`${fixture.baseURL}/account`);
      await expect(page.locator(".account-summary")).toContainText(managedUser);
      await expect(page.locator(".account-summary")).toContainText("BearStack-Datenbank");
      const form = page.locator('form[action="/account/password"]');
      await form.locator('input[name="current_password"]').fill(managedPassword);
      await form.locator('input[name="new_password"]').fill(managedSelfPassword);
      await form.locator('input[name="new_password_confirmation"]').fill(managedSelfPassword);
      await Promise.all([
        page.waitForURL((url) => url.pathname === "/account" && url.searchParams.has("notice")),
        form.getByRole("button", { name: "Passwort ändern" }).click(),
      ]);
      await expect(page.locator(".notice", { hasText: "Passwort wurde geändert. Andere Sitzungen wurden beendet." }).first()).toBeVisible();
    } finally {
      await context.close();
    }

    const rejectedContext = await browser.newContext();
    const rejectedPage = await rejectedContext.newPage();
    try {
      await expectLoginFailure(rejectedPage, fixture, managedUser, managedPassword);
    } finally {
      await rejectedContext.close();
    }

    const accepted = await authenticatedPage(browser, fixture, managedUser, managedSelfPassword);
    try {
      await expect(accepted.page).not.toHaveURL(/\/login$/);
      await expect(accepted.page.locator("body")).toContainText("Dokumente");
    } finally {
      await accepted.context.close();
    }
  });

  test("administrator changes rights, resets password and disables the user", async ({ browser }) => {
    const { context, page } = await authenticatedPage(browser, fixture, configAdmin, configPassword);
    try {
      await page.goto(`${fixture.baseURL}/settings/users`);
      await userCard(page, managedUser).getByRole("link", { name: "Bearbeiten" }).click();
      const accessForm = page.locator("form.user-editor-form").first();
      await accessForm.locator('select[name="role"]').selectOption("documents_editor");
      const redundantUpload = accessForm.locator('input[name="permissions"][value="documents.upload"]');
      if ((await redundantUpload.isChecked()) && (await redundantUpload.isEnabled())) {
        await redundantUpload.uncheck();
      }
      await accessForm.locator('input[name="current_password"]').fill(configPassword);
      await submitAndWaitForNotice(page, accessForm.getByRole("button", { name: "Zugriff speichern" }), "/settings/users/");
      await expect(page.locator('select[name="role"]')).toHaveValue("documents_editor");
      await expect(page.getByText("Zugriffsrechte wurden gespeichert")).toBeVisible();

      const editorSession = await authenticatedPage(browser, fixture, managedUser, managedSelfPassword);
      try {
        await expect(editorSession.page.locator("#file-upload")).toBeVisible();
      } finally {
        await editorSession.context.close();
      }

      const passwordForm = page.locator('form[action$="/password"]');
      await passwordForm.locator('input[name="new_password"]').fill(managedResetPassword);
      await passwordForm.locator('input[name="new_password_confirmation"]').fill(managedResetPassword);
      await passwordForm.locator('input[name="current_password"]').fill(configPassword);
      await submitAndWaitForNotice(page, passwordForm.getByRole("button", { name: "Passwort zurücksetzen" }), "/settings/users/");
      await expect(page.getByText("Passwort wurde zurückgesetzt")).toBeVisible();

      const disableForm = page.locator('form[action$="/disable"]');
      await disableForm.locator('input[name="current_password"]').fill(configPassword);
      await submitAndWaitForNotice(page, disableForm.getByRole("button", { name: "Konto deaktivieren" }), "/settings/users/");
      await expect(page.getByText("Konto wurde deaktiviert")).toBeVisible();
      await expect(page.getByRole("button", { name: "Konto aktivieren" })).toBeVisible();
    } finally {
      await context.close();
    }

    const disabledContext = await browser.newContext();
    const disabledPage = await disabledContext.newPage();
    try {
      await expectLoginFailure(disabledPage, fixture, managedUser, managedResetPassword);
    } finally {
      await disabledContext.close();
    }
  });

  test("all management forms work with JavaScript disabled", async ({ browser }) => {
    const context = await browser.newContext({ javaScriptEnabled: false });
    const page = await context.newPage();
    try {
      await login(page, fixture, configAdmin, configPassword);
      await page.goto(`${fixture.baseURL}/settings/users/new`);
      const form = page.locator('form[action="/settings/users"]');
      await form.locator('input[name="username"]').fill("no-js-user");
      await form.locator('input[name="new_password"]').fill("No-JS-User!2026");
      await form.locator('input[name="new_password_confirmation"]').fill("No-JS-User!2026");
      await form.locator('select[name="role"]').selectOption("documents_read");
      await form.locator('input[name="current_password"]').fill(configPassword);
      await Promise.all([
        page.waitForURL((url) => url.pathname === "/settings/users" && url.searchParams.has("notice")),
        form.getByRole("button", { name: "Nutzer anlegen" }).click(),
      ]);
      await expect(userCard(page, "no-js-user")).toContainText("BearStack-Datenbank");

      await userCard(page, "no-js-user").getByRole("link", { name: "Bearbeiten" }).click();
      const accessForm = page.locator("form.user-editor-form").first();
      await accessForm.locator('select[name="role"]').selectOption("custom");
      const documentsRead = accessForm.locator('input[name="permissions"][value="documents.read"]');
      await expect(documentsRead).toBeEnabled();
      await documentsRead.check();
      await accessForm.locator('input[name="current_password"]').fill(configPassword);
      await Promise.all([
        page.waitForURL((url) => url.pathname.startsWith("/settings/users/") && url.searchParams.has("notice")),
        accessForm.getByRole("button", { name: "Zugriff speichern" }).click(),
      ]);
      await expect(page.locator('select[name="role"]')).toHaveValue("custom");
    } finally {
      await context.close();
    }
  });

  test("user management fits a 390px mobile viewport", async ({ browser }) => {
    const { context, page } = await authenticatedPage(browser, fixture, configAdmin, configPassword, {
      hasTouch: true,
      isMobile: true,
      viewport: { width: 390, height: 844 },
    });
    try {
      await page.goto(`${fixture.baseURL}/settings/users`);
      await expect(page.locator(".user-list")).toBeVisible();
      await expectNoHorizontalPageOverflow(page);
      const firstCardBox = await page.locator(".user-card").first().boundingBox();
      expect(firstCardBox).toBeTruthy();
      expect(firstCardBox.x).toBeGreaterThanOrEqual(0);
      expect(firstCardBox.x + firstCardBox.width).toBeLessThanOrEqual(390);

      await page.getByRole("link", { name: "Nutzer anlegen" }).click();
      await expect(page.locator(".user-editor-form")).toBeVisible();
      await expectNoHorizontalPageOverflow(page);
      for (const field of await page.locator(".user-editor-form input, .user-editor-form select").all()) {
        if (!(await field.isVisible())) continue;
        const box = await field.boundingBox();
        expect(box).toBeTruthy();
        expect(box.x).toBeGreaterThanOrEqual(0);
        expect(box.x + box.width).toBeLessThanOrEqual(390);
      }
    } finally {
      await context.close();
    }
  });
});

async function authenticatedPage(browser, fixture, username, password, options = {}) {
  const context = await browser.newContext(options);
  const page = await context.newPage();
  await login(page, fixture, username, password);
  return { context, page };
}

async function login(page, fixture, username, password) {
  await page.goto(`${fixture.baseURL}/login`);
  await page.locator('input[name="username"]').fill(username);
  await page.locator('input[name="password"]').fill(password);
  await Promise.all([
    page.waitForURL((url) => url.pathname !== "/login"),
    page.getByRole("button", { name: "Anmelden" }).click(),
  ]);
}

async function expectLoginFailure(page, fixture, username, password) {
  await page.goto(`${fixture.baseURL}/login`);
  await page.locator('input[name="username"]').fill(username);
  await page.locator('input[name="password"]').fill(password);
  await page.getByRole("button", { name: "Anmelden" }).click();
  await expect(page).toHaveURL(/\/login$/);
  await expect(page.locator(".login-error")).toContainText("Login fehlgeschlagen");
}

async function submitAndWaitForNotice(page, button, pathPrefix) {
  await Promise.all([
    page.waitForURL((url) => url.pathname.startsWith(pathPrefix) && url.searchParams.has("notice")),
    button.click(),
  ]);
}

function userCard(page, username) {
  return page.locator(".user-card", { has: page.getByRole("heading", { name: new RegExp(`^${escapeRegExp(username)}(?:\\s|$)`) }) });
}

async function expectNoHorizontalPageOverflow(page) {
  const dimensions = await page.evaluate(() => ({
    clientWidth: document.documentElement.clientWidth,
    scrollWidth: document.documentElement.scrollWidth,
  }));
  expect(dimensions.scrollWidth).toBeLessThanOrEqual(dimensions.clientWidth + 1);
}

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

async function createUsersFixture(credentials = []) {
  const root = await mkdtemp(path.join(os.tmpdir(), "bearstack-users-playwright-"));
  const dataDir = path.join(root, "data");
  await mkdir(dataDir, { recursive: true });
  const port = await freePort();
  const configPath = path.join(root, "bearstack.json");
  const config = {
    addr: `127.0.0.1:${port}`,
    data_dir: dataDir,
  };
  if (credentials.length > 0) {
    config.auth = { credentials };
  }
  await writeFile(configPath, JSON.stringify(config, null, 2));
  return {
    root,
    baseURL: `http://127.0.0.1:${port}`,
    configPath,
  };
}

async function startBearStack(fixture, healthCredential = null) {
  const child = spawn("go", ["run", "./cmd/bearstack"], {
    cwd: repoRoot,
    env: {
      ...process.env,
      BEARSTACK_CONFIG: fixture.configPath,
      GOCACHE: path.join(fixture.root, "go-cache"),
    },
    stdio: ["ignore", "pipe", "pipe"],
  });
  const state = { child, output: "" };
  child.stdout.on("data", (chunk) => {
    state.output += chunk.toString();
  });
  child.stderr.on("data", (chunk) => {
    state.output += chunk.toString();
  });
  child.once("exit", (code, signal) => {
    state.exited = { code, signal };
  });
  await waitForHealth(fixture.baseURL, state, healthCredential);
  return state;
}

async function stopBearStack(state) {
  if (!state?.child || state.exited) return;
  state.child.kill("SIGTERM");
  await Promise.race([
    new Promise((resolve) => state.child.once("exit", resolve)),
    new Promise((resolve) => setTimeout(resolve, 2_000)),
  ]);
  if (!state.exited) {
    state.child.kill("SIGKILL");
  }
}

async function waitForHealth(baseURL, state, credential) {
  const headers = {};
  if (credential) {
    headers.authorization = `Basic ${Buffer.from(`${credential.username}:${credential.password}`).toString("base64")}`;
  }
  const deadline = Date.now() + 30_000;
  while (Date.now() < deadline) {
    if (state.exited) {
      throw new Error(`BearStack exited before healthcheck: ${JSON.stringify(state.exited)}\n${state.output}`);
    }
    try {
      const response = await fetch(`${baseURL}/healthz`, { headers });
      if (response.ok) return;
    } catch (_) {
      // Server is still starting.
    }
    await new Promise((resolve) => setTimeout(resolve, 200));
  }
  throw new Error(`BearStack did not become healthy\n${state.output}`);
}

async function freePort() {
  return await new Promise((resolve, reject) => {
    const server = net.createServer();
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => {
      const address = server.address();
      server.close(() => resolve(address.port));
    });
  });
}
