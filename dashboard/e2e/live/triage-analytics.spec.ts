import { readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test, type Page } from "@playwright/test";
import { liveOnly, PREFIX, unique } from "./live";

/**
 * Incidents, Event explorer, Threat hunting, the analytics pages, Proxy status
 * and Scanner pool, driven against the real proxy. What a test changes on the
 * proxy (an incident's status, a saved hunt) it puts back, also when it fails.
 */

function env(name: string): string {
  if (process.env[name]) return process.env[name] as string;
  try {
    const line = readFileSync(join(process.cwd(), ".env.local"), "utf8")
      .split(/\r?\n/)
      .find((l) => l.startsWith(`${name}=`));
    return line ? line.slice(name.length + 1).trim().replace(/^"|"$/g, "") : "";
  } catch {
    return "";
  }
}

const API = env("NEXT_PUBLIC_API_URL") || "http://localhost:8443";
const HEADERS = { "X-Tamga-Admin-Key": env("NEXT_PUBLIC_ADMIN_KEY") };

/** The words the console uses for a provider, as in components/console/names.ts. */
const PROVIDER_WORDS: Record<string, string> = { openai: "OpenAI", anthropic: "Anthropic", gemini: "Gemini", azure: "Azure OpenAI" };
const providerWord = (p?: string) => (p ? (PROVIDER_WORDS[p] ?? p) : "Unknown");

/** Text that only shows when a value was missing or a date did not parse. */
const BROKEN_TEXT = /\bNaN\b|undefined|Invalid Date|\bnull\b|\[object Object\]/;

interface Ev {
  request_id: string;
  action?: string;
  provider?: string;
  model?: string;
  findings?: { category: string; type: string; severity: string }[] | null;
}

async function apiGet<T>(page: Page, path: string): Promise<T> {
  const res = await page.request.get(`${API}${path}`, { headers: HEADERS });
  expect(res.ok(), `GET ${path} answered ${res.status()}`).toBeTruthy();
  return (await res.json()) as T;
}

const getEvents = (page: Page, query: string) => apiGet<{ events: Ev[] | null; total: number }>(page, `/api/v1/events?${query}`);

const isIncident = (e: Ev) => !!e.action && e.action !== "PASS" && e.action !== "LOG";

/** The incidents the queue lists: the acted-on ones among the 200 most recent requests. */
async function queue(page: Page) {
  const { events } = await getEvents(page, "page=1&limit=200");
  return (events ?? []).filter(isIncident);
}

const panel = (page: Page) => page.getByRole("complementary", { name: "Request detail" });
const rows = (page: Page) => page.locator(".t-work tbody tr");
const headers = (page: Page) => page.locator(".t-work thead th");
const requestId = (page: Page) => new URL(page.url()).searchParams.get("request_id");

// What went wrong in the page while a test ran.
let pageErrors: string[] = [];
let failedResponses: string[] = [];

test.beforeEach(async ({ page }) => {
  liveOnly();
  pageErrors = [];
  failedResponses = [];
  page.on("pageerror", (err) => pageErrors.push(err.message));
  page.on("response", (res) => {
    if (res.status() < 400) return;
    // An incident nobody has triaged has no record: the console reads the 404 as "Open".
    if (res.status() === 404 && res.request().method() === "GET" && /\/api\/v1\/incidents\/[^/?]+$/.test(res.url())) return;
    failedResponses.push(`${res.status()} ${res.request().method()} ${res.url()}`);
  });
});

test.afterEach(() => {
  expect(pageErrors, "uncaught errors in the page").toEqual([]);
  expect(failedResponses, "requests the proxy or the console refused").toEqual([]);
});

