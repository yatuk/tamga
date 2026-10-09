import { readFileSync } from "node:fs";
import path from "node:path";
import { expect, test, type APIRequestContext, type Page } from "@playwright/test";
import { liveOnly, PREFIX, unique } from "./live";

/**
 * Integrations, the audit log and Reports against a real proxy.
 *
 * Destinations created here point at the reserved .invalid TLD, so a test
 * delivery cannot reach anyone: a failed delivery is the expected outcome.
 */

/** NEXT_PUBLIC_* as the dev server reads them; Playwright does not load .env.local. */
function fromEnv(name: string): string {
  if (process.env[name]) return process.env[name] as string;
  try {
    const file = readFileSync(path.join(process.cwd(), ".env.local"), "utf8");
    const line = file.split(/\r?\n/).find((l) => l.startsWith(`${name}=`));
    return line ? line.slice(name.length + 1).trim().replace(/^"|"$/g, "") : "";
  } catch {
    return "";
  }
}

const API = fromEnv("NEXT_PUBLIC_API_URL") || "http://localhost:8443";
const admin = () => ({ "X-Tamga-Admin-Key": fromEnv("NEXT_PUBLIC_ADMIN_KEY") });
const SINK = "https://example.invalid/e2e-live";

type Hook = { id: string; label: string; kind: string; enabled: boolean };

async function listHooks(request: APIRequestContext): Promise<Hook[]> {
  const res = await request.get(`${API}/api/v1/webhooks`, { headers: admin(), timeout: 10_000 });
  expect(res.ok(), "the proxy lists destinations").toBeTruthy();
  return ((await res.json()) as { items?: Hook[] }).items ?? [];
}

async function createHook(request: APIRequestContext, label: string): Promise<Hook> {
  const res = await request.post(`${API}/api/v1/webhooks`, {
    headers: admin(),
    data: { label, kind: "generic", url: SINK, enabled: false },
  });
  expect(res.ok(), "the proxy creates a destination").toBeTruthy();
  const body = (await res.json()) as Hook & { webhook?: Hook };
  return body.webhook ?? body;
}

const deleteHook = (request: APIRequestContext, id: string) =>
  request.delete(`${API}/api/v1/webhooks/${encodeURIComponent(id)}`, { headers: admin() });

/** Removes every destination a test of this folder left behind. */
async function removeLeftovers(request: APIRequestContext) {
  for (const h of await listHooks(request)) {
    if (h.label.startsWith(PREFIX)) await deleteHook(request, h.id);
  }
}

/** Counts the calls that would create a destination. */
function watchCreates(page: Page) {
  const seen: string[] = [];
  page.on("request", (r) => {
    if (r.method() === "POST" && /\/api\/v1\/webhooks$/.test(new URL(r.url()).pathname)) seen.push(r.url());
  });
  return seen;
}

const dialog = (page: Page, name: string) => page.getByRole("dialog", { name: `Connect ${name}`, exact: true });
const connectButton = (page: Page, name: string) => page.getByRole("button", { name: `Connect ${name}`, exact: true });
const submit = (page: Page, name: string) => dialog(page, name).getByRole("button", { name: "Connect", exact: true });

async function openIntegrations(page: Page) {
  await page.goto("/dashboard/integrations");
  await expect(page.getByRole("heading", { name: "Integrations", level: 1 })).toBeVisible();
  await expect(connectButton(page, "Slack")).toBeVisible();
}

test.beforeEach(() => liveOnly());

