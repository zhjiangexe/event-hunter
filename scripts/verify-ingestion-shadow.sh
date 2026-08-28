#!/usr/bin/env bash
set -euo pipefail

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

EVENT_HUNTER_CONFIG_PATH="${EVENT_HUNTER_INGESTION_CONFIG:-config/ingestion-cutover.env}"
if [[ ! -f "${EVENT_HUNTER_CONFIG_PATH}" ]]; then
  echo "找不到 ingestion cutover 設定：${EVENT_HUNTER_CONFIG_PATH}" >&2
  exit 2
fi
# shellcheck disable=SC1090
source "${EVENT_HUNTER_CONFIG_PATH}"

usage() {
  echo "用法：bash scripts/verify-ingestion-shadow.sh --correlation CORRELATION_ID" >&2
}

EVENT_HUNTER_CORRELATION_ID=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --correlation)
      EVENT_HUNTER_CORRELATION_ID="${2:-}"
      shift 2
      ;;
    *)
      usage
      exit 2
      ;;
  esac
done

if [[ ! "${EVENT_HUNTER_CORRELATION_ID}" =~ ^[A-Za-z0-9][A-Za-z0-9._:/-]{0,255}$ ]]; then
  echo "correlation ID 為必填，且只能包含英數、點、底線、冒號、斜線與連字號。" >&2
  exit 2
fi

validate_identifier() {
  local value="$1"
  local label="$2"
  if [[ ! "${value}" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]]; then
    echo "${label} 不是安全的 ClickHouse identifier：${value}" >&2
    exit 2
  fi
}

validate_identifier "${EVENT_HUNTER_LEGACY_EVENTS_TABLE}" "legacy table"
validate_identifier "${EVENT_HUNTER_CANDIDATE_EVENTS_TABLE}" "candidate table"
validate_identifier "${EVENT_HUNTER_CANDIDATE_FAILURE_TABLE}" "candidate failure table"
validate_identifier "${EVENT_HUNTER_RAW_EVENTS_DATABASE}" "raw database"
validate_identifier "${EVENT_HUNTER_RAW_EVENTS_TABLE}" "raw table"

EVENT_HUNTER_CONNECTOR_STATUS_URL="http://localhost:${EVENT_HUNTER_POC_CONNECT_REST_PORT}/connectors/${EVENT_HUNTER_POC_CONNECTOR_NAME}/status"
if ! curl --fail --silent --show-error "${EVENT_HUNTER_CONNECTOR_STATUS_URL}" | python3 -c '
import json
import sys

status = json.load(sys.stdin)
tasks = status.get("tasks", [])
running = status.get("connector", {}).get("state") == "RUNNING" and tasks and all(
    task.get("state") == "RUNNING" for task in tasks
)
raise SystemExit(0 if running else 1)
'; then
  echo "POC connector 尚未 RUNNING：${EVENT_HUNTER_CONNECTOR_STATUS_URL}" >&2
  exit 1
fi

clickhouse_query() {
  local statement="$1"
  docker compose exec -T clickhouse sh -ec \
    'clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB" --param_correlation "$1" --query "$2"' \
    -- "${EVENT_HUNTER_CORRELATION_ID}" "${statement}"
}

EVENT_HUNTER_TEMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/event-hunter-shadow.XXXXXX")"
cleanup_shadow_files() {
  rm -rf -- "${EVENT_HUNTER_TEMP_DIR}"
}
trap cleanup_shadow_files EXIT

parity_query() {
  local table="$1"
  cat <<SQL
SELECT
    event_id,event_type,event_version,occurred_at,producer,correlation_id,
    ifNull(causation_id,''),ifNull(trace_id,''),aggregate_type,aggregate_id,sequence,
    kafka_topic,kafka_partition,kafka_offset,
    toJSONString(CAST(payload, 'JSON')) AS normalized_payload
FROM
(
    SELECT *,row_number() OVER (
        PARTITION BY kafka_topic,kafka_partition,kafka_offset
        ORDER BY ingested_at DESC
    ) AS delivery_rank
    FROM ${table}
    WHERE correlation_id={correlation:String}
)
WHERE delivery_rank=1
ORDER BY kafka_topic,kafka_partition,kafka_offset
FORMAT JSONEachRow
SQL
}

clickhouse_query "$(parity_query "${EVENT_HUNTER_LEGACY_EVENTS_TABLE}")" >"${EVENT_HUNTER_TEMP_DIR}/legacy.jsonl"
clickhouse_query "$(parity_query "${EVENT_HUNTER_CANDIDATE_EVENTS_TABLE}")" >"${EVENT_HUNTER_TEMP_DIR}/candidate.jsonl"

EVENT_HUNTER_LEGACY_COUNT="$(wc -l <"${EVENT_HUNTER_TEMP_DIR}/legacy.jsonl" | tr -d ' ')"
EVENT_HUNTER_CANDIDATE_COUNT="$(wc -l <"${EVENT_HUNTER_TEMP_DIR}/candidate.jsonl" | tr -d ' ')"
EVENT_HUNTER_FAILURE_COUNT="$(clickhouse_query "SELECT count() FROM ${EVENT_HUNTER_CANDIDATE_FAILURE_TABLE} FINAL WHERE correlation_id={correlation:String}")"
EVENT_HUNTER_RAW_COUNT="$(clickhouse_query "SELECT count() FROM ${EVENT_HUNTER_RAW_EVENTS_DATABASE}.${EVENT_HUNTER_RAW_EVENTS_TABLE} FINAL WHERE correlation_id={correlation:String}")"

if [[ "${EVENT_HUNTER_LEGACY_COUNT}" == "0" ]]; then
  echo "legacy path 找不到 correlation ${EVENT_HUNTER_CORRELATION_ID}，拒絕以空集合通過 parity。" >&2
  exit 1
fi
if [[ "${EVENT_HUNTER_FAILURE_COUNT}" != "0" ]]; then
  echo "candidate path 對 correlation ${EVENT_HUNTER_CORRELATION_ID} 產生 ${EVENT_HUNTER_FAILURE_COUNT} 筆 admission failures。" >&2
  exit 1
fi
if [[ "${EVENT_HUNTER_RAW_COUNT}" != "${EVENT_HUNTER_CANDIDATE_COUNT}" ]]; then
  echo "candidate 不守恆：raw=${EVENT_HUNTER_RAW_COUNT}, valid=${EVENT_HUNTER_CANDIDATE_COUNT}, failure=${EVENT_HUNTER_FAILURE_COUNT}。" >&2
  exit 1
fi
if ! cmp -s "${EVENT_HUNTER_TEMP_DIR}/legacy.jsonl" "${EVENT_HUNTER_TEMP_DIR}/candidate.jsonl"; then
  echo "shadow parity 不一致：legacy=${EVENT_HUNTER_LEGACY_COUNT}, candidate=${EVENT_HUNTER_CANDIDATE_COUNT}。" >&2
  EVENT_HUNTER_DIFF="$(diff -u "${EVENT_HUNTER_TEMP_DIR}/legacy.jsonl" "${EVENT_HUNTER_TEMP_DIR}/candidate.jsonl" || true)"
  printf '%s\n' "${EVENT_HUNTER_DIFF}" | sed -n '1,120p' >&2
  exit 1
fi

echo "Shadow parity verified: correlation=${EVENT_HUNTER_CORRELATION_ID}, legacy=${EVENT_HUNTER_LEGACY_COUNT}, candidate=${EVENT_HUNTER_CANDIDATE_COUNT}, failures=0."
