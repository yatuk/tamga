import { expect, test } from "@playwright/test";

test.describe("product dashboard", () => {
  test("keeps the operational hierarchy and access controls usable on desktop", async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.goto("/dashboard");

    await expect(page.getByRole("heading", { name: "Security overview" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Risk disposition" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Operational measures" })).toBeVisible();

    await page.locator("summary").filter({ hasText: /admin access/i }).click();
    await expect(page.getByPlaceholder("X-Tamga-Admin-Key")).toBeVisible();
    await expect(page.getByRole("button", { name: "Connect" })).toBeVisible();

    const hasHorizontalOverflow = await page.evaluate(
      () => document.documentElement.scrollWidth > window.innerWidth + 1,
    );
    expect(hasHorizontalOverflow).toBe(false);
  });

  test("preserves navigation and critical state on mobile", async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto("/dashboard");

    await expect(page.getByText("PROXY UNREACHABLE")).toBeVisible();
    await page.getByRole("button", { name: "Open menu" }).click();
    await expect(page.getByRole("link", { name: "Incidents" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Event explorer" })).toBeVisible();

    const hasHorizontalOverflow = await page.evaluate(
      () => document.documentElement.scrollWidth > window.innerWidth + 1,
    );
    expect(hasHorizontalOverflow).toBe(false);
  });
});
