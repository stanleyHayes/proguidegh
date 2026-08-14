# AGENTS.md — guide-mobile (ProGuideGH guide app)

Repo-wide rules in the root [`AGENTS.md`](../../AGENTS.md) apply here and win on
conflict. This file covers what is specific to the native app.

## Expo has changed

Read the versioned docs at <https://docs.expo.dev/versions/v57.0.0/> before writing
code. Do not rely on remembered Expo/React Native APIs — they move between SDKs.

Pinned: **Expo SDK 57 · React Native 0.86.2 · expo-router 57 · TypeScript 6.0**.

The TypeScript major deliberately diverges from the web workspaces' 5.9: `expo-doctor`
requires ~6.0.3 for SDK 57, and each workspace runs its own `tsc`. The shared
packages (`@proguidegh/contracts`, `@proguidegh/tokens`) are plain TS source and
compile under both.

## Where things live

- `src/app/` — expo-router routes. Typed routes are on (`experiments.typedRoutes`).
- `@proguidegh/tokens` — the only source of colours, spacing, type scale and radii.
  Never hardcode a hex value; `packages/ui` is DOM-only and must not be imported here.
- `@proguidegh/contracts` — generated API types, shared with the web apps.

## Background location — the reason this app is native at all

Background location is what a PWA cannot do and what Spec §34 authorises native for
(decision D5). It is also the fastest way to get rejected by both app stores. Rules:

- `expo-location` background task **plus** an Android foreground-service
  notification. Permissions and usage strings are already declared in `app.json`.
- Location is collected **only** while the guide is online or on an active tour.
  Never on a cold start, never while offline.
- Ship the in-app permission-rationale screen with M-15, not later as part of the
  store listing (M-18). Apple and Google both reject undisclosed background location.

## Non-negotiables for this app

- **Sessions go in `expo-secure-store`** (Keychain/Keystore), never `AsyncStorage`.
- **Do not mock Phase 5.** M-13…M-16 consume P5-01…P5-05. Mocked realtime hides the
  disconnect/reconnect/expiry failures P5-07 exists to catch (§M.4 note 5).
- Payout account fields are tokenised references only — no raw MoMo credentials.

## Current state

Scaffold (M-03). The "Go online" control is inert, not mocked, because P5-01 does not
exist yet. Task board: **Phase M** in [`agent_plan.md`](../../agent_plan.md).
