#!/usr/bin/env bash
set -euo pipefail

# 同時列出預設服務與選用 Temporal profile 的狀態。
EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

docker compose --profile temporal ps
