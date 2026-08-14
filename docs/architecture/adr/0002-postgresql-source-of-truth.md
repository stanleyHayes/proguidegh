# ADR 0002: PostgreSQL as the source of truth

**Status:** Accepted
**Date:** 2026-08-13
**Spec reference:** §1.2, §6.1, §8, §35

## Context

The system holds financial state (payments, payouts, ledger, commissions, levies), identity, certification workflow and audit records. These demand ACID transactions, constraints, and durable history. Realtime state (presence, live coordinates) and semi-structured payloads have different characteristics and are handled separately (ADR 0003, ADR 0004).

## Decision

PostgreSQL (managed, Render) is the single source of truth for users, guides, bookings, tours, reviews, payments, payouts, certification, commissions, tourism levies, audit references and all financial state.

Conventions from Spec §8:

- UUID primary keys (unless a provider natural key is explicitly required).
- `timestamptz`; `created_at`/`updated_at` on mutable entities.
- Financial and audit records are append-only; soft-deletion only where legal/audit retention requires it.
- Money as integer minor units where provider/currency semantics permit, else `NUMERIC` with explicit scale — never floating point (§9).
- Explicit SQL queries and transactions; no generic repository abstraction hiding SQL semantics (§7.2).
- Double-booking and overlap prevention via constraints/transactional checks in Postgres, not application-level best-effort (§10.2).
- Versioned migrations (golang-migrate or goose), backwards-compatible for rolling deploys, never auto-run destructively on production startup (§24.3, §26).

## Consequences

- Invariants (balanced ledger, single assignment, overlap prevention) can be enforced where they are strongest: in the database transaction.
- All reporting/reconciliation reads derive from one consistent store.
- PostgreSQL operational competence, backup/restore drills and migration discipline become mandatory team capabilities.
- Anything written to Redis or MongoDB must be reconstructible from, or disposable relative to, PostgreSQL.
