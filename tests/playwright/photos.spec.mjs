import { expect, test } from "@playwright/test";
import { spawn } from "node:child_process";
import { chmod, mkdir, mkdtemp, rm, utimes, writeFile } from "node:fs/promises";
import net from "node:net";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const testDir = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(testDir, "../..");
const password = "secret";
const tinyPNG = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=",
  "base64",
);

let fixture;
let server;

test.beforeAll(async () => {
  fixture = await createPhotoFixture();
  server = await startBearStack(fixture);
});

test.afterAll(async () => {
  await stopBearStack(server);
  if (fixture?.root) {
    await rm(fixture.root, { recursive: true, force: true });
  }
});

test("document upload, preview, columns and metadata work", async ({ browser }) => {
  const { context, page } = await adminPage(browser);
  const filename = "2026-05-18_Playwright.pdf";
  try {
    await page.goto(`${fixture.baseURL}/`);

    const uploadResponsePromise = page.waitForResponse((response) => {
      return response.url().endsWith("/upload") && response.request().method() === "POST";
    });
    await page.locator("#file-upload").setInputFiles({
      name: filename,
      mimeType: "application/pdf",
      buffer: multiPagePDF(["Playwright Seite 1", "Playwright Seite 2"]),
    });
    expect((await uploadResponsePromise).status()).toBe(201);

    await expect(page.locator("[data-upload-message]")).toContainText("1 hochgeladen");
    await expect(page.locator("[data-document-list]")).toContainText(filename);

    const row = page.locator("[data-document-list] tr", { hasText: filename }).first();
    await row.locator("[data-preview-url]").first().click();
    await expect(page.locator("[data-preview-modal]")).toBeVisible();
    await expect(page.locator("[data-preview-modal] [data-preview-title]")).toHaveText(filename);
    await expect(page.locator("[data-preview-modal] [data-preview-frame]")).toBeVisible();
    await page.locator("[data-preview-close]").click();
    await expect(page.locator("[data-preview-modal]")).not.toBeVisible();

    await page.goto(`${fixture.baseURL}/account`);
    const preferenceForm = page.locator('form[action="/account/preferences"]');
    await preferenceForm.locator('input[name="custom_pdf_preview_enabled"]').check();
    await Promise.all([
      page.waitForURL((url) => url.pathname === "/account" && url.searchParams.has("notice")),
      preferenceForm.getByRole("button", { name: "Darstellung speichern" }).click(),
    ]);
    await expect(page.locator("body")).toHaveAttribute("data-custom-pdf-preview", "true");

    await page.goto(`${fixture.baseURL}/documents`);
    const customRow = page.locator("[data-document-list] tr", { hasText: filename }).first();
    await customRow.locator("[data-preview-url]").first().click();
    const pdfViewer = page.locator("[data-preview-modal] [data-pdf-preview]");
    await expect(pdfViewer).toBeVisible();
    await expect(pdfViewer.locator("[data-pdf-pages]")).toHaveText("2");
    await expect.poll(() => pdfViewer.locator("[data-pdf-canvas]").evaluate((canvas) => canvas.width)).toBeGreaterThan(0);
    await pdfViewer.locator("[data-pdf-next]").click();
    await expect(pdfViewer.locator("[data-pdf-page]")).toHaveValue("2");
    const zoomBefore = await pdfViewer.locator("[data-pdf-zoom]").textContent();
    await pdfViewer.locator("[data-pdf-zoom-in]").click();
    await expect(pdfViewer.locator("[data-pdf-zoom]")).not.toHaveText(zoomBefore || "");
    await expect(pdfViewer.locator("[data-pdf-native]")).toHaveAttribute("href", /\/documents\/\d+\/preview$/);
    await expect(pdfViewer.locator("[data-pdf-download]")).toHaveAttribute("href", /\/documents\/\d+\/download$/);
    await page.setViewportSize({ width: 390, height: 844 });
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(1);
    const viewerBox = await pdfViewer.boundingBox();
    expect(viewerBox).toBeTruthy();
    expect(viewerBox.x + viewerBox.width).toBeLessThanOrEqual(390);
    await page.locator("[data-preview-close]").click();
    await page.setViewportSize({ width: 1280, height: 720 });

    await page.locator("[data-columns-open]").click();
    await expect(page.locator("[data-columns-modal]")).toBeVisible();
    await page.locator("[data-columns-close]").click();
    await expect(page.locator("[data-columns-modal]")).not.toBeVisible();

    await Promise.all([
      page.waitForURL(/\/documents\/\d+/),
      row.locator('td[data-column="name"] a.strong-link').click(),
    ]);
    const detailPDFViewer = page.locator('[data-tab-panel="pdf"] [data-pdf-preview]');
    await expect(detailPDFViewer).toBeVisible();
    await expect(detailPDFViewer.locator("[data-pdf-pages]")).toHaveText("2");
    await expect.poll(() => detailPDFViewer.locator("[data-pdf-canvas]").evaluate((canvas) => canvas.width)).toBeGreaterThan(0);
    await detailPDFViewer.locator("[data-pdf-fit-page]").click();
    await expect(page.locator("[data-preview-modal]")).not.toBeVisible();
    await expect(page.locator("[data-metadata-form]")).toBeVisible();
    await page.locator('[data-metadata-form] input[name="title"]').fill("Playwright Dokument");

    const metadataResponsePromise = page.waitForResponse((response) => {
      return response.url().includes("/metadata") && response.request().method() === "POST";
    });
    await page.locator('[data-metadata-form] button[type="submit"]').click();
    expect((await metadataResponsePromise).status()).toBe(200);
    await expect(page.locator("[data-document-title]")).toHaveText("Playwright Dokument");
  } finally {
    await context.close();
  }
});

