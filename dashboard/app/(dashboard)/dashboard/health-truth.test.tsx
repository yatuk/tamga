import { beforeEach, describe, expect, it } from "vitest";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createElement, type ComponentType } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { server } from "@/__mocks__/server";
import { ADMIN_KEY_STORAGE } from "@/hooks/useAdminKey";
import type { HealthReading } from "@/hooks/useHealth";
import LatencyPage from "./latency/page";
import { Overview } from "./overview";
import ProxyPage from "./proxy/page";
import { ScannerPoolBody } from "./scanner-pool/ScannerPoolBody";
import SettingsPage from "./settings/page";

/**
 * Every page that shows the proxy's health, in each state the health check
 * can be in. What the pages must never do: show an earlier answer as the
 * state of the proxy now, or say "Needs an admin key" of a proxy that did
 * not answer.
 *
 * See __mocks__/handlers.ts: "test-key" is a key the proxy accepts, one
 * starting with "wrong" is one it rejects.
 */

const NEEDS_KEY = "Needs an admin key";
const AS_OF = /^As of \d\d:\d\d, the last answer$/;

let qc: QueryClient;

function show(page: ComponentType, key = "") {
  if (key) localStorage.setItem(ADMIN_KEY_STORAGE, key);
  qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(createElement(QueryClientProvider, { client: qc }, createElement(page)));
}

/** From here on neither health call is answered. */
function down() {
  server.use(
    http.get("*/api/v1/health/detailed", () => HttpResponse.error()),
    http.get("*/api/v1/health/detail", () => HttpResponse.error()),
  );
}

/** The proxy stops answering, and the next check finds that out. */
async function stopAnswering() {
  down();
  await act(async () => {
    await qc.refetchQueries({ queryKey: ["tamga-health"] });
    await qc.refetchQueries({ queryKey: ["tamga-health-detail"] });
  });
}

const facts = (container: HTMLElement) =>
  Object.fromEntries([...container.querySelectorAll(".t-facts > div")].map((row) => [row.querySelector("dt")!.textContent, row.querySelector("dd")!.textContent]));

beforeEach(() => {
  localStorage.clear();
});

describe("Proxy status", () => {
  it("never answered: it is down, and nothing is known, a key or not", async () => {
    down();
    const { container } = show(ProxyPage);
    await waitFor(() => expect(container.querySelector(".t-figure")).toHaveTextContent("Down"));
    expect(screen.queryByText(NEEDS_KEY)).toBeNull();
    expect(screen.queryByText("No admin key in this browser.")).toBeNull();
    expect(Object.values(facts(container))).toEqual(["Not known", "Not known", "Not known", "Not known", "Not known"]);
    expect(screen.queryByRole("table")).toBeNull();
    expect(screen.getByText("The proxy did not answer, so the state of its parts is not known.")).toBeInTheDocument();
    expect(screen.queryByText(AS_OF)).toBeNull();
  });

  it("answered, then failing: the last answer is shown as the last answer, with its time", async () => {
    const { container } = show(ProxyPage, "test-key");
    await waitFor(() => expect(screen.getByRole("table")).toBeInTheDocument());
    expect(screen.getByText(/It has been running for/)).toBeInTheDocument();
    expect(screen.queryByText(AS_OF)).toBeNull();
    expect(container.querySelector("[data-stale]")).toBeNull();

    await stopAnswering();
    await waitFor(() => expect(container.querySelector(".t-figure")).toHaveTextContent("Down"));
    // How long it has been running is not known of a proxy that does not answer.
    expect(screen.queryByText(/It has been running for/)).toBeNull();
    // The facts stay, marked: once for the eye, and at each value for a reader that hears them.
    expect(screen.getByText(AS_OF)).toHaveClass("t-asof");
    const database = [...container.querySelectorAll(".t-facts dd")].find((dd) => dd.textContent?.startsWith("Connected"))!;
    expect(database.querySelector(".t-asof")).not.toBeNull();
    expect(database).toHaveTextContent(/^Connected, as of \d\d:\d\d, the last answer$/);
    expect(screen.queryByText(NEEDS_KEY)).toBeNull();

    // Whether the HTTP server answers is known now. Every other row is the last answer.
    expect(screen.getByText(/^All but the HTTP server as of \d\d:\d\d, the last answer$/)).toBeInTheDocument();
    const http_ = screen.getByRole("row", { name: /HTTP server/ });
    expect(http_).not.toHaveAttribute("data-stale");
    expect(within(http_).getByText("Failing")).toHaveAttribute("data-action", "FAILING");
    const rows = screen.getAllByRole("row").slice(1);
    expect(rows.filter((r) => r.getAttribute("data-stale") === "true")).toHaveLength(rows.length - 1);
    // A mark says what a part is doing now: an old row has none.
    for (const row of rows) if (row !== http_) expect(row.querySelector(".t-action")).toBeNull();
  });

  it("answered without a key, then failing: no longer says a key is what is missing", async () => {
    const { container } = show(ProxyPage);
    await waitFor(() => expect(screen.getAllByText(NEEDS_KEY)).toHaveLength(5));
    expect(screen.getByText("No admin key in this browser.")).toBeInTheDocument();

    await stopAnswering();
    await waitFor(() => expect(container.querySelector(".t-figure")).toHaveTextContent("Down"));
    expect(screen.queryByText(NEEDS_KEY)).toBeNull();
    // "The proxy is up, but ..." would be false now.
    expect(screen.queryByText("No admin key in this browser.")).toBeNull();
    expect(Object.values(facts(container))).toEqual(["Not known", "Not known", "Not known", "Not known", "Not known"]);
    expect(screen.getByText("The proxy did not answer, so the state of its parts is not known.")).toBeInTheDocument();
  });

  it("answering without a key, or with one it refuses: only then is a key what is missing", async () => {
    const first = show(ProxyPage);
    await waitFor(() => expect(screen.getAllByText(NEEDS_KEY)).toHaveLength(5));
    expect(first.container.querySelector(".t-figure")).toHaveTextContent("Up");
    first.unmount();

    const second = show(ProxyPage, "wrong-key");
    await waitFor(() => expect(screen.getAllByText(NEEDS_KEY)).toHaveLength(5));
    expect(screen.getByText("The proxy did not accept the admin key.")).toBeInTheDocument();
    expect(second.container.querySelector(".t-figure")).toHaveTextContent("Up");
  });

  it("answering with the detail: every value is the proxy's, and none is marked old", async () => {
    const { container } = show(ProxyPage, "test-key");
    await waitFor(() => expect(screen.getByRole("table")).toBeInTheDocument());
    expect(screen.queryByText(NEEDS_KEY)).toBeNull();
    expect(container.querySelector(".t-asof")).toBeNull();
    expect(container.querySelector("[data-stale]")).toBeNull();
    expect(facts(container)["Database"]).toBe("Connected");
  });
});

