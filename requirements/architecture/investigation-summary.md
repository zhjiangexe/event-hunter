---
document_id: EH-DOC-ARCH-004
status: active
owner: backend
last_reviewed: 2026-08-29
source_of_truth: true
canonical_topic: investigation-summary-read-model
supersedes: []
---

# Investigation Summary Read Model

Investigation Case detail 由兩種 read model 組成：目前 canonical 的 Case + Check Snapshot links，以及
為既有案件保留的 legacy Summary／Timeline／Pattern projection。它們都不是新的 Domain Aggregate，也
不直接修改訂單、付款、庫存或事件資料。新的檢查結果必須由 Event Check 保存 immutable Snapshot 後
掛入 Case；不得再以 legacy Pattern analysis 當成唯一判定來源。

本專案採 Grafana OSS 優先的邊界：Logs／Metrics／Traces、Dashboard、Explore、Correlations、
技術告警與通知由 Grafana OSS 及其資料來源負責；Event Hunter 不複製這些原始資料，也不建立
第二套通用 Observability Console。Event Hunter 只保存業務調查所需的 reference、Grafana／Tempo／
Loki deep link、告警來源與案件結論。Grafana Alerting 只有在告警需要業務語意調查時，才透過
Webhook 交給 Event Hunter 建立或補充案件。

因此主要 UI 提供 Overview、Event Check／Saved Results、Check Models、Investigation Cases、Ingestion
Issues 與 Scenario Lab。案件可以讀取已連結的 Check Snapshots、Evidence references、Notes 與 Audit；
品質指標仍由 Grafana 呈現，不提供第二套 Runtime Quality Console。Schema／Topic 管理與
Replay／Production Redrive 不屬於目前 UI 或 Phase 1.1 API。

正式 endpoint、request／response schema 以 [openapi.yaml](../../openapi.yaml) 為準；本文件補充 Query
Service、來源失敗與組合行為。

## 1. 使用情境

使用者在 Investigation Console 輸入案件 ID，希望一次回答：

```text
這個案件目前是什麼狀態？
已連結的 Check Snapshot 當時看到了哪些事件與結果？
是否有 duplicate、missing sequence 或 lag？
哪些 Trace／Log 可以證明問題？
哪一個 Check Model／Finding 支持這個判斷？
結果是否完整，還有哪些資料查不到？
```

摘要只回傳必要欄位與 Evidence reference；完整事件 payload、Log 與原始 Trace 仍保留在各自的
資料系統。

## 2. Context 與查詢責任

`investigation` Context 擁有摘要的查詢用例與輸出 DTO，但不擁有所有資料來源：

```text
InvestigationCaseReadServices
    ├── CaseQuery                  → PostgreSQL
    ├── CheckSnapshotQuery         → PostgreSQL immutable snapshots / findings / links
    ├── TimelineQuery              → ClickHouse canonical_forensics_events
    ├── QualityQuery               → ClickHouse event_quality_metrics
    ├── TechnicalObservationQuery  → Grafana OSS data sources（Tempo／Loki／Prometheus／ClickHouse）
    ├── AlertReferenceQuery        → Grafana Alerting annotations／Webhook reference
    └── LegacyPatternQuery         → compatibility findings / evidence references
```

每個 Query Port 由對應的 Outbound Adapter 實作；不做跨 PostgreSQL／ClickHouse 的 SQL JOIN，
也不讓 Huma Handler 直接取得資料庫連線。

## 3. 建議 API

### 3.1 Legacy 即時摘要（compatibility）

```http
GET /api/v1/investigations/{investigationId}/summary?from=2026-08-20T00:00:00Z&to=2026-08-20T01:00:00Z
```

用途：查詢已存在案件的既有 summary projection。API 只做有界、唯讀查詢，不啟動 Replay，也不修改
案件狀態。新的 deterministic result 另由 `GET /api/v1/investigations/{id}/check-snapshots` 取得。

初步參數：

| 參數 | 必填 | 說明 |
|---|---|---|
| `investigationId` | 是 | PostgreSQL 案件 UUID |
| `from` | 否 | 事件查詢起始時間（含）；未提供時使用最近 24 小時 |
| `to` | 否 | 事件查詢結束時間（不含）；未提供時使用目前時間 |
| `include_technical` | 否 | 是否查詢 Trace／Log，預設 `true` |
| `include_payload` | 否 | 是否回傳遮罩後 payload，預設 `false` |
| `limit` | 否 | Timeline 最大事件數，預設 1,000，上限 10,000 |

伺服器套用有界時間窗；最大 7 天。超過範圍回傳 `422 QUERY_WINDOW_TOO_LARGE`，MVP 不會讓 HTTP
request 長時間等待，也不會自動建立 Temporal Workflow。

