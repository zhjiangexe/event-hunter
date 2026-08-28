#!/usr/bin/env bash
set -euo pipefail

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

if [[ "${1:-}" != "--since" || -z "${2:-}" || ( -n "${3:-}" && "${3:-}" != "--all-since" ) || -n "${4:-}" ]]; then
  echo "用法：bash scripts/cleanup-e2e-data.sh --since <RFC3339 timestamp> [--all-since]" >&2
  exit 2
fi

EVENT_HUNTER_E2E_SINCE="$2"
EVENT_HUNTER_CLEAN_ALL_SINCE=false
if [[ "${3:-}" == "--all-since" ]]; then
  EVENT_HUNTER_CLEAN_ALL_SINCE=true
fi
if ! python3 -c 'import datetime,sys; datetime.datetime.fromisoformat(sys.argv[1].replace("Z", "+00:00"))' "${EVENT_HUNTER_E2E_SINCE}"; then
  echo "Invalid RFC3339 timestamp: ${EVENT_HUNTER_E2E_SINCE}" >&2
  exit 2
fi

# Release-gate E2E is exclusive to the local Compose environment. Cases carry
# an explicit [E2E] marker; Grafana receipts use dedicated fingerprints. Rows
# from the demo-service and Scenario Lab databases are bounded by gate start.
docker compose exec -T postgres psql \
  --username "${POSTGRES_USER:-event_hunter}" \
  --dbname "${POSTGRES_DB:-event_hunter}" \
  --set ON_ERROR_STOP=1 \
  --set e2e_since="${EVENT_HUNTER_E2E_SINCE}" \
  --set clean_all_since="${EVENT_HUNTER_CLEAN_ALL_SINCE}" <<'SQL'
BEGIN;
CREATE TEMP TABLE e2e_case_ids (id UUID PRIMARY KEY) ON COMMIT DROP;
INSERT INTO e2e_case_ids (id)
SELECT id FROM investigation_cases WHERE title LIKE '[E2E]%'
ON CONFLICT DO NOTHING;
INSERT INTO e2e_case_ids (id)
SELECT id FROM investigation_cases WHERE correlation_id LIKE 'E2E-GRAFANA-AUTO-%'
ON CONFLICT DO NOTHING;
-- Scenario Lab can naturally trigger the real Grafana business rule. Select
-- those generated cases through the persisted run correlation before deleting
-- scenario_runs; their production-like title intentionally has no [E2E] prefix.
INSERT INTO e2e_case_ids (id)
SELECT cases.id
FROM investigation_cases AS cases
JOIN scenario_runs AS runs ON runs.correlation_id = cases.correlation_id
WHERE runs.accepted_at >= :'e2e_since'::timestamptz
ON CONFLICT DO NOTHING;
INSERT INTO e2e_case_ids (id)
SELECT investigation_case_id
FROM grafana_alert_receipts
WHERE investigation_case_id IS NOT NULL
  AND (fingerprint LIKE 'e2e-%' OR fingerprint LIKE 'eh-ui-%')
ON CONFLICT DO NOTHING;
-- Historical suites did not consistently prefix case titles with [E2E]. The
-- explicit local maintenance mode selects every case created after a
-- fixture-safe cutoff. The default release-gate mode remains marker-based.
\if :clean_all_since
INSERT INTO e2e_case_ids (id)
SELECT id FROM investigation_cases WHERE created_at >= :'e2e_since'::timestamptz
ON CONFLICT DO NOTHING;
\endif

DELETE FROM grafana_alert_receipts
WHERE investigation_case_id IN (SELECT id FROM e2e_case_ids)
   OR fingerprint LIKE 'e2e-%'
   OR fingerprint LIKE 'eh-ui-%'
   OR (:clean_all_since AND received_at >= :'e2e_since'::timestamptz);
DELETE FROM case_notes WHERE investigation_case_id IN (SELECT id FROM e2e_case_ids);
DELETE FROM case_evidence WHERE investigation_case_id IN (SELECT id FROM e2e_case_ids);
DELETE FROM pattern_finding_feedback WHERE investigation_case_id IN (SELECT id FROM e2e_case_ids);
DELETE FROM pattern_findings WHERE investigation_case_id IN (SELECT id FROM e2e_case_ids);
DELETE FROM audit_logs
WHERE resource_id IN (SELECT id::text FROM e2e_case_ids)
   OR (actor_id LIKE 'demo-%' AND created_at >= :'e2e_since'::timestamptz);
DELETE FROM investigation_cases WHERE id IN (SELECT id FROM e2e_case_ids);
DELETE FROM scenario_runs WHERE accepted_at >= :'e2e_since'::timestamptz;
DELETE FROM saved_searches
WHERE owner_subject LIKE 'demo-%' AND created_at >= :'e2e_since'::timestamptz;
COMMIT;
SQL

cleanup_demo_database() {
  local service="$1"
  local username="$2"
  local database="$3"
  local entity_table="$4"
  docker compose exec -T "${service}" psql \
    --username "${username}" \
    --dbname "${database}" \
    --set ON_ERROR_STOP=1 \
    --command "BEGIN; DELETE FROM outbox_events WHERE created_at >= '${EVENT_HUNTER_E2E_SINCE}'::timestamptz; DELETE FROM ${entity_table} WHERE created_at >= '${EVENT_HUNTER_E2E_SINCE}'::timestamptz; COMMIT;"
}

