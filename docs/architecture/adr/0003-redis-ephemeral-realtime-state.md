# ADR 0003: Redis for ephemeral and realtime state

**Status:** Accepted
**Date:** 2026-08-13
**Spec reference:** §1.2, §6.1, §10.3, §11, §35

## Context

Dispatch offers expire in 20–45 seconds, guide locations update at high frequency, WebSocket presence is volatile, and rate limits/locks/idempotency caches are short-lived. Storing these in PostgreSQL would add write load and retention burden for data that is worthless once stale.

## Decision

Use Redis (Upstash) exclusively for ephemeral/high-frequency state:

- online/presence status and current guide location (TTL-bounded, §11.2),
- WebSocket presence bookkeeping,
- short-lived dispatch offers with TTL expiry (§10.3),
- distributed locks (offer acceptance, payout batching),
- rate limits,
- idempotency response cache (durable idempotency records still live in Postgres `idempotency_keys`),
- cached queries.

Rules:

- Redis is never permanent truth (§35). Losing a Redis key must never lose money, bookings, or audit history; anything safety/audit-relevant is persisted to PostgreSQL as coarse checkpoints/events per retention policy (§11.2).
- Redis Pub/Sub alone is not a durable job queue; durable work uses an explicit jobs table or reliable queue (§21).

## Consequences

- High-frequency location and offer traffic stays off the primary database.
- Offer expiry semantics are cheap and exact via TTL.
- Every Redis consumer must tolerate cold start/miss by design; cache-aside with Postgres fallback is the default pattern.
- Upstash access controls are a launch-checklist item (§33).
