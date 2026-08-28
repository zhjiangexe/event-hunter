#!/usr/bin/env bash
set -euo pipefail

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

EVENT_HUNTER_START_STACK=true
EVENT_HUNTER_DISRUPTIVE_CHECKS=true
for EVENT_HUNTER_ARGUMENT in "$@"; do
  case "${EVENT_HUNTER_ARGUMENT}" in
    --no-start) EVENT_HUNTER_START_STACK=false ;;
    --skip-disruptive) EVENT_HUNTER_DISRUPTIVE_CHECKS=false ;;
    *)
      echo "用法：bash scripts/test-phase-1-exit.sh [--no-start] [--skip-disruptive]" >&2
      exit 2
      ;;
  esac
done

EVENT_HUNTER_STARTED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
EVENT_HUNTER_EXIT_STATUS="failed"
EVENT_HUNTER_CURRENT_STAGE="initialization"
EVENT_HUNTER_EXIT_REPORT="build/reports/phase-1-exit-summary.json"

write_exit_report() {
  local EVENT_HUNTER_FINISHED_AT
  EVENT_HUNTER_FINISHED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  mkdir -p "$(dirname "${EVENT_HUNTER_EXIT_REPORT}")"
  python3 -c 'import json,sys; print(json.dumps({"status":sys.argv[1],"started_at":sys.argv[2],"finished_at":sys.argv[3],"last_stage":sys.argv[4],"performance_report":"build/reports/performance-summary.json","backend_karate_report":"artifacts/e2e/karate/backend/karate-summary.html","frontend_karate_report":"artifacts/e2e/karate/frontend/karate-summary.html"}, indent=2))' \
    "${EVENT_HUNTER_EXIT_STATUS}" "${EVENT_HUNTER_STARTED_AT}" "${EVENT_HUNTER_FINISHED_AT}" "${EVENT_HUNTER_CURRENT_STAGE}" \
    > "${EVENT_HUNTER_EXIT_REPORT}"
}
trap write_exit_report EXIT

run_stage() {
  EVENT_HUNTER_CURRENT_STAGE="$1"
  shift
  echo
  echo "[Phase 1] ${EVENT_HUNTER_CURRENT_STAGE}"
  "$@"
}

if [[ "${EVENT_HUNTER_START_STACK}" == "true" ]]; then
  run_stage "start local stack" bash scripts/dev-up.sh
fi

run_stage "validate contracts" python3 scripts/validate-contracts.py

EVENT_HUNTER_CURRENT_STAGE="check Go formatting"
EVENT_HUNTER_UNFORMATTED="$(gofmt -l backend)"
if [[ -n "${EVENT_HUNTER_UNFORMATTED}" ]]; then
  echo "Go files require gofmt:" >&2
  echo "${EVENT_HUNTER_UNFORMATTED}" >&2
  exit 1
fi
run_stage "verify Go modules" go -C backend mod verify
run_stage "run Go vet" go -C backend vet ./...
run_stage "run Go tests" go -C backend test ./...

run_stage "check generated OpenAPI client" pnpm --dir frontend run api:check
run_stage "check generated Event Lab OpenAPI client" pnpm --dir frontend run scenario-api:check
run_stage "check frontend formatting" pnpm --dir frontend run format:check
run_stage "lint frontend" pnpm --dir frontend run lint
run_stage "typecheck frontend" pnpm --dir frontend run typecheck
run_stage "test frontend components" pnpm --dir frontend run test:run
run_stage "build frontend" pnpm --dir frontend run build

run_stage "load demo domain and observability fixtures" bash scripts/load-demo-fixtures.sh
run_stage "test quality aggregation" bash scripts/test-quality-e2e.sh
run_stage "test backend acceptance" bash scripts/test-backend-e2e.sh
run_stage "test frontend acceptance" bash scripts/test-frontend-e2e.sh
if [[ "${EVENT_HUNTER_DISRUPTIVE_CHECKS}" == "true" ]]; then
  run_stage "test live OpenTelemetry vertical slice and restart persistence" bash scripts/test-live-observability.sh
else
  run_stage "test live OpenTelemetry vertical slice" bash scripts/test-live-observability.sh --skip-restart
fi
run_stage "test ingestion mapping and DLQ" bash scripts/test-ingestion-pipeline.sh
run_stage "verify quality worker failure mode" bash scripts/verify-quality-runtime.sh
run_stage "verify Grafana provisioning" bash scripts/verify-grafana-provisioning.sh

if [[ "${EVENT_HUNTER_DISRUPTIVE_CHECKS}" == "true" ]]; then
  run_stage "verify sink acknowledgement" bash scripts/test-ingestion-acknowledgement.sh --yes
  run_stage "verify ingestion readiness and automatic recovery" bash scripts/test-ingestion-recovery.sh --yes
  run_stage "verify investigation partial summary" bash scripts/test-investigation-partial-summary.sh
  run_stage "verify investigation incident window restart" bash scripts/test-investigation-incident-window-restart.sh
  run_stage "verify Pattern Analysis source failure" bash scripts/test-pattern-analysis-source-failure.sh
fi

run_stage "load CI performance fixture" python3 scripts/load-performance-fixture.py
# Backend／frontend E2E 共用本機 rate limiter；效能量測前重啟 API，避免前一階段流量污染 profile。
run_stage "reset API rate limiter before performance" docker compose restart event-hunter-api
run_stage "wait for API after performance reset" docker compose up -d --wait event-hunter-api
run_stage "run CI performance profile" python3 scripts/run-ci-performance-profile.py

if [[ "${EVENT_HUNTER_DISRUPTIVE_CHECKS}" == "true" ]]; then
  run_stage "verify restart persistence" bash scripts/verify-restart-persistence.sh
fi

# Successful release-gate runs must not leave cases, Scenario Lab runs, demo
# service rows or fresh ClickHouse aggregates that change the next Overview.
run_stage "clean isolated E2E data" bash scripts/cleanup-e2e-data.sh --since "${EVENT_HUNTER_STARTED_AT}"

# Performance fixtures replace the interactive quality dataset. Restore the
# domain, trace, log and event-time quality rows so the running demo remains
# usable after a successful gate.
run_stage "restore interactive demo fixtures" bash scripts/load-demo-fixtures.sh

EVENT_HUNTER_CURRENT_STAGE="complete"
EVENT_HUNTER_EXIT_STATUS="passed"
echo
echo "Phase 1 exit gate passed. Report: ${EVENT_HUNTER_EXIT_REPORT}"