describe("Latency", () => {
  it("never answered: the scan time is not known, and no pool is said to be undefined", async () => {
    down();
    const { container } = show(LatencyPage, "test-key");
    await waitFor(() => expect(container.querySelector(".t-figure")).toHaveTextContent("Not known"));
    expect(screen.getByText("The proxy is not answering, so how long it holds a request is not known.")).toBeInTheDocument();
    expect(screen.queryByText("No request has been scanned since the proxy started.")).toBeNull();
    expect(screen.queryByText(/No provider pool is defined/)).toBeNull();
    expect(screen.getByText("The proxy did not answer, so the state of its upstream endpoints is not known.")).toBeInTheDocument();
    expect(screen.queryByText(NEEDS_KEY)).toBeNull();
    const f = facts(container);
    expect(f["Scanners on every request"]).toBe("Not known");
    expect(f["Upstream endpoints"]).toBe("Not known");
    // The target is the console's own, and is always known.
    expect(f["Target, 95th percentile"]).toBe("2 s");
  });

  it("answered, then failing: the figure goes, and the last one is said as the last one", async () => {
    const { container } = show(LatencyPage, "test-key");
    await waitFor(() => expect(screen.getByRole("table", { name: "Upstream endpoints" })).toBeInTheDocument());
    expect(container.querySelector(".t-figure")).toHaveTextContent("22.1 ms");

    await stopAnswering();
    await waitFor(() => expect(container.querySelector(".t-figure")).toHaveTextContent("Not known"));
    expect(screen.getByText(/^As of \d\d:\d\d, the last answer: scanning added 22\.1 ms or less/)).toHaveClass("t-asof");
    // Under the figure and over the upstream table.
    expect(screen.getAllByText(AS_OF).length).toBeGreaterThanOrEqual(2);
    const rows = within(screen.getByRole("table", { name: "Upstream endpoints" })).getAllByRole("row").slice(1);
    expect(rows.length).toBeGreaterThan(0);
    for (const row of rows) {
      expect(row).toHaveAttribute("data-stale", "true");
      expect(row.querySelector(".t-action")).toBeNull();
    }
    expect(facts(container)["Scanners on every request"]).toMatch(/^6, as of \d\d:\d\d, the last answer$/);
    expect(facts(container)["Target, 95th percentile"]).toBe("2 s");
  });

  it("answering with a key it refuses: the scan time is public, the rest needs a key", async () => {
    const { container } = show(LatencyPage, "wrong-key");
    await waitFor(() => expect(container.querySelector(".t-figure")).toHaveTextContent("22.1 ms"));
    await waitFor(() => expect(screen.getAllByText(NEEDS_KEY)).toHaveLength(3));
    expect(screen.getByText(/shown once the proxy accepts an admin key/)).toBeInTheDocument();
    expect(container.querySelector(".t-asof")).toBeNull();
  });

  it("answering with the detail: nothing is withheld and nothing is marked old", async () => {
    const { container } = show(LatencyPage, "test-key");
    await waitFor(() => expect(screen.getByRole("table", { name: "Upstream endpoints" })).toBeInTheDocument());
    expect(screen.queryByText(NEEDS_KEY)).toBeNull();
    expect(container.querySelector(".t-asof")).toBeNull();
    expect(container.querySelector("[data-stale]")).toBeNull();
    expect(facts(container)["Scanners on every request"]).toBe("6");
  });
});

