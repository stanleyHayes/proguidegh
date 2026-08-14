# Vercel — Next.js frontends (Spec §24.3)

Four separate Vercel projects, one per app in this monorepo. Do **not** create
a single project at the repo root — each app deploys independently.

| Vercel project          | Root directory     | Framework preset | Local port |
| ----------------------- | ------------------ | ---------------- | ---------- |
| `proguidegh-tourist`   | `apps/tourist-web` | Next.js          | 3000       |
| `proguidegh-guide`     | `apps/guide-web`   | Next.js          | 3001       |
| `proguidegh-admin`     | `apps/admin-web`   | Next.js          | 3002       |
| `proguidegh-marketing` | `apps/marketing-web` | Next.js        | 3003       |

## Setup notes

- Import the same Git repo three times; set **Root Directory** per the table.
  Vercel detects the pnpm workspace (`pnpm-workspace.yaml`) — keep the default
  install (`pnpm install`) and build (`pnpm build`) commands, run from the
  app root.
- Node version: 22.
- Git-connected deploys give automatic PR preview deployments per app
  (Spec §24.2). The staging track is driven manually by
  `.github/workflows/deploy-staging.yml` once tokens exist.

## Environment variables (per project, Spec §25)

Set in Vercel → Project → Settings → Environment Variables, scoped to
Preview/Production as appropriate:

- `NEXT_PUBLIC_GOOGLE_MAPS_API_KEY` — browser-restricted key only
  (HTTP-referrer restricted to the app's domains; never a server key).
- `NEXT_PUBLIC_API_URL` — base URL of the api service on Render
  (staging vs production value per environment).
- `SENTRY_DSN` — optional; per-environment DSN if frontend error reporting
  is enabled.

Server-side keys (`GOOGLE_MAPS_API_KEY_SERVER`, JWT/session secrets, payment
keys) belong to the Render services only — never add them to Vercel projects.

## Marketing site — domain matters

`proguidegh-marketing` must own the **apex domain** `proguidegh.com`. This is not
cosmetic:

- `legal_documents` in the database points the mobile apps at
  `https://proguidegh.com/legal/{terms,privacy,location}`, and both app stores
  require a reachable privacy policy before they will accept a submission.
- The Google Play Data Safety form points at
  `https://proguidegh.com/account/delete`.

The three product apps sit on subdomains (`app.`, `guide.`, `admin.`).

### Environment variables

| Variable | Value | Why |
|---|---|---|
| `NEXT_PUBLIC_API_URL` | the Render API base URL | Reads published marketing content and policy versions |
| `NEXT_PUBLIC_SITE_URL` | `https://proguidegh.com` | Canonical URLs, sitemap, Open Graph |
| `NEXT_PUBLIC_APP_URL` | `https://app.proguidegh.com` | Where "Find a guide" and "Apply" send people |

The site renders from built-in launch copy if the API is unreachable, so a brief
API outage degrades content freshness rather than taking the marketing site down.
