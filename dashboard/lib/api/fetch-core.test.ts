import { describe, expect, it, vi, beforeEach } from "vitest";
import { ApiError, authHeaders, fetchAPI, permissionDenied, sessionRefused } from "@/lib/api/fetch-core";

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
      "Admin key is wrong or missing",
    );
  });

  it("keeps the proxy's sentence for a 401 that is an ended session, not a wrong key", async () => {
    const body = { error: "this session has ended; sign in again", code: "session_revoked" };
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(res(401, body)));

    const error = await fetchAPI("/test", { retry: 0 }).catch((e: unknown) => e);
    expect(error).toMatchObject({ status: 401, code: "session_revoked", message: "this session has ended; sign in again" });
    expect(sessionRefused(error)?.title).toBe("This session has ended.");
    expect(permissionDenied(error)).toBeNull();
  });

  it("reads an account without a role apart from a role that is too low", async () => {
    const body = { error: "this account has not been given access", code: "account_not_assigned", user_id: "583231" };
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(res(403, body)));

    const error = await fetchAPI("/test", { retry: 0 }).catch((e: unknown) => e);
    expect(sessionRefused(error)).toMatchObject({
      title: "This account has not been given a role.",
      detail: "An admin can assign one under Team: 583231.",
    });
    expect(permissionDenied(error)).toBeNull();
    // A wrong key is neither.
    expect(sessionRefused(new ApiError("Admin key is wrong or missing", 401, { error: "unauthorized" }))).toBeNull();
  });

  it("does not retry a 401: the same key gives the same answer", async () => {
    const mock = vi.fn().mockResolvedValue(res(401, { error: "unauthorized" }));
    vi.stubGlobal("fetch", mock);

    await expect(fetchAPI("/test", { retry: 2 })).rejects.toThrow(
      "Admin key is wrong or missing",
    );
    expect(mock).toHaveBeenCalledTimes(1);
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

  // -- 4xx is never retried ---------------------------------------------------

  it("does not retry 4xx even when retries remain", async () => {
    const mock = vi.fn().mockResolvedValue(res(400, { error: "bad request" }));
    vi.stubGlobal("fetch", mock);

    await expect(fetchAPI("/test", { retry: 2 })).rejects.toThrow("bad request");
    expect(mock).toHaveBeenCalledTimes(1);
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

  // -- the structured error --------------------------------------------------

  it("keeps the status, the code and the body of a refusal beside its message", async () => {
    const body = {
      error: "this change needs the admin role: it touches providers",
      code: "admin_role_required",
      privileged_sections: ["providers", "rules.pii_detection.notify"],
    };
    const mock = vi.fn().mockResolvedValue(res(403, body));
    vi.stubGlobal("fetch", mock);

    const error = await fetchAPI("/test", { retry: 2 }).catch((e: unknown) => e);
    expect(error).toBeInstanceOf(ApiError);
    expect(error).toBeInstanceOf(Error);
    const refused = error as ApiError;
    expect(refused.message).toBe("this change needs the admin role: it touches providers");
    expect(refused.status).toBe(403);
    expect(refused.code).toBe("admin_role_required");
    expect(refused.body.privileged_sections).toEqual(["providers", "rules.pii_detection.notify"]);
    // Still a 4xx: asked once.
    expect(mock).toHaveBeenCalledTimes(1);
  });

  it("reads the code of a 422 from inside the error object and keeps its details", async () => {
    const details = [{ field: "rate_limit.requests_per_minute", rule: "min", message: "must not be negative", severity: "error" }];
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(res(422, { error: { code: "validation_failed", message: "Policy validation failed", details } })),
    );

    const error = (await fetchAPI("/test", { retry: 0 }).catch((e: unknown) => e)) as ApiError;
    expect(error.code).toBe("validation_failed");
    expect(error.status).toBe(422);
    expect((error.body.error as { details: unknown }).details).toEqual(details);
    // The flattened sentence older callers show is unchanged.
    expect(error.message).toBe("Policy validation failed: rate_limit.requests_per_minute: must not be negative");
  });

  it("carries the status of a 5xx too, with an empty code when the proxy sent none", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(res(500, { error: "srv" })));

    const error = (await fetchAPI("/test", { retry: 0 }).catch((e: unknown) => e)) as ApiError;
    expect(error).toBeInstanceOf(ApiError);
    expect(error.status).toBe(500);
    expect(error.code).toBe("");
  });

  it("leaves a network failure a plain Error", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("ENOTFOUND")));

    const error = await fetchAPI("/test", { retry: 0 }).catch((e: unknown) => e);
    expect(error).not.toBeInstanceOf(ApiError);
  });
});