test("photo gallery loads and starts thumbnail queue", async ({ browser }) => {
  const { context, page } = await editorPage(browser);
  try {
    const thumbnailRequests = [];
    page.on("request", (request) => {
      if (request.url().includes("/photos/thumbnail?")) {
        thumbnailRequests.push(request.url());
      }
    });
    await page.goto(`${fixture.baseURL}/photos?sort=ascending_name`);

    await expect(page.locator("[data-photo-gallery]")).toBeVisible();
    await expect(page.locator("[data-photo-item]")).toHaveCount(4);
    await expect(photoItem(page, "public-a.png")).toHaveCount(1);
    await expect(photoItem(page, "public-song.mp3")).toHaveCount(1);
    await expect(photoItem(page, "private.png")).toHaveCount(0);
    await expect(page.locator(".photo-filters select[name='sort']")).toHaveCount(0);
    await expect(page.locator("[data-photo-filter-menu] summary")).toBeVisible();
    await expect(page.locator("[data-photo-filter-menu] .photo-filters")).toBeHidden();
    await expect(page.locator("[data-photo-sort-current]")).toHaveText("Name aufsteigend");
    await page.locator("[data-photo-sort-menu] summary").click();
    await expect(page.locator('[data-photo-sort-option="ascending_name"]')).toHaveAttribute("aria-current", "true");
    await expect(page.locator('[data-photo-sort-option="descending_date"]')).toHaveAttribute("href", /sort=descending_date/);

    await expect(page.locator("[data-photo-thumb-image]").first()).toHaveAttribute("src", /\/photos\/thumbnail|\/photos\/media/);
    await expect.poll(() => thumbnailRequests.length).toBeGreaterThan(0);
  } finally {
    await context.close();
  }
});

test("photo filter menu applies search and shows compact chips", async ({ browser }) => {
  const { context, page } = await editorPage(browser);
  try {
    await page.goto(`${fixture.baseURL}/photos?sort=ascending_name`);

    const filterMenu = page.locator("[data-photo-filter-menu]");
    await expect(page.locator(".photo-filter-chips")).toHaveCount(0);
    await filterMenu.locator("summary").click();
    await expect(filterMenu.locator(".photo-filters")).toBeVisible();
    await filterMenu.locator('input[name="q"]').fill("public-song");
    await filterMenu.locator('button[type="submit"]').click();

    await expect(page).toHaveURL(/q=public-song/);
    await expect(photoItem(page, "public-song.mp3")).toHaveCount(1);
    await expect(photoItem(page, "public-a.png")).toHaveCount(0);
    await expect(page.locator(".photo-filter-chips")).toContainText("Suche");
    await expect(page.locator(".photo-filter-chips")).toContainText("public-song");

    await page.locator("[data-photo-filter-menu] summary").click();
    await page.locator('[data-photo-filter-menu] select[name="type"]').selectOption("audio");
    await page.locator('[data-photo-filter-menu] button[type="submit"]').click();

    await expect(page).toHaveURL(/type=audio/);
    await expect(page.locator(".photo-filter-chips")).toContainText("Audios");
    await page.locator(".photo-filter-reset").click();
    await expect(page).toHaveURL(/\/photos$/);
    await expect(page.locator(".photo-filter-chips")).toHaveCount(0);
  } finally {
    await context.close();
  }
});

test("photo filter menu fits mobile viewport", async ({ browser }) => {
  const { context, page } = await editorPage(browser, {
    hasTouch: true,
    isMobile: true,
    viewport: { width: 390, height: 844 },
  });
  try {
    await page.goto(`${fixture.baseURL}/photos?type=audio`);

    await expect(page.locator(".photo-filter-chips")).toContainText("Audios");
    const modeToggleBox = await page.locator(".photo-mode-toggle").boundingBox();
    expect(modeToggleBox).toBeTruthy();
    expect(Math.round(modeToggleBox.width)).toBe(78);
    await page.locator("[data-photo-filter-menu] summary").tap();
    const headBox = await page.locator(".photo-page > .page-head").boundingBox();
    const panelBox = await page.locator("[data-photo-filter-menu] .photo-filters").boundingBox();
    expect(headBox).toBeTruthy();
    expect(panelBox).toBeTruthy();
    expect(Math.abs(panelBox.x - headBox.x)).toBeLessThanOrEqual(1);
    expect(Math.abs(panelBox.width - headBox.width)).toBeLessThanOrEqual(1);
  } finally {
    await context.close();
  }
});

test("photo gallery defaults to ascending date groups", async ({ browser }) => {
  const { context, page } = await editorPage(browser);
  try {
    await page.goto(`${fixture.baseURL}/photos`);

    const groups = page.locator("[data-photo-date-group]");
    await expect(groups).toHaveCount(2);
    await expect(groups.nth(0)).toContainText("30.05.2024");
    await expect(groups.nth(0)).toContainText("1 Medium");
    await expect(groups.nth(1)).toContainText("01.06.2024");
    await expect(groups.nth(1)).toContainText("3 Medien");
    await expect(photoItem(page, "public-a.png")).toHaveCount(1);
    await expect(photoItem(page, "public-song.mp3")).toHaveCount(1);
    await expect(photoItem(page, "public-video.mp4")).toHaveCount(1);
    await expect(photoItem(page, "public-b.png")).toHaveCount(1);
  } finally {
    await context.close();
  }
});

