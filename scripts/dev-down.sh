#!/usr/bin/env bash
set -euo pipefail

# down 不刪除 named volumes；PostgreSQL、ClickHouse 與 Grafana 資料會保留。
EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

# 帶入 temporal profile，確保曾啟動的選用 Temporal container 也會停止。
docker compose --profile temporal down

echo "Event Hunter 本機容器已停止；資料 volume 仍保留。"
