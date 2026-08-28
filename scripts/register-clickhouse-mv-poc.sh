#!/usr/bin/env bash
set -euo pipefail

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

EVENT_HUNTER_POC_CONNECT_URL="${CLICKHOUSE_POC_CONNECT_URL:-http://localhost:28345}"
EVENT_HUNTER_POC_CONNECTOR_TEMPLATES=(
  "infra/kafka-connect-clickhouse/connectors/poc-raw-landing.json"
  "infra/kafka-connect-clickhouse/connectors/poc-processing-attempt-raw-landing.json"
)

wait_for_connect_worker() {
  local EVENT_HUNTER_ATTEMPT
  for EVENT_HUNTER_ATTEMPT in {1..60}; do
    if curl --fail --silent --show-error "${EVENT_HUNTER_POC_CONNECT_URL}/connector-plugins" \
      | python3 -c '
import json
import sys

plugins = json.load(sys.stdin)
required = "com.clickhouse.kafka.connect.ClickHouseSinkConnector"
raise SystemExit(0 if any(plugin.get("class") == required for plugin in plugins) else 1)
' 2>/dev/null; then
      return 0
    fi
    sleep 1
  done
  echo "ClickHouse POC Connect worker 或官方 Sink plugin 未在 60 秒內 ready。" >&2
  return 1
}

wait_for_connector() {
  local EVENT_HUNTER_POC_CONNECTOR_NAME="$1"
  local EVENT_HUNTER_ATTEMPT
  for EVENT_HUNTER_ATTEMPT in {1..60}; do
    if curl --fail --silent --show-error \
      "${EVENT_HUNTER_POC_CONNECT_URL}/connectors/${EVENT_HUNTER_POC_CONNECTOR_NAME}/status" \
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

  echo "POC connector 未在 60 秒內進入 RUNNING。" >&2
  curl --silent --show-error \
    "${EVENT_HUNTER_POC_CONNECT_URL}/connectors/${EVENT_HUNTER_POC_CONNECTOR_NAME}/status" >&2 || true
  echo >&2
  return 1
}

render_connector_config() {
  local EVENT_HUNTER_POC_CONNECTOR_TEMPLATE="$1"
  local EVENT_HUNTER_RENDERED_CONFIG="$2"
  CLICKHOUSE_USER="${CLICKHOUSE_USER:-event_hunter}" \
  CLICKHOUSE_PASSWORD="${CLICKHOUSE_PASSWORD:-event_hunter_local_only}" \
  python3 - "${EVENT_HUNTER_POC_CONNECTOR_TEMPLATE}" > "${EVENT_HUNTER_RENDERED_CONFIG}" <<'PY'
import json
import os
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    document = json.load(handle)

config = document["config"]
config["username"] = os.environ["CLICKHOUSE_USER"]
config["password"] = os.environ["CLICKHOUSE_PASSWORD"]
json.dump(config, sys.stdout)
PY
}

wait_for_connect_worker
for EVENT_HUNTER_POC_CONNECTOR_TEMPLATE in "${EVENT_HUNTER_POC_CONNECTOR_TEMPLATES[@]}"; do
  EVENT_HUNTER_RENDERED_CONFIG="$(mktemp)"
  render_connector_config "${EVENT_HUNTER_POC_CONNECTOR_TEMPLATE}" "${EVENT_HUNTER_RENDERED_CONFIG}"
  EVENT_HUNTER_POC_CONNECTOR_NAME="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["name"])' "${EVENT_HUNTER_POC_CONNECTOR_TEMPLATE}")"
  curl --fail --silent --show-error \
    -X PUT "${EVENT_HUNTER_POC_CONNECT_URL}/connectors/${EVENT_HUNTER_POC_CONNECTOR_NAME}/config" \
    -H 'Content-Type: application/json' \
    --data-binary @"${EVENT_HUNTER_RENDERED_CONFIG}" \
    --output /dev/null
  rm -f "${EVENT_HUNTER_RENDERED_CONFIG}"
  wait_for_connector "${EVENT_HUNTER_POC_CONNECTOR_NAME}"
  echo "${EVENT_HUNTER_POC_CONNECTOR_NAME} 已註冊並進入 RUNNING。"
done
