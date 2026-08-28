---
document_id: EH-DOC-PROD-002
status: active
owner: product
last_reviewed: 2026-08-29
source_of_truth: true
canonical_topic: event-check-and-check-models-product-requirements
supersedes: []
---

# Event Check 與 Check Models 產品需求

## 文件狀態

本文件記錄已確認的下一個產品模型，適用於既有 `REQ-EH-001`、`REQ-EH-002`、`REQ-EH-003`、
`REQ-EH-004`、`REQ-EH-011`、`REQ-EH-012`、`REQ-EH-013` 與 `REQ-EH-015` 的整合演進。

本需求已確認並完成 `EH-ECM-000`～`EH-ECM-006`，目前是
`canonical_cutover_completed`：

- `/event-check` 與 `/check-models` 是主要產品入口；
- `/journey`、`/journey-profiles`、可無損映射的 `/timeline`／`/patterns` bookmarks 會轉成 bounded canonical state；
- 含廣泛 Timeline filters、複數識別碼或未知 Pattern reference 的舊網址保留唯讀相容畫面，避免靜默遺失條件；
- [Business Journey Profile Contract](../contracts/business-journey-profile-contract.md) 與
  [Pattern Governance Contract](../contracts/pattern-governance-contract.md) 僅保留 legacy compatibility 語意；
- 舊 API 已加 deprecation metadata；bounded legacy read paths 目前只作 compatibility adapter。實體 runtime／
  schema 移除必須另有 migration release、rollback window 與 release note，不與本次 cutover 混在一起。

正式工程 DAG 為 [Implementation Plan](../delivery/implementation-plan.yaml) 的 `EH-ECM-000`～
`EH-ECM-006` 且已全數完成；`EH-ECM-007` 是不阻擋主功能、預設關閉的 Evidence Archive follow-on。
任務狀態只以該文件為準。

## 產品定義

Event Hunter 是唯讀的事件鑑識與流程符合性檢查平台。使用者以一個已知識別碼為起點，系統建立
可解釋且有界的事件集合，再以版本化 Check Model 比對實際事件軌跡，辨識正常完成、合理失敗、
補償、進行中與流程偏離。Event Hunter 不控制 production workflow，不發送正式業務命令，也不成為
訂單、付款或出貨狀態的 source of truth。

```text
輸入任意已知 ID
  → 解析事件範圍與關聯原因
  → 取得 0～N 個 canonical events
  → 推薦一個主要 Check Model
  → 使用者確認或更換 Model
  → 執行主要流程、子流程與 Global Checks
  → 顯示 Summary、Timeline、Flow、Findings、Cases
  → 明確保存或建立 Case 時才建立 Check Snapshot
```

## 核心概念與責任

| 概念 | 責任 | 不負責 |
|---|---|---|
| Event Check | 從 ID 解析範圍、選 Model、執行檢查並組織結果 | 修改 canonical event |
| Business Timeline | 呈現實際事件事實、順序、識別碼與 telemetry links | 推測業務狀態 |
| Flow Result | 依 Check Model 解釋實際路徑、目前節點與候選後續 | 控制正式流程 |
| Check Model | 定義適用範圍、合理路徑、Expectation、子流程與結果分類 | 執行業務命令 |
| Global Check | 檢查跨流程的事件完整性、重複、trace context 等品質 | 描述完整領域流程 |
| Check Snapshot | 固定某次檢查的輸入範圍、Model 版本、判斷時間與結果 | 複製或改寫原始 payload |
| Investigation Case | 保存人員調查狀態、Snapshot、Evidence、Notes 與 Audit | 自動代表 production incident 已成立 |

`Journey Profile` 與 `Pattern Library` 在目標產品中整合為 `Check Models` 的兩種能力：Flow Models
描述合理業務路徑，Detection Rules／Expectations 描述缺漏、逾時、順序或跨流程偏離。Runtime
仍可使用不同 evaluator，但相同條件不得在 Journey anomaly 與 Pattern 各自重複定義。

## 識別碼與事件範圍

### 可接受的起點

Event Check 接受 Event ID、Trace ID、Correlation ID、Aggregate ID 或受治理的業務識別碼。系統不得只靠
字串外觀猜測類型；若無法唯一辨識，必須回傳候選類型與要求的補充條件。

