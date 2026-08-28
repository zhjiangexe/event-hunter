---
document_id: EH-ADR-006
status: adopted
owner: platform
last_reviewed: 2026-08-29
source_of_truth: true
canonical_topic: check-snapshot-and-finding-persistence
supersedes: []
---

# ADR-006：Immutable Check Snapshot 與通用 Finding persistence

## 狀態

`adopted`。`EH-ECM-003` 已以 `backend/migrations/postgres/00009_check_snapshots.sql` 建立本 ADR 的 Snapshot、
event reference、relation、Finding、feedback 與 Case link tables。Legacy `pattern_findings`、
`pattern_finding_feedback` 與 `case_evidence` 在相容遷移完成前仍保留。
`EH-ECM-004` 已在 Snapshot read response 提供目前 `finding_feedback` projection；projection 是可變的人工判定
狀態，不回寫 immutable result，也不參與 evaluation hash。

## 背景

Event Check evaluation 預設不保存，但使用者可以明確保存結果或把結果加入 Investigation Case。保存內容必須
固定當時的 Model version、事件集合、`as_of`、source health 與 deterministic result。若 evaluation 完成後有
late event 到達，保存操作不能在使用者不知情時改成另一個結果。

現有 `pattern_findings` 直接 FK 到 Investigation Case，只能表達 Case 內 Pattern 分析；它無法代表尚未建案的
Check Snapshot、Flow Expectation 或 Global Check，也會迫使新功能再建立另一套 Finding／Feedback。

## 決策

### 1. Evaluation ephemeral，Snapshot immutable

`POST /api/v1/event-checks/evaluations` 執行 stateless evaluation，不建立 PostgreSQL row。回應包含：

- normalized request；
- Model reference 與 checksum；
- `event_set_hash`；
- `evaluation_hash`；
- Scope、result 與 source health。

`POST /api/v1/check-snapshots` 帶回原 evaluation request、預期 hashes 與 `Idempotency-Key`。伺服器使用相同
`as_of` 與 Model version 重新解析／評估：

- hashes 相同：transactionally 保存 Snapshot；
- event set 或 result 已改變：回 `409 EVALUATION_CHANGED` 與新的 hashes，不保存；
- Model version 不再可執行：回 `409 MODEL_VERSION_UNAVAILABLE`，不偷偷套用新版；
- 相同 actor 與 Idempotency-Key 重送：回原 Snapshot，不重複寫入。

因此不需要暫存每一次 evaluation，也不接受前端自行提交結果 JSON 作為正式 Snapshot。

### 2. Hashing contract

所有 hash 使用小寫 SHA-256。ECM-001 必須定義 RFC 8785 JCS canonical document：

```text
event_set_hash = SHA256(JCS({
  scope_contract_version,
  included_event_metadata_and_payload_checksums,
  excluded_event_metadata_and_reasons,
  relationship_edges
}))

evaluation_hash = SHA256(JCS({
  evaluation_contract_version,
  normalized_request,
  model_ref_and_checksum,
  event_set_hash,
  source_health,
  deterministic_result
}))
```

Hash 是 consistency 與 integrity evidence，不是 authentication signature。

### 3. Target PostgreSQL model

PostgreSQL 保存控制面與人員調查資料；Check Model 仍由 generated immutable Registry 提供。

```text
check_snapshots
├── check_snapshot_event_refs
├── check_snapshot_relations
└── check_findings
      └── check_finding_feedback

investigation_cases
└── investigation_check_snapshots ── check_snapshots
```

`check_snapshots`、event refs、relations 與 findings append-only；repository 不提供 update/delete。Feedback 是
獨立 mutable current state，使用 optimistic lock，所有 mutation 與 Audit 同 transaction。

Snapshot 可以先獨立保存，再以 N:M link 加入一個或多個 Cases。Link 操作使用 Case `If-Match`、推進
`lock_version` 並新增 Audit；不複製 Snapshot 或 findings。

### 4. Minimal event metadata survives ClickHouse retention

