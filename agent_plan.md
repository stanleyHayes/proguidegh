# ProGuideGH — Agent Plan

**Version:** 1.1 • **Date:** 2026-08-13 • **Status:** Active

> **Design-system completion lane (2026-08-14):** product-wide responsive redesign implemented across marketing, tourist web, guide web, admin web, tourist mobile, guide mobile and shared UI/token packages. Static gates are green; see `docs/implementation-status.md` Phase 8 evidence. Host-started live QA is complete for tourist and marketing; guide/admin QA found and fixed two final shell issues, with fresh screenshot confirmation pending a restart of the host-owned processes on ports 3001–3002.
**Product name:** **ProGuideGH** (one word) • **Slug / repo root:** `proguidegh`
_Renamed from the legacy working name "Guide Ghana" — see Phase 0b. Source-document filenames below keep their original names on purpose._
**Source documents:**
- `Guide_Ghana_AI_Agent_End_to_End_Build_Specification.docx` (authoritative build spec)
- `AI_Native_Software_Engineering_Operations_Manual.docx` (SDLC / governance)
- `AI_Development_Workflow_Training_Manual.docx` (Jira/GitHub/AI workflow standards)

---

## 1. Operating Model

### Roles & Ownership (per Operations Manual §Phase 5 / Training Manual)
| Owner | Responsibility in this plan |
|---|---|
| **Claude** | Planning, documentation, ADRs, implementation-status tracking, review |
| **Kimi** | Research, analysis, spec interpretation, gap identification |
| **Codex** | Code generation (Go services, Next.js apps, migrations, tests) |
| **Human (Stakeholder)** | Credentials, provider accounts, approvals, launch sign-off |

> **Ownership override (2026-08-13):** Codex and Claude credits are exhausted. **All remaining tasks — code, docs, research, verification — are executed by the Kimi agent.** The Owner column below now records the originally-planned role only, not the actual executor. Ownership of any task may be reassigned freely; this table is not binding.

### Mobile (user directive, superseded 2026-08-13)
~~V1 mobile approach is installable **PWAs** for tourist and guide.~~ **Superseded the same day: the mobile deliverable is now native via Expo / React Native.** See **Phase M** below for the decision record, the task board and the cross-phase impacts. Mobile for **tourist and guide** remains in scope alongside — not instead of — their web apps; admin stays web-only (Spec §20: "Admin optimized for desktop").

**The PWA layer already shipped and is kept.** `manifest.webmanifest`, `sw.js`, icons, `/offline` and `ServiceWorkerRegister` landed in `apps/tourist-web` and `apps/guide-web` before the pivot. It is **retained as a web-app enhancement** (installable desktop/Android browser experience, offline shell) and is *not* rework — but it is no longer what satisfies "mobile". Do not extend it toward dispatch or background location; those live in the native apps (§M.2 D5).

Where a task is blocked on an external credential (Paystack, R2, Resend, FCM, Google Maps, SMS, **Apple Developer / Google Play**), the agent builds against sandbox/mock adapters and flags the credential need — it does not stop (Spec §31.29).

### Governance rules in force
- Every repo contains `CLAUDE.md` and `AGENTS.md` (AI Governance).
- Branch: `feature/PG-<n>-<name>`; Commit: `PG-<n> <message>`; PR: `PG-<n> <Title>`. **Prefix changed from `GG-` to `PG-` on 2026-08-13** (stakeholder decision D2, Phase 0b). `AGENTS.md`/`CLAUDE.md` still carry the old prefix until NC-09 lands; **this line is authoritative in the meantime**. Branches already open under `feature/GG-*` are not renamed — they finish as-is.
- Definition of Done: code + tests pass + docs updated + status files updated with evidence.
- Vertical slices: migration → repository → service → handler/OpenAPI → frontend → tests → observability → docs (Spec §31.22).
- Phases are sequential; no later phase starts until earlier exit criteria are green (Spec §28).

---

## 2. Master Task Board

Status legend: ⬜ Not started · 🔵 In progress · ✅ Done · ⛔ Blocked (external) · ⏸ Deferred (post-V1)

### Phase 0 — Foundation (Spec Days 1–5) — Epic E01
| ID | Task | Owner | Status |
|---|---|---|---|
| P0-01 | Monorepo scaffold per Spec §7.1 (repo root = `proguidegh/`) | Codex | ✅ |
| P0-02 | CLAUDE.md + AGENTS.md in repo | Claude | ✅ |
| P0-03 | ADRs (monolith, Postgres truth, Redis ephemeral, Mongo optional, payment adapter, object storage, sessions, ledger) | Claude | ✅ |
| P0-04 | docs/implementation-status.md — all phases/epics/acceptance criteria as checkboxes | Claude | ✅ |
| P0-05 | Go API bootstrap (cmd/api, config loader, structured logging, request IDs, health/readiness) | Codex | ✅ |
| P0-06 | Go worker bootstrap (job runner skeleton) | Codex | ✅ |
| P0-07 | Docker Compose: PostgreSQL + Redis local dev | Codex | ✅ |
| P0-08 | Migration framework (goose/golang-migrate) + initial schema + up/down tested | Codex | ✅ |
| P0-09 | Three Next.js apps bootstrap (tourist-web, guide-web, admin-web) + packages/{ui,contracts,config} | Codex | ✅ |
| P0-10 | OpenAPI baseline + generated TS client pipeline | Codex | ✅ |
| P0-11 | GitHub Actions CI (lint, test, build, OpenAPI/migration validation) | Codex | ✅ |
| P0-12 | .env.example per Spec §25 | Codex | ✅ |
| **Exit** | All apps build in CI; API connects to Postgres/Redis; migrations up/down tested; no business logic | |✅ |

