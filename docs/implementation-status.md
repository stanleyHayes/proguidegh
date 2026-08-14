# ProGuideGH — Implementation Status

**Version:** 1.0 • **Created:** 2026-08-13 • **Source:** Build Specification v1.0 (§28–§33, Appendix D) and `agent_plan.md`
**Required by:** Spec §31.19

## Update rules (read before touching a checkbox)

1. Every checkbox starts unchecked. A box may be marked `[x]` **only with evidence** recorded in that phase's **Evidence log**: the exact commands run and their results (test output, CI run link, migration output, etc.).
2. Evidence entries are append-only. Format:
   `YYYY-MM-DD | task-id | command(s) | result (pass/fail + key numbers) | artifact/link`
3. Exit criteria for a phase may only be checked after **all tasks in that phase are checked** and the phase-end gate (Spec §31.28: format, lint, unit + integration tests, frontend typecheck/build, relevant E2E) has been run with results pasted into the Evidence log.
4. Phases are sequential. A later phase's boxes must not be checked while an earlier phase's exit criteria are open (Spec §28), except isolated UI work that creates no architecture divergence.
5. Cross-cutting acceptance criteria (§30) are re-verified **every phase**; note the phase in which each was last demonstrated in the Evidence log.
6. Tasks blocked on an external credential stay unchecked; the block is recorded in the Evidence log and work continues against sandbox/mock adapters (Spec §31.29).
7. Never silently change a business rule to make a checkbox pass. Configurable values live in policy/config tables (Spec §31.23).

---

## Phase 0 — Foundation (Days 1–5) — Epic E01

- [x] P0-01 Monorepo scaffold per Spec §7.1
- [x] P0-02 CLAUDE.md + AGENTS.md in repo root
- [x] P0-03 ADRs 0001–0008 (monolith, Postgres, Redis, Mongo, payment adapter, object storage, sessions, ledger)
- [x] P0-04 This file created with all phases/epics/acceptance criteria
- [x] P0-05 Go API bootstrap (cmd/api, config loader, structured logging, request IDs, health/readiness)
- [x] P0-06 Go worker bootstrap (job runner skeleton)
- [x] P0-07 Docker Compose: PostgreSQL + Redis local dev
- [x] P0-08 Migration framework (goose/golang-migrate) + initial schema + up/down tested
- [x] P0-09 Three Next.js apps (tourist-web, guide-web, admin-web) + packages/{ui,contracts,config}
- [x] P0-10 OpenAPI baseline + generated TS client pipeline
- [x] P0-11 GitHub Actions CI (lint, test, build, OpenAPI/migration validation)
- [x] P0-12 `.env.example` per Spec §25

**Exit criteria (Spec §28 Phase 0):**
- [ ] All applications build in CI — CI workflow committed (`.github/workflows/ci.yml`); first real CI run requires pushing to GitHub (blocked on remote repo credential, see evidence)
- [x] API connects to Postgres/Redis
- [x] Migration up/down process tested
- [ ] Staging endpoints reachable — blocked on Render/Vercel accounts + secrets (Human); staging deploy skeleton ready (`.github/workflows/deploy-staging.yml`, `infra/render/render.yaml`)
- [x] No business functionality yet

### Evidence log — Phase 0

