---
document_id: EH-DOC-CON-008
status: superseded
owner: product
last_reviewed: 2026-08-29
source_of_truth: false
canonical_topic: timeline-evidence-attachment
supersedes: []
---

# Timeline Event Evidence Attachment Contract

> **Superseded compatibility contract.** Canonical handoff 是 Event Check 明確保存 immutable Check Snapshot，
> 再建立或加入 Investigation Case。本文件僅保留 legacy Timeline 單事件 attachment endpoint 的既有契約；
> 新流程以 [Event Check Target Design](../architecture/event-check-target-design.md) 為準。

更新日期：2026-08-23  
需求：`REQ-EH-015`  
工程工作包：`EH-P1.1-006`／`P1.1-02-02`

## 目的與邊界

Investigator／Admin 可在 Timeline event detail 選擇一個未結案案件，將該 canonical event 保存為
案件 Evidence reference。Event Hunter 不把 event payload 複製進 PostgreSQL；ClickHouse 仍是 event
內容的權威來源，案件只保存穩定 reference、完整性 checksum 與收集時間。Viewer 只能查看。

這不是 Kafka replay、事件修改或通用檔案附件功能，也不會因為加入案件而改寫正式業務事件。

## Command contract

`POST /api/v1/investigations/{investigationId}/evidence/events` 必須包含：

- `If-Match: "v{lock_version}"`；缺少或過期分別回 `428`／`409`。
- `event_id`：trim 後 1～200 字元。
- `from`、`to`：原 Timeline 查詢的 RFC 3339 bounded window；`to` 必須晚於 `from`，最長七天。

Application service 先讀取案件並檢查版本，再以 `event_id` 與 bounded window 到 ClickHouse 精確查找。
找不到 event 回 `404 EVENT_NOT_FOUND`；依賴失敗不得偽裝成找不到資料。

## Aggregate 與 persistence invariants

- 只有未結案案件可附加 evidence。
- Evidence type 固定為 `EVENT`，reference 固定為已驗證的 canonical `event_id`。
- Checksum 使用小寫 SHA-256；PostgreSQL 不保存 event payload。
- 若 event correlation 不等於案件 primary correlation，aggregate 將它加入 bounded、去重且排序的
  `related_correlation_ids`。
- 同一案件重送同一 EVENT reference 為 idempotent no-op：回 `attached=false`，不增加 `lock_version`，
  不新增 evidence 或 audit。
- 新 reference 的 `case_evidence` insert、related correlation update 與 `lock_version + 1` 必須在同一個
  PostgreSQL transaction 內完成。
- 成功新增後寫入 `ATTACH_INVESTIGATION_EVENT` audit，記錄 event ID、event correlation、evidence ID
  與完成後 lock version，但不記錄 payload。

## UI 行為

- 「加入案件」只在事件 detail 展開且角色可寫入時出現。
- 候選案件清單只在 modal 開啟後載入，排除 CLOSED，相同 correlation 的案件優先。
- 成功後精確更新 React Query 中的案件 detail／list cache，並使 Evidence／Summary cache 失效。
- UI 明示只保存 reference，不讓使用者誤以為 payload 已封存到案件。

## 驗收

- Backend：`e2e/backend/investigation.feature` 覆蓋新增、重送、stale ETag、Evidence、Audit、找不到
  event 與 Viewer forbidden。
- Browser：`e2e/frontend/investigation-flow.feature` 從 Timeline 展開 event、選案件、加入後由 API
  回查 Evidence 與 Audit。
- Restart gate：重啟 PostgreSQL 與 Event Hunter API 後，同一案件仍可查回 EVENT reference、
  checksum、related correlation、audit 與相同 `lock_version`。
