# ADR 0008: Immutable double-entry ledger

**Status:** Accepted
**Date:** 2026-08-13
**Spec reference:** §1.2, §8.1, §9, §30.4, §36

## Context

The platform collects tourist payments, accrues a 15% platform fee and 3% Tourism Levy (both configurable, effective-dated), owes guides payouts, processes refunds and must reconcile against provider settlements. Mutable balance columns on bookings/wallets drift silently under retries, partial failures and corrections; financial correctness is a first-class requirement (§9).

## Decision

Model all money movements as an immutable double-entry ledger in PostgreSQL:

- `ledger_accounts` (logical accounts per owner/currency), `ledger_transactions` (immutable header with reference/type/booking), `ledger_entries` (balanced debit/credit lines) (§8.1).
- Every ledger transaction balances; posted entries are never updated or deleted.
- Corrections (refunds, disputes, adjustments) are reversal/adjustment transactions; originals are preserved (§4.5, §9.2).
- Provider reference uniqueness prevents duplicate postings from webhook replay (§9.2).
- Wallet balances and statements are derived from the ledger, never from mutable booking/payment records (§1.2, §13.4).
- Commission and levy rates are effective-dated financial rules in configuration, not code constants (§4.5, §27).
- Amounts are integer minor units or `NUMERIC` with explicit scale; never floating point (§9).

## Consequences

- Balanced-entry, immutability and provider-reference-uniqueness tests are mandatory before any payment task is considered done (§31.26).
- Every financial feature must name its account movements; "just update a balance column" is not an acceptable implementation.
- Reconciliation reports are projections over ledger data, so finance totals reconcile to provider settlement inputs by construction (§30.4).
- Storage grows monotonically; acceptable for V1 volumes and required by audit.
