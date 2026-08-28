#!/usr/bin/env bash
set -euo pipefail

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

EVENT_HUNTER_CLEAN_REPORTS=false
case "${1:-}" in
  "") ;;
  --reports) EVENT_HUNTER_CLEAN_REPORTS=true ;;
  *)
    echo "用法：bash scripts/clean-generated-artifacts.sh [--reports]" >&2
    exit 2
    ;;
esac

# These directories contain reproducible compiler, browser and Python cache
# output only. Persistent database volumes and canonical fixtures are outside
# this cleanup boundary.
rm -rf -- target frontend/dist scripts/__pycache__

if [[ "${EVENT_HUNTER_CLEAN_REPORTS}" == "true" ]]; then
  # Keep only the canonical latest full-suite reports referenced by sign-off
  # documents. Tagged/debug/history reports are reproducible and unreferenced.
  if [[ -d artifacts/e2e/karate ]]; then
    for report_dir in artifacts/e2e/karate/*; do
      [[ -e "${report_dir}" ]] || continue
      case "${report_dir}" in
        artifacts/e2e/karate/backend|artifacts/e2e/karate/frontend) ;;
        *) rm -rf -- "${report_dir}" ;;
      esac
    done
  fi
  rm -rf -- artifacts/e2e/legacy-maven
fi

echo "Generated artifacts cleaned. Canonical backend/frontend Karate reports were preserved."

