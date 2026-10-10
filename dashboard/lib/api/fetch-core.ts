import { BUILT_IN_ADMIN_KEY } from "@/lib/built-in-key";

export const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8443";
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

/** The credential every call to the management API carries: in a header, never in a URL. */
export function authHeaders(adminKey?: string): HeadersInit {
  // The built-in key is empty in a production build: see lib/built-in-key.ts.
  const key = adminKey || BUILT_IN_ADMIN_KEY;
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

/**
 * Whether the proxy refused a call because the key's role is too low (403,
 * code "permission_denied"), and what it asked for. The key itself is valid.
 * `needs` is a sentence for the person; `refused` one to put beside a button.
 */
export function permissionDenied(error: unknown): { permission: string; needs: string; refused: string } | null {
  if (!(error instanceof ApiError) || error.code !== "permission_denied") return null;
  const required = error.body.required_permission;
  const permission = typeof required === "string" ? required : "";
  const needs = permission ? `The proxy asks for the ${permission} permission.` : "The proxy asks for a higher role.";
  return { permission, needs, refused: `This key's role cannot do this. ${needs}` };
}

/**
 * Whether the proxy refused a session token that is well formed and signed:
 * the account has no role (403, "account_not_assigned"), or its sessions were
 * ended (401, "session_revoked"). Neither is a wrong key. The console sends a
 * key and gets neither today; this is for a caller that signs in.
 */
export function sessionRefused(error: unknown): { title: string; detail: string; refused: string } | null {
  if (!(error instanceof ApiError)) return null;
  if (error.status === 403 && error.code === "account_not_assigned") {
    const id = typeof error.body.user_id === "string" ? error.body.user_id : "";
    const title = "This account has not been given a role.";
    const detail = id ? `An admin can assign one under Team: ${id}.` : "An admin can assign one under Team.";
    return { title, detail, refused: `${title} ${detail}` };
  }
  if (error.status === 401 && error.code === "session_revoked") {
    const title = "This session has ended.";
    const detail = "Its role changed, or the account was signed out. Sign in again.";
    return { title, detail, refused: `${title} ${detail}` };
  }
  return null;
}

/** The sentence for an answer outside 2xx. A 401 is a wrong key unless the proxy says a session ended. */
function messageFor(status: number, body: Record<string, unknown>): string {
  if (status === 401 && body.code !== "session_revoked") return "Admin key is wrong or missing";
  return formatFetchErrorMessage(status, body);
}

/**
 * The error for an answer outside 2xx, read from the response. For the calls
 * that do not go through fetchAPI because their answer is not JSON.
 */
export async function errorFromResponse(res: Response): Promise<ApiError> {
  const body = (await res.json().catch(() => ({}))) as Record<string, unknown>;
  const message = messageFor(res.status, body);
  return new ApiError(message, res.status, body && typeof body === "object" ? body : {});
}

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
        const message = messageFor(res.status, errBody);
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
