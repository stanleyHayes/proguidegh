/**
 * Minimal typed client for the ProGuideGH API (v1).
 *
 * Duplicated per app on purpose: apps must not share code outside packages/.
 * Keep this file tiny and identical across apps.
 *
 * - Base URL from NEXT_PUBLIC_API_URL (default http://localhost:8080).
 * - Sessions are HttpOnly cookies, so every request sends credentials.
 * - On 401, transparently retries once after POST /auth/refresh.
 * - Errors are thrown as ApiError parsed from the standard envelope:
 *   { "error": { "code", "message", "details", "request_id" } }
 */

export const API_BASE_URL = (
  process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080"
).replace(/\/+$/, "");

const API_PREFIX = "/api/v1";

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details?: unknown;
  readonly requestId?: string;

  constructor(
    status: number,
    body: {
      code: string;
      message: string;
      details?: unknown;
      requestId?: string;
    },
  ) {
    super(body.message);
    this.name = "ApiError";
    this.status = status;
    this.code = body.code;
    this.details = body.details;
    this.requestId = body.requestId;
  }
}

export interface RequestOptions {
  method?: "GET" | "POST" | "PATCH" | "PUT" | "DELETE";
  body?: unknown;
  /** Extra request headers (e.g. Idempotency-Key for replay-safe mutations). */
  headers?: Record<string, string>;
  /** Skip the one-time refresh retry (used by auth endpoints themselves). */
  skipRefreshRetry?: boolean;
}

interface ErrorEnvelope {
  error?: {
    code?: string;
    message?: string;
    details?: unknown;
    request_id?: string;
    requestId?: string;
  };
}

let refreshPromise: Promise<boolean> | null = null;

/** Rotate the session via the refresh cookie; dedupes concurrent calls. */
function refreshSession(): Promise<boolean> {
  if (!refreshPromise) {
    refreshPromise = fetch(`${API_BASE_URL}${API_PREFIX}/auth/refresh`, {
      method: "POST",
      credentials: "include",
    })
      .then((res) => res.ok)
      .catch(() => false)
      .finally(() => {
        refreshPromise = null;
      });
  }
  return refreshPromise;
}

export async function api<T = unknown>(
  path: string,
  options: RequestOptions = {},
): Promise<T> {
  const { method = "GET", body, skipRefreshRetry = false, headers } = options;

  const res = await fetch(`${API_BASE_URL}${API_PREFIX}${path}`, {
    method,
    credentials: "include",
    headers:
      body !== undefined || headers
        ? {
            ...(body !== undefined
              ? { "Content-Type": "application/json" }
              : {}),
            ...headers,
          }
        : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });

  if (res.status === 401 && !skipRefreshRetry) {
    const refreshed = await refreshSession();
    if (refreshed) {
      return api<T>(path, { ...options, skipRefreshRetry: true });
    }
  }

  if (res.status === 204) {
    return undefined as T;
  }

  const data: unknown = await res.json().catch(() => null);

  if (!res.ok) {
    const envelope = (data as ErrorEnvelope | null)?.error;
    throw new ApiError(res.status, {
      code: envelope?.code ?? "unknown_error",
      message:
        envelope?.message ?? `Request failed with status ${res.status}.`,
      details: envelope?.details,
      requestId: envelope?.request_id ?? envelope?.requestId,
    });
  }

  return data as T;
}

/**
 * Unwrap a response that may be either the resource itself or wrapped in a
 * single key (e.g. `{ "profile": {...} }` vs `{...}`) — the API shape is
 * being built concurrently, so callers stay tolerant.
 */
export function unwrap<T>(data: unknown, key: string): T {
  if (data !== null && typeof data === "object" && key in data) {
    return (data as Record<string, unknown>)[key] as T;
  }
  return data as T;
}

/** User-friendly message for any caught error. */
export function errorMessage(err: unknown, fallback: string): string {
  if (err instanceof ApiError) return err.message;
  return fallback;
}
