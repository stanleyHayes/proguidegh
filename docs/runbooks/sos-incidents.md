# SOS & incident response runbook (Phase 6, spec §12)

## SOS lifecycle

1. **Trigger**: tourist or guide hits SOS on an active booking →
   `sos_events` row + a CRITICAL incident + `sos.triggered` on
   `/ws/admin/operations`. The operations board and Safety desk light up
   immediately.
2. **Acknowledge** (target < 2 min): Safety desk → incident →
   Acknowledge. This also marks the SOS events acknowledged.
3. **Contact**: call the tourist first, then the guide, using the booking
   contact details. If unreachable or danger is indicated, escalate to
   local emergency services with the last known location from
   `GET /api/v1/bookings/{id}/location` (active-window tracking).
4. **Work the incident**: notes for every action (they are timestamped and
   attributed, §12 step 11), escalate severity if the situation worsens,
   assign to a specific operator when handing off.
5. **Resolve** with a mandatory resolution note describing the outcome;
   **close** once any follow-up (refund, guide suspension, police report)
   is complete. Reopen only if new information surfaces.

## Other incident types

- **quality/safety reports** from reviews or admin observation: same
  workflow, lower severity. Acknowledge within the business day.
- **Payment/ledger anomalies**: severity high; involve finance before
  resolving.

## Quality queue (§4.4)

- Review aggregation opens flags automatically: rolling average below the
  low threshold → retraining flag; Elite qualification → elite review.
- Work the queue weekly: resolve each flag with a note (required).
  Retraining flags should link the guide to the relevant course in
  Training admin; repeat offenders go to certification review
  (REQUIRES_RETRAINING / suspension per §5).

## Escalation matrix

| Severity | First responder | If unresolved 30 min | If unresolved 2 h |
|---|---|---|---|
| critical (SOS) | on-call operator | operations lead | technical owner + authorities as needed |
| high | on-call operator | operations lead | operations lead decides |
| medium/low | next business day | — | — |
