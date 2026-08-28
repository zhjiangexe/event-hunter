#!/usr/bin/env bash
set -euo pipefail

# 從任何工作目錄執行時，都先切到 Event Hunter 專案根目錄。
EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

EVENT_HUNTER_CONFIG_PATH="${EVENT_HUNTER_INGESTION_CONFIG:-config/ingestion-cutover.env}"
if [[ ! -f "${EVENT_HUNTER_CONFIG_PATH}" ]]; then
  echo "找不到 ingestion 設定：${EVENT_HUNTER_CONFIG_PATH}" >&2
  exit 2
fi
# Compose 必須取得與 canonical views 相同的正式 ingestion mode。
# shellcheck disable=SC1090
set -a
source "${EVENT_HUNTER_CONFIG_PATH}"
set +a

EVENT_HUNTER_COMPOSE_ARGS=()
if [[ "${1:-}" == "--temporal" ]]; then
  # Temporal 只供長時間調查與後續 Replay 階段使用，預設不啟動。
  EVENT_HUNTER_COMPOSE_ARGS+=(--profile temporal)
elif [[ -n "${1:-}" ]]; then
  echo "用法：bash scripts/dev-up.sh [--temporal]" >&2
  exit 2
fi

compose_with_optional_profiles() {
  if ((${#EVENT_HUNTER_COMPOSE_ARGS[@]})); then
    docker compose "${EVENT_HUNTER_COMPOSE_ARGS[@]}" "$@"
  else
    docker compose "$@"
  fi
}

# Fresh volumes 尚未有 ClickHouse views 與 connector registrations，因此先啟動
# containers、不等待 API readiness；完成 migration/registration 後再做全 stack wait。
compose_with_optional_profiles up -d --build
docker compose up -d --wait \
  postgres demo-order-postgres demo-payment-postgres demo-shipping-postgres \
  clickhouse redpanda kafka-connect kafka-connect-clickhouse-poc
bash scripts/dev-migrate.sh
bash scripts/bootstrap-topics.sh
bash scripts/register-debezium-connectors.sh
bash scripts/clickhouse-mv-poc-up.sh

compose_with_optional_profiles up -d --build --wait
bash scripts/verify-event-pipeline-readiness.sh
bash scripts/load-demo-fixtures.sh

echo "Event Hunter 本機依賴已啟動："
echo "  Grafana:          http://localhost:${GRAFANA_PORT:-28332}"
echo "  Redpanda Console: http://localhost:${REDPANDA_CONSOLE_PORT:-28323}"
echo "  Prometheus:       http://localhost:${PROMETHEUS_PORT:-28326}"
echo "  ClickHouse HTTP:  http://localhost:${CLICKHOUSE_HTTP_PORT:-28317}"
echo "  Event Hunter UI:  http://localhost:${EVENT_HUNTER_FRONTEND_PORT:-28334}"
echo "  Scenario Lab API: http://localhost:${EVENT_LAB_PORT:-28343}"
if [[ "${1:-}" == "--temporal" ]]; then
  echo "  Temporal UI:      http://localhost:${TEMPORAL_UI_PORT:-28342}"
fi