### 3.2 非同步報告（Phase 2 以後）

若未來摘要需要跨多天、執行 Projection Rebuild、產生大量 Evidence 或等待外部資料，可由 Temporal
管理。這裡的 retry 是查詢、Evidence 產生或隔離分析等 Temporal Activity 的流程重試；Kafka
Consumer 的事件重試仍由 Consumer Framework／Retry Topic／DLQ 與其 Log／Trace 遙測負責：

```text
POST /api/v1/investigations/{investigationId}/summary-jobs
    → 202 Accepted + workflow_id
    → GET /api/v1/investigations/{investigationId}/summary-jobs/{jobId}
```

MVP 先實作即時摘要與固定 query budget；非同步報告先保留方向，不先加入正式 API。

## 4. 回應模型

```json
{
  "investigation_id": "6f8a2e6f-7d55-4a6a-9e78-2ed8c2865f90",
  "generated_at": "2026-08-20T01:02:03Z",
  "query_window": {
    "from": "2026-08-20T00:00:00Z",
    "to": "2026-08-20T01:00:00Z"
  },
  "partial": false,
  "warnings": [],
  "source_status": {
    "postgres": "OK",
    "clickhouse": "OK",
    "technical_observability": "OK"
  },
  "case": {},
  "timeline": {},
  "quality": {},
  "pattern_findings": [],
  "technical_observations": {},
  "evidence_references": []
}
```

以上是 legacy Summary response；`pattern_findings` 不代表新的 Check Model 結果。Canonical Check Snapshot
以 `check_status`、`business_outcome`、Expectations、`check_findings`、event refs 與 relations 表達。

欄位規則：

| 欄位 | 說明 |
|---|---|
| `generated_at` | 此次摘要產生時間，不代表所有來源同一時間完成寫入 |
| `query_window` | 所有事件與品質查詢使用的時間範圍 |
| `partial` | 是否有非必要來源失敗或資料不完整 |
| `warnings` | 缺失、延遲、截斷或部分失敗原因 |
| `source_status` | 每個資料來源的成功、失敗或超時狀態；Grafana 技術查詢也要單獨標示 |
| `case` | PostgreSQL 案件基本資料與目前狀態 |
| `timeline` | ClickHouse 事件時間線與 `truncated` 資訊 |
| `quality` | duplicate、lag、schema violation 等品質摘要 |
| `pattern_findings` | Deprecated Pattern compatibility 結果；新結果讀 Check Snapshot |
| `technical_observations` | Trace／Log 摘要與 deep link，不內嵌全部內容 |
| `evidence_references` | 指向事件、Trace、Log、品質違規或報告的參照 |

## 5. 資料來源與失敗行為

| 摘要區塊 | 來源 | MVP 是否必要 | 查詢失敗時的行為 |
|---|---|---|---|
| `case` | PostgreSQL `investigation_cases` | 是 | `503` 或 `404`，不能產生沒有案件的摘要 |
| `timeline` | ClickHouse `canonical_forensics_events` | 是 | `503`／`504`，不能產生空白假摘要 |
| `quality` | ClickHouse `event_quality_metrics` | 否 | `partial=true`，加入 warning |
| `pattern_findings` | Legacy Pattern Engine | 否（compatibility） | `partial=true`，標記 legacy analysis 未完成 |
| `technical_observations` | Grafana OSS data sources（Tempo／Loki／Prometheus／ClickHouse） | 否 | `partial=true`，保留 unavailable warning |
| `evidence_references` | `case_evidence` + 本次收集結果 | 是 | 不能靜默遺失；回傳警告並寫入稽核 |

必要來源失敗時不要回傳看似完整的空結果；非必要來源失敗可以回傳部分結果，但必須公開
`partial`、`warnings` 與 `source_status`。

## 6. 查詢組合流程

```text
1. 讀取 InvestigationCase，確認案件存在與使用者權限
2. 驗證 from／to／limit 與 query budget
3. 並行查詢案件、事件、品質與技術觀測資料
4. 對事件依 occurred_at、sequence、event_id 排序
5. 將事件與 Trace／品質資料轉為 Evidence DTO
6. 讀取已連結 Check Snapshots；legacy endpoint 需要時再讀 Pattern findings
7. 彙整 warnings、source_status、truncated 與 partial
8. 回傳 InvestigationSummary DTO
```

Go Application Layer 可以使用 bounded `errgroup` 平行執行互不依賴的 Query Port；每個 Port 必須
接收同一個 `context.Context`，由上層統一設定 timeout 與取消。不要用無界 goroutine、channel
queue 或背景工作把 HTTP request 脫離追蹤。

## 7. 一致性與時間語意

摘要不是跨 PostgreSQL、ClickHouse、Tempo、Loki 的分散式交易，因此不宣稱所有來源同一個
snapshot。回應必須保留：