# Order idempotency rows reference orders and therefore must be removed first.
docker compose exec -T demo-order-postgres psql \
  --username "${DEMO_ORDER_POSTGRES_USER:-demo_order}" \
  --dbname demo_order \
  --set ON_ERROR_STOP=1 \
  --command "BEGIN; DELETE FROM outbox_events WHERE created_at >= '${EVENT_HUNTER_E2E_SINCE}'::timestamptz; DELETE FROM idempotency_keys WHERE created_at >= '${EVENT_HUNTER_E2E_SINCE}'::timestamptz; DELETE FROM orders WHERE created_at >= '${EVENT_HUNTER_E2E_SINCE}'::timestamptz; COMMIT;"
cleanup_demo_database demo-payment-postgres "${DEMO_PAYMENT_POSTGRES_USER:-demo_payment}" demo_payment payments
docker compose exec -T demo-shipping-postgres psql \
  --username "${DEMO_SHIPPING_POSTGRES_USER:-demo_shipping}" \
  --dbname demo_shipping \
  --set ON_ERROR_STOP=1 \
  --command "BEGIN; DELETE FROM outbox_events WHERE created_at >= '${EVENT_HUNTER_E2E_SINCE}'::timestamptz; DELETE FROM returns WHERE created_at >= '${EVENT_HUNTER_E2E_SINCE}'::timestamptz; DELETE FROM shipments WHERE created_at >= '${EVENT_HUNTER_E2E_SINCE}'::timestamptz; COMMIT;"

EVENT_HUNTER_CLICKHOUSE_ENDPOINT="${CLICKHOUSE_URL:-http://localhost:28317}/?database=${CLICKHOUSE_DB:-event_hunter}&mutations_sync=2"
clickhouse_delete() {
  curl --fail --silent --show-error \
    --user "${CLICKHOUSE_USER:-event_hunter}:${CLICKHOUSE_PASSWORD:-event_hunter_local_only}" \
    --data-binary "$1" \
    "${EVENT_HUNTER_CLICKHOUSE_ENDPOINT}"
}

EVENT_HUNTER_E2E_TIME_SQL="parseDateTime64BestEffort('${EVENT_HUNTER_E2E_SINCE}')"
clickhouse_delete "ALTER TABLE forensics_events DELETE WHERE ingested_at >= ${EVENT_HUNTER_E2E_TIME_SQL}"
clickhouse_delete "ALTER TABLE event_processing_attempts DELETE WHERE observed_at >= ${EVENT_HUNTER_E2E_TIME_SQL}"
clickhouse_delete "ALTER TABLE event_ingestion_failures DELETE WHERE observed_at >= ${EVENT_HUNTER_E2E_TIME_SQL}"
clickhouse_delete "ALTER TABLE ingestion_technical_failures DELETE WHERE observed_at >= ${EVENT_HUNTER_E2E_TIME_SQL}"
clickhouse_delete "ALTER TABLE poc_forensics_events DELETE WHERE ingested_at >= ${EVENT_HUNTER_E2E_TIME_SQL}"
clickhouse_delete "ALTER TABLE poc_event_admission_failures DELETE WHERE failed_at >= ${EVENT_HUNTER_E2E_TIME_SQL}"
clickhouse_delete "ALTER TABLE poc_event_processing_attempts DELETE WHERE observed_at >= ${EVENT_HUNTER_E2E_TIME_SQL}"
clickhouse_delete "ALTER TABLE poc_processing_attempt_admission_failures DELETE WHERE failed_at >= ${EVENT_HUNTER_E2E_TIME_SQL}"
clickhouse_delete "ALTER TABLE redpanda_consumer_group_metrics DELETE WHERE sampled_at >= ${EVENT_HUNTER_E2E_TIME_SQL}"
clickhouse_delete "ALTER TABLE event_quality_metrics DELETE WHERE calculated_at >= ${EVENT_HUNTER_E2E_TIME_SQL}"

EVENT_HUNTER_POC_CLICKHOUSE_ENDPOINT="${CLICKHOUSE_URL:-http://localhost:28317}/?database=event_hunter_poc&mutations_sync=2"
curl --fail --silent --show-error \
  --user "${CLICKHOUSE_USER:-event_hunter}:${CLICKHOUSE_PASSWORD:-event_hunter_local_only}" \
  --data-binary "ALTER TABLE poc_event_landing_raw DELETE WHERE received_at >= ${EVENT_HUNTER_E2E_TIME_SQL}" \
  "${EVENT_HUNTER_POC_CLICKHOUSE_ENDPOINT}"
curl --fail --silent --show-error \
  --user "${CLICKHOUSE_USER:-event_hunter}:${CLICKHOUSE_PASSWORD:-event_hunter_local_only}" \
  --data-binary "ALTER TABLE poc_processing_attempt_landing_raw DELETE WHERE received_at >= ${EVENT_HUNTER_E2E_TIME_SQL}" \
  "${EVENT_HUNTER_POC_CLICKHOUSE_ENDPOINT}"

echo "E2E rows created since ${EVENT_HUNTER_E2E_SINCE} were removed from control-plane, demo-service and ClickHouse stores."
