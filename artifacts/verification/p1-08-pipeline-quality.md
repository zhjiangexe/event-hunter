# P1-08 Pipeline／Quality 驗收紀錄

驗收日期：2026-08-21

## 結果

- Quality Worker scheduler：healthy；每分鐘執行一分鐘 UTC tumbling window，late-arrival grace 為兩分鐘。
- Deterministic backfill：`quality-window.json` → production `quality-worker aggregate` → ClickHouse → Karate，2 scenarios passed，包含只有 ingestion failure 的窗口。
- Failure mode：ClickHouse unavailable 時 worker 回傳非零，固定失敗窗口前後 row count 不變。
- Grafana provisioning：4 datasources、1 dashboard、6 file-provisioned alert rules。
- Alert evaluation：6 rules 的 Grafana health 均為 `ok`，provenance 均為 `file`。
- Restart persistence：PostgreSQL investigation cases `200 → 200`、ClickHouse forensics events `79 → 79`、Redpanda 原有 topics 全數保留。
- Restart recovery：完整 Compose services 回復 healthy，Grafana assets 與 Quality Worker health 再驗證通過。

## 執行入口

```bash
python3 scripts/validate-contracts.py
GOCACHE=/tmp/event-hunter-go-cache go -C backend test ./...
bash scripts/test-quality-e2e.sh
bash scripts/verify-quality-runtime.sh
bash scripts/verify-grafana-provisioning.sh
bash scripts/verify-restart-persistence.sh
```

Karate report：`artifacts/e2e/karate/backend-quality/karate-summary.html`

`EH-MVP-014` 的直接 outputs／acceptance 已完成；依 `implementation-plan.yaml` 的 dependency completion rule，仍維持 `in_progress`，直到 `EH-MVP-012` 與 `EH-MVP-013` 的剩餘 acceptance 在後續工作包關閉。
