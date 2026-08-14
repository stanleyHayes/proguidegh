#!/usr/bin/env bash
# ProGuideGH Postgres restore (P9-03).
#
# Restores a custom-format dump created by scripts/backup.sh into
# RESTORE_DATABASE_URL (default: a throwaway *_restore database on the local
# server). Restoring over a live database is NOT what this script does —
# for disaster recovery, restore into a fresh database, verify, then point
# DATABASE_URL at it (see docs/runbooks/backup-restore.md).
#
# Usage: scripts/restore.sh backups/proguidegh-<stamp>.dump
set -euo pipefail

DUMP_FILE="${1:?usage: restore.sh <dump-file>}"
RESTORE_DATABASE_URL="${RESTORE_DATABASE_URL:-postgres://proguidegh:proguidegh@localhost:5432/proguidegh_restore?sslmode=disable}"

# Drop and recreate the target database for a clean restore.
psql "$(echo "$RESTORE_DATABASE_URL" | sed 's|/proguidegh_restore|/postgres|')" \
  -c "DROP DATABASE IF EXISTS proguidegh_restore" \
  -c "CREATE DATABASE proguidegh_restore"

pg_restore --no-owner --no-privileges --exit-on-error \
  --dbname="$RESTORE_DATABASE_URL" "$DUMP_FILE"

echo "restored $DUMP_FILE -> $RESTORE_DATABASE_URL"
echo "verify: psql \"$RESTORE_DATABASE_URL\" -c 'SELECT COUNT(*) FROM bookings'"
