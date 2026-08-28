#!/usr/bin/env bash
set -euo pipefail

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"
source scripts/lib/karate.sh

python3 scripts/load-quality-fixture.py
docker compose run --rm quality-worker aggregate \
  --from=2026-08-20T15:00:00Z \
  --to=2026-08-20T15:01:00Z
docker compose run --rm quality-worker aggregate \
  --from=2026-08-20T16:00:00Z \
  --to=2026-08-20T16:01:00Z

EVENT_HUNTER_OUTPUT_DIR="${KARATE_OUTPUT_DIR:-artifacts/e2e/karate/backend-quality}"
event_hunter_run_karate "${EVENT_HUNTER_OUTPUT_DIR}" e2e/backend/quality-metrics.feature
