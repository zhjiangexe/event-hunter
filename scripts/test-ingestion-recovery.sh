#!/usr/bin/env bash
set -euo pipefail

if [[ "${1:-}" != "--yes" ]]; then
  echo "此測試會短暫停止本機 Redpanda，驗證 ingestion 自動恢復與 readiness。" >&2
  echo "確認可中斷本機 Event Hunter 後，請執行：bash scripts/test-ingestion-recovery.sh --yes" >&2
  exit 2
fi

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

EVENT_HUNTER_API_READY_URL="${EVENT_HUNTER_API_READY_URL:-http://localhost:28333/health/ready}"
EVENT_HUNTER_EVENT_LAB_URL="${EVENT_HUNTER_EVENT_LAB_URL:-http://localhost:28343}"
EVENT_HUNTER_REDPANDA_RESTORED=false

restore_redpanda() {
  if [[ "${EVENT_HUNTER_REDPANDA_RESTORED}" != "true" ]]; then
    docker compose start redpanda >/dev/null
    docker compose up -d --wait redpanda >/dev/null
    EVENT_HUNTER_REDPANDA_RESTORED=true
  fi
}
trap restore_redpanda EXIT

api_status() {
  curl --silent --output /tmp/event-hunter-api-readiness.json --write-out '%{http_code}' \
    "${EVENT_HUNTER_API_READY_URL}" || true
}

wait_for_api_status() {
  local EVENT_HUNTER_EXPECTED_STATUS="$1"
  local EVENT_HUNTER_ATTEMPTS="$2"
  for ((EVENT_HUNTER_ATTEMPT = 1; EVENT_HUNTER_ATTEMPT <= EVENT_HUNTER_ATTEMPTS; EVENT_HUNTER_ATTEMPT++)); do
    if [[ "$(api_status)" == "${EVENT_HUNTER_EXPECTED_STATUS}" ]]; then
      return 0
    fi
    sleep 1
  done
  echo "API readiness did not reach HTTP ${EVENT_HUNTER_EXPECTED_STATUS}. Last response:" >&2
  sed -n '1,8p' /tmp/event-hunter-api-readiness.json >&2 || true
  return 1
}

bash scripts/clickhouse-mv-poc-up.sh >/dev/null
bash scripts/verify-event-pipeline-readiness.sh
wait_for_api_status 200 10

EVENT_HUNTER_CONNECT_CONTAINER_ID="$(docker compose ps -q kafka-connect-clickhouse-poc)"
docker compose stop redpanda >/dev/null

# The API must become not-ready while its required Kafka ingestion capability is absent.
wait_for_api_status 503 20

restore_redpanda

# Do not restart or recreate the ClickHouse Sink worker here. Its existing process must
# retry, rejoin the consumer group and catch up by itself.
bash scripts/verify-event-pipeline-readiness.sh
wait_for_api_status 200 20

if [[ "$(docker compose ps -q kafka-connect-clickhouse-poc)" != "${EVENT_HUNTER_CONNECT_CONTAINER_ID}" ]]; then
  echo "ClickHouse Sink worker was recreated instead of recovering in place." >&2
  exit 1
fi

EVENT_HUNTER_RUN_RESPONSE="$(curl --fail --silent --show-error \
  -H 'Content-Type: application/json' \
  -d '{"scenario_id":"S8"}' \
  "${EVENT_HUNTER_EVENT_LAB_URL}/api/v1/scenario-runs")"
EVENT_HUNTER_RUN_ID="$(python3 -c 'import json,sys; print(json.load(sys.stdin)["run_id"])' <<<"${EVENT_HUNTER_RUN_RESPONSE}")"

EVENT_HUNTER_SCENARIO_STATUS=""
EVENT_HUNTER_SCENARIO_RESPONSE=""
for _ in {1..90}; do
  EVENT_HUNTER_SCENARIO_RESPONSE="$(curl --fail --silent --show-error \
    "${EVENT_HUNTER_EVENT_LAB_URL}/api/v1/scenario-runs/${EVENT_HUNTER_RUN_ID}")"
  EVENT_HUNTER_SCENARIO_STATUS="$(python3 -c 'import json,sys; print(json.load(sys.stdin)["status"])' <<<"${EVENT_HUNTER_SCENARIO_RESPONSE}")"
  if [[ "${EVENT_HUNTER_SCENARIO_STATUS}" == "PASSED" ]]; then
    break
  fi
  if [[ "${EVENT_HUNTER_SCENARIO_STATUS}" == "FAILED" || "${EVENT_HUNTER_SCENARIO_STATUS}" == "TIMED_OUT" ]]; then
    echo "Scenario S8 ended as ${EVENT_HUNTER_SCENARIO_STATUS}: ${EVENT_HUNTER_SCENARIO_RESPONSE}" >&2
    exit 1
  fi
  sleep 1
done

if [[ "${EVENT_HUNTER_SCENARIO_STATUS}" != "PASSED" ]]; then
  echo "Scenario S8 did not pass after ingestion recovery: ${EVENT_HUNTER_SCENARIO_RESPONSE}" >&2
  exit 1
fi

bash scripts/verify-event-pipeline-readiness.sh
echo "Ingestion recovery verified: API reported 503 during broker outage, the existing ClickHouse Sink worker rejoined, and Scenario S8 passed."
