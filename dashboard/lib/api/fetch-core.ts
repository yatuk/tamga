export const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8443";
const DEFAULT_ADMIN_KEY = process.env.NEXT_PUBLIC_ADMIN_KEY || "";
const DEFAULT_TIMEOUT_MS = 5_000;

type APIOptions = RequestInit & {
  timeoutMs?: number;
  retry?: number;
  /**
   * A status outside 2xx whose body is still the answer, not an error. The
   * health check uses it: a proxy that reports a part down answers 503.
   */
  accept?: (status: number, body: Record<string, unknown>) => boolean;
};

export function authHeaders(adminKey?: string): HeadersInit {
  const key = adminKey || DEFAULT_ADMIN_KEY;
  return key ? { "X-Tamga-Admin-Key": key } : {};
}

/**
 * An answer outside 2xx. `message` is the sentence the proxy sent, as before;
 * `code` and `body` keep what the proxy said beside it (which sections were
 * refused, which fields broke a limit), for a caller that can say more.
 */
export class ApiError extends Error {
  readonly status: number;
  /** The proxy's machine-readable reason, e.g. "dual_control_required". Empty when it sent none. */
  readonly code: string;
  readonly body: Record<string, unknown>;

  constructor(message: string, status: number, body: Record<string, unknown>) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.body = body;
    // The code sits at the top of the body, or inside `error` when that is an object (422).
    const nested = body.error && typeof body.error === "object" ? (body.error as { code?: unknown }).code : undefined;
    const code = typeof body.code === "string" ? body.code : nested;
    this.code = typeof code === "string" ? code : "";
  }
}

/** A response the server answered with: retrying a 4xx gives the same answer. */
class ClientError extends ApiError {}

export async function fetchAPI<T>(path: string, options?: APIOptions): Promise<T> {
  const retries = options?.retry ?? 1;
  const timeoutMs = options?.timeoutMs ?? DEFAULT_TIMEOUT_MS;
  let lastError: Error | null = null;

  for (let attempt = 0; attempt <= retries; attempt++) {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), timeoutMs);
    try {
      const res = await fetch(`${API_BASE}${path}`, {
        ...options,
        signal: controller.signal,
        headers: {
          "Content-Type": "application/json",
          ...options?.headers,
        },
      });

      if (!res.ok) {
        const errBody = (await res.json().catch(() => ({}))) as Record<string, unknown>;
        if (options?.accept?.(res.status, errBody)) return errBody as T;
        const message = res.status === 401 ? "Admin key is wrong or missing" : formatFetchErrorMessage(res.status, errBody);
        // Only a server fault (5xx) is worth asking again. A wrong key or a
        // bad request fails the same way every time.
        if (res.status < 500) throw new ClientError(message, res.status, errBody);
        throw new ApiError(message, res.status, errBody);
      }

      return res.json();
    } catch (error) {
      lastError = error instanceof Error ? error : new Error("API request failed");
      if (error instanceof ClientError) break;
    } finally {
      clearTimeout(timeout);
    }
  }

  throw lastError || new Error("API request failed");
}

function formatFetchErrorMessage(status: number, errBody: Record<string, unknown>): string {
  const fallback = `API error: ${status}`;
  const errField = errBody.error;
  if (typeof errField === "string") {
    return errField;
  }
  if (errField && typeof errField === "object") {
    const e = errField as {
      message?: string;
      details?: Array<{ field?: string; message?: string }>;
    };
    const parts: string[] = [];
    if (e.message) {
      parts.push(e.message);
    }
    if (Array.isArray(e.details) && e.details.length) {
      const detailStr = e.details
        .map((d) => [d.field, d.message].filter(Boolean).join(": "))
        .filter(Boolean)
        .join("; ");
      if (detailStr) {
        parts.push(detailStr);
      }
    }
    if (parts.length) {
      return parts.join(": ");
    }
  }
  if (typeof errBody.message === "string") {
    return errBody.message;
  }
  return fallback;
}
