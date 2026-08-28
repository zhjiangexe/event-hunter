#!/usr/bin/env bash
set -euo pipefail

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

EVENT_HUNTER_TOPICS=(
  order.events
  payment.events
  shipping.events
  event-lab.events
  event-hunter.processing-attempts
  event-hunter.ingestion-failures.dlq
  event-hunter.poc.events
  event-hunter.poc-clickhouse-sink.dlq
)

for EVENT_HUNTER_TOPIC in "${EVENT_HUNTER_TOPICS[@]}"; do
  docker compose exec -T redpanda rpk topic create "${EVENT_HUNTER_TOPIC}" \
    --if-not-exists --partitions 3 --replicas 1
done

echo "Event Hunter Kafka topics are ready."
