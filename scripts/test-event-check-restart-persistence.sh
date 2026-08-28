#!/usr/bin/env bash
set -euo pipefail

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

EVENT_HUNTER_API_URL="${EVENT_HUNTER_API_URL:-http://localhost:28333}"
EVENT_HUNTER_TEST_TMP="$(mktemp -d)"
EVENT_HUNTER_COOKIE_JAR="${EVENT_HUNTER_TEST_TMP}/cookies.txt"
EVENT_HUNTER_EVALUATION="${EVENT_HUNTER_TEST_TMP}/evaluation.json"
EVENT_HUNTER_SNAPSHOT_BEFORE="${EVENT_HUNTER_TEST_TMP}/snapshot-before.json"
EVENT_HUNTER_SNAPSHOT_AFTER="${EVENT_HUNTER_TEST_TMP}/snapshot-after.json"
EVENT_HUNTER_CASE="${EVENT_HUNTER_TEST_TMP}/case.json"
EVENT_HUNTER_CASE_HEADERS="${EVENT_HUNTER_TEST_TMP}/case-headers.txt"
EVENT_HUNTER_LINKS_BEFORE="${EVENT_HUNTER_TEST_TMP}/links-before.json"
EVENT_HUNTER_LINKS_AFTER="${EVENT_HUNTER_TEST_TMP}/links-after.json"
EVENT_HUNTER_CASE_ID=""

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
  --data '{"identifier":{"type":"CORRELATION_ID","value":"ORDER-2001"},"from":"2026-08-20T11:00:00Z","to":"2026-08-20T11:06:00Z","model":{"id":"order-fulfillment","version":2}}' \
  "${EVENT_HUNTER_API_URL}/api/v1/event-checks/evaluations" \
  >"${EVENT_HUNTER_EVALUATION}"

EVENT_HUNTER_EVENT_SET_HASH="$(jq -er '.event_set_hash' "${EVENT_HUNTER_EVALUATION}")"
EVENT_HUNTER_EVALUATION_HASH="$(jq -er '.evaluation_hash' "${EVENT_HUNTER_EVALUATION}")"
EVENT_HUNTER_IDEMPOTENCY_KEY="event-check-restart-$(date -u +%s)-${RANDOM}"

jq -n \
  --slurpfile evaluation "${EVENT_HUNTER_EVALUATION}" \
  --arg event_set_hash "${EVENT_HUNTER_EVENT_SET_HASH}" \
  --arg evaluation_hash "${EVENT_HUNTER_EVALUATION_HASH}" \
  '{evaluation_request: $evaluation[0].normalized_request, expected_event_set_hash: $event_set_hash, expected_evaluation_hash: $evaluation_hash}' \
  | curl --fail --silent --show-error \
      --cookie "${EVENT_HUNTER_COOKIE_JAR}" \
      --header 'Content-Type: application/json' \
      --header "Idempotency-Key: ${EVENT_HUNTER_IDEMPOTENCY_KEY}" \
      --data-binary @- \
      "${EVENT_HUNTER_API_URL}/api/v1/check-snapshots" \
      >"${EVENT_HUNTER_SNAPSHOT_BEFORE}"

EVENT_HUNTER_SNAPSHOT_ID="$(jq -er '.id' "${EVENT_HUNTER_SNAPSHOT_BEFORE}")"
EVENT_HUNTER_RUN_TOKEN="$(date -u +%Y%m%dT%H%M%SZ)-${RANDOM}"

curl --fail --silent --show-error \
  --cookie "${EVENT_HUNTER_COOKIE_JAR}" \
  --dump-header "${EVENT_HUNTER_CASE_HEADERS}" \
  --header 'Content-Type: application/json' \
  --data "{\"title\":\"[Restart E2E] Event Check Snapshot ${EVENT_HUNTER_RUN_TOKEN}\",\"severity\":\"HIGH\",\"correlation_id\":\"ORDER-2001\"}" \
  "${EVENT_HUNTER_API_URL}/api/v1/investigations" \
  >"${EVENT_HUNTER_CASE}"

EVENT_HUNTER_CASE_ID="$(jq -er '.id' "${EVENT_HUNTER_CASE}")"
EVENT_HUNTER_CASE_ETAG="$(awk 'tolower($1) == "etag:" { gsub("\r", "", $2); print $2 }' "${EVENT_HUNTER_CASE_HEADERS}")"
if [[ -z "${EVENT_HUNTER_CASE_ETAG}" ]]; then
  echo "Create Investigation response did not include ETag" >&2
  exit 1
fi

