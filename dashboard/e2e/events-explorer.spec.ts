import { test, expect } from "@playwright/test";

test.describe("Event Explorer", () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem("tamga_admin_key", "test-admin-key");
    });
  });

  test("page loads and shows header", async ({ page }) => {
    await page.goto("/dashboard/events");
    await expect(page.getByRole("heading", { name: /event explorer/i })).toBeVisible();
  });

  test("renders events table with data", async ({ page }) => {
    test.skip(!!process.env.CI, "requires backend proxy");
    await page.goto("/dashboard/events?range=7d");
    // Table or data should appear
    await expect(page.locator('[role="grid"]')).toBeVisible({ timeout: 10_000 });
  });

  test("filter by block action updates URL", async ({ page }) => {
    await page.goto("/dashboard/events");
    await page.getByRole("button", { name: "block", exact: true }).click();
    await expect(page).toHaveURL(/action=block/);
  });

  test("URL with filters pre-applies them", async ({ page }) => {
    test.skip(!!process.env.CI, "requires backend proxy");
    await page.goto("/dashboard/events?action=block&range=7d");
    // Page should load with filters applied
    await expect(page.locator('[role="grid"]')).toBeVisible({ timeout: 10_000 });
    await expect(page).toHaveURL(/action=block/);
  });
});