### Scope Resolver

Resolver 先找到 seed event，再依下列可解釋關係建立有界事件集合：

1. 相同 Correlation ID；
2. 相同 Aggregate Type／Aggregate ID；
3. 明確 Causation ID；
4. Check Model 或 ingestion mapping 宣告的 Business Key 關係；
5. 受治理的 parent／child aggregate 關係。

一次檢查可以跨多個 Correlation ID，但每條跨 correlation 邊必須附上關聯種類、來源欄位與來源事件。
系統不得以時間接近、payload 相似或不可解釋的機率配對自動納入事件。

所有 scope 都必須有 `from`、`to`、result limit 與來源健康狀態。預設範圍可由系統建立；使用者若排除或
加入事件，該次執行必須標示為 `CUSTOM_SCOPE`、保存調整原因，且不得修改 canonical events。

### 無資料與未知事件

- 0 個事件回傳 `NO_DATA`，同時顯示 canonical source health；不執行 Check Model、不自動建立 Case。
- Model 未映射的 Event Type 列入 `unmapped_events`，預設不構成偏離；Model 可明確將特定未知事件升級為提醒或違反。
- 相同 `event_id` 重複是資料品質問題；相同 `event_type` 可合理重複，由 Model 的 cardinality 規則判斷。

## Check Model

### Registry 與版本

- UI 名稱為 `Check Models`，底下分成 `Flow Models` 與 `Global Checks`。
- Authoring source 維持 YAML／Git；前端在本階段唯讀，不提供 runtime CRUD。
- 已發布的 `model_id + version` 不可原地修改；內容變更必須發布新版本。
- 每次執行及 Snapshot 都保存 Model ID、version、source path 與 checksum。
- Runtime 只使用已發布且 active 的 generated immutable Registry，不在 production 啟動時動態解析 workspace YAML。

### 適用範圍與候選推薦

Model 至少可依 domain、aggregate type、start／trigger event types 與支援的 event schema versions 宣告
`applies_to`。Resolver 在取得事件集合後才選擇 Model：

- 只有一個高可信候選時自動預選並執行，但 UI 必須顯示 Model、版本與推薦原因；
- 有多個候選時，UI 顯示候選與理由，要求使用者確認主要 Model；
- 沒有候選時仍可查看 Timeline，但 Flow Result 顯示 `NO_APPLICABLE_MODEL`；
- 使用者切換 Model 時沿用同一 event scope 重新評估，不重新查詢或寫回事件。

一次檢查只有一個主要 Flow Model。主要 Model 可以引用子 Flow Models，適用的 Global Checks 由系統自動
執行；一般使用者不需要逐項勾選通用資料品質規則。

### 表達能力與邊界

Flow Model 採受限制的事件流程圖，支援：

- 分支與替代路徑；
- optional milestone；
- event cardinality／合理重複；
- parent／child Flow Model；
- success、expected failure、cancelled／compensated 等 terminal outcome；
- event-time Expectation、Reminder、Violation 與 late satisfaction；
- 受治理的 exclusion 與 unmapped-event policy。

Model 不得包含 production command、HTTP／RPC action、業務重試、補償執行或任意程式碼。Event Hunter
可以建立自己的 Finding、Snapshot、Case 與通知意圖，但不得要求 payment、shipping 或 order service
執行正式操作。

### Outcome

Model 可定義領域結果名稱，再映射平台分類：

```yaml
outcome:
  code: RETURNED_AND_REFUNDED
  label: 已退貨並退款
  category: COMPENSATED
```

平台分類至少包含：

- `SUCCESS`
- `EXPECTED_FAILURE`
- `COMPENSATED`
- `INCOMPLETE`

合理失敗或補償路徑可以是 `CONFORMANT`；業務結果不得與流程符合性混成一個欄位。

## 檢查結果語意

結果分成三個互相獨立的維度：

