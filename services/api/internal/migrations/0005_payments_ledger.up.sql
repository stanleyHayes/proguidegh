-- 0005_payments_ledger.up.sql — Phase 4: payments, ledger & receipts
-- (spec §4.5, §8.3, §9, §13.3, §14, §17).
--
-- The payments table (0001) already carries the full §8.3 state set
-- (CREATED, PENDING, SUCCEEDED, FAILED, EXPIRED, REFUND_PENDING,
-- PARTIALLY_REFUNDED, REFUNDED) as a CHECK; legal edges are enforced in the
-- internal/payments state machine, not by triggers. This migration adds the
-- receipts and webhook-dedupe tables and seeds the V1 chart of accounts.

-- ---------------------------------------------------------------------------
-- Hosted authorization URL for in-flight payments, stored so an idempotent
-- replay of payment initiation returns the SAME provider URL/reference
-- (spec §14) without re-initializing at the provider. NULL for pre-Phase-4
-- rows; never logged (spec §22.1).
-- ---------------------------------------------------------------------------

ALTER TABLE payments ADD COLUMN authorization_url text;

-- ---------------------------------------------------------------------------
-- Receipts (spec §17). One receipt per booking (UNIQUE booking_id — issuing
-- is idempotent by construction). receipt_number is the human-readable
-- reference ("PGH-XXXXX" style, random with collision retry, same convention
-- as bookings.reference). object_key points into private object storage and
-- contains no personal data; downloads are short-lived signed URLs only
-- (stop condition 8). APPEND-ONLY by intent: receipts are immutable once
-- issued; corrections are new documents.
-- ---------------------------------------------------------------------------

CREATE TABLE receipts (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id     uuid NOT NULL UNIQUE REFERENCES bookings (id),
    receipt_number text NOT NULL UNIQUE,
    object_key     text NOT NULL,
    issued_at      timestamptz NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- Webhook dedupe (spec §14: provider callback signature verification +
-- replay protection). The row is written in the SAME database transaction as
-- the side effects it gates (payment confirmation, ledger posting, receipt),
-- so a crash mid-processing rolls the claim back and the provider retry is
-- processed fresh. UNIQUE (provider, event_reference) makes a replayed
-- webhook a no-op: provider_reference (the provider's transaction id) is the
-- event_reference for charge events — Paystack delivers no per-event id.
-- raw_body_hash is the sha256 of the exact bytes received (payload archive
-- per §14 policy).
-- ---------------------------------------------------------------------------

CREATE TABLE webhook_events (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider        text NOT NULL,
    event_reference text NOT NULL,
    raw_body_hash   text NOT NULL,
    processed_at    timestamptz NOT NULL DEFAULT now(),
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider, event_reference)
);
CREATE INDEX idx_webhook_events_provider ON webhook_events (provider, created_at);

-- ---------------------------------------------------------------------------
-- V1 chart of accounts (spec §9.1, §9.2) — platform-owned, owner_id NULL,
-- one GHS account per code. Per-booking attribution travels on
-- ledger_transactions.booking_id, so per-guide payable sub-accounts are not
-- needed for V1 reconciliation (payout phase may add them).
--
--   code                    class      purpose
--   ----------------------  ---------  -------------------------------------
--   tourist_clearing        asset      cash collected from the tourist,
--                                      debited at payment confirmation,
--                                      settled out against the provider
--                                      settlement (finance export, §9.2)
--   platform_revenue        revenue    platform fee share (default 15%)
--   tourism_levy_payable    liability  Tourism Levy share (default 3%) owed
--                                      to the tourism authority
--   guide_payable_pending   liability  guide gross share held from collection
--                                      until completion (§4.5)
--   guide_payable_eligible  liability  guide share eligible for payout after
--                                      the payout-delay policy (pending ->
--                                      eligible move lands with completion)
--   gateway_fees            expense    provider processing fees, recorded
--                                      separately per §9.1 once settlement
--                                      data identifies who bears the fee
--
-- Allocation of a succeeded payment (spec §9.1, GHS 450 @ 15%/3%):
--   debit  tourist_clearing        450.00
--   credit platform_revenue         67.50
--   credit tourism_levy_payable     13.50
--   credit guide_payable_pending   369.00
-- Debits equal credits exactly; amounts are integer pesewas in domain code
-- and NUMERIC(14,2) here. Refunds post a REVERSAL transaction with flipped
-- directions (originals are immutable — §9.2).
-- ---------------------------------------------------------------------------

INSERT INTO ledger_accounts (owner_type, owner_id, currency, code)
SELECT 'platform', NULL, 'GHS', v.code
FROM (VALUES
    ('tourist_clearing'),
    ('platform_revenue'),
    ('tourism_levy_payable'),
    ('guide_payable_pending'),
    ('guide_payable_eligible'),
    ('gateway_fees')
) AS v (code)
WHERE NOT EXISTS (
    SELECT 1 FROM ledger_accounts a
    WHERE a.owner_type = 'platform' AND a.owner_id IS NULL AND a.code = v.code
);
