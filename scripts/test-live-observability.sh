#!/usr/bin/env bash
set -euo pipefail

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

EVENT_HUNTER_ORDER_URL="${DEMO_ORDER_URL:-http://localhost:28335}"
EVENT_HUNTER_CLICKHOUSE_URL="${CLICKHOUSE_URL:-http://localhost:28317}"
EVENT_HUNTER_TEMPO_URL="${TEMPO_URL:-http://localhost:28328}"
EVENT_HUNTER_LOKI_URL="${LOKI_URL:-http://localhost:28327}"
EVENT_HUNTER_CLICKHOUSE_USER="${CLICKHOUSE_USER:-event_hunter}"
EVENT_HUNTER_CLICKHOUSE_PASSWORD="${CLICKHOUSE_PASSWORD:-event_hunter_local_only}"
EVENT_HUNTER_VERIFY_RESTART=true
for EVENT_HUNTER_ARGUMENT in "$@"; do
  case "${EVENT_HUNTER_ARGUMENT}" in
    --skip-restart) EVENT_HUNTER_VERIFY_RESTART=false ;;
    *)
      echo "用法：bash scripts/test-live-observability.sh [--skip-restart]" >&2
      exit 2
      ;;
  esac
done
EVENT_HUNTER_REQUEST_ID="$(python3 -c 'import uuid; print(uuid.uuid4())')"
EVENT_HUNTER_RESPONSE_FILE="$(mktemp)"
EVENT_HUNTER_TRACE_FILE="$(mktemp)"
EVENT_HUNTER_LOG_FILE="$(mktemp)"
trap 'rm -f "${EVENT_HUNTER_RESPONSE_FILE}" "${EVENT_HUNTER_TRACE_FILE}" "${EVENT_HUNTER_LOG_FILE}"' EXIT

wait_for_clickhouse_trace() {
  local EVENT_HUNTER_ACTUAL_TRACE_ID=""
  for _ in $(seq 1 90); do
    EVENT_HUNTER_ACTUAL_TRACE_ID="$(curl --fail --silent --show-error \
      --user "${EVENT_HUNTER_CLICKHOUSE_USER}:${EVENT_HUNTER_CLICKHOUSE_PASSWORD}" \
      --data-binary "SELECT if(count() = 3 AND uniqExact(trace_id) = 1, any(trace_id), '') FROM event_hunter.forensics_events WHERE correlation_id = '${EVENT_HUNTER_CORRELATION_ID}' AND event_type IN ('OrderCreated','PaymentCompleted','ShipmentCreated')" \
      "${EVENT_HUNTER_CLICKHOUSE_URL}")"
    if [[ "${EVENT_HUNTER_ACTUAL_TRACE_ID}" =~ ^[0-9a-f]{32}$ ]]; then
      printf '%s' "${EVENT_HUNTER_ACTUAL_TRACE_ID}"
      return 0
    fi
    sleep 1
  done
  return 1
}

wait_for_tempo_trace() {
  for _ in $(seq 1 30); do
    if curl --fail --silent \
      "${EVENT_HUNTER_TEMPO_URL}/api/traces/${EVENT_HUNTER_TRACE_ID}" \
      > "${EVENT_HUNTER_TRACE_FILE}"; then
      if python3 -c 'import json,sys
p=json.load(open(sys.argv[1])); expected={"order-service","payment-service","shipping-service"}; found=set()
for batch in p.get("batches",[]):
  for attr in batch.get("resource",{}).get("attributes",[]):
    if attr.get("key")=="service.name": found.add(attr.get("value",{}).get("stringValue"))
raise SystemExit(0 if expected <= found else 1)' "${EVENT_HUNTER_TRACE_FILE}"; then
        return 0
      fi
    fi
    sleep 1
  done
  return 1
}

