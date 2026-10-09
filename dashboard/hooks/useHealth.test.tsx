import { beforeEach, describe, expect, it } from "vitest";
import { act, render, renderHook, screen, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { server } from "@/__mocks__/server";
import { HealthDetailNotice } from "@/components/console/health";
import { useProxyHealth } from "@/components/console/use-proxy-health";
import { ADMIN_KEY_STORAGE } from "@/hooks/useAdminKey";
import { useHealth, useHealthDetail } from "./useHealth";

let qc: QueryClient;

function Wrapper({ children }: { children?: ReactNode }) {
  return createElement(QueryClientProvider, { client: qc }, children);
}

/** Stores or removes the admin key the way Settings does. */
function setKey(key: string) {
  if (key) localStorage.setItem(ADMIN_KEY_STORAGE, key);
  else localStorage.removeItem(ADMIN_KEY_STORAGE);
  window.dispatchEvent(new Event("tamga:admin-key"));
}

beforeEach(() => {
  qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  localStorage.removeItem(ADMIN_KEY_STORAGE);
});

describe("useHealth without an admin key", () => {
  it("resolves the header's state from the public body", async () => {
    const { result } = renderHook(() => ({ header: useProxyHealth(), health: useHealth() }), { wrapper: Wrapper });
    expect(result.current.header.up).toBeUndefined();
    await waitFor(() => expect(result.current.header.up).toBe(true));
    expect(result.current.header.reason).toBe("");
    expect(result.current.header.scanP50).toBe(8.4);
    expect(result.current.health.detail).toBeUndefined();
    expect(result.current.health.detailState).toBe("no-key");
  });

  it("sends no key header", async () => {
    let sent: string | null = "unset";
    server.use(
      http.get("/api/v1/health/detailed", ({ request }) => {
        sent = request.headers.get("X-Tamga-Admin-Key");
        return HttpResponse.json({ proxy: "up", authenticated: false });
      }),
    );
    const { result } = renderHook(() => useHealth(), { wrapper: Wrapper });
    await waitFor(() => expect(result.current.detailState).toBe("no-key"));
    expect(sent).toBeNull();
  });

  it("reports the proxy as down when it does not answer", async () => {
    server.use(http.get("/api/v1/health/detailed", () => HttpResponse.error()));
    const { result } = renderHook(() => ({ header: useProxyHealth(), health: useHealth() }), { wrapper: Wrapper });
    await waitFor(() => expect(result.current.header.up).toBe(false));
    // Nothing is known, so nothing is said about the key either.
    expect(result.current.health.detailState).toBe("pending");
  });
});

describe("useHealth with an admin key", () => {
  it("sends the key and uses the full body", async () => {
    setKey("test-key");
    let sent: string | null = null;
    server.use(
      http.get("/api/v1/health/detailed", ({ request }) => {
        sent = request.headers.get("X-Tamga-Admin-Key");
        return HttpResponse.json({ proxy: "up", authenticated: true, database: "connected", scanner_count: 6, uptime_seconds: 60, policy_path: "/p" });
      }),
    );
    const { result } = renderHook(() => useHealth(), { wrapper: Wrapper });
    await waitFor(() => expect(result.current.detailState).toBe("full"));
    expect(sent).toBe("test-key");
    expect(result.current.detail?.database).toBe("connected");
    expect(result.current.detail?.scanner_count).toBe(6);
    expect(result.current.data?.proxy).toBe("up");
  });

  it("replaces the anonymous body when a key is entered later", async () => {
    const { result } = renderHook(() => ({ header: useProxyHealth(), health: useHealth() }), { wrapper: Wrapper });
    await waitFor(() => expect(result.current.health.detailState).toBe("no-key"));
    const seen: string[] = [];

    act(() => setKey("test-key"));
    // From the first render with the key, the anonymous body is not the detail and the header stays up.
    seen.push(result.current.health.detailState);
    expect(result.current.health.detail).toBeUndefined();
    expect(result.current.header.up).toBe(true);

    await waitFor(() => expect(result.current.health.detailState).toBe("full"));
    expect(seen).toEqual(["pending"]);
    expect(result.current.health.detail?.policy_path).toBe("/etc/tamga/policy.yaml");
  });

  it("drops the full body as soon as the key is removed", async () => {
    setKey("test-key");
    const { result } = renderHook(() => ({ header: useProxyHealth(), health: useHealth() }), { wrapper: Wrapper });
    await waitFor(() => expect(result.current.health.detailState).toBe("full"));

    act(() => setKey(""));
    expect(result.current.health.detail).toBeUndefined();
    expect(result.current.health.detailState).toBe("pending");
    expect(result.current.header.up).toBe(true);

    await waitFor(() => expect(result.current.health.detailState).toBe("no-key"));
    expect(result.current.health.detail).toBeUndefined();
    expect(result.current.health.data).not.toHaveProperty("policy_path");
  });

  it("degrades to the public body when the proxy rejects the key", async () => {
    setKey("wrong-key");
    const { result } = renderHook(() => ({ header: useProxyHealth(), health: useHealth() }), { wrapper: Wrapper });
    await waitFor(() => expect(result.current.health.detailState).toBe("rejected"));
    expect(result.current.health.detail).toBeUndefined();
    expect(result.current.health.isError).toBe(false);
    // The proxy is up; only its detail is missing.
    expect(result.current.header.up).toBe(true);
    expect(result.current.header.scanP50).toBe(8.4);
  });

  it("reads a body without the marker, from an older proxy, as the full one", async () => {
    server.use(
      http.get("/api/v1/health/detailed", () =>
        HttpResponse.json({ proxy: "up", database: "not_configured", scanner_count: 3, uptime_seconds: 1, policy_path: "/p" }),
      ),
    );
    const { result } = renderHook(() => useHealth(), { wrapper: Wrapper });
    await waitFor(() => expect(result.current.detailState).toBe("full"));
    expect(result.current.detail?.scanner_count).toBe(3);
  });
});

describe("useHealthDetail", () => {
  it("has no data without a key", async () => {
    const { result } = renderHook(() => useHealthDetail(), { wrapper: Wrapper });
    await waitFor(() => expect(result.current.detailState).toBe("no-key"));
    expect(result.current.data).toBeUndefined();
  });

  it("has the runtime detail with a key the proxy accepts", async () => {
    setKey("test-key");
    const { result } = renderHook(() => useHealthDetail(), { wrapper: Wrapper });
    await waitFor(() => expect(result.current.detailState).toBe("full"));
    expect(result.current.data?.version).toBe("v1.1.0");
    expect(result.current.data?.tls_enabled).toBe(true);
  });

  it("has no data with a key the proxy rejects", async () => {
    setKey("wrong-key");
    const { result } = renderHook(() => useHealthDetail(), { wrapper: Wrapper });
    await waitFor(() => expect(result.current.detailState).toBe("rejected"));
    expect(result.current.data).toBeUndefined();
  });
});

describe("HealthDetailNotice", () => {
  it("says the detail needs an admin key when there is none", () => {
    render(createElement(HealthDetailNotice, { state: "no-key" }));
    expect(screen.getByText("No admin key in this browser.")).toBeInTheDocument();
  });

  it("says the key was not accepted when the proxy rejected it", () => {
    render(createElement(HealthDetailNotice, { state: "rejected" }));
    expect(screen.getByText("The proxy did not accept the admin key.")).toBeInTheDocument();
  });

  it("says nothing while the detail is there or on its way", () => {
    const { container, rerender } = render(createElement(HealthDetailNotice, { state: "full" }));
    expect(container).toBeEmptyDOMElement();
    rerender(createElement(HealthDetailNotice, { state: "pending" }));
    expect(container).toBeEmptyDOMElement();
  });
});
