#!/usr/bin/env bash
set -euo pipefail

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

if [[ "${1:-}" != "--correlation" || -z "${2:-}" || -n "${3:-}" ]]; then
  echo "用法：bash scripts/verify-processing-attempt-shadow.sh --correlation <correlation-id>" >&2
  exit 2
fi

EVENT_HUNTER_CORRELATION_ID="$2"
if [[ ! "${EVENT_HUNTER_CORRELATION_ID}" =~ ^[A-Za-z0-9._:-]+$ ]]; then
  echo "correlation ID 含有不允許的字元。" >&2
  exit 2
fi

clickhouse_query() {
  local statement="$1"
  docker compose exec -T clickhouse sh -ec \
    'clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB" --query "$1"' \
    -- "${statement}"
}

logical_signature() {
  local table="$1"
  clickhouse_query "SELECT concat(toString(count()),'|',toString(groupBitXor(cityHash64(attempt_id,latest_event_id,latest_event_type,latest_correlation_id,latest_consumer_group_id,latest_consumer_service,latest_attempt,latest_processing_status,coalesce(latest_retry_reason,''),coalesce(latest_retry_topic,''),latest_kafka_topic,latest_kafka_partition,latest_kafka_offset,latest_started_at,coalesce(latest_completed_at,toDateTime64(0,3,'UTC')),latest_observed_at)))) FROM (SELECT attempt_id,argMax(event_id,observed_at) AS latest_event_id,argMax(event_type,observed_at) AS latest_event_type,argMax(correlation_id,observed_at) AS latest_correlation_id,argMax(consumer_group_id,observed_at) AS latest_consumer_group_id,argMax(consumer_service,observed_at) AS latest_consumer_service,argMax(attempt,observed_at) AS latest_attempt,argMax(processing_status,observed_at) AS latest_processing_status,argMax(retry_reason,observed_at) AS latest_retry_reason,argMax(retry_topic,observed_at) AS latest_retry_topic,argMax(kafka_topic,observed_at) AS latest_kafka_topic,argMax(kafka_partition,observed_at) AS latest_kafka_partition,argMax(kafka_offset,observed_at) AS latest_kafka_offset,argMax(started_at,observed_at) AS latest_started_at,argMax(completed_at,observed_at) AS latest_completed_at,max(observed_at) AS latest_observed_at FROM ${table} AS source WHERE source.correlation_id='${EVENT_HUNTER_CORRELATION_ID}' GROUP BY attempt_id)"
}

EVENT_HUNTER_LEGACY_SIGNATURE="$(logical_signature event_processing_attempts)"
if [[ "${EVENT_HUNTER_LEGACY_SIGNATURE%%|*}" == "0" ]]; then
  echo "Legacy processing-attempt table 找不到 correlation ${EVENT_HUNTER_CORRELATION_ID}。" >&2
  exit 1
fi

for EVENT_HUNTER_ATTEMPT in {1..60}; do
  EVENT_HUNTER_CANDIDATE_SIGNATURE="$(logical_signature poc_event_processing_attempts)"
  if [[ "${EVENT_HUNTER_CANDIDATE_SIGNATURE}" == "${EVENT_HUNTER_LEGACY_SIGNATURE}" ]]; then
    echo "Processing-attempt shadow parity passed: correlation=${EVENT_HUNTER_CORRELATION_ID} signature=${EVENT_HUNTER_CANDIDATE_SIGNATURE}."
    exit 0
  fi
  sleep 1
done

echo "Processing-attempt shadow parity failed after 60s." >&2
echo "legacy=${EVENT_HUNTER_LEGACY_SIGNATURE} candidate=${EVENT_HUNTER_CANDIDATE_SIGNATURE:-missing}" >&2
exit 1
