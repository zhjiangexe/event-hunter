#!/usr/bin/env bash
set -euo pipefail

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"
source scripts/lib/karate.sh

if [[ "${1:-}" != "--correlation" || -z "${2:-}" || -n "${3:-}" ]]; then
  echo "用法：bash scripts/test-ingestion-cutover-rehearsal.sh --correlation CORRELATION_ID" >&2
  exit 2
fi
EVENT_HUNTER_REHEARSAL_CORRELATION="$2"
EVENT_HUNTER_ROLLBACK_REQUIRED=false

rollback_on_exit() {
  if [[ "${EVENT_HUNTER_ROLLBACK_REQUIRED}" == "true" ]]; then
    echo "Cutover rehearsal 結束，強制恢復 committed legacy source。" >&2
    bash scripts/reconcile-ingestion-mode.sh --mode legacy || true
  fi
}
trap rollback_on_exit EXIT INT TERM

bash scripts/reconcile-ingestion-mode.sh --mode shadow --correlation "${EVENT_HUNTER_REHEARSAL_CORRELATION}"
bash scripts/reconcile-ingestion-mode.sh --mode clickhouse-mv --correlation "${EVENT_HUNTER_REHEARSAL_CORRELATION}"
EVENT_HUNTER_ROLLBACK_REQUIRED=true

event_hunter_run_karate \
  artifacts/e2e/karate/backend-event-pipeline-clickhouse-mv-cutover \
  e2e/backend/event-pipeline.feature

bash scripts/reconcile-ingestion-mode.sh --mode legacy
EVENT_HUNTER_ROLLBACK_REQUIRED=false

event_hunter_run_karate \
  artifacts/e2e/karate/backend-event-pipeline-post-rollback \
  e2e/backend/event-pipeline.feature

echo "Cutover rehearsal verified: candidate E2E passed and canonical source returned to legacy."
