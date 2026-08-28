#!/usr/bin/env bash
set -euo pipefail

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

EVENT_HUNTER_API_URL="${EVENT_HUNTER_API_URL:-http://localhost:28333}"
EVENT_HUNTER_TEST_TMP="$(mktemp -d)"
EVENT_HUNTER_COOKIE_JAR="${EVENT_HUNTER_TEST_TMP}/cookies.txt"
EVENT_HUNTER_CASE_JSON="${EVENT_HUNTER_TEST_TMP}/case.json"
EVENT_HUNTER_ANALYSIS_JSON="${EVENT_HUNTER_TEST_TMP}/analysis.json"

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
  --data '{"title":"[Failure E2E] Pattern source unavailable","severity":"HIGH","correlation_id":"ORDER-2001","incident_from":"2026-08-20T11:00:00Z","incident_to":"2026-08-20T11:06:00Z"}' \
  "${EVENT_HUNTER_API_URL}/api/v1/investigations" >"${EVENT_HUNTER_CASE_JSON}"

EVENT_HUNTER_CASE_ID="$(jq -er '.id' "${EVENT_HUNTER_CASE_JSON}")"
docker compose stop clickhouse >/dev/null

EVENT_HUNTER_HTTP_STATUS="$(curl --silent --show-error \
  --cookie "${EVENT_HUNTER_COOKIE_JAR}" \
  --header 'Content-Type: application/json' \
  --data '{"execution_mode":"SYNC"}' \
  --output "${EVENT_HUNTER_ANALYSIS_JSON}" \
  --write-out '%{http_code}' \
  "${EVENT_HUNTER_API_URL}/api/v1/investigations/${EVENT_HUNTER_CASE_ID}/analyze")"

if [[ "${EVENT_HUNTER_HTTP_STATUS}" != "503" ]]; then
  echo "Pattern analysis status = ${EVENT_HUNTER_HTTP_STATUS}, want 503" >&2
  jq . "${EVENT_HUNTER_ANALYSIS_JSON}" >&2 || true
  exit 1
fi

jq -e '.code == "PATTERN_SOURCE_UNAVAILABLE"' "${EVENT_HUNTER_ANALYSIS_JSON}" >/dev/null

curl --fail --silent --show-error \
  --cookie "${EVENT_HUNTER_COOKIE_JAR}" \
  "${EVENT_HUNTER_API_URL}/api/v1/investigations/${EVENT_HUNTER_CASE_ID}" \
  | jq -e '.id != null and .pattern_findings == [] and .evidence == []' >/dev/null

docker compose up -d --wait clickhouse kafka-connect-clickhouse-poc technical-dlq-projector event-hunter-api >/dev/null
trap - EXIT
rm -rf -- "${EVENT_HUNTER_TEST_TMP}"

echo "Pattern source failure returned an explicit error without persisting a false result, and ClickHouse recovered."
