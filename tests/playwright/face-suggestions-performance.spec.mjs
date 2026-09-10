import { expect, test } from "@playwright/test";
import { readFile } from "node:fs/promises";

// Isolate the shipped modal renderer from server/database and image-decoding
// costs. Synthetic snapshots model rankings improving after every 32 groups.
test("face-match modal keeps at most 20 options while consuming many snapshots", async ({ page }) => {
  test.skip(process.env.BEARSTACK_FACE_SUGGESTION_PERF !== "1", "opt-in modal rendering audit");
  await loadModal(page);
  for (const width of [1440, 390]) {
    await page.setViewportSize({ width, height: 900 });
    const result = await page.evaluate(async () => {
      const form = document.querySelector("[data-person-picker]");
      form.dataset.personFaceId = "1";
      form.dataset.personExclude = "1";
      let controller;
      window.fetch = async url => String(url).includes("/faces/1/suggestions")
        ? new Response(new ReadableStream({ start(c) { controller = c; } }), { headers: { "Content-Type": "application/x-ndjson" } })
        : new Response(JSON.stringify({ people: [] }), { headers: { "Content-Type": "application/json" } });
      const dialog = document.querySelector("dialog");
      if (!dialog.open) dialog.showModal();
      const input = document.querySelector("[data-person-search]"); input.focus();
      const list = document.querySelector("[data-person-options]");
      let maxOptions = 0, renders = 0;
      const observer = new MutationObserver(records => { renders += records.length; maxOptions = Math.max(maxOptions, list.children.length); });
      observer.observe(list, { childList: true });
      const frames = 315;
      const encoded = [];
      const encoder = new TextEncoder();
      for (let frame = 0; frame < frames; frame++) {
        const people = Array.from({ length: 20 }, (_, i) => ({ id: 2 + frame * 32 + i, name: `Person ${frame * 32 + i}`, count: 10, face_id: 2 + frame * 32 + i }));
        encoded.push(encoder.encode(JSON.stringify({ people, done: frame === frames - 1 }) + "\n"));
      }
      const start = performance.now();
      document.querySelector("[data-person-face-match]").click();
      while (!controller) await new Promise(resolve => setTimeout(resolve, 0));
      // Separate stream chunks, already buffered: worst burst after buffering
      // in a proxy or while a background tab was not scheduled.
      for (const bytes of encoded) controller.enqueue(bytes);
      controller.close();
      await new Promise(resolve => {
        const check = () => document.querySelector("[data-person-face-match]").hasAttribute("aria-busy") ? setTimeout(check, 0) : resolve();
        check();
      });
      const elapsed = performance.now() - start;
      observer.disconnect();
      return { elapsed, maxOptions, renders, finalOptions: list.children.length, frames, feedback: document.querySelector("[data-person-feedback]").textContent };
    });
    expect(result.maxOptions).toBeLessThanOrEqual(20);
    expect(result.finalOptions).toBe(20);
    expect(result.renders).toBeLessThanOrEqual(3);
    expect(result.feedback).toBe("20 Vorschläge verfügbar.");
    console.log(JSON.stringify({ modal_render_width: width, ...result }));
  }
});

async function loadModal(page) {
  const template = await readFile(new URL("../../internal/server/templates/person_edit_dialog.html", import.meta.url), "utf8");
  const markup = template.slice(template.indexOf("<dialog"), template.indexOf("</dialog>") + 9);
  const script = await readFile(new URL("../../internal/server/static/app-person-dialog.js", import.meta.url), "utf8");
  const styles = await readFile(new URL("../../internal/server/static/app.css", import.meta.url), "utf8");
  await page.route("**/*", route => route.request().resourceType() === "document"
    ? route.fulfill({ contentType: "text/html", body: `<!doctype html><meta charset="utf-8"><style>${styles}</style>${markup}<script>${script}</script>` })
    : route.fulfill({ status: 204, body: "" }));
  await page.goto("http://bearstack-perf.test/");
}

for (const outcome of ["cancel", "error", "final"]) {
  test(`queued face-match rendering respects ${outcome}`, async ({ page }) => {
    await loadModal(page);
    await page.evaluate(() => {
      window.requestAnimationFrame = callback => { window.queuedFaceFrame = callback; return 1; };
      window.cancelAnimationFrame = () => { window.faceFrameCancelled = true; };
      window.fetch = async (url, options) => {
        if (!String(url).includes("/faces/1/suggestions")) return new Response(JSON.stringify({ people: [] }));
        const stream = new ReadableStream({ start(c) { window.faceTestStream = c; } });
        options.signal.addEventListener("abort", () => { try { window.faceTestStream.error(new DOMException("Aborted", "AbortError")); } catch (_) {} });
        return new Response(stream, { headers: { "Content-Type": "application/x-ndjson" } });
      };
      document.querySelector("[data-person-picker]").dataset.personFaceId = "1";
      document.querySelector("dialog").showModal();
    });
    await page.getByRole("button", { name: "Ähnliche benannte Personen suchen" }).click();
    await page.evaluate(() => window.faceTestStream.enqueue(new TextEncoder().encode(JSON.stringify({ people: [{ id: 2, name: "Vorläufig", face_id: 2, count: 1 }], done: false }) + "\n")));
    await expect.poll(() => page.evaluate(() => typeof window.queuedFaceFrame)).toBe("function");
    await expect(page.getByRole("option")).toHaveCount(0);
    if (outcome === "cancel") {
      await page.getByRole("combobox").fill("Neuer Name");
    } else {
      await page.evaluate(outcome => window.faceTestStream.enqueue(new TextEncoder().encode(JSON.stringify(outcome === "error"
        ? { people: [], done: true, error: "Abgleichfehler" }
        : { people: [{ id: 3, name: "Endergebnis", face_id: 3, count: 1 }], done: true }) + "\n")), outcome);
      await expect(page.locator("[data-person-feedback]")).toContainText(outcome === "error" ? "Abgleichfehler" : "1 Vorschlag verfügbar");
    }
    expect(await page.evaluate(() => window.faceFrameCancelled)).toBe(true);
    await page.evaluate(() => window.queuedFaceFrame());
    await expect(page.getByRole("option")).toHaveCount(outcome === "final" ? 1 : 0);
    if (outcome === "final") await expect(page.getByRole("option")).toContainText("Endergebnis");
    if (outcome === "cancel") await expect(page.getByRole("combobox")).toHaveValue("Neuer Name");
  });
}
