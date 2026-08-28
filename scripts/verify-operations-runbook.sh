#!/usr/bin/env bash
set -euo pipefail

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

EVENT_HUNTER_STATIC_ONLY=false
EVENT_HUNTER_RESTART=false
EVENT_HUNTER_CONFIRMED=false

for EVENT_HUNTER_ARGUMENT in "$@"; do
  case "${EVENT_HUNTER_ARGUMENT}" in
    --static) EVENT_HUNTER_STATIC_ONLY=true ;;
    --restart) EVENT_HUNTER_RESTART=true ;;
    --yes) EVENT_HUNTER_CONFIRMED=true ;;
    *)
      echo "用法：bash scripts/verify-operations-runbook.sh [--static] [--restart --yes]" >&2
      exit 2
      ;;
  esac
done

if [[ "${EVENT_HUNTER_STATIC_ONLY}" == "true" && "${EVENT_HUNTER_RESTART}" == "true" ]]; then
  echo "--static 與 --restart 不可同時使用。" >&2
  exit 2
fi
if [[ "${EVENT_HUNTER_RESTART}" == "true" && "${EVENT_HUNTER_CONFIRMED}" != "true" ]]; then
  echo "--restart 會依序重啟本機服務；確認可中斷後加上 --yes。" >&2
  exit 2
fi

for EVENT_HUNTER_REQUIRED_FILE in \
  requirements/operations/operations-runbook.md \
  compose.yaml \
  .env.example \
  scripts/dev-up.sh \
  scripts/dev-down.sh \
  scripts/dev-status.sh \
  scripts/verify-persistence.sh \
  scripts/verify-event-pipeline-readiness.sh \
  scripts/verify-restart-persistence.sh \
  scripts/test-event-check-source-failure.sh \
  scripts/test-event-check-restart-persistence.sh \
  scripts/backup-local-volumes.sh; do
  if [[ ! -f "${EVENT_HUNTER_REQUIRED_FILE}" ]]; then
    echo "Runbook 依賴檔案不存在：${EVENT_HUNTER_REQUIRED_FILE}" >&2
    exit 1
  fi
done

for EVENT_HUNTER_SCRIPT in scripts/*.sh; do
  bash -n "${EVENT_HUNTER_SCRIPT}"
done
python3 scripts/validate-contracts.py

if [[ "${EVENT_HUNTER_STATIC_ONLY}" == "true" ]]; then
  echo "Operations runbook static verification passed."
  exit 0
fi

docker compose config --quiet
bash scripts/verify-persistence.sh
bash scripts/verify-event-pipeline-readiness.sh
bash scripts/verify-grafana-provisioning.sh

curl --fail --silent --show-error "http://localhost:${EVENT_HUNTER_API_PORT:-28333}/health/live" >/dev/null
curl --fail --silent --show-error "http://localhost:${EVENT_HUNTER_API_PORT:-28333}/health/ready" >/dev/null
curl --fail --silent --show-error "http://localhost:${EVENT_LAB_PORT:-28343}/health/ready" >/dev/null
curl --fail --silent --show-error "http://localhost:${EVENT_HUNTER_FRONTEND_PORT:-28334}/login" >/dev/null

if [[ "${EVENT_HUNTER_RESTART}" == "true" ]]; then
  bash scripts/verify-restart-persistence.sh
fi

echo "Operations runbook smoke verification passed."
