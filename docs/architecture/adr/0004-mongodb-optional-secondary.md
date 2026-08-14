# ADR 0004: MongoDB optional and secondary

**Status:** Accepted
**Date:** 2026-08-13
**Spec reference:** §1.2, §6.1, §35

## Context

Some data is genuinely semi-structured: learning-content bodies, dynamic form payloads, integration payload archives. None of it is on the transactional or financial path. Provisioning a second database platform "just in case" adds cost, backup surface and operational skill requirements.

## Decision

MongoDB (Atlas) is optional and secondary:

- Use it only for clearly semi-structured data: learning content, dynamic form payloads, integration payload archives.
- The core system must work without MongoDB. No user, booking, payment, ledger, certification or audit state may live there.
- Do not provision Atlas until a concrete document-data need exists (§35); until then the corresponding features store structured payloads in PostgreSQL (`jsonb`) or private object storage.

## Consequences

- V1 ships with one database to back up, secure and reason about.
- Any future MongoDB adoption is scoped, justified per use case, and recorded in a follow-up ADR.
- Code paths that might later move to MongoDB keep payloads behind a narrow module interface so the swap is local.
