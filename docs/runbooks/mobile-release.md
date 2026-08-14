# Mobile release runbook (Phase M, M-17)

Covers `apps/tourist-mobile` and `apps/guide-mobile`. EAS builds are **not** part
of PR CI (M-08 runs typecheck/lint/`expo-doctor` only) — they are run
deliberately, from a clean commit.

## Prerequisites (Human — EXT-3)

| Item | Needed for | Cost |
|---|---|---|
| Expo account + `eas login` | any build | free tier works for internal distribution |
| Apple Developer Program | iOS builds, TestFlight, App Store | ~$99/yr |
| Google Play Console | Play internal testing, production | ~$25 one-time |

Until these exist, everything below fails at the first `eas` call. Nothing in the
apps is blocked on them for development — use `pnpm --filter … start` with Expo
Go for anything that does not need background location.

## One-time project linking

```bash
cd apps/tourist-mobile && npx eas init      # writes extra.eas.projectId + owner
cd ../guide-mobile      && npx eas init
```

`eas init` writes `extra.eas.projectId` and `owner` into `app.json`. Commit both —
they are the app's identity on EAS, not secrets.

Then fill the two placeholders in each `eas.json` `submit.production.ios` block
(`appleTeamId`, `ascAppId`). Those come from App Store Connect after the app
record exists.

## Build profiles

| Profile | Distribution | Use |
|---|---|---|
| `development` | internal | dev client; needed for anything using native modules not in Expo Go |
| `preview` | internal | **the Phase M exit gate** — installs on a physical device without a store |
| `production` | store | App Store / Play submission |

`requireCommit: true` is set deliberately: a build must be traceable to a commit.

## The Phase M exit gate

```bash
cd apps/tourist-mobile && npx eas build --profile preview --platform all
cd ../guide-mobile      && npx eas build --profile preview --platform all
```

Install both on a physical Android and a physical iPhone, then verify:

1. **Tourist:** search → guide → book → pay on the Paystack sandbox page →
   booking leaves `PAYMENT_PENDING` → receipt opens.
2. **Guide:** go online → receive a dispatched offer → accept → transition
   en-route → **lock the phone** → confirm positions still arrive at
   `POST /bookings/{id}/location` → complete the tour → confirm collection stops.

Step 2 must be done on a **physical device with the screen locked**. A simulator
does not exercise the background execution limits that are the entire reason
these apps are native (decision D5).

## Background location cannot be tested in Expo Go

`expo-location` background updates need a development or preview build. If a
guide-app change touches `lib/location-task.ts`, an Expo Go run proves nothing —
rebuild the dev client.

## Store submission (after M-18 listings exist)

```bash
npx eas build --profile production --platform all
npx eas submit --profile production --platform all
```

Both stores require the background-location justification. The in-app rationale
screen (`/location-permission`) exists to satisfy that review; the listing copy
in M-18 must say the same thing. Expect **1–7 days** of review latency — submit
before the launch window, not during it (§M.3).

## Versioning

`appVersionSource: "remote"` — EAS owns build numbers; `autoIncrement` is on for
production only. Bump the user-facing `version` in `app.json` by hand for a
release; never hand-edit build numbers.
