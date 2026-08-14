-- 0005_payments_ledger.down.sql — roll back Phase 4 payments/ledger/receipts.

-- Seeded platform accounts are only removed when they have no entries;
-- a database with posted financial history keeps them (append-only ledger,
-- spec §9.2 — never delete posted accounting rows).
DELETE FROM ledger_accounts
WHERE owner_type = 'platform' AND owner_id IS NULL
  AND code IN ('tourist_clearing', 'platform_revenue', 'tourism_levy_payable',
               'guide_payable_pending', 'guide_payable_eligible', 'gateway_fees')
  AND id NOT IN (SELECT DISTINCT account_id FROM ledger_entries);

DROP TABLE IF EXISTS webhook_events;
DROP TABLE IF EXISTS receipts;

ALTER TABLE payments DROP COLUMN IF EXISTS authorization_url;
