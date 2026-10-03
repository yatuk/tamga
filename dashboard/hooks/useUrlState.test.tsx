import { describe, expect, it, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { withNuqsTestingAdapter } from "nuqs/adapters/testing";

vi.unmock("@/hooks/useUrlState");
const { useEnumParam, useFlagParam, useStringParam } = await import("./useUrlState");

const TABS = ["editor", "diff", "history"] as const;

describe("useEnumParam", () => {
  it("uses the default when the URL has no value", () => {
    const { result } = renderHook(() => useEnumParam("tab", TABS, "editor"), { wrapper: withNuqsTestingAdapter() });
    expect(result.current[0]).toBe("editor");
  });

  it("reads a known value from the URL", () => {
    const { result } = renderHook(() => useEnumParam("tab", TABS, "editor"), {
      wrapper: withNuqsTestingAdapter({ searchParams: "?tab=history" }),
    });
    expect(result.current[0]).toBe("history");
  });

  it("falls back to the default for a value outside the set", () => {
    const { result } = renderHook(() => useEnumParam("tab", TABS, "editor"), {
      wrapper: withNuqsTestingAdapter({ searchParams: "?tab=settings" }),
    });
    expect(result.current[0]).toBe("editor");
  });

  it("writes the choice to the URL and leaves the default out", async () => {
    const onUrlUpdate = vi.fn();
    const { result } = renderHook(() => useEnumParam("tab", TABS, "editor"), {
      wrapper: withNuqsTestingAdapter({ onUrlUpdate }),
    });
    await act(async () => result.current[1]("diff"));
    await waitFor(() => expect(onUrlUpdate).toHaveBeenCalledTimes(1));
    expect(onUrlUpdate.mock.calls[0][0].queryString).toBe("?tab=diff");

    await act(async () => result.current[1]("editor"));
    await waitFor(() => expect(onUrlUpdate).toHaveBeenCalledTimes(2));
    expect(onUrlUpdate.mock.calls[1][0].queryString).toBe("");
  });
});

describe("useStringParam", () => {
  it("is empty by default and reads text from the URL", () => {
    const empty = renderHook(() => useStringParam("q"), { wrapper: withNuqsTestingAdapter() });
    expect(empty.result.current[0]).toBe("");

    const set = renderHook(() => useStringParam("q"), {
      wrapper: withNuqsTestingAdapter({ searchParams: "?q=credit_card" }),
    });
    expect(set.result.current[0]).toBe("credit_card");
  });

  it("removes the key when the text is cleared", async () => {
    const onUrlUpdate = vi.fn();
    const { result } = renderHook(() => useStringParam("q"), {
      wrapper: withNuqsTestingAdapter({ searchParams: "?q=abc", onUrlUpdate }),
    });
    await act(async () => result.current[1](""));
    expect(result.current[0]).toBe("");
    await waitFor(() => expect(onUrlUpdate).toHaveBeenCalledOnce());
    expect(onUrlUpdate.mock.calls[0][0].queryString).toBe("");
  });
});

describe("useFlagParam", () => {
  it("is on only for the value 1", () => {
    const on = renderHook(() => useFlagParam("shadow"), { wrapper: withNuqsTestingAdapter({ searchParams: "?shadow=1" }) });
    expect(on.result.current[0]).toBe(true);

    const other = renderHook(() => useFlagParam("shadow"), {
      wrapper: withNuqsTestingAdapter({ searchParams: "?shadow=true" }),
    });
    expect(other.result.current[0]).toBe(false);
  });

  it("writes 1 when turned on and removes the key when turned off", async () => {
    const onUrlUpdate = vi.fn();
    const { result } = renderHook(() => useFlagParam("shadow"), { wrapper: withNuqsTestingAdapter({ onUrlUpdate }) });
    await act(async () => result.current[1](true));
    await waitFor(() => expect(onUrlUpdate).toHaveBeenCalledTimes(1));
    expect(onUrlUpdate.mock.calls[0][0].queryString).toBe("?shadow=1");

    await act(async () => result.current[1](false));
    await waitFor(() => expect(onUrlUpdate).toHaveBeenCalledTimes(2));
    expect(onUrlUpdate.mock.calls[1][0].queryString).toBe("");
  });
});
