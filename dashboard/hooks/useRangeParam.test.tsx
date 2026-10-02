import { describe, expect, it, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { withNuqsTestingAdapter } from "nuqs/adapters/testing";

vi.unmock("@/hooks/useRangeParam");
const { useRangeParam } = await import("./useRangeParam");

describe("useRangeParam", () => {
  it("falls back to the default when the URL has no range", () => {
    const { result } = renderHook(() => useRangeParam("7d"), { wrapper: withNuqsTestingAdapter() });
    expect(result.current[0]).toBe("7d");
  });

  it("reads the range from the URL", () => {
    const { result } = renderHook(() => useRangeParam("7d"), {
      wrapper: withNuqsTestingAdapter({ searchParams: "?range=30d" }),
    });
    expect(result.current[0]).toBe("30d");
  });

  it("ignores a value that is not a known range", () => {
    const { result } = renderHook(() => useRangeParam("24h"), {
      wrapper: withNuqsTestingAdapter({ searchParams: "?range=forever" }),
    });
    expect(result.current[0]).toBe("24h");
  });

  it("writes a new range to the URL", async () => {
    const onUrlUpdate = vi.fn();
    const { result } = renderHook(() => useRangeParam("7d"), {
      wrapper: withNuqsTestingAdapter({ onUrlUpdate }),
    });
    await act(async () => result.current[1]("24h"));
    expect(result.current[0]).toBe("24h");
    // nuqs batches URL writes, so the update lands a moment after the state.
    await waitFor(() => expect(onUrlUpdate).toHaveBeenCalledOnce());
    expect(onUrlUpdate.mock.calls[0][0].queryString).toBe("?range=24h");
  });
});
