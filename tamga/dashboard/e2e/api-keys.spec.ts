import { test, expect } from "@playwright/test";

test.describe("API Keys", () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem("tamga_admin_key", "test-admin-key");
    });
  });

  test("page loads and shows keys table", async ({ page }) => {
    await page.goto("/dashboard/keys");
    await expect(page.getByRole("heading", { name: /api keys/i })).toBeVisible();
  });

  test("create key dialog opens", async ({ page }) => {
    await page.goto("/dashboard/keys");
    const newBtn = page.getByRole("button", { name: /new api key/i });
    await newBtn.click();
    await expect(page.getByText(/create api key/i)).toBeVisible();
  });
});
