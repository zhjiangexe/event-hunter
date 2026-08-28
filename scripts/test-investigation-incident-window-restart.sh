#!/usr/bin/env bash
set -euo pipefail

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

EVENT_HUNTER_API_URL="${EVENT_HUNTER_API_URL:-http://localhost:28333}"
EVENT_HUNTER_TEST_TMP="$(mktemp -d)"
EVENT_HUNTER_COOKIE_JAR="${EVENT_HUNTER_TEST_TMP}/cookies.txt"
EVENT_HUNTER_CASE_BEFORE="${EVENT_HUNTER_TEST_TMP}/case-before.json"
EVENT_HUNTER_CASE_AFTER="${EVENT_HUNTER_TEST_TMP}/case-after.json"
EVENT_HUNTER_SUMMARY_BEFORE="${EVENT_HUNTER_TEST_TMP}/summary-before.json"
EVENT_HUNTER_SUMMARY_AFTER="${EVENT_HUNTER_TEST_TMP}/summary-after.json"

recover_services() {
  docker compose up -d --wait postgres event-hunter-api >/dev/null || true
  rm -rf -- "${EVENT_HUNTER_TEST_TMP}"
}
trap recover_services EXIT

curl --fail --silent --show-error \
  --cookie-jar "${EVENT_HUNTER_COOKIE_JAR}" \
  --header 'Content-Type: application/json' \
  --data '{"role":"INVESTIGATOR"}' \
  "${EVENT_HUNTER_API_URL}/api/v1/auth/demo-session" >/dev/null

curl --fail --silent --show-error \
  --cookie "${EVENT_HUNTER_COOKIE_JAR}" \
  --header 'Content-Type: application/json' \
  --data '{"title":"[Restart E2E] durable incident window","severity":"HIGH","correlation_id":"ORDER-2001","incident_from":"2026-08-20T11:00:00Z","incident_to":"2026-08-20T11:06:00Z"}' \
  "${EVENT_HUNTER_API_URL}/api/v1/investigations" >"${EVENT_HUNTER_CASE_BEFORE}"

EVENT_HUNTER_CASE_ID="$(jq -er '.id' "${EVENT_HUNTER_CASE_BEFORE}")"

curl --fail --silent --show-error \
  --cookie "${EVENT_HUNTER_COOKIE_JAR}" \
  "${EVENT_HUNTER_API_URL}/api/v1/investigations/${EVENT_HUNTER_CASE_ID}/summary" \
  >"${EVENT_HUNTER_SUMMARY_BEFORE}"

jq -e '
  .incident_from == "2026-08-20T11:00:00Z" and
  .incident_to == "2026-08-20T11:06:00Z" and
  .incident_window_source == "TIMELINE_SEARCH"
' "${EVENT_HUNTER_CASE_BEFORE}" >/dev/null

docker compose restart postgres >/dev/null
docker compose up -d --wait postgres >/dev/null
docker compose restart event-hunter-api >/dev/null
docker compose up -d --wait event-hunter-api >/dev/null

curl --fail --silent --show-error \
  --cookie "${EVENT_HUNTER_COOKIE_JAR}" \
  "${EVENT_HUNTER_API_URL}/api/v1/investigations/${EVENT_HUNTER_CASE_ID}" \
  >"${EVENT_HUNTER_CASE_AFTER}"

curl --fail --silent --show-error \
  --cookie "${EVENT_HUNTER_COOKIE_JAR}" \
  "${EVENT_HUNTER_API_URL}/api/v1/investigations/${EVENT_HUNTER_CASE_ID}/summary" \
  >"${EVENT_HUNTER_SUMMARY_AFTER}"

jq --argjson before "$(jq '{incident_from, incident_to, incident_window_source}' "${EVENT_HUNTER_CASE_BEFORE}")" \
  -e '{incident_from, incident_to, incident_window_source} == $before' \
  "${EVENT_HUNTER_CASE_AFTER}" >/dev/null

jq --argjson before "$(jq '{incident_from: .case.incident_from, incident_to: .case.incident_to, incident_window_source: .case.incident_window_source, event_count: .timeline.event_count}' "${EVENT_HUNTER_SUMMARY_BEFORE}")" \
  -e '{incident_from: .case.incident_from, incident_to: .case.incident_to, incident_window_source: .case.incident_window_source, event_count: .timeline.event_count} == $before' \
  "${EVENT_HUNTER_SUMMARY_AFTER}" >/dev/null

EVENT_HUNTER_PERSISTED_WINDOW="$(docker compose exec -T postgres psql \
  --username "${POSTGRES_USER:-event_hunter}" \
  --dbname "${POSTGRES_DB:-event_hunter}" \
  --tuples-only --no-align \
  --command "SELECT incident_from::text || '|' || incident_to::text || '|' || incident_window_source FROM investigation_cases WHERE id = '${EVENT_HUNTER_CASE_ID}';" \
  | tr -d '[:space:]')"

if [[ "${EVENT_HUNTER_PERSISTED_WINDOW}" != "2026-08-2011:00:00+00|2026-08-2011:06:00+00|TIMELINE_SEARCH" ]]; then
  echo "Unexpected persisted incident window: ${EVENT_HUNTER_PERSISTED_WINDOW}" >&2
  exit 1
fi

trap - EXIT
rm -rf -- "${EVENT_HUNTER_TEST_TMP}"

echo "Investigation incident window survived PostgreSQL and API restart: ${EVENT_HUNTER_CASE_ID}."
