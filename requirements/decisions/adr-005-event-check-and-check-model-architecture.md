---
document_id: EH-ADR-005
status: adopted
owner: product
last_reviewed: 2026-08-29
source_of_truth: true
canonical_topic: event-check-and-check-model-architecture
supersedes: []
---

# ADR-005：Event Check 與 Check Model 架構

## 狀態

`adopted`。本 ADR 定義目標架構；`EH-ECM-001`～`EH-ECM-005` 的 contracts、domain、後端 API、persistence、
workspace UI 與 compatibility migration 已交付。新 `/event-check` 與 `/check-models` 是主要入口；舊
Timeline、Journey、Journey Profile、Pattern、Saved Search 與 Investigation deep links 透過無損 redirect
或清楚標示的唯讀 adapter 保持可讀。舊 runtime 只會在 `EH-ECM-006` 完成全量與 restart 驗收後移除。

## 背景

Event Hunter 現在有兩套互相接近但分離的解讀機制：

- Business Journey 以唯一 active default Journey Profile 將同一 correlation 的事件分成線性 milestones；
- Investigation Pattern Analysis 在未指定 Pattern IDs 時執行所有 active Patterns。

`PaymentCompleted` 後缺少 `ShipmentCreated` 同時存在於 Journey `anomaly_rules` 與 Pattern Library，造成
相同業務期待有兩個 authoring sources。Journey 也把 Return 當成固定第五個 milestone，無法自然表達
optional branch、child journey、合理失敗與補償路徑。

產品需求已收斂為：使用者由任一受支援 ID 找到 0～N 個有明確關聯理由的事件，以一個主要 Flow Model
加上 child models 與 Global Checks 進行唯讀符合性檢查，再視需要保存 Snapshot 或建立 Case。

## 決策

### 1. Event Hunter 是被動檢查平台

Event Hunter 只讀取 canonical events 與 source health，產生 evaluation、Snapshot、Finding、Case 與
notification intent。它不得：

- 發送 production command；
- 呼叫正式服務執行付款、出貨、取消或退款；
- 控制業務 retry／compensation；
- 成為正式流程或 aggregate state 的 source of truth。

Check Model 可以描述事件順序、分支、cardinality、Expectation、deadline 與 outcome，但不得包含任意程式碼、
HTTP／RPC action、command topic 或 workflow activity。

### 2. Journey Profile 與 Pattern Library 整合為 Check Models

Authoring 與治理整合為 versioned YAML／Git Check Model Registry：

- `FLOW`：定義 applicability、milestones、paths、child models、Expectations 與 business outcomes；
- `GLOBAL_CHECK`：定義不屬於單一流程的事件完整性、重複與 telemetry quality 規則。

Runtime 可以保留不同 evaluator，但同一條 Expectation 只有一個 authoring source。Journey view 使用它推導
等待／逾時狀態；Finding 也由同一條 Expectation 產生，不再複製 Journey anomaly 與 Pattern condition。

已發布的 Model 版本不可修改。Parent Model 必須 pin child Model ID 與 version，不接受 `latest`。

### 3. 一次 evaluation 只有一個主要 Flow Model

Scope Resolver 取得事件集合後，Candidate Resolver 依 domain、aggregate type、start／trigger event type 與
event schema version 提供候選與理由：

- 一個高可信候選：自動預選，UI 仍顯示 Model 與推薦理由；
- 多個候選：回 `MODEL_SELECTION_REQUIRED`，不得靜默猜測；
- 無候選：回 `NO_APPLICABLE_MODEL`，Timeline 仍可使用。

主要 Model 可以啟動 0～N 個 pinned child models；適用的 Global Checks 自動執行。一般使用者不逐項勾選
所有通用規則。

### 4. Scope 只使用可解釋關係

Scope Resolver 可以使用 same correlation、same aggregate、causation、受治理 business key 與 parent／child
aggregate 關係跨 correlation 收集事件。每條 edge 都保存 relation type、source field 與來源事件。

