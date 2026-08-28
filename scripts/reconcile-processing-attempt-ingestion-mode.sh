#!/usr/bin/env bash
set -euo pipefail

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

EVENT_HUNTER_CONFIG_PATH="${EVENT_HUNTER_INGESTION_CONFIG:-config/ingestion-cutover.env}"
if [[ ! -f "${EVENT_HUNTER_CONFIG_PATH}" ]]; then
  echo "找不到 ingestion cutover 設定：${EVENT_HUNTER_CONFIG_PATH}" >&2
  exit 2
fi
# shellcheck disable=SC1090
source "${EVENT_HUNTER_CONFIG_PATH}"

usage() {
  echo "用法：bash scripts/reconcile-processing-attempt-ingestion-mode.sh [--mode clickhouse-mv|status] [--correlation ID]" >&2
}

EVENT_HUNTER_REQUESTED_MODE="${EVENT_HUNTER_ATTEMPTS_INGESTION_MODE:-clickhouse-mv}"
EVENT_HUNTER_CORRELATION_ID=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --mode) EVENT_HUNTER_REQUESTED_MODE="${2:-}"; shift 2 ;;
    --correlation) EVENT_HUNTER_CORRELATION_ID="${2:-}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) usage; exit 2 ;;
  esac
done

case "${EVENT_HUNTER_REQUESTED_MODE}" in
  clickhouse-mv|status) ;;
  *) usage; exit 2 ;;
esac

validate_identifier() {
  local value="$1"
  local label="$2"
  if [[ ! "${value}" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]]; then
    echo "${label} 不是安全的 ClickHouse identifier：${value}" >&2
    exit 2
  fi
}

validate_identifier "${EVENT_HUNTER_ATTEMPTS_CANONICAL_ACTIVE_VIEW}" "attempt active view"
validate_identifier "${EVENT_HUNTER_ATTEMPTS_CANONICAL_STANDBY_VIEW}" "attempt standby view"
validate_identifier "${EVENT_HUNTER_LEGACY_ATTEMPTS_TABLE}" "legacy attempts table"
validate_identifier "${EVENT_HUNTER_CANDIDATE_ATTEMPTS_TABLE}" "candidate attempts table"

clickhouse_query() {
  local statement="$1"
  docker compose exec -T clickhouse sh -ec \
    'clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB" --query "$1"' \
    -- "${statement}"
}

active_source() {
  clickhouse_query "SELECT multiIf(position(create_table_query,'${EVENT_HUNTER_CANDIDATE_ATTEMPTS_TABLE}') > 0,'clickhouse-mv',position(create_table_query,'${EVENT_HUNTER_LEGACY_ATTEMPTS_TABLE}') > 0,'legacy','unknown') FROM system.tables WHERE database=currentDatabase() AND name='${EVENT_HUNTER_ATTEMPTS_CANONICAL_ACTIVE_VIEW}'"
}

exchange_views() {
  clickhouse_query "EXCHANGE TABLES ${EVENT_HUNTER_ATTEMPTS_CANONICAL_ACTIVE_VIEW} AND ${EVENT_HUNTER_ATTEMPTS_CANONICAL_STANDBY_VIEW}"
}

runtime_env() {
  local key="$1"
  local fallback="$2"
  docker compose exec -T event-hunter-api printenv "${key}" 2>/dev/null || printf '%s\n' "${fallback}"
}

reconcile_api() {
  local attempts_mode="$1"
  local domain_mode
  local rate_window
  local rate_requests
  domain_mode="$(runtime_env EVENT_HUNTER_INGESTION_MODE "${EVENT_HUNTER_INGESTION_MODE:-clickhouse-mv}")"
  rate_window="$(runtime_env EVENT_HUNTER_RATE_LIMIT_WINDOW 1m)"
  rate_requests="$(runtime_env EVENT_HUNTER_RATE_LIMIT_REQUESTS 300)"
  EVENT_HUNTER_INGESTION_MODE="${domain_mode}" \
  EVENT_HUNTER_ATTEMPTS_INGESTION_MODE="${attempts_mode}" \
  EVENT_HUNTER_RATE_LIMIT_WINDOW="${rate_window}" \
  EVENT_HUNTER_RATE_LIMIT_REQUESTS="${rate_requests}" \
    docker compose up -d --no-deps --build --force-recreate --wait event-hunter-api quality-worker event-lab >/dev/null
  docker compose up -d --no-deps --force-recreate --wait grafana >/dev/null
}

ensure_views() {
  local count
  count="$(clickhouse_query "SELECT count() FROM system.tables WHERE database=currentDatabase() AND name IN ('${EVENT_HUNTER_ATTEMPTS_CANONICAL_ACTIVE_VIEW}','${EVENT_HUNTER_ATTEMPTS_CANONICAL_STANDBY_VIEW}')")"
  if [[ "${count}" != "2" ]]; then
    echo "Processing-attempt canonical views 尚未建立；請先啟動 ClickHouse MV POC。" >&2
    exit 1
  fi
}

show_status() {
  echo "Processing-attempt read source: $(active_source) (${EVENT_HUNTER_ATTEMPTS_CANONICAL_ACTIVE_VIEW})"
  echo "Committed default mode: ${EVENT_HUNTER_ATTEMPTS_INGESTION_MODE:-clickhouse-mv}"
}

if [[ "${EVENT_HUNTER_REQUESTED_MODE}" != "status" ]]; then
  bash scripts/clickhouse-mv-poc-up.sh
fi
ensure_views

case "${EVENT_HUNTER_REQUESTED_MODE}" in
  status)
    show_status
    ;;
  clickhouse-mv)
    if [[ -z "${EVENT_HUNTER_CORRELATION_ID}" && "$(active_source)" != "clickhouse-mv" ]]; then
      echo "clickhouse-mv cutover 必須提供已通過雙寫的 correlation ID。" >&2
      exit 2
    fi
    if [[ -n "${EVENT_HUNTER_CORRELATION_ID}" ]]; then
      bash scripts/verify-processing-attempt-shadow.sh --correlation "${EVENT_HUNTER_CORRELATION_ID}"
    fi
    if [[ "$(active_source)" == "legacy" ]]; then
      exchange_views
    fi
    reconcile_api clickhouse-mv
    bash scripts/verify-event-pipeline-readiness.sh
    show_status
    ;;
esac
