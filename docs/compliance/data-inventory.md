# Data inventory

The single source of truth behind the Apple privacy nutrition label, the Google
Play Data Safety form, the privacy policy and the deletion/export endpoints. If
any of those four disagree with this table, this table is wrong or they are —
reconcile before submitting, because store reviewers compare them.

Generated from the schema in `services/api/internal/migrations/` and the
collection points in the apps. Re-check whenever a migration adds a column that
holds personal data.

## What is collected

| Data | Where it lives | Why | Deleted on erasure? | Apple category | Play category |
|---|---|---|---|---|---|
| Email address | `users.email` | Account identity, sign-in, receipts | **Yes** (replaced with an unusable value) | Email Address | Personal info › Email address |
| Phone number | `users.phone_e164` | OTP, guide/tourist contact during a tour | **Yes** | Phone Number | Personal info › Phone number |
| Password | `users.password_hash` (Argon2id/bcrypt) | Authentication | **Yes** (replaced with a non-verifying sentinel) | — (credentials, not collected data) | — |
| Name | `tourist_profiles.full_name`, `guide_profiles.public_name` | Identify guide/tourist to each other | **Yes** (guide profile becomes "Former guide") | Name | Personal info › Name |
| Nationality, preferred language | `tourist_profiles` | Guide matching | **Yes** | Other Data | Personal info › Other info |
| Emergency contact name + phone | `tourist_profiles` | SOS escalation only | **Yes** | Contact Info | Personal info › Other info |
| Precise location (guide) | Redis (60s TTL) + `location_checkpoints` | Dispatch matching, live tour tracking, emergency response | **Yes** | Precise Location | Location › Precise location |
| Verification documents | Private R2, keyed in `guide_documents` | Certification (Spec §5) | **Yes** — rows and objects | Sensitive Info | Personal info › Other info |
| Payout account (tokenised) | `payout_accounts.account_ref_tokenized` | MoMo payouts | **Yes** | Payment Info | Financial info › Payment info |
| Bookings | `bookings` | The service itself | **No** — see retention | Purchase History | Financial info › Purchase history |
| Payments, ledger, receipts | `payments`, `ledger_*`, `receipts` | Tax + tourism-levy reconciliation | **No** — see retention | Purchase History | Financial info › Purchase history |
| Reviews | `reviews` | Guide quality signal for other travellers | **No** — retained, unlinked from the author's identity | User Content | App info › Other user-generated content |
| Consent records | `consent_records` | Proof of consent | **No** — retaining it *is* the point | Other Data | App info › Other |
| Audit log | `audit_logs` | Privileged/financial action trail (§22) | **No** — append-only by design | Other Data | App activity › Other actions |

## What is deliberately NOT collected

Stating these plainly matters: both stores penalise over-declaring almost as
much as under-declaring.

- **No card or Mobile Money credentials.** Payment happens on the provider's
  hosted page; the apps never render a card field (Spec §1.2 #6). The provider
  returns a tokenised reference only.
- **No advertising identifiers, no cross-app tracking, no ad SDKs.** This is why
  `NSPrivacyTracking` is `false` and the apps never show an ATT prompt.
- **No contacts, photos, microphone or calendar access.**
- **No tourist background location.** Only the guide app collects location, and
  only during an active tour — see below.

## Location: the sensitive one

This is the declaration both stores scrutinise, and the reason the apps are
native at all (decision D5).

- **Who:** guides only. The tourist app has no location permission at all.
- **When:** only while the guide is online, or on a tour in
  `GUIDE_EN_ROUTE`/`GUIDE_ARRIVED`/`IN_PROGRESS` (Spec §11.1). Enforced in
  `apps/guide-mobile/src/lib/location-task.ts` by an active-booking key in
  secure storage — with no active booking the background task discards every
  fix instead of sending it.
- **Background:** yes, with an Android foreground-service notification the guide
  can see. iOS requires "Always" permission.
- **Retention:** high-frequency coordinates live in Redis with a 60s TTL and are
  never persisted. Coarse checkpoints persist per the §11.2 retention policy.
- **Shared with:** the tourist on that booking, and operations staff holding
  `dispatch.manage`. Nobody else. Historical movement is never exposed to
  tourists.
- **Disclosure:** `apps/guide-mobile/src/app/location-permission.tsx` explains
  all of the above in-app *before* the OS prompt, which is what the store
  policies require.

## Retention after account deletion

Erasure is anonymisation, not row deletion — `bookings`, `ledger_entries`,
`receipts` and `audit_logs` are append-only (Spec §8) and all reference
`users.id`. Dropping the row would either cascade away immutable financial
history or break referential integrity.

Retention of those records rests on a legal obligation (tax, tourism-levy
reconciliation, fraud investigation) rather than consent, which is a recognised
exemption from erasure. What must go is the personal data, and that is exactly
what anonymisation removes: with the `users` row anonymised, the retained
financial rows reference an id that identifies nobody.

The precise legal citations in the published privacy policy must be confirmed by
counsel — see `ghana-data-protection.md`.

## Sub-processors

| Processor | Data | Purpose |
|---|---|---|
| Paystack | Payment details (entered on their page, never ours) | Collections and payouts |
| Cloudflare R2 | Verification documents, receipts | Private object storage |
| Cloudflare | Traffic metadata | DNS, WAF, CDN |
| Render | All application data at rest/in transit | Hosting |
| Vercel | Web traffic metadata | Frontend hosting |
| Google Maps | Coarse map tile requests | Maps |
| FCM (planned) | Device push tokens | Notifications — not yet shipped |
| SMS provider | Phone number | OTP and SOS fallback |

Each must appear in the published privacy policy. A processor missing from the
policy is a finding in a data-protection audit.
