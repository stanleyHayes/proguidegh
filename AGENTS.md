# AGENTS.md — ProGuideGH

Canonical operating rules for AI agents (and humans) working in this repository. `CLAUDE.md` mirrors this file; if they ever diverge, **this file wins**, then the Build Specification, then `../agent_plan.md`.

**Authoritative sources:** Build Specification v1.0 (`../../extracted/spec.md`), master task board (`../../agent_plan.md`), status tracker (`docs/implementation-status.md`), ADRs (`docs/architecture/adr/`).

## 1. Project overview

ProGuideGH is a certified-tourist-guide marketplace for Ghana (launch: Accra, Cape Coast, Kumasi; 90-day V1). Tourists discover/book/pay/track certified guides; guides apply, get certified, accept dispatched jobs and receive MoMo payouts; admins get operational, financial, quality and safety oversight.

- **Stack:** Go API + worker (Render), three Next.js 16 + TypeScript apps (Vercel), PostgreSQL (source of truth), Redis/Upstash (ephemeral), Cloudflare R2 (private files), Paystack (payments adapter), Resend (email), FCM (push), Google Maps, Sentry + OpenTelemetry.
- **Repo layout (Spec §7.1):**
  - `apps/tourist-web`, `apps/guide-web`, `apps/admin-web` — Next.js frontends
  - `services/api` — Go HTTP/WebSocket API (`cmd/api`, `internal/{platform,<domain>}`, `migrations/`, `openapi/`)
  - `services/worker` — Go jobs/schedulers
  - `packages/ui`, `packages/contracts`, `packages/config` — shared frontend packages
  - `infra/` — render/vercel/cloudflare config; `scripts/`
  - `docs/architecture/adr/`, `docs/runbooks/`, `docs/api/`, `docs/implementation-status.md`

## 2. Non-negotiable engineering decisions (Spec §1.2)

1. Modular monolith for V1. No Kubernetes, no independently deployed microservices (ADR 0001).
2. PostgreSQL is the source of truth for all durable and financial state (ADR 0002).
3. MongoDB is optional/secondary — semi-structured payloads only; core works without it (ADR 0004).
4. Redis holds ephemeral/high-frequency state only: presence, live location, locks, rate limits, offer TTLs, cache (ADR 0003).
5. Immutable double-entry ledger for all money movement; balances are derived, never mutated (ADR 0008).
6. No raw card data. Card/MoMo collection via provider-hosted/tokenized flows through the payment adapter, Paystack first (ADR 0005).
7. Sensitive documents in private R2, short-lived signed URLs only (ADR 0006).
8. Every privileged or financially significant action is audited.
9. Every retried mutation (mobile clients, webhooks) is idempotent.
10. Build in phases; each phase stays deployable and testable.

## 3. Coding standards

### Go (`services/*`)
- Explicit SQL queries and transactions. **No generic repository abstraction** that hides SQL semantics; keep persistence behind module-local interfaces (Spec §7.2).
- State machines (booking §8.2, payment §8.3, payout §8.4, certification §5) are explicit and enforced in a single domain service each. No arbitrary status writes from controllers or admin forms.
- Money: integer minor units or `NUMERIC` with explicit scale. Never float.
- Structured JSON logs with `request_id` and correlation fields; never log secrets, auth headers, documents or payment tokens (§22.1).
- Every job handler idempotent; Redis Pub/Sub is not a durable queue (§21).

### TypeScript (`apps/*`, `packages/*`)
- `strict` mode. No `any` without a justification comment.
- Frontends consume **generated** contracts from the committed OpenAPI spec (`packages/contracts`); never hand-maintain duplicate API types (§13).
- Shared design tokens/components from `packages/ui`; explicit loading/empty/offline/retry/error states; map never the sole representation of operational state (§18.4).

### Vertical slices (Spec §31.22)
Every feature: migration → repository → domain/service → handler/OpenAPI → frontend → tests → observability → documentation.

### Required tests per task type (Spec §31.24–27)
- DB migration: backward-compatibility + index check for new query paths.
- Payment/financial task: replay/idempotency tests + ledger-invariant tests.
- Privileged/admin task: permission + audit tests.
- Realtime task: disconnect/reconnect/expiry tests.

## 4. Branch, commit, PR conventions (Operations Manual Phase 5)

- Branch: `feature/PG-<n>-<name>` (e.g. `feature/PG-12-booking-state-machine`)
- Commit: `PG-<n> <imperative message>`
- PR: `PG-<n> <Title>` — must reference the Jira issue and link updated status evidence.
- One concern per branch; no unrelated changes (AI Governance).

## 5. AI role split (Operations Manual / Training Manual)

| Agent | Role |
|---|---|
| **Claude** | Planning, documentation, ADRs, implementation-status tracking, review |
| **Kimi** | Research, analysis, spec interpretation, gap identification |
| **Codex** | Code generation (Go services, Next.js apps, migrations, tests) |
| **Human** | Credentials, provider accounts, approvals, launch sign-off |

Blocked on an external credential? Build against sandbox/mock adapters, document the required secret, keep production enablement behind config — do not stop unrelated work (Spec §31.29).

## 6. Definition of done

A task is done when **all** hold: code + migrations merged via PR; required tests (per §3) pass in CI; docs updated (OpenAPI regenerated, runbooks/ADRs where relevant); `docs/implementation-status.md` updated with evidence (commands + results) per its update rules; no stop condition introduced.

Phase-end gate (Spec §31.28): formatting, lint, unit + integration tests, frontend typecheck/build, relevant E2E — results recorded in the phase evidence log.

## 7. Security rules (Spec §15, §22)

- Never commit secrets. `.env.example` carries names and safe descriptions only; real secrets live in platform secret settings.
- Never store or log raw card/MoMo data; payout identifiers tokenized.
- Private buckets + signed URLs; object keys contain no raw personal data.
- Audit every privileged/financial mutation (actor, before/after, IP).
- Idempotency-Key on booking creation, payment initiation, refunds and any replay-sensitive mutation.
- Webhook signature verification mandatory; archive raw payload hash as policy permits.
- Server-authoritative pricing; never trust client-supplied totals.
- RBAC enforced in backend handlers on every request; frontend guards are convenience only.
- MFA for Super Admin/finance; step-up auth for role, payout-account and refund actions.
- Rate-limit login, OTP, reset, payment initiation and SOS abuse vectors.

## 8. Agent stop conditions (Spec Appendix D — launch blockers)

These block **production launch**, not unrelated development. If your work touches one, flag it immediately in the PR and the status file:

1. No verified production payment webhook/signature setup.
2. Ledger invariant tests failing.
3. Critical auth/RBAC bypass.
4. SOS event cannot reach operations dashboard.
5. No database backup/restore procedure.
6. Admin privileged accounts lack required MFA.
7. Guide "verified/insured" badge displayable without valid evidence/status.
8. Critical personal documents publicly accessible.
9. Duplicate payout possible under retries/concurrency.
10. Production secrets committed to the repository.

## 9. Working with the status tracker

Before marking anything done in `docs/implementation-status.md`, read its update rules: a checkbox requires evidence (commands + results) appended to the phase's evidence log. Phases are sequential (Spec §28).
