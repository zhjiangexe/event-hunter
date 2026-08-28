#!/usr/bin/env bash
set -euo pipefail

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

EVENT_HUNTER_ORDER_URL="${ORDER_SERVICE_URL:-http://localhost:28335}"
EVENT_HUNTER_API_URL="${API_BASE_URL:-http://localhost:28333}"
EVENT_HUNTER_TEST_TOKEN="$(date -u +%Y%m%d%H%M%S)-$$"
EVENT_HUNTER_BASELINE_CORRELATION=""
EVENT_HUNTER_BACKLOG_CORRELATION=""
EVENT_HUNTER_CLICKHOUSE_STOPPED=false
EVENT_HUNTER_COOKIE_JAR="$(mktemp)"

clickhouse_query() {
  docker compose exec -T clickhouse sh -ec \
    'clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB" --query "$1"' \
    -- "$1"
}

cleanup_correlations() {
  local correlation
  for correlation in "${EVENT_HUNTER_BASELINE_CORRELATION}" "${EVENT_HUNTER_BACKLOG_CORRELATION}"; do
    [[ -n "${correlation}" ]] || continue
    clickhouse_query "ALTER TABLE event_hunter.forensics_events DELETE WHERE correlation_id = '${correlation}' SETTINGS mutations_sync = 1" >/dev/null || true
    clickhouse_query "ALTER TABLE event_hunter.poc_forensics_events DELETE WHERE correlation_id = '${correlation}' SETTINGS mutations_sync = 1" >/dev/null || true
    clickhouse_query "ALTER TABLE event_hunter.poc_event_admission_failures DELETE WHERE correlation_id = '${correlation}' SETTINGS mutations_sync = 1" >/dev/null || true
    clickhouse_query "ALTER TABLE event_hunter_poc.poc_event_landing_raw DELETE WHERE correlation_id = '${correlation}' SETTINGS mutations_sync = 1" >/dev/null || true
    clickhouse_query "ALTER TABLE event_hunter.event_processing_attempts DELETE WHERE correlation_id = '${correlation}' SETTINGS mutations_sync = 1" >/dev/null || true
    clickhouse_query "ALTER TABLE event_hunter.poc_event_processing_attempts DELETE WHERE correlation_id = '${correlation}' SETTINGS mutations_sync = 1" >/dev/null || true
    clickhouse_query "ALTER TABLE event_hunter.poc_processing_attempt_admission_failures DELETE WHERE correlation_id = '${correlation}' SETTINGS mutations_sync = 1" >/dev/null || true
    clickhouse_query "ALTER TABLE event_hunter_poc.poc_processing_attempt_landing_raw DELETE WHERE correlation_id = '${correlation}' SETTINGS mutations_sync = 1" >/dev/null || true
    docker compose exec -T demo-order-postgres psql --username "${DEMO_ORDER_POSTGRES_USER:-demo_order}" --dbname demo_order --set ON_ERROR_STOP=1 \
      --command "BEGIN; DELETE FROM outbox_events WHERE correlation_id='${correlation}'; DELETE FROM idempotency_keys WHERE order_id IN (SELECT id FROM orders WHERE correlation_id='${correlation}'); DELETE FROM orders WHERE correlation_id='${correlation}'; COMMIT;" >/dev/null || true
    docker compose exec -T demo-payment-postgres psql --username "${DEMO_PAYMENT_POSTGRES_USER:-demo_payment}" --dbname demo_payment --set ON_ERROR_STOP=1 \
      --command "BEGIN; DELETE FROM outbox_events WHERE correlation_id='${correlation}'; DELETE FROM payments WHERE correlation_id='${correlation}'; COMMIT;" >/dev/null || true
    docker compose exec -T demo-shipping-postgres psql --username "${DEMO_SHIPPING_POSTGRES_USER:-demo_shipping}" --dbname demo_shipping --set ON_ERROR_STOP=1 \
      --command "BEGIN; DELETE FROM outbox_events WHERE correlation_id='${correlation}'; DELETE FROM returns WHERE correlation_id='${correlation}'; DELETE FROM shipments WHERE correlation_id='${correlation}'; COMMIT;" >/dev/null || true
  done
}

