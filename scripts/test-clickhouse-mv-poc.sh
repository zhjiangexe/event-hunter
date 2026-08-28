#!/usr/bin/env bash
set -euo pipefail

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"
source scripts/lib/karate.sh

EVENT_HUNTER_POC_RUN_TOKEN="$(date -u +%Y%m%d%H%M%S)-$$"
export POC_RUN_TOKEN="${EVENT_HUNTER_POC_RUN_TOKEN}"
EVENT_HUNTER_POC_MARKER="E2E-POC-${EVENT_HUNTER_POC_RUN_TOKEN}"
EVENT_HUNTER_ATTEMPT_POC_MARKER="E2E-ATTEMPT-${EVENT_HUNTER_POC_RUN_TOKEN}"
EVENT_HUNTER_OUTPUT_DIR="${KARATE_OUTPUT_DIR:-artifacts/e2e/karate/clickhouse-mv-poc}"
EVENT_HUNTER_TECHNICAL_SOURCE_PARTITION=""
EVENT_HUNTER_TECHNICAL_SOURCE_OFFSET=""

cleanup_poc_rows() {
  docker compose exec -T clickhouse sh -ec \
    'clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB" --query "$1"' \
    -- "ALTER TABLE event_hunter.poc_event_admission_failures DELETE WHERE payload_sha256 IN (SELECT payload_sha256 FROM event_hunter_poc.poc_event_landing_raw WHERE position(raw_payload, '${EVENT_HUNTER_POC_MARKER}') > 0) SETTINGS mutations_sync = 1" >/dev/null || true
  docker compose exec -T clickhouse sh -ec \
    'clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB" --query "$1"' \
    -- "ALTER TABLE event_hunter.poc_forensics_events DELETE WHERE startsWith(event_id, '${EVENT_HUNTER_POC_MARKER}') SETTINGS mutations_sync = 1" >/dev/null || true
  docker compose exec -T clickhouse sh -ec \
    'clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB" --query "$1"' \
    -- "ALTER TABLE event_hunter_poc.poc_event_landing_raw DELETE WHERE position(raw_payload, '${EVENT_HUNTER_POC_MARKER}') > 0 SETTINGS mutations_sync = 1" >/dev/null || true
  docker compose exec -T clickhouse sh -ec \
    'clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB" --query "$1"' \
    -- "ALTER TABLE event_hunter.poc_processing_attempt_admission_failures DELETE WHERE startsWith(attempt_id, '${EVENT_HUNTER_ATTEMPT_POC_MARKER}') SETTINGS mutations_sync = 1" >/dev/null || true
  docker compose exec -T clickhouse sh -ec \
    'clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB" --query "$1"' \
    -- "ALTER TABLE event_hunter.poc_event_processing_attempts DELETE WHERE startsWith(attempt_id, '${EVENT_HUNTER_ATTEMPT_POC_MARKER}') SETTINGS mutations_sync = 1" >/dev/null || true
  docker compose exec -T clickhouse sh -ec \
    'clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB" --query "$1"' \
    -- "ALTER TABLE event_hunter_poc.poc_processing_attempt_landing_raw DELETE WHERE position(raw_payload, '${EVENT_HUNTER_ATTEMPT_POC_MARKER}') > 0 SETTINGS mutations_sync = 1" >/dev/null || true
  if [[ -n "${EVENT_HUNTER_TECHNICAL_SOURCE_PARTITION}" && -n "${EVENT_HUNTER_TECHNICAL_SOURCE_OFFSET}" ]]; then
    docker compose exec -T clickhouse sh -ec \
      'clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB" --query "$1"' \
      -- "ALTER TABLE event_hunter.ingestion_technical_failures DELETE WHERE source_topic = 'event-hunter.poc.events' AND source_partition = ${EVENT_HUNTER_TECHNICAL_SOURCE_PARTITION} AND source_offset = ${EVENT_HUNTER_TECHNICAL_SOURCE_OFFSET} SETTINGS mutations_sync = 1" >/dev/null || true
  fi
}

trap cleanup_poc_rows EXIT

bash scripts/clickhouse-mv-poc-up.sh
EVENT_HUNTER_TECHNICAL_RESULT="$(printf '\n' | docker compose exec -T redpanda rpk topic produce event-hunter.poc.events \
  -k "${EVENT_HUNTER_POC_MARKER}-TECHNICAL-DLQ" -Z -X brokers=localhost:9092 \
  --output-format='%p:%o')"
EVENT_HUNTER_TECHNICAL_COORDINATE="$(printf '%s\n' "${EVENT_HUNTER_TECHNICAL_RESULT}" | tail -n 1 | tr -d '[:space:]')"
EVENT_HUNTER_TECHNICAL_SOURCE_PARTITION="${EVENT_HUNTER_TECHNICAL_COORDINATE%%:*}"
EVENT_HUNTER_TECHNICAL_SOURCE_OFFSET="${EVENT_HUNTER_TECHNICAL_COORDINATE##*:}"
if [[ ! "${EVENT_HUNTER_TECHNICAL_SOURCE_PARTITION}" =~ ^[0-9]+$ || ! "${EVENT_HUNTER_TECHNICAL_SOURCE_OFFSET}" =~ ^[0-9]+$ ]]; then
  echo "無法解析 technical DLQ poison record 座標：${EVENT_HUNTER_TECHNICAL_RESULT}" >&2
  exit 1
fi
export POC_TECHNICAL_SOURCE_PARTITION="${EVENT_HUNTER_TECHNICAL_SOURCE_PARTITION}"
export POC_TECHNICAL_SOURCE_OFFSET="${EVENT_HUNTER_TECHNICAL_SOURCE_OFFSET}"
event_hunter_run_karate "${EVENT_HUNTER_OUTPUT_DIR}" \
  e2e/poc/clickhouse-mv-ingestion.feature \
  e2e/poc/clickhouse-mv-processing-attempt.feature
POC_VERIFY_MARKER="${EVENT_HUNTER_POC_MARKER}" bash scripts/verify-clickhouse-mv-poc.sh --restart