| 維度 | 值 | 意義 |
|---|---|---|
| Check status | `NO_DATA`、`IN_PROGRESS`、`CONFORMANT`、`DEVIATED`、`INCONCLUSIVE`、`AMBIGUOUS` | 整組事件是否足以判斷且符合 Model |
| Business outcome | `SUCCESS`、`EXPECTED_FAILURE`、`COMPENSATED`、`INCOMPLETE` 加領域 outcome code | 業務最後走到什麼結果 |
| Expectation state | `SATISFIED`、`WAITING`、`REMINDER`、`VIOLATED`、`LATE_SATISFIED` | 單一預期事件的時間狀態 |

尚未走完且仍可能符合多條合理路徑時，結果維持 `IN_PROGRESS`，列出候選路徑與正在等待的事件；不得過早
猜測單一路徑。若同一 `as_of` 同時符合互斥 terminal outcomes，回傳 `AMBIGUOUS` 並說明衝突證據。

### 時間語意

- `as_of` 使用查詢結束時間；一般即時查詢的結束時間預設為目前時間。
- Reminder 與 Violation 門檻由各 Check Model 定義，不使用全平台固定分鐘數。
- 缺少事件只有在 deadline 已成熟且 canonical source health 可用時才能成為 `VIOLATED`。
- 來源不健康或範圍不足時回 `INCONCLUSIVE`，不得將「沒有讀到」偽裝成業務異常。
- 逾時後事件才到達時，最新即時計算可回 `LATE_SATISFIED`；既有 Snapshot 不得被覆寫。

## Check Snapshot 與 Case

一般查詢只即時計算，不保存每次瀏覽。只有使用者明確按「保存結果」或建立／加入 Investigation Case
時，才建立 immutable Check Snapshot。Snapshot 至少保存：

- scope type、seed identifier、from、to、`as_of` 與 `STANDARD_SCOPE`／`CUSTOM_SCOPE`；
- 納入／排除的 Event references 與每條關聯的 provenance；
- Model ID、version、checksum、主要 Model 與子 Models；
- Check status、Business outcome、Expectation states、候選／實際路徑；
- Findings、unmapped events 與 canonical source health；
- 執行者、執行時間及自訂 scope 原因。

Snapshot 只保存 immutable Evidence references，不複製完整 payload。Payload 仍由 canonical store 與既有
masking／RBAC 管理。

為了讓 Snapshot 在 ClickHouse event retention 到期後仍可理解，Snapshot 同時保存最少且不可變的 event
metadata：event ID、event type、occurred time、producer、aggregate、correlation、trace ID 與 payload
checksum；不因此取得或保存完整 payload。若原始事件已過期，UI 必須顯示 source expired，而不是假裝
Evidence 仍可展開。

### 後續 Evidence Archive

可設定長期證據保留不阻擋本階段。後續 `EH-ECM-007` 採 PostgreSQL archive catalog、DuckDB query／export
adapter 與 immutable encrypted Parquet objects；DuckDB 不取代 PostgreSQL Snapshot／Case／Finding，也不
作為多程序共享 transactional database。

Retention Profile 維持 YAML／Git、versioned 與前端唯讀，至少支援 `REFERENCE_ONLY`、`METADATA_ONLY`、
`MASKED_FIELDS` 與 `MASKED_ENVELOPE`。實際可保存內容是 Model 建議、組織 policy、data classification、
角色與使用者限制的交集；使用者只能選擇相同或更嚴格的 Profile，不能提升自己可保存的資料範圍。

進階欄位分類、Raw evidence approval、legal hold、多租戶隔離與 archive key rotation 可在該 follow-on
細化；目前主線仍必須具備 Viewer 不可保存／建案、Investigator 才可保存、自訂 Scope Audit、既有 masking
沿用、無 raw payload 與不暴露任意 DuckDB SQL／檔案路徑的最低安全基線。

本階段 `DEVIATED`／高嚴重度 Finding 顯示「建議建立 Case」，仍由使用者確認。持續掃描、Reminder 通知、
Finding 去重及自動建立 Case 不列入本階段實作；未來可以 policy 啟用，但不得回頭控制 production workflow。

## 目標資訊架構

側邊選單按工作目的分組，不將所有 Registry 與查詢頁平鋪：

