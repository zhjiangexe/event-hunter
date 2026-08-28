#!/usr/bin/env bash
set -euo pipefail

# 這是尚未有 Go migration binary 前的本機 bootstrap；只執行 goose Up 區段，不會執行 Down。
EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

apply_postgres_migration_if_table_missing() {
  local EVENT_HUNTER_TABLE_NAME="$1"
  local EVENT_HUNTER_MIGRATION_PATH="$2"
  local EVENT_HUNTER_TABLE_EXISTS

  # table name 只由本檔案內的固定值傳入，不接受使用者輸入或任意 SQL identifier。
  EVENT_HUNTER_TABLE_EXISTS="$({
    docker compose exec -T postgres sh -ec \
      "psql -U \"\$POSTGRES_USER\" -d \"\$POSTGRES_DB\" -tAc \"SELECT to_regclass('public.${EVENT_HUNTER_TABLE_NAME}') IS NOT NULL\""
  } | tr -d '[:space:]')"

  if [[ "${EVENT_HUNTER_TABLE_EXISTS}" == "t" ]]; then
    echo "PostgreSQL ${EVENT_HUNTER_TABLE_NAME} 已存在，略過 ${EVENT_HUNTER_MIGRATION_PATH}。"
    return
  fi

  echo "套用 PostgreSQL migration：${EVENT_HUNTER_MIGRATION_PATH}"
  sed '/^-- +goose Down$/q' "${EVENT_HUNTER_MIGRATION_PATH}" \
    | docker compose exec -T postgres sh -ec \
      'psql --set ON_ERROR_STOP=1 --single-transaction -U "$POSTGRES_USER" -d "$POSTGRES_DB"'
}

# 尚未建立 Go goose binary 前，用代表性 table 判斷兩份本機 baseline 是否已套用。
apply_postgres_migration_if_table_missing \
  investigation_cases \
  backend/migrations/postgres/00001_mvp_control_plane.sql
apply_postgres_migration_if_table_missing \
  grafana_alert_receipts \
  backend/migrations/postgres/00002_grafana_alert_receipts.sql
apply_postgres_migration_if_table_missing \
  scenario_runs \
  backend/migrations/postgres/00003_scenario_runs.sql
apply_postgres_migration_if_table_missing \
  saved_searches \
  backend/migrations/postgres/00004_saved_searches.sql
apply_postgres_migration_if_table_missing \
  case_notes \
  backend/migrations/postgres/00005_case_collaboration.sql
apply_postgres_migration_if_table_missing \
  pattern_finding_feedback \
  backend/migrations/postgres/00006_pattern_finding_feedback.sql

apply_postgres_migration() {
  local EVENT_HUNTER_MIGRATION_PATH="$1"
  echo "套用 PostgreSQL migration：${EVENT_HUNTER_MIGRATION_PATH}"
  sed '/^-- +goose Down$/q' "${EVENT_HUNTER_MIGRATION_PATH}" \
    | docker compose exec -T postgres sh -ec \
      'psql --set ON_ERROR_STOP=1 --single-transaction -U "$POSTGRES_USER" -d "$POSTGRES_DB"'
}

apply_postgres_migration backend/migrations/postgres/00007_live_scenario_catalog.sql
apply_postgres_migration backend/migrations/postgres/00008_investigation_incident_window.sql
apply_postgres_migration_if_table_missing \
  check_snapshots \
  backend/migrations/postgres/00009_check_snapshots.sql
apply_postgres_migration backend/migrations/postgres/00010_saved_search_event_check.sql

