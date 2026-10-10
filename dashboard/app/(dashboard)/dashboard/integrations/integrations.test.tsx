import { beforeEach, describe, expect, it } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import { createElement } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ADMIN_KEY_STORAGE } from "@/hooks/useAdminKey";
import { webhookHost } from "./integrationWebhookHelpers";
import IntegrationsPage from "./page";

function show() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(createElement(QueryClientProvider, { client: qc }, createElement(IntegrationsPage)));
}

beforeEach(() => {
  localStorage.removeItem(ADMIN_KEY_STORAGE);
});

describe("webhookHost", () => {
  it("is the host alone for an HTTPS destination", () => {
    expect(webhookHost("https://hooks.slack.com/[redacted]")).toBe("hooks.slack.com");
  });
  it("keeps the scheme and port of one that is not HTTPS", () => {
    expect(webhookHost("http://collector.internal:8088/[redacted]")).toBe("http://collector.internal:8088");
  });
  it("drops the marker from something that is not a URL", () => {
    expect(webhookHost("not a url/[redacted]")).toBe("not a url");
  });
});

describe("Integrations page", () => {
  it("shows the host of each destination, and says once what is kept back", async () => {
    localStorage.setItem(ADMIN_KEY_STORAGE, "test-key");
    show();
    const table = await screen.findByRole("table", { name: "Connected destinations" });
    const rows = within(table).getAllByRole("row");
    expect(within(table).getByRole("columnheader", { name: "Host" })).toBeInTheDocument();
    expect(within(rows[1]).getByText("Security channel")).toBeInTheDocument();
    expect(within(rows[1]).getByText("hooks.slack.com")).toBeInTheDocument();
    expect(within(rows[1]).getByText("Never")).toBeInTheDocument();
    expect(within(rows[2]).getByText("http://collector.internal:8088")).toBeInTheDocument();
    expect(within(rows[2]).getByText("Switched off")).toBeInTheDocument();

    expect(table.textContent).not.toContain("[redacted]");
    expect(screen.getAllByText(/Only the host of each destination is shown/)).toHaveLength(1);
    expect(screen.getByText(/2 destinations, 1 of them switched on/)).toBeInTheDocument();
  });

  it("says the key's role cannot read the list, and does not claim nothing is connected", async () => {
    localStorage.setItem(ADMIN_KEY_STORAGE, "viewer-key");
    show();
    await waitFor(() => expect(screen.getByText("This key's role cannot read the connected destinations.")).toBeInTheDocument());
    expect(screen.getByText(/The proxy asks for the webhooks:read permission/)).toBeInTheDocument();
    expect(screen.queryByText(/Nothing is connected yet/)).toBeNull();
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("without a key says so, and does not claim nothing is connected", () => {
    show();
    expect(screen.getByText("No admin key in this browser.")).toBeInTheDocument();
    expect(screen.queryByText(/Nothing is connected yet/)).toBeNull();
  });
});
