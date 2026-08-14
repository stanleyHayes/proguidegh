# Payout operations runbook (Phase 7, spec §8.4)

## Weekly cycle

- The API runs the payout batch automatically every **Monday** (hourly
  check; runs at startup too, so a weekend deploy cannot skip a cycle).
- Manual run: `POST /api/v1/admin/payouts/batch` (payouts.manage) — safe to
  re-run any time; `idx_payouts_guide_schedule` makes duplicates impossible
  (P7-07).
- The batch queues one QUEUED payout per guide whose eligible balance has
  cleared `payout_delay_days` (default 7), net of in-flight and paid
  payouts.

## Daily finance flow

1. Admin web → **Finance** → review QUEUED payouts.
2. **Export today's CSV** (payouts.manage). This is the only surface where
   destination account refs appear decrypted; the export is audited. Handle
   the file as sensitive; delete after processing.
3. Execute transfers in the provider dashboard (manual fallback until
   EXT-2 live transfer credentials arrive).
4. Move each payout QUEUED → PROCESSING → **PAID** (add the provider
   reference). PAID posts the ledger move (debit `guide_payable_eligible`,
   credit `tourist_clearing`) atomically and stores
   `payouts.ledger_transaction_id`.
5. On provider failure: PROCESSING → FAILED with a **mandatory reason**,
   then FAILED → RETRY_QUEUED for the next cycle, or → MANUAL_REVIEW for
   anything suspicious (wrong account, repeated failures, guide dispute).

## Reconciliation

- Weekly: Finance page payout totals vs ledger balances
  (`guide_payable_eligible` should trend down by the paid amount).
- Monthly: **Reports → tourism levy** (`GET /admin/reports/tourism-levy`)
  for the levy remittance; export bookings CSV for the same window.
- Any payout stuck in PROCESSING > 24h: check the provider, then either
  PAID (with reference) or FAILED (with reason) — never leave it hanging;
  in-flight payouts block the guide's next batch amount.

## Incident: duplicate or wrong payout

- Not paid yet: MANUAL_REVIEW and leave it there pending investigation.
- Paid in error: record a note in the audit (payout transition), contact
  the guide, and book the recovery through finance — **never** hand-edit
  ledger rows; corrections are new balanced postings.
