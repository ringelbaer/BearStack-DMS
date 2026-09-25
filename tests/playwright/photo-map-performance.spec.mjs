import { expect, test } from "@playwright/test";
import { readFile } from "node:fs/promises";

const asset = name => readFile(new URL(`../../internal/server/static/${name}`, import.meta.url), "utf8");
const [core, view, css] = await Promise.all([asset("app-photos-map.js"), asset("app-photos-map-view.js"), asset("app.css")]);

async function mapPage(page, points, tracks = false) {
  await page.route("https://tile.openstreetmap.org/**", route => route.abort());
  await page.setContent(`<section data-photo-map><div class="photo-map-canvas" data-photo-map-canvas tabindex="0" style="width:1000px;height:600px">
    <div class="photo-map-tile-layer" data-photo-map-tiles></div><div class="photo-map-controls"><button data-photo-map-zoom-in>+</button><button data-photo-map-zoom-out>−</button></div>
    <div class="photo-map-layer-panel"><input type="checkbox" checked data-photo-layer-toggle data-photo-map-layer="photos"></div>
    ${tracks ? '<svg class="photo-map-gpx-track" data-photo-map-gpx-track data-photo-map-layer="track"></svg>' : ''}
  </div></section>`);
  await page.addStyleTag({ content: css });
  await page.addScriptTag({ content: core });
  await page.evaluate(({ points, tracks }) => {
    const canvas = document.querySelector("[data-photo-map-canvas]");
    const fragment = document.createDocumentFragment();
    for (const [lat, lon] of points) {
      const marker = document.createElement("button");
      marker.className = "photo-map-marker";
      marker.dataset.photoMapMarker = "";
      marker.dataset.lat = lat;
      marker.dataset.lon = lon;
      fragment.append(marker);
    }
    canvas.append(fragment);
    if (tracks) document.querySelector("svg").dataset.points = points.map(p => p.join(",")).join(" ");
    const map = window.BearStack.photos.map, render = map.renderTiles;
    window.mapRenders = 0;
    map.renderTiles = (layer, state, size, cache) => {
      window.mapRenders++;
      window.currentMapState = { ...state };
      window.currentMapSize = { ...size };
      return render(layer, state, size, cache);
    };
  }, { points, tracks });
  await page.addScriptTag({ content: view });
  await page.evaluate(() => window.BearStack.photos.map.init());
}

test("map tile cache stays bounded and retains visible nodes", async ({ page }) => {
  await page.route("https://tile.openstreetmap.org/**", route => route.abort());
  await page.setContent('<div id="tiles"></div>');
  await page.addScriptTag({ content: core });
  const result = await page.evaluate(() => {
    const cache = new Map(), layer = document.querySelector("#tiles"), render = window.BearStack.photos.map.renderTiles;
    const size = { width: 1024, height: 768 };
    let max = 0;
    for (let i = 0; i < 200; i++) {
      render(layer, { lat: 50, lon: 5 + i * .1, zoom: 12 }, size, cache);
      max = Math.max(max, cache.size);
    }
    const before = Array.from(layer.children);
    render(layer, { lat: 50, lon: 24.9, zoom: 12 }, size, cache);
    return { max, retained: cache.size, visible: layer.children.length,
      reused: before.every((node, i) => node === layer.children[i]),
      allVisibleCached: Array.from(layer.children).every(node => [...cache.values()].includes(node)) };
  });
  expect(result).toEqual({ max: 128, retained: 128, visible: 20, reused: true, allVisibleCached: true });
});

