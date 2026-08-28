# Investigation List Query Contract

本契約定義 `GET /api/v1/investigations` 的複合篩選、排序、分頁與可分享 URL 語意，對應
`REQ-EH-002`、`REQ-EH-014` 與 `P1.1-02-03`。

## 查詢邊界

- 篩選條件可同時套用：`status`、`severity`、`priority`、`assignee`、`tag`、`correlation_id`。
- 所有篩選與排序都由 PostgreSQL read adapter 執行；前端不得只排序目前已載入的一頁。
- `sort_by` 僅允許 `created_at`、`updated_at`，預設 `created_at`。
- `sort_order` 僅允許 `asc`、`desc`，預設 `desc`。
- 所有排序都以相同方向的 `id` 作為 deterministic tie-breaker，確保相同時間戳仍有穩定順序。
- `page_size` 維持 1～200 的既有邊界；無效排序回傳 `422 INVALID_SORT`。

## Cursor 語意

- Cursor 是 opaque、versioned keyset token，不是 offset，也不是前端可解析的產品資料。
- v2 cursor 保存排序欄位、方向、邊界時間與案件 ID。
- Cursor 只能在相同 `sort_by`／`sort_order` 下續頁；換排序條件重用會回傳
  `422 INVALID_CURSOR`。
- 篩選條件或排序條件改變時，client 必須回到第一頁並清除既有 cursor chain。

## URL 與 UI

- `/investigations` 以 query string 保存所有 typed filter 與 sort state，因此網址可複製、reload、
  browser back／forward。
- 開啟或關閉案件 drawer 時必須保留原 query string。
- UI 僅送出 allowlisted typed values；未知 enum 不得成為前端的隱性篩選狀態。

## 驗收

- `e2e/backend/investigation.feature`：驗證後端同時套用完整複合條件。
- `e2e/backend/investigation-boundaries.feature`：驗證 asc keyset 分頁、穩定順序、cursor 排序綁定與
  無效排序錯誤。
- `e2e/frontend/investigation-flow.feature`：驗證可分享 URL 能還原表單並取得正確案件，修改排序後
  URL 與結果同步。