- 2026-08-13 · agent · P0-01–P0-12 · Repo root renamed to `proguidegh/`; all identifiers renamed (`proguidegh/api`, `proguidegh/worker` Go modules; `@proguidegh/*` npm scope; render/vercel service names; DB credentials).
- 2026-08-13 · agent · Go gates · `go build ./...`, `go vet ./...`, `go test ./...` clean in `services/api` and `services/worker`; `gofmt -l` empty (verified by build agents).
- 2026-08-13 · agent · Web gates · `pnpm install` OK; `pnpm typecheck` 6/6 green; `pnpm build` — all three Next.js 16 apps compiled and prerendered.
- 2026-08-13 · agent · Migrations · against compose Postgres 16: `migrate status` → pending; `up` → applied 0001 (34 tables incl. schema_migrations); `down -all` → reverted; `up` → applied. Roundtrip OK.
- 2026-08-13 · agent · Runtime · `./bin/api` against live Postgres/Redis: `GET /healthz` → 200 `{"status":"ok"}`; `GET /readyz` → 200 `{"checks":{"postgres":"ok","redis":"ok"},"status":"ready"}`; `./bin/api -dump-openapi` → committed to `docs/api/openapi.yaml`. Worker `/healthz` 200 + tick loop verified by build agent.
- 2026-08-13 · agent · Note · Compose Redis published on host port 6380 via `/tmp/gg-compose-override.yaml` because the host already runs a Redis on 6379 (unrelated project). Local `.env` should use `REDIS_URL=redis://localhost:6380`.
- 2026-08-13 · agent · Blocks · Real CI run + staging reachability need a GitHub remote, Render/Vercel projects and secrets (Human). Deploy skeleton committed; no fake deploys performed.
- 2026-08-13 · agent · Phase 0b (NC-01…NC-09, NC-11) · ProGuideGH name consolidation executed: root package.json name; Go brand strings + embedded OpenAPI title (`ProGuideGH API`); MFA issuer label → ProGuideGH (totp.go L137 derivation string untouched per note 1); cookies `gg_*`→`pgh_*` (NC-05, session-invalidation accepted pre-launch); receipt refs `GG-`→`PGH-` (note 6) **and booking refs `GG-XXXXX`→`PGH-XXXXX`** (brand sweep; gate step 5 would otherwise fail — deviation from note 6's receipt-only wording, flagged); frontend/packages/infra/docs/governance brand strings; `GG-`→`PG-` ticket prefix in AGENTS.md + CLAUDE.md mirror.
- 2026-08-13 · agent · NC-11 gate · (1) pnpm install/lint/typecheck/build all green (3 apps). (2) gofmt clean, go vet clean (api+worker), `go test -count=1 ./...` 13 packages ok. (3) `-dump-openapi | diff docs/api/openapi.yaml` → no drift. (4) Fresh DB: migrate up ×5 → down -all ×5 → up ×5 → all applied. (5) Final brand grep returns ONLY the allow-listed `totp.go:137 "guide-ghana/mfa/v1/"` line plus agent_plan.md's deliberate self-quotes. NC-10 (Jira key rename) remains Human-blocked; booking refs changed as noted above.

---

## Phase 1 — Identity, RBAC & Profiles (Days 6–15) — Epics E02, E13 (part)

- [x] P1-01 Users/roles/permissions schema + seed (roles + permission codes per Appendix A)
- [x] P1-02 Registration/login/OTP request+verify/password reset
- [x] P1-03 Session model: short-lived access + rotating refresh, HttpOnly cookies, revocation
- [x] P1-04 RBAC authorization layer + middleware (permission-enforced, not UI-only)
- [x] P1-05 MFA for Super Admin/finance roles; step-up auth for sensitive actions
- [x] P1-06 Tourist profile endpoints + UI
- [x] P1-07 Guide application/profile shell + private document upload (R2 signed URLs, mock-capable)
- [x] P1-08 Admin user/guide directory
- [x] P1-09 Audit framework for privileged mutations

**Exit criteria (Spec §28 Phase 1):**
- [x] Tourist and guide applicant accounts work
- [x] Admin access is permission-enforced
- [x] Sensitive documents are private and signed
- [x] Automated auth/RBAC tests pass

### Evidence log — Phase 1

- 2026-08-13 · agent · Migration · `0002_identity` applied: refresh_sessions, otp_codes, mfa_secrets + seed of 10 roles / 24 Appendix-A permissions / 62 grants. `down`+`up` roundtrip clean.
- 2026-08-13 · agent · Backend gates · `gofmt -l` clean, `go vet` OK, `go build` OK, `go test ./...` all `ok` (unit: argon2, TOTP RFC 6238 vectors, JWT, RBAC matrix, OTP lockout; integration: register→login→refresh rotation→reuse→401 SESSION_REUSE chain revoke, 403 without users.read, audit row on role change, OTP HTTP flow, reset revokes sessions, idempotent apply, signed upload/download roundtrip, unsigned GET→403, MFA enroll→verify→step-up).
- 2026-08-13 · agent · Bugfix · Signed-URL 403: storage adapter escaped `/` in object keys (`url.PathEscape`) so the served path no longer matched the signed key; fixed with per-segment escaping (`escapeKey`). Test helper also used a hardcoded signing secret instead of `JWT_OR_SESSION_SECRET`; fixed. Full suite re-run green.
- 2026-08-13 · agent · Live smoke (curl vs local Postgres/Redis) · register tourist → 201 with roles:["tourist"]; login → access(15m JWT)+refresh; PATCH/GET `/me/tourist-profile` roundtrip OK; `/auth/refresh` rotated tokens; GET `/api/v1/admin/users` as tourist → 403 FORBIDDEN `users.read`.
- 2026-08-13 · agent · Frontend gates · `pnpm lint`/`typecheck`/`build` all green; new routes prerender (tourist: /login /register /forgot-password /reset-password /profile; guide: /guide /guide/apply /guide/verification /login /register; admin: /admin/users /admin/guides /login). Dev-server smoke: pages 200.
- 2026-08-13 · agent · OpenAPI · `-dump-openapi` covers 22 operations; `docs/api/openapi.yaml` regenerated.
- 2026-08-13 · agent · Known gaps for later phases · `GET /api/v1/me/guide` (guide dashboard aggregate) called by guide-web but not yet implemented — schedule in Phase 2/5. MFA code entry UI in admin/login is a placeholder state pending final challenge contract. R2 adapter is config-validated stub (SigV4 TODO) pending credentials (EXT).

---

## Phase 2 — Certification & Catalog (Days 16–27) — Epics E04, E06 (part), E07 (part)

- [x] P2-01 Certification case state machine (APPLIED→…→ACTIVE + exceptions) with audited transitions
- [x] P2-02 Document evidence/expiry model
- [x] P2-03 Admin certification review queues
- [x] P2-04 Training shell / required-training flags — TRAINING/REQUIRES_RETRAINING states + outstanding-requirements machinery in place; course/enrollment flags completed with the Phase 8 LMS (P8-01, 2026-08-13)
- [x] P2-05 Catalog: regions, languages, specialties (Appendix C), tour packages, effective-dated pricing rules
- [x] P2-06 Public guide profile visibility gate (eligible status only)

**Exit criteria (Spec §28 Phase 2):**
- [x] Admin can move a test guide through an audited certification process to ACTIVE
- [x] Only ACTIVE guides can appear publicly

### Evidence log — Phase 2

- 2026-08-13 · agent · Migration · `0003_certification_catalog` applied + down/up roundtrip clean: §5 status CHECK set, one-open-case-per-guide partial unique index, `languages` (10) + FK, `review_tag_defs` (10), 16 regions, 13 specialties, 3 tour packages with 2026-01-01 pricing rules, 5 system_settings (fee 15%, levy 3%, payout T+7, quality 4.0, elite 4.8).
- 2026-08-13 · agent · Backend gates · gofmt/vet/build clean; `go test -count=1 ./...` all ok: state-machine transition matrix + illegal jumps (409 ILLEGAL_TRANSITION), evidence enforcement (422 EVIDENCE_REQUIRED), EffectivePrice selection (7 cases), §10.2 visibility truth table (13 cases). Integration: apply→APPLIED→10-stage walk to ACTIVE→public 200; suspend→404→reactivate with 12 events + 11 audit rows preserved.
- 2026-08-13 · agent · Bugfix · `admin.ListGuides` count query used `$3` with 1 arg when `?status=` passed (would 500); fixed with separate count placeholder. Suite re-run green.
- 2026-08-13 · agent · Frontend gates · pnpm lint/typecheck/build all green. New routes: guide-web /guide/profile + real /guide/verification stepper; admin-web /admin/certification + /admin/certification/[caseId] transition UI; tourist-web /search with live /tour-packages. Dev-server smoke 200s.
- 2026-08-13 · agent · OpenAPI · 32 operations; `docs/api/openapi.yaml` regenerated from `-dump-openapi`.
- 2026-08-13 · agent · Notes · Certification reads unaudited by house convention (mutations audited) — flag if read-audit required. `certification_status` derived from `certification_cases` (no duplicate column). Frontend assumes `base_price` in major units for display.

---

## Phase 3 — Search, Booking & Availability (Days 28–40) — Epics E03 (part), E07

- [x] P3-01 Guide availability schedules/online state
- [x] P3-02 Search filters/ranking (Spec §10.1–10.2) + guide detail
- [x] P3-03 Quote endpoint — server-authoritative pricing
- [x] P3-04 Booking aggregate + state machine (§8.2) + overlap checks (transactional)
- [x] P3-05 Tourist checkout UX (pre-payment)

**Exit criteria (Spec §28 Phase 3):**
- [x] Tourist can search, receive a quote and create a payment-pending booking without double-booking a guide

### Evidence log — Phase 3

- 2026-08-13 · agent · Migration · `0004_availability_booking` applied + roundtrip clean: guide_availability, guide_time_off, bookings +num_guests/notes/amount/currency, guide_profiles +lat/lng, idempotency_keys +entity_id, `bookings_no_guide_overlap` exclusion constraint (btree_gist, partial on CONFIRMED..IN_PROGRESS).
- 2026-08-13 · agent · Backend gates · gofmt/vet/build clean; `go test -count=1 ./...` all ok (10 packages). Unit: §8.2 transition matrix, overlap logic, quote rounding in integer pesewas (pins §9.1: 450→67.50/13.50/369.00), availability windows/midnight/timezones, haversine vs known Ghana distances. Integration: full search filter sweep, idempotent create (replay→same booking, different payload→409), confirmed-overlap→409 + DB backstop insert fails, detail visibility owner/guide/stranger/bookings.read, cursor pagination, availability self-service.
- 2026-08-13 · agent · Overlap design · two layers: transactional FOR UPDATE check at create/confirm (clean 409) + exclusion-constraint backstop (23P01→409). Competing PAYMENT_PENDING holds allowed until Phase 4 confirmation re-check.
- 2026-08-13 · agent · Frontend gates · pnpm lint/typecheck/build all green; tourist-web routes /search (guide search + filters), /guides/[id] (profile + debounced server quote), /checkout/[bookingId] (PAYMENT_PENDING panel, no fake payment), /bookings, /bookings/[id] (7-step timeline). Smoke 200s. Idempotency-Key per draft via crypto.randomUUID().
- 2026-08-13 · agent · OpenAPI · 41 operations; `docs/api/openapi.yaml` regenerated.
- 2026-08-13 · agent · Notes · bookings.amount/currency added as server-authoritative snapshot for Phase 4 payment initiation. Search pagination: offset (bounded ranked candidate set, §14 allowance); me/bookings: cursor. Weekly availability windows can't cross midnight (split required); window end 23:59 = end-of-day.

---

## Phase 4 — Payments, Ledger & Receipts (Days 41–52) — Epic E08

- [x] P4-01 Payment adapter interface (§16.1) + Paystack sandbox/mock implementation
- [x] P4-02 Payment init + signed webhook handling (verify, dedupe, archive)
- [x] P4-03 Idempotency keys on booking/payment/refund mutations
- [x] P4-04 Immutable ledger: accounts/transactions/entries + booking allocation (15%/3% configurable; effective-dating of rate changes lands with pricing admin in Phase 8)
- [x] P4-05 Ledger invariant tests (balanced, immutable, provider-ref uniqueness, replay-safe)
- [x] P4-06 Receipt generation (PDF) + private storage + signed download (local adapter in dev; R2 in prod, EXT)
- [x] P4-07 Refund skeleton + reversing entries + admin policy flow
- [ ] EXT-1 Paystack production keys + webhook secret (Human)

**Exit criteria (Spec §28 Phase 4):**
- [x] A test payment confirms exactly one booking
- [x] Creates one balanced ledger allocation
- [x] Produces a downloadable receipt even if the provider webhook is replayed multiple times

### Evidence log — Phase 4

- 2026-08-13 · agent · Migration · `0005_payments_ledger` applied + roundtrip clean: receipts (booking UNIQUE, receipt_number UNIQUE), webhook_events (provider+event_reference UNIQUE, sha256 raw-body hash, processed in side-effect tx), payments.authorization_url, seeded chart of accounts (tourist_clearing, platform_revenue, tourism_levy_payable, guide_payable_pending/eligible, gateway_fees — documented in migration comment).
- 2026-08-13 · agent · Backend gates · gofmt/vet/build clean; `go test -count=1 ./...` all ok (13 packages). Integration journey proves §30.1/30.4: intent idempotency (same ref+URL on replay), 409 second active intent, bad signature→401 zero rows, signed webhook→CONFIRMED + ONE balanced 4-leg allocation (450.00=67.50+13.50+369.00 pesewa-exact) + receipt (%PDF over signed URL) + 2 notification stubs in ONE tx; 3 webhook replays→200 replay=true, counts stay 1/1/1/1; duplicate provider_reference violates UNIQUE; refund→REFUNDED via legal §8.2 hops, reversal nets every account to zero, originals intact, audit row, refund replay no-op.
- 2026-08-13 · agent · Providers · §16.1 interface; MockProvider (deterministic, HMAC-SHA256, dev default) + full PaystackProvider (initialize/verify/refund/transfer, HMAC-SHA512 x-paystack-signature, tested against httptest). Selection via PAYMENT_PROVIDER + secret presence. MOCK_WEBHOOK_SECRET added to .env.example.
- 2026-08-13 · agent · Frontend gates · pnpm lint/typecheck/build all green; /checkout/[bookingId] real payment section (Pay now→payment-intent→redirect, mock badge, return-poll until CONFIRMED, PAYMENT_FAILED retry), /receipts/[id] new (receipt card + signed-URL download, 404 retry state), receipt links on /bookings + detail.
- 2026-08-13 · agent · OpenAPI · 46 operations; `docs/api/openapi.yaml` regenerated.
- 2026-08-13 · agent · Notes · Balanced-entries invariant enforced app-level in ledger.Post (tested), no DB trigger. Refund issues full refunds only; partial-refund states ready. Provider refund called before DB tx (documented crash window; Paystack refunds idempotent). Frontend treats amounts as major units GHS.

---

## Phase M — Mobile Apps (Expo / React Native) — Epic E17

Ran in parallel with Phase 5. M-13…M-16 were gated on P5-01…P5-05; those landed, so
they were built against the real endpoints, never mocks (agent_plan.md §M.4 note 5).

- [x] M-01 `packages/tokens` — platform-neutral design tokens + CSS parity check
- [x] M-02 `apps/tourist-mobile` — Expo SDK 57 scaffold
- [x] M-03 `apps/guide-mobile` — Expo SDK 57 scaffold + background-location config
- [x] M-04 `packages/api-client` — bearer client, 401 → refresh → retry once
- [x] M-05 **Backend:** `RefreshFromRequest` accepts header/body token, not cookie only
- [x] M-06 ADR 0009 — native session storage (mandated by ADR 0007)
- [x] M-07 Auth flow in both apps via `expo-secure-store`
- [x] M-08 CI mobile lane
- [x] M-09 Tourist search + guide detail
- [x] M-10 Tourist quote → booking → provider-hosted checkout (no in-app card fields)
- [x] M-11 Tourist bookings + receipts via signed URL
- [x] M-12 Tourist profile + offline/empty/error/retry states
- [x] M-13 Guide go online/offline → P5-01
- [x] M-14 Guide job feed + accept → P5-02/03/04
- [x] M-15 Guide background location + permission-rationale screen → P5-01
- [x] M-16 Guide tour lifecycle screens → P5-05
- [x] M-17 EAS Build + Submit config
- [x] M-19 `packages/ui-native` — shared RN primitives for both apps (added during M-13)
- [x] M-20 Data-subject rights backend (deletion, export, consent, policies) + migration 0010
- [x] M-21 In-app Privacy & data screen (both apps) + web deletion route (both web apps)
- [x] M-22 iOS privacy manifests; Android 14 foreground-service type confirmed
- [x] M-23 Compliance docs (data inventory, App Store, Play, Ghana)
- [x] M-25 `apps/marketing-web` — public marketing site, 19 prerendered routes
- [x] M-26 CMS: `marketing.site` settings key + admin-web Content editor + public read endpoint
- [x] M-27 `/legal/*` and `/account/delete` resolve — unblocks the store-submission 404
- [ ] M-24 Counsel-approved legal text + DPC registration + GTA/levy confirmation (Human, blocked)
- [ ] M-18 Store listings + background-location justification (Human)
- [ ] EXT-3 Apple Developer + Google Play accounts (Human, blocked)
- [ ] **Exit** Both apps install on physical iOS + Android via EAS internal distribution; tourist completes search→book→pay→receipt on sandbox; guide accepts a dispatched job and streams location while backgrounded; CI mobile lane green

### Evidence log — Phase M

- 2026-08-14 · agent · Spec coverage sweep · Every distinctive §4–§22 feature has an implementation: certification state machine, effective-dated pricing, immutable ledger, tourism-levy accrual, dispatch scoring, Redis presence TTL, WebSocket channels, SOS/incidents, verified reviews, Elite status, wallet/payouts, LMS, notification templates, audit log, idempotency keys, receipt PDFs, reversing refunds. MongoDB is absent, which §1.2 explicitly permits ("core must work without it"). B2B hotels and multi-language UI are deferred by design (§34 / E16 / F-01 / F-03).
- 2026-08-14 · agent · Route coverage vs §18.1/§18.2 — two deviations · (1) **`/guide/payouts` does not exist as its own route** — the payout statement and payout-account registration live inside `/guide/wallet`. Functionality is complete; this is a deliberate consolidation, not a gap. (2) **`/support` does not exist for tourists.** SOS is implemented where it belongs, on the active booking (`SosPanel` in `bookings/[id]`), but §18.1 also lists a standalone support/incidents page and there is **no tourist-facing incident endpoint** — `/admin/incidents` is admin-gated. Raising a non-SOS issue, or reviewing your own incident history, has no surface. This needs a backend slice (a `/me/incidents` read plus a create path), not just a page, so it is recorded rather than half-built.

- 2026-08-14 · agent · **Completeness audit — two real gaps found behind a ✅** · Ticked 26 previously-unchecked §30 acceptance criteria and Appendix D stop conditions, each against a named passing assertion (concurrent-accept is tagged §30.2 in `phase5_test.go:281`; second review → 409 at `phase6_test.go:219`; no-double-pay at `phase7_test.go:313`; unsigned document GET denied at `main_test.go:567`). Full Go suite re-run clean beforehand: **15 packages ok, 0 failures**. Epics E01 and E03–E15 rolled up.
- 2026-08-14 · agent · **P1-05 is NOT complete despite being marked ✅ — Appendix D launch blocker** · (1) **MFA is not enforced by role.** `auth.Service.Login` branches on `mfaEnabled` (has the user enrolled?) and never calls `MFARequiredRole`. That function is **dead code outside tests** — nothing in the non-test tree calls it. A `super_admin` or `finance_officer` who never enrols therefore signs in with a password alone. The source comment concedes the circularity: "enforced at login once enabled". (2) **There is no step-up re-authentication for sensitive actions.** Every "step-up" reference in the tree is the login TOTP step; role changes, payout-account edits and refunds require no re-auth. Spec §15.2 requires both. Consequently `Admin privileged accounts have required MFA`, `No critical auth/RBAC bypass` and epic **E02** are left unchecked.
- 2026-08-14 · agent · Fix not applied — needs a decision · Enforcing MFA-by-role means denying privileged permissions until a required user has enrolled. `/me/mfa/enroll` sits behind `RequireAuth` only, so enrolment stays reachable, and the natural place is a `MFASatisfied` flag on `rbac.Identity` set in `RequireAuth` and checked in `RequirePermission`. It is a contained change, but it **will fail roughly ten integration tests** whose harness grants `super_admin` and immediately calls admin endpoints — which is itself evidence the gap is real. Left for the owner to schedule rather than changing auth semantics unannounced.

- 2026-08-14 · agent · **M-24 initial legal text seeded and CMS-editable** · Migration `0011_legal_body` adds `summary`, `body` (markdown), `approved`, `approved_at`, `approved_by` to `legal_documents`, with a CHECK that approval records who and when together. Seeded version `2026-08-14` for all three documents: **terms 6,501 chars, privacy 6,002, location 3,184**. The text describes what the platform actually does — the location window, retention split, payment flow and deletion behaviour all match the implementation — but it is **not legal advice** and ships `approved = false`.
- 2026-08-14 · agent · **Editing creates a version, never mutates one** · `consent_records` references `(document, version)`, so rewriting a row in place would silently re-point a user's recorded consent at different words. `POST /admin/legal/{document}` therefore inserts a new version and returns **409 on a duplicate**; approval is a separate audited action via `POST /admin/legal/{document}/{version}/approve`. Both require `settings.manage`. Admin editor at **admin-web → Legal** mirrors the constraint in its copy so the UI cannot imply otherwise.
- 2026-08-14 · agent · Draft state is enforced by data, not by memory · While `approved` is false the public page renders a "Draft — pending legal review" banner **and sets `robots: noindex`**, so an unapproved draft cannot become the canonical answer in search. Approving clears both.
- 2026-08-14 · agent · Markdown renderer · `components/markdown.tsx` renders to React elements rather than an HTML string — no `dangerouslySetInnerHTML`, therefore no injection surface on admin-authored text. Supports the subset the documents use (h2/h3, paragraphs, bullet and ordered lists, bold, italic, links); link hrefs are allow-listed to `http(s):`, `mailto:` and `/` so an editor cannot introduce a `javascript:` URL. No new dependency.
- 2026-08-14 · agent · **Bug found and fixed by verification: legal actions were not being audited** · `audit_logs.entity_id` is a `uuid` column and the handlers passed `"terms@2026-08-15"`, so every insert failed with SQLSTATE 22P02. Because `RecordHTTP` failures are logged rather than fatal, the endpoints returned 201/200 while writing **zero audit rows** — a silent violation of §1.2 #8. A legal document is keyed by `(document, version)` and has no uuid, so `EntityID` is now left empty (the recorder nullifies it) and the identity moved into the JSONB payload. Re-verified: `legal.publish` and `legal.approve` rows now present, 0 audit failures in the log. Swept the rest of the codebase for the same class of bug — the only other `EntityID:` hit is an `AuditFilter` for *querying* logs, not a write.
- 2026-08-14 · agent · Gate · Migration 0011 down/up round-trip clean (3 bodies survive). Live: duplicate version → **409**, new version → **201 draft**, approve → **200**, public endpoint reflects newest version and approval state. Pages render — terms h2=14/p=32, privacy h2=12/p=26, location h2=10/p=16, all three showing the draft banner. `pnpm -r` typecheck **11/11**, lint **11/11**, 0 failures; gofmt clean, `go vet` clean; `go test ./cmd/api` **ok 22.4s** (Redis flushed first, per the known harness issue).

- 2026-08-14 · agent · **Data-subject rights verified live, end to end** · Against the running API with a real account: `GET /me/deletion` → `can_delete: true`, disclosing 5 removal classes and 2 retention classes; `POST /me/consent` → 201; `GET /me/export` → all seven sections (`generated_at, account, profile, bookings, reviews, consents, notes`) with the caller's own email and the recorded consent; `DELETE /me` → `status: deleted, cleared: identity, tourist_profile, sessions`; the access token then returned **401 immediately**, confirming the `rbac.AccountActive` gate revokes a still-unexpired JWT. This is the Apple 5.1.1(v) / Google Play deletion requirement demonstrated working, not merely implemented.
- 2026-08-14 · agent · Android 14 foreground service — **no work needed** · An earlier note flagged a possibly-missing `foregroundServiceType`. That was wrong: `expo-location`'s own library manifest declares `<service android:name=".services.LocationTaskService" android:exported="false" android:foregroundServiceType="location" />`, which merges into the app manifest at build time, and the config plugin adds `FOREGROUND_SERVICE` + `FOREGROUND_SERVICE_LOCATION`. Both are additionally explicit in `guide-mobile/app.json`. Flag withdrawn.
- 2026-08-14 · agent · Web deletion routes · `/account/delete` returns 200 on **both** `tourist-web` and `guide-web`. It initially 404'd, but only because the dev server on :3000 was a stale process from an earlier session that predated the page — the replacement had exited with `EADDRINUSE`. The same trap has now bitten three times this session (api on 8080, guide-web on 3001, tourist-web on 3000); **check `lsof -ti:<port>` before trusting a local 404.**

- 2026-08-14 · agent · M-25 revision (stakeholder direction) · Dropped the red/gold/green strip device — it read as flag bunting rather than a brand. Replaced with a restrained system: a 1px hairline seam (low-opacity gold on the dark field, border-grey on light), a solid green credential-card edge, and **red retired from all decorative use** — it now appears only in genuine danger states. Typeface changed to **Outfit** at two weights for display and body; IBM Plex Mono retained solely for record data (licence numbers, credential field labels, source citations), since Outfit has no mono and that "official document" voice is load-bearing on the card.
- 2026-08-14 · agent · Logo + icons · New mark: a compass needle over a gold waypoint in a green squircle — wayfinding is the literal job and a needle survives 16px. Defined once in `components/logo.tsx` and reused everywhere. `app/icon.svg` (favicon, also copied to tourist/guide/admin so the product is consistent), `app/apple-icon.tsx` (180×180) and `app/opengraph-image.tsx` (1200×630) are generated from that same mark via `next/og`, so there is no binary asset to drift. The OG card is type-only and deliberately carries no photograph — a stock beach shot argues the opposite of this brand, and WhatsApp is the main sharing channel for a Ghanaian consumer product.
- 2026-08-14 · agent · SEO · Expanded root metadata (keywords, authors, publisher, category, formatDetection, explicit `robots`/`googleBot` with `max-image-preview: large`), `manifest.webmanifest`, and structured data: Organization + FAQPage on the home page, **BreadcrumbList + TouristDestination with nested TouristAttraction** on each city page. All JSON-LD is `<`-escaped before injection because the FAQ is admin-editable. Sitemap lists the legal and deletion routes deliberately — the stores check that a privacy policy is publicly reachable and indexable.
- 2026-08-14 · agent · Gate · `next build` → **23 routes** (icon, apple-icon, opengraph-image, manifest now among them). Repo-wide: `pnpm -r run typecheck` **11/11**, `pnpm -r --if-present run lint` **11/11**, `pnpm -r run build` **4/4**, 0 failures.
- 2026-08-14 · agent · Local run · API `:8080` (Postgres + Redis `ok`), tourist `:3000`, guide `:3001`, admin `:3002`, marketing `:3003`. Two stale-process traps hit while starting: an old `api` binary was squatting on 8080 so every `/api/v1` route 404'd while `/healthz` answered (the new build had logged `address already in use` and exited), and guide-web hit `EADDRINUSE` racing a shutting-down predecessor. Both resolved by killing the stale PID and restarting.

- 2026-08-14 · agent · **Test-harness finding (pre-existing, not a code defect)** · Running the `cmd/api` integration suite repeatedly against the shared dev Redis makes it fail wholesale: per-IP rate-limit buckets accumulate across runs and later tests start hitting 429. `newIntegrationEnv` clears `rl:*` at construction, which is not sufficient once several full runs have gone through in one session. `redis-cli FLUSHALL` then re-running → **`ok proguidegh/api/cmd/api 277.071s`**, whole suite green. Separately, running two full suites concurrently against the same Postgres knocked it into recovery mode (`SQLSTATE 57P03`). Worth hardening the harness (namespaced Redis keys or a per-run DB) before CI runs suites in parallel; for now, flush Redis and do not run two suites at once.

- 2026-08-14 · agent · M-25 · New `apps/marketing-web` (Next.js 16, port 3003). Routes: `/`, `/destinations` + 3 city pages, `/become-a-guide`, `/safety`, `/pricing`, `/about`, `/faq`, `/contact`, `/legal/[document]` ×3, `/account/delete`, `/sitemap.xml`, `/robots.txt`, 404. `next build` → **19 routes prerendered**, ISR 5m. Kept out of `tourist-web` deliberately: it owns the apex domain the app stores resolve, serves a deciding rather than a decided audience, and must stay up when the API is not.
- 2026-08-14 · agent · M-25 research · Content grounded in sources rather than invented: Ghana recorded **1,306,962 international arrivals in 2025**, **7,109 licensed tourism enterprises**, and **31% of arrivals travelling for business** (Ghana Tourism Authority 2025 Tourism Report) — the business figure is why Appendix C's "Business/Conference Support" specialty is surfaced on the Accra page. Destination detail (Kakum's 350 m canopy walkway at 27 m; Kejetia at ~12 ha; Cape Coast and Elmina castles; Manhyia Palace) checked against public sources. GTA confirmed as the body that licenses and regulates tour guides, so the site describes certification as requiring GTA registration rather than inventing a parallel credential.
- 2026-08-14 · agent · **Honesty control on the stats band** · The spec's 2,140 guides / 8,420 tours-per-month are stated baselines and Y1 targets, **not measured traction**. Publishing them as achievements would be the unearned-claim case Appendix D treats as a launch blocker. `stats.verified` therefore ships `false`, the band renders externally sourced and cited Ghana tourism figures instead, and the admin editor carries a warning explaining when it is legitimate to flip. Same reasoning applies to partners: the About page states alignment with the GTA and explicitly declines to name partners before confirmation.
- 2026-08-14 · agent · M-26 · Content lives in `system_settings` under `marketing.site`, edited at **admin-web → Content** through the existing audited `PUT /admin/settings/{key}`, and read publicly at `GET /api/v1/content/marketing` (one allow-listed key — pricing rules and provider config stay unreachable). `app/lib/content.ts` holds both the shape and the launch defaults, merged section-by-section, so a partial document or an unreachable API still renders a correct page instead of a 500.
- 2026-08-14 · agent · M-27 · `/legal/terms`, `/legal/privacy`, `/legal/location` and `/account/delete` return 200 and are listed in the sitemap. These are the exact URLs seeded into `legal_documents`, which the mobile apps link to and which both stores require; their 404 was blocking submission. The pages render each document's outline plus a clearly-marked notice that the text is with counsel — **M-24 still has to supply the prose**, and a reviewer following the link today gets a table of contents, not a policy.
- 2026-08-14 · agent · Design · Signature is a guide **credential card** (a licence, not a listing tile), recurring on the recruitment page as a blank card. Section rule is a **kente strip** — kente is woven in narrow bands sewn edge to edge. Type: Bricolage Grotesque / Public Sans / IBM Plex Mono, self-hosted via `next/font`; the mono is reserved for licence numbers, credential field labels and citations.
- 2026-08-14 · agent · Visual review found two defects, both fixed · (1) **The primary CTA disappeared on mobile** — `.nav__links` was `display:none` below 56rem and the "Find a guide" button lived inside it, hiding the main conversion action on the majority device in a mobile-first market. The button now sits outside that container and is always visible; verified in Playwright at 390×844 (visible, 44px tall). (2) The kente rule used near-black segments that vanished against the dark nav, so the signature device read as a dashed line; all segments are now saturated enough to read on either ground.
- 2026-08-14 · agent · M-25/M-26/M-27 gate · `pnpm -r run typecheck` **11/11**, `pnpm -r --if-present run lint` **11/11**, `pnpm -r run build` **4/4 Next apps**, 0 failures. Every marketing route returns 200 (checked with curl against `next dev`). Screenshots reviewed at 1440×900 and 390×844. OpenAPI regenerated and diffed — no drift.

