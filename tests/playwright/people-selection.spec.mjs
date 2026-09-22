import { expect, test } from "@playwright/test";
import { fileURLToPath } from "node:url";

const controlsPath = fileURLToPath(new URL("../../internal/server/static/app-people-controls.js", import.meta.url));
const views = [
  { name: "Personenübersicht", card: "person-overview-card", input: "data-person-select", tag: "a", open: "person-card" },
  { name: "Personendetails", card: "person-face-card", input: "data-detail-select", tag: "button", open: "person-photo-button" },
  { name: "Ignorierte Gesichter", card: "ignored-face-card", input: "data-ignored-select", tag: "div", open: "ignored-face-card" },
];

for (const view of views) {
  test(`${view.name}: Shift selects a row-ordered range with pointer, checkboxes and keyboard`, async ({ page }) => {
    await page.setContent(`<style>
      #grid[data-selection-mode="true"] { user-select:none; }
      #grid { display:grid; grid-template-columns:repeat(4,minmax(0,1fr)); gap:8px; }
      article { min-height:70px; border:1px solid; padding:8px; }
      article > a, article > button { display:block; min-height:40px; }
      @media(max-width:600px) { #grid { grid-template-columns:repeat(2,minmax(0,1fr)); } }
    </style>
    <button id="mode">Auswahlmodus</button><button id="all" data-selection-label="Alle dieser Seite auswählen"></button><button id="clear">Auswahl aufheben</button>
    <output id="count"></output><div id="grid">${Array.from({ length: 12 }, (_, i) => `<article class="${view.card}" data-person-name="Foto ${i + 1}">
      <label><input type="checkbox" ${view.input} value="${i + 1}">Foto ${i + 1} auswählen</label>
      ${view.tag === "div" ? `<span>Foto ${i + 1}</span>` : `<${view.tag} class="${view.open}" ${view.tag === "a" ? 'href="#opened"' : 'type="button"'}>Foto ${i + 1}</${view.tag}>`}
    </article>`).join("")}</div>`);
    await page.addScriptTag({ path: controlsPath });
    await page.evaluate(view => {
      window.selectionBlocked = false;
      window.changes = 0;
      const grid = document.querySelector("#grid");
      window.selection = window.BearStackPeopleControls.bindSelection({
        grid, button: document.querySelector("#mode"), all: document.querySelector("#all"), clear: document.querySelector("#clear"),
        input: `[${view.input}]`, card: `.${view.card}`, open: `.${view.open}`,
        blocked: () => window.selectionBlocked,
        changed: () => { window.changes++; document.querySelector("#count").textContent = grid.querySelectorAll("input:checked").length; },
      });
    }, view);
    const cards = page.locator("article");
    const inputs = cards.locator("input");
    const target = index => view.tag === "div" ? cards.nth(index) : cards.nth(index).locator(`.${view.open}`);
    const selected = async values => expect.poll(() => inputs.evaluateAll(items => items.filter(item => item.checked).map(item => Number(item.value)))).toEqual(values);
    const click = (index, shift = false) => target(index).click({ modifiers: shift ? ["Shift"] : [], ...(view.tag === "div" ? { position: { x: 10, y: 65 } } : {}) });
    await page.locator("#mode").click();
    for (const width of [1000, 390]) {
      await page.setViewportSize({ width, height: 900 });
      await page.locator("#clear").click();
      await click(1);
      const first = await target(1).boundingBox(), last = await target(6).boundingBox();
      expect(last.y).toBeGreaterThan(first.y); // Range really crosses a row boundary.
      await click(6, true);
      await selected([2, 3, 4, 5, 6, 7]);
      await expect(page.locator("#count")).toHaveText("6");
      await click(3, true); // A checked endpoint must stay selected.
      await selected([2, 3, 4, 5, 6, 7]);
      await click(0, true); // Continue from the most recently clicked selection.
      await selected([1, 2, 3, 4, 5, 6, 7]);
      await click(11); // Preserve a disjoint selection outside the next range.
      await click(8, true);
      await selected([1, 2, 3, 4, 5, 6, 7, 9, 10, 11, 12]);
    }
    await page.locator("#clear").click();
    await click(9, true); // No anchor: select just this card.
    await selected([10]);
    await click(3, true); // Reverse direction.
    await selected([4, 5, 6, 7, 8, 9, 10]);

    // Native checkbox activation toggles before click; Shift must not undo endpoints.
    await page.locator("#clear").click();
    await inputs.nth(2).click();
    await inputs.nth(6).click({ modifiers: ["Shift"] });
    await selected([3, 4, 5, 6, 7]);
    await expect(page.locator("#count")).toHaveText("5");
    await inputs.nth(3).click({ modifiers: ["Shift"] });
    await selected([3, 4, 5, 6, 7]);
    await expect(page.locator("#count")).toHaveText("5");
    await cards.nth(0).locator("label").click({ modifiers: ["Shift"], position: { x: 40, y: 8 } });
    await selected([1, 2, 3, 4, 5, 6, 7]);

    // Enter and Space retain Shift and perform only one selection operation.
    await page.locator("#clear").click();
    await target(2).press("Enter");
    const changes = await page.evaluate(() => window.changes);
    await target(7).press("Shift+Space");
    await selected([3, 4, 5, 6, 7, 8]);
    expect(await page.evaluate(() => window.changes)).toBe(changes + 1);
    await inputs.nth(9).press("Shift+Enter");
    await selected([3, 4, 5, 6, 7, 8, 9, 10]);

    // Disabling the mode and select-all/clear reset the anchor, not unrelated selections.
    await page.locator("#mode").click(); await page.locator("#mode").click();
    await click(0, true);
    await selected([1, 3, 4, 5, 6, 7, 8, 9, 10]);
    await page.locator("#all").click(); await page.locator("#all").click();
    await click(11, true); await selected([12]);

    // Removed/replaced cards cannot retain an old anchor after AJAX refresh.
    await cards.nth(11).evaluate(card => card.replaceWith(card.cloneNode(true)));
    await page.evaluate(() => window.selection.update());
    await click(0, true); await selected([1, 12]);
    await page.locator("#clear").click();
    await click(0);
    await inputs.nth(0).click(); // Deselect the anchor.
    await click(5, true); await selected([6]);

    // Selection lock also applies to range actions during writes or uncertain responses.
    await page.evaluate(() => { window.selectionBlocked = true; window.selection.update(); });
    await target(9).dispatchEvent("click", { shiftKey: true });
    await target(9).dispatchEvent("keydown", { key: "Enter", shiftKey: true });
    await selected([6]);
    expect(await page.evaluate(() => location.hash)).toBe("");
  });
}
