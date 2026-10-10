import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { useLiveEventsStream } from "./useLiveEventsStream";

const encoder = new TextEncoder();
let fetchMock: ReturnType<typeof vi.fn>;
let signals: AbortSignal[];
let feeds: ReadableStreamDefaultController<Uint8Array>[];

beforeEach(() => {
  signals = [];
  feeds = [];
  // Every call gets a stream that stays open until the test ends it.
  fetchMock = vi.fn((_url: string, init: RequestInit) => {
    if (init.signal) signals.push(init.signal);
    const body = new ReadableStream<Uint8Array>({ start: (c) => void feeds.push(c) });
    return Promise.resolve(new Response(body, { status: 200 }));
  });
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("useLiveEventsStream", () => {
  it("without a key connects to nothing and says the feed needs one", async () => {
    const { result } = renderHook(() => useLiveEventsStream(""));
    await waitFor(() => expect(result.current.status).toBe("no-key"));
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("counts scanned and stopped requests, and nothing else", async () => {
    const { result } = renderHook(() => useLiveEventsStream("test-key"));
    await waitFor(() => expect(result.current.status).toBe("open"));
    act(() => {
      feeds[0].enqueue(
        encoder.encode("event: request_scanned\ndata: {}\n\nevent: policy_reloaded\ndata: {}\n\nevent: request_blocked\ndata: {}\n\n"),
      );
    });
    await waitFor(() => expect(result.current.liveCount).toBe(2));
    act(() => result.current.resetCounter());
    expect(result.current.liveCount).toBe(0);
  });

  it("aborts the request when the page is left", async () => {
    const { result, unmount } = renderHook(() => useLiveEventsStream("test-key"));
    await waitFor(() => expect(result.current.status).toBe("open"));
    expect(signals[0].aborted).toBe(false);
    unmount();
    expect(signals[0].aborted).toBe(true);
  });

  it("stays shut after a refusal until the key changes, then opens with the new key", async () => {
    fetchMock.mockImplementationOnce(() => Promise.resolve(new Response(JSON.stringify({ error: "unauthorized" }), { status: 401 })));
    const { result, rerender } = renderHook(({ adminKey }) => useLiveEventsStream(adminKey), { initialProps: { adminKey: "wrong-key" } });
    await waitFor(() => expect(result.current.status).toBe("refused"));
    expect(fetchMock).toHaveBeenCalledTimes(1);

    rerender({ adminKey: "test-key" });
    await waitFor(() => expect(result.current.status).toBe("open"));
    expect(fetchMock).toHaveBeenCalledTimes(2);
    const [, init] = fetchMock.mock.calls[1] as [string, RequestInit];
    expect(new Headers(init.headers).get("X-Tamga-Admin-Key")).toBe("test-key");
  });
});
