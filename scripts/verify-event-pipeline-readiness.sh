#!/usr/bin/env bash
set -euo pipefail

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

EVENT_HUNTER_CONNECT_URL="${KAFKA_CONNECT_URL:-http://localhost:28324}"
EVENT_HUNTER_CLICKHOUSE_POC_CONNECT_URL="${CLICKHOUSE_POC_CONNECT_URL:-http://localhost:28345}"
EVENT_HUNTER_CLICKHOUSE_POC_CONNECTOR_NAME="${CLICKHOUSE_POC_CONNECTOR_NAME:-event-hunter-poc-raw-landing}"
EVENT_HUNTER_CLICKHOUSE_POC_ATTEMPTS_CONNECTOR_NAME="${CLICKHOUSE_POC_ATTEMPTS_CONNECTOR_NAME:-event-hunter-poc-processing-attempt-raw-landing}"
EVENT_HUNTER_TECHNICAL_PROJECTOR_URL="${TECHNICAL_DLQ_PROJECTOR_URL:-http://localhost:28346}"
EVENT_HUNTER_TIMEOUT_SECONDS="${EVENT_HUNTER_PIPELINE_READY_TIMEOUT_SECONDS:-30}"

connectors_ready() {
  curl --fail --silent --show-error "${EVENT_HUNTER_CONNECT_URL}/connectors?expand=status" 2>/dev/null \
    | python3 -c '
import json
import sys

required = (
    "event-hunter-demo-order-outbox-v1",
    "event-hunter-demo-payment-outbox-v1",
    "event-hunter-demo-shipping-outbox-v1",
)
payload = json.load(sys.stdin)
ready = all(
    name in payload
    and payload[name]["status"]["connector"]["state"] == "RUNNING"
    and payload[name]["status"]["tasks"]
    and all(task["state"] == "RUNNING" for task in payload[name]["status"]["tasks"])
    for name in required
)
raise SystemExit(0 if ready else 1)
' 2>/dev/null
}

kafka_connect_connector_ready() {
  local EVENT_HUNTER_CONNECTOR_NAME="$1"
  curl --fail --silent --show-error \
    "${EVENT_HUNTER_CLICKHOUSE_POC_CONNECT_URL}/connectors/${EVENT_HUNTER_CONNECTOR_NAME}/status" 2>/dev/null \
    | python3 -c '
import json
import sys

status = json.load(sys.stdin)
tasks = status.get("tasks", [])
ready = status.get("connector", {}).get("state") == "RUNNING" and tasks and all(
    task.get("state") == "RUNNING" for task in tasks
)
raise SystemExit(0 if ready else 1)
' 2>/dev/null
}

technical_projector_ready() {
  curl --fail --silent --show-error "${EVENT_HUNTER_TECHNICAL_PROJECTOR_URL}/health/ready" >/dev/null 2>&1
}

active_ingestion_mode() {
  docker compose exec -T clickhouse sh -ec \
    'clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB" --query "$1"' \
    -- "SELECT if(position(create_table_query,'poc_forensics_events') > 0,'clickhouse-mv','legacy') FROM system.tables WHERE database=currentDatabase() AND name='canonical_forensics_events'" 2>/dev/null
}

active_attempt_ingestion_mode() {
  docker compose exec -T clickhouse sh -ec \
    'clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB" --query "$1"' \
    -- "SELECT if(position(create_table_query,'poc_event_processing_attempts') > 0,'clickhouse-mv','legacy') FROM system.tables WHERE database=currentDatabase() AND name='canonical_event_processing_attempts'" 2>/dev/null
}

for ((EVENT_HUNTER_ATTEMPT = 1; EVENT_HUNTER_ATTEMPT <= EVENT_HUNTER_TIMEOUT_SECONDS; EVENT_HUNTER_ATTEMPT++)); do
  EVENT_HUNTER_ACTIVE_INGESTION_MODE="$(active_ingestion_mode || true)"
  EVENT_HUNTER_ACTIVE_ATTEMPTS_INGESTION_MODE="$(active_attempt_ingestion_mode || true)"
  if connectors_ready \
    && [[ "${EVENT_HUNTER_ACTIVE_INGESTION_MODE}" == "clickhouse-mv" ]] \
    && [[ "${EVENT_HUNTER_ACTIVE_ATTEMPTS_INGESTION_MODE}" == "clickhouse-mv" ]] \
    && kafka_connect_connector_ready "${EVENT_HUNTER_CLICKHOUSE_POC_CONNECTOR_NAME}" \
    && kafka_connect_connector_ready "${EVENT_HUNTER_CLICKHOUSE_POC_ATTEMPTS_CONNECTOR_NAME}" \
    && technical_projector_ready; then
    echo "Event pipeline ready: Debezium tasks RUNNING; domain=${EVENT_HUNTER_ACTIVE_INGESTION_MODE} attempts=${EVENT_HUNTER_ACTIVE_ATTEMPTS_INGESTION_MODE}."
    exit 0
  fi
  sleep 1
done

echo "Event pipeline did not become ready within ${EVENT_HUNTER_TIMEOUT_SECONDS}s." >&2
echo "Kafka Connect status:" >&2
curl --silent --show-error "${EVENT_HUNTER_CONNECT_URL}/connectors?expand=status" >&2 || true
echo >&2
echo "Active ingestion mode: ${EVENT_HUNTER_ACTIVE_INGESTION_MODE:-unknown}" >&2
echo "Active processing-attempt ingestion mode: ${EVENT_HUNTER_ACTIVE_ATTEMPTS_INGESTION_MODE:-unknown}" >&2
echo "Domain-event ClickHouse Sink connector status:" >&2
curl --silent --show-error \
  "${EVENT_HUNTER_CLICKHOUSE_POC_CONNECT_URL}/connectors/${EVENT_HUNTER_CLICKHOUSE_POC_CONNECTOR_NAME}/status" >&2 || true
echo >&2
echo "Processing-attempt ClickHouse Sink connector status:" >&2
curl --silent --show-error \
  "${EVENT_HUNTER_CLICKHOUSE_POC_CONNECT_URL}/connectors/${EVENT_HUNTER_CLICKHOUSE_POC_ATTEMPTS_CONNECTOR_NAME}/status" >&2 || true
echo >&2
echo "Technical DLQ projector readiness:" >&2
curl --silent --show-error "${EVENT_HUNTER_TECHNICAL_PROJECTOR_URL}/health/ready" >&2 || true
exit 1