curl --fail --silent --show-error \
  --cookie "${EVENT_HUNTER_COOKIE_JAR}" \
  --header 'Content-Type: application/json' \
  --header "If-Match: ${EVENT_HUNTER_CASE_ETAG}" \
  --data "{\"snapshot_id\":\"${EVENT_HUNTER_SNAPSHOT_ID}\"}" \
  "${EVENT_HUNTER_API_URL}/api/v1/investigations/${EVENT_HUNTER_CASE_ID}/check-snapshots" \
  >/dev/null

curl --fail --silent --show-error \
  --cookie "${EVENT_HUNTER_COOKIE_JAR}" \
  "${EVENT_HUNTER_API_URL}/api/v1/investigations/${EVENT_HUNTER_CASE_ID}/check-snapshots" \
  >"${EVENT_HUNTER_LINKS_BEFORE}"

docker compose restart postgres >/dev/null
docker compose up -d --wait postgres >/dev/null
docker compose restart event-hunter-api >/dev/null
docker compose up -d --wait event-hunter-api >/dev/null

curl --fail --silent --show-error \
  --cookie "${EVENT_HUNTER_COOKIE_JAR}" \
  "${EVENT_HUNTER_API_URL}/api/v1/check-snapshots/${EVENT_HUNTER_SNAPSHOT_ID}" \
  >"${EVENT_HUNTER_SNAPSHOT_AFTER}"

curl --fail --silent --show-error \
  --cookie "${EVENT_HUNTER_COOKIE_JAR}" \
  "${EVENT_HUNTER_API_URL}/api/v1/investigations/${EVENT_HUNTER_CASE_ID}/check-snapshots" \
  >"${EVENT_HUNTER_LINKS_AFTER}"

EVENT_HUNTER_SNAPSHOT_PROJECTION='{
  id,
  normalized_request,
  as_of,
  source_health,
  model,
  event_set_hash,
  evaluation_hash,
  result,
  event_references,
  relationships,
  finding_feedback,
  idempotency_key
}'

jq --argjson before "$(jq "${EVENT_HUNTER_SNAPSHOT_PROJECTION}" "${EVENT_HUNTER_SNAPSHOT_BEFORE}")" \
  -e "${EVENT_HUNTER_SNAPSHOT_PROJECTION} == \$before" \
  "${EVENT_HUNTER_SNAPSHOT_AFTER}" >/dev/null

jq --arg snapshot_id "${EVENT_HUNTER_SNAPSHOT_ID}" \
  --arg investigation_id "${EVENT_HUNTER_CASE_ID}" \
  -e 'length == 1 and .[0].snapshot_id == $snapshot_id and .[0].investigation_id == $investigation_id' \
  "${EVENT_HUNTER_LINKS_BEFORE}" >/dev/null

jq --argjson before "$(jq -S . "${EVENT_HUNTER_LINKS_BEFORE}")" \
  -e '. == $before' \
  "${EVENT_HUNTER_LINKS_AFTER}" >/dev/null

EVENT_HUNTER_CASE_AFTER_HEADERS="${EVENT_HUNTER_TEST_TMP}/case-after-headers.txt"
curl --fail --silent --show-error \
  --cookie "${EVENT_HUNTER_COOKIE_JAR}" \
  --dump-header "${EVENT_HUNTER_CASE_AFTER_HEADERS}" \
  "${EVENT_HUNTER_API_URL}/api/v1/investigations/${EVENT_HUNTER_CASE_ID}" \
  >/dev/null
EVENT_HUNTER_CASE_AFTER_ETAG="$(awk 'tolower($1) == "etag:" { gsub("\r", "", $2); print $2 }' "${EVENT_HUNTER_CASE_AFTER_HEADERS}")"
if [[ -z "${EVENT_HUNTER_CASE_AFTER_ETAG}" ]]; then
  echo "Get Investigation response did not include ETag after restart" >&2
  exit 1
fi

curl --fail --silent --show-error \
  --cookie "${EVENT_HUNTER_COOKIE_JAR}" \
  --header 'Content-Type: application/json' \
  --header "If-Match: ${EVENT_HUNTER_CASE_AFTER_ETAG}" \
  --data '{"root_cause":"EH-ECM-006 restart acceptance","resolution_summary":"Snapshot and Case link survived PostgreSQL and API restart"}' \
  "${EVENT_HUNTER_API_URL}/api/v1/investigations/${EVENT_HUNTER_CASE_ID}/close" \
  >/dev/null

trap - EXIT
rm -rf -- "${EVENT_HUNTER_TEST_TMP}"

echo "Event Check restart persistence verified: Snapshot ${EVENT_HUNTER_SNAPSHOT_ID}, Case ${EVENT_HUNTER_CASE_ID}."
