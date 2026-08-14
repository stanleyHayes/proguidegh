# CLAUDE.md — ProGuideGH

**Canonical rules live in [`AGENTS.md`](./AGENTS.md).** This file is the required mirror for Claude-based tooling (company AI Governance). If the two ever diverge, AGENTS.md wins — fix this file to match. Summary below is for orientation, not authority.

## What this is

ProGuideGH: certified-tourist-guide marketplace for Ghana (Accra, Cape Coast, Kumasi; 90-day V1). Go modular monolith API + worker on Render, three Next.js 16/TS apps on Vercel, PostgreSQL (source of truth), Redis (ephemeral only), R2 (private signed-URL storage), Paystack via payment adapter.

## The ten non-negotiables (Spec §1.2)

1. Modular monolith; no K8s/microservices in V1.
2. PostgreSQL is the source of truth.
3. MongoDB optional/secondary only; core works without it.
4. Redis is ephemeral state only, never permanent truth.
5. Immutable double-entry ledger for all money; balances derived, never mutated.
6. No raw card data — provider-hosted/tokenized flows via the payment adapter (Paystack first).
7. Sensitive documents in private R2 behind short-lived signed URLs.
8. Every privileged/financial action audited.
9. Every retried mutation idempotent.
10. Phased build; each phase deployable and testable.

## Coding standards (condensed)

- Go: explicit SQL, no generic repository abstraction; state machines centralized in one domain service each; integer minor units/`NUMERIC` for money, never float.
- TS: `strict`; consume generated OpenAPI contracts from `packages/contracts`, never hand-maintained duplicates.
- Vertical slices: migration → repository → service → handler/OpenAPI → frontend → tests → observability → docs.
- Required tests: migrations (compat/indexes), financial (idempotency + ledger invariants), privileged (permission + audit), realtime (disconnect/reconnect/expiry).

## Conventions

- Branch `feature/PG-<n>-<name>` · Commit `PG-<n> <message>` · PR `PG-<n> <Title>`.
- AI roles: Claude = planning/docs · Kimi = research/analysis · Codex = code · Human = credentials/approvals.
- Definition of done: code + tests green in CI + docs updated + `docs/implementation-status.md` updated **with evidence** (commands + results).
- Security: no secrets committed, no raw card data, audit privileged actions, idempotency on retried mutations, server-authoritative pricing, backend-enforced RBAC.
- Appendix D stop conditions (see AGENTS.md §8) block launch — flag immediately if touched.

## Key references

- Build spec: `../../extracted/spec.md` · Task board: `../../agent_plan.md`
- Status/evidence: `docs/implementation-status.md` · ADRs: `docs/architecture/adr/`
