-- Phase 7: wallet, payouts & finance (spec §8.4, P7-01…P7-07)

-- failure_reason records why a payout FAILED (operator-supplied);
-- ledger_transaction_id links a PAID payout to its balanced ledger posting
-- (debit guide_payable_eligible, credit tourist_clearing) so finance can
-- reconcile payouts against the immutable ledger.
ALTER TABLE payouts
    ADD COLUMN failure_reason         text,
    ADD COLUMN ledger_transaction_id  uuid REFERENCES ledger_transactions (id);

-- Batch idempotency (P7-07): at most one non-failed payout per guide per
-- scheduled date, so re-running the weekly batch — manually or via the
-- scheduler — can never double-queue a payout.
CREATE UNIQUE INDEX idx_payouts_guide_schedule
    ON payouts (guide_id, scheduled_for)
    WHERE status <> 'FAILED';