test("photo lightbox opens from gallery", async ({ browser }) => {
  const { context, page } = await editorPage(browser);
  try {
    await page.goto(`${fixture.baseURL}/photos?sort=ascending_name`);
    await photoItem(page, "public-a.png").locator(".photo-card-button").click();

    await expect(page.locator("[data-photo-lightbox][open]")).toBeVisible();
    await expect(page.locator("[data-photo-lightbox] [data-photo-title]")).toHaveText("public-a.png");
    await expect(page.locator("[data-photo-lightbox] [data-photo-image]")).toHaveAttribute("src", /\/photos\/thumbnail\?path=public-a\.png&size=1920/);

    const stageBox = await page.locator(".photo-lightbox-stage").boundingBox();
    expect(stageBox).toBeTruthy();
    const imageBox = await page.locator("[data-photo-lightbox] [data-photo-image]").boundingBox();
    expect(imageBox.width).toBeGreaterThan(stageBox.width * 0.95);
    expect(imageBox.height).toBeGreaterThan(stageBox.height * 0.95);

    const zoomControls = page.locator("[data-photo-lightbox] [data-photo-zoom-controls]");
    const zoomIndicator = page.locator("[data-photo-lightbox] [data-photo-zoom-indicator]");
    await revealPhotoControls(page);
    await expect.poll(async () => await lightboxHeaderOpacity(page)).toBeGreaterThan(0.9);
    await expect.poll(async () => await lightboxZoomControlsOpacity(page)).toBeGreaterThan(0.9);
    await expect(zoomControls).toBeVisible();
    await expect(zoomIndicator).toHaveText("100%");
    await expect(zoomIndicator).toBeHidden();
    const zoomBox = await zoomControls.boundingBox();
    expect(zoomBox).toBeTruthy();
    expect(zoomBox.x + zoomBox.width).toBeGreaterThan(stageBox.x + stageBox.width * 0.75);
    expect(zoomBox.y + zoomBox.height).toBeGreaterThan(stageBox.y + stageBox.height * 0.75);

    await page.locator("[data-photo-lightbox] [data-photo-zoom-in]").click();
    await expect.poll(async () => await lightboxZoomPercent(page)).toBeGreaterThan(100);
    await expect(zoomIndicator).toBeVisible();
    await page.mouse.click(stageBox.x + stageBox.width * 0.9, stageBox.y + stageBox.height / 2);
    await expect(page.locator("[data-photo-lightbox] [data-photo-title]")).toHaveText("public-a.png");
    await page.mouse.move(stageBox.x + stageBox.width / 2, stageBox.y + stageBox.height - 8);
    await expect.poll(async () => await lightboxZoomControlsOpacity(page)).toBeGreaterThan(0.9);
    await page.locator("[data-photo-lightbox] [data-photo-zoom-reset]").click();
    await expect(zoomIndicator).toHaveText("100%");
    await expect(zoomIndicator).toBeHidden();

    await page.mouse.click(stageBox.x + stageBox.width * 0.9, stageBox.y + stageBox.height / 2);
    await expect(page.locator("[data-photo-lightbox] [data-photo-title]")).toHaveText("public-b.png");
    await page.mouse.click(stageBox.x + stageBox.width * 0.1, stageBox.y + stageBox.height / 2);
    await expect(page.locator("[data-photo-lightbox] [data-photo-title]")).toHaveText("public-a.png");

    await page.mouse.move(400, 300);
    await expect.poll(async () => await lightboxHeaderOpacity(page)).toBeLessThan(0.1);
    await expect.poll(async () => await lightboxZoomControlsOpacity(page)).toBeLessThan(0.1);
    await revealPhotoControls(page);
    await expect.poll(async () => await lightboxHeaderOpacity(page)).toBeGreaterThan(0.9);
    await expect.poll(async () => await lightboxZoomControlsOpacity(page)).toBeGreaterThan(0.9);
    await page.mouse.move(stageBox.x + stageBox.width / 2, stageBox.y + stageBox.height / 2);
    await expect.poll(async () => await lightboxHeaderOpacity(page)).toBeLessThan(0.1);
    await expect.poll(async () => await lightboxZoomControlsOpacity(page)).toBeLessThan(0.1);
    await page.mouse.move(stageBox.x + stageBox.width / 2, stageBox.y + stageBox.height - 8);
    await expect.poll(async () => await lightboxHeaderOpacity(page)).toBeGreaterThan(0.9);
    await expect.poll(async () => await lightboxZoomControlsOpacity(page)).toBeGreaterThan(0.9);

    await page.locator("[data-photo-info-toggle]").click();
    await expect(page.locator("[data-photo-lightbox] [data-photo-info-name]")).toHaveText("public-a.png");
    await expect(page.locator("[data-photo-lightbox] [data-photo-info-rating]")).toHaveText("4 Sterne");
  } finally {
    await context.close();
  }
});