### Phase 0b — Name Consolidation → ProGuideGH (Epic E01b) — added 2026-08-13

> **Read §2b.1 – §2b.4 below before touching a single file.** Two of these strings are
> load-bearing (a key-derivation salt and a cookie name) and one of them will lock every
> MFA-enrolled admin out permanently if renamed. This is a mechanical sweep with three
> sharp edges, not a find-and-replace.

| ID | Task | Owner | Status |
|---|---|---|---|
| NC-01 | Root `package.json` `"name": "guide-ghana"` → `"proguidegh"`; re-run `pnpm install`; commit `pnpm-lock.yaml` if the root importer entry changes | Kimi | ✅ |
| NC-02 | Go source strings → `ProGuideGH`: `services/api/cmd/api/main.go` L1 + L243 (embedded OpenAPI title), `services/worker/cmd/worker/main.go` L1, `services/api/internal/migrations/0001_init.up.sql` L1 (header comment only — see §2b.4 note 5) | Kimi | ✅ |
| NC-03 | `docs/api/openapi.yaml` L3 `title:` → `ProGuideGH API`. **Must ship in the same commit as NC-02** or the CI _Contracts (OpenAPI drift)_ job (`.github/workflows/ci.yml` L135) fails — it diffs the binary's dumped spec against this file | Kimi | ✅ |
| NC-04 | MFA issuer label: `services/api/internal/auth/service.go` L413 `TOTPURI("Guide Ghana", …)` → `"ProGuideGH"`; update any asserting test | Kimi | ✅ |
| NC-05 | Session cookie names `gg_access`/`gg_refresh` → `pgh_access`/`pgh_refresh` in `services/api/internal/platform/auth/cookies.go` L10–11; `main_test.go` L139 follows the exported constant, so verify it still compiles. **Invalidates every live session — see §2b.4 note 3** | Kimi | ✅ |
| NC-06 | Frontend brand strings (8 files, 12 strings): `apps/{tourist,guide,admin}-web/app/layout.tsx`, `apps/guide-web/app/page.tsx`, `apps/tourist-web/app/register/page.tsx`, `apps/{tourist,guide,admin}-web/app/lib/api.ts` header comments | Kimi | ✅ |
| NC-07 | Shared-package comments: `packages/ui/src/tokens.css`, `packages/contracts/src/index.ts`, `packages/contracts/README.md`, `packages/config/eslint.config.base.mjs` | Kimi | ✅ |
| NC-08 | Infra + docs headers: `infra/compose.yaml` L1, `infra/render/render.yaml` L1, `.env.example` L2, `README.md` L1, `docs/implementation-status.md` L1, `docs/architecture/adr/0001-modular-monolith.md`, `docs/architecture/adr/0005-payment-provider-adapter.md` | Kimi | ✅ |
| NC-09 | Governance: `GG-` → `PG-` in `AGENTS.md` L58–60 and `CLAUDE.md` L31; brand → ProGuideGH in `AGENTS.md` L1/L9 and `CLAUDE.md` L1/L7. **AGENTS.md is canonical; CLAUDE.md must mirror it exactly** (CLAUDE.md preamble rule) | Kimi | ✅ |
| NC-10 | Jira: rename project key `GG` → `PG` (or create `PG` and migrate issues). Blocks nothing in code; PR descriptions reference these keys | Human | ⛔ |
| NC-11 | Verification gate §2b.4 run in full; commands + output pasted into the `docs/implementation-status.md` Evidence log under Phase 0 | Kimi | ✅ |
| **Exit** | Residual-name grep (§2b.4 step 5) returns **only** allow-listed lines from §2b.3; full CI green; evidence logged | |✅ |

**Already applied** (2026-08-13, this file only): `agent_plan.md` title → ProGuideGH; governance line §1 → `PG-`. Everything else in the table is open.

#### §2b.1 Verified starting state (checked 2026-08-13, do not re-litigate)

There is **no `ghana-guide` directory** — not nested in this repo, not under `~/Desktop`, `~/Documents`
or `~/Downloads`. Nothing needs to be physically moved. The repo root is already `proguidegh/` as
P0-01 specified. What remains is a **naming split inside the existing tree**.

