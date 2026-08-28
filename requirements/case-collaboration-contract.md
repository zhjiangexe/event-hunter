# Investigation Case Collaboration Contract

更新日期：2026-08-23  
需求：`REQ-EH-014`  
工程工作包：`EH-P1.1-005`／`P1.1-02-01`

## 目的與邊界

案件協作最小模型讓 Investigator／Admin 可以交接 owner、設定處理優先級、加上分類 tags、關聯其他
correlation ID，並以不可覆寫的 note 留下調查紀錄。Viewer 只能讀取。這不是完整 Incident
Management：不提供 on-call directory、通知、escalation policy、審批或正式 OIDC identity。

## Aggregate invariants

- Owner 沿用既有 `assignee`；空字串代表未指派，非空值 trim 後最長 200 字元。
- Priority 僅允許 `P0`、`P1`、`P2`、`P3`。未明示時依 severity 對應：
  `CRITICAL→P0`、`HIGH→P1`、`MEDIUM→P2`、其他→`P3`。
- Tags 會 trim、轉小寫、去重並排序；最多 10 個，每個最長 50 字元。
- Related correlation IDs 會 trim、去重並排序；最多 20 個，每個最長 200 字元。
- Metadata update 與 note append 都必須攜帶 `If-Match: "v{lock_version}"`；版本不一致回 `409`。
- 已結案案件不可再修改 metadata 或新增 note。
- `last_updated_by` 取目前 session subject；Phase 1.1 不把可偽造的 request body actor 當身分來源。

## SLA policy

SLA 不另存可變欄位，而是由 priority、`created_at` 與案件狀態即時計算：

| Priority | Due window |
|---|---:|
| P0 | 1 小時 |
| P1 | 4 小時 |
| P2 | 24 小時 |
| P3 | 72 小時 |

- 已結案：`COMPLETED`。
- 現在時間大於等於 due time：`BREACHED`。
- 剩餘時間小於等於 1 小時：`DUE_SOON`。
- 其他：`ON_TRACK`。

API 回傳 `sla_due_at` 與 `sla_status`；client 不自行推導。Priority 改變時 due time 會依原始
`created_at` 重新計算，確保同一案件在所有讀取端一致。

## Append-only notes

`POST /api/v1/investigations/{investigationId}/notes` 新增 note。每筆 note 保存獨立 UUID、body、
session subject、role 與 database timestamp。Body trim 後不得為空，最長 2,000 字元。案件 PATCH
不接受 notes 欄位，也沒有 update／delete note endpoint，因此舊 note 不能被覆寫；每次 append 同時
提升案件 `lock_version` 並新增 audit entry。

## Persistence 與驗收

- Mutable metadata：`postgres.investigation_cases`。
- Immutable collaboration history：`postgres.case_notes`、`postgres.audit_logs`。
- Migration：`backend/migrations/postgres/00005_case_collaboration.sql`。
- Contract：`openapi.yaml` 的 `updateInvestigation`、`addInvestigationNote` 與 Investigation schemas。
- Backend acceptance：`e2e/backend/investigation.feature`。
- Browser acceptance：`e2e/frontend/investigation-flow.feature`。
- Restart gate：重啟 PostgreSQL 與 Event Hunter API 後，必須能由 GET API 查回相同 metadata、notes
  與 lock version。

## 相鄰切片

Timeline 選取事件加入既有案件已由 `REQ-EH-015`／`P1.1-02-02` 獨立定義，不擴張本協作契約。
Production OIDC、user/team directory、通知與 escalation 屬後續 production identity／incident
integration 工作，不回填到本最小模型。
