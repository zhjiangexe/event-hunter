# Requirements 是什麼？

這裡的 `requirements/` 不是 Python 套件清單，也不是另一份長篇 PRD。它是 Event Hunter 的
「需求追蹤與交付規格」，用穩定 ID 把使用者能力一路連到 API、資料表、前端路由與可執行測試。

## 兩種不同 ID

| ID | 代表什麼 | 穩定性 | 範例 |
|---|---|---|---|
| `REQ-EH-*` | 使用者或系統必須具備的能力 | 功能仍存在時不應更名或重用 | `REQ-EH-003` 確定性 Domain Pattern |
| `EH-MVP-*` | 為完成需求而執行的工程任務 | 可隨實作拆分、合併或重排 | `EH-MVP-007` 實作 Pattern Engine |
| `EH-POC-*` | 由隔離實驗開始的架構工作；若正式採用須新增 adoption task | 保留決策歷史，不等同產品需求 | `EH-POC-003` ClickHouse-first adoption |

需求描述「要交付什麼結果」，工程任務描述「依什麼順序完成它」。一個需求可能需要多個工程任務；
一個工程任務也可能同時支援多個需求。

## 目錄內的文件

### `project-scope.yaml`

產品範圍與全域決策的最高層來源，定義正式 `REQ-EH-*` 集合、Temporal 邊界、身份模式及明確排除項目。
`traceability.yaml` 與 `implementation-plan.yaml` 的 requirement 集合不得和它分歧。

### `current-architecture.md` 與圖表

目前可執行系統的主要架構入口。PlantUML 原始檔與渲染 PNG 一併放在本目錄：

- `event-hunter-architecture.puml`／`event-hunter-architecture.png`
- `event-hunter-activity.puml`／`event-hunter-activity.png`

### `data-model.md` 與 `investigation-summary.md`

前者記錄 PostgreSQL／ClickHouse 資料模型及 ownership；後者記錄 Investigation Summary read model、
來源組合與 partial failure 語意。精確 HTTP schema 仍以 repository root 的 `openapi.yaml` 為準。

### `ui-prototype.html`

早期產品原型，只用於需求與 Phase 對照，不是正式前端 runtime。正式實作狀態以
`prototype-phase-matrix.yaml`、React routes 與 acceptance tests 為準。

### `traceability.yaml`

需求追蹤矩陣。每個 `REQ-EH-*` 必須明確連到：

```text
需求能力
├── OpenAPI operationId
├── Event／Pattern contract
├── PostgreSQL／ClickHouse store
├── React route
└── Karate acceptance feature
```

例如 `REQ-EH-003` 不是只寫「系統要能找異常」，而是確定連到：

```text
listPatterns + analyzeInvestigation + classifyPatternFinding
→ contracts/patterns/payment-completed-without-shipment.yaml
→ postgres.pattern_findings + postgres.pattern_finding_feedback + postgres.audit_logs
→ /patterns
→ e2e/backend/pattern.feature
```

這讓 Agent 能判斷修改 Pattern 時，哪些 API、資料表、畫面與測試必須一起檢查。

`REQ-EH-007`～`REQ-EH-009` 是沒有自建 Event Hunter 頁面的 supporting capability：Grafana Alert
Intake、品質聚合／Grafana assets，以及示範 Outbox-to-Timeline pipeline。它們以 integration contract、
外部 UI、資料表與 infrastructure Karate feature 追蹤，不為了形式硬加 React route。

### `implementation-plan.yaml`

工程任務 DAG。`depends_on` 表示前置任務，Agent 不應跳過契約或儲存層，直接從 UI 開始猜 API。
每個任務包含：

- `outputs`：必須產生的程式碼或 artifact。
- `acceptance`：什麼結果才算完成。
- `requirement_ids`：此任務正在交付哪些需求。
- `optional`／`feature_flag`：例如 Temporal 是選用能力，不可阻擋核心 MVP。
- `status`：只使用 `pending`、`in_progress`、`completed`；完成任務的所有前置任務也必須完成。

### `phase-1-delivery-plan.md`

Phase 1 收尾清單與 `requirements/ui-prototype.html` 對照表。它把原型內容分成 Phase 1 必須完成、Phase 1
非阻擋項目、Phase 2 Projection Rebuild、Phase 3 Sandbox Replay 與永久排除項目，並列出最終
驗收命令。後續 Agent 在處理 `EH-MVP-009` 或 `EH-MVP-016` 前必須先閱讀此文件。

### `phase-1-1-development-plan.md`

Phase 1 完成後且已 sign-off 的產品化開發計畫。它保留 event-centric 證據核心，並交付 Overview、
Order／Business Journey、Investigation 協作、Pattern 成效、資料可信度、更多 live scenarios 與
production hardening 的本機基準。已交付切片使用穩定的 Phase 1.1 requirement（目前為 `REQ-EH-010`～
`REQ-EH-016`）；新的候選工作仍不得在未核准 scope／contract 前直接實作。

### `phase-1-1-product-ux-remediation-plan.md`

2026-08-26 逐頁桌面／390px 實機稽核後建立的產品 UX 與資訊可信度修正計畫。它不回溯否定
Phase 1.1 local release baseline，而是把動態查詢時間、案件 Incident Window、案件內 Pattern 閉環、
明確狀態轉換、responsive blockers，以及 Journey／Pattern／Case／Scenario 的判讀與 drill-down 缺口，
整理成 UX-A～UX-C 三波候選工作。正式開發前仍須核准 scope 並寫入 requirement、implementation DAG、
contract 與 acceptance tests。

