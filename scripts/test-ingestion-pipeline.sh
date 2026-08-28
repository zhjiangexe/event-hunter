#!/usr/bin/env bash
set -euo pipefail

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

EVENT_HUNTER_CLICKHOUSE_USER="${CLICKHOUSE_USER:-event_hunter}"
EVENT_HUNTER_CLICKHOUSE_PASSWORD="${CLICKHOUSE_PASSWORD:-event_hunter_local_only}"
EVENT_HUNTER_CLICKHOUSE_DB="${CLICKHOUSE_DB:-event_hunter}"
EVENT_HUNTER_TEST_SUFFIX="$(date -u +%Y%m%d%H%M%S)-$$"
EVENT_HUNTER_INVALID_EVENT_ID="evt-ingestion-invalid-${EVENT_HUNTER_TEST_SUFFIX}"
EVENT_HUNTER_INVALID_CORRELATION_ID="INGESTION-INVALID-${EVENT_HUNTER_TEST_SUFFIX}"
EVENT_HUNTER_ATTEMPT_ID="attempt-ingestion-duplicate-${EVENT_HUNTER_TEST_SUFFIX}"
EVENT_HUNTER_SAFE_TO_CLEANUP=false

clickhouse_query() {
  docker compose exec -T clickhouse clickhouse-client \
    --user "${EVENT_HUNTER_CLICKHOUSE_USER}" \
    --password "${EVENT_HUNTER_CLICKHOUSE_PASSWORD}" \
    --database "${EVENT_HUNTER_CLICKHOUSE_DB}" \
    --query "$1"
}

wait_for_value() {
  local description="$1"
  local expected="$2"
  local query="$3"
  local actual=""
  for _ in {1..30}; do
    actual="$(clickhouse_query "${query}" | tr -d '[:space:]')"
    if [[ "${actual}" == "${expected}" ]]; then
      return 0
    fi
    sleep 1
  done
  echo "${description}: expected ${expected}, got ${actual}" >&2
  return 1
}

cleanup_rows() {
  if [[ "${EVENT_HUNTER_SAFE_TO_CLEANUP}" != "true" ]]; then
    echo "保留未確認 offset commit 的 ingestion probe rows，避免刪除尚可能重送的資料。" >&2
    return
  fi
  clickhouse_query "ALTER TABLE event_hunter.poc_event_admission_failures DELETE WHERE event_id='${EVENT_HUNTER_INVALID_EVENT_ID}' SETTINGS mutations_sync=1" >/dev/null || true
  clickhouse_query "ALTER TABLE event_hunter.poc_forensics_events DELETE WHERE event_id='${EVENT_HUNTER_INVALID_EVENT_ID}' SETTINGS mutations_sync=1" >/dev/null || true
  clickhouse_query "ALTER TABLE event_hunter_poc.poc_event_landing_raw DELETE WHERE event_id='${EVENT_HUNTER_INVALID_EVENT_ID}' SETTINGS mutations_sync=1" >/dev/null || true
  clickhouse_query "ALTER TABLE event_hunter.poc_event_processing_attempts DELETE WHERE attempt_id='${EVENT_HUNTER_ATTEMPT_ID}' SETTINGS mutations_sync=1" >/dev/null || true
  clickhouse_query "ALTER TABLE event_hunter.poc_processing_attempt_admission_failures DELETE WHERE attempt_id='${EVENT_HUNTER_ATTEMPT_ID}' SETTINGS mutations_sync=1" >/dev/null || true
  clickhouse_query "ALTER TABLE event_hunter_poc.poc_processing_attempt_landing_raw DELETE WHERE attempt_id='${EVENT_HUNTER_ATTEMPT_ID}' SETTINGS mutations_sync=1" >/dev/null || true
}
trap cleanup_rows EXIT

wait_for_source_commit() {
  local group="$1"
  local topic="$2"
  local partition="$3"
  local produced_offset="$4"
  local expected_offset="$((produced_offset + 1))"
  local current_offset=""
  for _ in {1..30}; do
    current_offset="$(
      docker compose exec -T redpanda rpk group describe "${group}" \
        | awk -v topic="${topic}" -v partition="${partition}" '$1 == topic && $2 == partition {print $3}'
    )"
    if [[ "${current_offset}" =~ ^[0-9]+$ ]] && ((current_offset >= expected_offset)); then
      return 0
    fi
    sleep 1
  done
  echo "source offset did not commit: group=${group} topic=${topic} partition=${partition} produced=${produced_offset} current=${current_offset}" >&2
  return 1
}

bash scripts/clickhouse-mv-poc-up.sh >/dev/null
bash scripts/verify-event-pipeline-readiness.sh

