#!/usr/bin/env bash
set -euo pipefail

# Read-only smoke check for the persisted local infrastructure contract.
EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

docker compose config --quiet

for EVENT_HUNTER_VOLUME in \
  event-hunter_postgres_data \
  event-hunter_demo_order_postgres_data \
  event-hunter_demo_payment_postgres_data \
  event-hunter_demo_shipping_postgres_data \
  event-hunter_clickhouse_data \
  event-hunter_redpanda_data \
  event-hunter_prometheus_data \
  event-hunter_loki_data \
  event-hunter_tempo_data \
  event-hunter_grafana_data; do
  docker volume inspect "${EVENT_HUNTER_VOLUME}" >/dev/null
done

curl --fail --silent http://localhost:"${KAFKA_CONNECT_REST_PORT:-28324}"/connectors >/dev/null
docker compose exec -T redpanda rpk topic list -X brokers=localhost:9092 >/dev/null
curl --fail --silent http://localhost:"${CLICKHOUSE_HTTP_PORT:-28317}"/ping >/dev/null
curl --fail --silent http://localhost:"${GRAFANA_PORT:-28332}"/api/health >/dev/null
curl --fail --silent http://localhost:"${QUALITY_WORKER_HEALTH_PORT:-28338}"/health/ready >/dev/null

echo "Event Hunter persistence configuration is present and services are reachable."
