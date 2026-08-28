#!/usr/bin/env bash
set -euo pipefail

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

usage() {
  cat >&2 <<'USAGE'
用法：bash scripts/purge-clickhouse-mv-raw.sh --from RFC3339 --to RFC3339 [--execute --yes]

預設只顯示筆數，不刪除資料。實際刪除必須同時指定 --execute --yes。
每次時間窗最多 24 小時，且 --to 必須早於目前時間至少 1 小時，避免清除仍在處理的資料。
USAGE
}

EVENT_HUNTER_PURGE_FROM=""
EVENT_HUNTER_PURGE_TO=""
EVENT_HUNTER_PURGE_EXECUTE=false
EVENT_HUNTER_PURGE_CONFIRMED=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --from)
      EVENT_HUNTER_PURGE_FROM="${2:-}"
      shift 2
      ;;
    --to)
      EVENT_HUNTER_PURGE_TO="${2:-}"
      shift 2
      ;;
    --execute)
      EVENT_HUNTER_PURGE_EXECUTE=true
      shift
      ;;
    --yes)
      EVENT_HUNTER_PURGE_CONFIRMED=true
      shift
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

if [[ -z "${EVENT_HUNTER_PURGE_FROM}" || -z "${EVENT_HUNTER_PURGE_TO}" ]]; then
  usage
  exit 2
fi
if [[ "${EVENT_HUNTER_PURGE_EXECUTE}" != "${EVENT_HUNTER_PURGE_CONFIRMED}" ]]; then
  echo "實際刪除必須同時指定 --execute --yes；dry-run 不可附帶其中一個參數。" >&2
  exit 2
fi

EVENT_HUNTER_PURGE_VALIDATION="$(python3 - "${EVENT_HUNTER_PURGE_FROM}" "${EVENT_HUNTER_PURGE_TO}" <<'PY'
import datetime
import sys

def parse(value: str) -> datetime.datetime:
    parsed = datetime.datetime.fromisoformat(value.replace("Z", "+00:00"))
    if parsed.tzinfo is None:
        raise ValueError("timezone is required")
    return parsed.astimezone(datetime.timezone.utc)

try:
    start = parse(sys.argv[1])
    end = parse(sys.argv[2])
except ValueError as exc:
    raise SystemExit(f"invalid RFC3339 window: {exc}")

now = datetime.datetime.now(datetime.timezone.utc)
if start >= end:
    raise SystemExit("--from must be earlier than --to")
if end - start > datetime.timedelta(hours=24):
    raise SystemExit("purge window must not exceed 24 hours")
if end > now - datetime.timedelta(hours=1):
    raise SystemExit("--to must be at least one hour in the past")

print(start.isoformat().replace("+00:00", "Z"))
print(end.isoformat().replace("+00:00", "Z"))
PY
)" || {
  echo "raw purge 時間窗不合法。" >&2
  exit 2
}

EVENT_HUNTER_PURGE_FROM_NORMALIZED="$(printf '%s\n' "${EVENT_HUNTER_PURGE_VALIDATION}" | sed -n '1p')"
EVENT_HUNTER_PURGE_TO_NORMALIZED="$(printf '%s\n' "${EVENT_HUNTER_PURGE_VALIDATION}" | sed -n '2p')"

clickhouse_query() {
  docker compose exec -T clickhouse sh -ec \
    'clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --database "$CLICKHOUSE_DB" --query "$1"' \
    -- "$1"
}

EVENT_HUNTER_PURGE_PREDICATE="received_at >= parseDateTime64BestEffort('${EVENT_HUNTER_PURGE_FROM_NORMALIZED}') AND received_at < parseDateTime64BestEffort('${EVENT_HUNTER_PURGE_TO_NORMALIZED}')"
EVENT_HUNTER_PURGE_COUNT="$(clickhouse_query "SELECT count() FROM event_hunter_poc.poc_event_landing_raw FINAL WHERE ${EVENT_HUNTER_PURGE_PREDICATE}")"

if [[ "${EVENT_HUNTER_PURGE_EXECUTE}" == "false" ]]; then
  echo "Raw landing purge dry-run: from=${EVENT_HUNTER_PURGE_FROM_NORMALIZED} to=${EVENT_HUNTER_PURGE_TO_NORMALIZED} rows=${EVENT_HUNTER_PURGE_COUNT}"
  exit 0
fi

clickhouse_query "ALTER TABLE event_hunter_poc.poc_event_landing_raw DELETE WHERE ${EVENT_HUNTER_PURGE_PREDICATE} SETTINGS mutations_sync = 1" >/dev/null
EVENT_HUNTER_PURGE_REMAINING="$(clickhouse_query "SELECT count() FROM event_hunter_poc.poc_event_landing_raw FINAL WHERE ${EVENT_HUNTER_PURGE_PREDICATE}")"
if [[ "${EVENT_HUNTER_PURGE_REMAINING}" != "0" ]]; then
  echo "raw purge 完成後時間窗內仍有 ${EVENT_HUNTER_PURGE_REMAINING} 筆資料。" >&2
  exit 1
fi

echo "Raw landing purge executed: from=${EVENT_HUNTER_PURGE_FROM_NORMALIZED} to=${EVENT_HUNTER_PURGE_TO_NORMALIZED} deleted=${EVENT_HUNTER_PURGE_COUNT} remaining=0"