apply_demo_postgres_migration_if_table_missing() {
  local EVENT_HUNTER_COMPOSE_SERVICE="$1"
  local EVENT_HUNTER_TABLE_NAME="$2"
  local EVENT_HUNTER_MIGRATION_PATH="$3"
  local EVENT_HUNTER_TABLE_EXISTS

  EVENT_HUNTER_TABLE_EXISTS="$({
    docker compose exec -T "${EVENT_HUNTER_COMPOSE_SERVICE}" sh -ec \
      "psql -U \"\$POSTGRES_USER\" -d \"\$POSTGRES_DB\" -tAc \"SELECT to_regclass('public.${EVENT_HUNTER_TABLE_NAME}') IS NOT NULL\""
  } | tr -d '[:space:]')"

  if [[ "${EVENT_HUNTER_TABLE_EXISTS}" == "t" ]]; then
    echo "${EVENT_HUNTER_COMPOSE_SERVICE} ${EVENT_HUNTER_TABLE_NAME} 已存在，略過 ${EVENT_HUNTER_MIGRATION_PATH}。"
    return
  fi

  echo "套用 ${EVENT_HUNTER_COMPOSE_SERVICE} migration：${EVENT_HUNTER_MIGRATION_PATH}"
  sed '/^-- +goose Down$/q' "${EVENT_HUNTER_MIGRATION_PATH}" \
    | docker compose exec -T "${EVENT_HUNTER_COMPOSE_SERVICE}" sh -ec \
      'psql --set ON_ERROR_STOP=1 --single-transaction -U "$POSTGRES_USER" -d "$POSTGRES_DB"'
}

apply_demo_postgres_migration_if_table_missing \
  demo-order-postgres orders \
  backend/migrations/demo/order/00001_order_service.sql
apply_demo_postgres_migration_if_table_missing \
  demo-payment-postgres payments \
  backend/migrations/demo/payment/00001_payment_service.sql
apply_demo_postgres_migration_if_table_missing \
  demo-shipping-postgres shipments \
  backend/migrations/demo/shipping/00001_shipping_service.sql

apply_demo_postgres_migration() {
  local EVENT_HUNTER_COMPOSE_SERVICE="$1"
  local EVENT_HUNTER_MIGRATION_PATH="$2"

  echo "套用 ${EVENT_HUNTER_COMPOSE_SERVICE} migration：${EVENT_HUNTER_MIGRATION_PATH}"
  sed '/^-- +goose Down$/q' "${EVENT_HUNTER_MIGRATION_PATH}" \
    | docker compose exec -T "${EVENT_HUNTER_COMPOSE_SERVICE}" sh -ec \
      'psql --set ON_ERROR_STOP=1 --single-transaction -U "$POSTGRES_USER" -d "$POSTGRES_DB"'
}

# Incremental demo migrations use idempotent ALTER statements until a proper
# migration runner replaces this local bootstrap script.
apply_demo_postgres_migration demo-order-postgres backend/migrations/demo/order/00002_outbox_trace_context.sql
apply_demo_postgres_migration demo-payment-postgres backend/migrations/demo/payment/00002_outbox_trace_context.sql
apply_demo_postgres_migration demo-shipping-postgres backend/migrations/demo/shipping/00002_outbox_trace_context.sql
apply_demo_postgres_migration demo-order-postgres backend/migrations/demo/order/00003_live_simulation_profiles.sql
apply_demo_postgres_migration demo-payment-postgres backend/migrations/demo/payment/00003_live_simulation_profiles.sql
apply_demo_postgres_migration demo-shipping-postgres backend/migrations/demo/shipping/00003_live_simulation_profiles.sql

# ClickHouse local bootstrap migrations are idempotent and safe to re-apply.
for EVENT_HUNTER_MIGRATION_PATH in backend/migrations/clickhouse/*.sql; do
  echo "套用 ClickHouse migration：${EVENT_HUNTER_MIGRATION_PATH}"
  sed '/^-- +goose Down$/q' "${EVENT_HUNTER_MIGRATION_PATH}" \
    | docker compose exec -T clickhouse sh -ec \
      'clickhouse-client --multiquery --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB"'
done

# Existing ordinary views cannot use ALTER ... MODIFY QUERY in ClickHouse 26.6.
# Upgrade through active/standby EXCHANGE while preserving the selected source.
bash scripts/upgrade-canonical-admission-metadata.sh

echo "本機 baseline migration 已完成。"
