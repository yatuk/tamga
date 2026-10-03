import { expect, test } from "@playwright/test";
import { mockProxy, withAdminKey } from "./fixtures";

test.describe("Event Explorer", () => {
  test.beforeEach(async ({ page }) => {
    await withAdminKey(page);
    await mockProxy(page);
  });

  test("page loads and shows header", async ({ page }) => {
    await page.goto("/dashboard/events");
    await expect(page.getByRole("heading", { name: /event explorer/i })).toBeVisible();
  });

  test("renders events table with data", async ({ page }) => {
    await page.goto("/dashboard/events?range=7d");
    const grid = page.getByRole("grid");
    await expect(grid).toBeVisible();
    // 24 fixture events: the grid has real rows, not just a frame.
    await expect(grid.getByRole("row").nth(1)).toBeVisible();
    await expect(page.getByText(/of 24 loaded/)).toBeVisible();
  });

  test("filter by block action updates URL", async ({ page }) => {
    await page.goto("/dashboard/events");
    await page.getByRole("button", { name: "block", exact: true }).click();
    await expect(page).toHaveURL(/action=block/);
  });

  test("URL with filters pre-applies them", async ({ page }) => {
    await page.goto("/dashboard/events?action=block&range=7d");
    await expect(page.getByRole("grid")).toBeVisible();
    await expect(page).toHaveURL(/action=block/);
    await expect(page.getByRole("button", { name: "block", exact: true })).toHaveAttribute("aria-pressed", "true");
    // Only the 8 blocked fixture events are listed.
    await expect(page.getByText(/of 8 loaded/)).toBeVisible();
  });

  test("opening an event shows its detail", async ({ page }) => {
    await page.goto("/dashboard/events");
    await page.getByRole("button", { name: /^Open event/ }).first().click();
    const sheet = page.getByRole("dialog");
    await expect(sheet.getByRole("heading", { name: "Event Detail" })).toBeVisible();
    await expect(sheet.getByText("instruction_override")).toBeVisible();
    // Where the text was: role and location in the request body.
    await expect(sheet.getByText("In a tool result or document")).toBeVisible();
    await expect(sheet.getByText("messages[2].content[0].content")).toBeVisible();
  });

  test("an event without findings opens instead of crashing", async ({ page }) => {
    // The proxy sends findings: null for a clean request.
    await page.goto("/dashboard/events?action=pass");
    await page.getByRole("button", { name: /^Open event/ }).first().click();
    await expect(page.getByRole("dialog").getByText("Nothing was detected in this request.")).toBeVisible();
  });
});
