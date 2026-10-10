import { beforeEach, describe, expect, it } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { createElement, type ComponentType } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { delay, http, HttpResponse } from "msw";
import { server } from "@/__mocks__/server";
import { ADMIN_KEY_STORAGE } from "@/hooks/useAdminKey";
import CostsPage from "./costs/page";
import { Overview } from "./overview";
import TrafficPage from "./traffic/page";
import TrendsPage from "./trends/page";

/**
 * A page with a key present, before its figures have arrived and when its
 * first read fails. Neither may draw a figure: a 0 would read as a quiet
 * week, and "nothing in this window" as a fact.
 */

function show(page: ComponentType) {
  localStorage.setItem(ADMIN_KEY_STORAGE, "test-key");
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(createElement(QueryClientProvider, { client: qc }, createElement(page)));
}

const PAGES: [name: string, page: ComponentType, read: string, title: string][] = [
  ["Traffic", TrafficPage, "*/api/v1/stats", "Traffic"],
  ["Detection trends", TrendsPage, "*/api/v1/timeseries", "Detection trends"],
  ["Token costs", CostsPage, "*/api/v1/billing/costs/breakdown", "Token costs"],
  ["Overview", Overview, "*/api/v1/stats", "Overview"],
];

// The pages retry a failed read once before they give up on it.
const SLOW = { timeout: 8000 };

const drawn = (container: HTMLElement) =>
  container.querySelector(".t-figure, .t-facts, .t-kinds, .t-days, .t-trace, .t-empty, table") !== null;

beforeEach(() => {
  localStorage.clear();
});

describe.each(PAGES)("%s", { timeout: 20_000 }, (_name, page, read, title) => {
  it("shows the notice and no figures when its first read fails with a key present", async () => {
    server.use(http.get(read, () => HttpResponse.json({ error: "boom" }, { status: 500 })));
    const { container } = show(page);
    expect(await screen.findByText("The proxy is up, but it did not return this page's data.", {}, SLOW)).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 1, name: title })).toBeInTheDocument();
    expect(drawn(container)).toBe(false);
    expect(screen.queryByRole("status", { name: "Loading" })).toBeNull();
  });

  it("says a role that is too low for what it is, and draws no figures", async () => {
    server.use(
      http.get(read, () =>
        HttpResponse.json({ error: "role viewer lacks stats:read", code: "permission_denied", required_permission: "stats:read" }, { status: 403 }),
      ),
    );
    const { container } = show(page);
    expect(await screen.findByText(/The proxy asks for the stats:read permission\./, {}, SLOW)).toBeInTheDocument();
    expect(drawn(container)).toBe(false);
  });

  it("draws nothing in place of its figures while they are on their way", async () => {
    let release = false;
    server.use(
      http.get(read, async () => {
        while (!release) await delay(20);
        return HttpResponse.json({ error: "boom" }, { status: 500 });
      }),
    );
    const { container } = show(page);
    await screen.findByRole("status", { name: "Loading" });
    // Give every other read time to come back: the page still waits for this one.
    await delay(120);
    expect(screen.getByRole("status", { name: "Loading" })).toBeInTheDocument();
    expect(drawn(container)).toBe(false);
    release = true;
    await waitFor(() => expect(screen.queryByRole("status", { name: "Loading" })).toBeNull(), SLOW);
  });
});

describe("with the figures read", () => {
  it.each(PAGES)("%s draws them", async (_name, page) => {
    const { container } = show(page);
    await waitFor(() => expect(container.querySelector(".t-figure")).not.toBeNull());
    expect(screen.queryByRole("status", { name: "Loading" })).toBeNull();
    expect(container.querySelector(".t-facts")).not.toBeNull();
  });
});
