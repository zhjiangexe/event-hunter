#!/usr/bin/env bash
set -euo pipefail

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

EVENT_HUNTER_CLICKHOUSE_URL="${CLICKHOUSE_URL:-http://localhost:${CLICKHOUSE_HTTP_PORT:-28317}}"
EVENT_HUNTER_CLICKHOUSE_USER="${CLICKHOUSE_USER:-event_hunter}"
EVENT_HUNTER_CLICKHOUSE_PASSWORD="${CLICKHOUSE_PASSWORD:-event_hunter_local_only}"
EVENT_HUNTER_CLICKHOUSE_DB="${CLICKHOUSE_DB:-event_hunter}"
EVENT_HUNTER_QUALITY_HEALTH_URL="${QUALITY_WORKER_HEALTH_URL:-http://localhost:${QUALITY_WORKER_HEALTH_PORT:-28338}}"

clickhouse_query() {
  curl --fail --silent --show-error \
    --user "${EVENT_HUNTER_CLICKHOUSE_USER}:${EVENT_HUNTER_CLICKHOUSE_PASSWORD}" \
    --data-binary "$1" \
    "${EVENT_HUNTER_CLICKHOUSE_URL}/?database=${EVENT_HUNTER_CLICKHOUSE_DB}"
}

curl --fail --silent --show-error "${EVENT_HUNTER_QUALITY_HEALTH_URL}/health/live" >/dev/null

# A preceding disruptive check restarts ClickHouse. The scheduler intentionally
# reports 503 until it has completed the next eligible window, and retries on
# its one-minute tick. Wait for that real recovery instead of treating the
# transient readiness state as a release-gate failure.
EVENT_HUNTER_READY_DEADLINE=$((SECONDS + 90))
until curl --fail --silent --show-error "${EVENT_HUNTER_QUALITY_HEALTH_URL}/health/ready" >/dev/null 2>&1; do
  if (( SECONDS >= EVENT_HUNTER_READY_DEADLINE )); then
    echo "Quality Worker did not recover readiness within 90 seconds after dependency restart." >&2
    exit 1
  fi
  sleep 2
done

EVENT_HUNTER_FAILED_WINDOW_COUNT_BEFORE="$(clickhouse_query "SELECT count() FROM event_quality_metrics WHERE window_start = toDateTime64('2000-01-01 00:00:00', 3, 'UTC') AND window_end = toDateTime64('2000-01-01 00:01:00', 3, 'UTC')")"

set +e
docker compose run --rm --no-deps \
  -e CLICKHOUSE_URL=http://127.0.0.1:1 \
  quality-worker aggregate \
  --from=2000-01-01T00:00:00Z \
  --to=2000-01-01T00:01:00Z >/dev/null 2>&1
EVENT_HUNTER_FAILURE_STATUS=$?
set -e

if [[ "${EVENT_HUNTER_FAILURE_STATUS}" == "0" ]]; then
  echo "Quality Worker unexpectedly succeeded with an unavailable ClickHouse endpoint." >&2
  exit 1
fi

EVENT_HUNTER_FAILED_WINDOW_COUNT_AFTER="$(clickhouse_query "SELECT count() FROM event_quality_metrics WHERE window_start = toDateTime64('2000-01-01 00:00:00', 3, 'UTC') AND window_end = toDateTime64('2000-01-01 00:01:00', 3, 'UTC')")"
if [[ "${EVENT_HUNTER_FAILED_WINDOW_COUNT_BEFORE}" != "${EVENT_HUNTER_FAILED_WINDOW_COUNT_AFTER}" ]]; then
  echo "Failed quality aggregation wrote a partial metric row." >&2
  exit 1
fi

echo "Quality Worker verified: scheduler ready and failed window writes no partial metric row."
