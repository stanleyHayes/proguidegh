DROP INDEX IF EXISTS idx_payouts_guide_schedule;

ALTER TABLE payouts
    DROP COLUMN IF EXISTS failure_reason,
    DROP COLUMN IF EXISTS ledger_transaction_id;