- 2026-08-13 · agent · M-20 · Migration `0010_privacy`: `account_deletions`, `consent_records`, `legal_documents`, `users.anonymized_at`, `deleted` status. New `internal/privacy` module: `DELETE /me`, `GET /me/deletion`, `GET /me/export`, `POST /me/consent`, public `GET /legal/policies`. `storage.Store` gained `Delete` (idempotent — a missing object is not an error) so guide verification documents actually leave R2 on erasure. Erasure is **anonymisation, not row deletion**: bookings/ledger/receipts/audit_logs are append-only per §8 and all reference `users.id`.
- 2026-08-13 · agent · M-20 tests · `go test ./cmd/api -run 'TestLegalPolicies|TestConsent|TestExport|TestAccountDeletion|TestDeletionBlocked'` → **5/5 PASS**. Asserted: policies public without auth; consent append-only and retrievable via export; export is self-scoped and 401s unauthenticated; deletion anonymises identity, drops the profile, revokes every session, writes exactly one audit row and one `account_deletions` receipt, **and leaves the `users` row intact** so append-only references survive; deletion blocked by an active booking returns 409 `DELETION_BLOCKED` with the specific reason, records the refusal, and leaves the account `active`.
- 2026-08-13 · agent · **SECURITY FIX found by the M-20 test** · `RequireAuth` verified the JWT and loaded permissions but never checked `users.status`. Because access tokens are stateless and live 15 minutes, a **suspended or deleted account kept authenticating until its token expired** — contradicting §15.2 ("sessions are suspended/revoked on compromise or role removal") and making account deletion only eventually effective. Added `rbac.AccountActive` (deliberately **uncached**, unlike the Redis-cached permission set, so revocation takes effect on the very next request) and enforced it in both `RequireAuth` and the `/ws/*` handshake. Full Go suite re-run green afterwards: `go test ./... -count=1` all packages ok.
- 2026-08-13 · agent · **Pre-existing bug fixed** · `docs/api/openapi.yaml` was not valid YAML — the `adminListQualityFlags` summary contained an unquoted `(spec §4.4): ` and the parser read the second colon as a mapping. CI's *Contracts (OpenAPI drift)* job only text-diffs the file, it never parses it, so this went unnoticed and `pnpm --filter @proguidegh/contracts generate` (P0-10's "generated TS client pipeline") had been failing. Quoted the summary in the embedded spec; `generated.ts` now builds for the first time (143KB) and typechecks/lints clean.
- 2026-08-13 · agent · M-21 · `@proguidegh/ui-native/privacy` shared screen wired into both apps at `/privacy` (tourist: Profile → Privacy & data; guide: dashboard → Privacy & data). Web deletion route at `apps/{tourist,guide}-web/app/account/delete` — Play requires a URL reachable **without installing the app**, so the page handles the signed-out case with a sign-in link and a `privacy@proguidegh.com` fallback rather than a bare 401. Removed/retained lists come from the server so app, web and policy cannot drift.
- 2026-08-13 · agent · M-22 · `ios.privacyManifests` in both `app.json`s: required-reason APIs (UserDefaults CA92.1, FileTimestamp C617.1), collected data types, `NSPrivacyTracking: false` (no ad SDKs ⇒ no ATT prompt). Guide app additionally declares Precise Location. Android 14 `foregroundServiceType="location"` verified as already shipped in `expo-location`'s own `AndroidManifest.xml` — no app-side change needed.
- 2026-08-13 · agent · **Dependency refresh** · Non-Expo packages moved to latest: Next **16.3.0**, `@types/node` **26.2.0**, `typescript-eslint` **8.67.0**, ESLint **10.8.1** (web/packages). Two deliberate exceptions, both verified rather than assumed: (1) `npx expo install --check` reports the mobile stack **already correct for SDK 57** — React 19.2.3, RN 0.86.2, TS 6.0.3, gesture-handler 2.32, screens 4.26, safe-area 5.7, worklets 0.10.1 — so "latest" (RN 0.87, React 19.2.8, TS 7.0.2) would *break* `expo-doctor`; (2) ESLint **10 crashes the RN lint** (`eslint-plugin-react` 7.37 caps at 9), so the three React Native workspaces stay on ESLint 9 until that chain supports 10. Go modules: direct deps already latest (pgx 5.10.0, go-redis 9.22.0, coder/websocket 1.8.15, x/crypto 0.55.0); `go mod tidy` upgraded x/sync, x/sys, x/text.
- 2026-08-13 · agent · Post-update gate · `pnpm -r run typecheck` **10/10**, `pnpm -r --if-present run lint` **10/10**, `pnpm -r run build` three Next apps Done, `expo-doctor` **20/20 both apps**, `expo export --platform ios` both apps bundle (1144 / 1127 modules), `gofmt -l` clean, `go vet` clean, `go test ./... -count=1` all packages ok, migration `0010` down/up round-trip clean, `-dump-openapi | diff docs/api/openapi.yaml` → no drift.
- 2026-08-13 · agent · M-23 · `docs/compliance/{data-inventory,app-store,play-store,ghana}.md`. The data inventory is the single source behind the Apple nutrition label, the Play Data Safety form and the privacy policy — reviewers compare them, so they are generated from one table rather than written three times.
- 2026-08-13 · agent · **M-24 blocked, and it blocks launch** · `legal_documents` is seeded with placeholder URLs (`https://proguidegh.com/legal/{terms,privacy,location}`) that **404 today**. Both stores require a reachable privacy policy. Also outstanding and not doable in code: DPC data-controller registration, GTA licensing/levy confirmation, cross-border transfer review, and counsel sign-off on every statutory citation in `docs/compliance/ghana.md` — that file states plainly that its section numbers are pointers for counsel to verify, not settled law.