wait_for_loki_logs() {
  local EVENT_HUNTER_NOW_NS
  local EVENT_HUNTER_START_NS
  EVENT_HUNTER_NOW_NS="$(python3 -c 'import time; print(time.time_ns())')"
  EVENT_HUNTER_START_NS="$((EVENT_HUNTER_NOW_NS - 600000000000))"
  for _ in $(seq 1 30); do
    curl --fail --silent --show-error --get "${EVENT_HUNTER_LOKI_URL}/loki/api/v1/query_range" \
      --data-urlencode "query={service_name=~\"order-service|payment-service|shipping-service\"} | correlation_id=\"${EVENT_HUNTER_CORRELATION_ID}\"" \
      --data-urlencode "start=${EVENT_HUNTER_START_NS}" \
      --data-urlencode "end=${EVENT_HUNTER_NOW_NS}" \
      --data-urlencode 'limit=100' \
      > "${EVENT_HUNTER_LOG_FILE}"
    if python3 -c 'import json,sys
p=json.load(open(sys.argv[1])); trace_id=sys.argv[2]; correlation_id=sys.argv[3]
expected={"order-service","payment-service","shipping-service"}; found=set()
required=("event_id","event_type","kafka_topic","kafka_partition","kafka_offset","span_id")
for item in p.get("data",{}).get("result",[]):
  stream=item.get("stream",{})
  if stream.get("trace_id") != trace_id or stream.get("correlation_id") != correlation_id: continue
  if all(stream.get(key) not in (None, "") for key in required): found.add(stream.get("service_name"))
raise SystemExit(0 if expected <= found else 1)' "${EVENT_HUNTER_LOG_FILE}" "${EVENT_HUNTER_TRACE_ID}" "${EVENT_HUNTER_CORRELATION_ID}"; then
      return 0
    fi
    sleep 1
  done
  return 1
}

assert_live_observability() {
  local EVENT_HUNTER_ACTUAL_TRACE_ID
  if ! EVENT_HUNTER_ACTUAL_TRACE_ID="$(wait_for_clickhouse_trace)"; then
    echo "Live event chain did not produce three events with one trace ID for ${EVENT_HUNTER_CORRELATION_ID}." >&2
    return 1
  fi
  if [[ "${EVENT_HUNTER_ACTUAL_TRACE_ID}" != "${EVENT_HUNTER_TRACE_ID}" ]]; then
    echo "ClickHouse trace ${EVENT_HUNTER_ACTUAL_TRACE_ID} does not match expected Tempo trace ${EVENT_HUNTER_TRACE_ID}." >&2
    return 1
  fi
  if ! wait_for_tempo_trace; then
    echo "Tempo trace ${EVENT_HUNTER_TRACE_ID} is missing order/payment/shipping services." >&2
    return 1
  fi
  if ! wait_for_loki_logs; then
    echo "Loki logs are missing canonical fields or matching trace ${EVENT_HUNTER_TRACE_ID} for all three services." >&2
    return 1
  fi
}

curl --fail --silent --show-error \
  -X POST "${EVENT_HUNTER_ORDER_URL}/api/v1/orders" \
  -H 'Content-Type: application/json' \
  -H "Idempotency-Key: otel-e2e-${EVENT_HUNTER_REQUEST_ID}" \
  --data-binary '{"customer_id":"CUSTOMER-OTEL-E2E","total_amount":1680,"currency":"TWD"}' \
  > "${EVENT_HUNTER_RESPONSE_FILE}"

EVENT_HUNTER_CORRELATION_ID="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["correlation_id"])' "${EVENT_HUNTER_RESPONSE_FILE}")"
if ! EVENT_HUNTER_TRACE_ID="$(wait_for_clickhouse_trace)"; then
  echo "Live event chain did not produce three events with one trace ID for ${EVENT_HUNTER_CORRELATION_ID}." >&2
  exit 1
fi
assert_live_observability

if [[ "${EVENT_HUNTER_VERIFY_RESTART}" == "true" ]]; then
  docker compose restart \
    clickhouse loki tempo otel-collector \
    demo-order-service demo-payment-service demo-shipping-service
  docker compose up -d --wait \
    clickhouse loki tempo otel-collector \
    demo-order-service demo-payment-service demo-shipping-service
  assert_live_observability
fi

echo "Live OpenTelemetry verified: correlation=${EVENT_HUNTER_CORRELATION_ID} trace=${EVENT_HUNTER_TRACE_ID} services=order,payment,shipping restart=${EVENT_HUNTER_VERIFY_RESTART}."