Already on the new name (leave alone): Go modules `proguidegh/api` + `proguidegh/worker`; all five
pnpm packages `@proguidegh/*`; Render services `proguidegh-api` / `proguidegh-worker`; Vercel projects
`proguidegh-{tourist,guide,admin}`; Postgres user/password/db `proguidegh`.

Still on the legacy name: **28 files**, ~40 occurrences — enumerated exhaustively in the NC-01…NC-09
rows above. That list is complete; if a sweep finds a string not in it, stop and add a row rather
than improvising.

#### §2b.2 Locked decisions (stakeholder, 2026-08-13)

- **D1 — Display name is `ProGuideGH`**, one word, no space, capital P/G/GH. Not "ProGuide GH",
  not "Guide Ghana". Lowercase slug stays `proguidegh`.
- **D2 — Ticket prefix `GG-` → `PG-`** for branch / commit / PR. Depends on NC-10 for the Jira side.
- **D3 — Source-of-record documents are frozen.** The three `.docx` files and `extracted/*.md` are
  inputs, not project code. Do **not** rewrite them.
- **D4 — The MFA key-derivation salt does not change.** See note 1 below.

#### §2b.3 Do-not-touch allow-list (grep will hit these; that is correct)

| Location | Why it stays |
|---|---|
| `services/api/internal/platform/auth/totp.go` L137 `"guide-ghana/mfa/v1/"` | Key-derivation input — note 1 |
| `extracted/spec.md` L343, L483 and all other hits | Frozen source document (D3) |
| `Guide_Ghana_AI_Agent_End_to_End_Build_Specification.docx` + filename references in this file's header | Real filename of a frozen source document |
| `agent_plan.md` §Phase 0b (this section) | Quotes the legacy names deliberately, to describe the migration |

#### §2b.4 Execution notes — the parts that actually bite

1. **Never change `services/api/internal/platform/auth/totp.go` L137.** The literal
   `"guide-ghana/mfa/v1/"` is a domain-separation prefix hashed with the app secret
   (`sha256.Sum256([]byte("guide-ghana/mfa/v1/" + appSecret))`) to derive the AES-GCM key that
   encrypts stored TOTP secrets. Change the string and the derived key changes, every existing
   encrypted `mfa.secret` fails `gcm.Open`, and **every MFA-enrolled admin is locked out with no
   recovery path except manual re-enrollment**. The string is never displayed to anyone. If it is
   ever changed for hygiene, that is its own task: introduce `proguidegh/mfa/v2/`, keep a v1 read
   path, and re-encrypt in a migration. Out of scope for Phase 0b.
2. **The MFA _issuer_ (NC-04) is safe to change** — unlike note 1, `TOTPURI("Guide Ghana", …)` is a
   display label inside the `otpauth://` URI, not key material. Existing enrollments keep showing
   "Guide Ghana" in the user's authenticator app and their codes keep working; only new enrollments
   show ProGuideGH. Acceptable pre-launch.
3. **NC-05 logs everyone out.** Browsers keep sending `gg_access`/`gg_refresh`; the server stops
   reading them. Harmless today (no production users), irreversible-in-practice after launch. Do it
   now or never. `pgh_` prefix chosen over `pg_` to avoid reading as "Postgres".
4. **NC-02 and NC-03 are one commit.** The OpenAPI title exists twice — embedded in the Go binary and
   committed to `docs/api/openapi.yaml`. Split them across commits and CI's drift job fails on the
   intermediate commit.
5. **Editing the `0001_init.up.sql` header comment is safe.** `schema_migrations`
   (`internal/migrations/migrations.go` L105) stores only `version`, `name`, `applied_at` — there is
   no checksum column, so a comment edit will not invalidate an applied migration. The general rule
   still holds: **never edit applied migration SQL**, comments only.
6. **Receipt reference format.** Spec §16 illustrates receipt refs as `GG-88291`. Not yet implemented.
   When P4-06 lands it must use `PGH-<n>`, not `GG-`. Recorded here so the receipt task does not
   inherit the legacy prefix from the frozen spec.
7. **Do not rename `apps/guide-web`.** "guide" there is the persona (tourist / guide / admin), not
   the old brand.

**Verification gate (NC-11) — run all five, paste output into the Evidence log:**

```bash
pnpm install --frozen-lockfile=false && pnpm lint && pnpm typecheck && pnpm build
gofmt -l services && go vet ./... && go test ./... # from repo root, go.work workspace
go build -o /tmp/api ./services/api/cmd/api && /tmp/api openapi | diff -u docs/api/openapi.yaml -
docker compose -f infra/compose.yaml up -d && (cd services/api && go run ./cmd/migrate up && go run ./cmd/migrate down -all)
grep -rIn "Guide Ghana\|guide-ghana\|guide_ghana\|GG-" . \
  --exclude-dir=node_modules --exclude-dir=.git --exclude-dir=.raven \
  --exclude-dir=extracted --exclude="pnpm-lock.yaml" --exclude="*.docx"
```

Step 5 must return **only** §2b.3 allow-listed lines. Anything else is an incomplete sweep.