每個 Snapshot event reference 保存：event ID、event type、occurred time、producer、aggregate type／ID、
correlation ID、trace ID、payload checksum、included／excluded、ordinal。Relation edge 另存 from/to event、
relation type、source field 與 source Model rule。

不保存完整 payload。若 ClickHouse event 已過期，Snapshot 仍可說明當時使用哪些事件與 hash，但 UI 必須
標示 source expired；不能把 metadata 摘要偽裝成仍可展開的 canonical event。

### 5. Pattern Finding 泛化為 Check Finding

目標模型將 `pattern_findings`／`pattern_finding_feedback` 泛化為 `check_findings`／
`check_finding_feedback`，保留既有 finding UUID、feedback、Case evidence reference 與 audit 可讀性。

Target `check_findings` 至少保存：Snapshot ID、rule kind、rule ID／version／checksum、severity、code、
Expectation state、matched conditions、Evidence refs、recommended next query 與 created time。

即時 evaluation 的 Finding 尚未持久化，因此 API `id=null`；保存 Snapshot 時配置 UUID。Finding UUID 是
storage identity，不屬於 deterministic evaluation input，也不納入 `evaluation_hash`。

遷移採 expand／backfill／compatibility／contract：

1. 建立 Snapshot、event ref、relation 與 Case link tables；
2. 擴充／泛化 Finding schema，保留 UUID；
3. 為 legacy rows 建立 `LEGACY_CASE_ANALYSIS` synthetic Snapshot 並連回原 Case；
4. feedback FK 與既有 API 透過 compatibility adapter 保持可讀寫；
5. 新 APIs 與 E2E 通過後才移除 legacy direct Case FK／舊 column names。

Legacy row 無法可靠還原不存在的 event metadata 或完整 analysis run grouping；不得猜測。Synthetic Snapshot
必須標示 `provenance=LEGACY_PATTERN_MIGRATION` 與 partial warnings。

### 6. Retention hook without DuckDB dependency

Snapshot 可以預留 nullable Retention Profile reference，但 ECM-001～006 不依賴 Evidence Archive。PostgreSQL
Snapshot metadata、Case links 與 Findings 是正式資料；後續 archive lifecycle 由獨立 catalog 管理。

## 未採用方案

### 每次 evaluation 都保存

不採用。它會把一般查詢變成大量控制面寫入，增加 retention、隱私與清理負擔，且違反使用者選定的
「明確保存才建立 Snapshot」。

### 保存時直接接受前端 result JSON

不採用。前端資料可被修改，也可能已落後 late events；正式 Snapshot 必須由後端重算並核對 hashes。

### Findings 只放在 Snapshot JSONB

不採用。Finding 需要跨 Snapshot 查詢、Case 關聯、feedback optimistic locking、effectiveness 與 audit。

### 永久保留 Pattern 與 Check 兩套 Finding tables

不採用。它會複製 feedback、Evidence 與 effectiveness 行為，重現本次要解決的雙重規則問題。

### Snapshot 複製完整 event payload

不採用於主線。Snapshot 只保存最少 metadata 與 checksum；需要較長期的 masked payload 時由 ADR-007 的
可選 Evidence Archive 處理。

## 後果

### 正面

- 使用者保存的結果與畫面一致；
- late event 不覆寫歷史 Snapshot；
- Finding 不再被綁死於 Pattern 或 Case；
- 現有 finding IDs、feedback 與 audit 可遷移；
- Event payload retention 與 Case retention 不再被迫相同。

### 成本

- 保存操作需要第二次 deterministic evaluation；
- 需要 expand／backfill／compatibility migration 與 restart-persistence 驗收；
- Snapshot、relations、findings 與 Case links 增加 PostgreSQL table 數量；
- synthetic legacy Snapshot 必須誠實呈現無法回填的資料。

## 相關文件

- [ADR-005：Event Check 架構](adr-005-event-check-and-check-model-architecture.md)
- [Target Design](../architecture/event-check-target-design.md)
- [Pattern Governance Contract](../contracts/pattern-governance-contract.md)
- [Data Model](../architecture/data-model.md)
