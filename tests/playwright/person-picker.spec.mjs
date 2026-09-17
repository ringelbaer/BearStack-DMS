import { expect, test } from "@playwright/test";
import { readFile } from "node:fs/promises";

async function loadPicker(page) {
  const script = await readFile(new URL("../../internal/server/static/app-person-picker.js", import.meta.url), "utf8");
  await page.route("**/*", async route => {
    if (route.request().resourceType() === "document") return route.fulfill({ contentType: "text/html", body: `
      <form action="/original-action" data-person-modal>
        <input data-person-search required><input data-person-target type="hidden">
        <div data-person-popup hidden><div id="options" data-person-options></div><div data-person-feedback></div></div>
        <button data-person-submit>Original action</button>
      </form><script>${script}</script><script>
      window.changes = []; window.choices = []; window.submits = 0;
      const form = document.querySelector('form');
      form.addEventListener('submit', event => { event.preventDefault(); window.submits++; });
      form.querySelector('input').setCustomValidity('Owned by caller');
      window.picker = BearStackPersonPicker.bind(form, {
        showThumbnails: true,
        onChange: state => changes.push(state), onChoose: state => choices.push(state)
      });
      window.samePicker = BearStackPersonPicker.bind(form) === picker;
      </script>` });
    if (route.request().url().includes("/photos/people?")) return route.fulfill({ json: { people: [
      { id: 42, name: "Ada", revision: 7, count: 2, face_id: 3 },
    ] } });
    return route.fulfill({ status: 204 });
  });
  await page.goto("http://picker.test/");
}

test("person picker works independently without changing or submitting its caller's form", async ({ page }) => {
  await loadPicker(page);
  const input = page.locator("[data-person-search]");
  await input.focus();
  await expect(page.locator("[data-person-option]")).toHaveCount(1);
  await expect(page.locator("[data-person-option] img")).toHaveAttribute("src", "/photos/faces/3/thumbnail");
  await input.press("ArrowDown");
  await input.press("Enter");
  await expect(page.locator("[data-person-target]")).toHaveValue("42");
  await expect(page.locator("[data-person-popup]")).toBeHidden();
  await expect(page.locator("form")).toHaveAttribute("action", "/original-action");
  await expect(page.locator("button")).toHaveText("Original action");
  const result = await page.evaluate(() => ({
    submits, choices, samePicker, validity: document.querySelector("input").validationMessage,
  }));
  expect(result.submits).toBe(0);
  expect(result.samePicker).toBe(true);
  expect(result.validity).toBe("Owned by caller");
  expect(result.choices).toEqual([expect.objectContaining({ target: "42", targetName: "Ada", revision: "7", assigned: true })]);
});

test("reset cancels an in-flight search and reports the new selection", async ({ page }) => {
  await loadPicker(page);
  await page.evaluate(() => {
    // Deliberately ignore the abort signal: even late responses must stay hidden.
    window.fetch = () => new Promise(resolve => { window.finishSearch = resolve; });
    document.querySelector("input").focus();
    picker.reset("New person");
    finishSearch(new Response(JSON.stringify({ people: [{ id: 9, name: "Old person", count: 1 }] }), { headers: { "Content-Type": "application/json" } }));
  });
  await expect(page.locator("[data-person-search]")).toHaveValue("New person");
  await expect(page.locator("[data-person-target]")).toHaveValue("");
  await expect(page.locator("[data-person-popup]")).toBeHidden();
  await expect(page.locator("[data-person-option]")).toHaveCount(0);
  expect(await page.evaluate(() => changes.at(-1))).toMatchObject({ name: "New person", assigned: false, searching: false });
});