### Phase 1 — Identity, RBAC & Profiles (Days 6–15) — Epic E02, E13(part)
| ID | Task | Owner | Status |
|---|---|---|---|
| P1-01 | Users/roles/permissions schema + seed (roles, permission codes per Appendix A) | Codex | ✅ |
| P1-02 | Registration/login/OTP request+verify/password reset | Codex | ✅ |
| P1-03 | Session model: short-lived access + rotating refresh, HttpOnly cookies, revocation | Codex | ✅ |
| P1-04 | RBAC authorization layer + middleware (permission-enforced, not UI-only) | Codex | ✅ |
| P1-05 | MFA for Super Admin/finance roles; step-up auth for sensitive actions | Codex | 🔵 **partial — Appendix D blocker.** TOTP enrol/verify and login challenge work, but MFA is **not enforced by role** (`MFARequiredRole` is dead code; `Login` only checks whether the user chose to enrol) and **step-up re-auth on sensitive actions does not exist**. See the 2026-08-14 finding in `docs/implementation-status.md` |
| P1-06 | Tourist profile endpoints + UI | Codex | ✅ |
| P1-07 | Guide application/profile shell + private document upload (R2 signed URLs, mock-capable) | Codex | ✅ |
| P1-08 | Admin user/guide directory | Codex | ✅ |
| P1-09 | Audit framework for privileged mutations | Codex | ✅ |
| **Exit** | Tourist & applicant accounts work; admin permission-enforced; docs private/signed; auth/RBAC tests pass | |✅ |

### Phase 2 — Certification & Catalog (Days 16–27) — Epics E04, E06(part), E07(part)
| ID | Task | Owner | Status |
|---|---|---|---|
| P2-01 | Certification case state machine (APPLIED→…→ACTIVE + exceptions) with audited transitions | Codex | ✅ |
| P2-02 | Document evidence/expiry model | Codex | ✅ |
| P2-03 | Admin certification review queues | Codex | ✅ |
| P2-04 | Training shell / required-training flags | Codex | ✅ |
| P2-05 | Catalog: regions, languages, specialties (Appendix C), tour packages, effective-dated pricing rules | Codex | ✅ |
| P2-06 | Public guide profile visibility gate (eligible status only) | Codex | ✅ |
| **Exit** | Admin can move test guide through audited process to ACTIVE; only ACTIVE guides public | |✅ |

### Phase 3 — Search, Booking & Availability (Days 28–40) — Epics E03(part), E07
| ID | Task | Owner | Status |
|---|---|---|---|
| P3-01 | Guide availability schedules/online state | Codex | ✅ |
| P3-02 | Search filters/ranking (Spec §10.1–10.2) + guide detail | Codex | ✅ |
| P3-03 | Quote endpoint — server-authoritative pricing | Codex | ✅ |
| P3-04 | Booking aggregate + state machine (§8.2) + overlap checks (transactional) | Codex | ✅ |
| P3-05 | Tourist checkout UX (pre-payment) | Codex | ✅ |
| **Exit** | Tourist can search, quote, create payment-pending booking without double-booking | |✅ |

### Phase 4 — Payments, Ledger & Receipts (Days 41–52) — Epic E08
| ID | Task | Owner | Status |
|---|---|---|---|
| P4-01 | Payment adapter interface (§16.1) + Paystack sandbox/mock implementation | Codex | ✅ |
| P4-02 | Payment init + signed webhook handling (verify, dedupe, archive) | Codex | ✅ |
| P4-03 | Idempotency keys on booking/payment/refund mutations | Codex | ✅ |
| P4-04 | Immutable ledger: accounts/transactions/entries + booking allocation (15%/3% configurable effective-dated) | Codex | ✅ |
| P4-05 | Ledger invariant tests (balanced, immutable, provider-ref uniqueness, replay-safe) | Codex | ✅ |
| P4-06 | Receipt generation (PDF) + private R2 storage + signed download | Codex | ✅ |
| P4-07 | Refund skeleton + reversing entries + admin policy flow | Codex | ✅ |
| **Exit** | Test payment confirms exactly one booking, one balanced allocation, downloadable receipt, replay-safe | |✅ |
| EXT-1 | Paystack production keys + webhook secret | Human | ⛔ |

### Phase M — Mobile Apps (Expo / React Native) — Epic E17 — added 2026-08-13

> **Supersedes the PWA plan.** Read §M.1 – §M.4 before starting. Phase M runs **in parallel with
> Phase 5**, not after it: the foundation (M-01…M-08) has no Phase 5 dependency, while the guide
> app's job feed and background location (M-13…M-16) consume P5-01/02/03/04/05 and must not be
> built against mocks.