describe("Overview", () => {
  const ready = (container: HTMLElement) => waitFor(() => expect(container.querySelector(".t-facts")).not.toBeNull());

  it("never answered: unreachable, and the rest is not known", async () => {
    down();
    const { container } = show(Overview, "test-key");
    await ready(container);
    await waitFor(() => expect(facts(container)["Proxy"]).toBe("Unreachable"));
    const f = facts(container);
    expect(f["Uptime"]).toBe("Not known");
    expect(f["Scan time, median"]).toBe("Not known");
    expect(f["Scanners running"]).toBe("Not known");
    expect(f["Database"]).toBe("Not known");
    expect(screen.queryByText(NEEDS_KEY)).toBeNull();
  });

  it("answered, then failing: unreachable now, the rest as of the last answer", async () => {
    const { container } = show(Overview, "test-key");
    await ready(container);
    await waitFor(() => expect(facts(container)["Database"]).toBe("Connected"));

    await stopAnswering();
    await waitFor(() => expect(facts(container)["Proxy"]).toBe("Unreachable"));
    expect(screen.getByText(AS_OF)).toBeInTheDocument();
    const f = facts(container);
    expect(f["Database"]).toMatch(/^Connected, as of \d\d:\d\d, the last answer$/);
    expect(f["Scan time, median"]).toMatch(/^8\.4 ms, as of/);
    expect(f["Scanners running"]).toMatch(/^6, as of/);
    // What is known now is not marked old.
    expect([...container.querySelectorAll(".t-facts dd")][0].querySelector(".t-asof")).toBeNull();
  });

  it("answering with a key it refuses: the public figures, and a key for the rest", async () => {
    const { container } = show(Overview, "wrong-key");
    await ready(container);
    await waitFor(() => expect(facts(container)["Uptime"]).toBe(NEEDS_KEY));
    const f = facts(container);
    expect(f["Proxy"]).toBe("Up");
    expect(f["Scan time, median"]).toBe("8.4 ms");
    expect(f["Scanners running"]).toBe(NEEDS_KEY);
    expect(f["Database"]).toBe(NEEDS_KEY);
  });

  it("answering with the detail", async () => {
    const { container } = show(Overview, "test-key");
    await ready(container);
    await waitFor(() => expect(facts(container)["Database"]).toBe("Connected"));
    expect(facts(container)["Proxy"]).toBe("Up");
    expect(container.querySelector(".t-asof")).toBeNull();
    expect(screen.queryByText(NEEDS_KEY)).toBeNull();
  });
});

