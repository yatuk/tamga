import { describe, expect, it, vi, beforeEach } from "vitest";
import { authHeaders, fetchAPI } from "@/lib/api/fetch-core";

// ---------------------------------------------------------------------------
// Helper: build a fresh fetch-like Response each call so the body stream
// isn't consumed across retries.
// ---------------------------------------------------------------------------
function res(
  status: number,
  body: unknown,
): { ok: boolean; status: number; json: () => Promise<unknown> } {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  };
}

// ---------------------------------------------------------------------------
// authHeaders
// ---------------------------------------------------------------------------
describe("authHeaders", () => {
  it("returns the X-Tamga-Admin-Key header when a key is provided", () => {
    expect(authHeaders("secret123")).toEqual({ "X-Tamga-Admin-Key": "secret123" });
  });

  it("returns an empty object when the key is an empty string", () => {
    expect(authHeaders("")).toEqual({});
  });
});

// ---------------------------------------------------------------------------
// fetchAPI
// ---------------------------------------------------------------------------
describe("fetchAPI", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  // -- success ---------------------------------------------------------------

  it("returns parsed JSON on a 200 response", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(res(200, { status: "ok" })));

    const result = await fetchAPI<{ status: string }>("/test");
    expect(result).toEqual({ status: "ok" });
  });

  // -- 401 handling -----------------------------------------------------------

  it("throws the Turkish message on 401 (caught by caller)", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(res(401, { error: "unauthorized" })));

    // retry=0 so the catch block breaks immediately on the first throw.
    await expect(fetchAPI("/test", { retry: 0 })).rejects.toThrow(
      "Admin key yanlış veya eksik",
    );
  });

  it("retries 401 if retries remain (catch block retries any throw), final error is still the 401 message", async () => {
    const mock = vi.fn().mockResolvedValue(res(401, { error: "unauthorized" }));
    vi.stubGlobal("fetch", mock);

    await expect(fetchAPI("/test", { retry: 2 })).rejects.toThrow(
      "Admin key yanlış veya eksik",
    );
    // retry=2 → 3 total attempts (0, 1, 2)
    expect(mock).toHaveBeenCalledTimes(3);
  });

  // -- retry on 5xx ----------------------------------------------------------

  it("retries on 5xx and returns the successful result", async () => {
    const mock = vi
      .fn()
      .mockResolvedValueOnce(res(503, { error: "boom" }))
      .mockResolvedValueOnce(res(502, { error: "boom" }))
      .mockResolvedValueOnce(res(200, { data: "ok" }));
    vi.stubGlobal("fetch", mock);

    const result = await fetchAPI<{ data: string }>("/test", { retry: 2 });
    expect(result).toEqual({ data: "ok" });
    expect(mock).toHaveBeenCalledTimes(3);
  });

  it("throws the error body message after exhausting retries on 5xx", async () => {
    const mock = vi.fn().mockResolvedValue(res(500, { error: "internal failure" }));
    vi.stubGlobal("fetch", mock);

    await expect(fetchAPI("/test", { retry: 1 })).rejects.toThrow(
      "internal failure",
    );
    // retry=1 → up to 2 attempts (0, 1)
    expect(mock).toHaveBeenCalledTimes(2);
  });

  // -- 4xx no-implicit-retry (5xx path) but still retried via catch ----------

  it("retries 4xx via the catch path because retries remain, final error is correct", async () => {
    const mock = vi.fn().mockResolvedValue(res(400, { error: "bad request" }));
    vi.stubGlobal("fetch", mock);

    await expect(fetchAPI("/test", { retry: 2 })).rejects.toThrow("bad request");
    // retry=2 → 3 total attempts
    expect(mock).toHaveBeenCalledTimes(3);
  });

  it("does not retry 4xx when retry is 0", async () => {
    const mock = vi.fn().mockResolvedValue(res(400, { error: "bad request" }));
    vi.stubGlobal("fetch", mock);

    await expect(fetchAPI("/test", { retry: 0 })).rejects.toThrow("bad request");
    expect(mock).toHaveBeenCalledTimes(1);
  });

  // -- retry on network (fetch rejection) ------------------------------------

  it("retries when fetch itself rejects (network error)", async () => {
    const mock = vi
      .fn()
      .mockRejectedValueOnce(new Error("ECONNREFUSED"))
      .mockResolvedValue(res(200, { recovered: true }));
    vi.stubGlobal("fetch", mock);

    const result = await fetchAPI<{ recovered: boolean }>("/test", { retry: 1 });
    expect(result).toEqual({ recovered: true });
    expect(mock).toHaveBeenCalledTimes(2);
  });

  it("throws the last error when all network retries are exhausted", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("ENOTFOUND")));

    await expect(fetchAPI("/test", { retry: 1 })).rejects.toThrow("ENOTFOUND");
  });

  // -- timeout (simulated via immediate AbortController) -------------------

  it("rejects when the request is aborted via timeout", async () => {
    // Simulate a timeout by making AbortController signal already-aborted.
    // fetch then rejects with an AbortError, which fetchAPI catches and re-throws.
    const origAC = globalThis.AbortController;
    vi.stubGlobal("AbortController", class {
      signal = {
        aborted: true,
        addEventListener: () => {},
        removeEventListener: () => {},
      } as unknown as AbortSignal;
      abort() {}
    });
    vi.stubGlobal(
      "fetch",
      vi.fn().mockRejectedValue(
        Object.assign(new Error("The operation was aborted"), { name: "AbortError" }),
      ),
    );

    try {
      await expect(
        fetchAPI("/test", { retry: 0 }),
      ).rejects.toThrow("The operation was aborted");
    } finally {
      vi.stubGlobal("AbortController", origAC);
    }
  });

  // -- zero retries ----------------------------------------------------------

  it("does not retry when retry is 0 and server returns 5xx", async () => {
    const mock = vi.fn().mockResolvedValue(res(500, { error: "srv" }));
    vi.stubGlobal("fetch", mock);

    await expect(fetchAPI("/test", { retry: 0 })).rejects.toThrow("srv");
    expect(mock).toHaveBeenCalledTimes(1);
  });

  // -- error body fallback ---------------------------------------------------

  it("falls back to a generic status message when the error body is empty", async () => {
    // json() rejects → .catch(() => ({})) → errBody is {} → fallback fires
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: false,
        status: 403,
        json: () => Promise.reject(new Error("Not JSON")),
      }),
    );

    await expect(fetchAPI("/test", { retry: 0 })).rejects.toThrow("API error: 403");
  });

  it("falls back to the .message field when .error is absent", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(res(500, { message: "something broke" })),
    );

    await expect(fetchAPI("/test", { retry: 0 })).rejects.toThrow("something broke");
  });
});