test("photo lightbox chooses large previews for large screens", async ({ browser }) => {
  const { context, page } = await editorPage(browser);
  try {
    await page.setViewportSize({ width: 2600, height: 1400 });
    const thumbnailRequests = [];
    page.on("request", (request) => {
      if (request.url().includes("/photos/thumbnail?")) {
        thumbnailRequests.push(request.url());
      }
    });
    await page.goto(`${fixture.baseURL}/photos?sort=ascending_name`);
    await photoItem(page, "public-a.png").locator(".photo-card-button").click();

    await expect(page.locator("[data-photo-lightbox] [data-photo-image]")).toHaveAttribute("src", /\/photos\/thumbnail\?path=public-a\.png&size=3840/);
    await expect.poll(() => {
      return thumbnailRequests.some((url) => url.includes("path=public-b.png") && url.includes("size=3840"));
    }).toBe(true);
  } finally {
    await context.close();
  }
});

test("photo lightbox toggles fullscreen mode", async ({ browser }) => {
  const { context, page } = await editorPage(browser);
  try {
    await page.goto(`${fixture.baseURL}/photos?sort=ascending_name`);
    await photoItem(page, "public-a.png").locator(".photo-card-button").click();

    const lightbox = page.locator("[data-photo-lightbox]");
    const fullscreen = lightbox.locator("[data-photo-fullscreen]");
    const supported = await page.evaluate(() => Boolean(document.fullscreenEnabled && document.documentElement.requestFullscreen));
    test.skip(!supported, "Fullscreen API is not available in this browser");

    await revealPhotoControls(page);
    await fullscreen.click();
    await expect(fullscreen).toHaveAttribute("aria-pressed", "true");
    await expect.poll(async () => {
      return await page.evaluate(() => document.fullscreenElement === document.documentElement);
    }).toBe(true);

    await fullscreen.click();
    await expect.poll(async () => {
      return await page.evaluate(() => Boolean(document.fullscreenElement));
    }).toBe(false);
    await expect(fullscreen).toHaveAttribute("aria-pressed", "false");

    await fullscreen.click();
    await expect.poll(async () => {
      return await page.evaluate(() => document.fullscreenElement === document.documentElement);
    }).toBe(true);
    await expect(fullscreen).toHaveAttribute("aria-pressed", "true");

    await revealPhotoControls(page);
    await lightbox.locator("[data-photo-close]").click();

    await expect(lightbox).not.toBeVisible();
    await expect.poll(async () => {
      return await page.evaluate(() => Boolean(document.fullscreenElement));
    }).toBe(false);
    await expect(fullscreen).toHaveAttribute("aria-pressed", "false");
  } finally {
    await context.close();
  }
});

test("photo lightbox controls work with touch", async ({ browser }) => {
  const { context, page } = await editorPage(browser, {
    hasTouch: true,
    isMobile: true,
    viewport: { width: 390, height: 844 },
  });
  try {
    await page.goto(`${fixture.baseURL}/photos?sort=ascending_name`);
    await photoItem(page, "public-a.png").locator(".photo-card-button").tap();

    const lightbox = page.locator("[data-photo-lightbox]");
    await expect(lightbox).toBeVisible();
    await expect.poll(async () => await lightboxHeaderOpacity(page)).toBeGreaterThan(0.9);
    await expect.poll(async () => await lightboxZoomControlsOpacity(page)).toBeGreaterThan(0.9);

    const stageBox = await page.locator(".photo-lightbox-stage").boundingBox();
    expect(stageBox).toBeTruthy();
    await page.touchscreen.tap(stageBox.x + stageBox.width / 2, stageBox.y + stageBox.height / 2);
    await expect.poll(async () => await lightboxHeaderOpacity(page)).toBeLessThan(0.1);
    await expect.poll(async () => await lightboxZoomControlsOpacity(page)).toBeLessThan(0.1);
    await page.touchscreen.tap(stageBox.x + stageBox.width / 2, stageBox.y + stageBox.height / 2);
    await expect.poll(async () => await lightboxHeaderOpacity(page)).toBeGreaterThan(0.9);
    await expect.poll(async () => await lightboxZoomControlsOpacity(page)).toBeGreaterThan(0.9);

    await pinchLightboxStage(page, 1.8);
    await expect.poll(async () => await lightboxZoomPercent(page)).toBeGreaterThan(100);
    await expect(lightbox.locator("[data-photo-zoom-indicator]")).toBeVisible();
    await page.touchscreen.tap(stageBox.x + stageBox.width * 0.9, stageBox.y + stageBox.height / 2);
    await expect(lightbox.locator("[data-photo-title]")).toHaveText("public-a.png");
    await lightbox.locator("[data-photo-zoom-reset]").tap();
    await expect.poll(async () => await lightboxZoomPercent(page)).toBe(100);
    await expect(lightbox.locator("[data-photo-zoom-indicator]")).toBeHidden();

    await page.touchscreen.tap(stageBox.x + stageBox.width * 0.9, stageBox.y + stageBox.height / 2);
    await expect(lightbox.locator("[data-photo-title]")).toHaveText("public-b.png");
    await expect.poll(async () => await lightboxHeaderOpacity(page)).toBeGreaterThan(0.9);
    await page.touchscreen.tap(stageBox.x + stageBox.width * 0.1, stageBox.y + stageBox.height / 2);
    await expect(lightbox.locator("[data-photo-title]")).toHaveText("public-a.png");
    await expect.poll(async () => await lightboxHeaderOpacity(page)).toBeGreaterThan(0.9);

    await lightbox.locator("[data-photo-close]").tap();
    await expect(lightbox).not.toBeVisible();
  } finally {
    await context.close();
  }
});

