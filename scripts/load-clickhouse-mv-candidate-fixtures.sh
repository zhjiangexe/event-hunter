#!/usr/bin/env bash
set -euo pipefail

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

# Compatibility entry point retained for existing local automation. Fixtures
# now load directly into the formally adopted promoted tables according to
# contracts/platform/ingestion-mapping.yaml; no legacy-to-candidate mirror exists.
echo "Compatibility wrapper: loading canonical synthetic fixtures into the adopted ClickHouse-first read models."
python3 scripts/load-domain-fixtures.py
