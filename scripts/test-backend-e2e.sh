#!/usr/bin/env bash
set -euo pipefail

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"
source scripts/lib/karate.sh

EVENT_HUNTER_OUTPUT_DIR="${KARATE_OUTPUT_DIR:-artifacts/e2e/karate/backend}"
EVENT_HUNTER_E2E_RATE_LIMIT_WINDOW="${EVENT_HUNTER_BACKEND_E2E_RATE_LIMIT_WINDOW:-10s}"
EVENT_HUNTER_RUNTIME_INGESTION_MODE="$(docker compose exec -T event-hunter-api printenv EVENT_HUNTER_INGESTION_MODE)"
EVENT_HUNTER_RUNTIME_ATTEMPTS_INGESTION_MODE="$(docker compose exec -T event-hunter-api printenv EVENT_HUNTER_ATTEMPTS_INGESTION_MODE)"
EVENT_HUNTER_RUNTIME_RATE_LIMIT_WINDOW="$(docker compose exec -T event-hunter-api printenv EVENT_HUNTER_RATE_LIMIT_WINDOW)"
EVENT_HUNTER_RUNTIME_RATE_LIMIT_REQUESTS="$(docker compose exec -T event-hunter-api printenv EVENT_HUNTER_RATE_LIMIT_REQUESTS)"

restore_api_runtime_profile() {
  EVENT_HUNTER_INGESTION_MODE="${EVENT_HUNTER_RUNTIME_INGESTION_MODE}" \
  EVENT_HUNTER_ATTEMPTS_INGESTION_MODE="${EVENT_HUNTER_RUNTIME_ATTEMPTS_INGESTION_MODE}" \
  EVENT_HUNTER_RATE_LIMIT_WINDOW="${EVENT_HUNTER_RUNTIME_RATE_LIMIT_WINDOW}" \
  EVENT_HUNTER_RATE_LIMIT_REQUESTS="${EVENT_HUNTER_RUNTIME_RATE_LIMIT_REQUESTS}" \
    docker compose up -d --no-deps --force-recreate --wait event-hunter-api >/dev/null || true
}
trap restore_api_runtime_profile EXIT

# The complete backend suite intentionally exercises hundreds of requests from one local IP.
# Keep the production request limit unchanged, but shorten only the E2E fixed window so
# unrelated late-running features are not coupled to earlier feature traffic.
EVENT_HUNTER_INGESTION_MODE="${EVENT_HUNTER_RUNTIME_INGESTION_MODE}" \
EVENT_HUNTER_ATTEMPTS_INGESTION_MODE="${EVENT_HUNTER_RUNTIME_ATTEMPTS_INGESTION_MODE}" \
EVENT_HUNTER_RATE_LIMIT_WINDOW="${EVENT_HUNTER_E2E_RATE_LIMIT_WINDOW}" \
EVENT_HUNTER_RATE_LIMIT_REQUESTS="${EVENT_HUNTER_RUNTIME_RATE_LIMIT_REQUESTS}" \
  docker compose up -d --no-deps --force-recreate --wait event-hunter-api >/dev/null

bash scripts/verify-event-pipeline-readiness.sh
event_hunter_run_karate "${EVENT_HUNTER_OUTPUT_DIR}" e2e/backend
