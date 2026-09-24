import { expect, test } from "@playwright/test";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { startBearStack, stopBearStack, freePort } from "./server-fixture.mjs";
import { nextcloudFixture } from "./nextcloud-fixture.mjs";

const folder = "2026/20260613-Geburtstag_Thomas";
const png = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+ip1sAAAAASUVORK5CYII=",
  "base64",
);
let fixture, cloud, server;
test.beforeAll(async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "bearstack-transfers-")),
    port = await freePort(),
    photos = path.join(root, "photos");
  await mkdir(path.join(photos, folder, "Unter_Ordner"), { recursive: true });
  for (const [name, data] of [
    ["root.png", png],
    [folder + "/one.png", png],
    [folder + "/two.png", png],
    [folder + "/Unter_Ordner/movie.mp4", Buffer.from("test video")],
    [folder + "/one.png.xmp", Buffer.from("sidecar")],
  ])
    await writeFile(path.join(photos, name), data, { mode: 0o444 });
  fixture = {
    root,
    photos,
    configPath: path.join(root, "config.json"),
    baseURL: `http://127.0.0.1:${port}`,
  };
  await writeFile(
    fixture.configPath,
    JSON.stringify({
      addr: `127.0.0.1:${port}`,
      data_dir: path.join(root, "data"),
      auth: {
        credentials: [
          { username: "admin", password: "secret", role: "admin" },
          { username: "reader", password: "secret", role: "photos_read" },
        ],
      },
      photos: {
        enabled: true,
        root_dir: photos,
        data_dir: path.join(root, "photo-data"),
        page_size: 20,
      },
    }),
  );
  cloud = await nextcloudFixture(root);
  for (const user of ["user-1", "user-2"]) {
    for (let i = 0; i < 35; i++)
      cloud.directories.add(
        `/remote.php/dav/files/${user}/Ordner ${String(i).padStart(2, "0")}`,
      );
    cloud.directories.add(
      `/remote.php/dav/files/${user}/Ordner 00/Unterordner`,
    );
  }
  server = await startBearStack(
    fixture,
    { username: "admin", password: "secret" },
    { SSL_CERT_FILE: cloud.cert },
  );
});
test.afterAll(async ({}, info) => {
  info.setTimeout(75000);
  await stopBearStack(server);
  await cloud?.close();
  if (fixture) await rm(fixture.root, { recursive: true, force: true });
});
async function login(page, username = "admin") {
  await page.goto(fixture.baseURL + "/login");
  await page.locator('input[name="username"]').fill(username);
  await page.locator('input[name="password"]').fill("secret");
  await Promise.all([
    page.waitForURL((url) => url.pathname !== "/login"),
    page.getByRole("button", { name: "Anmelden", exact: true }).click(),
  ]);
}
async function addConnection(page, name) {
  await page.getByRole("button", { name: "Verbindung hinzufügen" }).click();
  const draft = page.locator(".transfer-connection").last();
  await draft.getByLabel("Name", { exact: true }).fill(name);
  await draft.getByLabel("Serveradresse (HTTPS)").fill(cloud.baseURL);
  await draft.getByLabel("Aktiviert", { exact: true }).check();
  await draft.getByRole("button", { name: "Speichern", exact: true }).click();
  const connection = page
    .locator(".transfer-connection")
    .filter({ has: page.getByRole("heading", { name, exact: true }) });
  await expect(connection).toBeVisible();
  const popupPromise = page.waitForEvent("popup");
  await connection.getByRole("button", { name: "Konto verbinden" }).click();
  const popup = await popupPromise;
  await popup.getByRole("button", { name: "Zugriff erlauben" }).click();
  await popup.close();
  await expect(connection.locator(".transfer-badge")).toHaveText("Verbunden");
  await expect(
    connection.getByRole("button", { name: "Konto verbinden" }),
  ).toHaveCount(0);
  await expect(connection.locator(".transfer-connection-info")).toContainText(
    "Zielordner erforderlich",
  );
  await connection
    .getByRole("button", { name: "Basis-Zielordner wählen" })
    .click();
  const dialog = page.locator("[data-transfer-folders]");
  await expect(dialog.locator("[data-transfer-folder-select]")).toBeDisabled();
  await dialog.getByRole("button", { name: "Dateien", exact: true }).click();
  await expect(dialog.locator(".transfer-folder-entry")).toHaveCount(35);
  await dialog.getByRole("button", { name: "Ordner 00", exact: true }).click();
  await expect(dialog.locator("[data-transfer-folder-current]")).toHaveText(
    "Ordner 00",
  );
  await dialog
    .getByRole("button", { name: "Unterordner", exact: true })
    .click();
  await expect(dialog.locator("[data-transfer-folder-current]")).toHaveText(
    "Unterordner",
  );
  await dialog
    .locator("[data-transfer-folder-breadcrumbs]")
    .getByRole("button", { name: "Dateien", exact: true })
    .click();
  await expect(dialog.locator(".transfer-folder-entry")).toHaveCount(35);
  await expect(dialog.locator("[data-transfer-folder-current]")).toHaveText(
    "Dateien",
  );
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(
    dialog.locator("[data-transfer-folder-select]"),
  ).toBeInViewport();
  await expect
    .poll(() => dialog.evaluate((el) => el.scrollWidth - el.clientWidth))
    .toBeLessThanOrEqual(1);
  await page.screenshot({ path: "/tmp/bearstack-transfer-folders-mobile.png" });
  await dialog.getByRole("button", { name: "Ordner 34", exact: true }).click();
  await expect(dialog.locator("[data-transfer-folder-current]")).toHaveText(
    "Ordner 34",
  );
  await dialog.getByRole("button", { name: "Zurück", exact: true }).click();
  await expect(dialog.locator("[data-transfer-folder-current]")).toHaveText(
    "Dateien",
  );
  await page.setViewportSize({ width: 1280, height: 900 });
  await dialog.getByRole("button", { name: "Diesen Ordner verwenden" }).click();
  await expect(dialog).not.toBeVisible();
  await expect(connection.locator(".transfer-connection-info")).toContainText(
    "Dateien",
  );
  await expect(
    connection.getByRole("button", { name: "Zielordner ändern" }),
  ).toBeVisible();
  await expect(
    connection.getByRole("button", { name: "Basis-Zielordner wählen" }),
  ).toHaveCount(0);
}
async function preview(page, connectionName, target) {
  const dialog = page.locator("[data-transfer-modal]");
  await expect(dialog).toBeVisible();
  if (connectionName)
    await dialog
      .locator("[data-transfer-connection]")
      .selectOption({ label: connectionName + " · Dateien" });
  if (target) await dialog.locator("[data-transfer-target]").fill(target);
  await dialog.getByRole("button", { name: "Übertragung prüfen" }).click();
  await expect(dialog.locator("[data-transfer-preview-status]")).toContainText(
    "Bestehende Dateien werden erhalten",
  );
  return dialog;
}

