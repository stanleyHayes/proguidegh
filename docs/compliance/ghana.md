# Ghana regulatory compliance

> **Read this first.** This document is an engineering checklist, not legal
> advice. The obligations below are real and the platform work is done, but
> **every statutory citation, retention period and registration step must be
> confirmed by Ghanaian counsel before the privacy policy is published or the
> launch checklist is signed off** (Spec §33: "Required legal/privacy/terms/
> consent text approved by responsible parties"). Where this file names a
> section number, treat it as a pointer for counsel to verify, not as settled
> law.

## 1. Data Protection Act, 2012 (Act 843)

Ghana's data protection regime is administered by the **Data Protection
Commission (DPC)**.

### Registration — Human, blocked

Entities that process personal data must **register with the DPC as a data
controller**, and registration is renewable. This cannot be done in code.

- [ ] Register ProGuideGH as a data controller with the DPC
- [ ] Record the registration number; it usually must appear in the privacy policy
- [ ] Diarise the renewal date
- [ ] Confirm with counsel whether a Data Protection Supervisor/Officer must be
      appointed and named

### Data-subject rights — implemented

| Right | Endpoint | Implementation |
|---|---|---|
| Access to personal data | `GET /api/v1/me/export` | Returns account, profile, bookings, reviews and consent history as JSON. Audited as a privileged read. |
| Correction | `PATCH /api/v1/me/tourist-profile`, guide profile endpoints | Existing profile editing. |
| Erasure | `DELETE /api/v1/me` | Anonymisation — see [`data-inventory.md`](./data-inventory.md#retention-after-account-deletion) for why financial records survive. |
| Withdraw consent | `POST /api/v1/me/consent` (new version), deletion | Consent history is append-only. |

### Consent — implemented

`consent_records` stores which document version a user accepted and when.
Append-only, so consent history is auditable: a later acceptance is a new row,
never an overwrite. Consent must be demonstrable, which means retrievable — it
is included in the export.

- [ ] Counsel to confirm the lawful basis stated for each processing purpose
      (consent vs contract vs legal obligation), because that determines which
      data survives an erasure request

### Security — implemented

- Passwords hashed with Argon2id/bcrypt; MFA for admin and finance roles.
- Verification documents in **private** R2, reachable only via short-lived
  signed URLs — never public objects.
- TOTP secrets encrypted at rest with a derived key.
- Every privileged and financial action written to an append-only audit log.
- TLS everywhere; Cloudflare WAF in front.

### Cross-border transfer — needs legal review

Personal data is processed outside Ghana: Render (hosting), Cloudflare R2
(documents), Vercel (frontends), Paystack (payments).

- [ ] Counsel to confirm what Act 843 requires for foreign processing —
      disclosure in the policy, DPC notification, or contractual safeguards —
      and whether any data must remain in-country
- [ ] Ensure every processor in the data inventory's sub-processor table appears
      in the published policy

## 2. Ghana Tourism Authority (GTA)

The platform's premise is *certified* guides, which makes this substantive
rather than paperwork: an unearned "verified" badge is an explicit launch
blocker (Spec Appendix D).

- [ ] Confirm whether ProGuideGH itself needs a licence as a tour operator or
      travel-services provider, distinct from the guides it lists
- [ ] Confirm what makes a guide "certified" in law, and that our certification
      pipeline (Spec §5) matches it
- [ ] Confirm the Tourism Levy rate and remittance mechanics against the current
      legislation — the platform treats it as an **effective-dated configurable
      rule** in `pricing_rules`/`system_settings`, never hard-coded, so a rate
      change is a config change (Spec §31.23)
- [ ] Pilot guide dataset fully verified; no placeholder badges (Spec §33)

## 3. Payments and financial regulation

- Card and Mobile Money details are captured **only** on Paystack's hosted page.
  ProGuideGH never sees, transmits or stores them (Spec §1.2 #6). This keeps the
  platform out of direct PCI-DSS scope, but confirm the exact SAQ obligation
  with the acquirer.
- Paystack is the licensed payment service provider; Bank of Ghana authorisation
  and AML/KYC of the payment rails are theirs, not ours.
- [ ] Confirm whether ProGuideGH's role in holding and disbursing guide earnings
      creates its own regulatory obligation (agency, e-money, or payment
      aggregation) — this is the item most likely to surprise
- [ ] Confirm guide payout tax treatment: withholding obligations, and whether
      guides must be issued statements
- [ ] Confirm current electronic-transaction levy applicability. The platform
      models levies as effective-dated configurable rules, so a change is a
      config update rather than a code change — but the *current* position must
      be established before launch.

## 4. Consumer protection

- Server-authoritative pricing: the price shown is the price charged, computed
  server-side from effective-dated rules (Spec §1.2, §14). Clients never
  compute totals.
- Every booking produces an immutable receipt with the platform fee and levy
  broken out.
- Cancellation and refund policy must be published and must match the refund
  state machine in code.
- [ ] Counsel to review the cancellation/refund terms against Ghanaian consumer
      protection law before publication

## 5. What blocks launch

From Spec §33 and Appendix D, the items in this document that are hard blockers:

- [ ] DPC registration complete
- [ ] Privacy policy, terms and consent text approved and **published at the URLs
      in `legal_documents`** — they are placeholders returning 404 today
- [ ] Tourism Levy rate confirmed and configured
- [ ] No unearned "verified"/"insured" badges in the pilot dataset
- [ ] Data retention and privacy review signed off (P9-06)
