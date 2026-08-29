---
document_id: EH-DOC-DEL-004
status: completed
owner: backend
last_reviewed: 2026-08-29
source_of_truth: false
canonical_topic: backend-ddd-clean-architecture-refactor
supersedes: []
---

# Backend DDD／Clean Architecture 重構計畫

## 1. 目的與判定

本計畫將現有 Go backend 從「依賴方向大致正確，但部分責任混層」收斂成可由測試持續保護的
DDD／Clean Architecture。這是行為保持重構，不新增產品能力，也不改變 Event Check、Investigation Cases、
Ingestion Issues、Scenario Lab 或 compatibility API 的外部語意。

現況已確認：

- `domain` 不依賴 PostgreSQL、ClickHouse、Kafka、HTTP client、OpenTelemetry 或 `platform`。
- `application` 不反向依賴 `platform`，composition root 仍由外向內組裝。
- `InvestigationCase`、Event Check evaluator／scope／snapshot 已有實質 domain behavior。
- 主要缺口是 aggregate invariant 可被繞過、application capability 彼此耦合、HTTP／worker 邏輯留在
  `cmd`、Scenario Lab 混合 domain／application／adapter，以及 architecture tests 覆蓋過窄。

## 2. 不變條件與非目標

所有工作必須遵守：

1. 不改 OpenAPI operation、request／response schema、Kafka topic、event envelope 或 migration。
2. 不改現有案件狀態機、Check Model 結果、Scenario S1～S14 expected／actual 語意。
3. 每一個 task 單獨可編譯、可測試、可回滾；不允許先搬完全部檔案再一次修正。
4. `backend/internal/demo` 是被測的外部示範拓撲，本輪不把它改造成 Event Hunter bounded context。
5. deprecated Business Journey／Pattern compatibility 只做維持邊界所需的最小整理，不擴張功能。
6. generated Check Model／Journey／Pattern registry 與 `json.RawMessage` snapshot archival 本輪保留；另以
   architecture note 說明為刻意例外，不在同一輪改 storage contract。
7. package rename 只在能由 compiler 完整保護且不與責任搬移混在同一 task 時執行。

## 3. 目標依賴與目錄責任

```text
cmd/*
  └── process lifecycle + composition only
        ↓
internal/contexts/<context>/adapters/inbound/{httpapi,kafka,cli}
        ↓
internal/contexts/<context>/application/<capability>
        ↓
internal/contexts/<context>/{domain,ports}
        ↑
internal/contexts/<context>/adapters/outbound/{postgres,clickhouse,http,kafka}

internal/platform/{config,observability,health,db}
  └── cross-context technical runtime only
```

過渡期間既有 `internal/platform/postgres`／`clickhouse` 可以保留；本輪先拆責任與依賴，不為了目錄美觀
一次搬動所有 adapter。新 code 不得再把 business policy、SQL 與 transport mapping 放進同一檔案。

## 4. 任務 DAG

| Task | 內容 | 前置 | 狀態 | 必要性 |
|---|---|---|---|---|
| `EH-ARCH-000` | 基準盤點、計畫、依賴規則與非目標 | 無 | completed | 必要 |
| `EH-ARCH-001` | `InvestigationCase` invariant、rehydration 與 repository row mapping | 000 | completed | 必要 |
| `EH-ARCH-002` | application capability 去耦與必要 `UnitOfWork` | 001 | completed | 必要 |
| `EH-ARCH-003` | Investigation summary／evidence manifest use cases 與 HTTP adapter 瘦身 | 002 | completed | 必要 |
| `EH-ARCH-004` | Scenario Lab 拆成 domain／application／ports／adapters | 002 | completed | 必要 |
| `EH-ARCH-005` | quality worker／technical DLQ projector 移出 `cmd` | 000 | completed | 必要 |
| `EH-ARCH-006` | package dependency architecture tests、文件回填與完整 regression | 003,004,005 | completed | 必要 |

`EH-ARCH-003`、`EH-ARCH-004`、`EH-ARCH-005` 可在 `EH-ARCH-002` 完成後分支，但每次只合併一個可驗證
切片。正式執行順序採表格由上而下，避免同時大幅改動 API、Scenario 與 workers。

## 5. 詳細工作與驗收

### EH-ARCH-001：Aggregate invariant 與 rehydration

狀態：`completed`（2026-08-29）

交付：

- `InvestigationCase` 的 title、severity、resolved fields mutation 與 constructor 使用同一套驗證規則。
- persistence adapter 不直接把 database row 掃入可任意修改的 aggregate；先使用 row DTO，再透過
  `RehydrateInvestigationCase` 建立合法狀態。