test.describe("Incidents", () => {
  test("the switch, the provider list and the search filter the rows", async ({ page }) => {
    const incidents = await queue(page);
    const all = incidents.length;
    const stopped = incidents.filter((e) => e.action === "BLOCK").length;
    const redacted = incidents.filter((e) => e.action === "REDACT").length;
    expect(all, "the proxy has incidents to list").toBeGreaterThan(1);

    await page.goto("/dashboard/security");
    const shown = page.getByText(/\d+ of \d+ shown/);
    await expect(shown).toHaveText(`${all} of ${all} shown`);
    await expect(rows(page)).toHaveCount(all);
    await expect(page.locator(".t-lede")).toContainText(`${stopped} of them were stopped`);

    await page.getByRole("tab", { name: "Stopped" }).click();
    await expect(shown).toHaveText(`${stopped} of ${all} shown`);
    await expect(rows(page)).toHaveCount(stopped);
    await expect(rows(page).filter({ hasNot: page.locator('.t-action[data-action="BLOCK"]') })).toHaveCount(0);

    await page.getByRole("tab", { name: "Redacted" }).click();
    await expect(shown).toHaveText(`${redacted} of ${all} shown`);
    await expect(rows(page)).toHaveCount(redacted);
    await expect(rows(page).filter({ hasNot: page.locator('.t-action[data-action="REDACT"]') })).toHaveCount(0);

    await page.getByRole("tab", { name: "All", exact: true }).click();
    await expect(shown).toHaveText(`${all} of ${all} shown`);

    // The provider list offers the providers the incidents went to.
    const provider = providerWord(incidents[0].provider);
    const toProvider = incidents.filter((e) => providerWord(e.provider) === provider).length;
    await page.locator("#provider").click();
    await page.getByRole("option", { name: provider, exact: true }).click();
    await expect(shown).toHaveText(`${toProvider} of ${all} shown`);
    await expect(rows(page)).toHaveCount(toProvider);
    await page.locator("#provider").click();
    await page.getByRole("option", { name: "All providers", exact: true }).click();
    await expect(shown).toHaveText(`${all} of ${all} shown`);

    const search = page.getByRole("searchbox", { name: "Search incidents" });
    await search.fill(incidents[1].request_id);
    await expect(shown).toHaveText(`1 of ${all} shown`);
    await expect(rows(page)).toHaveCount(1);
    await expect(rows(page).first()).toContainText(incidents[1].request_id.slice(0, 13));

    await search.fill("no incident reads like this");
    await expect(shown).toHaveText(`0 of ${all} shown`);
    await expect(page.getByText("No incident matches these filters.")).toBeVisible();

    await search.fill("");
    await expect(shown).toHaveText(`${all} of ${all} shown`);
  });

  test("a row opens the panel, and the same row, Escape or the close button closes it", async ({ page }) => {
    const incidents = await queue(page);
    await page.goto("/dashboard/security");
    await expect(rows(page).first()).toBeVisible();
    await expect(headers(page)).toHaveCount(6);
    await expect(panel(page)).toHaveCount(0);

    const first = rows(page).nth(0);
    const second = rows(page).nth(1);

    await first.click();
    await expect(panel(page)).toBeVisible();
    await expect(panel(page)).toContainText(incidents[0].request_id);
    await expect(page).toHaveURL(new RegExp(`request_id=${incidents[0].request_id}`));
    await expect(headers(page)).toHaveCount(4);
    await expect(first).toHaveAttribute("aria-expanded", "true");

    // Another row swaps the content, the panel stays.
    await second.click();
    await expect(panel(page)).toContainText(incidents[1].request_id);
    await expect(panel(page)).not.toContainText(incidents[0].request_id);
    expect(requestId(page)).toBe(incidents[1].request_id);
    await expect(first).toHaveAttribute("aria-expanded", "false");
    await expect(second).toHaveAttribute("aria-expanded", "true");

    // The same row closes it.
    await second.click();
    await expect(panel(page)).toHaveCount(0);
    await expect(headers(page)).toHaveCount(6);
    await expect(page).not.toHaveURL(/request_id=/);

    await first.click();
    await expect(panel(page)).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(panel(page)).toHaveCount(0);
    await expect(page).not.toHaveURL(/request_id=/);

    await first.click();
    await expect(panel(page)).toBeVisible();
    await panel(page).getByRole("button", { name: "Close (Esc)" }).click();
    await expect(panel(page)).toHaveCount(0);
    await expect(page).not.toHaveURL(/request_id=/);
  });

  test("Enter and Space on a focused row do what a click does", async ({ page }) => {
    const incidents = await queue(page);
    await page.goto("/dashboard/security");
    const third = rows(page).nth(2);
    await expect(third).toBeVisible();

    await third.focus();
    await page.keyboard.press("Enter");
    await expect(panel(page)).toContainText(incidents[2].request_id);
    expect(requestId(page)).toBe(incidents[2].request_id);
    await third.focus();
    await page.keyboard.press("Enter");
    await expect(panel(page)).toHaveCount(0);

    await third.focus();
    await page.keyboard.press("Space");
    await expect(panel(page)).toContainText(incidents[2].request_id);
    await third.focus();
    await page.keyboard.press("Space");
    await expect(panel(page)).toHaveCount(0);
    await expect(page).not.toHaveURL(/request_id=/);
  });

  test("a link with ?request_id= opens that request, listed or not", async ({ page }) => {
    const incidents = await queue(page);
    const listed = incidents[incidents.length - 1];
    await page.goto(`/dashboard/security?request_id=${listed.request_id}`);
    await expect(panel(page)).toContainText(listed.request_id);
    await expect(headers(page)).toHaveCount(4);
    await expect(page.locator('.t-work tbody tr[aria-expanded="true"]')).toHaveCount(1);

    // A request that passed is not in the queue: it is fetched on its own.
    const { events } = await getEvents(page, "page=1&limit=1&action=PASS&range=30d");
    const passed = (events ?? [])[0];
    expect(passed, "the proxy has a request that passed").toBeTruthy();
    expect(incidents.some((e) => e.request_id === passed.request_id)).toBe(false);

    await page.goto(`/dashboard/security?request_id=${passed.request_id}`);
    await expect(panel(page)).toContainText(passed.request_id);
    await expect(panel(page).getByRole("heading", { name: "Nothing was found" })).toBeVisible();
    await expect(panel(page).locator(".t-action").first()).toHaveText("Passed");
    await expect(panel(page).getByRole("heading", { name: "Triage" })).toHaveCount(0);
    await expect(panel(page)).not.toContainText(BROKEN_TEXT);
    await expect(rows(page).first()).toBeVisible();
    await expect(headers(page)).toHaveCount(4);

    await page.keyboard.press("Escape");
    await expect(panel(page)).toHaveCount(0);
    await expect(headers(page)).toHaveCount(6);
  });

  test.describe("triage", () => {
    const touched = new Set<string>();

    // Whatever a test left, the incident is open again afterwards.
    test.afterEach(async ({ page }) => {
      for (const id of touched) {
        await page.request.patch(`${API}/api/v1/incidents/${id}`, { headers: HEADERS, data: { status: "Open" } });
      }
      touched.clear();
    });

    test("closing, reopening and marking a false positive are saved", async ({ page }) => {
      const incidents = await queue(page);
      // One nobody has decided on, so nothing of anyone's is overwritten.
      let target: Ev | undefined;
      for (const e of [...incidents].reverse()) {
        const res = await page.request.get(`${API}/api/v1/incidents/${e.request_id}`, { headers: HEADERS });
        const status = res.ok() ? ((await res.json()) as { status?: string }).status : "Open";
        if (res.status() === 404 || status === "Open") {
          target = e;
          break;
        }
      }
      expect(target, "an open incident to triage").toBeTruthy();
      const id = (target as Ev).request_id;
      touched.add(id);

      await page.goto(`/dashboard/security?request_id=${id}`);
      const status = panel(page).locator(".t-status");
      const close = panel(page).getByRole("button", { name: "Close as handled" });
      const falsePositive = panel(page).getByRole("button", { name: "False positive" });
      const reopen = panel(page).getByRole("button", { name: "Reopen" });

      await expect(status).toHaveText("Open, nobody has looked at this yet.");
      await expect(close).toBeVisible();
      await expect(falsePositive).toBeVisible();
      await expect(reopen).toHaveCount(0);

      await close.click();
      await expect(status).toHaveText("Closed.");
      await expect(reopen).toBeVisible();
      await expect(close).toHaveCount(0);
      await expect(falsePositive).toHaveCount(0);

      await page.reload();
      await expect(status).toHaveText("Closed.");
      await expect(reopen).toBeVisible();

      await reopen.click();
      await expect(status).toHaveText("Open, nobody has looked at this yet.");
      await expect(close).toBeVisible();
      await expect(reopen).toHaveCount(0);

      await falsePositive.click();
      await expect(status).toHaveText("Marked as a false positive.");
      await expect(reopen).toBeVisible();

      await page.reload();
      await expect(status).toHaveText("Marked as a false positive.");

      await reopen.click();
      await expect(status).toHaveText("Open, nobody has looked at this yet.");
      await page.reload();
      await expect(status).toHaveText("Open, nobody has looked at this yet.");
      await expect(panel(page).getByText("That did not save.")).toHaveCount(0);
    });
  });
});

