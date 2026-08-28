#!/usr/bin/env bash
set -euo pipefail

if [[ "${1:-}" != "--yes" ]]; then
  echo "此測試會暫停本機 ClickHouse，再確認 source offset 在 sink 恢復前不會提交。" >&2
  echo "確認可短暫中斷本機 Event Hunter 後，請執行：bash scripts/test-ingestion-acknowledgement.sh --yes" >&2
  exit 2
fi

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

EVENT_HUNTER_TEST_SUFFIX="$(date -u +%Y%m%d%H%M%S)-$$"
EVENT_HUNTER_EVENT_ID="evt-ingestion-ack-${EVENT_HUNTER_TEST_SUFFIX}"
EVENT_HUNTER_CORRELATION_ID="INGESTION-ACK-${EVENT_HUNTER_TEST_SUFFIX}"
EVENT_HUNTER_CLICKHOUSE_RESTORED=false

restore_clickhouse() {
  if [[ "${EVENT_HUNTER_CLICKHOUSE_RESTORED}" != "true" ]]; then
    docker compose start clickhouse >/dev/null
    docker compose up -d --wait clickhouse >/dev/null
    EVENT_HUNTER_CLICKHOUSE_RESTORED=true
  fi
}
trap restore_clickhouse EXIT

group_partition_row() {
  docker compose exec -T redpanda rpk group describe connect-event-hunter-poc-raw-landing \
    | awk '$1 == "order.events" && $2 == 0 {print $3 " " $6}'
}

bash scripts/clickhouse-mv-poc-up.sh >/dev/null

read -r EVENT_HUNTER_CURRENT_OFFSET EVENT_HUNTER_CURRENT_LAG <<<"$(group_partition_row)"
if [[ "${EVENT_HUNTER_CURRENT_LAG}" != "0" ]]; then
  echo "order.events partition 0 must be caught up before acknowledgement test; lag=${EVENT_HUNTER_CURRENT_LAG}" >&2
  exit 1
fi

docker compose stop clickhouse >/dev/null

EVENT_HUNTER_PAYLOAD="$(printf '{"eventId":"%s","eventType":"OrderCreated","eventVersion":1,"occurredAt":"2026-08-21T06:30:00Z","producer":"ingestion-ack-e2e","correlationId":"%s","causationId":null,"traceId":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","aggregateType":"Order","aggregateId":"%s","sequence":1,"payload":{"orderId":"%s","customerId":"CUSTOMER-ACK","totalAmount":108,"currency":"TWD"}}' "${EVENT_HUNTER_EVENT_ID}" "${EVENT_HUNTER_CORRELATION_ID}" "${EVENT_HUNTER_CORRELATION_ID}" "${EVENT_HUNTER_CORRELATION_ID}")"
EVENT_HUNTER_PRODUCE_RESULT="$(printf '%s\n' "${EVENT_HUNTER_PAYLOAD}" | docker compose exec -T redpanda rpk topic produce order.events -p 0 -k "${EVENT_HUNTER_EVENT_ID}" -o '%p %o\n')"
read -r EVENT_HUNTER_SOURCE_PARTITION EVENT_HUNTER_SOURCE_OFFSET <<<"${EVENT_HUNTER_PRODUCE_RESULT}"

EVENT_HUNTER_WITHHELD=false
for _ in {1..15}; do
  read -r EVENT_HUNTER_CURRENT_OFFSET EVENT_HUNTER_CURRENT_LAG <<<"$(group_partition_row)"
  if [[ "${EVENT_HUNTER_CURRENT_OFFSET}" == "${EVENT_HUNTER_SOURCE_OFFSET}" && "${EVENT_HUNTER_CURRENT_LAG}" == "1" ]]; then
    EVENT_HUNTER_WITHHELD=true
    break
  fi
  sleep 1
done
if [[ "${EVENT_HUNTER_WITHHELD}" != "true" ]]; then
  echo "source offset was not observably withheld: produced=${EVENT_HUNTER_SOURCE_OFFSET}, group=${EVENT_HUNTER_CURRENT_OFFSET}, lag=${EVENT_HUNTER_CURRENT_LAG}" >&2
  exit 1
fi

restore_clickhouse

EVENT_HUNTER_EXPECTED_OFFSET="$((EVENT_HUNTER_SOURCE_OFFSET + 1))"
EVENT_HUNTER_COMMITTED=false
for _ in {1..30}; do
  read -r EVENT_HUNTER_CURRENT_OFFSET EVENT_HUNTER_CURRENT_LAG <<<"$(group_partition_row)"
  if [[ "${EVENT_HUNTER_CURRENT_OFFSET}" == "${EVENT_HUNTER_EXPECTED_OFFSET}" && "${EVENT_HUNTER_CURRENT_LAG}" == "0" ]]; then
    EVENT_HUNTER_COMMITTED=true
    break
  fi
  sleep 1
done
if [[ "${EVENT_HUNTER_COMMITTED}" != "true" ]]; then
  echo "source offset did not commit after ClickHouse recovery: expected=${EVENT_HUNTER_EXPECTED_OFFSET}, group=${EVENT_HUNTER_CURRENT_OFFSET}, lag=${EVENT_HUNTER_CURRENT_LAG}" >&2
  exit 1
fi

EVENT_HUNTER_ROW_COUNT="$(docker compose exec -T clickhouse clickhouse-client \
  --user "${CLICKHOUSE_USER:-event_hunter}" \
  --password "${CLICKHOUSE_PASSWORD:-event_hunter_local_only}" \
  --database "${CLICKHOUSE_DB:-event_hunter}" \
  --query "SELECT uniqExact(tuple(kafka_topic,kafka_partition,kafka_offset)) FROM poc_forensics_events WHERE event_id='${EVENT_HUNTER_EVENT_ID}'" \
  | tr -d '[:space:]')"
if [[ "${EVENT_HUNTER_ROW_COUNT}" != "1" ]]; then
  echo "recovered event delivery identity count=${EVENT_HUNTER_ROW_COUNT}, want 1" >&2
  exit 1
fi

# Offset commit 已確認後才可刪除 probe rows；提前刪除會讓未提交 record
# 在 connector restart 時重送，造成下一個測試永遠保留一筆 lag。
docker compose exec -T clickhouse clickhouse-client \
  --user "${CLICKHOUSE_USER:-event_hunter}" \
  --password "${CLICKHOUSE_PASSWORD:-event_hunter_local_only}" \
  --database "${CLICKHOUSE_DB:-event_hunter}" \
  --query "ALTER TABLE poc_forensics_events DELETE WHERE event_id='${EVENT_HUNTER_EVENT_ID}' SETTINGS mutations_sync=1" \
  >/dev/null
docker compose exec -T clickhouse clickhouse-client \
  --user "${CLICKHOUSE_USER:-event_hunter}" \
  --password "${CLICKHOUSE_PASSWORD:-event_hunter_local_only}" \
  --database "${CLICKHOUSE_DB:-event_hunter}" \
  --query "ALTER TABLE event_hunter_poc.poc_event_landing_raw DELETE WHERE event_id='${EVENT_HUNTER_EVENT_ID}' SETTINGS mutations_sync=1" \
  >/dev/null

echo "ClickHouse Sink acknowledgement verified: offset ${EVENT_HUNTER_SOURCE_OFFSET} remained uncommitted while ClickHouse was down, then committed as ${EVENT_HUNTER_EXPECTED_OFFSET} after recovery."
