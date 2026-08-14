# ProGuideGH — guide mobile app

Native iOS/Android app for certified guides: go online, receive dispatched jobs,
run tours with live location, track earnings and payouts. Built with Expo SDK 57
and expo-router.

This app is the reason mobile went native rather than PWA: dispatch needs
**background location**, which no PWA delivers on iOS (Spec §34, decision D5).

Companion to `apps/guide-web` — the web app stays (Spec §18.2) as the fallback
surface, not the primary one.

## Running it

From the repo root:

```bash
pnpm install
pnpm --filter @proguidegh/guide-mobile start   # then press i / a, or scan the QR
```

Background location cannot be exercised in Expo Go — it needs a development build
(`npx expo run:ios` / `run:android`) or an EAS build. There is no `web` script:
`react-native-web` is deliberately not a dependency.

Point the app at a local API with `extra.apiUrl` in `app.json` (defaults to
`http://localhost:8080`).

## Checks

```bash
pnpm --filter @proguidegh/guide-mobile typecheck
pnpm --filter @proguidegh/guide-mobile lint
pnpm --filter @proguidegh/guide-mobile run expo:doctor
```

## Status

Scaffold only (Phase M, M-03). The job feed, dispatch accept, tour lifecycle and
background location (M-13…M-16) are **blocked on Phase 5** (P5-01…P5-05) and must
not be built against mocks. See [`AGENTS.md`](./AGENTS.md) and **Phase M** in
[`agent_plan.md`](../../agent_plan.md).
