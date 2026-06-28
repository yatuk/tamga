import { describe, it, expect, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import React, { createElement } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useCostsPage } from "./useCostsPage";

const qc = new QueryClient({
  defaultOptions: { queries: { retry: false } },
});

function Wrapper(props: Record<string, unknown>) {
  return createElement(QueryClientProvider, { client: qc }, props.children as React.ReactNode);
}

beforeEach(function () {
  localStorage.setItem("tamga_admin_key", "test-key");
});

describe("useCostsPage", function () {
  it("returns loading state initially", function () {
    const { result } = renderHook(function () { return useCostsPage(); }, { wrapper: Wrapper });
    expect(result.current.isLoading).toBe(true);
  });

  it("resolves budget stats after fetch", async function () {
    const { result } = renderHook(function () { return useCostsPage(); }, { wrapper: Wrapper });
    await waitFor(function () { return expect(result.current.isLoading).toBe(false); }, { timeout: 3000 });
    expect(result.current.tokensToday).toBeGreaterThan(0);
    expect(result.current.costToday).toBeGreaterThan(0);
  });

  it("produces model cost rows", async function () {
    const { result } = renderHook(function () { return useCostsPage(); }, { wrapper: Wrapper });
    await waitFor(function () { return expect(result.current.isLoading).toBe(false); }, { timeout: 3000 });
    expect(result.current.modelCostRows.length).toBeGreaterThan(0);
    expect(result.current.modelCostRows[0]).toHaveProperty("model");
    expect(result.current.modelCostRows[0]).toHaveProperty("cost");
  });
});