test.describe("Integrations", () => {
  test.afterEach(async ({ request }) => {
    if (process.env.E2E_LIVE) await removeLeftovers(request);
  });

  test("Connect opens the modal for that destination, and an empty URL is refused in the browser", async ({ page }) => {
    const creates = watchCreates(page);
    await openIntegrations(page);

    await connectButton(page, "Slack").click();
    await expect(dialog(page, "Slack")).toBeVisible();
    await expect(dialog(page, "Slack").getByLabel("Name")).toHaveValue("Slack");

    await submit(page, "Slack").click();
    await expect(dialog(page, "Slack").getByText("Enter the URL to send to.")).toBeVisible();
    await expect(dialog(page, "Slack")).toBeVisible();
    expect(creates).toHaveLength(0);
  });

  test("Cancel closes the modal and the next one opens without the old errors", async ({ page }) => {
    await openIntegrations(page);

    await connectButton(page, "Slack").click();
    await submit(page, "Slack").click();
    await expect(dialog(page, "Slack").getByText("Enter the URL to send to.")).toBeVisible();
    await dialog(page, "Slack").getByRole("button", { name: "Cancel" }).click();
    await expect(dialog(page, "Slack")).toBeHidden();

    // The same destination again, then another one: neither carries the error over.
    await connectButton(page, "Slack").click();
    await expect(dialog(page, "Slack")).toBeVisible();
    await expect(page.getByText("Enter the URL to send to.")).toHaveCount(0);
    await expect(dialog(page, "Slack").getByLabel("URL")).toHaveValue("");
    await page.keyboard.press("Escape");
    await expect(dialog(page, "Slack")).toBeHidden();

    await connectButton(page, "Generic Webhook").click();
    await expect(dialog(page, "Generic Webhook")).toBeVisible();
    await expect(page.getByText("Enter the URL to send to.")).toHaveCount(0);
  });

  test("Show more opens the rest of the list in place and Show fewer closes it", async ({ page }) => {
    await openIntegrations(page);
    await expect(connectButton(page, "Jira Cloud")).toHaveCount(0);

    const more = page.getByRole("button", { name: /^Show \d+ more$/ });
    const count = Number((await more.innerText()).match(/\d+/)?.[0]);
    const before = await page.locator("ul.t-destinations > li").count();
    await more.click();

    await expect(connectButton(page, "Jira Cloud")).toBeVisible();
    await expect(page.locator("ul.t-destinations > li")).toHaveCount(before + count);
    expect(new URL(page.url()).pathname).toBe("/dashboard/integrations");

    await page.getByRole("button", { name: "Show fewer" }).click();
    await expect(connectButton(page, "Jira Cloud")).toHaveCount(0);
    await expect(page.locator("ul.t-destinations > li")).toHaveCount(before);
    await expect(more).toBeVisible();
  });

  test("Jira asks for a project key and an issue type, and will not connect without the key", async ({ page }) => {
    const creates = watchCreates(page);
    await openIntegrations(page);
    await page.getByRole("button", { name: /^Show \d+ more$/ }).click();
    await connectButton(page, "Jira Cloud").click();

    const jira = dialog(page, "Jira Cloud");
    await expect(jira.getByLabel("Project key")).toBeVisible();
    await expect(jira.getByLabel("Issue type")).toBeVisible();

    await jira.getByLabel("URL").fill(SINK);
    await jira.getByLabel("Project key").fill("");
    await submit(page, "Jira Cloud").click();
    await expect(jira.getByText("Jira needs a project key to create issues.")).toBeVisible();
    await expect(jira.getByText("Enter the URL to send to.")).toHaveCount(0);
    expect(creates).toHaveLength(0);
  });

  test("PagerDuty and Opsgenie ask for their key and will not connect without it", async ({ page }) => {
    const creates = watchCreates(page);
    await openIntegrations(page);

    await connectButton(page, "PagerDuty").click();
    const pagerduty = dialog(page, "PagerDuty");
    await expect(pagerduty.getByLabel("Routing key", { exact: true })).toBeVisible();
    await pagerduty.getByLabel("URL").fill(SINK);
    await submit(page, "PagerDuty").click();
    await expect(pagerduty.getByText("PagerDuty rejects events without a routing key.")).toBeVisible();
    await pagerduty.getByRole("button", { name: "Cancel" }).click();
    await expect(pagerduty).toBeHidden();

    await page.getByRole("button", { name: /^Show \d+ more$/ }).click();
    await connectButton(page, "Opsgenie").click();
    const opsgenie = dialog(page, "Opsgenie");
    await expect(opsgenie.getByLabel("API key", { exact: true })).toBeVisible();
    await opsgenie.getByLabel("URL").fill(SINK);
    await submit(page, "Opsgenie").click();
    await expect(opsgenie.getByText("Opsgenie rejects alerts without an API key.")).toBeVisible();

    // A generic webhook has neither field.
    await opsgenie.getByRole("button", { name: "Cancel" }).click();
    await connectButton(page, "Generic Webhook").click();
    await expect(dialog(page, "Generic Webhook").getByLabel("URL")).toBeVisible();
    await expect(page.getByLabel("Routing key", { exact: true })).toHaveCount(0);
    await expect(page.getByLabel("API key", { exact: true })).toHaveCount(0);
    await expect(page.getByLabel("Project key")).toHaveCount(0);
    expect(creates).toHaveLength(0);
  });

  test("a generic webhook is connected, tested and disconnected", async ({ page, request }) => {
    test.setTimeout(90_000);
    const before = await listHooks(request);
    const label = unique("hook");
    await openIntegrations(page);

    await connectButton(page, "Generic Webhook").click();
    const modal = dialog(page, "Generic Webhook");
    await modal.getByLabel("Name").fill(label);
    await modal.getByLabel("URL").fill(SINK);
    await submit(page, "Generic Webhook").click();

    await expect(page.getByText("Destination connected")).toBeVisible();
    await expect(modal).toBeHidden();

    const table = page.getByRole("table", { name: "Connected destinations" });
    const row = table.getByRole("row").filter({ hasText: label });
    await expect(row).toBeVisible();
    await expect(row).toContainText("Generic Webhook");
    await expect(row).toContainText(SINK);
    await expect(row).toContainText("Never");

    const on = before.filter((h) => h.enabled).length + 1;
    const total = before.length + 1;
    await expect(page.locator(".t-lede")).toHaveText(
      `Stopped requests and alerts are sent to ${total} ${total === 1 ? "destination" : "destinations"}, ${on} of them switched on.`,
    );
    const created = (await listHooks(request)).find((h) => h.label === label);
    expect(created, "the proxy has the destination").toBeTruthy();

    // The URL cannot be reached, and the console has to say so in words.
    await row.getByRole("button", { name: "Send a test" }).click();
    await expect(page.getByText("Could not send the test")).toBeVisible({ timeout: 30_000 });
    await expect(page.getByText(/undefined|\[object/)).toHaveCount(0);
    await expect(row.getByRole("button", { name: "Send a test" })).toBeEnabled();

    // Disconnecting asks first; Keep leaves the row alone.
    await row.getByRole("button", { name: `Disconnect ${label}` }).click();
    await row.getByRole("button", { name: "Keep" }).click();
    await expect(row.getByRole("button", { name: `Disconnect ${label}` })).toBeVisible();
    expect((await listHooks(request)).some((h) => h.label === label)).toBeTruthy();

    await row.getByRole("button", { name: `Disconnect ${label}` }).click();
    await row.getByRole("button", { name: "Disconnect", exact: true }).click();
    await expect(page.getByText("Destination disconnected")).toBeVisible();
    await expect(page.getByText(label, { exact: true })).toHaveCount(0);
    expect((await listHooks(request)).some((h) => h.label === label)).toBeFalsy();
  });

  test("?connect=slack opens the Slack modal and leaves a clean URL", async ({ page }) => {
    await page.goto("/dashboard/integrations?connect=slack");
    await expect(dialog(page, "Slack")).toBeVisible();
    await expect(page).toHaveURL(/\/dashboard\/integrations$/);
    await expect(page.getByText("Enter the URL to send to.")).toHaveCount(0);

    // A reload of the cleaned URL does not open it again.
    await page.reload();
    await expect(connectButton(page, "Slack")).toBeVisible();
    await expect(dialog(page, "Slack")).toBeHidden();
  });
});

test.describe("Setup guides", () => {
  for (const [kind, name] of [
    ["slack", "Slack"],
    ["jira", "Jira Cloud"],
    ["pagerduty", "PagerDuty"],
  ] as const) {
    test(`the ${name} guide shows its steps and snippets, and leads to the modal`, async ({ page }) => {
      await page.goto(`/dashboard/integrations/${kind}`);
      await expect(page.getByRole("heading", { level: 1 })).toContainText("Setting up");
      await expect(page).toHaveTitle(/setup · Tamga$/);

      const steps = page.locator("ol.t-steps > li");
      expect(await steps.count()).toBeGreaterThan(1);
      await expect(steps.first().getByRole("heading", { level: 3 })).toBeVisible();

      // The payload preview is always there; it has to carry text, not an empty box.
      const snippets = page.locator(".cds--snippet");
      expect(await snippets.count()).toBeGreaterThan(0);
      expect((await snippets.last().locator("pre").innerText()).trim().length).toBeGreaterThan(10);

      await page.getByRole("link", { name: /^Connect / }).click();
      await expect(dialog(page, name)).toBeVisible();
      await expect(page).toHaveURL(/\/dashboard\/integrations$/);
    });
  }

  test("an unknown kind gives the not-found page", async ({ page }) => {
    await page.goto("/dashboard/integrations/e2e-live-no-such-kind");
    await expect(page.getByText("There is no page here")).toBeVisible();
    await expect(page.getByRole("heading", { level: 1, name: /Setting up/ })).toHaveCount(0);
    await expect(page).toHaveTitle("Integration not found · Tamga");
  });
});

test.describe("Audit log", () => {
  const rows = (page: Page) => page.getByRole("table", { name: "Audit entries" }).locator("tbody tr");
  const panel = (page: Page) => page.getByRole("complementary", { name: "Audit entry" });

  /** "12 of 340 shown" as numbers. */
  async function shown(page: Page): Promise<[number, number]> {
    const text = await page.getByText(/^\d+ of \d+ match$/).innerText();
    const [a, b] = text.match(/\d+/g)!.map(Number);
    return [a, b];
  }

  async function openAudit(page: Page, query = "") {
    await page.goto(`/dashboard/audit${query}`);
    await expect(page.getByRole("heading", { name: "Audit log", level: 1 })).toBeVisible();
    await expect(page.getByRole("table", { name: "Audit entries" })).toBeVisible();
  }

  test.afterEach(async ({ request }) => {
    if (process.env.E2E_LIVE) await removeLeftovers(request);
  });

  test("a change to integrations shows up without a reload, and the open panel survives the refresh", async ({ page, request }) => {
    test.setTimeout(90_000);
    const first = await createHook(request, unique("audit-a"));
    await openAudit(page, `?q=${first.id}`);
    await expect(rows(page)).toHaveCount(1);
    await rows(page).first().click();
    await expect(panel(page)).toContainText(first.label);

    // The log refetches every 15 seconds: the deletion arrives on its own,
    // and the entry being read stays open while the list grows above it.
    await deleteHook(request, first.id);
    await expect(rows(page)).toHaveCount(2, { timeout: 25_000 });
    await expect(rows(page).first()).toContainText("Integration removed");
    await expect(panel(page)).toBeVisible();
    await expect(panel(page).getByRole("heading", { level: 2 })).toHaveText("Integration connected");
    await expect(panel(page)).toContainText(first.label);
    await expect(rows(page).nth(1)).toHaveAttribute("aria-current", "true");
  });

  test("the search box and the two selects filter, the count follows, and Clear filters resets", async ({ page, request }) => {
    const hook = await createHook(request, unique("audit-b"));
    await deleteHook(request, hook.id);
    await openAudit(page);
    await expect(rows(page).first()).toBeVisible();

    const [allShown, all] = await shown(page);
    expect(allShown).toBe(all);
    // The log is read 50 entries at a time.
    expect(await rows(page).count()).toBe(Math.min(all, 50));
    await expect(page.getByRole("button", { name: "Clear filters" })).toHaveCount(0);

    // Search: the target of the two entries just made.
    await page.getByRole("searchbox", { name: "Search the log" }).fill(hook.id);
    await expect(rows(page)).toHaveCount(2);
    await expect(page.getByText(/^2 of \d+ match$/)).toBeVisible();
    await expect(page).toHaveURL(new RegExp(`[?&]q=${hook.id}`));

    // Action: one of the two is left.
    await page.locator("#audit-kind").selectOption("webhook.create");
    await expect(rows(page)).toHaveCount(1);
    await expect(rows(page).first()).toContainText("Integration connected");
    await expect(page.getByText(/^1 of \d+ match$/)).toBeVisible();
    await expect(page).toHaveURL(/[?&]kind=webhook\.create/);

    // Without the search, every row is of that action and the count matches.
    await page.getByRole("searchbox", { name: "Search the log" }).fill("");
    await expect(page).not.toHaveURL(/[?&]q=/);
    const [created] = await shown(page);
    expect(created).toBeGreaterThan(0);
    expect(created).toBeLessThan(all + 1);
    await expect(rows(page)).toHaveCount(Math.min(created, 50));
    await expect(rows(page).filter({ hasText: "Integration connected" })).toHaveCount(Math.min(created, 50));

    await page.getByRole("button", { name: "Clear filters" }).click();
    await expect(page.locator("#audit-kind")).toHaveValue("");
    await expect(page).not.toHaveURL(/[?&](q|kind|actor)=/);
    const [afterClear, total] = await shown(page);
    expect(afterClear).toBe(total);
    await expect(page.getByRole("button", { name: "Clear filters" })).toHaveCount(0);

    // Actor: every recorded actor is offered, and choosing one keeps only their rows.
    const actors = await page.locator("#audit-actor option").evaluateAll((os) => os.map((o) => (o as HTMLOptionElement).value).filter(Boolean));
    for (const actor of actors.slice(0, 1)) {
      await page.locator("#audit-actor").selectOption(actor);
      await expect(page).toHaveURL(/[?&]actor=/);
      const [byActor] = await shown(page);
      await expect(rows(page)).toHaveCount(byActor);
      await expect(rows(page).filter({ hasText: actor })).toHaveCount(byActor);
      await page.getByRole("button", { name: "Clear filters" }).click();
    }

    // A search nothing matches says so.
    await page.getByRole("searchbox", { name: "Search the log" }).fill("e2e-live-matches-nothing");
    await expect(page.getByText("No entry matches these filters.")).toBeVisible();
    await expect(page.getByText(/^0 of \d+ match$/)).toBeVisible();
  });

  test("the filters are restored from the URL", async ({ page, request }) => {
    const hook = await createHook(request, unique("audit-c"));
    await deleteHook(request, hook.id);

    await openAudit(page, `?q=${hook.id}&kind=webhook.delete`);
    await expect(page.getByRole("searchbox", { name: "Search the log" })).toHaveValue(hook.id);
    await expect(page.locator("#audit-kind")).toHaveValue("webhook.delete");
    await expect(rows(page)).toHaveCount(1);
    await expect(rows(page).first()).toContainText("Integration removed");
    await expect(page.getByRole("button", { name: "Clear filters" })).toBeVisible();

    await page.reload();
    await expect(page.getByRole("searchbox", { name: "Search the log" })).toHaveValue(hook.id);
    await expect(page.locator("#audit-kind")).toHaveValue("webhook.delete");
    await expect(rows(page)).toHaveCount(1);

    // An actor nobody has: the filter is applied and the page says nothing matches.
    await openAudit(page, "?actor=e2e-live-nobody");
    await expect(page.getByText("No entry matches these filters.")).toBeVisible();
    await expect(page.getByRole("button", { name: "Clear filters" })).toBeVisible();
    await page.getByRole("button", { name: "Clear filters" }).click();
    await expect(rows(page).first()).toBeVisible();
    await expect(page).not.toHaveURL(/actor=/);
  });

  test("a row opens the side panel, Escape and the close button close it, another row swaps it", async ({ page, request }) => {
    const hook = await createHook(request, unique("audit-d"));
    await deleteHook(request, hook.id);
    await openAudit(page, `?q=${hook.id}`);
    await expect(rows(page)).toHaveCount(2);
    await expect(panel(page)).toHaveCount(0);

    // Newest first: the deletion, which records no detail.
    await rows(page).nth(0).click();
    await expect(panel(page).getByRole("heading", { level: 2 })).toHaveText("Integration removed");
    for (const term of ["When", "By", "To"]) await expect(panel(page).locator("dt", { hasText: new RegExp(`^${term}$`) })).toBeVisible();
    await expect(panel(page).locator("dd.t-data")).toHaveText(hook.id);
    await expect(panel(page)).toContainText("This entry has no further detail.");

    // Another row swaps the content in the same panel.
    await rows(page).nth(1).click();
    await expect(panel(page).getByRole("heading", { level: 2 })).toHaveText("Integration connected");
    await expect(panel(page)).toContainText("Detail as recorded");
    await expect(panel(page).locator(".cds--snippet")).toContainText(hook.label);
    await expect(panel(page).locator(".cds--snippet")).toContainText('"kind": "generic"');
    await expect(panel(page)).toHaveCount(1);

    await page.keyboard.press("Escape");
    await expect(panel(page)).toHaveCount(0);

    await rows(page).nth(1).click();
    await expect(panel(page)).toBeVisible();
    await panel(page).getByRole("button", { name: "Close", exact: true }).click();
    await expect(panel(page)).toHaveCount(0);

    // The keyboard opens a row too, and the same row again closes it.
    await rows(page).nth(0).focus();
    await page.keyboard.press("Enter");
    await expect(panel(page)).toBeVisible();
    await rows(page).nth(0).click();
    await expect(panel(page)).toHaveCount(0);
  });

  test("Verify the chain asks the proxy and says what it answered", async ({ page }) => {
    await openAudit(page);
    const button = page.getByRole("button", { name: "Verify the chain" });
    await expect(button).toBeEnabled();

    const [res] = await Promise.all([
      page.waitForResponse((r) => r.url().includes("/api/v1/audit/verify") && r.request().method() === "GET"),
      button.click(),
    ]);
    expect(res.ok()).toBeTruthy();
    const answer = (await res.json()) as { chain_ok: boolean; entries: number; broken_at?: number | null };

    const head = page.locator(".t-pagehead");
    if (!answer.chain_ok) {
      await expect(head.getByText(`Chain broken at entry ${answer.broken_at ?? "?"}`)).toBeVisible();
    } else if (answer.entries === 0) {
      await expect(head.getByText("Nothing to verify yet")).toBeVisible();
    } else {
      const n = answer.entries.toLocaleString("en");
      await expect(head.getByText(`Chain intact over ${n} ${answer.entries === 1 ? "entry" : "entries"}`)).toBeVisible();
    }
    await expect(button).toBeEnabled();
  });
});

test.describe("Reports", () => {
  const WORDS = { "24h": "The last 24 hours", "7d": "The last 7 days", "30d": "The last 30 days" } as const;
  type Range = keyof typeof WORDS;

  type Stats = { total_requests?: number; blocked_requests?: number; redacted_requests?: number };
  const en = (v: number | undefined) => (v ?? 0).toLocaleString("en");

  const statsFor = (page: Page, range: Range) =>
    page.waitForResponse((r) => new URL(r.url()).pathname === "/api/v1/stats" && r.url().includes(`range=${range}`) && r.ok());
  const seriesFor = (page: Page, range: Range) =>
    page.waitForResponse((r) => new URL(r.url()).pathname.endsWith("/timeseries") && r.url().includes(`range=${range}`) && r.ok());

  async function openReports(page: Page, query = "") {
    await page.goto(`/dashboard/reports${query}`);
    await expect(page.getByRole("heading", { name: "Reports", level: 1 })).toBeVisible();
  }

  async function expectSentences(page: Page, stats: Stats) {
    await expect(page.locator(".t-prose p").first()).toHaveText(
      `${en(stats.total_requests)} requests went through the proxy. ${en(stats.blocked_requests)} were stopped and ${en(stats.redacted_requests)} went on with something redacted.`,
    );
  }

  test("24 hours, 7 days and 30 days change the sentences and the chart", async ({ page }) => {
    const [first, firstSeries] = await Promise.all([statsFor(page, "7d"), seriesFor(page, "7d"), openReports(page)]);
    await expect(page.locator(".t-lede")).toContainText(WORDS["7d"]);
    await expectSentences(page, (await first.json()) as Stats);
    const seen: Record<string, string> = { "7d": JSON.stringify(((await firstSeries.json()) as { points?: unknown }).points ?? []) };

    for (const [range, tab] of [
      ["24h", "24 hours"],
      ["30d", "30 days"],
    ] as const) {
      const [res, series] = await Promise.all([statsFor(page, range), seriesFor(page, range), page.getByRole("tab", { name: tab }).click()]);
      await expect(page).toHaveURL(new RegExp(`[?&]range=${range}`));
      await expect(page.getByRole("tab", { name: tab })).toHaveAttribute("aria-selected", "true");
      await expect(page.locator(".t-lede")).toContainText(WORDS[range]);
      await expect(page.getByText(`All of ${WORDS[range].toLowerCase()}, as CSV.`)).toBeVisible();
      await expectSentences(page, (await res.json()) as Stats);

      // The chart is drawn from the series for that window: one column per point.
      const body = (await series.json()) as { bucket?: string; points?: { total: number }[] };
      const points = body.points ?? [];
      seen[range] = JSON.stringify(points);
      const unit = body.bucket === "hour" ? "hour" : "day";
      await expect(page.getByRole("heading", { name: `Requests by ${unit}`, level: 2 })).toBeVisible();
      if (points.some((p) => p.total > 0)) {
        await expect(page.getByRole("img", { name: `Requests by ${unit}` }).locator(".t-day")).toHaveCount(points.length);
      }
    }
    expect(new Set(Object.values(seen)).size, "the three windows are not the same series").toBeGreaterThan(1);

    // The window is kept in the URL.
    await page.reload();
    await expect(page.getByRole("tab", { name: "30 days" })).toHaveAttribute("aria-selected", "true");
    await expect(page.locator(".t-lede")).toContainText(WORDS["30d"]);
  });

  test("CSV under Stopped most recently downloads the listed requests", async ({ page }) => {
    await openReports(page, "?range=30d");
    const section = page.locator("section[aria-labelledby=stopped]");
    const links = section.locator("ul.t-queue a");
    await expect(links.first()).toBeVisible();
    const listed = await links.count();

    const [download] = await Promise.all([page.waitForEvent("download"), section.getByRole("button", { name: "CSV" }).click()]);
    expect(download.suggestedFilename()).toBe("tamga-reports-blocked-30d.csv");
    const lines = readFileSync(await download.path(), "utf8").trim().split("\n");
    expect(lines[0]).toBe('"request_id","timestamp","provider","model","action"');
    expect(lines).toHaveLength(listed + 1);
    for (const line of lines.slice(1)) expect(line).toMatch(/^"[^"]+","\d{4}-\d\d-\d\dT[^"]+","[^"]*","[^"]*","BLOCK"$/);

    // The first line is the first request on the page.
    const href = await links.first().getAttribute("href");
    expect(lines[1].split(",")[0]).toBe(`"${decodeURIComponent(href!.split("request_id=")[1])}"`);
  });

  test("Download under What an auditor asks for gives every scanned request as CSV", async ({ page }) => {
    await openReports(page, "?range=30d");
    const section = page.locator("section[aria-labelledby=evidence]");
    const button = section.getByRole("button", { name: "Download" });
    await expect(button).toBeEnabled();

    const [download] = await Promise.all([page.waitForEvent("download"), button.click()]);
    expect(download.suggestedFilename()).toBe("tamga-events-30d.csv");
    const lines = readFileSync(await download.path(), "utf8").trim().split(/\r?\n/);
    const header = lines[0].split(",");
    for (const column of ["request_id", "timestamp", "action"]) expect(header).toContain(column);
    expect(lines.length).toBeGreaterThan(1);
    expect(lines[1].split(",").length).toBeGreaterThanOrEqual(header.length);
    await expect(page.getByText("Could not export events")).toHaveCount(0);
    await expect(button).toBeEnabled();
  });

  // The proxy hands both PDFs to its analyzer. Where the analyzer is not
  // reachable it answers 503, and the console has to say so and stay usable.
  for (const [name, path, file] of [
    ["OWASP report", "/api/v1/reports/owasp/pdf", "tamga-owasp-report-7d.pdf"],
    ["Incident report", "/api/v1/reports/incident/pdf", "tamga-incident-report-7d.pdf"],
  ] as const) {
    test(`${name} downloads a PDF or says why it could not`, async ({ page }) => {
      const [stats] = await Promise.all([statsFor(page, "7d"), openReports(page)]);
      expect(stats.ok()).toBeTruthy();
      const button = page.getByRole("button", { name });
      await expect(button).toBeEnabled();

      const download = page.waitForEvent("download", { timeout: 20_000 }).catch(() => null);
      const [res] = await Promise.all([page.waitForResponse((r) => new URL(r.url()).pathname === path, { timeout: 20_000 }), button.click()]);

      if (res.ok()) {
        const saved = await download;
        expect(saved, "a 200 from the proxy ends in a file").not.toBeNull();
        expect(saved!.suggestedFilename()).toBe(file);
        expect(readFileSync(await saved!.path()).subarray(0, 5).toString("latin1")).toBe("%PDF-");
        await expect(page.getByText(`${name} downloaded`)).toBeVisible();
      } else {
        await expect(page.getByText("Could not generate the PDF", { exact: true })).toBeVisible();
        await expect(page.getByText(`${name} downloaded`)).toHaveCount(0);
      }
      await expect(button).toBeEnabled();
    });
  }

  test.fixme("OWASP report and Incident report download a PDF", () => {
    // The proxy answers 503 "analyzer unavailable": it looks for the analyzer's HTTP API at localhost:8000 inside its own container.
  });

  test("a request under Stopped most recently opens Incidents with that request's panel", async ({ page }) => {
    await openReports(page, "?range=30d");
    const link = page.locator("section[aria-labelledby=stopped] ul.t-queue a").first();
    await expect(link).toBeVisible();
    const id = decodeURIComponent((await link.getAttribute("href"))!.split("request_id=")[1]);

    await link.click();
    await expect(page).toHaveURL(new RegExp(`/dashboard/security\\?request_id=${encodeURIComponent(id)}`));
    await expect(page.locator("aside.t-detail")).toBeVisible();
    await expect(page.locator("aside.t-detail")).toContainText(id);
  });
});
