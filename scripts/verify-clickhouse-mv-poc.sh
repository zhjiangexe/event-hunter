#!/usr/bin/env bash
set -euo pipefail

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

if [[ -n "${1:-}" && "${1:-}" != "--restart" ]]; then
  echo "用法：bash scripts/verify-clickhouse-mv-poc.sh [--restart]" >&2
  exit 2
fi

EVENT_HUNTER_VERIFY_RESTART=false
if [[ "${1:-}" == "--restart" ]]; then
  EVENT_HUNTER_VERIFY_RESTART=true
fi
EVENT_HUNTER_VERIFY_MARKER="${POC_VERIFY_MARKER:-}"

clickhouse_query() {
  docker compose exec -T clickhouse sh -ec \
    'clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB" --query "$1"' \
    -- "$1"
}

wait_for_connector() {
  local EVENT_HUNTER_ATTEMPT
  for EVENT_HUNTER_ATTEMPT in {1..60}; do
    if curl --fail --silent --show-error \
      "http://localhost:${CLICKHOUSE_POC_CONNECT_REST_PORT:-28345}/connectors/event-hunter-poc-raw-landing/status" \
      | python3 -c '
import json
import sys

status = json.load(sys.stdin)
tasks = status.get("tasks", [])
ready = status.get("connector", {}).get("state") == "RUNNING" and tasks and all(
    task.get("state") == "RUNNING" for task in tasks
)
raise SystemExit(0 if ready else 1)
' 2>/dev/null; then
      return 0
    fi
    sleep 1
  done
  echo "POC connector 未在 60 秒內恢復 RUNNING。" >&2
  return 1
}

assert_counts_consistent() {
  local EVENT_HUNTER_RAW_COUNT
  local EVENT_HUNTER_OUTPUT_COUNT
  local EVENT_HUNTER_UNIQUE_TRANSPORT_COUNT
  local EVENT_HUNTER_RAW_FILTER=""
  local EVENT_HUNTER_PROMOTED_FILTER=""
  local EVENT_HUNTER_FAILURE_FILTER=""

  if [[ -n "${EVENT_HUNTER_VERIFY_MARKER}" ]]; then
    EVENT_HUNTER_RAW_FILTER="WHERE position(raw_payload, '${EVENT_HUNTER_VERIFY_MARKER}') > 0"
    EVENT_HUNTER_PROMOTED_FILTER="WHERE startsWith(event_id, '${EVENT_HUNTER_VERIFY_MARKER}')"
    EVENT_HUNTER_FAILURE_FILTER="WHERE payload_sha256 IN (SELECT payload_sha256 FROM event_hunter_poc.poc_event_landing_raw WHERE position(raw_payload, '${EVENT_HUNTER_VERIFY_MARKER}') > 0)"
  fi

  EVENT_HUNTER_RAW_COUNT="$(clickhouse_query "SELECT count() FROM event_hunter_poc.poc_event_landing_raw FINAL ${EVENT_HUNTER_RAW_FILTER}")"
  EVENT_HUNTER_OUTPUT_COUNT="$(clickhouse_query "SELECT (SELECT count() FROM event_hunter.poc_forensics_events FINAL ${EVENT_HUNTER_PROMOTED_FILTER}) + (SELECT count() FROM event_hunter.poc_event_admission_failures FINAL ${EVENT_HUNTER_FAILURE_FILTER})")"
  EVENT_HUNTER_UNIQUE_TRANSPORT_COUNT="$(clickhouse_query "SELECT uniqExact(tuple(source_topic, source_partition, source_offset)) FROM event_hunter_poc.poc_event_landing_raw ${EVENT_HUNTER_RAW_FILTER}")"

  if [[ "${EVENT_HUNTER_RAW_COUNT}" != "${EVENT_HUNTER_OUTPUT_COUNT}" ]]; then
    echo "POC 分流不守恆：raw=${EVENT_HUNTER_RAW_COUNT}, promoted+quarantined=${EVENT_HUNTER_OUTPUT_COUNT}。" >&2
    return 1
  fi
  if [[ "${EVENT_HUNTER_RAW_COUNT}" != "${EVENT_HUNTER_UNIQUE_TRANSPORT_COUNT}" ]]; then
    echo "POC raw landing 的 FINAL rows 與 transport identity 不一致：rows=${EVENT_HUNTER_RAW_COUNT}, unique=${EVENT_HUNTER_UNIQUE_TRANSPORT_COUNT}。" >&2
    return 1
  fi

  printf '%s' "${EVENT_HUNTER_RAW_COUNT}"
}

wait_for_connector

# grafana_reader 只能讀安全的 event_hunter.* read models，不得讀取 raw database。
if docker compose exec -T clickhouse clickhouse-client \
  --user grafana_reader \
  --password grafana_reader_local_only \
  --query "SELECT count() FROM event_hunter_poc.poc_event_landing_raw" >/dev/null 2>&1; then
  echo "grafana_reader 不應能讀取 event_hunter_poc raw landing。" >&2
  exit 1
fi

EVENT_HUNTER_COUNT_BEFORE="$(assert_counts_consistent)"

if [[ "${EVENT_HUNTER_VERIFY_RESTART}" == "true" ]]; then
  docker compose --profile clickhouse-mv-poc restart kafka-connect-clickhouse-poc >/dev/null
  docker compose --profile clickhouse-mv-poc up -d --wait kafka-connect-clickhouse-poc >/dev/null
  wait_for_connector
  EVENT_HUNTER_COUNT_AFTER="$(assert_counts_consistent)"
  if [[ "${EVENT_HUNTER_COUNT_AFTER}" != "${EVENT_HUNTER_COUNT_BEFORE}" ]]; then
    echo "POC raw count 在 connector restart 後改變：${EVENT_HUNTER_COUNT_BEFORE} -> ${EVENT_HUNTER_COUNT_AFTER}。" >&2
    exit 1
  fi
fi

if [[ -n "${EVENT_HUNTER_VERIFY_MARKER}" ]]; then
  echo "ClickHouse MV POC verified: connector RUNNING, raw access isolated, ${EVENT_HUNTER_COUNT_BEFORE} marker-scoped transport deliveries conserved."
else
  echo "ClickHouse MV POC verified: connector RUNNING, raw access isolated, ${EVENT_HUNTER_COUNT_BEFORE} transport deliveries conserved."
fi
