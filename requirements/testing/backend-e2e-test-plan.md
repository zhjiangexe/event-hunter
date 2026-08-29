---
document_id: EH-DOC-TEST-001
status: active
owner: quality
last_reviewed: 2026-08-29
source_of_truth: true
canonical_topic: backend-e2e-test-plan
supersedes: []
---

# Event Hunter Backend E2E 測試計畫

更新日期：2026-08-29

## 目的與邊界

Backend E2E 使用 Karate standalone JAR，從公開 HTTP 介面驗證跨越 handler、application、domain、
PostgreSQL、ClickHouse、Kafka 與外部 observability service 的可觀察結果。案例數量不是主要目標；
每個案例都必須保護一項使用者或營運人員真正依賴的契約。

以下內容不放進 Backend E2E：

- Aggregate 細部 invariant、repository error 與 transaction rollback：由 Go unit／integration test 驗證。
- React route、drawer、keyboard 與 browser history：由 Frontend E2E 驗證。
- 無法由公開介面穩定觸發的 timeout／dependency fault：由 fault-injection integration test 驗證。
- 只為增加案例數而重複相同 status code、欄位或 fixture 的排列組合。

## 案例合理性準則

新增案例前必須同時滿足：

1. 能對應 OpenAPI operation、穩定需求 ID 或已記錄的 failure mode。
2. 驗證外部可觀察行為，而不是複製內部實作。
3. 有明確終止條件；非同步流程只能使用 bounded retry，不使用無限輪詢或任意長 sleep。
4. 測試資料由 scenario 自己建立，使用 UUID／唯一 correlation 隔離，不依賴執行順序。
5. mutation 後驗證沒有非預期副作用；可透過正式 API 收斂的 OPEN 案件必須在 scenario 尾端結案。
6. 一個 scenario 保護一段完整業務敘事；同一個 error mapping 不跨多個 feature 重複驗證。
7. Fixture telemetry、Scenario Lab synthetic telemetry 與 live service telemetry 必須清楚區分。

## 目前覆蓋基準

目前基準為 19 個 feature、126 個 runtime scenarios；2026-08-29 的完整 suite 已通過 126/126。

| 能力面 | 主要 feature | 已覆蓋的高風險行為 |
|---|---|---|
| Session、RBAC、payload | `auth.feature`、`payload-security.feature`、`investigation-boundaries.feature` | 簽署 session、Viewer mutation deny、ADMIN-only payload、遞迴遮罩 |
| 案件 control plane | `investigation.feature`、`investigation-boundaries.feature`、`investigation-overview.feature` | ETag、audit、summary、evidence、複合篩選、stable keyset sort、cursor 綁定、狀態機、404、Overview delta |
| Event Check scope 與 legacy 搜尋 | `event-check.feature`、`timeline.feature`、`smart-search.feature`、`saved-search.feature` | 多 identifier bounded scope、Model selection、typed query ownership，以及 legacy adapter 不遺失條件 |
| Ingestion 問題 | `ingestion-issues.feature` | 三種安全 failure read model、filter、keyset cursor、7 天上限、無 raw/message/stack 欄位 |
| Legacy Journey／Pattern compatibility | `business-journey.feature`、`pattern.feature` | deprecated endpoint 的 milestone、registry、analysis、feedback 與 audit 不退化；不作 canonical authoring acceptance |
| Canonical Event Check | `event-check.feature` | 五種 identifier、qualifier、Model selection、custom scope、no-data／invalid bounds、Snapshot idempotency、Saved Results 查詢／cursor、Finding feedback、Case handoff、Viewer boundaries、原始 Check Model YAML checksum |
| 真實事件管線 | `event-pipeline.feature` | Order API → Outbox → Debezium → Kafka → raw landing／admission → Event Check |
| 擴充事件情境 | `event-scenarios.feature`、`scenario-lab.feature` | S1～S14 actual result、causation、sequence、retry、failure；S12～S14 必須走 live outbox path |
| Alert 與品質 | `grafana-alert-webhook.feature`、`grafana-auto-case.feature`、`quality-metrics.feature` | HMAC、dedup、resolved semantics、真實 Grafana 自動建案、quality aggregation |
| Observability | `observability-deep-links.feature`、`event-scenarios.feature` | Grafana asset UID、Tempo trace、Loki synthetic logs 與 trusted link contract |

Live service observability 不在 Karate 內重複啟動另一筆完整訂單；它由
`scripts/test-live-observability.sh` 負責跨 ClickHouse／Tempo／Loki 驗收。腳本必須確認：

- `OrderCreated`、`PaymentCompleted`、`ShipmentCreated` 在 ClickHouse 共用一個 trace ID。
- Tempo 的同一條 trace 包含 order／payment／shipping 三個 service resources。
- Loki 三個服務都有 canonical event、Kafka 與 trace fields。
- 三個 producer event 都有具名 `PREPARING`／`COMMITTED` lifecycle pair，且 log body 明確包含 event type。
- 未加 `--skip-restart` 時，重啟後仍能查回同一批 telemetry。