test("photo lightbox video supports touch controls after controls are hidden", async ({ browser }) => {
  const { context, page } = await editorPage(browser, {
    hasTouch: true,
    isMobile: true,
    viewport: { width: 390, height: 844 },
  });
  try {
    await installVideoPlaybackProbe(page);
    await page.goto(`${fixture.baseURL}/photos?sort=ascending_name`);
    await photoItem(page, "public-video.mp4").locator(".photo-card-button").tap();

    const lightbox = page.locator("[data-photo-lightbox]");
    const video = lightbox.locator("[data-photo-video]");
    await expect(lightbox).toBeVisible();
    await expect(video).toBeVisible();
    await video.evaluate((element) => element.play());
    await expect(video).toHaveAttribute("data-play-state", "playing");

    const stageBox = await page.locator(".photo-lightbox-stage").boundingBox();
    expect(stageBox).toBeTruthy();

    await page.evaluate(() => {
      document.querySelector("[data-photo-lightbox]")?.classList.remove("controls-visible");
    });
    await expect.poll(async () => await lightboxHeaderOpacity(page)).toBeLessThan(0.1);

    await page.touchscreen.tap(stageBox.x + stageBox.width / 2, stageBox.y + stageBox.height / 2);
    await expect.poll(async () => await lightboxHeaderOpacity(page)).toBeGreaterThan(0.9);

    await lightbox.locator("[data-photo-next]").tap();
    await expect(lightbox.locator("[data-photo-title]")).toHaveText("public-a.png");

    await expect.poll(async () => await lightboxHeaderOpacity(page)).toBeGreaterThan(0.9);
    await lightbox.locator("[data-photo-close]").tap();
    await expect(lightbox).not.toBeVisible();
  } finally {
    await context.close();
  }
});

test("photo lightbox stops video when closed", async ({ browser }) => {
  const { context, page } = await editorPage(browser);
  try {
    await page.goto(`${fixture.baseURL}/photos?sort=ascending_name`);
    await photoItem(page, "public-video.mp4").locator(".photo-card-button").click();

    const lightbox = page.locator("[data-photo-lightbox]");
    const video = lightbox.locator("[data-photo-video]");
    await expect(lightbox).toBeVisible();
    await expect(video).toBeVisible();
    await expect(video).toHaveAttribute("src", /\/photos\/media\?path=public-video\.mp4/);

    await revealPhotoControls(page);
    await lightbox.locator("[data-photo-close]").click();

    await expect(lightbox).not.toBeVisible();
    await expect.poll(async () => {
      return await video.evaluate((element) => ({
        hidden: element.hidden,
        paused: element.paused,
        src: element.getAttribute("src") || "",
      }));
    }).toEqual({
      hidden: true,
      paused: true,
      src: "",
    });
  } finally {
    await context.close();
  }
});

test("photo lightbox opens audio player for mp3 files", async ({ browser }) => {
  const { context, page } = await editorPage(browser);
  try {
    await page.goto(`${fixture.baseURL}/photos?sort=ascending_name`);
    const item = photoItem(page, "public-song.mp3");
    await expect(item.locator(".photo-audio-placeholder")).toBeVisible();
    await item.locator(".photo-card-button").click();

    const lightbox = page.locator("[data-photo-lightbox]");
    const audio = lightbox.locator("[data-photo-audio]");
    await expect(lightbox).toBeVisible();
    await expect(audio).toBeVisible();
    await expect(audio).toHaveAttribute("src", /\/photos\/media\?path=public-song\.mp3/);

    await revealPhotoControls(page);
    await lightbox.locator("[data-photo-close]").click();
    await expect(lightbox).not.toBeVisible();
    await expect.poll(async () => {
      return await audio.evaluate((element) => ({
        src: element.getAttribute("src") || "",
      }));
    }).toEqual({ src: "" });
  } finally {
    await context.close();
  }
});

test("photo lightbox toggles video playback with space", async ({ browser }) => {
  const { context, page } = await editorPage(browser);
  try {
    await installVideoPlaybackProbe(page);
    await page.goto(`${fixture.baseURL}/photos?sort=ascending_name`);
    await photoItem(page, "public-video.mp4").locator(".photo-card-button").click();

    const video = page.locator("[data-photo-lightbox] [data-photo-video]");
    await expect(video).toBeVisible();

    await page.keyboard.press("Space");
    await expect(video).toHaveAttribute("data-play-state", "playing");

    await page.keyboard.press("Space");
    await expect(video).toHaveAttribute("data-play-state", "paused");
  } finally {
    await context.close();
  }
});

