# Security review — Phase 9 (P9-01)

Date: 2026-08-13 · Reviewer: automated build agent (Kimi) · Scope: API
(`services/api`), web apps (`apps/*-web`), mobile apps (`apps/*-mobile`),
infra (`infra/`)

## Scan results

| Scan | Tool | Result |
|---|---|---|
| Go dependency vulns | `govulncheck ./...` (x/vuln v1.7.0) | **0 affecting vulnerabilities** (1 vuln in a required module is unreachable from our call graph) |
| JS dependency vulns | `pnpm audit --prod` | 3 findings, all inside Expo **build tooling** of `apps/guide-mobile`: `uuid` <11.1.1 (moderate, bounds check) via `@expo/config-plugins>xcode`; `image-size` DoS ×2 (high) via `@expo/metro>metro`. None ship in the mobile runtime bundle or touch the API/web apps. Mitigation: bump when Expo SDK 57 line patches land; build tooling runs in CI/dev only |
| Static checks | `gofmt`, `go vet`, ESLint, `tsc` | clean across all packages (gate runs in CI) |
| Container scans | — | deferred: no production images exist yet (P9-04, Human); compose runs stock `postgres:16-alpine` + `redis:7-alpine` locally |

## Findings fixed in this review

1. **Missing security response headers.** The API set none. Added
   `httpx.SecurityHeadersMiddleware` (nosniff, frame DENY, no-referrer,
   deny-all CSP) on the root middleware chain with unit tests
   (`internal/platform/httpx/security.go`). HSTS intentionally left to the
   TLS terminator (Cloudflare).
2. **No CORS policy.** The web apps call the API cross-origin with
   session cookies, yet no CORS middleware existed (dev worked only via
   same-host setups). Added `httpx.CORSMiddleware` driven by
   `CORS_ALLOWED_ORIGINS` (default: the three local Next.js ports);
   reflects only allowlisted origins, allows credentials, answers
   preflight, never uses `*` with credentials.

## Posture summary (verified in code this session)

- **AuthN**: argon2id password hashing; opaque access tokens + rotating
  refresh sessions with reuse detection; optional TOTP MFA; sessions are
  HttpOnly cookies or Bearer tokens.
- **AuthZ**: RBAC with permission codes enforced per-route
  (`rbac.RequirePermission`); staff roles are least-privilege per spec §3;
  direct-SQL grants bypass the cache so tests flush `rbac:perms:*`.
- **Money**: integer minor units in Go; NUMERIC in Postgres; immutable
  double-entry ledger (UNIQUE references make postings idempotent);
  payout batch idempotency via partial unique index; payout destination
  refs AES-256-GCM encrypted at rest (decrypted only in the audited CSV
  export).
- **Webhooks**: provider-signature verification (Paystack HMAC; mock
  secret documented as dev-only).
- **Files**: signed short-lived upload/download URLs; the local adapter
  validates signatures in-handler — never public.
- **Rate limiting**: Redis-backed limiter on auth, SOS and other abusive
  surfaces.
- **Audit**: append-only `audit_logs` for privileged/financial actions
  with actor, before/after and IP.
- **Secrets**: none in the repo; config via env with safe local defaults
  only (`.env.example` documents production requirements).

## Remaining risks (accepted / human-owned)

- Live payment + payout transfer credentials (EXT-1/EXT-2, Human) — mock
  adapters and the audited manual export are the sanctioned fallbacks.
- Production WAF/TLS/rate-limit edge policy lands with P9-04 (Human).
- `image-size`/`uuid` advisories in Expo build deps — track and bump on
  the next Expo SDK patch cycle.
