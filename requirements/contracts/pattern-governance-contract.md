---
document_id: EH-DOC-CON-007
status: active
owner: product
last_reviewed: 2026-08-28
source_of_truth: true
canonical_topic: pattern-governance
supersedes: []
---

# Pattern Governance 與 Finding Feedback Contract

更新日期：2026-08-24  
對應：`REQ-EH-003`、`P1.1-04-02`

## 邊界

- Pattern 定義只存在於 `contracts/patterns/*.yaml`，由 generator 產生 immutable Go Registry；API 不提供新增、修改或刪除 Pattern。
- Registry 公開專案相對 `source_path`、原始 YAML bytes 的 SHA-256 `checksum`，以及 YAML 所列 fixtures 的 match／non-match／total 數量。
- checksum 與 fixture coverage 是部署內容的可追溯資訊，不是 runtime effectiveness，也不能由前端寫入。

## Finding 與人工判定

- `pattern_findings` 是 deterministic analysis 的 append-only 結果；人工判定不得改寫 severity、conditions、evidence 或 Pattern definition。
- 人工判定保存於獨立的 `pattern_finding_feedback` current-state table。
- 尚未建立 feedback row 時，API 回傳虛擬狀態 `UNREVIEWED`、`lock_version=0`；可寫狀態只有 `CONFIRMED`、`FALSE_POSITIVE`、`NEEDS_REVIEW`。
- 本階段不加入 reviewer、approval workflow 或受限狀態轉移；Investigator／Admin 可直接在三個可寫狀態間重新判定，Viewer 唯讀。
- `PATCH /api/v1/investigations/{investigationId}/findings/{findingId}/feedback` 必須使用該 feedback 自己的 `If-Match: "vN"`，不能借用案件版本。
- stale version 回 `409 OPTIMISTIC_LOCK_CONFLICT`；finding 不屬於指定案件或不存在時回 404；未知狀態回 422。
- feedback 更新與 `CLASSIFY_PATTERN_FINDING` audit 必須在同一個 PostgreSQL transaction；audit 保存 finding ID、狀態與新 feedback version。

## 案件內執行語意

- `POST /investigations/{id}/analyze` 未提供 `pattern_ids` 時，後端從 immutable Registry 解析並執行所有
  `ACTIVE` Pattern；前端不得自行補入某個固定 Pattern ID。
- 進階操作可明確提供一個或多個 Registry IDs；未知或 inactive ID 回 `422 UNKNOWN_PATTERN`。
- Response 的 `executed_pattern_ids` 記錄實際評估集合，即使 `findings=[]` 也不得省略；相同集合也保存到
  `ANALYZE_INVESTIGATION` audit metadata，讓案件重開後仍能說明最近一次分析。
- `NO_EVENTS` 表示無 canonical source event，不能顯示成「已評估且未命中」。`EVALUATED` 加空 findings
  才是 no-match；非空 findings 才是 match。
- Writable 且未結案案件可由案件 drawer 執行或重跑；Viewer 與 CLOSED 案件只顯示歷史結果。
- 成功後同一 workspace 必須刷新 Summary、Findings、Evidence 與 Audit；Timeline 建案後應交接到這個
  canonical routeable workspace，不再維護另一套完整 Pattern/Evidence 操作區。

## 驗證

- generator drift、checksum 與 fixture coverage 由 unit／contract checks 保護。
- Backend Karate 驗證初始狀態、成功判定、Viewer deny、stale version、summary 與 audit。
- Frontend component／browser tests 驗證 Pattern Library metadata，以及案件抽屜的人工判定操作。
- API restart 後必須仍可從 detail／summary API 查回 feedback 與 audit。
