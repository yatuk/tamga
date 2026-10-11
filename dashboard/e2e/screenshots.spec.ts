import { mkdirSync } from "node:fs";
import path from "node:path";
import { expect, test, type Page } from "@playwright/test";
import { mockProxy, withAdminKey } from "./fixtures";
import { NOW, OPENED, ORDINARY, PLAYGROUND_PROMPT } from "./screenshots.data";

/**
 * The pictures of the console in the README, written to docs/images.
 *
 * Not a test of anything, and not run with the others: it runs only when
 * asked for, against the console on port 3000.
 *
 *   E2E_SCREENSHOTS=1 E2E_BROWSER_CHANNEL=msedge npx playwright test screenshots
 *
 * Every answer comes from e2e/screenshots.data.ts and the browser's clock is
 * set to that file's NOW, so the pictures hold no real data and come out the
 * same on every run.
 */

const OUT = path.join(process.cwd(), "..", "docs", "images");

test.skip(!process.env.E2E_SCREENSHOTS, "set E2E_SCREENSHOTS=1 to take the pictures");

test.use({
  // Tall enough for the whole side navigation, which is as tall as the window and no taller.
  viewport: { width: 1280, height: 960 },
  // The README shows a picture at most 900 px wide: 1920 px of it stays sharp there and keeps each file under 400 KB.
  deviceScaleFactor: 1.5,
  colorScheme: "light",
  locale: "en-GB",
  timezoneId: "UTC",
  // Nothing is caught halfway through arriving.
  contextOptions: { reducedMotion: "reduce" },
});

test.beforeAll(() => {
  mkdirSync(OUT, { recursive: true });
});

test.beforeEach(async ({ page }) => {
  await page.clock.setFixedTime(NOW);
  await withAdminKey(page);
  // The live feed is a response that stays open, which a mocked route cannot
  // give: it is answered in the page, with a stream that says nothing more.
  await page.addInitScript(() => {
    const real = window.fetch.bind(window);
    window.fetch = (input, init) => {
      const url = typeof input === "string" ? input : input instanceof URL ? input.href : input.url;
      if (!url.includes("/api/v1/live/")) return real(input, init);
      const body = new ReadableStream<Uint8Array>({
        start(controller) {
          controller.enqueue(new TextEncoder().encode(": connected\n\n"));
        },
      });
      return Promise.resolve(new Response(body, { status: 200, headers: { "content-type": "text/event-stream" } }));
    };
  });
  await mockProxy(page, ORDINARY);
});

/**
 * Writes the page as it stands, without the Next dev overlay's badge: whole,
 * or only what the window shows when the page is a list hundreds of rows long.
 */
async function shoot(page: Page, name: string, whole = true) {
  await expect(page.getByRole("status", { name: "Loading", exact: true })).toHaveCount(0);
  await expect(page.locator(".cds--skeleton, .cds--skeleton__text")).toHaveCount(0);
  await page.evaluate(async () => {
    document.querySelectorAll("nextjs-portal").forEach((el) => el.remove());
    await document.fonts.ready;
    // Nothing is left pointed at or focused from the steps before.
    if (document.activeElement instanceof HTMLElement) document.activeElement.blur();
  });
  await page.mouse.move(0, 0);
  // Counts and bars settle.
  await page.waitForTimeout(800);
  // A list is cut under the last row the window shows whole, not through one.
  const clip = whole
    ? undefined
    : await page.evaluate(() => {
        const bottoms = [...document.querySelectorAll(".t-work tbody tr")].map((row) => Math.ceil(row.getBoundingClientRect().bottom));
        const height = Math.max(...bottoms.filter((bottom) => bottom <= window.innerHeight));
        return { x: 0, y: 0, width: window.innerWidth, height };
      });
  await page.screenshot({ path: path.join(OUT, `${name}.png`), fullPage: whole, clip, animations: "disabled" });
}

const title = (page: Page, name: string) => page.getByRole("heading", { level: 1, name, exact: true });

test("Overview", async ({ page }) => {
  await page.goto("/dashboard");
  await expect(title(page, "Overview")).toBeVisible();
  await expect(page.getByText(/stopped before they reached a provider/)).toBeVisible();
  await expect(page.getByRole("heading", { name: "Requests by day" })).toBeVisible();
  await shoot(page, "console-overview");
});

test("Event explorer, with one request open", async ({ page }) => {
  // Tall enough for the open request's findings, triage and request sections; the raw event below them is left out.
  await page.setViewportSize({ width: 1280, height: 1100 });
  await page.goto(`/dashboard/events?request_id=${OPENED}`);
  await expect(title(page, "Event explorer")).toBeVisible();
  const detail = page.getByRole("complementary", { name: "Request detail" });
  await expect(detail.getByRole("heading", { name: "Instruction override" })).toBeVisible();
  await expect(detail.getByText("example-policy, version 1.4")).toBeVisible();
  await expect(page.getByText("Live", { exact: true })).toBeVisible();
  await shoot(page, "console-event-detail", false);
});

test("Incidents", async ({ page }) => {
  await page.goto("/dashboard/security");
  await expect(title(page, "Incidents")).toBeVisible();
  await expect(page.locator(".t-lede")).toContainText(/The proxy acted on [\d,]+ requests in the last 7 days: [\d,]+ stopped, [\d,]+ redacted, [\d,]+ warned\./);
  await expect(page.locator(".t-lede")).toContainText(/The latest 200 of [\d,]+ are listed/);
  await expect(page.locator(".t-work tbody tr")).toHaveCount(200);
  await shoot(page, "console-incidents", false);
});

test("Policies, the editor", async ({ page }) => {
  await page.goto("/dashboard/policies");
  await expect(title(page, "Policies")).toBeVisible();
  await expect(page.locator(".cm-content")).toContainText("example-policy");
  await shoot(page, "console-policy-editor");
});

test("Playground, with a verdict", async ({ page }) => {
  await page.goto("/dashboard/playground");
  await expect(title(page, "Playground")).toBeVisible();
  await expect(page.locator("section[aria-labelledby='policy']")).toContainText("example-policy");
  await page.getByRole("textbox", { name: "Prompt" }).fill(PLAYGROUND_PROMPT);
  await page.getByRole("button", { name: "Run it against the policy" }).click();
  // Both findings are marked where the proxy placed them: the address it masked as well.
  await expect(page.locator(".t-sample mark")).toHaveText(["billing@example.com", "Ignore previous instructions"]);
  await shoot(page, "console-playground");
});

test("Traffic", async ({ page }) => {
  await page.goto("/dashboard/traffic");
  await expect(title(page, "Traffic")).toBeVisible();
  await expect(page.getByRole("heading", { name: "Requests by provider" })).toBeVisible();
  await shoot(page, "console-traffic");
});
