import { expect, test } from "@playwright/test";
import { mockProxy, withAdminKey } from "./fixtures";

test.describe("API Keys", () => {
  test.beforeEach(async ({ page }) => {
    await withAdminKey(page);
    await mockProxy(page);
  });

  test("page loads and shows keys table", async ({ page }) => {
    await page.goto("/dashboard/keys");
    await expect(page.getByRole("heading", { name: /api keys/i, level: 1 })).toBeVisible();
  });

  test("create key dialog opens", async ({ page }) => {
    await page.goto("/dashboard/keys");
    const newBtn = page.getByRole("button", { name: /new api key/i });
    await newBtn.click();
    await expect(page.getByText(/create api key/i)).toBeVisible();
  });
});
