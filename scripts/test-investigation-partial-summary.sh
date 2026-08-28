#!/usr/bin/env bash
set -euo pipefail

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

EVENT_HUNTER_API_URL="${EVENT_HUNTER_API_URL:-http://localhost:28333}"
EVENT_HUNTER_TEST_TMP="$(mktemp -d)"
EVENT_HUNTER_COOKIE_JAR="${EVENT_HUNTER_TEST_TMP}/cookies.txt"
EVENT_HUNTER_CASE_JSON="${EVENT_HUNTER_TEST_TMP}/case.json"
EVENT_HUNTER_SUMMARY_JSON="${EVENT_HUNTER_TEST_TMP}/summary.json"

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
  --data '{"title":"[Failure E2E] partial summary","severity":"HIGH","correlation_id":"ORDER-2001","incident_from":"2026-08-20T11:00:00Z","incident_to":"2026-08-20T11:06:00Z"}' \
  "${EVENT_HUNTER_API_URL}/api/v1/investigations" >"${EVENT_HUNTER_CASE_JSON}"

EVENT_HUNTER_CASE_ID="$(jq -er '.id' "${EVENT_HUNTER_CASE_JSON}")"
docker compose stop clickhouse >/dev/null

EVENT_HUNTER_HTTP_STATUS="$(curl --silent --show-error \
  --cookie "${EVENT_HUNTER_COOKIE_JAR}" \
  --output "${EVENT_HUNTER_SUMMARY_JSON}" \
  --write-out '%{http_code}' \
  "${EVENT_HUNTER_API_URL}/api/v1/investigations/${EVENT_HUNTER_CASE_ID}/summary")"

if [[ "${EVENT_HUNTER_HTTP_STATUS}" != "200" ]]; then
  echo "Summary status = ${EVENT_HUNTER_HTTP_STATUS}, want 200" >&2
  jq . "${EVENT_HUNTER_SUMMARY_JSON}" >&2 || true
  exit 1
fi

jq -e '
  .partial == true and
  .source_status.postgres == "OK" and
  .source_status.clickhouse == "UNAVAILABLE" and
  .source_last_success_at.postgres != null and
  .source_last_success_at.clickhouse == null and
  (.warnings | index("CLICKHOUSE_UNAVAILABLE")) != null and
  .case.correlation_id == "ORDER-2001" and
  .case.incident_window_source == "TIMELINE_SEARCH" and
  .timeline.event_count == 0 and
  .timeline.events == []
' "${EVENT_HUNTER_SUMMARY_JSON}" >/dev/null

docker compose up -d --wait clickhouse kafka-connect-clickhouse-poc technical-dlq-projector event-hunter-api >/dev/null
trap - EXIT
rm -rf -- "${EVENT_HUNTER_TEST_TMP}"

echo "Investigation partial Summary failure mode verified and ClickHouse recovered."
