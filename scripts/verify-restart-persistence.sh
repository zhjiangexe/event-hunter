#!/usr/bin/env bash
set -euo pipefail

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

postgres_count() {
  docker compose exec -T postgres psql \
    --username "${POSTGRES_USER:-event_hunter}" \
    --dbname "${POSTGRES_DB:-event_hunter}" \
    --tuples-only --no-align \
    --command "SELECT count(*) FROM investigation_cases;" | tr -d '[:space:]'
}

clickhouse_count() {
  curl --fail --silent --show-error \
    --user "${CLICKHOUSE_USER:-event_hunter}:${CLICKHOUSE_PASSWORD:-event_hunter_local_only}" \
    --data-binary "SELECT count() FROM canonical_forensics_events" \
    "http://localhost:${CLICKHOUSE_HTTP_PORT:-28317}/?database=${CLICKHOUSE_DB:-event_hunter}" | tr -d '[:space:]'
}

redpanda_topics() {
  docker compose exec -T redpanda rpk topic list -X brokers=localhost:9092 --format json \
    | python3 -c 'import json,sys; print("\n".join(sorted(item["name"] for item in json.load(sys.stdin))))'
}

EVENT_HUNTER_POSTGRES_BEFORE="$(postgres_count)"
EVENT_HUNTER_CLICKHOUSE_BEFORE="$(clickhouse_count)"
EVENT_HUNTER_TOPICS_BEFORE="$(redpanda_topics)"
EVENT_HUNTER_RESTARTED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

docker compose restart \
  postgres demo-order-postgres demo-payment-postgres demo-shipping-postgres \
  clickhouse redpanda prometheus loki tempo grafana
docker compose up -d --wait \
  postgres demo-order-postgres demo-payment-postgres demo-shipping-postgres \
  clickhouse redpanda prometheus loki tempo grafana

# Redpanda／PostgreSQL 重啟期間，既有 consumer group 可能停在 UNKNOWN_MEMBER_ID，
# Debezium task 也可能因一次連線失敗保持 FAILED；依賴服務必須在核心儲存就緒後重新加入。
docker compose restart \
  kafka-connect kafka-connect-clickhouse-poc technical-dlq-projector \
  demo-order-service demo-payment-service demo-shipping-service
docker compose up -d --wait \
  kafka-connect kafka-connect-clickhouse-poc technical-dlq-projector \
  demo-order-service demo-payment-service demo-shipping-service
bash scripts/register-debezium-connectors.sh
bash scripts/verify-event-pipeline-readiness.sh

# Restart the control plane only after its required dependencies recovered.
# The API must handle SIGTERM, drain active HTTP work and log completion before
# Compose starts the replacement process.
docker compose restart event-hunter-api quality-worker event-lab event-hunter-frontend
docker compose up -d --wait event-hunter-api quality-worker event-lab event-hunter-frontend
if ! docker compose logs --since "${EVENT_HUNTER_RESTARTED_AT}" event-hunter-api \
  | grep -F "Event Hunter API shutdown complete" >/dev/null; then
  echo "Event Hunter API did not report a completed graceful shutdown." >&2
  exit 1
fi

EVENT_HUNTER_POSTGRES_AFTER="$(postgres_count)"
EVENT_HUNTER_CLICKHOUSE_AFTER="$(clickhouse_count)"
EVENT_HUNTER_TOPICS_AFTER="$(redpanda_topics)"

if (( EVENT_HUNTER_POSTGRES_AFTER < EVENT_HUNTER_POSTGRES_BEFORE )); then
  echo "PostgreSQL investigation row count decreased across restart: ${EVENT_HUNTER_POSTGRES_BEFORE} -> ${EVENT_HUNTER_POSTGRES_AFTER}." >&2
  exit 1
fi
if (( EVENT_HUNTER_CLICKHOUSE_AFTER < EVENT_HUNTER_CLICKHOUSE_BEFORE )); then
  echo "ClickHouse forensics row count decreased across restart: ${EVENT_HUNTER_CLICKHOUSE_BEFORE} -> ${EVENT_HUNTER_CLICKHOUSE_AFTER}." >&2
  exit 1
fi

while IFS= read -r EVENT_HUNTER_TOPIC; do
  [[ -z "${EVENT_HUNTER_TOPIC}" ]] && continue
  if ! grep -Fqx "${EVENT_HUNTER_TOPIC}" <<<"${EVENT_HUNTER_TOPICS_AFTER}"; then
    echo "Redpanda topic disappeared across restart: ${EVENT_HUNTER_TOPIC}." >&2
    exit 1
  fi
done <<<"${EVENT_HUNTER_TOPICS_BEFORE}"

bash scripts/verify-persistence.sh
bash scripts/verify-grafana-provisioning.sh

echo "Restart persistence verified: PostgreSQL ${EVENT_HUNTER_POSTGRES_BEFORE}->${EVENT_HUNTER_POSTGRES_AFTER}, ClickHouse ${EVENT_HUNTER_CLICKHOUSE_BEFORE}->${EVENT_HUNTER_CLICKHOUSE_AFTER}, Redpanda topics preserved."
