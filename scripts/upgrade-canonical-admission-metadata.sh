#!/usr/bin/env bash
set -euo pipefail

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

EVENT_HUNTER_QUIET=false
if [[ -n "${1:-}" ]]; then
  if [[ "$1" != "--quiet" || -n "${2:-}" ]]; then
    echo "用法：bash scripts/upgrade-canonical-admission-metadata.sh [--quiet]" >&2
    exit 2
  fi
  EVENT_HUNTER_QUIET=true
fi

EVENT_HUNTER_CONFIG_PATH="${EVENT_HUNTER_INGESTION_CONFIG:-config/ingestion-cutover.env}"
if [[ ! -f "${EVENT_HUNTER_CONFIG_PATH}" ]]; then
  echo "找不到 ingestion cutover 設定：${EVENT_HUNTER_CONFIG_PATH}" >&2
  exit 2
fi
# shellcheck disable=SC1090
source "${EVENT_HUNTER_CONFIG_PATH}"

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

EVENT_HUNTER_NEXT_ACTIVE_VIEW="_canonical_admission_next_active"
EVENT_HUNTER_NEXT_STANDBY_VIEW="_canonical_admission_next_standby"

clickhouse_query() {
  local statement="$1"
  docker compose exec -T clickhouse sh -ec \
    'clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB" --query "$1"' \
    -- "${statement}"
}

views_have_admission_metadata() {
  [[ "$(clickhouse_query "SELECT count() FROM system.columns WHERE database=currentDatabase() AND table IN ('${EVENT_HUNTER_CANONICAL_ACTIVE_VIEW}','${EVENT_HUNTER_CANONICAL_STANDBY_VIEW}') AND name IN ('admission_status','quality_flags','admission_profile')")" == "6" ]]
}

active_source() {
  clickhouse_query "SELECT multiIf(position(create_table_query,'${EVENT_HUNTER_CANDIDATE_EVENTS_TABLE}') > 0,'clickhouse-mv',position(create_table_query,'${EVENT_HUNTER_LEGACY_EVENTS_TABLE}') > 0,'legacy','unknown') FROM system.tables WHERE database=currentDatabase() AND name='${EVENT_HUNTER_CANONICAL_ACTIVE_VIEW}'"
}

create_legacy_view() {
  local view_name="$1"
  clickhouse_query "CREATE VIEW ${view_name} AS SELECT event_id,event_type,event_version,occurred_at,producer,correlation_id,causation_id,trace_id,aggregate_type,aggregate_id,sequence,kafka_topic,kafka_partition,kafka_offset,service_version,'SEARCHABLE' AS admission_status,CAST([],'Array(String)') AS quality_flags,'domain-event-json-schema-v1' AS admission_profile,payload,ingested_at FROM ${EVENT_HUNTER_LEGACY_EVENTS_TABLE}"
}

create_candidate_view() {
  local view_name="$1"
  clickhouse_query "CREATE VIEW ${view_name} AS SELECT event_id,event_type,event_version,occurred_at,producer,correlation_id,causation_id,trace_id,aggregate_type,aggregate_id,sequence,kafka_topic,kafka_partition,kafka_offset,CAST(NULL,'Nullable(String)') AS service_version,admission_status,quality_flags,admission_profile,payload,ingested_at FROM ${EVENT_HUNTER_CANDIDATE_EVENTS_TABLE}"
}

if views_have_admission_metadata; then
  if [[ "${EVENT_HUNTER_QUIET}" != "true" ]]; then
    echo "Canonical views already expose admission metadata."
  fi
  exit 0
fi

EVENT_HUNTER_ACTIVE_SOURCE="$(active_source)"
if [[ "${EVENT_HUNTER_ACTIVE_SOURCE}" != "legacy" && "${EVENT_HUNTER_ACTIVE_SOURCE}" != "clickhouse-mv" ]]; then
  echo "無法辨識 canonical active source，拒絕重建 views。" >&2
  exit 1
fi

# Only these fixed internal names are removed. They may remain after an interrupted
# prior upgrade and never serve runtime reads.
clickhouse_query "DROP VIEW IF EXISTS ${EVENT_HUNTER_NEXT_ACTIVE_VIEW}"
clickhouse_query "DROP VIEW IF EXISTS ${EVENT_HUNTER_NEXT_STANDBY_VIEW}"

if [[ "${EVENT_HUNTER_ACTIVE_SOURCE}" == "legacy" ]]; then
  create_legacy_view "${EVENT_HUNTER_NEXT_ACTIVE_VIEW}"
  create_candidate_view "${EVENT_HUNTER_NEXT_STANDBY_VIEW}"
else
  create_candidate_view "${EVENT_HUNTER_NEXT_ACTIVE_VIEW}"
  create_legacy_view "${EVENT_HUNTER_NEXT_STANDBY_VIEW}"
fi

clickhouse_query "EXCHANGE TABLES ${EVENT_HUNTER_CANONICAL_ACTIVE_VIEW} AND ${EVENT_HUNTER_NEXT_ACTIVE_VIEW}"
clickhouse_query "EXCHANGE TABLES ${EVENT_HUNTER_CANONICAL_STANDBY_VIEW} AND ${EVENT_HUNTER_NEXT_STANDBY_VIEW}"
clickhouse_query "DROP VIEW ${EVENT_HUNTER_NEXT_ACTIVE_VIEW}"
clickhouse_query "DROP VIEW ${EVENT_HUNTER_NEXT_STANDBY_VIEW}"

if ! views_have_admission_metadata || [[ "$(active_source)" != "${EVENT_HUNTER_ACTIVE_SOURCE}" ]]; then
  echo "Canonical admission metadata upgrade 未保留 view contract 或 active source。" >&2
  exit 1
fi

if [[ "${EVENT_HUNTER_QUIET}" != "true" ]]; then
  echo "Canonical views now expose admission metadata; active source remains ${EVENT_HUNTER_ACTIVE_SOURCE}."
fi
