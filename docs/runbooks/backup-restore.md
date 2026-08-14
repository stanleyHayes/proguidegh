# Backup & restore runbook (P9-03)

Postgres is the system of record (spec §1, §7). Redis is ephemeral — never
back it up; losing it costs in-flight rate-limit counters and cached
permissions, nothing durable.

## Policy

| Tier | Schedule | Retention | Location |
|---|---|---|---|
| Nightly dump | 02:00 UTC daily | 30 days | object storage (R2/S3), off-host |
| Weekly dump | Sunday 02:00 UTC | 12 weeks | object storage, off-host |
| Pre-migration | before every `migrate up` in production | until release + 7 days | object storage |

`scripts/backup.sh` produces the dump (custom format, compressed, no
owner/privileges). **pg_dump must match the server major version** — with
docker-compose, use `docker exec` into the Postgres container.

Object storage uploads are a human/infra concern (EXT): wire the nightly
cron to `backup.sh` + an `rclone`/SDK upload step, and alert on non-zero
exit or a missing daily file older than 26h.

## Restore drill (quarterly, and after every Postgres upgrade)

1. Take a fresh dump (or fetch the latest nightly).
2. Restore into a throwaway database: `scripts/restore.sh <dump>` (targets
   `proguidegh_restore` by default — never the live database).
3. Verify row counts match live on the high-signal tables:
   `users`, `bookings`, `ledger_entries`, `payouts`, `audit_logs`.
4. Spot-check one recent booking row end-to-end.
5. `DROP DATABASE proguidegh_restore`.
6. Log the drill (date, dump timestamp, counts) in the ops log. A drill
   that has not run in the last quarter blocks launch sign-off (§33).

## Disaster recovery

1. Provision a fresh Postgres at the same major version.
2. `pg_restore` the latest good dump into it.
3. Run `go run ./cmd/migrate up` if the dump predates the current schema.
4. Point `DATABASE_URL` at the new server; restart API + worker.
5. Verify `/readyz` green, then a read-only smoke (search, one booking GET)
   before reopening traffic.

RTO target: 1 hour. RPO target: 24 hours (nightly) — tighten both by adding
WAL archiving when volume justifies it.
