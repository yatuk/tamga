import { beforeEach, describe, expect, it } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { createElement } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ADMIN_KEY_STORAGE } from "@/hooks/useAdminKey";
import ProxyPage from "./page";

function show() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(createElement(QueryClientProvider, { client: qc }, createElement(ProxyPage)));
}

beforeEach(() => {
  localStorage.removeItem(ADMIN_KEY_STORAGE);
});

describe("Proxy status page", () => {
  it("without a key says the proxy is up and that the detail needs a key", async () => {
    show();
    await waitFor(() => expect(screen.getByText("Up")).toBeInTheDocument());
    expect(screen.getByText("No admin key in this browser.")).toBeInTheDocument();
    expect(screen.getByText(/shown once the proxy accepts an admin key/)).toBeInTheDocument();
    // Nothing made of defaults stands in for the state of the proxy.
    expect(screen.queryByRole("table")).toBeNull();
    expect(screen.queryByText("Not known")).toBeNull();
    expect(screen.queryByText("Not set up")).toBeNull();
    expect(screen.queryByText(/Off: terminate/)).toBeNull();
    expect(screen.getAllByText("Needs an admin key")).toHaveLength(5);
  });

  it("with a key the proxy rejects says so, and still that the proxy is up", async () => {
    localStorage.setItem(ADMIN_KEY_STORAGE, "wrong-key");
    show();
    await waitFor(() => expect(screen.getByText("The proxy did not accept the admin key.")).toBeInTheDocument());
    expect(screen.getByText("Up")).toBeInTheDocument();
    expect(screen.queryByText("The proxy did not answer.")).toBeNull();
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("with a key the proxy accepts shows every part", async () => {
    localStorage.setItem(ADMIN_KEY_STORAGE, "test-key");
    show();
    await waitFor(() => expect(screen.getByRole("table")).toBeInTheDocument());
    expect(screen.queryByText("No admin key in this browser.")).toBeNull();
    expect(screen.queryByText("The proxy did not accept the admin key.")).toBeNull();
    expect(screen.queryByText("Needs an admin key")).toBeNull();
    expect(screen.getByText("Policy engine")).toBeInTheDocument();
  });
});
