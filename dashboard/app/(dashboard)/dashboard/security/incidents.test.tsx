import { beforeEach, describe, expect, it } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createElement } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { server } from "@/__mocks__/server";
import { ADMIN_KEY_STORAGE } from "@/hooks/useAdminKey";
import { Incidents } from "./incidents";

/**
 * Every figure on the queue is the proxy's count of the whole window. The
 * rows are a page of at most 200, and the page says so when there are more:
 * it never counts its own rows and calls that the number of incidents.
 */

type Row = { request_id: string; action: string; provider: string; model: string; event_type: string; findings: unknown[]; findings_count: number; timestamp: string };

const row = (i: number, action: string): Row => ({
  request_id: `req-${action.toLowerCase()}-${String(i).padStart(4, "0")}`,
  action,
  provider: i % 2 ? "openai" : "anthropic",
  model: "m",
  event_type: action === "BLOCK" ? "request_blocked" : "request_scanned",
  findings: [],
  findings_count: 0,
  timestamp: new Date(Date.UTC(2026, 9, 11, 9, 0, 0) - i * 60_000).toISOString(),
});

/** What the proxy holds: far more than a page, of three kinds. */
const HELD = { BLOCK: 1_203, REDACT: 4_518, WARN: 31 };

/** The proxy's answer: the latest `limit` rows of the asked kinds, and its counts of all of them. */
function proxy(asked: string[] = []) {
  server.use(
    http.get("*/api/v1/events", ({ request }) => {
      const url = new URL(request.url);
      asked.push(url.search);
      const wanted = (url.searchParams.get("action") ?? "").split(",").filter(Boolean);
      const limit = Number(url.searchParams.get("limit") ?? "50");
      const kinds = (Object.keys(HELD) as (keyof typeof HELD)[]).filter((k) => wanted.includes(k));
      const action_counts = Object.fromEntries(kinds.map((k) => [k, HELD[k]]));
      const total = kinds.reduce((sum, k) => sum + HELD[k], 0);
      // The newest rows, the kinds mixed as they would arrive.
      const events = Array.from({ length: Math.min(limit, total) }, (_, i) => row(i, kinds[i % kinds.length]));
      return HttpResponse.json({ events, total, action_counts });
    }),
  );
  return asked;
}

function show() {
  localStorage.setItem(ADMIN_KEY_STORAGE, "test-key");
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(createElement(QueryClientProvider, { client: qc }, createElement(Incidents)));
}

beforeEach(() => {
  localStorage.clear();
});

describe("the incident queue", { timeout: 20_000 }, () => {
  it("asks the proxy for the requests it acted on, in a window it names", async () => {
    const asked = proxy();
    show();
    await screen.findByRole("table");
    const query = new URLSearchParams(asked[0]);
    // Not a page of every request, with the incidents picked out of it afterwards.
    expect(query.get("action")).toBe("BLOCK,REDACT,STRIP,WARN");
    expect(query.get("range")).toBe("7d");
    expect(query.get("limit")).toBe("200");
  });

  it("states the proxy's counts of the whole window, and that the rows are the latest of them", async () => {
    proxy();
    show();
    const lede = await screen.findByText(/^The proxy acted on/);
    expect(lede).toHaveTextContent(
      "The proxy acted on 5,752 requests in the last 7 days: 1,203 stopped, 4,518 redacted, 31 warned. The latest 200 of 5,752 are listed, newest first.",
    );
    expect(screen.getByText("Latest 200 of 5,752")).toBeInTheDocument();
    // 200 rows are on the page; nothing calls that the number of incidents.
    expect(within(screen.getByRole("table")).getAllByRole("row")).toHaveLength(201);
    expect(screen.queryByText(/200 most recent|of 200 shown|67 of them/)).toBeNull();
  });

  it("asks again for one kind alone, so its list is the latest of that kind", async () => {
    const asked = proxy();
    show();
    await screen.findByRole("table");
    await userEvent.click(screen.getByRole("tab", { name: "Stopped" }));
    expect(await screen.findByText("Latest 200 of 1,203")).toBeInTheDocument();
    expect(asked.map((search) => new URLSearchParams(search).get("action")).sort()).toEqual(["BLOCK", "BLOCK,REDACT,STRIP,WARN"]);
    // The sentence above still counts the whole queue.
    expect(screen.getByText(/^The proxy acted on/)).toHaveTextContent("5,752 requests");
    expect(screen.getByText(/^The proxy acted on/)).toHaveTextContent("The latest 200 of 1,203 are listed");
  });

  it("says a search looks through the listed rows only, when there are more than are listed", async () => {
    proxy();
    show();
    await screen.findByRole("table");
    await userEvent.type(screen.getByRole("searchbox", { name: "Search incidents" }), "req-warn-0002");
    expect(await screen.findByText("1 of the latest 200 listed match")).toBeInTheDocument();
    await userEvent.clear(screen.getByRole("searchbox", { name: "Search incidents" }));
    await userEvent.type(screen.getByRole("searchbox", { name: "Search incidents" }), "no such request");
    expect(await screen.findByText(/^No incident among the latest 200 matches these filters\. Older ones are not searched here/)).toBeInTheDocument();
  });

  it("says the queue is complete when the proxy holds no more than is listed", async () => {
    server.use(
      http.get("*/api/v1/events", () =>
        HttpResponse.json({ events: [row(0, "BLOCK"), row(1, "REDACT"), row(2, "REDACT")], total: 3, action_counts: { BLOCK: 1, REDACT: 2 } }),
      ),
    );
    show();
    expect(await screen.findByText(/^The proxy acted on/)).toHaveTextContent(
      "The proxy acted on 3 requests in the last 7 days: 1 stopped, 2 redacted. They are listed newest first.",
    );
    expect(screen.getByText("3 listed")).toBeInTheDocument();
  });

  it("counts nothing itself when the proxy sends no counts by action", async () => {
    // A proxy older than the field: the total is still its own.
    server.use(http.get("*/api/v1/events", () => HttpResponse.json({ events: [row(0, "BLOCK"), row(1, "REDACT")], total: 2 })));
    show();
    expect(await screen.findByText(/^The proxy acted on/)).toHaveTextContent("The proxy acted on 2 requests in the last 7 days. They are listed newest first.");
  });

  it("says the window is empty, not that nothing was ever stopped", async () => {
    server.use(http.get("*/api/v1/events", () => HttpResponse.json({ events: [], total: 0, action_counts: {} })));
    show();
    expect(await screen.findByText("Nothing was stopped or redacted in the last 7 days. Requests that pass do not appear here.")).toBeInTheDocument();
    expect(screen.getByText("The queue is empty.")).toBeInTheDocument();
  });
});