test("photo selection and bulk tagging work", async ({ browser }) => {
  const { context, page } = await editorPage(browser);
  try {
    await page.goto(`${fixture.baseURL}/photos?sort=ascending_name`);
    await page.locator("[data-photo-mode-toggle]").click();
    await expect(page.locator("[data-photo-module]")).toHaveAttribute("data-photo-mode", "edit");

    await page.locator('input[name="ids"][value="public-a.png"]').evaluate((input) => {
      input.checked = true;
      input.dispatchEvent(new Event("change", { bubbles: true }));
    });
    await expect(page.locator("[data-photo-selection-count]")).toHaveText("1 ausgewählt");

    await page.getByRole("button", { name: "Tags ergänzen" }).click();
    await expect(page.locator("[data-tag-select-modal]")).toBeVisible();
    await page.locator("[data-tag-select-search]").fill("Smoke");
    await page.locator("[data-tag-create]").click();

    const responsePromise = page.waitForResponse((response) => {
      return response.url().endsWith("/photos/tags/add") && response.request().method() === "POST";
    });
    const reloadPromise = page.waitForEvent("framenavigated");
    await page.locator("[data-tag-select-apply]").click();
    const response = await responsePromise;
    expect(response.status()).toBe(200);
    await reloadPromise;
    await page.waitForLoadState("load");

    await page.goto(`${fixture.baseURL}/photos?q=tag%3Asmoke&sort=ascending_name`);
    await expect(photoItem(page, "public-a.png")).toHaveCount(1);
    await expect(photoItem(page, "public-b.png")).toHaveCount(0);
  } finally {
    await context.close();
  }
});

test("non-admin photo user cannot see admin-only content", async ({ browser }) => {
  const { context, page } = await editorPage(browser);
  try {
    await page.goto(`${fixture.baseURL}/photos?sort=ascending_name`);
    await expect(page.locator("[data-photo-item]")).toHaveCount(4);
    await expect(photoItem(page, "private.png")).toHaveCount(0);

    const media = await context.request.get(`${fixture.baseURL}/photos/media?path=secret%2Fprivate.png`);
    expect(media.status()).toBe(403);
  } finally {
    await context.close();
  }
});

test("admin can reveal admin-only content from sort menu for the session", async ({ browser }) => {
  const { context, page } = await adminPage(browser);
  try {
    await page.goto(`${fixture.baseURL}/photos?sort=ascending_name`);
    await expect(page.locator('.photo-folder-link[href="/photos?path=secret"]')).toHaveCount(0);

    const sortMenu = page.locator("[data-photo-sort-menu]");
    await sortMenu.locator("summary").click();
    const toggle = sortMenu.getByRole("switch", { name: "Admin-only anzeigen" });
    await expect(toggle).toHaveAttribute("aria-checked", "false");
    await Promise.all([
      page.waitForURL(/\/photos\?sort=ascending_name$/),
      toggle.click(),
    ]);

    await expect(page.locator('.photo-folder-link[href="/photos?path=secret"]')).toHaveCount(1);
    await page.locator('.photo-folder-link[href="/photos?path=secret"]').click();
    await expect(photoItem(page, "private.png")).toHaveCount(1);

    await page.goto(`${fixture.baseURL}/photos?sort=ascending_name`);
    await page.locator("[data-photo-sort-menu] summary").click();
    await expect(page.locator("[data-photo-sort-menu]").getByRole("switch", { name: "Admin-only ausblenden" })).toHaveAttribute("aria-checked", "true");
  } finally {
    await context.close();
  }
});

test("photo map renders gpx tracks with layer toggles", async ({ browser }) => {
  const { context, page } = await editorPage(browser);
  try {
    await page.route("https://tile.openstreetmap.org/**", async (route) => {
      await route.fulfill({ status: 200, contentType: "image/png", body: tinyPNG });
    });
    await page.goto(`${fixture.baseURL}/photos?path=album%2Ftrip&view=map`);

    const breadcrumb = page.locator(".folder-breadcrumb");
    await expect(breadcrumb.locator(".folder-breadcrumb-current")).toHaveText("Karte");
    await expect(breadcrumb.locator("a.folder-breadcrumb-link", { hasText: "trip" })).toHaveAttribute("href", /\/photos\?path=album%2Ftrip$/);

    const panel = page.locator("[data-photo-map-layer-panel]");
    await expect(panel).toBeVisible();
    await expect(panel).toContainText("Fotos");
    await expect(panel).toContainText("01.06.2024");
    await expect(panel).toContainText("wanderung.gpx");
    await expect(page.locator("[data-photo-layer-toggle]")).toHaveCount(3);

    const firstTrack = page.locator('[data-photo-map-gpx-track][data-photo-map-layer="gpx-0"]');
    const firstLabel = page.locator('[data-photo-map-gpx-label][data-photo-map-layer="gpx-0"]');
    await expect(firstTrack).toBeVisible();
    await expect(firstLabel).toHaveText("01.06.2024");
    const pointCount = await firstTrack.getAttribute("data-points").then((value) => {
      return (value || "").trim().split(/\s+/).filter(Boolean).length;
    });
    expect(pointCount).toBe(2);

    const firstToggle = page.locator('[data-photo-layer-toggle][data-photo-map-layer="gpx-0"]');
    await firstToggle.uncheck();
    await expect(firstTrack).toBeHidden();
    await expect(firstLabel).toBeHidden();
    await firstToggle.check();
    await expect(firstTrack).toBeVisible();
  } finally {
    await context.close();
  }
});

async function editorPage(browser, options = {}) {
  return await authenticatedPage(browser, "editor", options);
}

async function adminPage(browser, options = {}) {
  return await authenticatedPage(browser, "admin", options);
}

async function authenticatedPage(browser, username, options = {}) {
  const context = await browser.newContext(options);
  const page = await context.newPage();
  await login(page, username);
  return { context, page };
}

