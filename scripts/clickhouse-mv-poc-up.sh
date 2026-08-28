#!/usr/bin/env bash
set -euo pipefail

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

EVENT_HUNTER_POC_MIGRATION="backend/migrations/clickhouse/00006_clickhouse_mv_ingestion_poc.sql"
EVENT_HUNTER_CANONICAL_MIGRATION="backend/migrations/clickhouse/00007_canonical_forensics_read_model.sql"
EVENT_HUNTER_TECHNICAL_FAILURE_MIGRATION="backend/migrations/clickhouse/00008_ingestion_technical_failures.sql"
EVENT_HUNTER_ATTEMPT_POC_MIGRATION="backend/migrations/clickhouse/00009_clickhouse_mv_processing_attempts.sql"
EVENT_HUNTER_POC_PAYLOAD_REPAIR="infra/clickhouse/poc-upgrades/payload-object-compatibility.sql"

docker compose --profile clickhouse-mv-poc up -d --build --wait \
  redpanda clickhouse kafka-connect-clickhouse-poc

sed '/^-- +goose Down$/q' "${EVENT_HUNTER_POC_MIGRATION}" \
  | docker compose exec -T clickhouse sh -ec \
    'clickhouse-client --multiquery --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB"'

sed '/^-- +goose Down$/q' "${EVENT_HUNTER_CANONICAL_MIGRATION}" \
  | docker compose exec -T clickhouse sh -ec \
    'clickhouse-client --multiquery --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB"'

sed '/^-- +goose Down$/q' "${EVENT_HUNTER_TECHNICAL_FAILURE_MIGRATION}" \
  | docker compose exec -T clickhouse sh -ec \
    'clickhouse-client --multiquery --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB"'

sed '/^-- +goose Down$/q' "${EVENT_HUNTER_ATTEMPT_POC_MIGRATION}" \
  | docker compose exec -T clickhouse sh -ec \
    'clickhouse-client --multiquery --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB"'

EVENT_HUNTER_POC_PAYLOAD_REPAIR_REQUIRED="$(docker compose exec -T clickhouse sh -ec \
  'clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB" --query "$1"' \
  -- "SELECT if((SELECT position(create_table_query,'JSONExtractRaw') FROM system.tables WHERE database='event_hunter_poc' AND name='poc_valid_events_mv') = 0 OR (SELECT count() FROM event_hunter.poc_forensics_events WHERE JSONType(payload) = 'Array') > 0, 1, 0)")"
if [[ "${EVENT_HUNTER_POC_PAYLOAD_REPAIR_REQUIRED}" == "1" ]]; then
  echo "套用 POC payload object compatibility repair。"
  docker compose exec -T clickhouse sh -ec \
    'clickhouse-client --multiquery --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB"' \
    <"${EVENT_HUNTER_POC_PAYLOAD_REPAIR}"
fi

bash scripts/bootstrap-topics.sh
bash scripts/register-clickhouse-mv-poc.sh
docker compose --profile clickhouse-mv-poc up -d --build --wait technical-dlq-projector

clickhouse_query() {
  docker compose exec -T clickhouse sh -ec \
    'clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB" --query "$1"' \
    -- "$1"
}

# 正式採用後，啟動工具也負責將由舊版 migration 留下的 canonical views
# 對齊到 ClickHouse-first promoted tables。歷史表仍保留，但沒有 writer。
EVENT_HUNTER_DOMAIN_SOURCE="$(clickhouse_query "SELECT if(position(create_table_query,'poc_forensics_events') > 0,'clickhouse-mv','legacy') FROM system.tables WHERE database=currentDatabase() AND name='canonical_forensics_events'")"
if [[ "${EVENT_HUNTER_DOMAIN_SOURCE}" != "clickhouse-mv" ]]; then
  clickhouse_query "EXCHANGE TABLES canonical_forensics_events AND canonical_forensics_events_candidate"
fi

EVENT_HUNTER_ATTEMPT_SOURCE="$(clickhouse_query "SELECT if(position(create_table_query,'poc_event_processing_attempts') > 0,'clickhouse-mv','legacy') FROM system.tables WHERE database=currentDatabase() AND name='canonical_event_processing_attempts'")"
if [[ "${EVENT_HUNTER_ATTEMPT_SOURCE}" != "clickhouse-mv" ]]; then
  clickhouse_query "EXCHANGE TABLES canonical_event_processing_attempts AND canonical_event_processing_attempts_candidate"
fi

echo "ClickHouse-first ingestion 已就緒（內部相容識別仍沿用 poc）："
echo "  Kafka Connect REST: http://localhost:${CLICKHOUSE_POC_CONNECT_REST_PORT:-28345}"
echo "  Raw landing:        event_hunter_poc.poc_event_landing_raw"
echo "  Promoted events:    event_hunter.poc_forensics_events"
echo "  Failure summary:    event_hunter.poc_event_admission_failures"
echo "  Technical failures: event_hunter.ingestion_technical_failures"
echo "  Attempt raw landing: event_hunter_poc.poc_processing_attempt_landing_raw"
echo "  Promoted attempts:   event_hunter.poc_event_processing_attempts"
echo "  Attempt failures:    event_hunter.poc_processing_attempt_admission_failures"