test.describe("Event explorer", () => {
  const loaded = (page: Page) => page.getByText(/[\d,]+ of [\d,]+ loaded/);
  const box = (page: Page, word: string) => page.getByRole("checkbox", { name: word, exact: true });
  const tick = (page: Page, action: string) => page.locator(`label[for="action-${action}"]`).click();
  /** "100 of 420 loaded" → 420. */
  const total = async (page: Page) => Number((await loaded(page).innerText()).replace(/,/g, "").match(/of (\d+)/)?.[1]);
  const matching = async (page: Page, query: string) => (await getEvents(page, `page=1&limit=1&range=30d${query}`)).total;

  test("the checkboxes and the provider list filter the rows and stay in the URL", async ({ page }) => {
    const all = await matching(page, "");
    const blocked = await matching(page, "&action=block");
    const redacted = await matching(page, "&action=redact");
    const passed = await matching(page, "&action=pass");
    expect(blocked).toBeGreaterThan(0);
    expect(passed).toBeGreaterThan(0);

    await page.goto("/dashboard/events?range=30d");
    await expect(loaded(page)).toContainText(`of ${all.toLocaleString("en")} loaded`);
    await expect(page.getByRole("button", { name: "Clear filters" })).toHaveCount(0);

    await tick(page, "block");
    await expect(page).toHaveURL(/action=block/);
    await expect(loaded(page)).toContainText(`of ${blocked.toLocaleString("en")} loaded`);
    await expect(rows(page).first()).toBeVisible();
    await expect(rows(page).filter({ hasNot: page.locator('.t-action[data-action="BLOCK"]') })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Clear filters" })).toBeVisible();

    await page.reload();
    await expect(box(page, "Stopped")).toBeChecked();
    await expect(box(page, "Redacted")).not.toBeChecked();
    await expect(loaded(page)).toContainText(`of ${blocked.toLocaleString("en")} loaded`);

    await tick(page, "block");
    await tick(page, "redact");
    await expect(page).toHaveURL(/action=redact/);
    await expect(page).not.toHaveURL(/action=block/);
    await expect(loaded(page)).toContainText(`of ${redacted.toLocaleString("en")} loaded`);
    await tick(page, "redact");
    await expect(page).not.toHaveURL(/action=/);
    await expect(page.getByRole("button", { name: "Clear filters" })).toHaveCount(0);
    await expect(loaded(page)).toContainText(`of ${all.toLocaleString("en")} loaded`);

    await tick(page, "warn");
    await expect(page).toHaveURL(/action=warn/);
    await expect(loaded(page)).toContainText(`of ${(await matching(page, "&action=warn")).toLocaleString("en")} loaded`);
    await tick(page, "warn");

    await tick(page, "pass");
    await expect(page).toHaveURL(/action=pass/);
    await expect(loaded(page)).toContainText(`of ${passed.toLocaleString("en")} loaded`);
    await expect(rows(page).filter({ hasNot: page.locator('.t-action[data-action="PASS"]') })).toHaveCount(0);

    // "Sent to" narrows what the boxes left.
    const provider = (await getEvents(page, "page=1&limit=1&range=30d&action=pass")).events?.[0]?.provider ?? "openai";
    const toProvider = await matching(page, `&action=pass&provider=${provider}`);
    await page.locator("#provider").click();
    await page.getByRole("option", { name: providerWord(provider), exact: true }).click();
    await expect(page).toHaveURL(new RegExp(`provider=${provider}`));
    await expect(loaded(page)).toContainText(`of ${toProvider.toLocaleString("en")} loaded`);

    await page.reload();
    await expect(box(page, "Passed")).toBeChecked();
    await expect(page.locator("#provider")).toContainText(providerWord(provider));
    await expect(loaded(page)).toContainText(`of ${toProvider.toLocaleString("en")} loaded`);

    await page.getByRole("button", { name: "Clear filters" }).click();
    await expect(page).not.toHaveURL(/action=|provider=/);
    await expect(page).toHaveURL(/range=30d/);
    await expect(box(page, "Passed")).not.toBeChecked();
    await expect(page.locator("#provider")).toContainText("Any provider");
    await expect(page.getByRole("button", { name: "Clear filters" })).toHaveCount(0);
    await expect(loaded(page)).toContainText(`of ${all.toLocaleString("en")} loaded`);
  });

  test("two ticked boxes show both kinds", async ({ page }) => {
    const blocked = await matching(page, "&action=block");
    const redacted = await matching(page, "&action=redact");
    await page.goto("/dashboard/events?range=30d&action=block&action=redact");
    await expect(box(page, "Stopped")).toBeChecked();
    await expect(box(page, "Redacted")).toBeChecked();
    await expect(loaded(page)).toContainText(`of ${(blocked + redacted).toLocaleString("en")} loaded`);
  });

  test("a provider nothing was sent to leaves an empty list that says so", async ({ page }) => {
    const none = await matching(page, "&provider=bedrock");
    test.skip(none > 0, "the proxy has requests to Bedrock");
    await page.goto("/dashboard/events?range=30d&provider=bedrock");
    await expect(loaded(page)).toHaveText("0 of 0 loaded");
    await expect(page.getByText("No request matches these filters.")).toBeVisible();
    await expect(page.getByRole("button", { name: "Clear filters" })).toBeVisible();
  });

  test("loading the next page appends older rows without flashing them as new", async ({ page }) => {
    await page.goto("/dashboard/events?range=30d");
    await expect(rows(page).first()).toBeVisible();
    const all = await total(page);
    expect(all, "more than one page of requests").toBeGreaterThan(100);
    await expect(loaded(page)).toHaveText(`100 of ${all.toLocaleString("en")} loaded`);
    await expect(rows(page)).toHaveCount(100);
    const firstBefore = await rows(page).first().innerText();

    const next = Math.min(100, all - 100);
    await expect(page.getByText(`${(all - 100).toLocaleString("en")} more`)).toBeVisible();
    await page.getByRole("button", { name: `Load the next ${next}` }).click();
    await expect(rows(page)).toHaveCount(100 + next);
    // Read at once, while a flash would still be running.
    expect(await page.locator('.t-work tbody tr[data-new="true"]').count()).toBe(0);
    await expect(loaded(page)).toHaveText(`${100 + next} of ${all.toLocaleString("en")} loaded`);
    expect(await rows(page).first().innerText()).toBe(firstBefore);

    if (all > 200) {
      const after = Math.min(100, all - 200);
      await page.getByRole("button", { name: `Load the next ${after}` }).click();
      await expect(rows(page)).toHaveCount(200 + after);
      expect(await page.locator('.t-work tbody tr[data-new="true"]').count()).toBe(0);
    }
  });

  test("changing a filter does not flash the rows it brings back", async ({ page }) => {
    await page.goto("/dashboard/events?range=30d&action=block");
    await expect(rows(page).first()).toBeVisible();
    await page.locator('label[for="action-block"]').click();
    await expect(page).not.toHaveURL(/action=/);
    await expect(rows(page).filter({ hasNot: page.locator('.t-action[data-action="BLOCK"]') }).first()).toBeVisible();
    expect(await page.locator('.t-work tbody tr[data-new="true"]').count()).toBe(0);
  });

  test("a request that passed opens with nothing found and nothing to triage", async ({ page }) => {
    await page.goto("/dashboard/events?range=30d&action=pass");
    await expect(rows(page).first()).toBeVisible();
    await rows(page).first().click();
    await expect(panel(page)).toBeVisible();
    await expect(page).toHaveURL(/request_id=/);
    await expect(panel(page)).toContainText(requestId(page) as string);
    await expect(panel(page).getByRole("heading", { name: "Nothing was found" })).toBeVisible();
    await expect(panel(page).getByRole("heading", { name: "Triage" })).toHaveCount(0);
    await expect(panel(page).getByRole("heading", { name: /What was found/ })).toHaveCount(0);
    await expect(panel(page)).not.toContainText(BROKEN_TEXT);
    await expect(headers(page)).toHaveCount(4);
    // The filters stay beside the open request.
    await expect(page).toHaveURL(/action=pass/);

    await page.keyboard.press("Escape");
    await expect(panel(page)).toHaveCount(0);
    await expect(page).toHaveURL(/action=pass/);
  });

  test("a stopped request opens with its findings and a triage section", async ({ page }) => {
    await page.goto("/dashboard/events?range=30d&action=block");
    await expect(rows(page).first()).toBeVisible();
    await rows(page).first().click();
    await expect(panel(page).getByRole("heading", { name: /What was found/ })).toBeVisible();
    await expect(panel(page).getByRole("heading", { name: "Triage" })).toBeVisible();
    await expect(panel(page).locator(".t-status")).toBeVisible();
    await expect(panel(page)).not.toContainText(BROKEN_TEXT);
  });

  test("the live indicator says Live once the stream is open", async ({ page }) => {
    await page.goto("/dashboard/events");
    await expect(page.locator(".t-live")).toHaveText("Live", { timeout: 15_000 });
    await expect(page.locator('.t-live .t-dot[data-state="up"]')).toBeVisible();
  });
});

test.describe("Threat hunting", () => {
  const matches = (page: Page) => page.locator("section[aria-labelledby='matches'] .t-sectionhead .t-note");
  const expectMatches = (page: Page, n: number) =>
    expect(matches(page)).toHaveText(`${n.toLocaleString("en")} ${n === 1 ? "request" : "requests"}`);
  const found = async (page: Page, query: string) => (await getEvents(page, `page=1&limit=1&range=30d${query}`)).total;
  const clear = (page: Page) => page.getByRole("button", { name: /^Clear \d+ filters?$/ });

  async function deleteHunts(page: Page) {
    const res = await page.request.get(`${API}/api/v1/saved-hunts`, { headers: HEADERS });
    if (!res.ok()) return;
    const { items } = (await res.json()) as { items: { id: string; name: string }[] | null };
    for (const h of items ?? []) {
      if (h.name.startsWith(PREFIX)) await page.request.delete(`${API}/api/v1/saved-hunts/${h.id}`, { headers: HEADERS });
    }
  }

  test.afterEach(async ({ page }) => {
    await deleteHunts(page);
  });

  test("each filter narrows the matches and stays in the URL", async ({ page }) => {
    const { events } = await getEvents(page, "page=1&limit=200&range=30d");
    const sample = (events ?? []).find((e) => e.action === "BLOCK" && (e.findings ?? []).length > 0) as Ev;
    expect(sample, "a stopped request with a finding").toBeTruthy();
    const finding = (sample.findings ?? [])[0];
    const all = await found(page, "");

    await page.goto("/dashboard/hunting?range=30d");
    await expectMatches(page, all);
    await expect(clear(page)).toHaveCount(0);

    // What was done.
    await page.getByLabel("What was done").selectOption("BLOCK");
    await expect(page).toHaveURL(/action=BLOCK/);
    await expectMatches(page, await found(page, "&action=BLOCK"));
    await expect(rows(page).filter({ hasNot: page.locator('.t-action[data-action="BLOCK"]') })).toHaveCount(0);
    await expect(clear(page)).toHaveText("Clear 1 filter");

    // Kind of finding.
    await page.getByLabel("Kind of finding").selectOption(finding.type);
    await expect(page).toHaveURL(new RegExp(`finding_type=${finding.type}`));
    await expectMatches(page, await found(page, `&action=BLOCK&finding_type=${finding.type}`));
    await expect(clear(page)).toHaveText("Clear 2 filters");

    // Severity.
    await page.getByLabel("Severity").selectOption(finding.severity);
    await expect(page).toHaveURL(new RegExp(`severity=${finding.severity}`));
    await expectMatches(page, await found(page, `&action=BLOCK&finding_type=${finding.type}&severity=${finding.severity}`));
    await expect(clear(page)).toHaveText("Clear 3 filters");

    // The filters come back with the page.
    await page.reload();
    await expect(page.getByLabel("What was done")).toHaveValue("BLOCK");
    await expect(page.getByLabel("Kind of finding")).toHaveValue(finding.type);
    await expect(page.getByLabel("Severity")).toHaveValue(finding.severity);
    await expect(clear(page)).toHaveText("Clear 3 filters");

    await clear(page).click();
    await expect(page).not.toHaveURL(/action=|finding_type=|severity=/);
    await expect(page).toHaveURL(/range=30d/);
    await expect(page.getByLabel("What was done")).toHaveValue("");
    await expect(clear(page)).toHaveCount(0);
    await expectMatches(page, all);

    // Finding name.
    await page.getByLabel("Finding name contains").fill(finding.category);
    await expect(page).toHaveURL(new RegExp(`category=${finding.category}`));
    const named = await found(page, `&category=${finding.category}`);
    expect(named).toBeLessThan(all);
    await expectMatches(page, named);
    await page.getByLabel("Finding name contains").fill("");
    await expect(page).not.toHaveURL(/category=/);
    await expectMatches(page, all);

    // Technique or OWASP code.
    await page.getByLabel("Technique or OWASP code").fill("LLM01");
    await expect(page).toHaveURL(/technique=LLM01/);
    await expectMatches(page, await found(page, "&technique=LLM01"));
    await page.getByLabel("Technique or OWASP code").fill("");
    await expectMatches(page, all);

    // Search text: a request ID finds that request.
    const search = page.getByRole("searchbox", { name: "Search" });
    await search.fill(sample.request_id);
    await expect(page).toHaveURL(new RegExp(`q=${sample.request_id}`));
    await expectMatches(page, 1);
    await expect(rows(page)).toHaveCount(1);
    await expect(rows(page).first()).toContainText(sample.request_id.slice(0, 13));
    await search.fill("");
    await expectMatches(page, all);

    // Sent to.
    const provider = sample.provider ?? "openai";
    await page.getByLabel("Sent to").fill(provider);
    await expect(page).toHaveURL(new RegExp(`provider=${provider}`));
    await expectMatches(page, await found(page, `&provider=${provider}`));
    await expect(clear(page)).toHaveText("Clear 1 filter");

    // The routing-table box replaces "Sent to".
    await page.locator('label[for="hunt-shadow"]').click();
    await expect(page).toHaveURL(/shadow=1/);
    await expect(page).not.toHaveURL(/provider=/);
    await expect(page.getByLabel("Sent to")).toBeDisabled();
    await expect(page.getByLabel("Sent to")).toHaveValue("");
    await expectMatches(page, await found(page, "&shadow=true"));
    await expect(clear(page)).toHaveText("Clear 1 filter");

    await page.reload();
    await expect(page.getByRole("checkbox", { name: /not in the routing table/ })).toBeChecked();
    await expect(page.getByLabel("Sent to")).toBeDisabled();

    await clear(page).click();
    await expect(page).not.toHaveURL(/shadow=/);
    await expect(page.getByLabel("Sent to")).toBeEnabled();
    await expectMatches(page, all);

    // A search nothing fits says so.
    await search.fill("nothing-reads-like-this-e2e");
    await expectMatches(page, 0);
    await expect(page.getByText("No request in this window fits the search.")).toBeVisible();
  });

  test("the pager walks the matches and goes back to page 1 when a filter changes", async ({ page }) => {
    const all = await found(page, "");
    expect(all, "more than one page of matches").toBeGreaterThan(50);
    const pages = Math.ceil(all / 50);

    await page.goto("/dashboard/hunting?range=30d");
    const pager = page.locator(".t-pager");
    await expect(pager).toContainText(`Page 1 of ${pages}`);
    await expect(rows(page)).toHaveCount(50);
    await expect(pager.getByRole("button", { name: "Previous" })).toBeDisabled();
    const firstRow = (await rows(page).first().locator(".t-data").innerText()).trim();

    await pager.getByRole("button", { name: "Next" }).click();
    await expect(pager).toContainText(`Page 2 of ${pages}`);
    await expect(rows(page).first()).not.toContainText(firstRow);
    await expect(pager.getByRole("button", { name: "Previous" })).toBeEnabled();

    await pager.getByRole("button", { name: "Previous" }).click();
    await expect(pager).toContainText(`Page 1 of ${pages}`);
    await expect(rows(page).first()).toContainText(firstRow);

    await pager.getByRole("button", { name: "Next" }).click();
    await expect(pager).toContainText(`Page 2 of ${pages}`);
    const passed = await found(page, "&action=PASS");
    await page.getByLabel("What was done").selectOption("PASS");
    await expectMatches(page, passed);
    if (passed > 50) await expect(pager).toContainText(`Page 1 of ${Math.ceil(passed / 50)}`);
    else await expect(pager).toHaveCount(0);
    await expect(rows(page).first()).toBeVisible();

    // The last page holds the rest.
    if (passed > 50) {
      const last = Math.ceil(passed / 50);
      for (let p = 1; p < last; p++) await pager.getByRole("button", { name: "Next" }).click();
      await expect(pager).toContainText(`Page ${last} of ${last}`);
      await expect(rows(page)).toHaveCount(passed - (last - 1) * 50);
      await expect(pager.getByRole("button", { name: "Next" })).toBeDisabled();
    }
  });

  test("a hunt can be saved, run again and deleted", async ({ page }) => {
    const name = unique("hunt");
    const saved = page.locator("section[aria-labelledby='saved']");
    const nameField = page.getByLabel("Name for the current search");

    await page.goto("/dashboard/hunting?range=30d&action=BLOCK&severity=critical");
    await expect(page.getByLabel("What was done")).toHaveValue("BLOCK");

    // The name field opens in place and Escape puts it away.
    await saved.getByRole("button", { name: "Save this one" }).click();
    await expect(nameField).toBeVisible();
    await expect(nameField).toBeFocused();
    await expect(saved.getByRole("button", { name: "Save this one" })).toHaveCount(0);
    await expect(saved.getByRole("button", { name: "Save", exact: true })).toBeDisabled();
    await page.keyboard.press("Escape");
    await expect(nameField).toHaveCount(0);
    await expect(saved.getByRole("button", { name: "Save this one" })).toBeVisible();

    await saved.getByRole("button", { name: "Save this one" }).click();
    await nameField.fill(name);
    await saved.getByRole("button", { name: "Save", exact: true }).click();
    await expect(nameField).toHaveCount(0);
    const item = saved.locator("li", { hasText: name });
    await expect(item).toBeVisible();
    await expect(item).not.toContainText(BROKEN_TEXT);

    // It is on the proxy, not only in this browser.
    const { items } = await apiGet<{ items: { id: string; name: string }[] }>(page, "/api/v1/saved-hunts");
    expect(items.some((h) => h.name === name)).toBe(true);

    // Running it again restores its filters over whatever is set.
    await page.getByRole("button", { name: /^Clear \d+ filters?$/ }).click();
    await expect(page).not.toHaveURL(/action=|severity=/);
    await page.getByLabel("Finding name contains").fill("iban");
    await expect(page).toHaveURL(/category=iban/);
    await item.getByRole("button", { name: new RegExp(name) }).first().click();
    await expect(page).toHaveURL(/action=BLOCK/);
    await expect(page).toHaveURL(/severity=critical/);
    await expect(page).not.toHaveURL(/category=/);
    await expect(page.getByLabel("What was done")).toHaveValue("BLOCK");
    await expect(page.getByLabel("Severity")).toHaveValue("critical");
    await expect(page.getByLabel("Finding name contains")).toHaveValue("");
    await expectMatches(page, await found(page, "&action=BLOCK&severity=critical"));

    // It is still there after a reload.
    await page.reload();
    await expect(item).toBeVisible();

    // Delete asks in place; "Keep" backs out.
    await item.getByRole("button", { name: `Delete ${name}` }).click();
    await expect(item).toContainText(`Delete ${name}?`);
    await item.getByRole("button", { name: "Keep" }).click();
    await expect(item.getByRole("button", { name: `Delete ${name}` })).toBeVisible();

    await item.getByRole("button", { name: `Delete ${name}` }).click();
    await item.getByRole("button", { name: "Delete", exact: true }).click();
    await expect(item).toHaveCount(0);
    const after = await apiGet<{ items: { id: string; name: string }[] | null }>(page, "/api/v1/saved-hunts");
    expect((after.items ?? []).some((h) => h.name === name)).toBe(false);
  });
});

test.describe("Analytics", () => {
  const RANGES: [switchText: string, param: string, words: string][] = [
    ["24 hours", "24h", "the last 24 hours"],
    ["7 days", "7d", "the last 7 days"],
    ["30 days", "30d", "the last 30 days"],
  ];
  const main = (page: Page) => page.locator("main");
  const stats = (page: Page, range: string) => apiGet<{ total_requests: number; blocked_requests: number }>(page, `/api/v1/stats?range=${range}`);
  const series = (page: Page, range: string) =>
    apiGet<{ bucket: string; points: { t: string; total: number; blocked: number; redacted?: number; warned?: number; scan_p95?: number }[] | null }>(
      page,
      `/api/v1/timeseries?range=${range}`,
    );

  /** Every range in turn: the URL follows, and `check` reads the page for that range. */
  async function walkRanges(page: Page, path: string, check: (param: string, words: string) => Promise<void>) {
    await page.goto(path);
    for (const [text, param, words] of RANGES) {
      await page.getByRole("tab", { name: text }).click();
      await expect(page.getByRole("tab", { name: text })).toHaveAttribute("aria-selected", "true");
      await check(param, words);
      await expect(main(page)).not.toContainText(BROKEN_TEXT);
    }
    // The choice comes back with the page.
    await page.goto(`${path}?range=24h`);
    await expect(page.getByRole("tab", { name: "24 hours" })).toHaveAttribute("aria-selected", "true");
    await page.goto(`${path}?range=30d`);
    await expect(page.getByRole("tab", { name: "30 days" })).toHaveAttribute("aria-selected", "true");
  }

  test("Overview follows the range", async ({ page }) => {
    await walkRanges(page, "/dashboard", async (param, words) => {
      const s = await stats(page, param);
      await expect(page.locator(".t-lede")).toContainText(words);
      const line = page.locator(".t-statement .t-figureline");
      if (s.total_requests === 0) {
        await expect(line).toContainText(`requests went through the proxy in ${words}.`);
      } else {
        await expect(line.locator(".t-figure")).toHaveText(s.blocked_requests.toLocaleString("en"));
        await expect(line).toContainText(
          `of ${s.total_requests.toLocaleString("en")} requests ${s.blocked_requests === 1 ? "was" : "were"} stopped`,
        );
      }
      const ts = await series(page, param);
      await expect(page.locator("#days")).toHaveText(`Requests by ${ts.bucket === "hour" ? "hour" : "day"}`);
    });
    await expect(page.locator(".t-facts")).toContainText("Up");
    await expect(page.getByRole("link", { name: "Open the queue" })).toHaveAttribute("href", "/dashboard/security");
  });

  test("Overview keeps the default range out of the URL and other ranges in it", async ({ page }) => {
    await page.goto("/dashboard");
    await page.getByRole("tab", { name: "24 hours" }).click();
    await expect(page).toHaveURL(/range=24h/);
    await page.getByRole("tab", { name: "30 days" }).click();
    await expect(page).toHaveURL(/range=30d/);
    await page.getByRole("tab", { name: "7 days" }).click();
    await expect(page).not.toHaveURL(/range=/);
  });

  test("Traffic follows the range, reads a column out and exports a CSV", async ({ page }) => {
    await walkRanges(page, "/dashboard/traffic", async (param, words) => {
      const s = await stats(page, param);
      await expect(page).toHaveURL(param === "7d" ? /traffic$/ : new RegExp(`range=${param}`));
      await expect(page.locator(".t-lede")).toContainText(words);
      const line = page.locator(".t-statement .t-figureline");
      await expect(line.locator(".t-figure")).toHaveText(s.total_requests.toLocaleString("en"));
      await expect(line).toContainText(`${s.total_requests === 1 ? "request" : "requests"} went through the proxy in ${words}.`);
      const ts = await series(page, param);
      await expect(page.locator("#volume")).toHaveText(`Requests by ${ts.bucket === "hour" ? "hour" : "day"}`);
    });

    // The readout above the chart.
    const ts = await series(page, "30d");
    const points = ts.points ?? [];
    const busiest = points.reduce((top, p, i) => (p.total > points[top].total ? i : top), 0);
    const readout = page.locator("section[aria-labelledby='volume'] .t-readout");
    const column = page.locator("section[aria-labelledby='volume'] .t-days .t-day");
    await expect(column).toHaveCount(points.length);
    await expect(readout).toHaveText("Point at a column to read that day.");
    await column.nth(busiest).hover();
    await expect(readout).toContainText(`: ${points[busiest].total.toLocaleString("en")} request`);
    await expect(readout).not.toContainText(BROKEN_TEXT);
    await page.locator("h1").hover();
    await expect(readout).toHaveText("Point at a column to read that day.");

    // The file has a row per column, and every row has as many fields as the header.
    const [download] = await Promise.all([page.waitForEvent("download"), page.getByRole("button", { name: "Export CSV" }).click()]);
    expect(download.suggestedFilename()).toBe("tamga-traffic-30d.csv");
    const lines = readFileSync(await download.path(), "utf8").trim().split("\n");
    expect(lines[0]).toBe("time,total,blocked,passed");
    expect(lines.length).toBe(points.length + 1);
    for (const l of lines.slice(1)) expect(l.split(","), l).toHaveLength(4);
    const totals = lines.slice(1).reduce((sum, l) => sum + Number(l.split(",")[1]), 0);
    expect(totals).toBe(points.reduce((sum, p) => sum + p.total, 0));
  });

  test("Detection trends follows the range and reads the line out", async ({ page }) => {
    await walkRanges(page, "/dashboard/trends", async (param, words) => {
      const ts = await series(page, param);
      const points = ts.points ?? [];
      const attempted = points.reduce((sum, p) => sum + p.total, 0);
      await expect(page).toHaveURL(param === "30d" ? /trends$/ : new RegExp(`range=${param}`));
      await expect(page.locator(".t-lede")).toContainText(words);
      await expect(page.locator(".t-facts")).toContainText(attempted.toLocaleString("en"));
      await expect(page.locator("#share")).toHaveText(`Share acted on, by ${param === "24h" ? "hour" : "day"}`);
      await expect(page.locator(".t-statement .t-figure")).toHaveText(/^\d+(\.\d)?%$/);
    });

    const readout = page.locator("section[aria-labelledby='share'] .t-readout");
    const trace = page.locator("section[aria-labelledby='share'] .t-trace");
    await expect(readout).toHaveText("Point at the line to read one day.");
    await trace.hover({ position: { x: 4, y: 40 } });
    await expect(readout).toHaveText(/^\w{3} \d{1,2} \w{3,4}: (nothing measured|\d+(\.\d)?%)$/);
    const box = await trace.boundingBox();
    await trace.hover({ position: { x: (box?.width ?? 200) - 4, y: 40 } });
    await expect(readout).toHaveText(/^\w{3} \d{1,2} \w{3,4}: (nothing measured|\d+(\.\d)?%)$/);
    await page.locator("h1").hover();
    await expect(readout).toHaveText("Point at the line to read one day.");
  });

  test("Token costs follows the range and offers no export without cost data", async ({ page }) => {
    const costs = await apiGet<{ breakdown?: unknown[] | null }>(page, "/api/v1/billing/costs/breakdown?range=7d");
    await walkRanges(page, "/dashboard/costs", async (param, words) => {
      await expect(page).toHaveURL(param === "7d" ? /costs$/ : new RegExp(`range=${param}`));
      await expect(page.locator(".t-lede")).toContainText(words);
      await expect(page.locator(".t-statement .t-figureline")).toContainText(`is the estimated spend of ${words}.`);
      await expect(page.locator(".t-statement .t-figure")).toHaveText(/^\$[\d,.]+$/);
    });
    test.skip((costs.breakdown ?? []).length > 0, "the proxy has cost data, so the export is offered");
    await expect(page.getByRole("button", { name: "Export CSV" })).toBeDisabled();
    await expect(page.getByText("No usage in this window.")).toBeVisible();
  });

  test("Latency follows the range and reads the line out", async ({ page }) => {
    await walkRanges(page, "/dashboard/latency", async (param, words) => {
      const ts = await series(page, param);
      await expect(page).toHaveURL(param === "24h" ? /latency$/ : new RegExp(`range=${param}`));
      const unit = ts.bucket === "hour" ? "hour" : "day";
      await expect(page.locator("#over-time")).toHaveText(`95th percentile by ${unit}`);
      await expect(page.locator("section[aria-labelledby='over-time'] .t-sectionhead .t-note")).toHaveText(`In ${words}`);
      await expect(page.locator("#slowest")).toHaveText(`Slowest ${unit}s`);
    });

    const readout = page.locator("section[aria-labelledby='over-time'] .t-readout");
    const trace = page.locator("section[aria-labelledby='over-time'] .t-trace");
    const scanned = ((await series(page, "30d")).points ?? []).some((p) => (p.scan_p95 ?? 0) > 0);
    test.skip(!scanned, "nothing was scanned in this window, so there is no line");
    await expect(trace).toBeVisible();
    await expect(readout).toHaveText("Point at the line to read one day.");
    await trace.hover({ position: { x: 4, y: 40 } });
    await expect(readout).toHaveText(/^\w{3} \d{1,2} \w{3,4}: (nothing measured|[\d.]+(µs|ms|s))$/);
    await page.locator("h1").hover();
    await expect(readout).toHaveText("Point at the line to read one day.");
  });
});

test.describe("Status pages", () => {
  test("Proxy status shows what the proxy reports", async ({ page }) => {
    const health = await apiGet<{ scanner_count: number; database: string }>(page, "/api/v1/health/detailed");
    const detail = await apiGet<{ version?: string }>(page, "/api/v1/health/detail");

    await page.goto("/dashboard/proxy");
    await expect(page.locator(".t-statement .t-figure")).toHaveText("Up");
    await expect(page.locator(".t-statement .t-figureline")).toContainText(/and all \d+ parts in use are working\./);
    await expect(page.locator(".t-rest")).toContainText("It has been running for");
    const facts = page.locator(".t-facts");
    if (detail.version) await expect(facts).toContainText(detail.version);
    await expect(facts.locator("div", { hasText: "Scanners on every request" }).locator("dd")).toHaveText(String(health.scanner_count));
    if (health.database === "connected") await expect(facts.locator("div", { hasText: "Database" }).locator("dd")).toHaveText("Connected");

    const parts = page.getByRole("table", { name: "Parts of the proxy" }).locator("tbody tr");
    await expect(parts).toHaveCount(8);
    await expect(parts.filter({ hasText: "HTTP server" })).toContainText("Working");
    await expect(parts.filter({ hasText: "Scanners" })).toContainText(`${health.scanner_count} scanners ready`);
    await expect(page.locator("main")).not.toContainText(BROKEN_TEXT);
  });

  test("Scanner pool says plainly that the pool is switched off", async ({ page }) => {
    const health = await apiGet<{ scanner_count: number }>(page, "/api/v1/health/detailed");
    await page.goto("/dashboard/scanner-pool");
    await expect(page.getByRole("heading", { name: "Scanner pool" })).toBeVisible();
    const note = page.locator("main .t-empty");
    await expect(note).toContainText("The worker pool is not switched on.");
    await expect(note).toContainText(`${health.scanner_count} scanners`);
    await expect(note).toContainText("TAMGA_SCANNER_WORKER_POOL_SIZE");
    // One pair of brackets at most: no "(adaptive (default))".
    await expect(note).not.toContainText("))");
    await expect(page.locator("main")).not.toContainText(BROKEN_TEXT);
  });
});
