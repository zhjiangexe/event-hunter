#!/usr/bin/env bash
set -euo pipefail

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

python3 scripts/load-domain-fixtures.py
python3 scripts/load-observability-fixtures.py
docker compose run --rm quality-worker backfill \
  --from=2026-08-20T10:00:00Z \
  --to=2026-08-20T14:02:00Z \
  --window=1m

EVENT_HUNTER_QUALITY_COUNT="$(curl --fail --silent --show-error \
  --user "${CLICKHOUSE_USER:-event_hunter}:${CLICKHOUSE_PASSWORD:-event_hunter_local_only}" \
  --data-binary "SELECT count() FROM event_hunter.event_quality_metrics WHERE window_start >= toDateTime64('2026-08-20 10:00:00',3,'UTC') AND window_start < toDateTime64('2026-08-20 14:02:00',3,'UTC') AND source = 'quality-worker-v1'" \
  "${CLICKHOUSE_URL:-http://localhost:28317}")"
if [[ "${EVENT_HUNTER_QUALITY_COUNT}" -lt 1 ]]; then
  echo "Quality fixture backfill produced no event-time rows." >&2
  exit 1
fi

echo "Loaded domain, trace, log and ${EVENT_HUNTER_QUALITY_COUNT} event-time quality fixture rows."
