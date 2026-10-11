import { expect, test, type Page } from "@playwright/test";
import { EMPTY, mockProxy, mockProxyDown, roleTooLow, withAdminKey } from "./fixtures";

/**
 * The states a real proxy does not produce when a test asks for them: it is
 * down, it refuses the key's role, it has scanned nothing, a read fails or is
 * slow, a model has no price. Each is answered by e2e/fixtures.ts.
 *
 * What these have in common is the rule they hold the console to: a figure
 * is only drawn from an answer. No answer is never shown as 0.
 */

/** The header's one word on the proxy. */
const shellState = (page: Page) => page.getByRole("status").filter({ hasText: /^(Checking the proxy|Proxy up|Proxy unreachable)$/ });
/** The banner over every page while the proxy does not answer. */
const outage = (page: Page) => page.getByText("The proxy is not answering.", { exact: true });
/** The sentences under the Overview's figure, one for an empty window and one for a window with requests. */
const emptyWindow = (page: Page) => page.getByText(/requests went through the proxy in/);
const stoppedOf = (page: Page) => page.getByText(/stopped before (it|they) reached a provider/);
const title = (page: Page, name: string) => page.getByRole("heading", { level: 1, name, exact: true });

// A failed read is asked for again before the console gives up on it.
const SETTLED = { timeout: 15_000 };

test.beforeEach(async ({ page }) => {
  await withAdminKey(page);
});

test.describe("the proxy does not answer", () => {
  test("the header and a banner say so on every page, and no figure is drawn", async ({ page }) => {
    await mockProxyDown(page);
    await page.goto("/dashboard");

    await expect(shellState(page)).toHaveText("Proxy unreachable", SETTLED);
    await expect(outage(page)).toBeVisible();
    await expect(page.getByText(/Requests may be going out unscanned\./)).toBeVisible();

    await expect(title(page, "Overview")).toBeVisible();
    await expect(page.getByText("The proxy did not answer.", { exact: true })).toBeVisible(SETTLED);
    // Nothing was read, so nothing is counted: not even as none.
    await expect(emptyWindow(page)).toHaveCount(0);
    await expect(stoppedOf(page)).toHaveCount(0);

    // The banner leads to the page that says what is known.
    await page.getByRole("button", { name: "Proxy status", exact: true }).click();
    await expect(page).toHaveURL(/\/dashboard\/proxy$/);
    await expect(title(page, "Proxy status")).toBeVisible();
    await expect(page.getByText("The proxy is not answering its health check.")).toBeVisible(SETTLED);
    await expect(outage(page)).toBeVisible();
  });

  test("the request list says the proxy did not answer, not that there were no requests", async ({ page }) => {
    await mockProxyDown(page);
    await page.goto("/dashboard/events");

    await expect(title(page, "Event explorer")).toBeVisible();
    await expect(page.getByText("The proxy did not answer.", { exact: true })).toBeVisible(SETTLED);
    await expect(page.getByText("No request went through in this window.")).toHaveCount(0);
    await expect(page.getByText(/There are none yet\./)).toHaveCount(0);
  });
});

test.describe("the key's role is too low", () => {
  test("a list the role cannot read says which permission the proxy asked for", async ({ page }) => {
    await mockProxy(page, { "/apikeys": () => roleTooLow("apikeys:read") });
    await page.goto("/dashboard/keys");

    await expect(title(page, "API keys")).toBeVisible();
    await expect(page.getByText("This key's role cannot read API keys.", { exact: true })).toBeVisible(SETTLED);
    await expect(page.getByText("The proxy asks for the apikeys:read permission. Use a key with a higher role under Settings.")).toBeVisible();

    // The proxy is up and the key is valid: neither is blamed.
    await expect(shellState(page)).toHaveText("Proxy up");
    await expect(outage(page)).toHaveCount(0);
    await expect(page.getByText("The proxy did not accept the admin key.")).toHaveCount(0);
    // A list that was not read is not an empty list.
    await expect(page.getByText(/^No keys yet\./)).toHaveCount(0);
    await expect(page.getByRole("table", { name: "API keys" })).toHaveCount(0);
  });
});

test.describe("the proxy has scanned nothing yet", () => {
  test("the Overview says none in words, in every section", async ({ page }) => {
    await mockProxy(page, EMPTY);
    await page.goto("/dashboard");

    await expect(emptyWindow(page)).toHaveText("requests went through the proxy in the last 7 days.");
    // The figure beside that sentence is a nought, read from the proxy's answer.
    await expect(emptyWindow(page).locator("xpath=..")).toHaveText(/^\s*0\s*requests went through/);
    await expect(stoppedOf(page)).toHaveCount(0);

    await expect(page.getByText("Nothing was found in this window.")).toBeVisible();
    await expect(page.getByText("No request has been stopped yet.")).toBeVisible();
    await expect(page.getByText("No traffic in this window.")).toBeVisible();
    await expect(shellState(page)).toHaveText("Proxy up");
  });

  test("the Event explorer says there are none, with an empty table and no filter blamed", async ({ page }) => {
    await mockProxy(page, EMPTY);
    await page.goto("/dashboard/events");

    await expect(title(page, "Event explorer")).toBeVisible();
    await expect(page.getByText(/newest first\. There are none yet\./)).toBeVisible();
    await expect(page.getByText("No request went through in this window.")).toBeVisible();
    await expect(page.getByText("0 of 0 loaded")).toBeVisible();
    await expect(page.getByText("No request matches these filters.")).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Clear filters" })).toHaveCount(0);
  });
});

