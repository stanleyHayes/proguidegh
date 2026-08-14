#!/usr/bin/env bash
# ProGuideGH Postgres backup (P9-03).
#
# Creates a compressed custom-format dump of DATABASE_URL (default: local
# dev database) into ./backups with a UTC timestamp. Custom format supports
# selective restore and parallel pg_restore.
#
# Production policy (docs/runbooks/backup-restore.md): nightly dump retained
# 30 days + weekly retained 12 weeks, stored off-host (object storage).
#
# NOTE: pg_dump/pg_restore must match the server MAJOR version. With the
# docker-compose database, run inside the container instead:
#   docker exec infra-postgres-1 pg_dump -U proguidegh -d proguidegh \
#     --format=custom --no-owner --no-privileges -f /tmp/backup.dump
set -euo pipefail

DATABASE_URL="${DATABASE_URL:-postgres://proguidegh:proguidegh@localhost:5432/proguidegh?sslmode=disable}"
OUT_DIR="${BACKUP_DIR:-$(dirname "$0")/../backups}"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
OUT_FILE="${OUT_DIR}/proguidegh-${STAMP}.dump"

mkdir -p "$OUT_DIR"
pg_dump --format=custom --compress=6 --no-owner --no-privileges \
  --dbname="$DATABASE_URL" --file="$OUT_FILE"

# Integrity check: the dump must list a table of contents.
pg_restore --list "$OUT_FILE" > /dev/null

echo "backup written: $OUT_FILE ($(du -h "$OUT_FILE" | cut -f1))"