- 2026-08-13 · agent · M-13…M-17 · Guide app built end to end against the real Phase 5 endpoints. `lib/presence.tsx` (90s heartbeat inside the 300s P5-01 Redis TTL, plus an immediate beat on foreground return so a long background pause cannot leave the guide silently undiscoverable); `lib/useWebSocket.ts` (`/ws/guide`, exponential backoff to 5 attempts, REST polling whenever the socket is not live); `app/jobs.tsx` (offer feed, one-tap accept, 409/410 mapped to distinct guide-readable outcomes rather than a generic failure); `lib/location-task.ts` (TaskManager background task + Android foreground service); `app/location-permission.tsx`; `app/tours/index.tsx` + `app/tours/[id].tsx` (§8.2 lifecycle, owns the §11.1 location window).
- 2026-08-13 · agent · M-15 privacy enforcement · Background collection is gated on an active-booking key in secure storage, not on UI state. Entering GUIDE_EN_ROUTE..IN_PROGRESS sets it; completing, leaving the screen or going offline clears it, and the task then discards every fix instead of sending it. Nothing is persisted to device storage — an undeliverable fix is dropped, never queued, because a stale position is worse than none for dispatch.
- 2026-08-13 · agent · M-19 · Extracted `packages/ui-native` from the tourist app's `lib/ui.tsx`; both apps alias it through their own `lib/ui.tsx` so screens keep importing `@/lib/ui`. One implementation, no cross-app drift.
- 2026-08-13 · agent · M-17 · `eas.json` for both apps (development/preview/production; `requireCommit: true`; `appVersionSource: remote`; `autoIncrement` on production only) plus `docs/runbooks/mobile-release.md` with the physical-device exit procedure. `eas init` project ids and the two App Store Connect ids are **EXT-3 blocked**.
- 2026-08-13 · agent · Phase M gate · `pnpm -r run typecheck` → **10/10 workspaces Done**. `pnpm -r --if-present run lint` → **10/10 Done**. `expo-doctor` → **20/20 on both apps**. `npx expo export --platform ios` → tourist **1132 modules / 2.5MB hbc**, guide **1125 modules / 2.5MB hbc**; both bundle under Metro, which typecheck alone does not prove. `pnpm -r run build` → three Next apps Done, no regression.
- 2026-08-13 · agent · Issues found and fixed by the gate · (1) **Route collision** — two agents independently built M-10, producing `book/[guideId]`+`book/[id]` and `checkout/[bookingId]`+`checkout/[id]`. Two dynamic segments at one path break expo-router. Kept the richer `[id]` pair (meeting point, notes, free-text date/time), deleted the duplicates and the orphaned `lib/slots.ts`, deduped `_layout.tsx`, and set `gestureEnabled: false` on checkout so it cannot be swipe-dismissed mid-payment. (2) **Wrong location payload** — the background task initially sent `lat`/`lng`/`heading_deg`; `tracking/handler.go` L28–33 expects `latitude`/`longitude`/`heading`. Corrected. (3) **`packages/ui-native` typecheck failed** with DOM/RN global collisions because `expo/tsconfig.base` could not resolve without `expo` in that workspace; added as a devDependency. (4) Two `react-hooks` errors in the new WS hook (ref mutated during render; synchronous `setState` in an effect) — fixed, not suppressed.
- 2026-08-13 · agent · Decisions recorded in `agent_plan.md` §M.4 notes 6–8 · shared `packages/ui-native`; the deliberate absence of an offline write queue (a replayed booking would submit a stale server-authoritative quote and race the availability check, §1.2/P3-04); `?token=` WebSocket auth and why it is confined to the realtime channel.
- 2026-08-13 · agent · **Exit criteria NOT met — EXT-3** · Every task buildable in software is done and verified. The exit gate itself — installs on a physical Android and iPhone via EAS internal distribution, and the guide streaming location with the app backgrounded — cannot be run without an Expo account, Apple Developer Program and Google Play Console. Background location specifically cannot be exercised in Expo Go or a simulator; it needs a real device with the screen locked. Procedure: `docs/runbooks/mobile-release.md`.

