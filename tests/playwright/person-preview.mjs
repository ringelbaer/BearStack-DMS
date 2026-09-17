import { expect } from "@playwright/test";

export async function expectPersonPreview(dialog, side) {
  const image = dialog.locator("[data-person-preview-image]");
  await expect(image).toHaveAttribute("src", "/photos/people/groups/image/" + await side.getAttribute("data-side-face-id"));
  await expect(image).toBeVisible();
  await expect.poll(() => image.evaluate(img => img.complete && img.naturalWidth > 0)).toBe(true);
  await expect(dialog.locator("[data-person-preview-path]")).toHaveText(await side.getAttribute("data-display-path"));
  await expect(dialog.locator("[data-person-preview-box]")).toBeVisible();
  // With a small face, one dimension of the visible context spans three face boxes.
  await expect.poll(() => dialog.locator("[data-person-preview]").evaluate(frame => {
    const area = frame.getBoundingClientRect();
    const box = frame.querySelector("[data-person-preview-box]").getBoundingClientRect();
    return Math.min(Math.abs(3 * box.width - area.width), Math.abs(3 * box.height - area.height));
  })).toBeLessThan(.1);
  expect(await image.evaluate(img => new DOMMatrix(getComputedStyle(img).transform).a)).toBeGreaterThan(1);
}