test.describe("a read fails while the proxy is up", () => {
  test("the Overview says the proxy is up and did not return the data, and draws no figure", async ({ page }) => {
    await mockProxy(page, { "/stats": () => ({ status: 500, json: { error: "database unavailable" } }) });
    await page.goto("/dashboard");

    await expect(title(page, "Overview")).toBeVisible();
    await expect(page.getByText("The proxy is up, but it did not return this page's data.", { exact: true })).toBeVisible(SETTLED);
    await expect(page.getByText("Proxy status shows whether one of its parts is down.")).toBeVisible();

    await expect(shellState(page)).toHaveText("Proxy up");
    await expect(outage(page)).toHaveCount(0);
    await expect(emptyWindow(page)).toHaveCount(0);
    await expect(stoppedOf(page)).toHaveCount(0);
  });
});

test.describe("a read is slow", () => {
  test("the Overview holds the place of its figures until the answer is there", async ({ page }) => {
    let answer = () => {};
    const waited = new Promise<void>((resolve) => {
      answer = resolve;
    });
    const stats = { total_requests: 24, blocked_requests: 8, redacted_requests: 8, warned_requests: 0, passed_requests: 8, top_categories: { email: 8 } };
    await mockProxy(page, {
      "/stats": async () => {
        await waited;
        return { json: stats };
      },
    });

    try {
      await page.goto("/dashboard");
      await expect(title(page, "Overview")).toBeVisible();
      await expect(page.getByRole("status", { name: "Loading", exact: true })).toBeVisible();
      // The rest of the proxy has answered by now; the figures still wait for their own read.
      await expect(shellState(page)).toHaveText("Proxy up");
      await expect(emptyWindow(page)).toHaveCount(0);
      await expect(stoppedOf(page)).toHaveCount(0);
      await expect(page.getByText("Nothing was found in this window.")).toHaveCount(0);
    } finally {
      answer();
    }

    await expect(stoppedOf(page)).toHaveText("of 24 requests were stopped before they reached a provider.");
    await expect(page.getByRole("status", { name: "Loading", exact: true })).toHaveCount(0);
    await expect(emptyWindow(page)).toHaveCount(0);
  });
});

test.describe("a model the proxy has no price for", () => {
  /** One model the proxy can price and one, newer than its price list, that it cannot. */
  const COSTS = {
    range: "7d",
    daily: [],
    breakdown: [
      { provider: "anthropic", model: "claude-sonnet-4-6", model_family: "claude-4", model_version: "claude-sonnet-4-6", input_tokens: 10000, output_tokens: 2000, input_cost: 0, output_cost: 0, total_cost: 0, currency: "USD", pricing_id: 0, priced: false },
      { provider: "openai", model: "gpt-4o", model_family: "gpt-4o", model_version: "gpt-4o", input_tokens: 5000, output_tokens: 1000, input_cost: 0.014, output_cost: 0.01, total_cost: 0.024, currency: "USD", pricing_id: 1, priced: true },
    ],
    unpriced: [{ provider: "anthropic", model: "claude-sonnet-4-6", tokens: 12000 }],
    total_usd: 0.024,
    mtd_total_usd: 0.024,
    projected_monthly_usd: 0.35,
  };

  test("is shown as unpriced, not as free, and is said to be in none of the totals", async ({ page }) => {
    await mockProxy(page, { "/billing/costs/breakdown": () => ({ json: COSTS }) });
    await page.goto("/dashboard/costs");

    const models = page.getByRole("table", { name: "Cost by model" });
    const priced = models.getByRole("row", { name: /openai\/gpt-4o/ });
    await expect(priced).toContainText("6.0K");
    await expect(priced).toContainText("$0.024");
    await expect(priced).toContainText("100.0%");

    const unpriced = models.getByRole("row", { name: /anthropic\/claude-sonnet-4-6/ });
    await expect(unpriced).toContainText("12.0K");
    await expect(unpriced).toContainText("No price");
    await expect(unpriced).toContainText("Not priced");
    await expect(unpriced).not.toContainText("$");

    await expect(page.getByText("12.0K tokens across 1 model have no price and are in none of the totals on this page.")).toBeVisible();
  });
});