restore_environment() {
  if [[ "${EVENT_HUNTER_CLICKHOUSE_STOPPED}" == "true" ]]; then
    docker compose up -d --wait clickhouse >/dev/null || true
    EVENT_HUNTER_CLICKHOUSE_STOPPED=false
  fi
  docker compose --profile clickhouse-mv-poc up -d --wait \
    kafka-connect-clickhouse-poc technical-dlq-projector >/dev/null || true
  cleanup_correlations
  rm -f "${EVENT_HUNTER_COOKIE_JAR}"
}
trap restore_environment EXIT INT TERM

create_order() {
  local suffix="$1"
  curl --fail --silent --show-error \
    --header 'Content-Type: application/json' \
    --header "Idempotency-Key: candidate-only-recovery-${EVENT_HUNTER_TEST_TOKEN}-${suffix}" \
    --data-binary "{\"customer_id\":\"CUSTOMER-CANDIDATE-RECOVERY-${suffix}\",\"total_amount\":1680,\"currency\":\"TWD\"}" \
    "${EVENT_HUNTER_ORDER_URL}/api/v1/orders" \
    | python3 -c 'import json,sys; print(json.load(sys.stdin)["correlation_id"])'
}

wait_for_candidate_events() {
  local correlation="$1"
  local attempt
  for attempt in {1..90}; do
    if [[ "$(clickhouse_query "SELECT uniqExact(event_type) FROM event_hunter.poc_forensics_events WHERE correlation_id = '${correlation}' AND event_type IN ('OrderCreated','PaymentCompleted','ShipmentCreated')")" == "3" ]]; then
      return 0
    fi
    sleep 1
  done
  echo "candidate 未在 90 秒內保存 ${correlation} 的三服務事件鏈。" >&2
  return 1
}

wait_for_candidate_attempts() {
  local correlation="$1"
  local attempt
  for attempt in {1..90}; do
    if [[ "$(clickhouse_query "SELECT uniqExact(tuple(event_id, consumer_group_id)) FROM event_hunter.poc_event_processing_attempts FINAL WHERE correlation_id = '${correlation}' AND processing_status = 'SUCCEEDED'")" -ge "2" ]]; then
      return 0
    fi
    sleep 1
  done
  echo "candidate 未在 90 秒內保存 ${correlation} 的跨服務 processing attempts。" >&2
  return 1
}

wait_for_http_status() {
  local url="$1"
  local expected_status="$2"
  local attempt
  local actual_status=""
  for attempt in {1..90}; do
    actual_status="$(curl --silent --output /dev/null --write-out '%{http_code}' "${url}" || true)"
    if [[ "${actual_status}" == "${expected_status}" ]]; then
      return 0
    fi
    sleep 1
  done
  echo "${url} 未在 90 秒內回傳 HTTP ${expected_status}；最後狀態為 ${actual_status}。" >&2
  return 1
}

EVENT_HUNTER_ACTIVE_SOURCE="$(clickhouse_query "SELECT if(position(create_table_query,'poc_forensics_events') > 0,'clickhouse-mv','legacy') FROM system.tables WHERE database=currentDatabase() AND name='canonical_forensics_events'")"
if [[ "${EVENT_HUNTER_ACTIVE_SOURCE}" != "clickhouse-mv" ]]; then
  echo "candidate-only recovery 要求 canonical source 已是 clickhouse-mv；目前為 ${EVENT_HUNTER_ACTIVE_SOURCE}。" >&2
  exit 1
fi
EVENT_HUNTER_ATTEMPTS_ACTIVE_SOURCE="$(clickhouse_query "SELECT if(position(create_table_query,'poc_event_processing_attempts') > 0,'clickhouse-mv','legacy') FROM system.tables WHERE database=currentDatabase() AND name='canonical_event_processing_attempts'")"
if [[ "${EVENT_HUNTER_ATTEMPTS_ACTIVE_SOURCE}" != "clickhouse-mv" ]]; then
  echo "candidate-only recovery 要求 canonical attempts source 已是 clickhouse-mv；目前為 ${EVENT_HUNTER_ATTEMPTS_ACTIVE_SOURCE}。" >&2
  exit 1
fi
if docker compose ps --status running --services | grep -Fxq redpanda-connect; then
  echo "candidate-only recovery 要求舊 domain-event redpanda-connect 已停止。" >&2
  exit 1
fi
if docker compose ps --status running --services | grep -Fxq redpanda-connect-attempts; then
  echo "candidate-only recovery 要求舊 processing-attempt redpanda-connect 已停止。" >&2
  exit 1
fi

