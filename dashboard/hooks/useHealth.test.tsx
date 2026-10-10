import { beforeEach, describe, expect, it } from "vitest";
import { act, render, renderHook, screen, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { server } from "@/__mocks__/server";
import { HealthDetailNotice } from "@/components/console/health";
import { DataNotice } from "@/components/console/page";
import { useProxyHealth } from "@/components/console/use-proxy-health";
import { ADMIN_KEY_STORAGE } from "@/hooks/useAdminKey";
import { NEEDS_KEY, readHealth, useHealth, useHealthDetail } from "./useHealth";

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

describe("readHealth", () => {
  const at = 1_760_000_000_000;
  const read = (answered: boolean, failed: boolean, detailState: "pending" | "full" | "no-key" | "rejected") =>
    readHealth({ answered, failed, detailState, dataUpdatedAt: answered ? at : 0 });

  it("before the first check is back, is checking", () => {
    expect(read(false, false, "pending")).toEqual({ standing: "checking", stale: false, answeredAt: 0, missing: "Checking" });
  });

  it("never answered: nothing is known, and no key is asked for", () => {
    expect(read(false, true, "pending")).toEqual({ standing: "unreachable", stale: false, answeredAt: 0, missing: "Not known" });
  });

  it("answered, then failing: what is held is old, whatever the last answer was", () => {
    for (const detailState of ["full", "no-key", "rejected"] as const) {
      expect(read(true, true, detailState)).toEqual({ standing: "stale", stale: true, answeredAt: at, missing: "Not known" });
    }
  });

  it("answering without a key, or with one it refuses: a key is what is missing", () => {
    expect(read(true, false, "no-key")).toEqual({ standing: "no-key", stale: false, answeredAt: at, missing: NEEDS_KEY });
    expect(read(true, false, "rejected")).toEqual({ standing: "rejected", stale: false, answeredAt: at, missing: NEEDS_KEY });
  });

  it("answering with the detail", () => {
    expect(read(true, false, "full")).toEqual({ standing: "full", stale: false, answeredAt: at, missing: "Not known" });
  });

  it("an answer kept from another key is still being checked for this one", () => {
    expect(read(true, false, "pending").standing).toBe("checking");
  });
});

describe("the reading the hooks give", () => {
  const down = () =>
    server.use(
      http.get("*/api/v1/health/detailed", () => HttpResponse.error()),
      http.get("*/api/v1/health/detail", () => HttpResponse.error()),
    );

  it("goes from checking to the proxy's answer, and to stale when the next check fails", async () => {
    setKey("test-key");
    const { result } = renderHook(() => ({ health: useHealth(), detail: useHealthDetail() }), { wrapper: Wrapper });
    expect(result.current.health.reading.standing).toBe("checking");
    await waitFor(() => expect(result.current.health.reading.standing).toBe("full"));
    await waitFor(() => expect(result.current.detail.reading.standing).toBe("full"));
    const answeredAt = result.current.health.reading.answeredAt;
    expect(answeredAt).toBeGreaterThan(0);

    down();
    await act(() => qc.refetchQueries());
    await waitFor(() => expect(result.current.health.reading.standing).toBe("stale"));
    // The last answer is kept, with the time it was given, and the detail's own read says the same.
    expect(result.current.health.detail?.scanner_count).toBe(6);
    expect(result.current.health.reading.answeredAt).toBe(answeredAt);
    expect(result.current.health.reading.missing).toBe("Not known");
    await waitFor(() => expect(result.current.detail.reading.standing).toBe("stale"));
    expect(result.current.detail.data?.version).toBe("v1.1.0");
  });

  it("is unreachable when the proxy has never answered, with a key or without", async () => {
    down();
    const { result } = renderHook(() => ({ health: useHealth(), detail: useHealthDetail() }), { wrapper: Wrapper });
    await waitFor(() => expect(result.current.health.reading.standing).toBe("unreachable"));
    await waitFor(() => expect(result.current.detail.reading.standing).toBe("unreachable"));
    expect(result.current.health.reading.missing).toBe("Not known");
  });

  it("asks for a key only of a proxy that is answering", async () => {
    const { result } = renderHook(() => useHealth(), { wrapper: Wrapper });
    await waitFor(() => expect(result.current.reading.standing).toBe("no-key"));
    expect(result.current.reading.missing).toBe(NEEDS_KEY);
    down();
    await act(() => qc.refetchQueries());
    await waitFor(() => expect(result.current.reading.standing).toBe("stale"));
    expect(result.current.reading.missing).toBe("Not known");
  });
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
    rerender(createElement(HealthDetailNotice, { state: "checking" }));
    expect(container).toBeEmptyDOMElement();
    // "It is up" would be false of a proxy that has stopped answering, or never did.
    rerender(createElement(HealthDetailNotice, { state: "stale" }));
    expect(container).toBeEmptyDOMElement();
    rerender(createElement(HealthDetailNotice, { state: "unreachable" }));
    expect(container).toBeEmptyDOMElement();
  });
});

describe("useHealth when a part of the proxy is down", () => {
  it("reads a 503 that carries the health body as the proxy answering", async () => {
    server.use(
      http.get("*/api/v1/health/detailed", () =>
        HttpResponse.json(
          { authenticated: false, proxy: "up", proxy_status: { up: true }, status: "degraded" },
          { status: 503 },
        ),
      ),
    );
    const { result } = renderHook(() => ({ header: useProxyHealth(), health: useHealth() }), { wrapper: Wrapper });
    await waitFor(() => expect(result.current.health.data?.status).toBe("degraded"));
    expect(result.current.health.isError).toBe(false);
    expect(result.current.header.up).toBe(true);
  });

  it("still reports a 503 with no health body as the proxy not answering", async () => {
    server.use(http.get("*/api/v1/health/detailed", () => HttpResponse.json({ error: "bad gateway" }, { status: 503 })));
    const { result } = renderHook(() => useHealth(), { wrapper: Wrapper });
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.data).toBeUndefined();
  });
});

describe("DataNotice", () => {
  const show = (adminKey: string) => render(createElement(DataNotice, { adminKey, failed: true }), { wrapper: Wrapper });

  it("stops saying the key was refused, and that the proxy is up, once the proxy stops answering", async () => {
    setKey("wrong-key");
    show("wrong-key");
    expect(await screen.findByText("The proxy did not accept the admin key.")).toBeTruthy();
    server.use(http.get("*/api/v1/health/detailed", () => HttpResponse.error()));
    await act(() => qc.refetchQueries());
    expect(await screen.findByText("The proxy did not answer.")).toBeTruthy();
    expect(screen.queryByText("The proxy did not accept the admin key.")).toBeNull();
  });

  it("says the key was refused when the proxy is up and did not accept it", async () => {
    setKey("wrong-key");
    show("wrong-key");
    expect(await screen.findByText("The proxy did not accept the admin key.")).toBeTruthy();
  });

  it("says the proxy is up when the key is good and the page's data still failed", async () => {
    setKey("good-key");
    show("good-key");
    expect(await screen.findByText("The proxy is up, but it did not return this page's data.")).toBeTruthy();
  });

  it("says the proxy did not answer when its health check fails too", async () => {
    server.use(http.get("*/api/v1/health/detailed", () => HttpResponse.error()));
    setKey("good-key");
    show("good-key");
    expect(await screen.findByText("The proxy did not answer.")).toBeTruthy();
  });
});
