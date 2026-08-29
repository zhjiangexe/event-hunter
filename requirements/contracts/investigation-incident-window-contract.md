---
document_id: EH-DOC-CON-004
status: active
owner: backend
last_reviewed: 2026-08-29
source_of_truth: true
canonical_topic: investigation-incident-window
supersedes: []
---

# Investigation Incident Window 與 Partial Summary Contract

## 目的

案件必須保存「問題發生時正在調查的事件時間窗」，不能在每次重新開啟時改用 rolling now。否則隔日、
重啟或事件超出 retention 後，使用者會把「查詢窗口漂移」誤判成證據消失。

## Aggregate invariants

`InvestigationCase` 擁有不可變的 `IncidentWindow`：

| 欄位 | 規則 |
|---|---|
| `incident_from` | RFC 3339 timestamp，必須早於 `incident_to` |
| `incident_to` | RFC 3339 timestamp |
| `incident_window_source` | 只能是下表四種來源 |
| Window 長度 | 大於 0，且不得超過 7 天 |

案件的狀態、owner、priority、notes 或 lock version 改變時，不得改寫 Incident Window。API 的臨時
`from`／`to` query 也只影響該次 read，不是 mutation。

## 來源決策

| 來源 | 建立方式 | Window 決策 |
|---|---|---|
| `TIMELINE_SEARCH` | Event Check 建案或 legacy Timeline API 明確提供一對 `incident_from`／`incident_to`；enum 名稱為資料相容性保留 | 原樣保存已驗證的 bounded window；canonical Case current-view 可在 Snapshot `to` 後加 1 秒查詢邊界，但不得改寫 Snapshot |
| `MANUAL_DEFAULT` | 手動 API 未提供時間 | 只在建立瞬間計算 `created_at - 72h` 到 `created_at` |
| `GRAFANA_ALERT` | Grafana firing alert 自動建案 | `startsAt - incident_window_seconds` 到 `startsAt`；規則未設定時為 600 秒 |
| `LEGACY_CREATED_AT` | migration 回填舊案件 | `created_at - 24h` 到 `created_at` |

只提供 `incident_from` 或 `incident_to`、反向窗口、零長度或超過 7 天，都回傳
`422 INVALID_INCIDENT_WINDOW`。

## Read semantics

- `GET /investigations/{id}/summary` 未帶 query window 時使用案件 Incident Window。
- `GET /investigations/{id}/evidence` 未帶 query window 時使用案件 Incident Window。
- 明確傳入成對 `from`／`to` 可建立 current-view override；response 的 `query_window` 回報實際窗口，
  但案件內的 baseline 不變。
- 案件 UI 必須同時標示 baseline 與 current view，並可用 baseline deep link 回 Event Check Timeline。
- Legacy Pattern Analysis 保留 `EH-P1.1-011` 的 `EARLIEST_CORRELATION_EVENT` deterministic window；
  `P1.1-UX-03` 已在案件 workspace 揭露該窗口與執行結果。它是 Pattern 的證據安全邊界，不等同也不會
  改寫案件 Incident Window。
- Evidence 是 append-only durable reference。由 Pattern Analysis 保存的 Event／Trace／Finding 可能來自
  analysis effective window，而不在案件 Incident Window 內；Evidence 數量因此不能解讀為案件 Timeline
  的 event count。當最近 analysis effective window 與 baseline 不同且 Timeline 為空時，UI 必須說明差異，
  並提供帶 correlation ID 與 analysis window 的 Event Check deep link。

## Partial Summary semantics

PostgreSQL 是案件、Finding、Evidence 與 Audit 的 authoritative source；ClickHouse 是 canonical event
Timeline source。ClickHouse timeout 或 unavailable 時：

- HTTP 仍回 `200`，`partial=true`。
- `source_status.postgres=OK`；ClickHouse 為 `UNAVAILABLE` 或 `TIMEOUT`。
- PostgreSQL sections 照常回傳，event-derived Timeline 為不可用，不得顯示成可信的 0 events。
- `source_last_success_at` 表示本次 response 內各來源最後成功完成讀取的時間；不是跨 request 保存的
  historical health。失敗的 ClickHouse query 為 `null`。
- `event_retention_boundary` 是依目前 90 天 TTL 推導的近似邊界；ClickHouse 非同步 TTL 可能讓實際
  清理時間略晚於此值。
- `timeline.truncated=true` 表示達到 query limit，不代表資料來源失敗。

## Persistence 與驗收

- Migration `00008_investigation_incident_window.sql` 必須可重跑，且所有 legacy rows 都完成 deterministic backfill。
- `scripts/test-investigation-partial-summary.sh` 以真實 ClickHouse outage 驗證 partial response 與恢復。
- `scripts/test-investigation-incident-window-restart.sh` 驗證 PostgreSQL／API restart 前後 baseline、source、
  Summary query window 與事件數一致。
- Backend／Frontend Karate 必須覆蓋建立、override 不變更 baseline、Grafana source 與 UI baseline/current
  window 區分。
- 使用歷史 fixture 的 browser E2E 建案時必須明確提供 fixture 的 incident window，不得依賴
  `MANUAL_DEFAULT` rolling window；另以 component test 覆蓋 legacy／既存不一致案件的 Evidence window
  說明與 analysis-window deep link。