test("multiple connections, folder preview, selected media and queue", async ({
  browser,
}) => {
  test.setTimeout(90000);
  const context = await browser.newContext({ ignoreHTTPSErrors: true }),
    page = await context.newPage();
  page.setDefaultTimeout(10000);
  const errors = [];
  page.on("pageerror", (error) => errors.push(error.message));
  try {
    await login(page);
    await page.goto(
      fixture.baseURL + "/photos?path=" + encodeURIComponent(folder),
    );
    await expect(page.locator("[data-transfer-open]")).toHaveCount(0);
    await page.goto(fixture.baseURL + "/settings/storage-connections");
    await addConnection(page, "Familiencloud");
    await addConnection(page, "Privates Archiv");
    await expect(
      page.getByRole("heading", {
        name: "Einstellungen",
        exact: true,
        level: 1,
      }),
    ).toBeVisible();
    await expect(page.locator(".settings-section-head h2")).toHaveText(
      "Externe Speicher",
    );
    await page.screenshot({
      path: "/tmp/bearstack-transfer-settings.png",
      fullPage: true,
    });
    await page.goto(fixture.baseURL + "/photos?path=2026");
    await expect(page.locator('[data-transfer-open=""]')).toHaveCount(0);
    await page.goto(
      fixture.baseURL + "/photos?path=" + encodeURIComponent(folder),
    );
    await page
      .getByLabel("Weitere Fotoaktionen", { exact: true })
      .first()
      .click();
    await page
      .getByRole("button", { name: "Upload to Nextcloud", exact: true })
      .click();
    let dialog = await preview(page, "Familiencloud");
    await expect(dialog.locator("[data-transfer-summary]")).toContainText(
      "3 Dateien",
    );
    await expect(dialog.locator("[data-transfer-target]")).toHaveValue(
      "20260613-Geburtstag_Thomas",
    );
    await dialog.getByText("Dateien und Konflikte ansehen").click();
    await expect(dialog.locator("[data-transfer-preview-items]")).toContainText(
      "Unter Ordner",
    );
    await page.screenshot({ path: "/tmp/bearstack-transfer-preview.png" });
    const spacing = await dialog
      .locator(".app-dialog-body")
      .evaluate((el) => parseFloat(getComputedStyle(el).rowGap));
    expect(spacing).toBeGreaterThanOrEqual(16);
    await expect(dialog.locator(".transfer-metrics > div")).toHaveCount(4);
    await page.setViewportSize({ width: 390, height: 844 });
    await expect
      .poll(() => dialog.evaluate((el) => el.scrollWidth - el.clientWidth))
      .toBeLessThanOrEqual(1);
    await expect(dialog.locator("[data-transfer-start]")).toBeInViewport();
    await page.screenshot({
      path: "/tmp/bearstack-transfer-preview-mobile.png",
    });
    await page.setViewportSize({ width: 1280, height: 720 });
    await dialog
      .locator("[data-transfer-connection]")
      .selectOption({ label: "Privates Archiv · Dateien" });
    await expect(dialog.locator("[data-transfer-start]")).toBeDisabled();
    dialog = await preview(page, "Familiencloud");

    await dialog.getByRole("button", { name: "Upload starten" }).click();
    await expect.poll(() => cloud.files.size).toBe(3);
    await dialog.locator("[data-transfer-close]").click();
    // Repeating the same destination reports existing files and never uploads them again.
    await page
      .getByLabel("Weitere Fotoaktionen", { exact: true })
      .first()
      .click();
    await page.locator('[data-transfer-open=""]').click();
    dialog = await preview(page, "Familiencloud");
    await expect(dialog.locator("[data-transfer-summary]")).toContainText(
      "3 vorhanden",
    );
    await expect(dialog.locator("[data-transfer-summary]")).toContainText(
      "Zielordner ist bereits vorhanden",
    );
    await expect(dialog.locator("[data-transfer-start]")).toBeDisabled();
    await dialog.locator("[data-transfer-close]").click();
    // Explicit selections also work at the library root, below the folder-entry threshold.
    await page.goto(fixture.baseURL + "/photos");
    await page.locator("[data-photo-mode-toggle]").click();
    await page.locator("[data-photo-selection-mode]").click();
    await page
      .locator(
        '[data-photo-item][data-photo-path="root.png"] .photo-card-button',
      )
      .click();
    await page
      .getByRole("button", {
        name: "Upload to Nextcloud (Auswahl)",
        exact: true,
      })
      .click();
    dialog = await preview(page, "Privates Archiv", "Meine Auswahl");
    await expect(dialog.locator("[data-transfer-summary]")).toContainText(
      "1 Datei",
    );
    await dialog.getByRole("button", { name: "Upload starten" }).click();
    await expect
      .poll(() =>
        cloud.files.has("/remote.php/dav/files/user-2/Meine Auswahl/root.png"),
      )
      .toBe(true);
    await dialog.locator("[data-transfer-close]").click();
    await page.goto(fixture.baseURL + "/photos/uploads");
    await expect(page.locator(".transfer-job")).toHaveCount(2);
    await page
      .locator("[data-transfer-filter]")
      .selectOption({ label: "Privates Archiv" });
    await expect(page.locator(".transfer-job")).toHaveCount(1);
    await expect(page.locator(".transfer-job")).toContainText("Abgeschlossen");
    await expect(
      page.locator(".transfer-job .transfer-hint").first(),
    ).toContainText("Nextcloud");
    await page.getByText("Dateien und Fehler", { exact: true }).click();
    await expect(page.locator(".transfer-file-list li")).toHaveCount(1);
    await page.screenshot({ path: "/tmp/bearstack-transfer-queue.png" });
    await page.setViewportSize({ width: 390, height: 844 });
    await expect
      .poll(() =>
        page
          .locator(".transfer-job")
          .evaluate((el) => el.scrollWidth - el.clientWidth),
      )
      .toBeLessThanOrEqual(1);
    await page.screenshot({
      path: "/tmp/bearstack-transfer-queue-mobile.png",
      fullPage: true,
    });
    expect(cloud.log.some((r) => r.method === "DELETE")).toBe(false);
    expect(
      cloud.log
        .filter((r) => r.method === "PUT")
        .every((r) => r.headers["if-none-match"] === "*"),
    ).toBe(true);
    expect(await readFile(path.join(fixture.photos, "root.png"))).toEqual(png);
    await page.goto(fixture.baseURL + "/settings/storage-connections");
    const connection = page.locator(".transfer-connection").filter({
      has: page.getByRole("heading", {
        name: "Privates Archiv",
        exact: true,
      }),
    });
    await connection.getByLabel("Aktiviert", { exact: true }).uncheck();
    await connection
      .getByRole("button", { name: "Speichern", exact: true })
      .click();
    await expect(connection.locator(".transfer-connection-info")).toContainText(
      "Deaktiviert",
    );
    await expect(connection.locator(".transfer-badge")).toHaveText("Verbunden");
    await expect(
      connection.getByRole("button", { name: "Konto verbinden" }),
    ).toHaveCount(0);
    await expect(
      connection.getByRole("button", { name: "Zielordner ändern" }),
    ).toBeDisabled();
    await connection
      .getByRole("button", { name: "Verbindung trennen" })
      .click();
    await expect(connection.locator(".transfer-badge")).toHaveText(
      "Nicht verbunden",
    );
    await expect(
      connection.getByRole("button", { name: "Konto verbinden" }),
    ).toBeVisible();
    await expect(
      connection.getByRole("button", { name: "Verbindung trennen" }),
    ).toHaveCount(0);
    await expect(
      page
        .locator(".transfer-connection")
        .filter({
          has: page.getByRole("heading", {
            name: "Familiencloud",
            exact: true,
          }),
        })
        .locator(".transfer-badge"),
    ).toHaveText("Verbunden");
    expect(errors).toEqual([]);
  } finally {
    await context.close();
  }
});
test("non-administrators cannot access storage management or upload APIs", async ({
  page,
}) => {
  await login(page, "reader");
  for (const url of [
    "/settings/storage-connections",
    "/photos/uploads",
    "/api/transfers/v1/connections",
  ]) {
    const response = await page.goto(fixture.baseURL + url);
    expect(response.status()).toBe(403);
  }
  await page.goto(
    fixture.baseURL + "/photos?path=" + encodeURIComponent(folder),
  );
  await expect(page.locator("[data-transfer-open]")).toHaveCount(0);
});