- patch enum 不接受未知 status／severity／priority。
- 補 invalid mutation、resolved invariant 與 invalid persisted row tests。

驗收：

- aggregate 不會因 PATCH 或 rehydration 進入 constructor 不允許的狀態。
- optimistic lock、audit、現有 HTTP status 與 E2E 行為不退化。
- `go test ./internal/contexts/investigation/... ./internal/platform/postgres/... ./cmd/api/...` 通過。

完成證據：

- constructor、mutation、repository create／update 與 PostgreSQL row rehydration 共用 domain validation。
- invalid title／severity／status、resolved 欄位清空與 invalid persisted severity 有 focused tests。
- `go test ./...` 與 `go vet ./...` 通過。

### EH-ARCH-002：Application capability 去耦

狀態：`completed`（2026-08-29）

交付：

- `get_check_snapshot` 不依賴 `save_check_snapshot` 的 response type／mapper。
- snapshot evaluation 透過窄 `Evaluator` interface，而非具體 application service。
- mutation use case 的 `UnitOfWork` 為必要 dependency；單元測試使用明確 `NoOpUnitOfWork`。
- Forensics 共用 query contract 不再要求 use case 依賴另一個 use case 的 concrete service。

驗收：

- application capability 可由 interface test double 獨立測試。
- application package 之間沒有為了共用 DTO／mapper 而形成的方向性相依。
- Event Check deterministic hashes、Snapshot idempotency、Evidence audit 不變。

完成證據：

- Snapshot view model 已移至中立 application contract，`get_check_snapshot` 不再 import save capability。
- save capability 僅依賴窄 `Evaluator` interface；event search 與 Pattern analysis 各自依賴最小 read port。
- case lifecycle、evidence attachment、Pattern analysis 的 `UnitOfWork` 已成為 constructor 必要條件，不再靜默退化成非交易寫入。
- focused tests、`go test ./...` 與 `go vet ./...` 通過。

### EH-ARCH-003：Inbound HTTP adapter 瘦身

狀態：`completed`（2026-08-29）

交付：

- 新增 `get_investigation_summary` capability，負責 source orchestration、partial result 與 retention boundary。
- 新增 `generate_evidence_manifest` capability，負責 manifest、partial state 與 SHA-256。
- `cmd/api/investigations.go` 只保留 session／authorization、transport validation、use-case invocation 與
  response mapping；依責任拆成多個檔案。
- compatibility Business Journey 若仍需保留，其 deterministic build logic 移入 domain evaluator；不新增功能。

驗收：

- HTTP response 與既有 OpenAPI／Karate contract 相同。
- handler 不直接協調兩個 store、計算 evidence checksum 或建立 domain policy。
- `cmd/api` architecture tests 可證明沒有 SQL／ClickHouse query／manifest policy 回流。

完成證據：

- summary 的 PostgreSQL／ClickHouse orchestration、partial result、source status 與 90 天 retention boundary 已移至
  `get_investigation_summary` capability。
- evidence item allowlist、Grafana alert locator 驗證、partial warning 與 manifest SHA-256 已移至
  `generate_evidence_manifest` capability。
- summary／evidence transport handlers 已拆至責任獨立檔案；Business Journey 的 deterministic interpretation 已移至
  pure `domain/journeys` evaluator，application 僅讀事件並轉換 input。
- `go test ./...`、`go vet ./...` 與 backend Karate 19 features／126 scenarios 通過。

### EH-ARCH-004：Scenario Lab bounded context

狀態：`completed`（2026-08-29）

交付：

- `domain`：Scenario definition、Run state、Actual、Check evaluator。
- `application`：start／get／list run 與 async observation orchestration。
- `ports`：RunRepository、Publisher、Observer、OrderStarter、LinkBuilder、Clock。
- adapters：PostgreSQL repository、ClickHouse observer、HTTP Order starter、Grafana/Event Hunter link builder。
- `cmd/event-lab` 只保留 HTTP mapping、wiring、health 與 shutdown。

驗收：

- domain/application 不 import `database/sql`、`net/http`、Kafka client 或 OTel SDK。
- S1～S14 API、run persistence、actual checks、立即回傳 Run ID／Correlation ID 的行為不變。
- focused Go tests、Scenario Lab Karate 與 frontend flow 通過。

完成證據：

