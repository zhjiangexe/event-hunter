---
document_id: EH-DOC-GOV-003
status: active
owner: platform
last_reviewed: 2026-08-28
source_of_truth: true
canonical_topic: documentation-governance
supersedes: []
---

# 文件治理與生命週期

本文件定義 `requirements/` 的分類、狀態、唯一來源與維護方式。目標不是增加文件數量，而是讓人與 Agent
都能快速判斷「哪一份是現況、哪一份只是歷史證據」。

## 分類

| 目錄 | 回答的問題 |
|---|---|
| `product/` | 產品做什麼、不做什麼，以及原型如何映射正式功能？ |
| `architecture/` | 現行系統如何組成、資料如何流動與保存？ |
| `contracts/` | 某項產品或跨元件行為必須遵守什麼語意？ |
| `decisions/` | 為何選擇目前方案，替代方案為何未採用？ |
| `delivery/` | 現在按什麼順序開發，完成條件是什麼？ |
| `testing/` | 如何從系統外部證明需求成立？ |
| `operations/` | 如何啟停、診斷、備份、恢復及升級？ |
| `governance/` | 如何維護文件、程式庫與需求追蹤？ |

## 狀態

| 狀態 | 意義 | 是否可作為現行依據 |
|---|---|---|
| `draft` | 尚未核准 | 否 |
| `active` | 現行有效 | 是 |
| `in_progress` | 已核准且仍在交付 | 是，但完成狀態以 task／驗收為準 |
| `adopted` | 已採用的架構決策 | 是 |
| `reference` | 對照或輔助資料 | 否 |
| `verified` | 特定日期的稽核快照 | 否 |
| `completed` | 已完成的計畫或驗收證據 | 否，僅供歷史追溯 |
| `superseded` | 已被其他文件取代 | 否 |
| `archived` | 不再維護 | 否 |

## Metadata

主要 Markdown 使用 YAML front matter；機器可讀 YAML 使用 `document_control`。必要欄位為：

```yaml
document_id: EH-DOC-...
status: active
owner: platform
last_reviewed: 2026-08-28
source_of_truth: true
canonical_topic: current-runtime-architecture
supersedes: []
```

PlantUML、PNG 與 HTML 等 review assets 不重複嵌入 metadata，改由
[`document-catalog.yaml`](document-catalog.yaml) 管理。

## 唯一來源優先順序

1. 可執行契約：OpenAPI、AsyncAPI、JSON Schema、migration 與 platform contracts。
2. `product/project-scope.yaml`：產品邊界與穩定需求集合。
3. `governance/traceability.yaml`：需求到 API、store、route 與 acceptance test 的映射。
4. `delivery/implementation-plan.yaml`：工程 task DAG 與完成狀態。
5. `architecture/current-architecture.md` 與 `contracts/`：現行架構和行為語意。
6. `delivery/archive/`、`operations/audits/`：歷史證據，不得覆蓋現況。

## 變更流程

1. 先確認 `document-catalog.yaml` 中的 canonical topic，避免新增重複來源。
2. 修改現行文件時同步更新 `last_reviewed`、traceability、契約與 acceptance tests。
3. 計畫完成後移入 `delivery/archive/`，狀態改成 `completed`，但不刪除需求 ID 與驗收證據。
4. 架構決策改變時新增 ADR；舊 ADR 改成 `superseded` 並填入 `supersedes`／replacement 關係。
5. 完成前執行 `python3 scripts/validate-contracts.py`，檢查 catalog、metadata、本地連結與契約一致性。

## README、Guide 與 Runbook 的邊界

- 根目錄 `README.md`：讓第一次接觸專案的人理解用途、啟動方法與核心入口。
- `requirements/README.md`：文件導航與目前狀態，不複製完整規格。
- 前端 Guide：給使用者與接入團隊的操作說明，不宣告尚未實作的能力。
- Operations Runbook：給實際值班／維運人員的可執行命令與故障判讀。
