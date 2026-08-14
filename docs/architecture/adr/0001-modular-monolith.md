# ADR 0001: Modular monolith for V1

**Status:** Accepted
**Date:** 2026-08-13
**Spec reference:** §1.2, §6, §7.2

## Context

ProGuideGH must ship to production in 90 days with a small agent-driven team. The domain (bookings, payments, dispatch, certification, reviews, payouts) is highly interconnected; premature distribution would add network failure modes, distributed transactions and deployment complexity without a measured scaling need. The 5,000-guide Y1 target does not by itself require independently deployed services (Spec §23.4).

## Decision

Build V1 as a single deployable Go modular monolith (`services/api`, plus a `services/worker` process sharing the same internal modules). Enforce strong internal domain boundaries: one package per domain (`internal/bookings`, `internal/payments`, …), platform concerns isolated in `internal/platform/`, and no cross-module writes to another module's tables except through its service interface.

No Kubernetes and no independently deployed domain microservices. Extraction of a module into a separate service requires a later ADR justified by measured scale.

## Consequences

- One build, one deployment pipeline, one schema — fast iteration and simple rollback.
- Transactions span domain boundaries in-process, which the ledger and state-machine invariants depend on.
- Module boundaries and explicit service interfaces keep later extraction possible but not free; cross-module coupling through shared tables is forbidden and must be caught in review.
- Scaling is vertical/horizontal instance scaling on Render from measured bottlenecks only.
