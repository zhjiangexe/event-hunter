---
document_id: EH-DOC-CON-003
status: active
owner: backend
last_reviewed: 2026-08-28
source_of_truth: true
canonical_topic: historical-pattern-analysis
supersedes: []
---

# Historical Pattern Analysis Contract

更新日期：2026-08-24  
狀態：EH-P1.1-011 completed

## 目的

案件的 Pattern 分析必須由事件證據決定，而不是由執行分析當下的 wall clock 決定。只要相同 canonical
events 仍在保存期間內，同一案件延後重跑就應取得相同的分析視窗與 Pattern 結果。

## Server-owned effective window

分析器使用案件的 primary correlation ID 查詢 ClickHouse 中仍被保留的 canonical events，依 Kafka
topic／partition／offset 去重後取得：

- `anchor`：最早事件的 `occurred_at`。
- `latest_event_at`：最晚事件的 `occurred_at`。
- `source_event_count`：去重後事件數。
- `from`：等於 `anchor`。
- `to`：等於 `anchor + P7D`，採 exclusive upper bound。
- `observed_at`：`min(analyzed_at, to)`，避免案件變舊後 maturity 持續漂移。

客戶端不可指定或擴張此視窗。Pattern 本身定義的 trigger-relative threshold（例如 `PT5M`）仍由 domain
evaluator 在 effective window 內判斷；七天是資料讀取安全邊界，不是每個 Pattern 的業務 timeout。

## 回應語意

### `EVALUATED`

至少存在一筆 retained canonical event，且事件跨度可由單一七天視窗完整涵蓋。API 回傳
`effective_window`、`source_event_count` 與 findings；findings 為空代表「有資料、已分析、未命中」。

### `NO_EVENTS`

找不到任何 retained canonical event。API 回傳：

```json
{
  "analysis_status": "NO_EVENTS",
  "effective_window": null,
  "findings": []
}
```

這代表「沒有可分析資料」，不得解讀成 Pattern 未命中。

### `ANALYSIS_WINDOW_EXCEEDS_LIMIT`

若 `latest_event_at >= anchor + P7D`，單一 bounded query 無法完整涵蓋證據。API 回傳 HTTP 422 與
`ANALYSIS_WINDOW_EXCEEDS_LIMIT`，不得截斷資料後回傳可能誤導的分析結果。

## 保存與重跑限制

確定性承諾只適用於仍在 ClickHouse retention 內的 canonical events。目前事件 TTL 為 90 天；資料被 TTL
刪除後，分析器無法從案件 metadata 重建原始證據，會誠實回傳 `NO_EVENTS`。若未來需要跨 retention 重跑，
必須另行設計 evidence snapshot／archive，不能偷偷放寬線上查詢視窗。

同一份 retained evidence 重跑時：

- effective `from`／`to`／`anchor` 不變。
- `observed_at` 在七天視窗成熟後固定為 `to`。
- finding identity 與 persistence 維持 idempotent。
- 每次分析仍寫入 audit metadata，包含 analysis status 與 effective window。

未指定 `pattern_ids` 時，由後端執行 immutable Registry 中所有 ACTIVE definitions；response 與 audit
metadata 都保存 `executed_pattern_ids`。這讓空 findings 仍可回答「哪些規則真的被評估」，也避免前端
固定綁定單一物流 Pattern。

## 驗收證據

- Go unit tests：historical rerun、no events、七天跨度邊界。
- ClickHouse adapter tests：correlation event window aggregate 與 empty result。
- Backend Karate：`e2e/backend/pattern.feature` 的 `@eh-p1-1-011` scenarios。
- Frontend tests／Karate：案件 Pattern 頁籤揭露 `EVALUATED`／`NO_EVENTS`、執行集合與 event-time window，
  reload 後由 Audit 還原最近結果。
- OpenAPI：`AnalysisResult` 與 `EffectiveAnalysisWindow` 是 generated client 的唯一契約來源。