- Scenario definition、run／actual／check model 與 deterministic evaluator 已移至 pure `domain` package。
- start／get／list、非同步 observation 與結果更新由 `application.Runner` 負責，所有外部能力均透過窄 ports 注入。
- PostgreSQL、ClickHouse、Kafka、Order HTTP、link、clock 與 OTel 已分別置於 outbound adapters；HTTP handler 為 inbound adapter。
- `cmd/event-lab/main.go` 縮為 131 行的 composition root，只保留 wiring、health、signal 與 graceful shutdown。
- import scan 證明 domain／application／ports 不引用 SQL、HTTP、Kafka、OTel 或 platform package。
- `go test ./...`、`go vet ./...` 與 Scenario Lab focused Karate 2 features／29 scenarios 通過。

### EH-ARCH-005：Worker adapters 與 process lifecycle

狀態：`completed`（2026-08-29）

交付：

- quality aggregation window／backfill scheduling 與 ClickHouse executor 分離；SQL 留在 outbound adapter。
- technical DLQ 的 record summarization、projection use case、Kafka consumer 與 ClickHouse writer 分離。
- `cmd/quality-worker`、`cmd/technical-dlq-projector` 只組裝、處理 signal、readiness 與 shutdown。

驗收：

- transport identity、at-least-once、成功後 commit、原地 retry 與 safe metadata contract 不變。
- domain/application test 不需要 Kafka 或 ClickHouse。
- quality／ingestion recovery scripts 與既有 E2E 不退化。

完成證據：

- Quality window、31 天 backfill invariant 與 closed-window 計算已移至 domain；排程與 backfill orchestration 位於
  application；完整 ClickHouse aggregation SQL 位於 outbound adapter。
- Technical DLQ safe summarization 已成為 pure domain function；projection application service 透過 ports 操作 Kafka
  source 與 ClickHouse repository。
- application test 明確驗證 insert 失敗原地重試、insert 成功後才 commit，以及 commit 失敗原地重試。
- Kafka transport identity 由 adapter 封裝原始 record；raw payload、exception message 與 stack trace 仍不寫入正式表。
- Quality E2E 1 feature／2 scenarios 通過；ingestion recovery 驗證 broker outage 時 API 503、既有 connector 原地恢復與
  S8 通過。

### EH-ARCH-006：Guardrails 與封板

狀態：`completed`（2026-08-29）

交付：

- 使用 package import graph 的 architecture test，至少保護：
  - domain 不 import adapters／platform／database／HTTP／Kafka／OTel；
  - application 不 import adapters／platform／transport framework；
  - `cmd` 不保存 SQL 或 domain policy；
  - Scenario Lab 遵守與其他 context 相同規則。
- 更新 application architecture、repository layout、current architecture 與本計畫實際完成狀態。
- 移除搬移後的 dead files、重複 mapper 與無效 compatibility helpers。

封板驗收：

```bash
python3 scripts/validate-contracts.py
(cd backend && gofmt -w <changed-files> && go vet ./... && go test ./...)
bash scripts/test-backend-e2e.sh
```

若 HTTP／Scenario response 有任何可觀察變化，再補 frontend component／Karate；純 worker 內部拆分仍需執行
對應 recovery script。不得以只通過 architecture test 取代行為 regression。

完成證據：

- 新增 `internal/architecture/dependencies_test.go`，以 Go AST import graph／source scan 保護 domain、application、
  ports、context root 與 `cmd` 邊界。
- Scenario Lab 最後的 flat fixture helper 已移至 outbound emission adapter；context 根目錄不再有 production Go file。
- current architecture、application screaming architecture、repository layout 與 implementation plan 已依實際結構更新。
- `python3 scripts/validate-contracts.py` 通過：45 YAML、36 JSON、25 schemas、699 refs、44 documents、135 links。
- `docker compose config --quiet`、`gofmt -l cmd internal`、`go test ./...` 與 `go vet ./...` 全數通過。
- Quality Karate 1 feature／2 scenarios、destructive ingestion recovery 與完整 Backend Karate 19 features／126 scenarios
  全數通過；測試後 API runtime profile 已恢復，非 canonical reports 已清理。

## 6. 風險與回滾

| 風險 | 控制方式 |
|---|---|
| Aggregate private fields 造成大量 mapper 變更 | 先共用 validation／rehydration，再決定是否於同 task 私有化 |
| HTTP 拆分改變 JSON null／空陣列 | golden response／Karate 先固定，再搬 orchestration |
| Scenario async goroutine context 或 polling 行為改變 | 保留既有 timeout／poll interval contract，使用 fake clock／observer 測試 |
| Worker commit/retry 語意退化 | 先抽 pure summarizer，再抽 ports；最後才搬 consumer loop |
| 過度重構 deprecated compatibility | 僅修違反依賴規則的部分，不新增 profile／pattern 能力 |

任何 task 若需要 migration、OpenAPI 或產品行為變更，必須停止並另立需求／ADR，不得藏在本重構內。
