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

# Older local databases may have canonical views created before admission metadata
# was part of the API contract. The upgrade preserves whichever source is active.
bash scripts/upgrade-canonical-admission-metadata.sh --quiet

usage() {
  cat >&2 <<'USAGE'
用法：bash scripts/reconcile-ingestion-mode.sh [--mode clickhouse-mv|status] [--correlation ID]

未指定 --mode 時使用 config/ingestion-cutover.env 的 EVENT_HUNTER_INGESTION_MODE。
只有從尚未切換的舊資料庫首次 cutover 時，才必須提供已通過 parity 的 correlation ID。
USAGE
}

EVENT_HUNTER_REQUESTED_MODE="${EVENT_HUNTER_INGESTION_MODE:-clickhouse-mv}"
EVENT_HUNTER_CORRELATION_ID=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --mode)
      EVENT_HUNTER_REQUESTED_MODE="${2:-}"
      shift 2
      ;;
    --correlation)
      EVENT_HUNTER_CORRELATION_ID="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage
      exit 2
      ;;
  esac
done

case "${EVENT_HUNTER_REQUESTED_MODE}" in
  clickhouse-mv|status) ;;
  *)
    usage
    exit 2
    ;;
esac

validate_identifier() {
  local value="$1"
  local label="$2"
  if [[ ! "${value}" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]]; then
    echo "${label} 不是安全的 ClickHouse identifier：${value}" >&2
    exit 2
  fi
}

validate_identifier "${EVENT_HUNTER_CANONICAL_ACTIVE_VIEW}" "active view"
validate_identifier "${EVENT_HUNTER_CANONICAL_STANDBY_VIEW}" "standby view"
validate_identifier "${EVENT_HUNTER_LEGACY_EVENTS_TABLE}" "legacy table"
validate_identifier "${EVENT_HUNTER_CANDIDATE_EVENTS_TABLE}" "candidate table"

clickhouse_query() {
  local statement="$1"
  docker compose exec -T clickhouse sh -ec \
    'clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB" --query "$1"' \
    -- "${statement}"
}

EVENT_HUNTER_VIEW_COUNT="$(clickhouse_query "SELECT count() FROM system.tables WHERE database=currentDatabase() AND name IN ('${EVENT_HUNTER_CANONICAL_ACTIVE_VIEW}','${EVENT_HUNTER_CANONICAL_STANDBY_VIEW}')")"
if [[ "${EVENT_HUNTER_VIEW_COUNT}" != "2" ]]; then
  echo "canonical views 尚未建立；請先執行 bash scripts/dev-migrate.sh。" >&2
  exit 1
fi

active_source() {
  clickhouse_query "SELECT multiIf(position(create_table_query,'${EVENT_HUNTER_CANDIDATE_EVENTS_TABLE}') > 0,'clickhouse-mv',position(create_table_query,'${EVENT_HUNTER_LEGACY_EVENTS_TABLE}') > 0,'legacy','unknown') FROM system.tables WHERE database=currentDatabase() AND name='${EVENT_HUNTER_CANONICAL_ACTIVE_VIEW}'"
}

exchange_views() {
  clickhouse_query "EXCHANGE TABLES ${EVENT_HUNTER_CANONICAL_ACTIVE_VIEW} AND ${EVENT_HUNTER_CANONICAL_STANDBY_VIEW}"
}

reconcile_api_readiness_mode() {
  local mode="$1"
  echo "Reconcile API readiness probes: ${mode}."
  EVENT_HUNTER_INGESTION_MODE="${mode}" docker compose up -d --build --no-deps --force-recreate --wait event-hunter-api >/dev/null
  curl --fail --silent --show-error "http://localhost:${EVENT_HUNTER_API_PORT:-28333}/health/ready" >/dev/null
}

show_status() {
  local source
  source="$(active_source)"
  echo "Ingestion read source: ${source} (${EVENT_HUNTER_CANONICAL_ACTIVE_VIEW})"
  echo "Committed default mode: ${EVENT_HUNTER_INGESTION_MODE:-clickhouse-mv}"
}

case "${EVENT_HUNTER_REQUESTED_MODE}" in
  status)
    show_status
    ;;
  clickhouse-mv)
    if [[ -z "${EVENT_HUNTER_CORRELATION_ID}" && "$(active_source)" != "clickhouse-mv" ]]; then
      echo "clickhouse-mv cutover 必須提供 --correlation，不能略過 parity gate。" >&2
      exit 2
    fi
    bash scripts/clickhouse-mv-poc-up.sh
    if [[ -n "${EVENT_HUNTER_CORRELATION_ID}" ]]; then
      bash scripts/verify-ingestion-shadow.sh --correlation "${EVENT_HUNTER_CORRELATION_ID}"
    fi
    if [[ "$(active_source)" == "legacy" ]]; then
      exchange_views
      echo "Cutover completed: canonical view 已原子切到 clickhouse-mv candidate。"
    else
      echo "ClickHouse MV already active; no view exchange required."
    fi
    if ! reconcile_api_readiness_mode clickhouse-mv; then
      echo "ClickHouse-first readiness 未通過；canonical view 保持目前來源，請依 runbook 修復 Sink／ClickHouse。" >&2
      exit 1
    fi
    show_status
    ;;
esac
