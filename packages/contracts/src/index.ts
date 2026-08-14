/**
 * ProGuideGH API contracts (Phase 0 placeholders).
 *
 * These types are hand-written stand-ins for the API health endpoints so the
 * frontends can compile before the Go API's OpenAPI spec exists. Once
 * docs/api/openapi.yaml lands, run `pnpm --filter @proguidegh/contracts generate`
 * (see scripts/generate.sh) and replace these placeholders with the generated
 * `paths`/`components` types from `src/generated.ts`.
 */

/** GET /healthz — liveness probe. */
export interface HealthResponse {
  status: "ok";
  service: string;
  version: string;
  time: string; // RFC 3339 timestamp
}

/** GET /readyz — readiness probe with dependency checks. */
export interface ReadinessResponse {
  status: "ready" | "degraded";
  checks: ReadinessCheck[];
}

export interface ReadinessCheck {
  name: "postgres" | "redis" | (string & {});
  status: "up" | "down";
  latencyMs?: number;
}

/** Standard error envelope returned by the API. */
export interface ErrorResponse {
  error: {
    code: string;
    message: string;
    requestId?: string;
  };
}