不得以時間接近、payload 相似或 LLM／機率判斷自動納入事件。平台限制為最多 7 天、10,000 events、20
correlations 與 relation depth 3；Model 只能縮小，不能放大。

### 5. Query-time evaluation 是目前唯一執行模式

一般 evaluation 不保存、不排程、不持續掃描。`as_of` 等於查詢 `to`；前端一般查詢的 `to` 預設為現在。
只有使用者明確保存或建立／加入 Case 時才產生 immutable Check Snapshot。

Reminder、Violation 與 Case suggestion 可以在 query-time 算出；持續掃描、主動通知、Finding 去重／重開與
自動建立 Case 延後，不阻擋本次交付。

### 6. 三維結果語意

Evaluation 分開回傳：

- Check status：`NO_DATA`、`IN_PROGRESS`、`CONFORMANT`、`DEVIATED`、`INCONCLUSIVE`、`AMBIGUOUS`；
- Business outcome：平台 category 加 Model 定義的 domain outcome code；
- Expectation state：`SATISFIED`、`WAITING`、`REMINDER`、`VIOLATED`、`LATE_SATISFIED`。

合理失敗與補償路徑可以是 `CONFORMANT`。缺少事件只有在 deadline 成熟且 source health 足以支持否定判斷
時才是 `VIOLATED`；否則為 `WAITING`、`REMINDER` 或 `INCONCLUSIVE`。

### 7. 相容遷移後才切換 canonical UI

目標 UI 以 Event Check 組合 Summary、Timeline、Flow、Findings 與 Cases；Check Models 組合 Flow Models、
Global Checks、Versions 與 Test Scenarios。舊 `/timeline`、`/journey`、`/journey-profiles`、`/patterns` 與
Saved Search／Case deep links 在相容驗收前保留。

新舊 evaluator 必須先用固定 fixtures 對照。只有 `EH-ECM-006` 完成後，新入口才能成為 canonical；舊
runtime 的移除需要另外明確的 deprecation／removal 決策。

## 未採用方案

### 保持 Journey 與 Pattern 完全分離

不採用。它無法消除相同 Expectation 的重複定義與結果漂移。

### 讓 producer event 攜帶 `pattern_id` 或 `model_id`

不採用。這會讓 production services 耦合 Event Hunter Registry，無法對歷史事件套用新 Model，也無法表達
「事件尚未發生」或一筆事件涉及多個檢查規則。

### 每次執行所有 active Models

不採用。跨領域擴充後會產生不相關結果、不可控成本與難以解釋的 Case findings。

### 直接採用 BPMN／Workflow Engine

不採用。Event Hunter 需要的是 observed trace 對 reference model 的 conformance checking，不需要 token、
command、retry 或正式補償執行。

### 使用 fuzzy／LLM correlation

不採用於 canonical scope。未來即使提供候選提示，也不得在沒有人工確認與 provenance 的情況下納入正式
Snapshot。

## 後果

### 正面

- 一條業務期待只有一個 authoring source；
- 正常、合理失敗、補償與真正偏離可以分開；
- Timeline、Journey、Finding 與 Case 形成同一個可解釋工作流；
- Model 仍是 Git-managed、deterministic、可回歸測試；
- 不擴大 Event Hunter 對 production systems 的權限。

### 成本

- 需要新的 Scope Resolver、Model schema、evaluator、Snapshot 與 compatibility layer；
- 現有 Journey／Pattern APIs 與 findings 需要有序遷移；
- child model aggregation、late events 與 source health 會增加測試矩陣；
- 在新 runtime 驗收完成前必須維護新舊兩條讀取路徑。

## 相關文件

- [產品需求](../product/event-check-and-check-models-requirements.md)
- [Target Design](../architecture/event-check-target-design.md)
- [ADR-006：Snapshot 與 Finding persistence](adr-006-check-snapshot-and-finding-persistence.md)
- [ADR-007：Evidence Archive](adr-007-duckdb-parquet-evidence-archive.md)
- [Implementation Plan](../delivery/implementation-plan.yaml)
