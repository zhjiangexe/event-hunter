---
document_id: EH-DOC-IDX-001
status: active
owner: product
last_reviewed: 2026-08-29
source_of_truth: true
canonical_topic: requirements-index
supersedes: []
---

# Event Hunter 文件入口

這裡保存 Event Hunter 的產品範圍、現行架構、行為契約、交付計畫、測試、運維與歷史決策。
第一次接觸專案時，不需要逐份閱讀；先依角色與問題選擇入口。

## 目前狀態

| 範圍 | 狀態 | 正式依據 |
|---|---|---|
| Phase 1 MVP | `completed` | [Phase 1 歷史交付計畫](delivery/archive/phase-1-delivery-plan.md) |
| Phase 1.1 local release baseline | `completed` | [Phase 1.1 sign-off](delivery/archive/phase-1-1-sign-off.md) |
| Phase 1.1 product hardening | `internal_pilot_completed`；production target pending | [產品化計畫](delivery/phase-1-1-development-plan.md) |
| Phase 1.1 UX remediation | `completed` | [UX 修正計畫](delivery/phase-1-1-product-ux-remediation-plan.md) |
| Backend DDD／Clean Architecture 重構 | `in_progress`；行為保持重構 | [Backend 重構計畫](delivery/backend-ddd-clean-architecture-refactor-plan.md) |
| Event Check／Check Models 整合需求 | `canonical_cutover_completed`；EH-ECM-000～006 已完成 | [產品需求](product/event-check-and-check-models-requirements.md)／[Target Design](architecture/event-check-target-design.md) |
| Phase 2 Projection Rebuild | `deferred` | 未列入目前開發入口 |
| Phase 3 Sandbox Replay | `deferred` | 未列入目前開發入口 |

Phase 1.1 已具備 single-host internal staging／pilot hardening profile；這不代表 internet-facing production
ready。正式身分、HA、目標環境演練、容量與資料治理仍須由部署環境決策。

目前文件與 UI 一律使用下列 canonical 名稱：`Event Check`、`Saved Results`、`Check Models`、
`Investigation Cases`、`Ingestion Issues`、`Scenario Lab`。Business Timeline／Journey／Journey Profiles／
Pattern Library 只在 migration、deprecated API、legacy route 或歷史驗收證據中出現；遇到未標示用途的
舊名稱時，視為文件缺陷而不是新的產品入口。

## 依角色開始

### 第一次了解產品

1. [根目錄 README](../README.md)：專案用途、畫面與啟動方式。
2. [Product Scope](product/project-scope.yaml)：正式需求、排除範圍與 Phase 邊界。
3. [Event Check／Check Models 需求](product/event-check-and-check-models-requirements.md)：目前 canonical 產品模型；後端、persistence、workspace UI、legacy compatibility 與最終 cutover acceptance 均已完成。
4. [Event Check Target Design](architecture/event-check-target-design.md)：API、資料與安全設計；並標示目前已實作與後續邊界。
5. [Current Architecture](architecture/current-architecture.md)：目前實際執行的元件與資料流。

日常產品導覽固定為：Overview → Event Check／Saved Results → Investigation Cases／Ingestion Issues →
Check Models → Scenario Lab／Guide。`/timeline`、`/journey`、`/journey-profiles`、`/patterns` 與
`/saved-searches` 僅是 compatibility routes。

### 接入新的事件來源

1. [Product Scope](product/project-scope.yaml)：平台接受與不接受的能力。
2. [Current Architecture](architecture/current-architecture.md)：Kafka、ClickHouse 與 OTel 路徑。
3. [ClickHouse-first ADR](decisions/adr-001-clickhouse-first-ingestion.md)：為何採 raw landing + MV admission。
4. [Live Event Observability Contract](contracts/live-event-observability-contract.md)：logs／traces 的必要欄位。
5. 根目錄 `openapi.yaml`、`contracts/asyncapi.yaml` 與 `contracts/events/`：精確介面與 Schema。

### 開發後端或前端

1. [Implementation Plan](delivery/implementation-plan.yaml)：目前 task DAG 與完成條件。
2. [Backend DDD／Clean Architecture 重構計畫](delivery/backend-ddd-clean-architecture-refactor-plan.md)：目前 backend 分層收斂順序、非目標與逐 task 驗收。
3. [Traceability](governance/traceability.yaml)：需求對應 API、store、route 與 E2E。
4. [Repository Layout](governance/repository-layout-and-maintenance.md)：程式碼與測試應放的位置。
5. 對應的 [Architecture](architecture/) 或 [Behavior Contract](contracts/) 文件。

開發 Event Check／Check Models 時，先讀 [Target Design](architecture/event-check-target-design.md) 與
ADR-005～007；`EH-ECM-000`～`EH-ECM-006` 已完成。目前沒有未完成的必要 Event Check task；`EH-ECM-007` Evidence Archive 為預設關閉且不阻擋產品使用的選用後續。

### 執行測試與驗收

