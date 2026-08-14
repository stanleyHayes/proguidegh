# ProGuideGH

**"Adapt on the go"** — Certified Tourist Guide Supply System for Ghana.
Prepared for ADAPT Africa + Ghana Tourism Authority + Swedish Chamber.

A production-ready marketplace where tourists discover and book certified
guides, guides receive and perform paid assignments, and administrators get
real-time operational, financial, quality and safety oversight.

## Architecture (V1)

Modular monolith — Spec §6. PostgreSQL is the system of record; Redis is
ephemeral realtime state; MongoDB is optional and unused by default.

| Component | Tech | Path |
|---|---|---|
| Tourist web/PWA | Next.js + TypeScript | `apps/tourist-web` |
| Guide web/PWA | Next.js + TypeScript | `apps/guide-web` |
| Admin portal | Next.js + TypeScript | `apps/admin-web` |
| API (REST + WebSocket) | Go | `services/api` |
| Worker (jobs/schedulers) | Go | `services/worker` |
| Shared UI / contracts / config | TS packages | `packages/` |
| Infra (Render/Vercel/Cloudflare) | IaC/config | `infra/` |

## Quick start

```sh
cp .env.example .env
make infra-up        # PostgreSQL + Redis via Docker Compose
make migrate-up      # database migrations
make dev-api         # Go API on :8080
pnpm install && pnpm dev   # frontends
```

See `docs/` for architecture decision records, runbooks and API docs.
Build governance: see `CLAUDE.md` and `AGENTS.md`.
Delivery plan and status: `docs/implementation-status.md`.