| ID | Task | Owner | Status |
|---|---|---|---|
| **Foundation — no Phase 5 dependency, start now** | | |
| M-01 | `packages/tokens` — design tokens as TS (RN has no CSS). `tokens.css` stays the web import; drift is blocked by `pnpm --filter @proguidegh/tokens test`, which fails if any of the 29 tokens disagrees (rem→px aware) | Claude | ✅ |
| M-02 | `apps/tourist-mobile` — Expo SDK 57 + expo-router scaffold, TS strict, `@proguidegh/tokens` wired, demo template stripped, lint/typecheck/`expo-doctor` 20/20 green | Claude | ✅ |
| M-03 | `apps/guide-mobile` — same scaffold, plus background-location permissions, usage strings and Android foreground-service config declared in `app.json` | Claude | ✅ |
| M-04 | `packages/api-client` — shared fetch client for both mobile apps: base URL, bearer header, 401 → refresh → retry once, typed against `@proguidegh/contracts` | Kimi | ✅ |
| M-05 | **Backend:** `RefreshFromRequest` accepts the refresh token from `X-Refresh-Token` header or JSON body, not cookie only — rotation and logout are broken for native clients until this lands. See §M.4 note 1 | Kimi | ✅ |
| M-06 | **ADR 0009 — native session storage.** Mandated by ADR 0007's own consequence clause ("Bearer-token API clients… would need a separate mechanism and ADR"). Covers `expo-secure-store` (Keychain/Keystore), rotation, reuse detection, revocation on logout | Kimi | ✅ |
| M-07 | Auth flow in both apps: login → tokens in `expo-secure-store` → session context → protected routes. MFA challenge path included (admin-role guides) | Kimi | ✅ |
| M-08 | CI lane: `.github/workflows/ci.yml` mobile job — install, `tsc --noEmit`, lint, unit tests, `expo-doctor`. EAS builds stay out of PR CI | Kimi | ✅ |
| **Tourist app — depends on Phases 1–4 only (all done)** | | |
| M-09 | Search + guide detail screens (§10.1–10.2 parity with tourist-web) | Kimi | ✅ |
| M-10 | Quote → booking → checkout. Payment opens the provider-hosted page via `expo-web-browser`; **no card fields in the app** (non-negotiable §1.2 #6) | Kimi | ✅ |
| M-11 | Bookings list/detail, receipts via short-lived signed URL (never cached to disk — §1.2 #7) | Kimi | ✅ |
| M-12 | Profile + offline/empty/error/retry states (Spec §20). `lib/offline.tsx` gates writes on connectivity — no write queue, see §M.4 note 7 | Kimi | ✅ |
| **Guide app — blocked on Phase 5, do not mock** | | |
| M-13 | Go online/offline via `PresenceProvider` → P5-01. 90s heartbeat inside the 300s Redis TTL, plus an immediate beat on foreground return | Claude | ✅ |
| M-14 | Job feed over `/ws/guide` with `GET /me/guide/offers` as catch-up + fallback; one-tap accept, server arbitrates. 409/410 surfaced as distinct outcomes, not generic errors | Claude | ✅ |
| M-15 | **Background location** — `lib/location-task.ts` TaskManager task + Android foreground service, gated on an active-booking key so nothing is collected outside the §11.1 window. Permission-rationale screen shipped with it (§M.4 note 4) | Claude | ✅ |
| M-16 | Tour lifecycle (en route / arrived / start / complete) → P5-05. This screen owns the location window: entering it authorises collection, completing or leaving revokes it | Claude | ✅ |
| **Release** | | |
| M-17 | EAS Build + Submit config for both apps (development/preview/production, `requireCommit`, remote versioning) + `docs/runbooks/mobile-release.md`. `eas init` and the two App Store Connect ids still need EXT-3 | Claude | ✅ |
| **Compliance (M-20…M-23)** | | | |
| M-20 | **Data-subject rights backend** — migration `0010_privacy`; `DELETE /me` (erasure by anonymisation), `GET /me/deletion` (preview + blockers), `GET /me/export` (subject access), `POST /me/consent`, public `GET /legal/policies`. `storage.Store` gained `Delete` so verification documents leave R2. 5 integration tests | Claude | ✅ |
| M-21 | In-app Privacy & data screen in both apps (`@proguidegh/ui-native/privacy`) + web deletion route at `/account/delete` in both web apps — Play requires one reachable without installing | Claude | ✅ |
| M-22 | iOS privacy manifests (`ios.privacyManifests`) in both apps; Android 14 `foregroundServiceType` confirmed shipped by `expo-location`'s own manifest | Claude | ✅ |
| M-23 | Compliance docs: `docs/compliance/{data-inventory,app-store,play-store,ghana}.md` — one source of truth behind the Apple nutrition label, Play Data Safety form and privacy policy | Claude | ✅ |
| **Marketing site (M-25…M-27)** | | | |
| M-25 | **`apps/marketing-web`** — public site at the apex domain. Home, destinations (+3 city pages), become-a-guide, safety, pricing, about, FAQ, contact, 404, sitemap, robots, JSON-LD. 19 prerendered routes | Claude | ✅ |
| M-26 | **CMS**: content in `system_settings` key `marketing.site`, edited at **admin-web → Content**, read publicly via `GET /api/v1/content/marketing`. Reuses the audited settings endpoints rather than new CMS tables, so every copy change is attributable | Claude | ✅ |
| M-27 | **`/legal/{terms,privacy,location}` + `/account/delete` now resolve** — these are the URLs `legal_documents` points the apps at, and their 404 was blocking store submission. Pages render the document outline plus an explicit "with counsel" notice until M-24 supplies the approved text | Claude | ✅ |
| M-24a | **Initial legal text seeded + CMS.** Migration 0011 gives `legal_documents` a markdown body, summary and approval state; v2026-08-14 seeded for terms/privacy/location describing actual platform behaviour. Editing publishes a **new version** (409 on duplicate) because consent references `(document, version)`; approval is separate and audited. Editor at admin-web → Legal | Claude | ✅ |
| M-24b | **Counsel approval.** The text exists and renders, but ships `approved = false` — the public page shows a draft banner and `noindex` until a lawyer signs off via admin → Legal → Approve. Still needs DPC registration and GTA/levy confirmation (`docs/compliance/ghana.md`). **Do not submit to the stores while the banner is showing** | Human | ⛔ |
| M-18 | Store listings, privacy nutrition labels, background-location justification copy (both stores reject undisclosed background location) | Human | ⛔ |
| EXT-3 | **Apple Developer Program** (~$99/yr) + **Google Play Console** (~$25 one-time) accounts | Human | ⛔ |
| **Exit** | Both apps install on a physical Android and iOS device via EAS internal distribution; tourist completes search→book→pay→receipt against sandbox; guide goes online, accepts one dispatched job and streams location with the app backgrounded; CI mobile lane green | | ⛔ EXT-3 |

#### §M.1 Verified starting state (2026-08-13)

Pinned versions: **Expo SDK 57.0.12 · React Native 0.87.0 · expo-router 57.0.12** · Node 26.5 · pnpm 10.27.

The API is **already largely native-ready**, which is why M-05 is the only backend task here:

- [`cookies.go` L49–62](services/api/internal/platform/auth/cookies.go#L49-L62) — `AccessFromRequest` already falls back to `Authorization: Bearer`.
- [`handler.go` L388–395](services/api/internal/auth/handler.go#L388-L395) — `writeSession` already returns `access_token` / `refresh_token` / `expires_at` in the JSON body alongside the cookies.
- **The gap:** `RefreshFromRequest` (L42–47) reads the cookie only. Native rotation and logout fail silently until M-05.

Reusable as-is: `@proguidegh/contracts` (plain TS). **Not reusable:** `@proguidegh/ui` — it is DOM React (`div`, `className`, CSS files) and has no meaning in React Native. Do not try to make it universal; M-01 shares the *tokens*, and each platform keeps its own components.

#### §M.2 Locked decisions

- **D5 — Mobile is native (Expo/React Native), not PWA.** Justified by Spec §34, which authorises native "if PWA limitations affect GPS/background". Background location is a V1 dispatch requirement (§10.3, P5-01) and no PWA on iOS can deliver it. This pulls **F-02 forward from Post-V1**.
- **D6 — Web apps stay.** Spec §18.1/§18.2 routes remain; mobile complements them. Admin is web-only.
- **D7 — pnpm stays on the default isolated node-linker.** Expo supports isolated installs from SDK 54 and this repo is on 57. `nodeLinker: hoisted` is the documented fallback **only** if a React Native library fails to resolve — do not set it pre-emptively; it changes install layout for the three existing Next apps.
- **D8 — Native sessions use `expo-secure-store`** (iOS Keychain / Android Keystore), never `AsyncStorage`, which is unencrypted plaintext on disk.

#### §M.3 Cross-phase impacts (update these owners, don't discover them later)

| Touches | Impact |
|---|---|
| P1-03 / ADR 0007 | Native refresh path (M-05) + ADR 0009 (M-06). ADR 0007 explicitly requires this ADR |
| P5-01/02/04 | Mobile becomes the _primary_ guide dispatch surface; guide-web is the fallback, not the reverse |
| P6-01 SOS | Native SOS gets real background coordinates — strictly better than the web path; re-verify §30 safety criteria on device |
| P8-03 notifications | Push moves from FCM-web to `expo-notifications`; `FCM_PROJECT_ID` in `.env.example` still applies for Android |
| P9-04 launch | Store review adds **1–7 days** of latency that DNS/WAF cutover does not. Submit before the launch window, not during it |
| CI | New mobile lane (M-08). EAS builds are not PR-blocking |

#### §M.4 Execution notes

1. **M-05 before M-07.** Building the mobile auth flow against a cookie-only refresh endpoint produces an app that works until the first token expiry and then logs the user out with no diagnosable error. Do the backend first.
2. **Never put card fields in the app.** Payment opens the Paystack-hosted page in `expo-web-browser`; the app only observes the return deep link and polls its own booking status. Card capture in-app violates §1.2 #6 and is an Appendix D launch blocker.
3. **Receipts are signed-URL only.** Fetch through the short-lived signed URL and render; do not persist the PDF to device storage (§1.2 #7).
4. **Background location must be disclosed.** Both stores reject apps that collect background location without an in-app rationale screen and a store-listing justification. Build the permission-rationale screen in M-15, not as an afterthought in M-18.
5. **Do not mock Phase 5 for M-13…M-16.** The spec requires disconnect/reconnect/expiry tests (P5-07); mocked realtime hides exactly those failures.
6. **`packages/ui` stays DOM-only.** See §M.1. The two native apps share
   **`packages/ui-native`** instead (added during M-13): one implementation of the
   RN primitives, aliased through each app's `lib/ui.tsx` so screens keep importing
   `@/lib/ui`. Duplicating them per app was the alternative and would have drifted.
7. **No offline write queue in the tourist app.** Considered and rejected: replaying
   a queued booking would submit a stale server-authoritative quote and race the
   availability check (§1.2, P3-04). Offline means read cached data and block writes
   with a stated reason — never an optimistic success. `lib/offline.tsx` owns this.
8. **Account status is checked on every authenticated request.** `RequireAuth`
   and the WS handshake call `rbac.AccountActive`, which is deliberately
   uncached. Access tokens are stateless JWTs valid for their full 15-minute
   lifetime, so before this existed a suspended or deleted account kept working
   until its token expired — contradicting §15.2 and making account deletion
   only eventually effective. Found by the M-20 deletion test.
9. **`/ws/guide` authenticates via `?token=`, not a header.** React Native's
   WebSocket cannot set `Authorization` and there are no cookies; the server
   supports the query parameter (`realtime.go`). Access tokens are short-lived
   (15 min), but a query string can reach proxy logs — this is the realtime channel
   only, never plain HTTP, which uses the header via `@proguidegh/api-client`.

### Phase 5 — Dispatch, Realtime & Tour Operations (Days 53–64) — Epics E05(part), E10
| ID | Task | Owner | Status |
|---|---|---|---|
| P5-01 | Redis presence/online/location with TTL | Codex | ✅ |
| P5-02 | Dispatch scoring + batch offers + Redis TTL expiry (§10.3) | Codex | ✅ |
| P5-03 | Atomic offer accept (DB tx/distributed lock) + overlap prevention | Codex | ✅ |
| P5-04 | WebSocket channels: /ws/guide, /ws/booking/{id}, /ws/admin/operations | Codex | ✅ |
| P5-05 | Tour lifecycle transitions (en route/arrived/start/complete; ops override w/ reason) | Codex | ✅ |
| P5-06 | Admin live operations map + list fallback | Codex | ✅ |
| P5-07 | Disconnect/reconnect/expiry tests | Codex | ✅ |
| P5-08 | ~~Guide + tourist PWA mobile~~ **Superseded → Phase M.** The web PWA layer shipped and is retained as a web enhancement; the mobile deliverable is the native Expo apps. Guide realtime on mobile is **M-13…M-16**, which consume this phase's P5-01…P5-05 | — | ➡ Phase M |
| **Exit** | Nearby ACTIVE guide accepts one offer; location streams to authorized parties; valid transitions to completion | |✅ |

### Phase 6 — Safety, Reviews & Quality (Days 65–72) — Epics E11, E12
| ID | Task | Owner | Status |
|---|---|---|---|
| P6-01 | SOS endpoint + immutable event + HIGH/CRITICAL incident + realtime admin alert | Codex | ✅ |
| P6-02 | Fallback notifications (SMS/push/email) for SOS per policy | Codex | ✅ |
| P6-03 | Incident dashboard/workflow (ack, notes, escalation, closure — audited) | Codex | ✅ |
| P6-04 | Verified review flow (one per completed booking) + tags (Appendix B) | Codex | ✅ |
| P6-05 | Rating aggregation, <4.0 retraining flag, >4.8 Elite qualification review | Codex | ✅ |
| P6-06 | Quality/retraining queue UI | Codex | ✅ |
| **Exit** | SOS reaches ops with coords + audit; only completed bookings reviewable; thresholds flag correctly | |✅ |

### Phase 7 — Wallet, Payouts & Finance (Days 73–80) — Epic E09
| ID | Task | Owner | Status |
|---|---|---|---|
| P7-01 | Guide wallet/statement derived from ledger | Codex | ✅ |
| P7-02 | Payout account verification fields (tokenized refs) | Codex | ✅ |
| P7-03 | Eligibility scheduler (T+7) + weekly payout batch | Codex | ✅ |
| P7-04 | Provider transfer integration or safe manual export fallback | Codex | ✅ |
| P7-05 | Retry/manual-review states (§8.4) + finance dashboard | Codex | ✅ |
| P7-06 | Tourism Levy accrual/reconciliation reports | Codex | ✅ |
| P7-07 | Idempotency/concurrency tests: no duplicate payout under retries | Codex | ✅ |
| **Exit** | Eligible earnings batch without duplicates; retries idempotent; finance reconciles | |✅ |
| EXT-2 | Production transfer/payout credentials | Human | ⛔ |

### Phase 8 — Training, Analytics & Admin Polish (Days 81–86) — Epics E06(part), E13, E14
| ID | Task | Owner | Status |
|---|---|---|---|
| P8-01 | Light LMS: courses/modules/lessons/enrollment/progress/quiz/certificates | Codex | ✅ |
| P8-02 | Executive KPI dashboard + operational reports + permitted CSV exports | Codex | ✅ |
| P8-03 | Notification templates/settings (versioned) | Codex | ✅ |
| P8-04 | Audit viewer + policy configuration UI | Codex | ✅ |
| P8-05 | Web PWA polish + offline/retry UX states for `tourist-web`/`guide-web` (web only — native polish is Phase M) | Kimi | ✅ |

### Phase 9 — Hardening & Launch (Days 87–90) — Epic E15 + Launch Checklist §33
| ID | Task | Owner | Status |
|---|---|---|---|
| P9-01 | Security review + dependency/container scans | Kimi | ✅ |
| P9-02 | Load/performance tests (search, booking, webhook bursts, location, admin realtime) | Codex | ✅ |
| P9-03 | Backup policy + restore drill | Codex | ✅ |
| P9-04 | Production env, live keys, domain/DNS/Cloudflare WAF | Human | ⛔ |
| P9-05 | Monitoring/alerts/on-call runbooks (docs/runbooks/) | Claude | ✅ |
| P9-06 | Data retention/privacy review + legal pages | Human | ⛔ |
| P9-07 | Launch smoke test: Accra/Cape Coast/Kumasi config; checklist §33 sign-off | Human | ⛔ |

### Post-V1 (Deferred) — Epic E16 + §34
| ID | Task | Owner | Status |
|---|---|---|---|
| F-01 | Hotel/B2B accounts, priority pool, subscription/invoicing | — | ⏸ |
| F-02 | ~~Native mobile apps if PWA limits require~~ **Pulled forward to V1 as Phase M** (decision D5, 2026-08-13) — background location for dispatch is a V1 requirement a PWA cannot meet | — | ➡ Phase M |
| F-03 | Multi-language UI, in-app chat, surge pricing, referrals, AI trip planner, ML dispatch | — | ⏸ |

---

## 3. Cross-Cutting Acceptance Criteria (Spec §30 — verified every phase)
- [ ] Booking: server-only quotes; idempotent create; webhook-only confirmation; replay-safe; receipt matches records
- [ ] Dispatch: eligible-only offers; single-assignment atomicity; no overlapping tours; expired offers rejected; unmatched reason visible
- [ ] Review: owner-of-completed-booking only; max one per booking; aggregate without double count; reproducible flags
- [ ] Finance: balanced allocations; reversing refunds; payout ≤ eligible balance; no double-pay on callback replay; reconciliation

## 4. Agent Stop Conditions (Spec Appendix D — launch blockers)
No prod webhook verification · failing ledger invariants · auth/RBAC bypass · SOS not reaching ops · no backup/restore · admin MFA missing · unearned verified/insured badges · public personal documents · duplicate payout possible · secrets committed.

## 4b. Audit findings — 2026-08-14 (open, behind a ✅)

Two items were marked done but are not. Both are recorded with evidence in
`docs/implementation-status.md`.

| # | Finding | Severity |
|---|---|---|
| A | **P1-05 MFA is not enforced by role.** `Login` checks only whether a user *enrolled*; `MFARequiredRole` is dead code outside tests. A `super_admin` who never enrols signs in with a password alone. There is also **no step-up re-auth** for role changes, payout-account edits or refunds — §15.2 requires both | **Appendix D launch blocker** |
| B | **No tourist `/support` surface.** SOS works on an active booking, but §18.1's standalone support/incidents page is missing and there is no tourist-facing incident endpoint (`/admin/incidents` is admin-only) | Spec gap, needs a backend slice |

Not gaps, recorded so nobody re-opens them: `/guide/payouts` is deliberately
folded into `/guide/wallet`; MongoDB is absent, which §1.2 permits.

Fixing A means denying privileged permissions until a required user enrols —
a `MFASatisfied` flag on `rbac.Identity` set in `RequireAuth`, checked in
`RequirePermission`. `/me/mfa/enroll` sits behind `RequireAuth` only, so
enrolment stays reachable. It will fail ~10 integration tests whose harness
grants `super_admin` and immediately calls admin endpoints, which is itself
proof the gap is real.

## 5. Current Status Snapshot
- **Active phase:** Phase 5 — Dispatch, Realtime & Tour Operations (Phases 0, 0b, 1–4 complete and evidence-logged).
- **Next task:** two tracks in parallel — Phase 5 (P5-01 Redis presence/location onward) and **Phase M foundation (M-01 → M-08)**, which has no Phase 5 dependency. M-13…M-16 stay blocked until P5-01…P5-05 land.
- **External blockers:** NC-10 Jira key rename (Human); EXT-1 Paystack production keys (due Phase 9 go-live); EXT-2 payout credentials (Phase 7+).
- See `docs/implementation-status.md` for per-phase evidence log (created in P0-04).
