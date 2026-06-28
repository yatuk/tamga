import { describe, it, expect, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import React, { createElement } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useLatencyPage } from "./useLatencyPage";

const qc = new QueryClient({
  defaultOptions: { queries: { retry: false } },
});

function Wrapper(props: Record<string, unknown>) {
  return createElement(QueryClientProvider, { client: qc }, props.children as React.ReactNode);
}

beforeEach(function () {
  localStorage.setItem("tamga_admin_key", "test-key");
});

describe("useLatencyPage", function () {
  it("returns loading state initially", function () {
    const { result } = renderHook(function () { return useLatencyPage(); }, { wrapper: Wrapper });
    expect(result.current.isLoading).toBe(true);
  });

  it("resolves P50/P95/P99 after fetch", async function () {
    const { result } = renderHook(function () { return useLatencyPage(); }, { wrapper: Wrapper });
    await waitFor(function () { return expect(result.current.isLoading).toBe(false); }, { timeout: 3000 });
    expect(result.current.p50).toBeGreaterThan(0);
    expect(result.current.p95).toBeGreaterThan(0);
    expect(result.current.p99).toBeGreaterThan(0);
    expect(result.current.p50).toBeLessThanOrEqual(result.current.p95);
  });

  it("extracts provider pools from health", async function () {
    const { result } = renderHook(function () { return useLatencyPage(); }, { wrapper: Wrapper });
    await waitFor(function () { return expect(result.current.isLoading).toBe(false); }, { timeout: 3000 });
    expect(result.current.providerPools.length).toBeGreaterThan(0);
    expect(result.current.providerPools[0]).toHaveProperty("name");
    expect(result.current.providerPools[0]).toHaveProperty("state");
  });
});