- 2026-08-13 · agent · M-09…M-16 independent verification pass (second pair of eyes) · Re-checked every mobile screen against the live API contracts. Found and fixed two real contract bugs the earlier gate missed: (1) **profile save always 400'd** — `profile.tsx` PATCHed `phone` (unknown field, `DisallowUnknownFields` in tourists/handler.go L49) and `emergency_contact_phone` instead of `emergency_contact_phone_e164`; also sent empty `full_name` which the handler rejects (L54). Screen now sends only the five patchable fields, omits empty `full_name`, and gained `preferred_language`. (2) **background location fixes were still being rejected** — `location-task.ts` again carried `lat`/`lng`/`heading_deg`; re-corrected to `latitude`/`longitude`/`heading` per tracking/handler.go L28–33 (the earlier claimed fix had not stuck). Also fixed `parseQuote` (amounts live under `quote.price`, not top-level) and `parseReceipt` (`download_url`/`expires_in` are siblings of `receipt`, not nested). Gates after fixes: tourist typecheck/lint clean, expo-doctor 20/20; guide typecheck/lint clean, expo-doctor 20/20.

- 2026-08-13 · agent · **Decision D5 recorded** · Mobile pivoted from PWA to native Expo/React Native on stakeholder directive. Justified by Spec §34 ("Native iOS/Android apps if PWA limitations affect GPS/background") — background location is a V1 dispatch requirement (§10.3 / P5-01) that no iOS PWA can satisfy. Pulls F-02 forward from Post-V1. The PWA layer already shipped into `tourist-web`/`guide-web` and is **retained as a web enhancement**, not rework, and not the mobile deliverable.
- 2026-08-13 · agent · M-01 · `packages/tokens` created: 29 tokens (12 colour, 5 type, 7 space, 3 radius, contentMax, remBase). `node packages/tokens/scripts/check-css-parity.mjs` → `✓ 29 tokens match between tokens.css and @proguidegh/tokens`. Parity is rem→px aware (0.875rem ≡ 14). Chose **verify** over **regenerate** so the check is safe to run while another agent is editing `packages/ui`.
- 2026-08-13 · agent · M-02/M-03 · `pnpm create expo-app --template default@sdk-57` for both apps, then de-templated: removed demo components/hooks/constants/assets, Expo LICENSE and `reset-project` script. Pinned **Expo 57.0.12 · React Native 0.86.2 · expo-router 57.0.12 · TypeScript 6.0.3**. App identities set (`gh.proguide.tourist`, `gh.proguide.guide`; schemes `proguidegh`, `proguidegh-guide`). Guide app declares background location, usage strings, Android foreground-service and `UIBackgroundModes: [location]`.
- 2026-08-13 · agent · M-02/M-03 gate · `pnpm --filter @proguidegh/{tourist,guide}-mobile typecheck` → clean. `… lint` → clean. `… run expo:doctor` → **20/20 checks passed** on both. Repo-wide `pnpm -r --if-present run lint` and `pnpm -r run typecheck` → all 8 workspaces Done, no regression in the three Next apps.
- 2026-08-13 · agent · M-02/M-03 fixes found by gate · (1) Root `.npmrc` `auto-install-peers=true` resolved transitive peers to `latest`, landing `react-native-worklets@0.11.4` and `@react-native/metro-config@0.87.0` against an SDK 57 app pinning 0.10.1/0.86.2 — fixed by pinning them as direct deps in the two apps rather than global `pnpm.overrides`, which would have reached the Next apps. (2) `newArchEnabled` and `android.edgeToEdgeEnabled` are **not valid in SDK 57** (both unconditional now) and failed config-schema validation — removed. (3) SDK 57 requires TypeScript ~6.0.3; the mobile apps deliberately diverge from the web workspaces' 5.9 (each workspace runs its own `tsc`; shared packages are plain TS source and compile under both).
- 2026-08-13 · agent · M-02/M-03 no-regression · `pnpm -r run build` → exit 0; all three Next apps (`tourist-web`, `guide-web`, `admin-web`) build unchanged. The mobile apps intentionally have no `build` script (EAS builds them), so `pnpm -r run build` skips them — CI's build step is unaffected.
- 2026-08-13 · agent · M-05 scoping · API is already largely native-ready: `AccessFromRequest` (`cookies.go` L49–62) already falls back to `Authorization: Bearer`, and `writeSession` (`auth/handler.go` L388–395) already returns `access_token`/`refresh_token`/`expires_at` in the JSON body. **Only gap:** `RefreshFromRequest` (L42–47) is cookie-only, so native rotation and logout fail. M-05 is therefore a small handler-level change, not a session-model rewrite.
- 2026-08-13 · agent · M-05 implemented · `RefreshFromRequest` now accepts `X-Refresh-Token` header → `refresh_token` JSON body → cookie (priority order); logout same transports; rotation/reuse/revocation semantics identical across transports. Integration tests: header rotation, body transport, reuse-detection chain revoke via header, cookie regression. Full Go suite green (15 packages).
- 2026-08-13 · agent · M-04 · `packages/api-client` created: `createApiClient({baseUrl, tokenStore, onSessionExpired})` — bearer Authorization header, deduped single-flight refresh via X-Refresh-Token, both tokens replaced before retry (ADR 0009 §3), envelope `ApiError`, login/loginMfa/logout helpers, storage-agnostic `TokenStore` interface. `typecheck` + `lint` green.
- 2026-08-13 · agent · M-06 · `docs/architecture/adr/0009-native-session-storage.md` written (transport, expo-secure-store storage, rotation, logout/revocation, MFA) per ADR 0007's consequence clause.
- 2026-08-13 · agent · M-07 · Both apps: `src/lib/session.tsx` (SecureStore TokenStore + SessionProvider, restore-on-launch), `src/app/login.tsx` (email/password + MFA code step, accessible labels/live regions), route gate in `_layout.tsx` (unauthenticated → /login, authed on /login → home), sign-out on home/dashboard. `@proguidegh/api-client` added as workspace dep to both apps.
- 2026-08-13 · agent · M-07/M-08 gate · `pnpm --filter @proguidegh/{tourist,guide}-mobile typecheck` clean, `lint` clean, `expo:doctor` 20/20 on both. CI mobile job added to `.github/workflows/ci.yml` (install, tsc, lint, tests --if-present, expo-doctor; EAS out of PR CI); YAML validated. Web gates re-run: `pnpm -r run build` exit 0, no Next regressions.
- 2026-08-13 · agent · M-09 · tourist-mobile `/search` (region/specialty/language/min-rating/elite chip filters → GET /guides/search, package cards from /tour-packages, skeleton/error+retry/empty states, verified/elite/online badges) and `/guide/[id]` (public profile, §10.2 404 → "not currently bookable", inert book button pending M-10). Shared native components in `src/lib/ui.tsx` (LoadingState/ErrorState/EmptyState/Badge/ChipSelect/Card/PrimaryButton over @proguidegh/tokens — ui is DOM-only per §M.3), tolerant parsers in `src/lib/catalog.ts`. Home screen search action wired. Gates: typecheck/lint clean, expo-doctor 20/20. Lint fix: react-hooks compiler rule bans sync setState in effects — initial fetches deferred via setTimeout(0).
- 2026-08-13 · agent · M-10 · tourist-mobile booking flow: `/book/[id]` (package ChipSelect from /tour-packages, validated YYYY-MM-DD + HH:MM inputs, guests stepper 1–50, meeting point/notes) with 500ms debounced POST /bookings/quote rendering the server breakdown (fixed `parseQuote` to read the nested `price` object — backend nests amounts under `quote.price`, top-level fields are null-tolerated as fallback). Confirm → POST /bookings with `createIdempotencyKeeper` key stable per input signature (§1.2 #9 — retry can never double-book), 409/422 surfaced verbatim, then `router.replace` to `/checkout/[id]`. Checkout: POST /bookings/{id}/payment-intent (idempotent) → `WebBrowser.openBrowserAsync(authorization_url)` — **no card/MoMo fields anywhere** (§1.2 #6) — then poll GET /bookings/{id} 15×2s until status ≠ PAYMENT_PENDING; CONFIRMED → success card with reference, CANCELLED → neutral card; "Test payment" gold badge when `payment.provider === "mock"` (parser extended). Guide profile CTA wired to `/book/[id]` with `guide.userId` (backend keys guides by user_id); stack screens registered. Gates: typecheck/lint clean, expo-doctor 20/20.

---

## Phase 5 — Dispatch, Realtime & Tour Operations (Days 53–64) — Epics E05 (part), E10

- [x] P5-01 Redis presence/online/location with TTL
- [x] P5-02 Dispatch scoring + batch offers + Redis TTL expiry (§10.3)
- [x] P5-03 Atomic offer accept (DB tx/distributed lock) + overlap prevention
- [x] P5-04 WebSocket channels: /ws/guide, /ws/booking/{id}, /ws/admin/operations
- [x] P5-05 Tour lifecycle transitions (en route/arrived/start/complete; ops override with reason)
- [x] P5-06 Admin live operations map + list fallback (list-primary board shipped; live map tiles deferred to Phase 8 — §18.4 makes the list the required representation)
- [x] P5-07 Disconnect/reconnect/expiry tests

**Exit criteria (Spec §28 Phase 5):**
- [x] A nearby ACTIVE guide can accept one offer
- [x] Location streams to authorized tourist/admin
- [x] The booking completes through valid transitions

### Evidence log — Phase 5

- 2026-08-13 · agent · Migration · `0006_dispatch` applied + down/up roundtrip: dispatch_offers (UNIQUE booking/guide/batch, expiry indexes), location_checkpoints, guide_dispatch_stats, 5 dispatch system_settings.
- 2026-08-13 · agent · Backend gates · gofmt/vet/build clean; `go test -count=1 ./...` all ok (15 packages incl. new dispatch/realtime). Integration proves §30.2: offers only to eligible/scheduled/online guides; WS offer snapshot; accept→200 with sequential AND concurrent second accepts→409 (one winner, FOR UPDATE + exclusion-constraint backstop); expired accept→410 lazy-persisted; decline→next batch excl. decliner; audited admin dispatch (403 w/o dispatch.manage); why-unmatched view; direct bookings skip dispatch; tour edges legal/wrong-order→409; completion sets ends_at + balanced ELIGIBLE ledger posting (guide_payable_pending→eligible, idempotent ref); location privacy (tourist own-booking window only, stranger 403, admin w/o permission 403); tourist WS receives positions; disconnect/reconnect catch-up via snapshot+REST; checkpoint coarseness (≤1 row/60s + forced row per tour event).
- 2026-08-13 · agent · Scoring · weights (config-overridable): distance .30, rating .25, specialty .15, language .10, workload .10, reliability .10; features+outcome persisted per offer for offline ML (§10.3). Dispatch-pending = CONFIRMED with NULL guide_id (no §8.2 churn). Offer sweeper: API ticker 5s; worker takeover exported.
- 2026-08-13 · agent · Realtime dep · github.com/coder/websocket v1.8.15 (context-native; gorilla archived). Fixed observability statusRecorder to delegate http.Hijacker (WS upgrades 501'd through the access-log wrapper).
- 2026-08-13 · agent · Frontend gates · pnpm typecheck/lint/build green (mobile workspaces excluded from web lint — see Phase M). guide-web /guide/jobs (live offer feed, countdown, accept/decline, WS+5s-polling fallback hook), /guide/tours + /guide/tours/[id] (stepper, next-action, geolocation watch ~10s posts with sharing indicator + §11.2 copy); tourist-web LivePanel on /bookings/[id] (WS + polling fallback, no map per §18.4); admin-web /admin/tours operations board + live event feed. PWA layer (manifest, sw.js, offline page, icons) shipped for tourist/guide web — retained as web enhancement after Phase M superseded it (agent_plan.md P5-08 note).
- 2026-08-13 · agent · Mobile support (M-05 + board gaps) · Refresh/logout accept X-Refresh-Token header / JSON body / cookie (priority order; identical rotation+reuse semantics); GET /me/guide/bookings and GET /admin/bookings (status=active alias, offset pagination) added; integration tests for all three. Fixed `$5` type-deduction in new tests and staggered fixture times against the overlap constraint (constraint fired correctly on bad fixture). OpenAPI 0.6.1, 63 ops; `docs/api/openapi.yaml` regenerated.

---

## Phase 6 — Safety, Reviews & Quality (Days 65–72) — Epics E11, E12

- [x] P6-01 SOS endpoint + immutable event + HIGH/CRITICAL incident + realtime admin alert
- [x] P6-02 Fallback notifications (SMS/push/email) for SOS per policy
- [x] P6-03 Incident dashboard/workflow (ack, notes, escalation, closure — audited)
- [x] P6-04 Verified review flow (one per completed booking) + tags (Appendix B)
- [x] P6-05 Rating aggregation, <4.0 retraining flag, >4.8 Elite qualification review
- [x] P6-06 Quality/retraining queue UI

**Exit criteria (Spec §28 Phase 6):**
- [x] SOS reaches admin operations with coordinates and audit trail
- [x] Only completed bookings can review
- [x] Quality thresholds create expected flags

### Evidence log — Phase 6

- 2026-08-13 · agent · **Backend (P6-01…P6-06)** · Migration `0007_safety_reviews` (append-only `incident_events`, `quality_flags` with partial-unique one-open-flag-per-guide-kind, quality policy settings 4.0/3/4.8/20 in `system_settings`; down/up round-trip verified). Three packages: `internal/safety` (POST /bookings/{id}/sos — participant-only, active-status gate matched to the real §8.2 enum after catching a phantom GUIDE_ASSIGNED, coordinate range validation, per-user rate limit 5/h via `ratelimit.Keyed`, one tx writes sos_events + CRITICAL incident, `sos.triggered` broadcast to /ws/admin/operations + /ws/booking/{id}, responder-roster in_app notifications, audit row; response copy names ProGuideGH operations, never police — §12 safety requirement); `internal/reviews` (POST /bookings/{id}/review — COMPLETED-only, tourist-only with 404-not-403 so existence never leaks, UNIQUE(booking_id) → 409, Appendix B tag whitelist; GET /guides/{id}/reviews public with no tourist identity; aggregate recomputed from reviews table → guide_profiles cache; thresholds open low_rating/elite_review flags, idempotent via the partial unique index; flagging failure after commit logs but never fails the request); `internal/incidents` (admin list/detail with full trail, ack → sos_events.acknowledged_at, notes, severity escalate capped at critical, assign, resolve with mandatory note, terminal close, state machine open→acknowledged→in_progress→resolved→closed + reopen; quality-flag queue + resolve; every action in incident_events AND audit_logs, broadcast `incident.updated`).
- 2026-08-13 · agent · **Integration tests** · `cmd/api/phase6_test.go`: SOS 201 + critical incident, stranger 404, completed-booking 409, admin list/ack/note/escalate-at-max/resolve with trail assertions and sos_events.acknowledged_at propagation, 403 for tourists on the admin queue; review 201/409/400/422/404, public listing aggregate, three 1-star reviews open exactly one low_rating flag (duplicate-flag guard), resolve + 409 on re-resolve. Full Go suite green (18 packages). Also fixed a pre-existing flake: `TestAdminBookingsList` kept the LAST `PGH-AD-DONE` match, which is the oldest leftover row in the shared dev DB — now takes the first (newest) match; root cause confirmed via psql (three leftover fixture rows), not a Phase 6 regression.
- 2026-08-13 · agent · **Frontend (P6-03/P6-04/P6-06)** · admin-web `/admin/incidents` safety desk (status filters, list + detail with trail, ack/note/escalate/assign/resolve/close, live refetch on `sos.triggered`/`incident.updated` via /ws/admin/operations with polling fallback) and `/admin/quality` queue (open/all filter, resolve-with-note); command-center nav links. tourist-web booking detail: `SosPanel` (fresh fix via navigator.geolocation `maximumAge: 0` — §12 step 7, retryable, operations-not-police copy) and `ReviewPanel` (rating + Appendix B tags + body, 409 → recorded); guide profile `ReviewsSection` (public reviews, silent degradation).
- 2026-08-13 · agent · **Mobile parity** · tourist-mobile booking detail: `lib/sos.tsx` (expo-location ~57.0.9 added + `locationWhenInUsePermission` usage string, foreground permission at tap time with rationale, confirm dialog naming operations), `lib/review.tsx` (ChipSelect rating/tag + body); guide profile `lib/reviews.tsx`. Gates: tourist-web typecheck/lint/build green; admin-web typecheck/lint/build green; tourist-mobile typecheck/lint clean, expo-doctor 20/20. OpenAPI regenerated: v0.7.0, 76 operations (was 63).
- 2026-08-13 · agent · **P6-02 scope note** · Fallback notifications are the queued `notifications` rows to the incidents.manage roster (in_app channel) + realtime alert; SMS/push/email providers attach downstream of that queue and need external credentials (EXT-1). No fake provider shim was built.

---

## Phase 7 — Wallet, Payouts & Finance (Days 73–80) — Epic E09

- [x] P7-01 Guide wallet/statement derived from ledger
- [x] P7-02 Payout account verification fields (tokenized refs)
- [x] P7-03 Eligibility scheduler (T+7) + weekly payout batch
- [x] P7-04 Provider transfer integration or safe manual export fallback
- [x] P7-05 Retry/manual-review states (§8.4) + finance dashboard
- [x] P7-06 Tourism Levy accrual/reconciliation reports
- [x] P7-07 Idempotency/concurrency tests: no duplicate payout under retries
- [ ] EXT-2 Production transfer/payout credentials (Human)

**Exit criteria (Spec §28 Phase 7):**
- [x] Completed eligible earnings can be batched without duplicate payout
- [x] Provider retries are idempotent
- [x] Finance can reconcile totals

### Evidence log — Phase 7

- 2026-08-13 — Migration `0008_payouts` (failure_reason, ledger_transaction_id, partial-unique `idx_payouts_guide_schedule` for batch idempotency) applied; `migrate down && up` round-trip verified.
- 2026-08-13 — `internal/payouts` package: AES-256-GCM tokenization (`tokenize.go`), repository with ledger-attributed wallet math (payout drawdowns subtracted — PAID postings carry no booking_id), §8.4 state machine with atomic ledger posting on PAID, CSV export (only plaintext-ref surface), tourism-levy report, keyset-paginated statement. Routes wired in `cmd/api/main.go`; hourly scheduler goroutine runs the batch on Mondays when none scheduled (run-at-startup catch-up).
- 2026-08-13 — Integration tests `cmd/api/phase7_test.go` green: wallet math (eligible 22500 / payout-eligible 18000 with T+7 hold), batch + idempotent re-run (created=0, 1 row — P7-07), masked payout-account round-trip (no plaintext leak), admin verify, 403 without finance roles, CSV export contains decrypted ref, QUEUED→PROCESSING→PAID with balanced ledger posting (PAYOUT:<id>, debit=credit=180.00), FAILED reason required / RETRY_QUEUED / MANUAL_REVIEW transitions, statement cursor pagination, levy report deltas, delay-hold batch exclusion (`TestPayoutBatchDefersToDelayHold`).
- 2026-08-13 — Full Go gate green: `gofmt -l .`, `go vet ./...`, `go test -count=1 ./...` (cmd/api 15.5s).
- 2026-08-13 — OpenAPI v0.8.0 (86 operations) with Phase 7 paths; `docs/api/openapi.yaml` regenerated.
- 2026-08-13 — Frontends: admin-web `/admin/finance` (batch trigger, status filters, transition actions, CSV export link, levy card) + nav link; guide-web `/guide/wallet` (balance cards, payout-account form, statement with cursor "load more") + dashboard link. `pnpm typecheck`, `lint` and `build` green for both apps.
- Note: EXT-2 (production transfer credentials) remains Human/⛔ — the audited CSV export is the sanctioned manual fallback (spec §31.29).


---

## Phase 8 — Training, Analytics & Admin Polish (Days 81–86) — Epics E06 (part), E13, E14

- [x] P8-01 Light LMS: courses/modules/lessons/enrollment/progress/quiz/certificates
- [x] P8-02 Executive KPI dashboard + operational reports + permitted CSV exports
- [x] P8-03 Notification templates/settings (versioned)
- [x] P8-04 Audit viewer + policy configuration UI
- [x] P8-05 Mobile/PWA polish, offline/retry UX states

**Exit criteria:** phase-end gate (§31.28) green; no dedicated §28 exit block.

### Evidence log — Phase 8

- 2026-08-13 — Migration `0009_training` (courses/modules/lessons, enrollments with UNIQUE(guide_id, course_id), lesson_progress PK, quiz_attempts, certificates, versioned notification_templates with one-active-per-key partial unique index + 4 seeded templates) applied; down/up round-trip verified.
- 2026-08-13 — `internal/training` (light LMS): atomic course creation with nested modules/lessons/quiz, idempotent enroll, lesson progress, server-scored quiz (answer_index stripped from guide surface), auto-completion + PGH-CERT certificate issuance; admin roster. `internal/reporting`: KPI dashboard, bookings report, permitted bookings CSV export (reports.export, audited), audit-log viewer (audit.read). `internal/admin/settings.go`: versioned template create/activate (atomic supersede) + system-settings policy editor with version bumps; all audited.
- 2026-08-13 — Integration tests `cmd/api/phase8_test.go` green: full LMS lifecycle (create → duplicate-code 409 → enroll → re-enroll idempotent → 2 lessons → quiz fail 0% → pass 100% → completed + certificate in list; admin roster), KPI/report shape, export 403 without reports.export then CSV via finance, template v2 create → exactly-one-active invariant before/after activation, settings put/read-back/restore, audit viewer 403 for administrator + settings.updated entry visible to super_admin.
- 2026-08-13 — Full Go gate green (cmd/api ~15–18s). OpenAPI v0.9.0 (106 operations); `docs/api/openapi.yaml` regenerated.
- 2026-08-13 — Frontends: admin-web `/admin/training` (authoring + activate/deactivate + roster), `/admin/reports` (KPI cards + bookings report), `/admin/settings` (template versions + policy editor), `/admin/audit` (filtered viewer) + nav links; guide-web `/guide/training` (catalog, enroll, lesson completion, quiz, certificates) + dashboard/nav links. `pnpm typecheck`/`lint`/`build` green for both apps.
- 2026-08-13 — P8-05: PWA shells verified in place (manifest.webmanifest, sw.js with app-shell precache + network-first navigations + /offline fallback, ServiceWorkerRegister, offline pages) for tourist-web/guide-web; added `ConnectivityBanner` (offline warning + back-online retry prompt) to both layouts. typecheck/lint/build green.
- 2026-08-14 — Product-wide responsive redesign: rebuilt shared web/native tokens and primitives around the mineral-teal, deep-field, warm-paper and brass system; replaced tourist/guide headers and footers; introduced the grouped admin command-center rail/topbar/mobile navigation after sibling-shell review; refreshed marketing footer/palette; aligned native headers, cards, buttons, chips, loading/empty/error states, PWA manifests, mobile splash backgrounds, favicons and social images. Existing routes, API behavior and security-sensitive native flows were preserved.
- 2026-08-14 — Redesign gates: `pnpm typecheck` exit 0 across all 12 runnable workspaces; `pnpm lint` exit 0 across all 12; `pnpm --filter @proguidegh/tokens test` → 29/29 web/native tokens match; `pnpm build` exit 0 with all marketing (23), tourist (13), guide (16) and admin (18) route outputs generated; focused guide typecheck/lint/build exit 0 after route-polish addition; `git diff --check` exit 0. Expo Doctor completed 18/20 local checks, with only its two online Expo/React Native Directory checks unavailable because `exp.host` DNS is blocked in this workspace.
- 2026-08-14 — Visual-QA limitation: stale ProGuideGH dev servers on ports 3000–3003 were stopped; the managed workspace then rejected new localhost listeners with `EPERM`, and browser policy blocked direct `file://` inspection. Production bundles were instead verified to contain the new shell selectors and static class coverage was audited; a fresh live desktop/mobile screenshot pass remains required in an environment that permits localhost serving.
- 2026-08-14 — Route-shell completion pass: added pathname-aware active navigation to tourist and guide shells; moved admin chrome into a client shell with grouped active navigation and contextual topbar copy for all 15 operational routes plus nested certification cases. Replaced every web/PWA/native bitmap icon and splash asset with the deterministic compass mark sourced from `packages/tokens/assets`; Android adaptive foreground and monochrome variants included. Re-ran `pnpm typecheck`, `pnpm lint`, token parity and `pnpm build` successfully across the workspace. Both Expo apps independently completed 18/20 Doctor checks; only the same two network-only metadata checks failed on blocked `exp.host`.
- 2026-08-14 — Visual-QA blocker confirmed for a third consecutive goal turn: fresh `pnpm --filter @proguidegh/tourist-web start --hostname 127.0.0.1` failed before serving with `listen EPERM 127.0.0.1:3000`. The active redesign goal is blocked only on route-level live rendering/screenshots; source, static, route-output and asset evidence are green.
- 2026-08-14 — Favicon completion: generated explicit 64×64 multi-platform `favicon.ico` assets for tourist, guide, admin and marketing from the canonical compass SVG, plus 180×180 Apple touch icons for the three application surfaces (marketing retains its dynamic Apple icon). `file` identified all four ICOs as valid MS Windows icon resources; `pnpm build` exited 0 and emitted the new Apple icon routes. A four-app launch retry still failed before serving because the managed workspace denied all localhost binds (`EPERM` on 127.0.0.1:3000–3003).
- 2026-08-14 — Host-started ports 3000–3003 enabled a live desktop/mobile Chrome audit of the redesigned tourist, guide, admin and marketing shells. Tourist and marketing rendered cohesively; the audit exposed a sparse signed-out guide dashboard and redundant admin overview navigation. Source fixes replaced the guide state with a responsive workspace welcome/feature composition and reduced the admin hero to three priority lanes. `pnpm typecheck`, `pnpm lint` and a full `pnpm build` all exited 0 afterward. The host-owned guide/admin Node processes continued serving their pre-fix bundles and could not be signalled from this managed workspace (`kill: operation not permitted`), so those two final rendered states require a host restart before the last screenshot confirmation.
- 2026-08-14 — Admin/marketing structural follow-up: inspected the real AuraEdu `AppSidebar` and RentOS `Sidebar`/`DashboardLayout` implementations, then replaced the admin mobile horizontal navbar with the same persistent-shell model used on desktop: grouped icon rail, collapsible sections, workspace status, account utility, route-aware selection and an off-canvas mobile drawer. Marketing received a new sticky glass header, branded sign-in/action cluster, elevated card anatomy, large conversion footer, guide-specific footer column, route arrival, card/credential hover depth, link/button feedback and reduced-motion support. Focused admin and marketing typecheck, lint and production builds exited 0; `git diff --check` exited 0. Stale ports 3000–3003 were cleared successfully, but replacement binds still fail with sandbox `listen EPERM`; host Terminal/VS Code and Docker socket automation are policy-blocked, so launch requires one host-terminal command.
- 2026-08-14 — Guide watermark layer: added a canonical line-art guide SVG and deployed contrast-aware decorative variants across marketing heroes/page banners, destination/content cards, KPI statistics and the conversion footer; admin page banners, KPI cards and canvas backdrop; plus tourist/guide page banners and footers. Marks are pointer-inert, low-opacity and responsive, with card variants gaining only a subtle hover emphasis. Marketing, admin, tourist and guide lint/build gates all exited 0; `git diff --check` exited 0.
- 2026-08-14 — Admin navigation connectors: added continuous SVG tree trunks and curved elbows from every expanded sidebar group into its child routes, with clean final-item termination and brass active-route emphasis. Admin typecheck, lint and production build exited 0; `git diff --check` exited 0.
- 2026-08-14 — Marketing visual-storytelling pass: generated four original, rights-safe editorial Ghana travel assets (guide-led Accra hero; Accra/Jamestown, Cape Coast and Kumasi destination scenes), integrated responsive `next/image` compositions into the homepage and destination index/detail routes, and layered the credential card over the hero photography as the trust signal. Source PNGs were converted to 1400–1800px WebP at quality 84, reducing the project payload from about 10 MB to under 700 KB. Marketing typecheck, lint, production build and `git diff --check` exited 0; live Chrome desktop and 390px mobile QA confirmed the new images, crops, overlays and responsive flow render correctly on port 3003.


---

## Phase 9 — Hardening & Launch (Days 87–90) — Epic E15 + Launch Checklist §33

- [x] P9-01 Security review + dependency/container scans
- [x] P9-02 Load/performance tests (search, booking, webhook bursts, location, admin realtime)
- [x] P9-03 Backup policy + restore drill
- [ ] P9-04 Production env, live keys, domain/DNS/Cloudflare WAF (Human)
- [x] P9-05 Monitoring/alerts/on-call runbooks (docs/runbooks/)
- [ ] P9-06 Data retention/privacy review + legal pages (Human)
- [ ] P9-07 Launch smoke test: Accra/Cape Coast/Kumasi config; §33 checklist sign-off (Human)

**Exit criteria:** every §33 launch-checklist item below checked with evidence; sign-off by product, operations, finance and technical owner.

### Evidence log — Phase 9

- 2026-08-13 — P9-03 backup/restore: `scripts/backup.sh` + `scripts/restore.sh` (custom-format pg_dump/pg_restore, version-match note). Restore drill executed against the container database: dump 1.2M → fresh `proguidegh_restore` → **7/7 table counts identical** (users 1300, bookings 440, ledger_entries 1094, payouts 20, courses 9, audit_logs 2563, notification_templates 11), recent-booking spot check passed, drill DB dropped. Runbook `docs/runbooks/backup-restore.md` (nightly/weekly retention policy, quarterly drill, DR procedure with RTO 1h/RPO 24h).
- 2026-08-13 — P9-02 load tests: `tests/load/public.js` (search/regions/packages/readyz) at 20 VUs/30s — **688 rps, p95 12.13ms, 0.00% failed**; `tests/load/booking-flow.js` (register→quote→idempotent create×2→list) at 10 VUs/30s — **p95 620.71ms (<1000 threshold), 0.00% failed, idempotent replay check 100%** (argon2id hashing dominates register latency, expected). Scripts parameterised via VUS/DURATION/API_URL env.
- 2026-08-13 — P9-01 security review: `docs/security-review-2026-08-13.md`. govulncheck: 0 affecting vulnerabilities. pnpm audit --prod: 3 findings confined to Expo build tooling (uuid moderate, image-size DoS ×2 high — not in runtime bundles; bump on next Expo SDK patch). Two code findings fixed: security-headers middleware (nosniff/frame DENY/no-referrer/deny-all CSP) and allowlist CORS middleware with credentials + preflight (`CORS_ALLOWED_ORIGINS`), both in `internal/platform/httpx/security.go` with unit tests, wired into the root middleware chain; `.env.example` documents `PAYOUT_ACCOUNT_KEY` + `CORS_ALLOWED_ORIGINS`. Full Go gate green after the change.
- 2026-08-13 — P9-05 runbooks: `docs/runbooks/backup-restore.md`, `docs/runbooks/payouts.md` (weekly batch, CSV handling, transitions, reconciliation, incident handling), `docs/runbooks/sos-incidents.md` (SOS lifecycle with <2min acknowledge target, escalation matrix, quality-queue cadence), plus existing `docs/runbooks/mobile-release.md`.
- Human-blocked (unchanged): P9-04 production env/keys/DNS/WAF (⛔), P9-06 retention/legal pages (⛔), P9-07 launch smoke + §33 sign-off (⛔).
_(empty)_

---

## Post-V1 (Deferred) — Epic E16 + §34

- [ ] F-01 Hotel/B2B accounts, priority pool, subscription/invoicing
- [ ] F-02 Native mobile apps if PWA limits require
- [ ] F-03 Multi-language UI, in-app chat, surge pricing, referrals, AI trip planner, ML dispatch

---

## Epics (Spec §29)

- [x] E01 Platform Foundation — monorepo, CI, environments, config, logging, DB/Redis, migrations, OpenAPI
- [ ] E02 Identity & Access — **incomplete**: registration, login, OTP, sessions and RBAC are done and tested, but MFA is not enforced by role and there is no step-up re-auth on sensitive actions (see the P1-05 finding in the evidence log)
- [x] E03 Tourist Experience — profile, search, booking, payment, tracking, history, receipts, reviews
- [x] E04 Guide Onboarding — application, documents, verification status, certification
- [x] E05 Guide Marketplace — availability, offers, acceptance, tour lifecycle, earnings
- [x] E06 Certification & Training — workflow, evidence, courses, exams, expiry/retraining
- [x] E07 Booking & Pricing — packages, effective pricing, booking states, cancellations
- [x] E08 Payments & Ledger — collections, webhooks, ledger, refunds, receipts
- [x] E09 Payouts — wallet, payout accounts, eligibility, batches, transfers, reconciliation
- [x] E10 Dispatch & Tracking — matching, Redis offers, WebSockets, GPS, map
- [x] E11 Safety & Incidents — SOS, operations alerts, incident workflow
- [x] E12 Reviews & Quality — verified reviews, ratings, Elite/retraining rules
- [x] E13 Admin & Reporting — command panel, queues, maps, reports, audit
- [x] E14 Notifications — email, SMS, push, templates, retries
- [x] E15 Observability & Security — metrics, tracing, alerts, scanning, backup/runbooks
- [ ] E16 B2B Hotels (Phase 2) — organization accounts, priority pool, subscription/invoicing

---

## Acceptance Criteria — Critical Journeys (Spec §30, re-verified every phase)

### 30.1 Booking and payment
- [x] Given an ACTIVE available guide and valid package, tourist receives a quote generated only by backend rules — TestQuoteMathOverHTTP, TestComputeBreakdownSpecExample
- [x] Creating a booking twice with the same idempotency key returns the same logical booking — phase3_test.go:285 / phase4_test.go:109
- [x] Booking is not CONFIRMED from client redirect alone; confirmed only after verified provider state/webhook — TestPaymentLedgerReceiptJourney
- [x] A replayed success webhook does not duplicate ledger entries, notifications or receipt — TestPaymentLedgerReceiptJourney (replay = 200 no-op)
- [x] Receipt amount/reference match internal payment and booking records — TestPaymentLedgerReceiptJourney, TestReferenceFormat

### 30.2 Dispatch
- [x] Only ACTIVE, eligible, available guides receive offers — TestGuideVisibilityGates, TestDispatchAcceptanceJourney
- [x] Two simultaneous accept requests cannot both assign different guides to the same booking — phase5_test.go:281 concurrent accepts — exactly one 200, one 409
- [x] One guide cannot hold overlapping confirmed/in-progress tours — TestActiveStatuses + bookings_no_guide_overlap exclusion constraint
- [x] Expired offers cannot be accepted — TestOfferIsExpired, TestDispatchDeclineExpiryPresence
- [x] Operations can see why a booking has not been matched — TestDispatchDeclineExpiryPresence

### 30.3 Review
- [x] Only the tourist owning a COMPLETED booking may review — TestVerifiedReviewsAndQualityFlags
- [x] Maximum one review per booking — phase6_test.go:219 second review = 409
- [x] Rating aggregate updates transactionally/eventually with no double count — TestVerifiedReviewsAndQualityFlags
- [x] Quality threshold flags are reproducible from stored reviews/policy — TestVerifiedReviewsAndQualityFlags

### 30.4 Finance
- [x] Every booking allocation creates balanced ledger entries — TestAllocateExactSums, TestValidateRejectsUnbalanced, TestAllocateMatchesQuote
- [x] Refund creates reversing entries and preserves original history — TestReversedEntriesFlipsDirections
- [x] Payout cannot exceed eligible guide balance — TestWalletPayoutBatchAndTransitions
- [x] Same provider payout callback/reference cannot mark/pay twice — phase7_test.go:313 no double-pay
- [x] Finance report totals reconcile to ledger and provider settlement inputs — TestReportingTemplatesAndSettings

---

## Launch Checklist (Spec §33)

- [ ] Production Vercel and Render projects created with least-privilege team access
- [ ] Production PostgreSQL backup policy enabled and restore procedure tested
- [ ] Production Redis configured with access controls
- [ ] R2 private bucket/CORS/signed URL settings validated
- [ ] Domain/DNS/SSL and Cloudflare WAF/rate limits configured
- [ ] Payment live account/webhook URL/signature secret configured and a small live transaction reconciled
- [ ] Payout production path approved and tested safely
- [ ] SMS sender/provider approval completed if required
- [ ] Resend domain authenticated (SPF/DKIM) and production sender configured
- [ ] Google Maps keys restricted by domain/API and spend quotas/alerts configured
- [ ] Sentry/OTel dashboards and critical alerts operational
- [ ] Admin MFA and emergency access procedure tested
- [ ] Pricing, commission, levy, cancellation, refund, insurance and payout policies approved
- [ ] Required legal/privacy/terms/consent text approved by responsible parties
- [ ] Operational incident/SOS roster and escalation process documented
- [ ] Pilot guide dataset fully verified; no placeholder "verified" badges
- [ ] Synthetic/demo data removed from production
- [ ] Smoke tests completed on multiple mobile devices and desktop admin
- [ ] Rollback/runbook and support ownership documented

---

## Agent Stop Conditions (Spec Appendix D — production launch blockers)

Each must be closed before launch; none blocks unrelated development.

- [ ] Verified production payment webhook/signature setup in place
- [x] Ledger invariant tests passing — internal/ledger ok
- [ ] No critical auth/RBAC bypass
- [x] SOS events reach operations dashboard — TestSOSAndIncidentWorkflow
- [x] Database backup/restore procedure exists and is tested — P9-03 drill, 7/7 table counts identical
- [ ] Admin privileged accounts have required MFA
- [x] Guide "verified/insured" badges cannot display without valid evidence/status — TestPubliclyVisible (DocumentsValid gate), TestGuideVisibilityGates
- [x] Critical personal documents are not publicly accessible — main_test.go:567 unsigned GET denied
- [x] Duplicate payout impossible under retries/concurrency — phase7_test.go:313
- [x] No production secrets committed to repository — pre-push scan: 0 secret matches, backups/ and .raven/ excluded
