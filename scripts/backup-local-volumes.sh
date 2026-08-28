#!/usr/bin/env bash
set -euo pipefail

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

if [[ "${1:-}" != "--output" || -z "${2:-}" || -n "${3:-}" ]]; then
  echo "用法：bash scripts/backup-local-volumes.sh --output <new-backup-directory>" >&2
  exit 2
fi

EVENT_HUNTER_BACKUP_DIR="$2"
if [[ "${EVENT_HUNTER_BACKUP_DIR}" != /* ]]; then
  EVENT_HUNTER_BACKUP_DIR="${EVENT_HUNTER_PROJECT_DIR}/${EVENT_HUNTER_BACKUP_DIR}"
fi
if [[ -e "${EVENT_HUNTER_BACKUP_DIR}" ]]; then
  echo "備份目錄已存在，拒絕覆寫：${EVENT_HUNTER_BACKUP_DIR}" >&2
  exit 1
fi
if [[ -n "$(docker compose --profile temporal ps -q)" ]]; then
  echo "一致性 volume snapshot 前必須先執行 bash scripts/dev-down.sh。" >&2
  exit 1
fi

EVENT_HUNTER_COMPOSE_PROJECT="${COMPOSE_PROJECT_NAME:-event-hunter}"
EVENT_HUNTER_VOLUME_NAMES=(
  postgres_data
  demo_order_postgres_data
  demo_payment_postgres_data
  demo_shipping_postgres_data
  clickhouse_data
  redpanda_data
  prometheus_data
  loki_data
  tempo_data
  grafana_data
  temporal_data
)

mkdir -p "${EVENT_HUNTER_BACKUP_DIR}"

for EVENT_HUNTER_LOGICAL_VOLUME in "${EVENT_HUNTER_VOLUME_NAMES[@]}"; do
  EVENT_HUNTER_VOLUME="${EVENT_HUNTER_COMPOSE_PROJECT}_${EVENT_HUNTER_LOGICAL_VOLUME}"
  if ! docker volume inspect "${EVENT_HUNTER_VOLUME}" >/dev/null 2>&1; then
    echo "略過不存在的 volume：${EVENT_HUNTER_VOLUME}"
    continue
  fi
  docker run --rm \
    --volume "${EVENT_HUNTER_VOLUME}:/source:ro" \
    --volume "${EVENT_HUNTER_BACKUP_DIR}:/backup" \
    postgres:18.4-alpine \
    tar -C /source -czf "/backup/${EVENT_HUNTER_LOGICAL_VOLUME}.tar.gz" .
done

(
  cd "${EVENT_HUNTER_BACKUP_DIR}"
  shasum -a 256 ./*.tar.gz > SHA256SUMS
)
docker compose config > "${EVENT_HUNTER_BACKUP_DIR}/compose.resolved.yaml"
cp .env.example "${EVENT_HUNTER_BACKUP_DIR}/env.example.snapshot"

echo "Event Hunter cold volume backup completed: ${EVENT_HUNTER_BACKUP_DIR}"
echo "請妥善保護備份；其中可能包含 CONFIDENTIAL event payload 與本機 secrets。"