wait_for_http_status "${EVENT_HUNTER_API_URL}/health/ready" 200
EVENT_HUNTER_BASELINE_CORRELATION="$(create_order baseline)"
wait_for_candidate_events "${EVENT_HUNTER_BASELINE_CORRELATION}"
wait_for_candidate_attempts "${EVENT_HUNTER_BASELINE_CORRELATION}"

docker compose stop clickhouse >/dev/null
EVENT_HUNTER_CLICKHOUSE_STOPPED=true
wait_for_http_status "${EVENT_HUNTER_API_URL}/health/ready" 503
EVENT_HUNTER_BACKLOG_CORRELATION="$(create_order backlog)"

docker compose up -d --wait clickhouse >/dev/null
EVENT_HUNTER_CLICKHOUSE_STOPPED=false
docker compose --profile clickhouse-mv-poc restart \
  kafka-connect-clickhouse-poc technical-dlq-projector >/dev/null
docker compose --profile clickhouse-mv-poc up -d --wait \
  kafka-connect-clickhouse-poc technical-dlq-projector >/dev/null
wait_for_http_status "http://localhost:${CLICKHOUSE_POC_CONNECT_REST_PORT:-28345}/connectors/event-hunter-poc-raw-landing/status" 200
wait_for_http_status "http://localhost:${TECHNICAL_DLQ_PROJECTOR_HEALTH_PORT:-28346}/health/ready" 200
wait_for_http_status "${EVENT_HUNTER_API_URL}/health/ready" 200
wait_for_candidate_events "${EVENT_HUNTER_BASELINE_CORRELATION}"
wait_for_candidate_events "${EVENT_HUNTER_BACKLOG_CORRELATION}"
wait_for_candidate_attempts "${EVENT_HUNTER_BASELINE_CORRELATION}"
wait_for_candidate_attempts "${EVENT_HUNTER_BACKLOG_CORRELATION}"

for EVENT_HUNTER_CORRELATION in "${EVENT_HUNTER_BASELINE_CORRELATION}" "${EVENT_HUNTER_BACKLOG_CORRELATION}"; do
  if [[ "$(clickhouse_query "SELECT count() FROM event_hunter.forensics_events WHERE correlation_id = '${EVENT_HUNTER_CORRELATION}'")" != "0" ]]; then
    echo "legacy 在停止後仍出現 ${EVENT_HUNTER_CORRELATION}，candidate-only 條件不成立。" >&2
    exit 1
  fi
  if [[ "$(clickhouse_query "SELECT count() FROM event_hunter.event_processing_attempts WHERE correlation_id = '${EVENT_HUNTER_CORRELATION}'")" != "0" ]]; then
    echo "legacy attempts 在停止後仍出現 ${EVENT_HUNTER_CORRELATION}，candidate-only 條件不成立。" >&2
    exit 1
  fi
done

curl --fail --silent --show-error \
  --cookie-jar "${EVENT_HUNTER_COOKIE_JAR}" \
  --header 'Content-Type: application/json' \
  --data-binary '{"role":"INVESTIGATOR"}' \
  "${EVENT_HUNTER_API_URL}/api/v1/auth/demo-session" >/dev/null

EVENT_HUNTER_FROM="$(python3 -c 'import datetime; print((datetime.datetime.now(datetime.timezone.utc)-datetime.timedelta(minutes=15)).isoformat().replace("+00:00","Z"))')"
EVENT_HUNTER_TO="$(python3 -c 'import datetime; print((datetime.datetime.now(datetime.timezone.utc)+datetime.timedelta(minutes=1)).isoformat().replace("+00:00","Z"))')"
for EVENT_HUNTER_CORRELATION in "${EVENT_HUNTER_BASELINE_CORRELATION}" "${EVENT_HUNTER_BACKLOG_CORRELATION}"; do
  curl --fail --silent --show-error --get \
    --cookie "${EVENT_HUNTER_COOKIE_JAR}" \
    --data-urlencode "from=${EVENT_HUNTER_FROM}" \
    --data-urlencode "to=${EVENT_HUNTER_TO}" \
    "${EVENT_HUNTER_API_URL}/api/v1/timelines/${EVENT_HUNTER_CORRELATION}" \
    | python3 -c 'import json,sys; document=json.load(sys.stdin); raise SystemExit(0 if document.get("event_count") == 3 else 1)'
done

echo "Candidate-only recovery verified: both legacy workers remained stopped, readiness changed 200→503→200, and baseline plus outage backlog each produced domain events and processing attempts only through ClickHouse-first ingestion."
