import { describe, it, expect, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import React, { createElement } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useTrafficPage } from "./useTrafficPage";

const qc = new QueryClient({
  defaultOptions: { queries: { retry: false } },
});

function Wrapper(props: Record<string, unknown>) {
  return createElement(QueryClientProvider, { client: qc }, props.children as React.ReactNode);
}

beforeEach(function () {
  localStorage.setItem("tamga_admin_key", "test-key");
});

describe("useTrafficPage", function () {
  it("returns loading state initially", function () {
    const { result } = renderHook(function () { return useTrafficPage(); }, { wrapper: Wrapper });
    expect(result.current.isLoading).toBe(true);
  });

  it("resolves stats after fetch", async function () {
    const { result } = renderHook(function () { return useTrafficPage(); }, { wrapper: Wrapper });
    await waitFor(function () { return expect(result.current.isLoading).toBe(false); }, { timeout: 3000 });
    expect(result.current.totalRequests).toBeGreaterThan(0);
  });

  it("produces chart data from timeseries", async function () {
    const { result } = renderHook(function () { return useTrafficPage(); }, { wrapper: Wrapper });
    await waitFor(function () { return expect(result.current.isLoading).toBe(false); }, { timeout: 3000 });
    expect(result.current.chartData.length).toBeGreaterThan(0);
  });

  it("produces model usage data", async function () {
    const { result } = renderHook(function () { return useTrafficPage(); }, { wrapper: Wrapper });
    await waitFor(function () { return expect(result.current.isLoading).toBe(false); }, { timeout: 3000 });
    expect(result.current.modelUsage.length).toBeGreaterThan(0);
  });
});
