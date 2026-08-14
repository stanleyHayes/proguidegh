# ProGuideGH — tourist mobile app

Native iOS/Android app for tourists: find a certified guide, book, pay, track the
tour, download receipts. Built with Expo SDK 57 and expo-router.

Companion to `apps/tourist-web` — the web app stays (Spec §18.1); this does not
replace it.

## Running it

From the repo root:

```bash
pnpm install
pnpm --filter @proguidegh/tourist-mobile start   # then press i / a, or scan the QR
```

`pnpm --filter @proguidegh/tourist-mobile ios` and `… android` open a simulator
directly. There is no `web` script: `react-native-web` is deliberately not a
dependency, because the web experience is `apps/tourist-web`.

Point the app at a local API with `extra.apiUrl` in `app.json` (defaults to
`http://localhost:8080`). Bring the API up with `make dev` / `infra/compose.yaml`.

## Checks

```bash
pnpm --filter @proguidegh/tourist-mobile typecheck
pnpm --filter @proguidegh/tourist-mobile lint
pnpm --filter @proguidegh/tourist-mobile run expo:doctor
```

## Status

Scaffold only (Phase M, M-02). See [`AGENTS.md`](./AGENTS.md) for the rules that
apply here and **Phase M** in [`agent_plan.md`](../../agent_plan.md) for the board.
