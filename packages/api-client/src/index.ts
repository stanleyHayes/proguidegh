/**
 * @proguidegh/api-client — shared API client for the native mobile apps
 * (Phase M, M-04). Platform-agnostic: the app supplies a TokenStore backed
 * by expo-secure-store (ADR 0009); this client never touches storage,
 * cookies, or platform APIs directly.
 *
 * - Bearer transport: access token in the Authorization header; refresh via
 *   the X-Refresh-Token header (M-05), never cookies.
 * - On 401, rotates once via POST /auth/refresh (deduped across concurrent
 *   calls), persists the new token pair, and retries the original request.
 * - Rotation failure (expired/revoked/reuse) clears the session and calls
 *   onSessionExpired so the app can route to login.
 * - Errors are thrown as ApiError parsed from the standard envelope:
 *   { "error": { "code", "message", "details", "request_id" } }
 */

export interface TokenPair {
  accessToken: string;
  refreshToken: string;
}

/** Implemented by the app with expo-secure-store (ADR 0009). */
export interface TokenStore {
  getTokens(): Promise<TokenPair | null>;
  setTokens(tokens: TokenPair): Promise<void>;
  clear(): Promise<void>;
}

export interface ApiClientOptions {
  /** e.g. "http://localhost:8080" — no trailing slash required. */
  baseUrl: string;
  tokenStore: TokenStore;
  /** Called after the session is unrecoverably cleared (route to login). */
  onSessionExpired?: () => void;
}

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
  /** Send without the Authorization header (login/register/OTP endpoints). */
  anonymous?: boolean;
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

interface AuthTokenResponse {
  access_token?: string;
  refresh_token?: string;
}

const API_PREFIX = "/api/v1";

export interface ApiClient {
  api<T = unknown>(path: string, options?: RequestOptions): Promise<T>;
  /** Log in and persist the returned token pair. */
  login(credentials: {
    email?: string;
    phone?: string;
    password: string;
  }): Promise<{ mfaRequired: boolean; challenge?: string }>;
  /** Complete an MFA step-up login challenge. */
  loginMfa(challenge: string, code: string): Promise<void>;
  /** Revoke the session server-side and clear local tokens (ADR 0009 §4). */
  logout(): Promise<void>;
  /** True when a refresh token is present locally. */
  hasSession(): Promise<boolean>;
}

export function createApiClient(options: ApiClientOptions): ApiClient {
  const baseUrl = options.baseUrl.replace(/\/+$/, "");
  const { tokenStore, onSessionExpired } = options;

  let refreshPromise: Promise<boolean> | null = null;

  /** Rotate via X-Refresh-Token (M-05); dedupes concurrent refreshes. */
  function refreshSession(): Promise<boolean> {
    if (!refreshPromise) {
      refreshPromise = (async () => {
        const tokens = await tokenStore.getTokens();
        if (!tokens) return false;
        const res = await fetch(`${baseUrl}${API_PREFIX}/auth/refresh`, {
          method: "POST",
          headers: { "X-Refresh-Token": tokens.refreshToken },
        }).catch(() => null);
        if (!res || !res.ok) return false;
        const data = (await res.json().catch(() => null)) as
          | AuthTokenResponse
          | null;
        if (!data?.access_token || !data.refresh_token) return false;
        // ADR 0009 §3: replace BOTH tokens before any further calls.
        await tokenStore.setTokens({
          accessToken: data.access_token,
          refreshToken: data.refresh_token,
        });
        return true;
      })().finally(() => {
        refreshPromise = null;
      });
    }
    return refreshPromise;
  }

  async function expireSession(): Promise<void> {
    await tokenStore.clear().catch(() => undefined);
    onSessionExpired?.();
  }

  async function api<T = unknown>(
    path: string,
    requestOptions: RequestOptions = {},
  ): Promise<T> {
    const {
      method = "GET",
      body,
      headers,
      anonymous = false,
      skipRefreshRetry = false,
    } = requestOptions;

    const authHeaders: Record<string, string> = {};
    if (!anonymous) {
      const tokens = await tokenStore.getTokens();
      if (tokens) authHeaders.Authorization = `Bearer ${tokens.accessToken}`;
    }

    const res = await fetch(`${baseUrl}${API_PREFIX}${path}`, {
      method,
      headers: {
        ...(body !== undefined ? { "Content-Type": "application/json" } : {}),
        ...authHeaders,
        ...headers,
      },
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });

    if (res.status === 401 && !anonymous && !skipRefreshRetry) {
      const refreshed = await refreshSession();
      if (refreshed) {
        return api<T>(path, { ...requestOptions, skipRefreshRetry: true });
      }
      await expireSession();
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

  async function persistLogin(data: unknown): Promise<void> {
    const body = data as AuthTokenResponse | null;
    if (!body?.access_token || !body.refresh_token) {
      throw new ApiError(500, {
        code: "bad_login_response",
        message: "Login response did not include session tokens.",
      });
    }
    await tokenStore.setTokens({
      accessToken: body.access_token,
      refreshToken: body.refresh_token,
    });
  }

  return {
    api,

    async login(credentials) {
      const data = await api<Record<string, unknown> & AuthTokenResponse>(
        "/auth/login",
        {
          method: "POST",
          body: credentials,
          anonymous: true,
          skipRefreshRetry: true,
        },
      );
      if (data.mfa_required === true) {
        return {
          mfaRequired: true,
          challenge:
            typeof data.challenge === "string" ? data.challenge : undefined,
        };
      }
      await persistLogin(data);
      return { mfaRequired: false };
    },

    async loginMfa(challenge, code) {
      const data = await api<AuthTokenResponse>("/auth/login/mfa", {
        method: "POST",
        body: { challenge, code },
        anonymous: true,
        skipRefreshRetry: true,
      });
      await persistLogin(data);
    },

    async logout() {
      const tokens = await tokenStore.getTokens();
      if (tokens) {
        await fetch(`${baseUrl}${API_PREFIX}/auth/logout`, {
          method: "POST",
          headers: { "X-Refresh-Token": tokens.refreshToken },
        }).catch(() => undefined); // local logout proceeds regardless
      }
      await tokenStore.clear().catch(() => undefined);
    },

    async hasSession() {
      return (await tokenStore.getTokens()) !== null;
    },
  };
}

/**
 * Unwrap a response that may be either the resource itself or wrapped in a
 * single key (e.g. `{ "profile": {...} }` vs `{...}`).
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
