#!/usr/bin/env bash
set -euo pipefail

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

EVENT_HUNTER_CONNECT_URL="${KAFKA_CONNECT_URL:-http://localhost:28324}"

wait_for_connector() {
  local EVENT_HUNTER_CONNECTOR_NAME="$1"
  local EVENT_HUNTER_ATTEMPT

  for EVENT_HUNTER_ATTEMPT in {1..30}; do
    if curl --fail --silent --show-error \
      "${EVENT_HUNTER_CONNECT_URL}/connectors/${EVENT_HUNTER_CONNECTOR_NAME}/status" 2>/dev/null \
      | python3 -c '
import json
import sys

status = json.load(sys.stdin)
tasks = status.get("tasks", [])
ready = status.get("connector", {}).get("state") == "RUNNING" and tasks and all(
    task.get("state") == "RUNNING" for task in tasks
)
raise SystemExit(0 if ready else 1)
' 2>/dev/null; then
      return 0
    fi
    sleep 1
  done

  echo "connector ${EVENT_HUNTER_CONNECTOR_NAME} did not reach RUNNING within 30s" >&2
  curl --silent --show-error \
    "${EVENT_HUNTER_CONNECT_URL}/connectors/${EVENT_HUNTER_CONNECTOR_NAME}/status" >&2 || true
  echo >&2
  return 1
}

render_connector() {
  local EVENT_HUNTER_TEMPLATE="$1"
  local EVENT_HUNTER_RENDERED
  EVENT_HUNTER_RENDERED="$(mktemp)"
  sed \
    -e "s|\${DEMO_ORDER_POSTGRES_USER}|${DEMO_ORDER_POSTGRES_USER:-demo_order}|g" \
    -e "s|\${DEMO_ORDER_POSTGRES_PASSWORD}|${DEMO_ORDER_POSTGRES_PASSWORD:-demo_order_local_only}|g" \
    -e "s|\${DEMO_PAYMENT_POSTGRES_USER}|${DEMO_PAYMENT_POSTGRES_USER:-demo_payment}|g" \
    -e "s|\${DEMO_PAYMENT_POSTGRES_PASSWORD}|${DEMO_PAYMENT_POSTGRES_PASSWORD:-demo_payment_local_only}|g" \
    -e "s|\${DEMO_SHIPPING_POSTGRES_USER}|${DEMO_SHIPPING_POSTGRES_USER:-demo_shipping}|g" \
    -e "s|\${DEMO_SHIPPING_POSTGRES_PASSWORD}|${DEMO_SHIPPING_POSTGRES_PASSWORD:-demo_shipping_local_only}|g" \
    "${EVENT_HUNTER_TEMPLATE}" > "${EVENT_HUNTER_RENDERED}"
  printf '%s' "${EVENT_HUNTER_RENDERED}"
}

for EVENT_HUNTER_TEMPLATE in infra/debezium/*-connector.json; do
  EVENT_HUNTER_RENDERED="$(render_connector "${EVENT_HUNTER_TEMPLATE}")"
  EVENT_HUNTER_CONNECTOR_NAME="$(sed -n 's/.*"name": "\([^"]*\)".*/\1/p' "${EVENT_HUNTER_RENDERED}" | head -n 1)"
  EVENT_HUNTER_CONFIG="$(mktemp)"
  python3 -c 'import json, sys; print(json.dumps(json.load(open(sys.argv[1]))["config"]))' "${EVENT_HUNTER_RENDERED}" > "${EVENT_HUNTER_CONFIG}"
  curl --fail --silent --show-error \
    -X PUT "${EVENT_HUNTER_CONNECT_URL}/connectors/${EVENT_HUNTER_CONNECTOR_NAME}/config" \
    -H 'Content-Type: application/json' \
    --data-binary @"${EVENT_HUNTER_CONFIG}" \
    --output /dev/null
  rm -f "${EVENT_HUNTER_RENDERED}" "${EVENT_HUNTER_CONFIG}"
  wait_for_connector "${EVENT_HUNTER_CONNECTOR_NAME}"
  echo "registered and running ${EVENT_HUNTER_CONNECTOR_NAME}"
done
