import { expect, test } from "@playwright/test";
import { mockProxy, withAdminKey } from "./fixtures";

test.describe("Traffic Analytics", () => {
  test.beforeEach(async ({ page }) => {
    await withAdminKey(page);
    await mockProxy(page);
  });

  test("page loads with metric cards", async ({ page }) => {
    await page.goto("/dashboard/traffic");
    await expect(page.getByRole("heading", { name: /traffic/i, level: 1 })).toBeVisible();
  });

  test("time range switch updates content", async ({ page }) => {
    await page.goto("/dashboard/traffic");
    const range24h = page.getByRole("radio", { name: "24h" });
    await range24h.click();
    await expect(range24h).toBeChecked();
    // The window is part of the URL, so the view can be shared.
    await expect(page).toHaveURL(/range=24h/);
  });
});
