# Access and onboarding review — 2026-08-14

Scope: admin invitation and role-management flow, administrator authentication/security settings, and guide account creation → application → verification.

## Findings resolved

1. **Critical — guide application could not pass API validation.** The page posted `region` and `languages` to a handler that only accepts `region_id`; unknown fields are rejected. It now uses catalog region IDs and persists language selections through the profile endpoint.
2. **High — guide document registration used an incompatible contract.** The UI sent unsupported keys and document values. It now sends `type` and `content_type` with the API's supported evidence dictionary.
3. **High — no administrator invitation lifecycle existed.** Added audited, 72-hour, single-use staff invitations; protected create/list endpoints; a public acceptance endpoint; an acceptance page outside the admin shell; lifecycle status; and replay protection.
4. **High — administrator and guide MFA screens stopped at placeholder copy.** Both sign-in screens now complete real MFA challenges, and admin security settings now support authenticator enrollment, verification, and one-time recovery-code display.
5. **High — role changes silently replaced a multi-role account with one dropdown value.** Access review now shows the complete role set in a branded confirmation panel. Empty role sets and self-removal of `super_admin` are rejected.
6. **Medium — email-only API was presented as email-or-phone authentication.** Guide registration and guide/admin sign-in now accurately request email, preventing guaranteed validation failures.
7. **Medium — guide dashboard completeness read fields from the wrong response level.** Languages, specialties, and outstanding requirements are now returned/read consistently.
8. **Medium — onboarding lacked orientation and used stale hardcoded regions.** The application has an Account → Application → Verification progress indicator, live Ghana region data, branded language choices, and clearer next actions.
9. **Medium — native confirmation interrupted admin work.** The browser `confirm()` role mutation was replaced by an accessible in-product dialog with explicit scope and audit messaging.

## Known delivery boundary

The invitation endpoint returns a secure one-time link for an authorized administrator to copy and share. Automated email delivery remains dependent on the planned notification worker/provider integration; no production email behavior is claimed in this change.

## Verification expectations

- Migration 0012 must round-trip and expose the pending-email and expiry indexes.
- Invitation create requires `users.manage`; list requires `users.read`; creation is audited.
- Acceptance is single-use, expired links return 410, and assigned roles are verified at login.
- Guide application uses real region UUIDs and persisted language codes.
- Admin and guide web lint/typecheck/build, generated OpenAPI contracts, Go formatting/vet/tests, and route smoke checks must pass before publish.