```text
總覽
└── Overview

探索與檢查
├── Event Check
└── Saved Results

問題與調查
├── Investigation Cases
└── Ingestion Issues

模型與規則
└── Check Models

工具與說明
├── Scenario Lab
└── Event Hunter Guide
```

Event Check 查詢成功後提供：

```text
Summary | Timeline | Flow | Findings | Cases
```

找到適用 Model 時預設開啟 Summary；沒有 Model 或沒有資料時優先顯示 Timeline／說明與 Source Health。
Timeline 每筆事件提供以該 event、correlation、trace 與時間窗建構的 Grafana Explore、Loki、Tempo 與
Quality Dashboard trusted deep links。

Event Check 的結果操作明確分成三種，不以單一「保存並交接」動作混合：

- `保存結果`：只建立 immutable Check Snapshot；可由 `Saved Results` 列表重新查看，不建立案件。
- `建立案件`：若目前仍是即時計算，先保存 Snapshot，再建立新 Investigation Case 並掛上該 Snapshot。
- `加入案件`：若目前仍是即時計算，先保存 Snapshot，再選擇既有 Investigation Case 掛上該 Snapshot。

`Saved Results` 是 Snapshot 的唯讀工作清單，支援 identifier／check status 篩選與 stable keyset pagination，
顯示保存時間、Model、來源健康、事件／Finding／案件數量；不在列表回傳 raw payload。建立案件成功但掛接
失敗時，UI 必須保留已建立的案件連結並提供重試，不能假裝整個操作回滾成功。

Check Models 提供：

```text
Flow Models | Global Checks | Versions | Test Scenarios
```

Registry 首頁只顯示可比較的 Model 列表；點擊列後才以 URL-addressable modal 顯示 Overview、Versions 與
Test Scenarios，並支援 Escape、焦點還原與直接 deep link。

## 現有功能遷移表

| 現有功能／入口 | 目標位置 | 遷移要求 |
|---|---|---|
| Business Timeline `/timeline` | Event Check → Timeline | 保留 bounded query、event detail、telemetry links 與 URL state |
| Smart Search | Event Check 起始入口 | 擴充為 Scope Resolver，顯示 identifier type 與關聯 provenance |
| Business Journey `/journey` | Event Check → Flow | 改用使用者確認的主要 Check Model，不再只套唯一 default profile |
| Journey Profiles `/journey-profiles` | Check Models → Flow Models | 改名、保留版本／checksum／來源與唯讀 detail |
| Pattern Library `/patterns` | Check Models → Global Checks／model-bound rules | 相同 Expectation 不得再與 Journey anomaly 重複定義 |
| Investigation Pattern Analysis | Event Check → Findings；Case 保存 Snapshot | 不再以「未指定就執行所有 ACTIVE Patterns」作為主要使用流程 |
| Saved Searches | Event Check 查詢捷徑 | 保留 typed bounded query，不保存任意 URL、payload 或 SQL |
| Timeline Evidence Attachment | Event Check → Cases | 建立 Case 時保存 Snapshot 與 Evidence references |
| Investigation Cases | 獨立保留 | 接受 Event Check Snapshot、Findings、Journey／Flow refs 與人工調查狀態 |
| Ingestion Issues | 問題與調查分組 | 維持 ingestion admission failure，不能與業務流程偏離混為一談 |

舊網址採 compatibility redirect／adapter，不直接刪除，避免 Saved Search、Case evidence、README 與外部書籤
失效。只有能保留 typed identifier、bounded time window、Model 與 tab 的網址才自動轉換；無法無損映射的
廣泛探索條件必須留在 Legacy Event Explorer 並清楚標示相容狀態。

## Model 測試與發布門檻

新增或修改 Model 時，除了 YAML Schema 與 generated registry drift check，每條宣告路徑都必須有 deterministic
fixture。最低案例集包括：

- success；
- expected failure 或 compensated；
- in progress；
- reminder；
- violation；
- late satisfied；
- unmapped event；
- 同 event type 合理重複與重複 event ID；
- 跨 Correlation 且有明確關聯的子流程；
- source unhealthy 導致 inconclusive；
- 互斥 terminal outcomes 導致 ambiguous。

