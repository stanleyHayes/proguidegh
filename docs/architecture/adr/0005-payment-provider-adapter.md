# ADR 0005: Payment provider adapter, Paystack first

**Status:** Accepted
**Date:** 2026-08-13
**Spec reference:** §1.2, §16.1, §35

## Context

Collections must support Ghanaian cards and Mobile Money, and payouts need provider transfers. Commercial terms or coverage may later require a second provider (Hubtel). Raw card/MoMo handling by ProGuideGH is prohibited (§1.2). Provider callbacks are retried and must be treated as hostile input until authenticated (§36).

## Decision

All payment/payout access goes through a single Go adapter interface (Spec §16.1): `InitializePayment`, `VerifyPayment`, `Refund`, `CreateTransfer`, `VerifyWebhook`. Paystack is the first implementation, selected per the chosen stack (§35); Hubtel is added later through the same interface only if commercial/operational requirements demand it.

Rules:

- No raw card data ever touches our systems; collection uses provider-hosted/tokenized flows (§1.2).
- Webhook signature verification is mandatory; provider reference uniqueness prevents duplicate postings (§9.2, §14).
- Webhooks are authoritative for asynchronous payment success/failure; client redirects never confirm a booking (§4.5, §30.1).
- Provider selection is configuration (`PAYMENT_PROVIDER`), not compile-time coupling.
- Sandbox/mock implementations satisfy development and CI when credentials are unavailable (§31.29).

## Consequences

- Provider swap or addition is localized to the adapter package.
- All payment flows inherit uniform webhook verification, dedupe and idempotency behavior.
- Domain code cannot accidentally depend on Paystack-specific request/response shapes; cross-cutting leakage is a review finding.
- Gateway fees are recorded separately according to who bears them (§9.1).