async function login(page, username) {
  await page.goto(`${fixture.baseURL}/login`);
  await page.locator('input[name="username"]').fill(username);
  await page.locator('input[name="password"]').fill(password);
  await Promise.all([
    page.waitForURL((url) => url.pathname !== "/login"),
    page.getByRole("button", { name: "Anmelden" }).click(),
  ]);
}

function photoItem(page, title) {
  return page.locator(`[data-photo-item][data-photo-title="${title}"]`);
}

async function revealPhotoControls(page) {
  await page.mouse.move(400, 8);
}

async function installVideoPlaybackProbe(page) {
  await page.addInitScript(() => {
    Object.defineProperty(HTMLMediaElement.prototype, "paused", {
      configurable: true,
      get() {
        return this.dataset.playState !== "playing";
      },
    });
    Object.defineProperty(HTMLMediaElement.prototype, "ended", {
      configurable: true,
      get() {
        return false;
      },
    });
    HTMLMediaElement.prototype.play = function () {
      this.dataset.playState = "playing";
      return Promise.resolve();
    };
    HTMLMediaElement.prototype.pause = function () {
      this.dataset.playState = "paused";
    };
  });
}

async function lightboxHeaderOpacity(page) {
  return await page.locator("[data-photo-lightbox] > header").evaluate((header) => {
    return Number.parseFloat(window.getComputedStyle(header).opacity || "0");
  });
}

async function lightboxZoomControlsOpacity(page) {
  return await page.locator("[data-photo-lightbox] [data-photo-zoom-controls]").evaluate((controls) => {
    return Number.parseFloat(window.getComputedStyle(controls).opacity || "0");
  });
}

async function lightboxZoomPercent(page) {
  return await page.locator("[data-photo-lightbox] [data-photo-zoom-indicator]").evaluate((node) => {
    const value = Number.parseFloat((node.textContent || "").replace(/[^\d.]+/g, ""));
    return Number.isFinite(value) ? value : 0;
  });
}

async function pinchLightboxStage(page, factor = 1.6) {
  await page.evaluate((zoomFactor) => {
    const stage = document.querySelector(".photo-lightbox-stage");
    if (!stage || typeof PointerEvent !== "function") return;
    const rect = stage.getBoundingClientRect();
    if (!rect.width || !rect.height) return;
    const centerX = rect.left + rect.width / 2;
    const centerY = rect.top + rect.height / 2;
    const startOffset = Math.max(14, Math.min(rect.width, rect.height) * 0.1);
    const endOffset = startOffset * Math.max(1.2, zoomFactor);
    const dispatch = (type, pointerId, x, y, isPrimary) => {
      stage.dispatchEvent(new PointerEvent(type, {
        bubbles: true,
        cancelable: true,
        pointerId,
        pointerType: "touch",
        isPrimary,
        clientX: x,
        clientY: y,
        buttons: 1,
      }));
    };

    dispatch("pointerdown", 1, centerX - startOffset, centerY, true);
    dispatch("pointerdown", 2, centerX + startOffset, centerY, false);
    dispatch("pointermove", 1, centerX - endOffset, centerY, true);
    dispatch("pointermove", 2, centerX + endOffset, centerY, false);
    dispatch("pointerup", 2, centerX + endOffset, centerY, false);
    dispatch("pointerup", 1, centerX - endOffset, centerY, true);
  }, factor);
}

async function createPhotoFixture() {
  const root = await mkdtemp(path.join(os.tmpdir(), "bearstack-playwright-"));
  const photosRoot = path.join(root, "photos");
  const secretRoot = path.join(photosRoot, "secret");
  const tripRoot = path.join(photosRoot, "album", "trip");
  const dataDir = path.join(root, "data");
  const photoDataDir = path.join(root, "photos-data");
  const toolsDir = path.join(root, "tools");
  const thumbnailFixture = path.join(root, "thumbnail.png");
  await mkdir(secretRoot, { recursive: true });
  await mkdir(tripRoot, { recursive: true });
  await mkdir(dataDir, { recursive: true });
  await mkdir(photoDataDir, { recursive: true });
  await mkdir(toolsDir, { recursive: true });

  await writeFile(path.join(photosRoot, ".order_ascending_name.pg2conf"), "");
  await writeFile(path.join(photosRoot, "public-a.png"), tinyPNG);
  await writeFile(path.join(photosRoot, "public-a.png.xmp"), xmpRating(4));
  await writeFile(path.join(photosRoot, "public-b.png"), tinyPNG);
  await writeFile(path.join(photosRoot, "public-song.mp3"), Buffer.from("not a real mp3"));
  await writeFile(path.join(photosRoot, "public-video.mp4"), Buffer.from("not a real video"));
  await writeFile(path.join(secretRoot, "private.png"), tinyPNG);
  await writeFile(path.join(secretRoot, ".adminonly"), "");
  await writeFile(path.join(tripRoot, "2024-06-01-run.gpx"), gpxTrack([
    [52.520000, 13.405000],
    [52.520010, 13.405000],
    [52.520120, 13.405000],
  ]));
  await writeFile(path.join(tripRoot, "wanderung.gpx"), gpxTrack([
    [52.500000, 13.370000],
    [52.500100, 13.370000],
  ]));
  await writeFile(thumbnailFixture, tinyPNG);
  await setFixtureMTime(path.join(photosRoot, "public-a.png"), "2024-06-01T12:00:00Z");
  await setFixtureMTime(path.join(photosRoot, "public-song.mp3"), "2024-06-01T11:00:00Z");
  await setFixtureMTime(path.join(photosRoot, "public-video.mp4"), "2024-06-01T10:00:00Z");
  await setFixtureMTime(path.join(photosRoot, "public-b.png"), "2024-05-30T12:00:00Z");
  await writeFakeFFmpeg(path.join(toolsDir, "ffmpeg"));

  const port = await freePort();
  const configPath = path.join(root, "bearstack.json");
  await writeFile(configPath, JSON.stringify({
    addr: `127.0.0.1:${port}`,
    data_dir: dataDir,
    auth: {
      credentials: [
        { username: "admin", password, role: "admin" },
        { username: "editor", password, role: "photos_editor" },
      ],
    },
    photos: {
      enabled: true,
      root_dir: photosRoot,
      data_dir: photoDataDir,
      page_size: 20,
    },
  }, null, 2));

  return {
    root,
    baseURL: `http://127.0.0.1:${port}`,
    configPath,
    toolsDir,
    thumbnailFixture,
  };
}

