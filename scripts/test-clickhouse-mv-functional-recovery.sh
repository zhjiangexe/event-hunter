#!/usr/bin/env bash
set -euo pipefail

# Backward-compatible entry point retained for runbooks and CI jobs created
# while ClickHouse-first was still a candidate. The adopted route now requires
# both domain events and processing attempts to pass the stricter candidate-only
# outage/backlog recovery test.
EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

exec bash scripts/test-clickhouse-mv-candidate-only-recovery.sh