- `generated_at`：摘要服務完成組合的時間。
- `occurred_at`：業務事件發生時間。
- `ingested_at`：事件進入 ClickHouse 的時間。
- `source_status`：每個來源的實際查詢結果。
- `warnings`：延遲、缺失、截斷或部分失敗。

案件狀態以 PostgreSQL 為準；業務事件以 ClickHouse append-only 資料為準；技術觀測以 Grafana OSS
所查詢的 Tempo／Loki／Prometheus 結果為準。不同來源的最新時間可能不同，不能用單一 `updated_at`
假裝全域一致。

## 8. Check Snapshot 與 legacy Pattern 整合

Canonical flow 是 Event Check evaluation → explicit save → Check Snapshot → Case link。Legacy Pattern
Engine 仍不得直接向外部資料庫任意查詢，只能使用預先定義的 Query Port 與模板：

```text
EvidenceSnapshot
    → PatternRegistry
    → PatternDefinition.Evaluate
    → PatternFinding
```

Pattern Finding 至少包含：

```text
pattern_id
matched_conditions
severity
evidence_references
recommended_next_query
query_template_id
```

Legacy Pattern 執行只產生 compatibility 結果，不修改案件、正式業務資料或環境設定。Application Service 以
`idempotency_key` 寫入 PostgreSQL `pattern_findings`；需要案件證據索引時再寫入 `case_evidence`，
並將操作追加到 `audit_logs`。

## 9. Evidence 與追溯

Evidence 不把完整 payload 複製到摘要或案件表，而是保存：

```text
evidence_type
reference
event_id／trace_id／log_id／alert_id
query_template_id
collected_at
checksum（Evidence manifest 必填；資料表索引可為 NULL）
pii_masking_policy
```

完整事件仍在 ClickHouse；完整 Trace／Log／Metrics 仍在 Grafana OSS 連接的技術觀測平台。使用者
從摘要的 reference 進入 Grafana／Tempo／Loki deep link、Dashboard annotation 或受控查詢，Pattern 結果才能被重新驗證，
也避免 PostgreSQL 變成 PII payload 倉庫。

## 10. 查詢限制與安全

- 所有 ClickHouse 查詢必須有時間範圍、allowlist 欄位與最大回傳筆數。
- 不接受使用者傳入任意 SQL、ClickHouse function 或排序欄位。
- `include_payload=true` 需要額外權限，且只回傳遮罩後內容。
- 技術觀測 deep link 不應洩露未授權的服務、租戶或 Log query。
- 每個來源設定獨立 timeout；整體摘要設定更短的 deadline。
- 查詢執行資訊記錄到 `audit_logs`，但不記錄完整事件 payload。
- API 回應不得把 ClickHouse、Tempo 或 Loki 的 credential／內部錯誤直接傳給使用者。

## 11. MVP 儲存策略

目前不新增 `investigation_summaries` 表。Legacy Summary 每次由 Query Service 即時組合；canonical
可重現結果已由 `check_snapshots`、event refs、relations、`check_findings` 與 Case link tables 保存。

只有在確認查詢成本或歷史報告需求後，才評估新增 immutable snapshot：

```text
investigation_summary_snapshots
├── id
├── investigation_case_id
├── query_window
├── generated_at
├── schema_version
├── summary_json（已遮罩）
└── checksum
```

Snapshot 是報告版本，不取代 PostgreSQL 案件狀態，也不取代 ClickHouse 原始事件。

## 12. 測試案例

至少涵蓋：

- 案件、事件、品質與 Trace 全部成功，回傳 `partial=false`。
- Tempo／Loki timeout，仍回傳事件與案件，但 `partial=true` 並含 warning。
- ClickHouse timeout，回傳 `503`／`504`，不能產生空白假摘要。
- 時間線超過 `limit`，回傳 `truncated=true`。
- duplicate、missing sequence、付款後缺少出貨事件能產生正確 Pattern Finding。
- `include_payload=false` 時省略 payload；有權限時也必須套用遮罩。
- 同一案件同時被更新時，摘要讀取不繞過 `lock_version` 規則。
- 相同 query 重試不重複寫入 `case_evidence` 或 `audit_logs`。
- 結果包含 `event_id`、`trace_id` 與 query template，能重新定位原始證據。

## 13. 與未來 LLM 的邊界

未來若加入 Python AI Analysis Service，LLM 只接收已遮罩、已整理的 `Evidence Bundle` 或摘要，
回傳結構化 finding／建議；它不能直接連 PostgreSQL、ClickHouse、Tempo、Loki 或正式業務系統。

LLM 的輸出不能取代 Pattern Finding；固定規則仍是可稽核的基準，LLM 只提供補充摘要或候選解釋。
