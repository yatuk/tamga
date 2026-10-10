import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { openLiveStream, retryDelay, type LiveEvent, type LiveStreamStatus } from "@/lib/api/live-stream";

const KEY = "stream-test-key";
const encoder = new TextEncoder();

/** A response whose body the test feeds and ends by hand. */
function stream() {
  let controller!: ReadableStreamDefaultController<Uint8Array>;
  const body = new ReadableStream<Uint8Array>({ start: (c) => void (controller = c) });
  return {
    response: new Response(body, { status: 200, headers: { "Content-Type": "text/event-stream" } }),
    send: (text: string) => controller.enqueue(encoder.encode(text)),
    end: () => controller.close(),
  };
}

const refusal = (status: number, body: object) => new Response(JSON.stringify(body), { status });

function watch() {
  const statuses: LiveStreamStatus[] = [];
  const events: LiveEvent[] = [];
  return { statuses, events, handlers: { onStatus: (s: LiveStreamStatus) => statuses.push(s), onEvent: (e: LiveEvent) => events.push(e) } };
}

let fetchMock: ReturnType<typeof vi.fn>;
/** Lets the promise chain of the stream run without moving the clock. */
const settle = () => vi.advanceTimersByTimeAsync(0);

beforeEach(() => {
  vi.useFakeTimers();
  fetchMock = vi.fn();
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("openLiveStream", () => {
  it("sends nothing without a key, and says the feed needs one", async () => {
    const w = watch();
    const close = openLiveStream("", w.handlers);
    await settle();
    expect(fetchMock).not.toHaveBeenCalled();
    expect(w.statuses).toEqual(["no-key"]);
    close();
  });

  it("sends the key in a header and never in the URL", async () => {
    const s = stream();
    fetchMock.mockResolvedValue(s.response);
    const w = watch();
    const close = openLiveStream(KEY, w.handlers);
    await settle();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("http://localhost:3000/api/v1/live/events");
    expect(url).not.toContain(KEY);
    expect(url).not.toContain("?");
    expect(new Headers(init.headers).get("X-Tamga-Admin-Key")).toBe(KEY);
    expect(w.statuses).toEqual(["connecting", "open"]);
    close();
  });

  it("hands over each event, whole, however the bytes arrive", async () => {
    const s = stream();
    fetchMock.mockResolvedValue(s.response);
    const w = watch();
    const close = openLiveStream(KEY, w.handlers);
    await settle();

    s.send(": tamga-live\n\n");
    s.send('event: request_scanned\ndata: {"request_id"');
    await settle();
    expect(w.events).toEqual([]);
    s.send(':"a"}\n\nevent: request_blocked\r\ndata: {"request_id":"b"}\r\n\r\n');
    await settle();

    expect(w.events).toEqual([
      { type: "request_scanned", data: '{"request_id":"a"}' },
      { type: "request_blocked", data: '{"request_id":"b"}' },
    ]);
    close();
  });

  it.each([
    [401, { error: "unauthorized" }, "refused"],
    [403, { error: "role none lacks live:read", code: "permission_denied" }, "forbidden"],
  ] as const)("stops on %i and does not ask again", async (status, body, expected) => {
    fetchMock.mockImplementation(() => Promise.resolve(refusal(status, body)));
    const w = watch();
    const close = openLiveStream(KEY, w.handlers);
    await settle();
    expect(w.statuses).toEqual(["connecting", expected]);

    // However long the page stays open, the same key is not tried again.
    await vi.advanceTimersByTimeAsync(10 * 60_000);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(w.statuses).toEqual(["connecting", expected]);
    close();
  });

  it("tries again after a dropped stream, waiting longer each time", async () => {
    const first = stream();
    fetchMock.mockResolvedValueOnce(first.response).mockRejectedValue(new TypeError("Failed to fetch"));
    const w = watch();
    const close = openLiveStream(KEY, w.handlers);
    await settle();
    expect(w.statuses).toEqual(["connecting", "open"]);

    first.end();
    await settle();
    expect(w.statuses.at(-1)).toBe("error");
    expect(fetchMock).toHaveBeenCalledTimes(1);

    // 1s, then 2s, then 4s: no attempt comes before its wait is over.
    await vi.advanceTimersByTimeAsync(999);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(fetchMock).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(1_999);
    expect(fetchMock).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(1);
    expect(fetchMock).toHaveBeenCalledTimes(3);
    await vi.advanceTimersByTimeAsync(3_999);
    expect(fetchMock).toHaveBeenCalledTimes(3);
    await vi.advanceTimersByTimeAsync(1);
    expect(fetchMock).toHaveBeenCalledTimes(4);

    // It says "reconnecting" throughout, not "connecting" again.
    expect(w.statuses.filter((s) => s === "connecting")).toHaveLength(1);
    close();
  });

  it("starts the waits over after a stream that held", async () => {
    const first = stream();
    const second = stream();
    fetchMock
      .mockRejectedValueOnce(new TypeError("Failed to fetch"))
      .mockRejectedValueOnce(new TypeError("Failed to fetch"))
      .mockResolvedValueOnce(first.response)
      .mockResolvedValueOnce(second.response);
    const w = watch();
    const close = openLiveStream(KEY, w.handlers);
    // Two failures: waits of 1s and 2s, then a stream that opens.
    await vi.advanceTimersByTimeAsync(3_000);
    expect(w.statuses.at(-1)).toBe("open");

    // It holds for a minute, then drops: the next attempt is 1s away, not 4s.
    await vi.advanceTimersByTimeAsync(60_000);
    first.end();
    await settle();
    expect(w.statuses.at(-1)).toBe("error");
    await vi.advanceTimersByTimeAsync(1_000);
    expect(fetchMock).toHaveBeenCalledTimes(4);
    expect(w.statuses.at(-1)).toBe("open");
    close();
  });

  it("says Live again once a retry gets through", async () => {
    const second = stream();
    fetchMock.mockRejectedValueOnce(new TypeError("Failed to fetch")).mockResolvedValueOnce(second.response);
    const w = watch();
    const close = openLiveStream(KEY, w.handlers);
    await settle();
    expect(w.statuses).toEqual(["connecting", "error"]);
    await vi.advanceTimersByTimeAsync(1_000);
    expect(w.statuses).toEqual(["connecting", "error", "open"]);
    close();
  });

  it("retries a 503 but not a 404", async () => {
    fetchMock.mockResolvedValueOnce(refusal(503, { error: "live stream unavailable" })).mockResolvedValueOnce(refusal(404, {}));
    const w = watch();
    const close = openLiveStream(KEY, w.handlers);
    await settle();
    expect(w.statuses).toEqual(["connecting", "error"]);
    await vi.advanceTimersByTimeAsync(1_000);
    expect(w.statuses).toEqual(["connecting", "error", "closed"]);
    await vi.advanceTimersByTimeAsync(60_000);
    expect(fetchMock).toHaveBeenCalledTimes(2);
    close();
  });

  it("aborts the request when closed, and reports nothing afterwards", async () => {
    const s = stream();
    let signal: AbortSignal | undefined;
    fetchMock.mockImplementation((_url: string, init: RequestInit) => {
      signal = init.signal ?? undefined;
      return Promise.resolve(s.response);
    });
    const w = watch();
    const close = openLiveStream(KEY, w.handlers);
    await settle();
    expect(signal?.aborted).toBe(false);

    close();
    expect(signal?.aborted).toBe(true);
    const seen = [...w.statuses];
    s.send("event: request_scanned\ndata: {}\n\n");
    s.end();
    await vi.advanceTimersByTimeAsync(60_000);
    expect(w.statuses).toEqual(seen);
    expect(w.events).toEqual([]);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("does not retry after being closed while waiting to", async () => {
    fetchMock.mockRejectedValue(new TypeError("Failed to fetch"));
    const w = watch();
    const close = openLiveStream(KEY, w.handlers);
    await settle();
    close();
    await vi.advanceTimersByTimeAsync(60_000);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("never puts the key in what it logs or reports", async () => {
    const logged: unknown[][] = [];
    for (const level of ["log", "warn", "error", "info", "debug"] as const) {
      vi.spyOn(console, level).mockImplementation((...args: unknown[]) => void logged.push(args));
    }
    fetchMock.mockResolvedValueOnce(refusal(503, {})).mockResolvedValueOnce(refusal(401, { error: "unauthorized" }));
    const w = watch();
    const close = openLiveStream(KEY, w.handlers);
    await vi.advanceTimersByTimeAsync(1_000);
    close();
    expect(w.statuses).toEqual(["connecting", "error", "refused"]);
    expect(JSON.stringify(logged)).not.toContain(KEY);
    expect(JSON.stringify(w.statuses)).not.toContain(KEY);
  });
});

describe("retryDelay", () => {
  it("doubles from one second and stops at thirty", () => {
    expect([0, 1, 2, 3, 4, 5, 6, 50].map(retryDelay)).toEqual([1_000, 2_000, 4_000, 8_000, 16_000, 30_000, 30_000, 30_000]);
  });
});
