import { test, expect } from "@playwright/test";

test.describe("Traffic Analytics", () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem("tamga_admin_key", "test-admin-key");
    });
  });

  test("page loads with metric cards", async ({ page }) => {
    await page.goto("/dashboard/traffic");
    await expect(page.getByRole("heading", { name: /traffic/i })).toBeVisible();
  });

  test("time range switch updates content", async ({ page }) => {
    await page.goto("/dashboard/traffic");
    // Click 24h range button
    const btn24h = page.getByRole("button", { name: "24h" });
    await btn24h.click();
    await expect(btn24h).toHaveClass(/bg-emerald-600/);
  });
});
