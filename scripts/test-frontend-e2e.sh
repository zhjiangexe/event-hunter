#!/usr/bin/env bash
set -euo pipefail

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"
source scripts/lib/karate.sh
EVENT_HUNTER_OUTPUT_DIR="${KARATE_OUTPUT_DIR:-artifacts/e2e/karate/frontend}"
event_hunter_run_karate "${EVENT_HUNTER_OUTPUT_DIR}" e2e/frontend/investigation-flow.feature