# Missing currency is valid JSON and has a complete envelope, but violates the
# known OrderCreated payload contract. Raw remains available to privileged
# forensics while only a safe failure summary enters the application model.
EVENT_HUNTER_INVALID_PAYLOAD="$(printf '{"eventId":"%s","eventType":"OrderCreated","eventVersion":1,"occurredAt":"2026-08-21T06:00:00Z","producer":"ingestion-e2e","correlationId":"%s","causationId":null,"traceId":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","aggregateType":"Order","aggregateId":"%s","sequence":1,"payload":{"orderId":"%s","customerId":"CUSTOMER-E2E","totalAmount":99}}' "${EVENT_HUNTER_INVALID_EVENT_ID}" "${EVENT_HUNTER_INVALID_CORRELATION_ID}" "${EVENT_HUNTER_INVALID_CORRELATION_ID}" "${EVENT_HUNTER_INVALID_CORRELATION_ID}")"
EVENT_HUNTER_PAYLOAD_SHA256="$(printf '%s' "${EVENT_HUNTER_INVALID_PAYLOAD}" | python3 -c 'import hashlib,sys; print(hashlib.sha256(sys.stdin.buffer.read()).hexdigest())')"

EVENT_HUNTER_INVALID_PRODUCE_RESULT="$(
  printf '%s\n' "${EVENT_HUNTER_INVALID_PAYLOAD}" \
    | docker compose exec -T redpanda rpk topic produce order.events -p 0 -k "${EVENT_HUNTER_INVALID_EVENT_ID}" -o '%p %o\n'
)"
read -r EVENT_HUNTER_INVALID_PARTITION EVENT_HUNTER_INVALID_OFFSET <<<"${EVENT_HUNTER_INVALID_PRODUCE_RESULT}"

wait_for_value "invalid event raw identity" "1" \
  "SELECT uniqExact(tuple(source_topic,source_partition,source_offset)) FROM event_hunter_poc.poc_event_landing_raw WHERE event_id='${EVENT_HUNTER_INVALID_EVENT_ID}'"
wait_for_value "invalid event admission summary" "1" \
  "SELECT count() FROM event_hunter.poc_event_admission_failures WHERE event_id='${EVENT_HUNTER_INVALID_EVENT_ID}' AND error_code='SCHEMA_VIOLATION' AND payload_sha256='${EVENT_HUNTER_PAYLOAD_SHA256}'"
wait_for_value "invalid event must not be searchable" "0" \
  "SELECT count() FROM event_hunter.poc_forensics_events FINAL WHERE event_id='${EVENT_HUNTER_INVALID_EVENT_ID}'"

if clickhouse_query "DESCRIBE TABLE event_hunter.poc_event_admission_failures FORMAT TSVRaw" | awk '{print $1}' | grep -Eq '^(raw_payload|raw_payload_base64)$'; then
  echo "admission failure summary must not expose raw payload" >&2
  exit 1
fi

EVENT_HUNTER_ATTEMPT_PAYLOAD="$(printf '{"attemptId":"%s","eventId":"evt-order-1001-001","eventType":"OrderCreated","correlationId":"ORDER-1001","traceId":"11111111111111111111111111111111","consumerGroupId":"payment-service-v1","consumerService":"payment-service","attempt":1,"processingStatus":"SUCCEEDED","retryReason":null,"retryTopic":null,"kafkaTopic":"order.events","kafkaPartition":0,"kafkaOffset":1000,"startedAt":"2026-08-21T06:15:00Z","completedAt":"2026-08-21T06:15:00.050Z","observedAt":"2026-08-21T06:15:00.051Z"}' "${EVENT_HUNTER_ATTEMPT_ID}")"
EVENT_HUNTER_ATTEMPT_PRODUCE_RESULT="$(
  printf '%s\n%s\n' "${EVENT_HUNTER_ATTEMPT_PAYLOAD}" "${EVENT_HUNTER_ATTEMPT_PAYLOAD}" \
    | docker compose exec -T redpanda rpk topic produce event-hunter.processing-attempts -p 0 -k "${EVENT_HUNTER_ATTEMPT_ID}" -o '%p %o\n'
)"
read -r EVENT_HUNTER_ATTEMPT_PARTITION EVENT_HUNTER_ATTEMPT_OFFSET <<<"$(tail -n 1 <<<"${EVENT_HUNTER_ATTEMPT_PRODUCE_RESULT}")"

wait_for_value "processing-attempt raw redelivery" "2" \
  "SELECT count() FROM event_hunter_poc.poc_processing_attempt_landing_raw WHERE attempt_id='${EVENT_HUNTER_ATTEMPT_ID}'"
wait_for_value "processing-attempt logical identity" "1" \
  "SELECT count() FROM event_hunter.poc_event_processing_attempts FINAL WHERE attempt_id='${EVENT_HUNTER_ATTEMPT_ID}'"

wait_for_source_commit \
  connect-event-hunter-poc-raw-landing order.events \
  "${EVENT_HUNTER_INVALID_PARTITION}" "${EVENT_HUNTER_INVALID_OFFSET}"
wait_for_source_commit \
  connect-event-hunter-poc-processing-attempt-raw-landing event-hunter.processing-attempts \
  "${EVENT_HUNTER_ATTEMPT_PARTITION}" "${EVENT_HUNTER_ATTEMPT_OFFSET}"
EVENT_HUNTER_SAFE_TO_CLEANUP=true

echo "ClickHouse-first ingestion verified: invalid event retained raw and quarantined safely; duplicate processing attempt remained one logical attempt."