### `prototype-phase-matrix.yaml`

`requirements/ui-prototype.html` 的機器可讀功能索引。每個 `PROTO-EH-*` 都標示 phase、是否阻擋 Phase 1、目前
實作狀態、需求 ID、route／external UI 與剩餘工作，供 Agent 不需重新猜測原型內容。

### `repository-layout-and-maintenance.md`

程式碼、測試、設定與生成物的目錄責任及維護規範。它定義 backend dependency direction、
Karate source／runner／report 分流、canonical report 保留政策，以及 contract、`.env.example`、
Compose、infra config 與 script fallback 的設定優先順序。

### `clickhouse-mv-ingestion-poc.md`

ClickHouse-first ingestion 的 POC 結論與正式採用基準。它記錄「全量 raw landing → 最低 Admission
Contract → valid promotion／failure summary」、domain／attempt 正式 runtime、安全邊界、官方 Sink 版本、
Karate／recovery 驗收與仍延後的 production governance。

### `dependency-upgrade-audit.md`

2026-08-27 的應用程式、建置工具與 Compose infrastructure 版本稽核。它記錄本輪已更新且通過的
toolchain／package、因 24 小時供應鏈政策或 peer range 保留的版本，以及 stateful infrastructure
不可直接追 latest 的 migration 與 restart-persistence gate。

### `live-event-observability-contract.md`

三個 live demo services 的事件生命週期 log 契約。它定義發布前、outbox commit 後、失敗與 Kafka
consumer completed 的訊息與欄位，並明確區分「outbox 已提交」和「Kafka 已送達」。Timeline 的 Loki
deep link、OTel trace correlation、隱私邊界及可執行驗收都以此文件為準。

## 與其他文件的分工

| 文件 | 回答的問題 |
|---|---|
| `requirements/project-scope.yaml` | MVP 做什麼、不做什麼，以及已接受的全域決策 |
| `requirements/current-architecture.md` | 目前實際可執行的元件、資料流、Scenario Lab、使用者活動與 backend dependency；不混入 Phase 2／3 |
| `requirements/traceability.yaml` | 每項需求落到哪些 API、資料、UI 與驗收測試 |
| `requirements/implementation-plan.yaml` | 工程任務的依賴順序與完成條件 |
| `requirements/business-journey-profile-contract.md` | Journey Profile YAML 的 authoring、規則語意、runtime 與多 profile 邊界 |
| `requirements/investigation-list-query-contract.md` | 案件清單複合篩選、穩定排序、keyset cursor 與可分享 URL 語意 |
| `requirements/investigation-incident-window-contract.md` | 案件不可變基準窗口、來源推導、current-view override、partial Summary 與 restart 語意 |
| `contracts/platform/investigation-state-machine.yaml` | Aggregate-owned 案件狀態轉移、必填結論、`allowed_transitions` 與 optimistic-lock 邊界 |
| `requirements/phase-1-delivery-plan.md` | Phase 1 收尾順序、原型差異與 exit criteria |
| `requirements/phase-1-1-development-plan.md` | Phase 1.1 產品化候選工作包、優先順序與多視角產品入口 |
| `requirements/phase-1-1-product-ux-remediation-plan.md` | Phase 1.1 逐頁 UX／資訊可信度稽核後的修正順序、驗收與排除範圍 |
| `requirements/prototype-phase-matrix.yaml` | 原型功能的 phase、狀態與正式需求映射 |
| `requirements/repository-layout-and-maintenance.md` | 程式碼、測試、設定、報告與生成物應放在哪裡，以及完成變更前的檢查 |
| `requirements/clickhouse-mv-ingestion-poc.md` | ClickHouse-first ingestion POC 的隔離方式、最低契約、驗收與未決策邊界 |
| `requirements/dependency-upgrade-audit.md` | 本輪套件升級、latest 差異、延後原因與 infrastructure 升級驗收門檻 |
| `requirements/live-event-observability-contract.md` | Live Domain Event 發布前後／失敗 logs、OTel 欄位、outbox/Kafka 語意與 Loki 驗收 |
| `openapi.yaml`／`contracts/asyncapi.yaml` | HTTP／Kafka 的精確介面 |
| `backend/migrations/**` | 實際資料庫結構 |
| `e2e/**/*.feature` | 從外部觀察時，功能應如何表現 |

## 變更規則

1. 新增使用者能力時，先新增 `REQ-EH-*`，再補 API、store、route 與 acceptance feature 映射。
2. 只調整內部重構或工作順序時，修改 `EH-MVP-*`，不需要新增需求 ID。
3. 刪除或延後需求時，不重用原 ID；保留紀錄並修改 scope／status。
4. Requirement 只有在對應的 contract、implementation 與 acceptance test 都完成時才算完成。
5. `scripts/validate-contracts.py` 會檢查 operationId、feature 路徑、需求集合、任務 DAG、fixture Schema、Topic、目標表、狀態機與 Demo 服務拓撲，防止契約彼此漂移。

這種方式比純自然語言文件更接近可執行規格：自然語言只解釋意圖，真正邊界由 YAML、OpenAPI、
JSON Schema、migration 與 Karate assertions 共同鎖定。
