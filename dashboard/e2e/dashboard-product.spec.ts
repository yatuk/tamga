import { expect, test, type Page } from "@playwright/test";
import { mockProxy, mockProxyDown, withAdminKey } from "./fixtures";

const hasHorizontalOverflow = (page: Page) =>
  page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 1);

/** Every page in the sidebar, with the heading it must show. */
const PAGES: [path: string, heading: RegExp][] = [
  ["/dashboard", /security overview/i],
  ["/dashboard/security", /incidents/i],
  ["/dashboard/hunting", /threat hunting/i],
  ["/dashboard/events", /event explorer/i],
  ["/dashboard/traffic", /traffic/i],
  ["/dashboard/trends", /detection trends/i],
  ["/dashboard/costs", /token costs/i],
  ["/dashboard/latency", /latency/i],
  ["/dashboard/reports", /reports/i],
  ["/dashboard/policies", /policies/i],
  ["/dashboard/playground", /playground/i],
  ["/dashboard/patterns", /patterns/i],
  ["/dashboard/proxy", /proxy status/i],
  ["/dashboard/scanner-pool", /scanner pool/i],
  ["/dashboard/keys", /api keys/i],
  ["/dashboard/integrations", /integrations/i],
  ["/dashboard/audit", /audit log/i],
  ["/dashboard/team", /team/i],
  ["/dashboard/settings", /settings/i],
];

test.describe("product dashboard", () => {
  test("keeps the operational hierarchy and access controls usable on desktop", async ({ page }) => {
    await withAdminKey(page);
    await mockProxy(page);
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.goto("/dashboard");

    await expect(page.getByRole("heading", { name: "Security overview" })).toBeVisible();
    await expect(page.getByRole("radio", { name: "7d" })).toBeChecked();
    await expect(page.getByRole("button", { name: "Refresh" })).toBeVisible();
    // Numbers come from the proxy, not from placeholders.
    await expect(page.getByText(/of 24 requests blocked/)).toBeVisible();
    expect(await hasHorizontalOverflow(page)).toBe(false);
  });

  test("preserves navigation and critical state on mobile", async ({ page }) => {
    await mockProxyDown(page);
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto("/dashboard");

    await expect(page.getByText("PROXY UNREACHABLE")).toBeVisible();
    await page.getByRole("button", { name: "Toggle navigation" }).click();
    await expect(page.getByRole("link", { name: "Incidents" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Event explorer" })).toBeVisible();
    expect(await hasHorizontalOverflow(page)).toBe(false);
  });

  for (const viewport of [
    { name: "desktop", width: 1440, height: 1000 },
    { name: "phone", width: 375, height: 812 },
  ]) {
    test(`every page renders without horizontal overflow on a ${viewport.name}`, async ({ page }) => {
      test.setTimeout(120_000);
      await withAdminKey(page);
      await mockProxy(page);
      await page.setViewportSize({ width: viewport.width, height: viewport.height });

      for (const [path, heading] of PAGES) {
        await page.goto(path);
        await expect(page.getByRole("heading", { level: 1, name: heading }), path).toBeVisible();
        expect(await hasHorizontalOverflow(page), `${path} overflows at ${viewport.width}px`).toBe(false);
      }
    });
  }

  test("a tab is part of the URL and survives a reload", async ({ page }) => {
    await withAdminKey(page);
    await mockProxy(page);
    await page.goto("/dashboard/settings");
    await page.getByRole("tab", { name: "Runtime" }).click();
    await expect(page).toHaveURL(/tab=runtime/);

    await page.reload();
    await expect(page.getByRole("tab", { name: "Runtime" })).toHaveAttribute("aria-selected", "true");
    await expect(page.getByText("Transport and storage")).toBeVisible();
  });
});
