---
document_id: EH-DOC-GOV-004
status: verified
owner: platform
last_reviewed: 2026-08-28
source_of_truth: false
canonical_topic: documentation-inventory-2026-08-28
supersedes: []
---

# 文件盤點與整理紀錄（2026-08-28）

## 盤點結論

整理前 `requirements/` 有 30 份規格與圖表全部混在同一層。主要問題不是內容不足，而是文件角色不明：

- 已完成 Phase、現行 Phase 1.1 backlog 與 sign-off 證據放在一起。
- POC 已正式採用，檔名仍讓人誤判為可選實驗。
- Architecture、Data Model、behavior contracts 與 Runbook 沒有清楚分類。
- 多份文件重複描述 scope、進度與架構，但缺少明確優先順序。
- 搬移前路徑由腳本、前端 Guide、README 和 YAML 直接引用，缺少本地連結檢查。

## 本輪動作

| 原內容 | 判定 | 動作 |
|---|---|---|
| `project-scope.yaml` | 產品範圍唯一來源 | 移至 `product/`，保持 active |
| Current architecture、data model、application architecture、summary read model | 角色互補，不合併 | 移至 `architecture/`，以 current architecture 為入口 |
| 8 份 feature behavior documents | 仍由 API／測試使用 | 移至 `contracts/`，保持 active |
| ClickHouse-first POC | 已正式採用，不再是 candidate | 改為 `decisions/adr-001-clickhouse-first-ingestion.md` |
| Phase 1 delivery plan | 已完成 | 移至 `delivery/archive/` |
| Phase 1.1 sign-off | 歷史驗收證據 | 移至 `delivery/archive/` |
| Phase 1.1 development／UX remediation | UX 已封板；single-host internal pilot hardening 已完成，target production hardening 待決策 | 留在 `delivery/`；development 保持 in_progress，UX work status completed |
| Dependency audit | 特定日期快照 | 移至 `operations/audits/`，狀態 verified |
| Prototype HTML／matrix | review reference，不是 runtime | 移至 `product/prototypes/` |
| Traceability／repository rules | 文件與需求治理 | 移至 `governance/` |

## 重複內容處理原則

- Scope 只由 `product/project-scope.yaml` 定義；Phase 文件只能引用，不重建另一套需求集合。
- Task 狀態只由 `delivery/implementation-plan.yaml` 定義；敘述性計畫保留背景與執行順序。
- Runtime 現況只由 `architecture/current-architecture.md` 說明；ADR 只記錄決策原因與取捨。
- HTTP／Kafka 精確欄位仍以根目錄 `openapi.yaml` 與 `contracts/` 下的可執行契約為準。
- 測試數量屬於時間點證據；長期 acceptance coverage 以 feature path 與 traceability 為準。

## 未刪除的內容

本輪沒有刪除 Phase 1／1.1 歷史計畫、原型或稽核資料。它們仍能解釋需求如何演進，但不再出現在現行
開發入口的第一層，也不得被 Agent 當成新的待辦來源。
