#!/usr/bin/env bash
set -euo pipefail

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

EVENT_HUNTER_API_URL="${EVENT_HUNTER_API_URL:-http://localhost:28333}"
EVENT_HUNTER_TEST_TMP="$(mktemp -d)"
EVENT_HUNTER_COOKIE_JAR="${EVENT_HUNTER_TEST_TMP}/cookies.txt"
EVENT_HUNTER_BASELINE_JSON="${EVENT_HUNTER_TEST_TMP}/baseline.json"
EVENT_HUNTER_FAILURE_JSON="${EVENT_HUNTER_TEST_TMP}/failure.json"
EVENT_HUNTER_RECOVERED_JSON="${EVENT_HUNTER_TEST_TMP}/recovered.json"
EVENT_HUNTER_EVALUATION_REQUEST='{"identifier":{"type":"CORRELATION_ID","value":"ORDER-2001"},"from":"2026-08-20T11:00:00Z","to":"2026-08-20T11:06:00Z","model":{"id":"order-fulfillment","version":2}}'

recover_dependencies() {
  docker compose up -d --wait clickhouse kafka-connect-clickhouse-poc technical-dlq-projector event-hunter-api >/dev/null || true
  rm -rf -- "${EVENT_HUNTER_TEST_TMP}"
}
trap recover_dependencies EXIT

curl --fail --silent --show-error \
  --cookie-jar "${EVENT_HUNTER_COOKIE_JAR}" \
  --header 'Content-Type: application/json' \
  --data '{"role":"INVESTIGATOR"}' \
  "${EVENT_HUNTER_API_URL}/api/v1/auth/demo-session" >/dev/null

curl --fail --silent --show-error \
  --cookie "${EVENT_HUNTER_COOKIE_JAR}" \
  --header 'Content-Type: application/json' \
  --data "${EVENT_HUNTER_EVALUATION_REQUEST}" \
  "${EVENT_HUNTER_API_URL}/api/v1/event-checks/evaluations" \
  >"${EVENT_HUNTER_BASELINE_JSON}"

jq -e '.source_health.status == "HEALTHY" and .result.check_status != null' \
  "${EVENT_HUNTER_BASELINE_JSON}" >/dev/null

docker compose stop clickhouse >/dev/null

EVENT_HUNTER_HTTP_STATUS="$(curl --silent --show-error \
  --cookie "${EVENT_HUNTER_COOKIE_JAR}" \
  --header 'Content-Type: application/json' \
  --data "${EVENT_HUNTER_EVALUATION_REQUEST}" \
  --output "${EVENT_HUNTER_FAILURE_JSON}" \
  --write-out '%{http_code}' \
  "${EVENT_HUNTER_API_URL}/api/v1/event-checks/evaluations")"

if [[ "${EVENT_HUNTER_HTTP_STATUS}" != "503" ]]; then
  echo "Event Check source failure status = ${EVENT_HUNTER_HTTP_STATUS}, want 503" >&2
  jq . "${EVENT_HUNTER_FAILURE_JSON}" >&2 || true
  exit 1
fi

jq -e \
  '.code == "EVENT_CHECK_SOURCE_UNAVAILABLE" and (.result | not) and (.snapshot_id | not)' \
  "${EVENT_HUNTER_FAILURE_JSON}" >/dev/null

docker compose up -d --wait clickhouse kafka-connect-clickhouse-poc technical-dlq-projector event-hunter-api >/dev/null

curl --fail --silent --show-error \
  --cookie "${EVENT_HUNTER_COOKIE_JAR}" \
  --header 'Content-Type: application/json' \
  --data "${EVENT_HUNTER_EVALUATION_REQUEST}" \
  "${EVENT_HUNTER_API_URL}/api/v1/event-checks/evaluations" \
  >"${EVENT_HUNTER_RECOVERED_JSON}"

jq -e \
  --arg evaluation_hash "$(jq -er '.evaluation_hash' "${EVENT_HUNTER_BASELINE_JSON}")" \
  '.source_health.status == "HEALTHY" and .evaluation_hash == $evaluation_hash' \
  "${EVENT_HUNTER_RECOVERED_JSON}" >/dev/null

trap - EXIT
rm -rf -- "${EVENT_HUNTER_TEST_TMP}"

echo "Event Check source failure returned 503 without a false business result, and recovered deterministically."