失敗 phase 與訊息格式由 Go unit test 保護；完整語意見
[Live Event Observability Contract](../contracts/live-event-observability-contract.md)。

正式 ClickHouse-first ingestion 的 infrastructure acceptance 由 `e2e/poc/clickhouse-mv-ingestion.feature`
驗證 store-all admission classification 與真實 technical DLQ projector；它因會操作基礎設施而不加入預設
19-feature HTTP suite。故障／恢復與有界 purge 分別由
`scripts/test-clickhouse-mv-functional-recovery.sh`、`scripts/test-clickhouse-mv-raw-purge.sh` 驗證，避免把
本機 Docker 中斷注入塞進純 HTTP feature。

Event Check 的 deterministic model regression 不透過 HTTP 重複排列所有 fixtures；
`backend/internal/contexts/eventcheck/domain/evaluator_contract_test.go` 直接讀取
`contracts/event-check/fixtures/check-model-scenarios.json`，目前 18/18 通過，包含 cross-correlation child
flow 與 partial-source `INCONCLUSIVE`。完全無法讀取 ClickHouse 的 hard outage 則由
`scripts/test-event-check-source-failure.sh` 驗證明確 503、無假業務結果與恢復後 hash 一致；Snapshot／Case
link 的 PostgreSQL 與 API restart 由 `scripts/test-event-check-restart-persistence.sh` 驗證。

## 2026-08-24 新增的案件 API 邊界案例

`investigation-boundaries.feature` 目前包含 11 個 runtime scenarios，其中案件查詢新增：

- 三筆唯一資料以 `updated_at asc, id asc` 跨兩頁時順序穩定、不遺漏也不重複，且 `next_cursor` 正確終止。
- Cursor 換排序方向重用回傳 `INVALID_CURSOR`；`page_size`、malformed cursor、無效 `sort_by`／
  `sort_order` 分別回傳穩定的 422 error code。
- `If-Match`、unknown field、非法狀態轉移、resolution 必填與 CLOSED immutability 組成完整狀態機敘事。
- Viewer 能讀案件，但 patch、note、event attachment、analyze 與 close 全部拒絕且資料未改變。
- 不存在案件在 detail、summary、evidence bundle、patch 與 analyze 維持一致 `404 NOT_FOUND`。

分頁與 Viewer scenario 建立的 OPEN 案件會在驗證後透過正式 close API 收斂，不增加 Overview 的
open count；完整 Exit Gate 另以 gate 起始時間與 `[E2E]` title marker 執行 isolated data cleanup，移除
control-plane、demo service 與 ClickHouse 測試資料，再還原互動式 fixtures。

## 2026-08-24 新增的真實 Grafana 自動建案案例

`grafana-auto-case.feature` 建立一筆唯一 terminal DLQ processing attempt，之後只輪詢 Event Hunter 公開 API；
它不自行組 webhook payload 或簽章。案例驗證 provisioned Grafana multi-dimensional rule 保留真實
`correlation_id`，經 Notification Policy 與 timestamped HMAC Contact Point 建立 HIGH 案件與 Alerting
Evidence。再加入較新的 `SUCCEEDED` attempt 後，同一 alert instance 必須 resolved、Evidence 增加為兩筆，
案件不得自動結案。兩段 retry 各自上限 60 秒，沒有無限輪詢。

## 2026-08-24 新增的歷史 Pattern 分析案例

`pattern.feature` 增加 2 個 EH-P1.1-011 runtime scenarios：

- 建立仍在 retention 內、但早於目前七天 wall-clock window 的 historical canonical events，驗證兩次分析
  使用完全相同的 server-owned effective window 並得到相同 finding。
- 對沒有 canonical event 的唯一 correlation 執行分析，驗證 API 明確回傳 `NO_EVENTS`、
  `effective_window=null` 與空 findings，不把「沒有資料」誤報成「已分析但未命中」。

案例使用 `2026-07-01` 事件時間，刻意落在目前 ClickHouse 90 天 retention 內；過期且已被 TTL 刪除的資料
不可能由分析器復原，該限制記錄於 `historical-pattern-analysis-contract.md`。

## 封版補充案例與生命週期

EH-P1.1-012 已於 2026-08-24 完成，完整 release gate 額外保護：

- restart persistence 與 ingestion dependency interruption／automatic recovery。
- `/api/` success 與 error response 的 JSON content type 一致性。
- gate-scoped E2E data cleanup；清理後 `[E2E]` 案件與本次 Scenario runs 皆為 0。
- `/investigations/{id}` route、reload、back／forward、close confirmation、dialog keyboard 與 390px layout
  留在 Frontend E2E，避免在 Backend feature 重複 UI 行為。

## 執行與報告

聚焦執行：

```bash
java -jar /tmp/event-hunter-karate-2.1.2.jar run \
  --no-pom --configdir=e2e \
  --output=artifacts/e2e/karate/backend-investigation-boundaries \
  e2e/backend/investigation-boundaries.feature
```

完整回歸：

```bash
bash scripts/test-backend-e2e.sh
```

報告固定輸出到 `artifacts/e2e/karate/`，不得放回 `e2e/backend/`。
