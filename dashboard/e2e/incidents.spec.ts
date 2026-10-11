import { expect, test, type Page } from "@playwright/test";
import { mockProxy, withAdminKey } from "./fixtures";
import { HOSTILE } from "./hostile";

/**
 * The incident queue, against fixed answers (e2e/fixtures.ts).
 *
 * Opening an incident nobody has touched is the ordinary case, and it used to
 * put an error in the browser's console: the proxy answered 404 for its
 * triage state, and the console read that as "open". The proxy now answers
 * that it is open. Nothing on this page may write an error to the console, and
 * no call it makes to the proxy may be answered with an error.
 */

/** Everything the browser would show in red in its console while `page` is open. */
function consoleErrors(page: Page): string[] {
  const seen: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "error") seen.push(`console: ${message.text()}`);
  });
  page.on("pageerror", (error) => seen.push(`uncaught: ${error.message}`));
  // A failed call is an error line of its own in the console, whatever the page does with it.
  page.on("response", (response) => {
    const url = new URL(response.url());
    if (url.pathname.includes("/api/v1/") && response.status() >= 400) seen.push(`${response.status()} ${url.pathname}`);
  });
  return seen;
}

const title = (page: Page) => page.getByRole("heading", { level: 1, name: "Incidents", exact: true });
const rows = (page: Page) => page.getByRole("table").locator("tbody tr");

/** Opens the nth request. A row drawn by the server takes a click only once the page has woken up, so it is asked until the panel is there. */
async function open(page: Page, nth: number) {
  await expect(async () => {
    if ((await page.getByRole("complementary", { name: "Request detail" }).count()) === 0) await rows(page).nth(nth).click();
    await expect(page.getByRole("complementary", { name: "Request detail" })).toBeVisible({ timeout: 2_000 });
  }).toPass({ timeout: 30_000 });
}

test.beforeEach(async ({ page }) => {
  await withAdminKey(page);
});

// The dev server compiles the page the first time it is asked for.
test.describe.configure({ timeout: 120_000 });

test("opening an incident nobody has touched reads it as open, with no error in the console", async ({ page }) => {
  const errors = consoleErrors(page);
  await mockProxy(page);
  await page.goto("/dashboard/security", { timeout: 90_000 });
  await expect(title(page)).toBeVisible({ timeout: 60_000 });

  // 8 stopped and 8 redacted of the fixture's 24 requests: the proxy's count, and every one is listed.
  await expect(page.getByText(/^The proxy acted on 16 requests in the last 7 days: 8 stopped, 8 redacted\. They are listed newest first\./)).toBeVisible({
    timeout: 20_000,
  });
  await expect(rows(page)).toHaveCount(16);
  await expect(page.getByText("16 listed", { exact: true })).toBeVisible();

  // Each of the first few, in turn: every one asks the proxy for a triage state that was never written.
  for (let i = 0; i < 3; i++) {
    await open(page, i);
    const panel = page.getByRole("complementary", { name: "Request detail" });
    await expect(panel.getByText("Open, nobody has looked at this yet.")).toBeVisible({ timeout: 15_000 });
    await expect(panel.getByRole("button", { name: "Close as handled" })).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(panel).toHaveCount(0);
  }

  expect(errors, errors.join("\n")).toEqual([]);
});

test("a state the proxy could not give is said to be unread, and is not shown as open", async ({ page }) => {
  await mockProxy(page, { "/incidents/*": () => ({ status: 500, json: { error: "boom" } }) });
  await page.goto("/dashboard/security", { timeout: 90_000 });
  await expect(rows(page).first()).toBeVisible({ timeout: 60_000 });
  const panel = page.getByRole("complementary", { name: "Request detail" });
  await open(page, 0);
  await expect(panel.getByText("The triage state could not be read from the proxy.")).toBeVisible({ timeout: 20_000 });
  await expect(panel.getByText("Open, nobody has looked at this yet.")).toHaveCount(0);
  await expect(panel.getByRole("button", { name: "Close as handled" })).toHaveCount(0);
});

test("a queue longer than the page lists says how many there are and how many it shows", async ({ page }) => {
  const errors = consoleErrors(page);
  // A proxy that holds over 800,000 incidents and returns the latest of them.
  await mockProxy(page, HOSTILE);
  await page.goto("/dashboard/security", { timeout: 90_000 });
  await expect(title(page)).toBeVisible({ timeout: 60_000 });

  const lede = page.getByText(/^The proxy acted on/);
  await expect(lede).toBeVisible({ timeout: 20_000 });
  const listed = await rows(page).count();
  expect(listed).toBeGreaterThan(0);
  // The figure is the proxy's, not the number of rows on the page.
  const said = (await lede.textContent()) ?? "";
  const total = Number(/acted on ([\d,]+) requests/.exec(said)?.[1].replace(/,/g, ""));
  expect(total).toBeGreaterThan(listed);
  expect(said).toContain(`The latest ${listed} of ${total.toLocaleString("en")} are listed, newest first.`);
  await expect(page.getByText(`Latest ${listed} of ${total.toLocaleString("en")}`, { exact: true })).toBeVisible();

  expect(errors, errors.join("\n")).toEqual([]);
});
