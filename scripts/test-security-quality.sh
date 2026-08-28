#!/usr/bin/env bash
set -euo pipefail

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

EVENT_HUNTER_REPORT="build/reports/security-quality-summary.json"
EVENT_HUNTER_STARTED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
EVENT_HUNTER_STATUS="failed"
EVENT_HUNTER_STAGE="initialization"

write_report() {
  local EVENT_HUNTER_FINISHED_AT
  EVENT_HUNTER_FINISHED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  mkdir -p "$(dirname "${EVENT_HUNTER_REPORT}")"
  python3 -c 'import json,sys; print(json.dumps({"status":sys.argv[1],"started_at":sys.argv[2],"finished_at":sys.argv[3],"last_stage":sys.argv[4],"go_toolchain":sys.argv[5],"staticcheck":"v0.8.1","govulncheck":"v1.7.0","frontend_audit":"pnpm audit --audit-level high","compose_policy":"scripts/check-compose-security.py"}, indent=2))' \
    "${EVENT_HUNTER_STATUS}" "${EVENT_HUNTER_STARTED_AT}" "${EVENT_HUNTER_FINISHED_AT}" "${EVENT_HUNTER_STAGE}" "$(go -C backend env GOVERSION)" \
    > "${EVENT_HUNTER_REPORT}"
}
trap write_report EXIT

run_stage() {
  EVENT_HUNTER_STAGE="$1"
  shift
  echo
  echo "[Security Quality] ${EVENT_HUNTER_STAGE}"
  "$@"
}

run_stage "verify Go modules" go -C backend mod verify
run_stage "run Staticcheck v0.8.1" go -C backend run honnef.co/go/tools/cmd/staticcheck@v0.8.1 ./...
run_stage "run govulncheck v1.7.0" go -C backend run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...
run_stage "lint frontend" pnpm --dir frontend run lint
run_stage "audit frontend dependencies" pnpm --dir frontend audit --audit-level high
run_stage "check Compose security policy" python3 scripts/check-compose-security.py

EVENT_HUNTER_STAGE="complete"
EVENT_HUNTER_STATUS="passed"
echo
echo "Security quality scan passed. Report: ${EVENT_HUNTER_REPORT}"
