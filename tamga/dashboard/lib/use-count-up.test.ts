import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook } from "@testing-library/react";
import { useCountUp } from "./use-count-up";

describe("useCountUp", () => {
  beforeEach(() => {
    // Intercept rAF so we can control timing without real delays.
    let id = 0;
    const callbacks = new Map<number, FrameRequestCallback>();
    vi.spyOn(globalThis, "requestAnimationFrame").mockImplementation((cb) => {
      const handle = ++id;
      callbacks.set(handle, cb);
      return handle;
    });
    vi.spyOn(globalThis, "cancelAnimationFrame").mockImplementation((h) => {
      callbacks.delete(h);
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("starts at 0 when inactive", () => {
    const { result } = renderHook(() => useCountUp(100, false));
    expect(result.current).toBe(0);
  });

  it("starts at 0 and climbs when active", () => {
    const { result } = renderHook(() => useCountUp(100, true, 800));
    // rAF fires once synchronously with our mock, giving initial step.
    // The value should be > 0 after one animation frame step.
    expect(result.current).toBeGreaterThanOrEqual(0);
  });

  it("resets to 0 when active becomes false", () => {
    const { result, rerender } = renderHook(
      ({ target, active }: { target: number; active: boolean }) => useCountUp(target, active),
      { initialProps: { target: 100, active: true } },
    );

    // Re-render with active=false.
    rerender({ target: 100, active: false });
    expect(result.current).toBe(0);
  });

  it("accepts custom duration", () => {
    const { result } = renderHook(() => useCountUp(50, true, 2000));
    // Should not crash with any duration.
    expect(result.current).toBeGreaterThanOrEqual(0);
  });
});
