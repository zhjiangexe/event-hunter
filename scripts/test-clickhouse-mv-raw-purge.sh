#!/usr/bin/env bash
set -euo pipefail

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

bash scripts/clickhouse-mv-poc-up.sh

EVENT_HUNTER_PURGE_TOKEN="$(date -u +%Y%m%d%H%M%S)-$$"
EVENT_HUNTER_PURGE_TOPIC="e2e.poc.raw-purge.${EVENT_HUNTER_PURGE_TOKEN}"
EVENT_HUNTER_PURGE_FROM="$(python3 -c 'import datetime; print((datetime.datetime.now(datetime.timezone.utc)-datetime.timedelta(hours=4)).isoformat().replace("+00:00","Z"))')"
EVENT_HUNTER_PURGE_ROW_TIME="$(python3 -c 'import datetime; print((datetime.datetime.now(datetime.timezone.utc)-datetime.timedelta(hours=3)).isoformat().replace("+00:00","Z"))')"
EVENT_HUNTER_PURGE_TO="$(python3 -c 'import datetime; print((datetime.datetime.now(datetime.timezone.utc)-datetime.timedelta(hours=2)).isoformat().replace("+00:00","Z"))')"

clickhouse_query() {
  docker compose exec -T clickhouse sh -ec \
    'clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB" --query "$1"' \
    -- "$1"
}

cleanup_probe() {
  clickhouse_query "ALTER TABLE event_hunter_poc.poc_event_landing_raw DELETE WHERE source_topic = '${EVENT_HUNTER_PURGE_TOPIC}' SETTINGS mutations_sync = 1" >/dev/null || true
  clickhouse_query "ALTER TABLE event_hunter.poc_event_admission_failures DELETE WHERE source_topic = '${EVENT_HUNTER_PURGE_TOPIC}' SETTINGS mutations_sync = 1" >/dev/null || true
}
trap cleanup_probe EXIT

clickhouse_query "INSERT INTO event_hunter_poc.poc_event_landing_raw (raw_payload, source_topic, source_partition, source_offset, received_at) VALUES ('{broken-json', '${EVENT_HUNTER_PURGE_TOPIC}', 0, 1, parseDateTime64BestEffort('${EVENT_HUNTER_PURGE_ROW_TIME}'))" >/dev/null

EVENT_HUNTER_PURGE_DRY_RUN="$(bash scripts/purge-clickhouse-mv-raw.sh --from "${EVENT_HUNTER_PURGE_FROM}" --to "${EVENT_HUNTER_PURGE_TO}")"
if [[ "${EVENT_HUNTER_PURGE_DRY_RUN}" != *"rows=1"* ]]; then
  echo "raw purge dry-run 未精確找到 probe：${EVENT_HUNTER_PURGE_DRY_RUN}" >&2
  exit 1
fi
if [[ "$(clickhouse_query "SELECT count() FROM event_hunter_poc.poc_event_landing_raw FINAL WHERE source_topic = '${EVENT_HUNTER_PURGE_TOPIC}'")" != "1" ]]; then
  echo "raw purge dry-run 不應刪除 probe。" >&2
  exit 1
fi

bash scripts/purge-clickhouse-mv-raw.sh \
  --from "${EVENT_HUNTER_PURGE_FROM}" \
  --to "${EVENT_HUNTER_PURGE_TO}" \
  --execute --yes

if [[ "$(clickhouse_query "SELECT count() FROM event_hunter_poc.poc_event_landing_raw FINAL WHERE source_topic = '${EVENT_HUNTER_PURGE_TOPIC}'")" != "0" ]]; then
  echo "raw purge 未刪除時間窗內 probe。" >&2
  exit 1
fi

if bash scripts/purge-clickhouse-mv-raw.sh \
  --from "$(python3 -c 'import datetime; print((datetime.datetime.now(datetime.timezone.utc)-datetime.timedelta(minutes=30)).isoformat().replace("+00:00","Z"))')" \
  --to "$(python3 -c 'import datetime; print(datetime.datetime.now(datetime.timezone.utc).isoformat().replace("+00:00","Z"))')" \
  --execute --yes >/dev/null 2>&1; then
  echo "raw purge 不應接受距今不足一小時的時間窗。" >&2
  exit 1
fi

echo "ClickHouse MV raw purge verified: dry-run is non-mutating, execution is bounded, and recent windows are rejected."
