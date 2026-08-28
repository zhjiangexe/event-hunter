---
document_id: EH-ADR-007
status: adopted
owner: platform
last_reviewed: 2026-08-29
source_of_truth: true
canonical_topic: duckdb-parquet-evidence-archive
supersedes: []
---

# ADR-007：DuckDB／Parquet Evidence Archive

## 狀態

`adopted`，但實作是 `EH-ECM-007` optional follow-on，預設 feature flag 關閉，不阻擋 Event Check 與 Check
Models canonical cutover。

## 背景

ClickHouse canonical events 有 retention，Investigation Case 與 Check Snapshot 可能保留更久。主線 Snapshot
只保存最少 event metadata 與 payload checksum；部分案件未來可能需要依治理政策保留經遮罩的 event
fields 或 envelope，並能在 canonical source 過期後離線查詢、匯出與驗證。

直接延長所有 ClickHouse retention 會把一般 telemetry 成本與少數 Evidence hold 綁在一起；把完整 payload
放入 PostgreSQL JSONB 會擴大控制面、備份與權限風險；共享 DuckDB native database 又不適合作為 Event Hunter
多程序 transactional source of truth。

## 決策

### 1. Storage responsibility

```text
PostgreSQL
└── archive catalog、policy ref、checksum、expiry、lifecycle、Case／Snapshot link

Immutable encrypted Parquet objects
└── 經 Retention Profile 選定且遮罩後的 evidence rows

DuckDB adapter
└── 建立、驗證、查詢與匯出 Parquet；不接受前端任意 SQL
```

PostgreSQL 仍是 Snapshot、Case、Finding 與 archive lifecycle 的 authoritative store。DuckDB 是應用程式內的
query／export engine，不作共享多程序交易資料庫，也不保存需要頻繁 update 的控制面 state。

### 2. Immutable object boundary

一個 archive object 綁定一個 Snapshot 與 Retention Profile version。物件建立後不可 append 或 update；後續
補充建立新 object／revision，由 PostgreSQL catalog 保存 lineage。每個 object 有：

- object ID、Snapshot ID、profile ID／version；
- schema version、row count、created／expires time；
- plaintext canonical manifest checksum 與 encrypted object checksum；
- encryption key reference，不保存 key；
- storage URI（只由 server-side allowlisted storage adapter 解析）；
- lifecycle：`CREATING`、`AVAILABLE`、`EXPIRED`、`PURGED`、`FAILED`。

Parquet field IDs 與 schema version 必須固定；reader 不能以 `union_by_name` 靜默吸收不相容欄位變更。

### 3. Versioned Retention Profiles

Retention Profiles 是 YAML／Git immutable Registry，最低模式：

- `REFERENCE_ONLY`
- `METADATA_ONLY`
- `MASKED_FIELDS`
- `MASKED_ENVELOPE`

Profile 定義 event selection、context before／after、relation depth、event type allow／deny lists、payload field
allow／deny lists、telemetry reference policy、archive TTL 與 encryption policy。

實際可保存範圍是 Model 建議、organization policy、data classification、actor role 與使用者限制的交集。
使用者只能選相同或更嚴格的 Profile，不能提升自己可讀或可封存的欄位。

`RAW_RESTRICTED`、legal hold、多租戶隔離與 runtime policy editor 不由本 ADR 自動啟用；需要另行核准完整身分、
key management、audit 與刪除政策。

### 4. Security boundary

- 不提供 SQL、file path、URI、extension name 或 network destination 作為 public API input；
- DuckDB extension allowlist 固定在 build／deployment configuration；
- archive worker 使用專用 OS identity 與最小 storage permission；
- encryption key 由外部 secret／key provider 提供，不寫入 YAML、PostgreSQL、Parquet metadata 或 logs；
- query API 只接受 typed allowlisted filters、projection 與 bounded limits；
- temp files、failure logs 與 audit 不包含明文 payload；
- archive purge 必須有 dry-run、bounded target、confirmation、Audit 與 checksum evidence。

### 5. Failure and recovery

Archive 建立採 outbox／job 或可重試 command，但它只處理 Event Hunter Evidence，不控制 production business
flow。Catalog 先建立 `CREATING`，object 以 temporary name 寫入、完成 encryption／checksum 驗證後 atomic
publish，再標記 `AVAILABLE`。失敗標記 `FAILED`，不得留下可被一般 reader 掃描的半成品。

Snapshot 與 Case 不因 archive 失敗而消失；UI 顯示 metadata-only Evidence 與 archive failure。Archive 是
可選增強，不是 Event Check 正確性的前置條件。

## 未採用方案

### 共享 DuckDB native database 作為主資料庫

不採用。Event Hunter API、worker、backup／restore 與未來多實例需要清楚的 concurrent ownership；共享檔案
鎖定與多程序 write 會增加不必要的 operational coupling。

### 將完整 Payload 存入 PostgreSQL Snapshot JSONB

不採用。這會擴大控制面、備份、RBAC 與資料刪除風險，並讓一般案件查詢承擔大型 payload 成本。

### 永久延長 ClickHouse retention

不採用作為 Evidence hold。一般事件分析 retention 與少數案件 Evidence retention 應可分開治理。

### 一個可持續 append 的 Case archive file

不採用。Immutable per-Snapshot objects 更容易驗證 checksum、控制 retention、重試與避免同時寫入衝突。

## 後果

### 正面

- Snapshot 主線不需要保存完整 payload；
- Evidence retention 可以依案件與資料分類設定；
- Parquet 可攜、可壓縮、可加密，DuckDB 能做 bounded analytical query／export；
- archive failure 不影響 Event Check 與 Case metadata；
- PostgreSQL 保持控制面唯一來源。

### 成本

- 需要 object storage／volume、key、catalog、worker、purge 與 backup／restore；
- 必須維護 Parquet schema compatibility 與 DuckDB dependency；
- Retention Profile 與欄位分類需要額外治理；
- 正式開啟前必須增加 archive security、recovery 與 deletion E2E。

## 官方技術依據

- [DuckDB concurrency](https://duckdb.org/docs/current/connect/concurrency)
- [DuckDB Parquet read/write](https://duckdb.org/docs/lts/data/parquet/overview)
- [DuckDB Parquet encryption](https://duckdb.org/docs/current/data/parquet/encryption)
- [Securing DuckDB](https://duckdb.org/docs/current/operations_manual/securing_duckdb/overview)

## 相關文件

- [ADR-006：Snapshot persistence](adr-006-check-snapshot-and-finding-persistence.md)
- [Target Design](../architecture/event-check-target-design.md)
- [產品需求](../product/event-check-and-check-models-requirements.md)
