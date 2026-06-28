export const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8443";
const DEFAULT_ADMIN_KEY = process.env.NEXT_PUBLIC_ADMIN_KEY || "";
const DEFAULT_TIMEOUT_MS = 5_000;

type APIOptions = RequestInit & {
  timeoutMs?: number;
  retry?: number;
};

export function authHeaders(adminKey?: string): HeadersInit {
  const key = adminKey || DEFAULT_ADMIN_KEY;
  return key ? { "X-Tamga-Admin-Key": key } : {};
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
        const message = formatFetchErrorMessage(res.status, errBody);
        const err = new Error(res.status === 401 ? "Admin key yanlış veya eksik" : message);
        if (res.status >= 500 && attempt < retries) {
          lastError = err;
          continue;
        }
        throw err;
      }

      return res.json();
    } catch (error) {
      lastError = error instanceof Error ? error : new Error("API request failed");
      if (attempt >= retries) {
        break;
      }
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
      return parts.join(" — ");
    }
  }
  if (typeof errBody.message === "string") {
    return errBody.message;
  }
  return fallback;
}