發布後的版本不可修改；新版本必須能對固定 fixtures 產生可審查的結果差異。Frontend 不執行自己的狀態
推導，Summary、Flow、Findings 與 Snapshot 均以後端 deterministic evaluation contract 為準。

## 本階段範圍

### 必須完成

1. 先定義 Check Model、Scope、Evaluation、Snapshot 與 Case handoff contracts。
2. 建立 immutable YAML／Git Registry 與 model selection contract。
3. 將重複的 Journey anomaly 與 Pattern condition 收斂為單一 Expectation source。
4. 提供 query-time evaluation、Model 推薦／切換與三維結果語意。
5. 整併 Event Check／Check Models UI，並保留舊 route compatibility。
6. 補齊 backend Karate、frontend component/browser 與 fixture regression。

### 明確延後

- 持續掃描所有業務實例；
- Reminder 主動通知；
- 自動 Finding 去重／重開；
- 高嚴重度 Finding 自動建立 Case；
- runtime Model CRUD／審核 UI；
- 模糊關聯或 LLM 推論；
- production command、retry 或 compensation execution。
- configurable DuckDB／Parquet Evidence Archive 與進階 retention／masking 治理（`EH-ECM-007`，非阻擋）。

## 建議遷移順序

| 階段 | 內容 | 完成條件 |
|---|---|---|
| D0 Design | ADR-005～007、API sequence、logical DB model、evaluation/save consistency | 架構與資料邊界核准後才開始 executable contract |
| M0 Contract | Check Model schema、result vocabulary、scope provenance、Snapshot contract | Schema fixtures 與 contract validation 通過 |
| M1 Domain | Scope Resolver、candidate selection、Flow／Expectation evaluator | unit tests 覆蓋所有固定狀態與路徑 |
| M2 Persistence/API | Check response、optional Snapshot、Case handoff | OpenAPI、migration、RBAC、restart persistence 通過 |
| M3 UI | Event Check tabs、Check Models registry、分組 navigation | component tests、可存取性與 bounded URL state 通過 |
| M4 Compatibility | 舊 route redirect、Saved Search／Case deep-link migration | 舊 deep links 與既有 Phase 1／1.1 E2E 不退化 |
| M5 Acceptance | 多 ID、跨 correlation、late event、custom scope、Case snapshot E2E | 新增 backend／frontend E2E 全數通過 |
| M6 Optional Archive | Retention Profiles、DuckDB query/export、encrypted Parquet、archive lifecycle | feature flag 預設關閉且 archive security／recovery／purge 驗收通過 |

M0～M5 已完成。Journey／Pattern 的 bounded legacy read paths 保留為 compatibility adapter；舊 API 與
physical schema 的 removal 版本必須另行決定，不因 canonical UI cutover 而立即刪除。

## 驗收原則

1. 使用者能由任一受支援 ID 找到有界事件集合，並看懂每筆事件為何被納入。
2. 系統推薦 Model 時顯示可解釋理由；多候選時不靜默猜測。
3. 合理失敗／補償路徑可以是 `CONFORMANT`，不再與系統異常混淆。
4. Reminder、Violation、Late satisfied 與 Inconclusive 有不同且可重現的時間語意。
5. 同一條缺漏規則只有一個 authoring source，不在 Journey 與 Pattern 各自複製。
6. 保存或建案後可用 Model version、checksum、`as_of` 與 Evidence refs 重現當時判斷。
7. 新功能不發送 production command，也不依賴 LLM 或不可解釋的 fuzzy correlation。

## EH-ECM-006 封版證據

- Backend Karate：19 features、126/126 scenarios。
- Frontend Karate：26/26 browser scenarios；frontend component tests：105/105。
- Check Model contract fixtures：18/18，含跨 Correlation child flow 與 partial-source `INCONCLUSIVE`。
- 完全無法讀取 ClickHouse 時回 `503 EVENT_CHECK_SOURCE_UNAVAILABLE`，不產生假的 `VIOLATED`；恢復後
  evaluation hash 與故障前一致。
- Check Snapshot 與 Investigation link 經 PostgreSQL／API restart 後逐欄一致。
- `go test ./...`、`go vet ./...`、gofmt、typecheck、lint、format、production build、OpenAPI／Registry drift
  與 contract validation 均通過。