test("panning 10000 markers and track points changes only the overlay", async ({ page }) => {
  await mapPage(page, Array.from({ length: 10000 }, (_, i) => [48 + i / 100000, 11 + i / 100000]), true);
  const before = await page.evaluate(() => {
    const overlay = document.querySelector(".photo-map-overlay");
    window.geometryWrites = 0;
    new MutationObserver(records => {
      window.geometryWrites += records.filter(record => record.target !== overlay).length;
    }).observe(overlay, { attributes: true, childList: true, subtree: true });
    const marker = overlay.querySelector("button");
    const rect = marker.getBoundingClientRect();
    return { x: rect.x, renders: window.mapRenders };
  });
  await page.locator("[data-photo-map-canvas]").press("ArrowRight");
  await expect.poll(() => page.evaluate(() => window.mapRenders)).toBe(before.renders + 1);
  const after = await page.evaluate(() => ({ writes: window.geometryWrites,
    x: document.querySelector(".photo-map-marker").getBoundingClientRect().x }));
  expect(after.writes).toBe(0);
  expect(after.x).toBeCloseTo(before.x - 80, 1);
  await page.locator("[data-photo-layer-toggle]").uncheck();
  await expect(page.locator(".photo-map-marker").first()).toBeHidden();
  await page.locator("[data-photo-layer-toggle]").check();
  await expect(page.locator(".photo-map-marker").first()).toBeVisible();
  const marker = page.locator(".photo-map-marker").last();
  await marker.evaluate(node => node.addEventListener("click", () => { window.markerClicked = true; }));
  await marker.click();
  expect(await page.evaluate(() => window.markerClicked)).toBe(true);
  const markerBox = await marker.boundingBox();
  const canvasBox = await page.locator("[data-photo-map-canvas]").boundingBox();
  await page.mouse.move(canvasBox.x + 250, canvasBox.y + 550);
  await page.mouse.down();
  await page.mouse.move(canvasBox.x + 290, canvasBox.y + 565);
  await page.mouse.up();
  await expect.poll(async () => (await marker.boundingBox()).x).toBeCloseTo(markerBox.x + 40, 1);
});

test("map geometry matches direct projection through world wraps, zoom and resize", async ({ page }) => {
  await mapPage(page, [[0, -180], [1, -179], [2, -90], [3, 0], [4, 90], [5, 179], [6, 180]], true);
  async function checkGeometry() {
    const error = await page.evaluate(() => {
      const expected = window.BearStack.photos.map.projector(window.currentMapState, window.currentMapSize);
      const overlay = document.querySelector(".photo-map-overlay"), matrix = new DOMMatrix(getComputedStyle(overlay).transform);
      let error = 0;
      for (const marker of overlay.querySelectorAll("button")) {
        const point = expected({ lat: Number(marker.dataset.lat), lon: Number(marker.dataset.lon) });
        error = Math.max(error, Math.abs(parseFloat(marker.style.left) + matrix.e - point.x), Math.abs(parseFloat(marker.style.top) + matrix.f - point.y));
      }
      const polyline = overlay.querySelector("polyline");
      const source = overlay.querySelector("svg").dataset.points.split(" ");
      for (let i = 0; i < source.length; i++) {
        const [lat, lon] = source[i].split(",").map(Number), point = expected({ lat, lon });
        const actual = polyline.points.getItem(i);
        error = Math.max(error, Math.abs(actual.x + matrix.e - point.x), Math.abs(actual.y + matrix.f - point.y));
      }
      return error;
    });
    expect(error).toBeLessThan(.03);
  }
  const canvas = page.locator("[data-photo-map-canvas]");
  for (let i = 0; i < 35; i++) {
    const before = await page.evaluate(() => window.mapRenders);
    await canvas.press(i < 20 ? "ArrowRight" : "ArrowLeft");
    await expect.poll(() => page.evaluate(() => window.mapRenders)).toBe(before + 1);
    await checkGeometry();
  }
  for (const selector of ["[data-photo-map-zoom-in]", "[data-photo-map-zoom-out]"]) {
    const before = await page.evaluate(() => window.mapRenders);
    await page.locator(selector).click();
    await expect.poll(() => page.evaluate(() => window.mapRenders)).toBe(before + 1);
    await checkGeometry();
  }
  const before = await page.evaluate(() => window.mapRenders);
  await canvas.evaluate(node => { node.style.width = "640px"; window.dispatchEvent(new Event("resize")); });
  await expect.poll(() => page.evaluate(() => window.mapRenders)).toBe(before + 1);
  await checkGeometry();
});