async function setFixtureMTime(target, value) {
  const date = new Date(value);
  await utimes(target, date, date);
}

function gpxTrack(points) {
  return `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="bearstack-test">
  <trk><trkseg>
${points.map(([lat, lon]) => `    <trkpt lat="${lat.toFixed(6)}" lon="${lon.toFixed(6)}"></trkpt>`).join("\n")}
  </trkseg></trk>
</gpx>`;
}

function xmpRating(value) {
  return `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/">
  <rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
    <rdf:Description xmlns:xmp="http://ns.adobe.com/xap/1.0/" xmp:Rating="${value}"/>
  </rdf:RDF>
</x:xmpmeta>
<?xpacket end="w"?>`;
}

async function writeFakeFFmpeg(target) {
  await writeFile(target, `#!/bin/sh
last=""
for arg in "$@"; do
  last="$arg"
done
cp "$BEARSTACK_E2E_THUMBNAIL" "$last"
`);
  await chmod(target, 0o700);
}

async function startBearStack(currentFixture) {
  const child = spawn("go", ["run", "./cmd/bearstack"], {
    cwd: repoRoot,
    env: {
      ...process.env,
      BEARSTACK_CONFIG: currentFixture.configPath,
      BEARSTACK_E2E_THUMBNAIL: currentFixture.thumbnailFixture,
      GOCACHE: path.join(currentFixture.root, "go-cache"),
      PATH: `${currentFixture.toolsDir}${path.delimiter}${process.env.PATH || ""}`,
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
  await waitForHealth(currentFixture.baseURL, state);
  return state;
}

async function stopBearStack(state) {
  if (!state?.child || state.exited) return;
  state.child.kill("SIGTERM");
  await Promise.race([
    new Promise((resolve) => state.child.once("exit", resolve)),
    new Promise((resolve) => setTimeout(resolve, 2_000)).then(() => state.child.kill("SIGKILL")),
  ]);
}

async function waitForHealth(baseURL, state) {
  const authorization = `Basic ${Buffer.from(`admin:${password}`).toString("base64")}`;
  const deadline = Date.now() + 20_000;
  while (Date.now() < deadline) {
    if (state.exited) {
      throw new Error(`BearStack exited before healthcheck: ${JSON.stringify(state.exited)}\n${state.output}`);
    }
    try {
      const response = await fetch(`${baseURL}/healthz`, { headers: { authorization } });
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

function multiPagePDF(labels) {
  const objects = [];
  const pageObjectNumbers = [];
  const contentObjectNumbers = [];
  for (let index = 0; index < labels.length; index += 1) {
    pageObjectNumbers.push(3 + index * 2);
    contentObjectNumbers.push(4 + index * 2);
  }
  const fontObjectNumber = 3 + labels.length * 2;
  objects[1] = "<< /Type /Catalog /Pages 2 0 R >>";
  objects[2] = `<< /Type /Pages /Kids [${pageObjectNumbers.map((number) => `${number} 0 R`).join(" ")}] /Count ${labels.length} >>`;
  labels.forEach((label, index) => {
    const pageNumber = pageObjectNumbers[index];
    const contentNumber = contentObjectNumbers[index];
    const escaped = label.replace(/([\\()])/g, "\\$1");
    const stream = `BT /F1 24 Tf 72 720 Td (${escaped}) Tj ET`;
    objects[pageNumber] = `<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 ${fontObjectNumber} 0 R >> >> /Contents ${contentNumber} 0 R >>`;
    objects[contentNumber] = `<< /Length ${Buffer.byteLength(stream)} >>\nstream\n${stream}\nendstream`;
  });
  objects[fontObjectNumber] = "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>";

  let body = "%PDF-1.7\n%âãÏÓ\n";
  const offsets = [0];
  for (let number = 1; number < objects.length; number += 1) {
    offsets[number] = Buffer.byteLength(body, "binary");
    body += `${number} 0 obj\n${objects[number]}\nendobj\n`;
  }
  const xrefOffset = Buffer.byteLength(body, "binary");
  body += `xref\n0 ${objects.length}\n0000000000 65535 f \n`;
  for (let number = 1; number < objects.length; number += 1) {
    body += `${String(offsets[number]).padStart(10, "0")} 00000 n \n`;
  }
  body += `trailer\n<< /Size ${objects.length} /Root 1 0 R >>\nstartxref\n${xrefOffset}\n%%EOF\n`;
  return Buffer.from(body, "binary");
}
