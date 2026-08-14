#!/usr/bin/env bash
# Regenerate TypeScript API contracts from the Go API's OpenAPI spec.
# The spec is produced by services/api (Phase 0 baseline); until it exists
# this script will fail — that is expected.
set -euo pipefail

cd "$(dirname "$0")/.."

SPEC="../../docs/api/openapi.yaml"
if [[ ! -f "$SPEC" ]]; then
  echo "error: $SPEC not found — generate the OpenAPI baseline from services/api first." >&2
  exit 1
fi

pnpm exec openapi-typescript "$SPEC" -o src/generated.ts
echo "wrote src/generated.ts"