1. [Backend E2E Test Plan](testing/backend-e2e-test-plan.md)。
2. `e2e/README.md` 與 `e2e/**/*.feature`。
3. [Traceability](governance/traceability.yaml) 中的 acceptance feature mapping。

### 啟停、診斷或恢復服務

1. [Operations Runbook](operations/operations-runbook.md)。
2. [Single-host Hardening Baseline](operations/single-host-hardening-baseline.md)：內部 staging／pilot 的 TLS、secret、network 與 retention 邊界。
3. `infra/README.md`、`compose.yaml` 與 `compose.hardening.yaml`。
4. [Dependency Audit](operations/audits/dependency-upgrade-audit-2026-08-27.md) 只代表特定日期快照，
   不可當成永遠最新版本清單。

### 理解歷史決策

- [ADR-001：ClickHouse-first ingestion](decisions/adr-001-clickhouse-first-ingestion.md)
- [ADR-002：識別碼與關聯模型](decisions/adr-002-identifiers-and-correlation.md)
- [ADR-003：Telemetry provenance](decisions/adr-003-telemetry-provenance.md)
- [ADR-004：Scenario Lab 邊界](decisions/adr-004-scenario-lab-boundary.md)
- [ADR-005：Event Check 與 Check Model 架構](decisions/adr-005-event-check-and-check-model-architecture.md)
- [ADR-006：Snapshot 與 Finding persistence](decisions/adr-006-check-snapshot-and-finding-persistence.md)
- [ADR-007：DuckDB／Parquet Evidence Archive](decisions/adr-007-duckdb-parquet-evidence-archive.md)
- [文件盤點紀錄](governance/document-inventory.md)

## 目錄責任

| 目錄 | 內容 |
|---|---|
| [`product/`](product/) | 產品範圍與 prototype mapping |
| [`architecture/`](architecture/) | 現行架構、資料模型、read model 與 diagrams |
| [`contracts/`](contracts/) | 產品行為與跨元件語意；精確 machine contract 仍在 repository `contracts/` |
| [`decisions/`](decisions/) | 已採用的架構決策與取捨 |
| [`delivery/`](delivery/) | 現行 implementation DAG 與 Phase 1.1 計畫 |
| [`delivery/archive/`](delivery/archive/) | 已完成計畫與 sign-off 證據 |
| [`testing/`](testing/) | E2E 策略與案例合理性 |
| [`operations/`](operations/) | Runbook 與 point-in-time audits |
| [`governance/`](governance/) | Traceability、文件 catalog 與 repository 維護規範 |

## 需求與工程 ID

| ID | 用途 | 穩定性 |
|---|---|---|
| `REQ-EH-*` | 使用者或系統必須具備的能力 | 功能存在時不應更名或重用 |
| `EH-MVP-*`／`EH-P1.1-*` | 為交付需求而執行的工程任務 | 可拆分、合併或重排 |
| `EH-POC-*` | 隔離實驗或 adoption task | 不自動成為正式產品需求 |
| `EH-ADR-*` | 已採用的架構決策 | 被取代時保留並標記 superseded |

需求必須能追到：

```text
REQ-EH-*
  → OpenAPI / AsyncAPI operation
  → executable contract / schema
  → PostgreSQL / ClickHouse store
  → React route or trusted external UI
  → Karate acceptance feature
```

## 唯一來源

| 問題 | 唯一來源 |
|---|---|
| 產品做什麼、不做什麼？ | [`product/project-scope.yaml`](product/project-scope.yaml) |
| 需求落到哪些實作與測試？ | [`governance/traceability.yaml`](governance/traceability.yaml) |
| 工程任務順序與狀態？ | [`delivery/implementation-plan.yaml`](delivery/implementation-plan.yaml) |
| 現在實際跑什麼架構？ | [`architecture/current-architecture.md`](architecture/current-architecture.md) |
| HTTP／Kafka／event 精確格式？ | `openapi.yaml`、`contracts/asyncapi.yaml`、`contracts/events/` |
| 如何操作與排障？ | [`operations/operations-runbook.md`](operations/operations-runbook.md) |
| 文件狀態與責任？ | [`governance/document-catalog.yaml`](governance/document-catalog.yaml) |

## 維護規則

1. 新增能力前先配置 `REQ-EH-*`，再補 traceability、task、contract 與 acceptance test。
2. 不在 Phase 文件、README 或 Guide 另建一套需求狀態；它們只能引用正式來源。
3. 計畫完成後移入 `delivery/archive/`，不刪除歷史 ID 與驗收證據。
4. 架構選擇改變時新增 ADR，不直接改寫舊決策的歷史原因。
5. 新增、移動或封存文件時同步更新
   [`document-catalog.yaml`](governance/document-catalog.yaml) 與 `last_reviewed`。
6. 完成前執行 `python3 scripts/validate-contracts.py`；它會驗證契約、文件 metadata、catalog coverage
   與 Markdown 本地連結。

詳細規則請看 [文件治理與生命週期](governance/document-control.md)。