describe("Settings", () => {
  async function open(tab: "Runtime" | "Providers", key = "") {
    const view = show(SettingsPage, key);
    await userEvent.setup().click(await screen.findByRole("tab", { name: tab }));
    return view;
  }

  it("Runtime, never answered: unreachable, nothing known, no key asked for", async () => {
    down();
    const { container } = await open("Runtime", "test-key");
    await waitFor(() => expect(facts(container)["Proxy"]).toBe("Unreachable"));
    await waitFor(() => expect(facts(container)["TLS"]).toBe("Not known"));
    expect(facts(container)["Version"]).toBe("Not known");
    expect(facts(container)["Running for"]).toBe("Not known");
    expect(screen.queryByText(NEEDS_KEY)).toBeNull();
    expect(screen.queryByText(AS_OF)).toBeNull();
  });

  it("Runtime, answered then failing: unreachable now, the rest as of the last answer", async () => {
    const { container } = await open("Runtime", "test-key");
    await waitFor(() => expect(facts(container)["Version"]).toBe("v1.1.0"));
    expect(facts(container)["Proxy"]).toBe("Up");

    await stopAnswering();
    await waitFor(() => expect(facts(container)["Proxy"]).toBe("Unreachable"));
    expect(screen.getByText(AS_OF)).toBeInTheDocument();
    expect(facts(container)["Version"]).toMatch(/^v1\.1\.0, as of \d\d:\d\d, the last answer$/);
    expect(facts(container)["TLS"]).toMatch(/^On, as of/);
  });

  it("Runtime, answering without a key or with one it refuses: a key is what is missing", async () => {
    const first = await open("Runtime");
    await waitFor(() => expect(facts(first.container)["Version"]).toBe(NEEDS_KEY));
    expect(facts(first.container)["Proxy"]).toBe("Up");
    expect(screen.getByText("No admin key in this browser.")).toBeInTheDocument();
    first.unmount();

    const second = await open("Runtime", "wrong-key");
    await waitFor(() => expect(facts(second.container)["Version"]).toBe(NEEDS_KEY));
    expect(screen.getByText("The proxy did not accept the admin key.")).toBeInTheDocument();
  });

  it("Runtime, answering with the detail", async () => {
    const { container } = await open("Runtime", "test-key");
    await waitFor(() => expect(facts(container)["Version"]).toBe("v1.1.0"));
    expect(container.querySelector(".t-asof")).toBeNull();
    expect(screen.queryByText(NEEDS_KEY)).toBeNull();
  });

  it("Providers, never answered: says the proxy did not answer, not that a key is missing", async () => {
    down();
    await open("Providers", "test-key");
    expect(await screen.findByText("The proxy did not answer, so the state of its provider pools is not known.")).toBeInTheDocument();
    expect(screen.queryByText(/shown once the proxy accepts an admin key/)).toBeNull();
  });

  it("Providers, answered then failing: the pools are the last answer, and say so", async () => {
    const { container } = await open("Providers", "test-key");
    await waitFor(() => expect(screen.getByRole("table", { name: "default pool" })).toBeInTheDocument());
    expect(screen.getByText("2 of 2 taking traffic")).toBeInTheDocument();

    await stopAnswering();
    await waitFor(() => expect(screen.getAllByText(AS_OF).length).toBeGreaterThan(0));
    expect(screen.queryByText("2 of 2 taking traffic")).toBeNull();
    const rows = [...container.querySelectorAll("tbody tr")];
    expect(rows.length).toBeGreaterThan(0);
    for (const row of rows) expect(row).toHaveAttribute("data-stale", "true");
  });

  it("Providers, answering without a key: the pools need one", async () => {
    await open("Providers");
    expect(await screen.findByText(/provider pools are shown once the proxy accepts an admin key/)).toBeInTheDocument();
    expect(screen.getByText("No admin key in this browser.")).toBeInTheDocument();
  });
});

describe("Scanner pool", () => {
  const pool = {
    workersIdle: 2,
    workersActive: 2,
    queueDepth: 1,
    queueSize: 16,
    utilization: 0.5,
    jobsSubmitted: 100,
    jobsCompleted: 99,
    jobsFailed: 0,
    jobsShed: 0,
    perScannerDurationMs: {},
  };
  const reading = (standing: HealthReading["standing"], missing = "Not known"): HealthReading => ({
    standing,
    stale: standing === "stale",
    answeredAt: standing === "stale" || standing === "full" ? Date.UTC(2026, 9, 10, 12, 0) : 0,
    missing,
  });
  const body = (scannerCount: number | null, r: HealthReading) => {
    localStorage.setItem(ADMIN_KEY_STORAGE, "test-key");
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    return render(
      createElement(
        QueryClientProvider,
        { client: qc },
        createElement(ScannerPoolBody, { poolEnabled: true, pool, scannerCount, reading: r, pipelineMode: "workerpool", loading: false, error: null }),
      ),
    );
  };

  it("shows the scanner count the proxy gave", () => {
    const { container } = body(6, reading("full"));
    expect(facts(container)["Scanners"]).toBe("6");
    expect(container.querySelector(".t-asof")).toBeNull();
  });

  it("marks a count held from before the health check began to fail", () => {
    const { container } = body(6, reading("stale"));
    expect(facts(container)["Scanners"]).toMatch(/^6, as of \d\d:\d\d, the last answer$/);
    expect(screen.getByText(AS_OF)).toBeInTheDocument();
  });

  it("never draws a count the proxy did not give as 0", () => {
    const withheld = body(null, reading("rejected", NEEDS_KEY));
    expect(facts(withheld.container)["Scanners"]).toBe(NEEDS_KEY);
    withheld.unmount();
    const unknown = body(null, reading("unreachable"));
    expect(facts(unknown.container)["Scanners"]).toBe("Not known");
  });
});
