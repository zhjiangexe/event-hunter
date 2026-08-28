# P1-09 Ingestion Pipeline／Processing Attempt 驗收紀錄

驗收日期：2026-08-21

## 結果

- Runtime schema：Redpanda Connect 以 JSON Schema 2020-12 驗證 canonical envelope 與 Phase 1 六種 domain events。
- Debezium Outbox：JSON payload 展開時保留 nullable `causationId`／`traceId`，實際 connector 設定已重新套用。
- Source acknowledgement：ClickHouse 停止時 `order.events` offset 36 維持未提交且 lag 為 1；ClickHouse 恢復後才提交為 37。
- Schema violation：缺少必填 `currency` 的 `OrderCreated` 被分類為 `SCHEMA_VIOLATION`，未寫入 `forensics_events`。
- Restricted DLQ：DLQ 保存 raw payload base64、來源 topic／partition／offset 與 SHA-256；ClickHouse `event_ingestion_failures` 僅保存 checksum metadata，沒有 raw-payload 欄位。
- Transport redelivery：Timeline 以 Kafka topic／partition／offset 作 delivery identity，append-only sink 的重送不會產生第二筆邏輯事件。
- Processing-attempt identity：相同 `attemptId` 的兩次實體 delivery 只計為一次邏輯 attempt，結果為 `1|1|SUCCEEDED`。
- Timeline processing summary：Karate 驗證 payment event 的 `attempt_count=3`、`final_status=DLQ`、`consumer_group_id=shipping-service-v1`。
- Generated frontend client：`openapi-typescript 7.13.0` 產生 `paths`／`components`／`operations`，`openapi-fetch 0.17.0` 統一 request serialization 與錯誤處理。
- Contract drift gate：`pnpm api:check` 通過；生成型別同時發現並修正 Investigation 的 `cursor`／`next_cursor` 契約漂移。
- Frontend regression：TypeScript、production build、20 個 Vitest 與 5 個 browser Karate scenarios 全部通過。
- Backend regression：7 features、28 scenarios，全部通過。

## 執行入口

```bash
python3 scripts/validate-contracts.py
GOCACHE=/tmp/event-hunter-go-cache go -C backend test ./...
pnpm --dir frontend run api:check
pnpm --dir frontend run typecheck
pnpm --dir frontend run test:run
pnpm --dir frontend run build
bash scripts/test-ingestion-pipeline.sh
bash scripts/test-ingestion-acknowledgement.sh --yes
python3 scripts/load-domain-fixtures.py
bash scripts/test-backend-e2e.sh
bash scripts/test-frontend-e2e.sh
```

Karate reports：`artifacts/e2e/karate/backend/karate-summary.html`、`artifacts/e2e/karate/frontend/karate-summary.html`

`EH-MVP-012` 與 `EH-MVP-013` 的直接 outputs／acceptance 已完成。依 `implementation-plan.yaml` 的嚴格 dependency completion rule，兩者仍維持 `in_progress`，直到各自的上游 `EH-MVP-003`／`EH-MVP-011` task status 完成；`EH-MVP-014` 也因此暫不改為 `completed`。
