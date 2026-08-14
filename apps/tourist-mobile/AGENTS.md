# AGENTS.md — tourist-mobile (ProGuideGH tourist app)

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

## Non-negotiables for this app

- **No card fields, ever.** Payment opens the provider-hosted page in
  `expo-web-browser`; the app observes the return deep link and polls its own
  booking status. In-app card capture violates Spec §1.2 #6 and is an Appendix D
  launch blocker.
- **Sessions go in `expo-secure-store`** (Keychain/Keystore), never `AsyncStorage`,
  which is unencrypted plaintext on disk.
- **Receipts are signed-URL only.** Render from the short-lived URL; never persist
  the PDF to device storage (Spec §1.2 #7).
- **Do not mock Phase 5.** See §M.4 note 5 in `agent_plan.md`.

## Current state

Scaffold (M-02). Screens are inert placeholders, not mocks. Next: M-05 (native
refresh on the API) → M-07 (auth) → M-09…M-12 (search, booking, receipts, profile).
Task board: **Phase M** in [`agent_plan.md`](../../agent_plan.md).
