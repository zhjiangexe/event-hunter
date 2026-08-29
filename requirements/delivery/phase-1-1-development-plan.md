---
document_id: EH-DOC-DEL-002
status: in_progress
owner: product
last_reviewed: 2026-08-29
source_of_truth: true
canonical_topic: phase-1-1-delivery
supersedes: []
---

# Event Hunter Phase 1.1 產品化開發計畫

## 1. 文件狀態

- 狀態：`in_progress`（`2026-08-24` 已完成並簽核 **Phase 1.1 本機 release baseline**，`2026-08-28`
  已完成產品 UX remediation 與 **single-host internal staging／pilot hardening profile**；目前只剩目標環境
  production hardening、正式身分、HA／DR 與治理決策。
  baseline 證據見 `phase-1-1-sign-off.md`，剩餘工作以第 12、13 節及 `implementation-plan.yaml` 為準）
- 前置基準：Phase 1 本機 Exit Gate 已通過（本地驗證日誌：`2026-08-22`）。
  本次不使用 hosted CI；若未來改採 GitHub 自動化，會以本文件附加「Hosted CI 補驗」為附加 gate。
- 目的：在不啟動 Phase 2 Projection Rebuild 或 Phase 3 Sandbox Behavioral Replay 的前提下，
  將 Phase 1 從可用的事件調查工具提升成日常營運與工程團隊可共同使用的調查工作台。
- Scope 規則：本文件目前是候選開發計畫，不會自行擴張 `requirements/product/project-scope.yaml` 的正式需求。
  開始任一新增使用者能力前，必須先核准範圍，再新增穩定的 `REQ-EH-*`，並同步更新
  `requirements/governance/traceability.yaml`、`requirements/delivery/implementation-plan.yaml`、OpenAPI、資料契約與
  Karate acceptance feature。

### 2026-08-29 canonical 改版附錄

`EH-ECM-000`～`EH-ECM-006` 完成後，產品入口已收斂為 Event Check、Saved Results、Check Models、
Investigation Cases、Ingestion Issues、Scenario Lab 與 Guide。下文 Wave A／B／C 中的 Business Timeline、
Business Journey、Journey Profile、Pattern Library 名稱是當時的交付歷史；只有標示 compatibility 的舊
route／API 仍可使用，不得再由這些段落建立新的 authoring source 或主要 UI。

目前剩餘產品化工作只有目標環境 production hardening、正式身分、HA／DR 與資料治理決策；canonical
功能完成狀態以 `implementation-plan.yaml`、產品範圍以 `project-scope.yaml` 為準。

## 2. 產品定位

Event Hunter 的資料與鑑識核心是 **event-centric**：事件是跨服務、可排序、可追蹤且可保存 checksum
的事實來源；Event Check、Check Model、Snapshot／Evidence 與 OpenTelemetry trace/log 都以事件識別碼及
關聯識別碼串接。

但是，**底層以事件為證據，不代表使用者只能從事件角度操作**。同一批 canonical events 可以提供多種
不互相衝突的入口：

| 視角 | 使用者最先提出的問題 | 適合對象 | Event Hunter 的呈現方式 |
|---|---|---|---|
| Event-centric | 這個 event 發生了什麼？前後有哪些 event？ | 平台／後端工程師 | Event Check Timeline、event detail、Kafka coordinates、trace/log deep links |
| Order／Aggregate-centric | 這張訂單目前走到哪裡？缺了哪個合理步驟？ | 客服、營運、領域工程師 | Event Check + 適用的 Flow Model |
| Transaction／Flow-centric | 從下單到付款、出貨，整段流程在哪裡中斷？ | 營運、SRE | Flow result、Expectations、Findings |
| Investigation-centric | 哪些問題要先處理？誰負責？目前結論是什麼？ | Incident commander、支援團隊 | Case queue、owner、notes、severity、SLA、Evidence |
| Service-centric | 哪個 producer、consumer 或版本最常出錯？ | 服務 owner、SRE | Ingestion Issues、quality aggregates 與 Grafana deep links |
| Outcome-centric | 有多少付款成功卻未出貨？影響多少訂單？ | 產品、營運主管 | Overview KPI、異常分布與趨勢 |
| Quality／SLO-centric | 資料完整嗎？延遲、重複、DLQ 是否惡化？ | 平台團隊 | Data freshness、quality aggregates、Grafana dashboard |

建議產品策略是：

> 事件作為可信證據層；Event Check／Flow Model 作為主要探索入口；Investigation 作為協作入口；
> Service 與 Quality 視角交由聚合摘要和既有 Grafana 完成。

這樣不需要建立另一套非事件系統，也不會把 Event Hunter 變成通用 APM、Incident Management 或
Workflow 平台。

## 3. Phase 1.1 目標

1. 使用者不必先知道 Correlation ID，也能發現目前值得調查的問題。
2. 從任意已知 ID、訂單或告警快速進入有界 Event Check 與適用 Flow Model。
3. 讓案件具備最低限度的分工、紀錄、篩選與交接能力。
4. 讓 Check Finding 的判定來源、人工回饋與 fixture 覆蓋可重現。
5. 任何空結果都能區分「真的沒有事件」與「資料來源延遲或不可用」。
6. Grafana 告警、Event Check Snapshot、案件與 Evidence 形成可追蹤的調查閉環。
7. 增加真實服務失敗情境，持續驗證 Kafka、ClickHouse 與 OpenTelemetry vertical slice。
8. 完成正式 CI、安全、備份、保留與維運準備。

## 4. 明確不做

- Phase 2 隔離 Projection Rebuild。
- Phase 3 指定服務版本的 Sandbox Behavioral Replay。
- Production Redrive 或將事件重新送回正式 topic。
- 任意 SQL、任意事件注入器或 runtime Pattern 編輯器。
- Event Hunter 自建 Logs／Metrics／Traces Explore、Kafka Explorer 或通用 Quality Console。
- 取代 Grafana Alerting、On-call、Escalation 或完整 Incident Management 系統。
- 將 Temporal 用於每筆事件、Kafka routing 或 consumer retry。
- 在未核准正式身分整合前，直接把 Demo Role Session 當成 production authentication。

## 5. 工作流與優先順序

### P1.1-00：Phase 1 正式封版

**目的**：先建立可信基準，避免新增功能掩蓋尚未正式簽核的 Phase 1。

交付：

- 本機 `scripts/test-phase-1-exit.sh` 第一次完整成功執行並產生 pass summary（含
  `build/reports/phase-1-exit-summary.json`、Backend/Frontend Karate report、效能報告與 restart 驗證）。
- 重新核定 `implementation-plan.yaml` 所有 Phase 1 任務狀態與 dependency DAG；
  將「Hosted CI」從本輪強制條件改為「後續可選補驗」。
- 完成 Phase 1 sign-off（本輪以本機 Exit Gate + 對應 artifacts 作為 sign-off）。
- 清理 README／requirements 內已過期或彼此矛盾的待辦敘述。
- 可選補強：納入 Staticcheck、lint 與 `govulncheck`，若不納入則在 sign-off 備註中記錄風險接受決策。

驗收：本機 Exit Gate 使用一致契約與 acceptance baseline 通過，且沒有未解釋的狀態漂移；
後續若啟用 hosted CI，需再做一次等價 pass。

### P1.1-01：真實 Overview 與調查佇列

**目的**：讓使用者一進入系統就知道目前發生什麼、該先處理什麼。

交付：

- 真實 aggregate API，不在前端計算或填入 mock count。
- Open／Investigating／Closed 案件數與 severity 分布。
- 最近 72 小時新增、結案、Grafana 告警與 Scenario Lab 結果。
- 最常命中的 Pattern、producer 與 event type。
- 最近資料時間、資料來源健康及 partial warning。
- 所有卡片都能 deep link 到帶入條件的 Timeline、Investigation、Pattern 或 Grafana 頁面。

驗收：固定資料集的 API、UI 與 Karate 數值一致；來源不可用時不得顯示誤導性的零。

### P1.1-02：案件協作能力

**目的**：讓 Investigation 從單人查閱變成可交接的最小工作單位。

交付範圍：

- Assignee／owner、tags、priority 與來源類型。
- Internal notes 與 append-only audit entries。
- Related investigations 與相同 correlation/aggregate 的關聯提示。
- 案件持續時間、SLA 狀態、最近更新人與最近更新時間。
- 列表排序、複合篩選，以及將 Timeline 選取結果附加到既有案件。
- 匯出現有 JSON Investigation Summary／Evidence manifest；不新增 ZIP、PDF、raw log 或 raw trace。

P1.1-02-01 已交付 owner（沿用 `assignee`）、P0～P3 priority、tags、related correlation IDs、
自動 SLA、最近更新人及 append-only notes。Related investigations 提示、Timeline 選取結果加入既有
Timeline 選案的同 correlation 優先排序由 P1.1-02-02 交付；更完整案件列表複合排序保留在後續切片，
不視為 P1.1-02-01 的缺漏。精確 invariant、限制與
SLA 時窗以 `requirements/contracts/case-collaboration-contract.md` 為準。

驗收：Viewer／Investigator／Admin 的可見操作和後端授權一致；所有 mutable update 使用
`lock_version`／`If-Match`，notes 與 Evidence 保持 append-only。

### P1.1-03：智慧搜尋、Saved Search 與 Business Journey

**目的**：降低「必須先知道 Correlation ID」的使用門檻，提供 Order／Journey-centric 入口。

交付：

- 單一輸入框辨識 Event ID、Trace ID、Correlation ID、Aggregate ID 或 Grafana alert fingerprint。
- 無法唯一判斷時回傳候選類型，不以猜測結果直接查詢。
- 儲存常用查詢、最近查詢、可分享且有 bounded time 的 URL state。
- 一鍵查詢常用情境，例如付款失敗、付款完成但未出貨、重複事件與 DLQ。
- Business Journey 將同一 correlation/aggregate 的事件依業務 milestone 分組，顯示
  expected/actual、跨服務 duration、缺漏與異常 Pattern。
- 從 event detail 一鍵切換同 aggregate、trace、causation chain 或 Investigation。

驗收：所有查詢仍使用 allowlisted filters、bounded time、result limit、timeout 與 masking；
Business Journey 是現有事件的唯讀組織方式，不寫入或重建 production projection。
第一版採固定物流 Order profile，完整狀態以 `ShipmentDelivered` 為 completion anchor；
只有 `ShipmentCreated` 時仍為 `IN_PROGRESS`，避免將「建立出貨」誤認為「已送達」。

### P1.1-04：Pattern 成效與 Git-based 治理

**目的**：保留 deterministic、read-only Pattern Registry，同時知道規則是否真正有價值。

交付：

- 每個 Pattern 的命中數、最近命中、建立案件數及 Scenario 覆蓋率。
- Investigator 可對 finding 標記 confirmed／false-positive／needs-review；標記不修改規則本身。
- Pattern YAML 範本、schema validation、generator drift check 與 fixture regression。
- Pull request 中以固定 fixture 驗證新增／修改 Pattern 的預期命中差異。
- Pattern 版本、checksum 與變更來源可由 UI 唯讀查看。

驗收：Pattern 判斷仍由版本化 YAML 生成的 Go registry 執行；不得加入 LLM、runtime CRUD 或任意 SQL。

### P1.1-05：資料可信度與 Source Health

**目的**：防止「資料來源失效」被誤判成「沒有事件」。

交付：

- ClickHouse 最新事件時間、ingestion lag、processing-attempt freshness。
- Tempo、Loki、Grafana、PostgreSQL、ClickHouse 的 source availability 與最後成功時間。
- 全站一致的 fresh／stale／partial／unavailable 呈現。
- 查詢窗口中的 ingestion gap、retention boundary 與 truncation 說明。
- Live SDK、Scenario Lab 及 synthetic fixture telemetry 的明確來源標示。

驗收：source timeout、部分失敗、restart 與 retention boundary 均有 backend 與 browser E2E；
不可因來源失效回傳看似完整的空結果。

### P1.1-06：Grafana Alert 到 Investigation 的調查閉環

**目的**：在既有 signed webhook 邊界內，減少告警後的人工重複操作。

現況基準：

- 已完成 signed webhook endpoint、HMAC／timestamp replay protection、receipt persistence、dedup、
  建立／連結案件、Evidence 與 resolved 不自動結案。
- 現有 Backend E2E 直接送出簽章 fixture，並非由 Grafana notification pipeline 實際送達。
- 原有六條 provisioned Event Quality rules 是 aggregate alerts；它們沒有
  `event_hunter=investigate` 或單一 `correlation_id`，因此不會自然觸發自動建案。
- 2026-08-24 的 P1.1-06-02 已另行 provision correlation-aware terminal DLQ rule、HMAC Contact Point
  與 Notification Policy；aggregate rules 的邊界保持不變。

自動建案資格：

- Webhook status 必須是 `firing`。
- severity 必須是 `HIGH` 或 `CRITICAL`。
- alert label 必須包含 `event_hunter=investigate`。
- 必須能提供真實且非空的單一 `correlation_id`。
- 同 correlation 已有未結案件時必須附加 receipt／Evidence，不建立重複案件。
- 相同 org、group、fingerprint、status 的通知必須維持 idempotent。
- `resolved` 只記錄 receipt／Evidence，不自動結案。

告警邊界：

- Business Alert 可定位單一業務流程時，才允許自動建立 Investigation。
- Consumer lag、整體 duplicate rate、整體 ingestion delay、schema violation count 等 Aggregate Quality
  Alert 預設留在 Grafana／Platform Incident，不得使用 `PLATFORM` 等假 correlation ID 建立業務案件。
- 若 aggregate alert 後續能可靠解析出受影響 correlations，必須逐一套用 dedup／severity／權限政策，
  而不是將整批影響塞入一個假的 Business Timeline。

已交付（P1.1-06-02）：

- Provision Event Hunter signed webhook Contact Point，Secret 由環境／Secret Manager 注入。
- Provision Notification Policy，只路由明確標記 `event_hunter=investigate` 的 Business Alerts。
- 建立至少一條能從查詢結果帶出真實單一 `correlation_id` 的 Business Alert rule。
- Alert rule 可映射一個預設 Pattern；建案後由 application use case 觸發一次 deterministic analysis。
- 同 correlation 的重複告警附加到既有未結案件，不重複建案或寫入相同 receipt。
- 案件 Audit 顯示 firing／resolved；resolved 仍不得自動結案。
- 建案與 Pattern 完成後更新 Investigation Summary。

驗收：以實際 provisioned Grafana firing 驗證 Contact Point → Notification Policy → signed webhook →
自動建案／連結案件 → Evidence → Alerting deep link；驗證 duplicate、resolved、restart persistence 與
不合格 labels／severity／缺少 correlation 的 ignored 結果。維持 HMAC、timestamp replay protection、
idempotency 與 transaction boundary；不新增通用通知路由、On-call 或 escalation 功能。

實作證據：`event-hunter-dlq-investigation` 以 Reduce／Threshold expressions 保留 ClickHouse 查詢的
`correlation_id` label；`e2e/backend/grafana-auto-case.feature` 不直接呼叫 webhook，已實打 firing 建案、
Alerting Evidence、後續 resolved Evidence 與不自動結案。Provisioning smoke 另驗證 secret 回傳為遮罩值、
七條 rules 均 healthy 且 provenance 為 file。

### P1.1-07：真實服務 Failure Scenario 擴充

**目的**：讓更多情境自然經過 order/payment/shipping services，而不是只驗證事件注入。

候選情境：

- Payment rejected。
- Payment timeout and retry。
- Duplicate `OrderCreated`／`PaymentCompleted`。
- Shipping transient failure。
- Consumer processing 中途重啟。
- Outbox／Debezium 短暫中斷後恢復。
- Out-of-order candidate 與 transport redelivery。

交付限制：

- Fault control 只可用於 demo services／Scenario Lab 的受控本機與 CI profile。
- Scenario expected result 固定，但 PASS／FAIL 必須來自 Kafka、ClickHouse、Tempo、Loki、Prometheus
  與 service state 的實際回查。
- 不得向 live-service production topic 發送不受控 deterministic injection。

驗收：每個 live scenario 都驗證 canonical envelope、trace propagation、service logs、processing attempt、
重啟或 retry 結果，以及 Event Hunter deep links。

### P1.1-08：Production Hardening

**目的**：把可展示的本機系統提升為可評估部署的服務。

交付：

- 核心 Domain Unit Test 與 PostgreSQL／ClickHouse integration tests。
- TLS、Secret injection、backup／restore、retention、容量與 disaster recovery runbook。
- Migration forward／rollback policy，以及 Compose／部署版本固定與 restart verification。
- PII masking、Audit retention、Evidence retention 與刪除政策。
- OIDC／SSO、正式 user/role source 與完整 RBAC 另列決策；未核准前保持 deferred。
- 依實際部署風險執行 dependency、container image 與 Go／Node vulnerability scan。

驗收：在 staging 執行 install、upgrade、restart、backup、restore、secret rotation 與權限 smoke test，
並保留可追蹤報告。

## 6. 建議里程碑

| 里程碑 | 工作包 | 可交付價值 | 建議順序 |
|---|---|---|---|
| M0 Baseline | P1.1-00 | Phase 1 正式可重現、可簽核 | 第一優先 |
| M1 Discover | P1.1-01、P1.1-03、P1.1-05 | 不需先知道 ID，也能找到問題並信任結果 | 第二優先 |
| M2 Collaborate | P1.1-02 | 案件可分工、交接與追蹤 | 第三優先 |
| M3 Improve Detection | P1.1-04、P1.1-06 | 告警、Pattern 與案件形成回饋閉環 | 第四優先 |
| M4 Confidence | P1.1-07、P1.1-08 | 增加真實回歸覆蓋並具備部署準備 | 第五優先 |

若只能先選三個產品功能，建議依序實作：

1. 真實 Overview 與調查佇列。
2. 智慧搜尋與 Business Journey。
3. 案件協作能力。

## 7. 依賴原則

```text
P1.1-00 Phase 1 baseline
├── P1.1-01 Overview ───────┐
├── P1.1-03 Search/Journey ─┼── P1.1-02 Case collaboration
├── P1.1-05 Source Health ──┘
├── P1.1-04 Pattern metrics ─── P1.1-06 Alert investigation loop
└── P1.1-07 Live scenarios ──── P1.1-08 Production hardening
```

- Overview 與 Business Journey 必須先定義 query/read-model contract，再做 UI。
- Source Health 必須沿用現有 backend source status 與 Grafana deep links，不建立第二套 observability backend。
- Case collaboration 在 Phase 1.1 沿用既有 `assignee` 字串作 owner；actor 取 demo session subject。
  正式 OIDC user/team directory 與 assignee referential integrity 延後到 production identity 工作包。
- Pattern feedback 與 Pattern definition 分離：使用者回饋可以寫入 PostgreSQL，正式規則仍透過 Git 變更。
- Phase 1.1 不依賴 Temporal；若未來真的出現長時間、需人工核准且可恢復的工作，再另行核准
  `EH-MVP-010`。

## 8. 每個工作包的 Definition of Done

1. Scope 與穩定 Requirement ID 已核准。
2. OpenAPI／event／data contract 先於 implementation 完成。
3. PostgreSQL mutable state 使用 `lock_version`；append-only state 使用唯一鍵或 idempotency key。
4. Frontend 使用 generated OpenAPI client，不建立重複 DTO。
5. Viewer／Investigator／Admin 的 UI 與 backend authorization 一致。
6. 有正常、空結果、partial、timeout、unauthorized、conflict 與 restart persistence 測試。
7. Backend Go tests、Frontend Vitest、Karate backend/browser E2E、contract drift check 通過。
8. 需要效能基準的 API 納入既有 CI performance profile。
9. 文件、traceability、implementation DAG 與實際程式狀態同步。
10. 不依賴 fixture 假造 live telemetry 或產品統計。

## 9. 2026-08-24 全面合理性檢查與封版決策

本輪同時檢查 backend domain/application/repository/HTTP 邊界、frontend route/state/UX、Kafka ingestion、
Scenario Lab、OpenTelemetry/Grafana deep links、契約 drift 與現有 E2E。當時的檢查發現數個會讓
「畫面看起來健康，但真實資料鏈已中斷」的回歸與產品化缺口，因此重新開啟封版 gate；下列問題現已由
EH-P1.1-007～012 修正並於同日完成 sign-off。

| 等級 | 檢查結果 | 封版處置 | 工程任務 |
|---|---|---|---|
| P0 | S8 已送出三個 Kafka events，但 `event-hunter-forensics-ingestion-v1` 無 active member、lag 為 3；Scenario 最終 `TIMED_OUT`，HTTP `/ready` 仍回 healthy | readiness 必須包含真實 ingestion capability；修復 consumer 自動恢復並補 disruptive E2E | EH-P1.1-007 |
| P1 | `include_payload` 契約要求額外權限，但目前讀取權限即可要求 payload；masking 只處理 top-level 欄位 | 在 API 邊界強制權限並實作遞迴 masking | EH-P1.1-009 |
| P1 | Investigation detail/summary 忽略 findings、evidence、notes 子查詢錯誤，可能把不完整資料呈現為完整 | 回傳明確 failure/partial，不得吞錯 | EH-P1.1-008 |
| P1 | 建案、更新、結案、note、evidence、Pattern analysis 的 audit 寫入錯誤可能被忽略 | mutation 與 audit 同一可靠 transaction；audit 失敗不得留下未稽核變更 | EH-P1.1-008 |
| P1 | Investigation 前端仍有固定 demo 時窗、root cause、resolution，且列表選案未同步 URL | 改為 API/使用者驅動狀態，完成可分享且 refresh-safe 的 detail route | EH-P1.1-010 |
| P1 | `/investigations/{id}` 沒有被 router 正確處理，會落入 Timeline | 實作 route ↔ drawer 同步與 back/forward E2E | EH-P1.1-010 |
| P2 | 390px viewport 出現 document-level 水平 overflow；drawer/modal 缺 dialog semantics、focus trap、Escape 與 focus restore | 納入 release quality gate 的 responsive/accessibility 驗收 | EH-P1.1-012 |
| P2 | Pattern analysis 使用 wall-clock `now - 7d`，歷史案件可能無提示地 no-match | 改用可重現的案件/事件錨點並揭露 effective window | EH-P1.1-011 |
| P1 | `scenario-api:check` 因生成格式 drift 失敗，會阻擋正式 gate | 固定 generator/check 輸出並納入一致性驗收 | EH-P1.1-012 |
| P1 | `/ready` 未反映依賴與 consumer 狀態、API 缺 graceful shutdown；E2E 資料累積造成 dashboard 汙染 | 加入 dependency-aware readiness、shutdown flush、測試隔離/清理與 content-type 一致性 | EH-P1.1-007、EH-P1.1-012 |

本輪已通過的基準：Go tests、`go vet`、Frontend 42/42、typecheck、production build、contract validation、
非基礎設施 Backend Karate 61/61、Grafana assets/deep links 5/5，主要頁面瀏覽器檢查無 console error。
未通過：完整 backend E2E 卡在 pipeline readiness；Frontend Karate 11/12，失敗項為 Scenario Lab live ingestion。
這些成功項目可作為修復後的 non-regression baseline，但不能抵銷 P0/P1 問題。

封版規則：

1. P0 與 P1 必須全部完成，且 EH-P1.1-012 的整體 gate 成功後才能 Phase 1.1 sign-off。
2. P2 responsive/accessibility 問題納入 EH-P1.1-012；若要延期，必須有明確風險接受紀錄，不能默默略過。
3. EH-P1.1-001～006 的 `completed` 代表歷史切片曾驗收，不代表目前 release 可簽核；回歸由
   EH-P1.1-007～012 追蹤，避免竄改歷史紀錄或產生重複需求。
4. 本輪問題都屬既有 `REQ-EH-*` 的正確性與產品化修正，不新增 scope；工程 DAG 以
   `requirements/delivery/implementation-plan.yaml` 為準。

### 封版結果（2026-08-24）

- `scripts/test-phase-1-exit.sh --no-start` 完整通過，summary 狀態為 `passed`。
- Go test/vet、契約驗證、兩份 generated-client drift check、Frontend 49/49、typecheck 與 production build 通過。
- Backend Karate 16 features／92 scenarios 與 Frontend Karate 17/17 通過。
- Live OTel 驗證 order／payment／shipping 共用同一 trace，Tempo、Loki 與 ClickHouse 可交叉核對；
  ingestion outage/recovery、sink acknowledgement、quality failure mode 與 restart persistence 均通過。
  2026-08-28 再以 [Live Event Observability Contract](../contracts/live-event-observability-contract.md) 固定事件發布前、
  outbox commit 後、失敗與 consumer completed 的具名 log 語意及可執行驗收。
- 10 萬 canonical events 效能資料集的 200 次查詢零 HTTP error；Timeline p95 25.89ms、
  Investigation Summary p95 72.20ms，均低於 gate 門檻。
- Gate 結束後清除本輪 `[E2E]` 案件、Scenario runs、Grafana receipts 與相關資料，再還原互動式 fixtures；
  控制面確認沒有本輪 E2E 資料殘留。

## 10. 開始實作前的決策點

以下決策會改變 API 與資料模型，應在工作包啟動前逐一確認：

1. Phase 1.1 的主要使用者是客服／營運、工程師，或兩者皆是？
2. 首頁主要 KPI 以 Investigation、Order outcome 還是 Event quality 為優先？
3. **已修訂（2026-08-23）**：Business Journey 第一版只啟用固定物流 Order profile，但 profile 已改為
   Git-managed YAML，milestone、狀態與 anomaly 規則可經審查後版本化；目前不提供 runtime 編輯。啟用
   第二份 profile 前必須先定義 selection contract，避免只靠 correlation ID 猜測其他領域語意。
4. **已決策（2026-08-22）**：Assignee 先使用正規化字串／team owner，異動人取 session subject；
   不讓 OIDC 阻擋協作最小模型，正式 identity reference 待 production OIDC 核准後另做 migration。
5. **已決策（2026-08-22）**：第一版 Saved Search 是依 session subject 隔離的個人資料；Viewer、Investigator、Admin 都可管理自己的搜尋。常用情境由後端版本化為全域唯讀 preset，不提供 runtime 編輯或角色共享。
6. Pattern false-positive feedback 是否需要 reviewer 與狀態轉移？
7. 正式部署的 retention、PII、SSO、備份與 RTO／RPO 要求為何？

未完成以上決策前，可以先執行 P1.1-00；其餘工作包應先完成對應契約，不應直接從 UI 開始。

## 11. Phase 1.1 建議啟動清單（可直接執行）

### A. 即時啟動（P1.1-01 / P1.1-03 / P1.1-05）

1. 先補「可發現」：補齊 Overview / Search / Source Health 的 API 與前端入口（以非阻塞方式上線）。
2. 將「結果可信度」先做好：統一展示 source state（fresh / stale / partial / unavailable）、空結果原因與 timeout 友善訊息。
3. 驗證方式：先跑 `scripts/test-phase-1-exit.sh --no-start --skip-disruptive`，再補
   `Backend` + `Frontend` 對應 feature 的回歸。

### B. 第一波收斂（P1.1-02 / P1.1-04 / P1.1-06）

1. 先做案件協作最小模型（owner / notes / SLA / evidence 追加）與 Pattern metrics 基礎欄位。
2. 先做 Alerting 來源清單（只建立「可識別且可對應到單一 correlation」的告警建案條件）。
3. 驗證方式：跑 `scripts/test-phase-1-exit.sh` 的完整指令 + 專項 `e2e/backend/grafana-alert-webhook.feature`。

### C. 第三波產品化（P1.1-07 / P1.1-08）

1. 先接上 live failure scenario 的可回歸組合（S1~S6 可先做一組主線）。
2. 同步完成 deployment / backup / 恢復 / secret / 可運維文件（不需要一次完備，但必須有可執行 runbook）。
3. 驗證方式：在每批上線後保留 restart persistence、quality window、Grafana provisioning 與 deep-link 實打開證據。

建議每波皆以「contract + backend test + frontend test + E2E」四件套作為結束條件；下一波才能銜接。

## 12. P1.1 現場執行 Backlog（可交付任務清單）

以下以「3 波」排程，搭配可驗收輸出；若你只想看本週要做的，先做 1～5。

### Wave A（立刻可開工）：可見性優先

1. **P1.1-01-01：建立 Overview 聚合摘要 API**

- 狀態：`completed`（2026-08-22）

- 目標：`/api/v1/investigations/overview`（或對應路由）提供固定欄位，僅含可追溯資料（非前端推算）
- 內容：Open / Investigating / Closed、severity 分布、近 72h 新增/結案、Grafana 告警、Scenario Lab 近期結果、來源健康摘要
- 依據：`requirements/governance/traceability.yaml`（新增或綁定 REQ-EH-*）、OpenAPI、service contract、DB query budget
- 驗收：固定資料集下 UI 與 E2E 數值一致；資料不可用時回傳 partial reason，不顯示誤導性 0
- 測試：新增/更新 backend integration + `e2e/backend/investigation-overview.feature`（新建）

2. **P1.1-01-02：前端 `/dashboard`/總覽頁整合（最小可用）**

- 狀態：`completed`（2026-08-22）

- 目標：展示 P1.1-01 聚合摘要卡片，避免重複 mock 數字
- 內容：卡片與 deep link 到 timeline/investigation/pattern/grafana；資料缺口顯示「不可用」
- 驗收：卡片欄位能在 1 次載入完成，路由/連結測試通過
- 測試：新增/更新 frontend component + karate ui 測試

3. **P1.1-03-01：Smart Search 輸入解析器（只做預先驗證 + 候選回傳）**

- 狀態：`completed`（2026-08-22）

- 目標：一個輸入框可辨識 event/trace/correlation/aggregate/alert fingerprint，不做模糊猜測查詢
- 內容：先回傳候選類型與明確下一步；支援空白、過短、失效格式提示
- 驗收：invalid input 不進查詢、可識別格式可展開導向
- 測試：新增 backend 搜尋解析 contract 測試 + UI 表單互動測試

4. **P1.1-03-02：唯讀 Business Journey**

- 狀態：`completed`（2026-08-22）

- 目標：將同一 correlation 的 canonical events 依物流 milestone 組織，降低直接閱讀事件表的門檻
- 內容：Order／Payment／Shipping／Delivery／Return expected/actual、跨里程碑 duration、確定性缺漏提示；Correlation Smart Search 直接導入 Journey
- Profile：`contracts/journeys/order-fulfillment.yaml` 是 authoring source，generated Go registry 是 runtime source；API 顯示 profile ID／version，`/journey-profiles` 唯讀列表呈現可比較摘要，點選後以 routeable 右側抽屜查看來源、checksum、milestones 與 anomaly rules
- 完成條件：`ShipmentDelivered`；只有 `ShipmentCreated` 時維持 `IN_PROGRESS`
- 邊界：只讀 ClickHouse、最多七天與 1000 events；不建立 projection、不重送事件、不由前端推導狀態
- 驗收：`e2e/backend/business-journey.feature` 覆蓋 Journey 與 profile catalog、`e2e/frontend/investigation-flow.feature` 覆蓋 Registry 導覽，並以瀏覽器驗證 Journey → Timeline event link

5. **P1.1-03-03：個人 Saved Search 與唯讀常用 Preset**

- 狀態：`completed`（2026-08-22）

- 目標：讓使用者命名並重播 Timeline／Journey 的 bounded allowlist 查詢，不必反覆輸入相同條件
- 內容：以 session subject 隔離的個人 list/create/delete、`saved_searches` JSONB query state、4 個後端版本化唯讀常用情境，以及 Timeline／Journey 共用的「查詢捷徑」右側抽屜；舊 `/saved-searches` 路徑只保留相容導向
- 權限：Viewer 可管理自己的 Saved Search，但不因此取得 Investigation mutation；其他 principal 無法列出或刪除
- 時間語意：`ABSOLUTE` 保存固定 `from/to`；`RELATIVE` 保存 1 小時、24 小時、72 小時或 7 天視窗並由後端在每次 list/create 時重算 `open_url`。相對模式仍保存建立當下的 `from/to` 作稽核快照與舊資料相容，不用它重開查詢
- 邊界：只保存 typed query state，由後端重建 `/timeline`／`/journey` URL；不保存 payload、任意 URL、SQL、排序或超過七天的時間窗
- 驗收：專屬 backend Karate、frontend Chrome E2E、frontend tests、Go test/vet、contract validation、typecheck/build、API container restart persistence 與 deployed browser preset → Timeline 還原皆通過

6. **P1.1-05-01：Source Health API 與 Frontend 呈現規格化**

- 狀態：`completed`（2026-08-22；source timeout／restart 的完整 disruptive suite 仍在 P1.1-08 再擴充）

- 目標：統一 `fresh/stale/partial/unavailable` 四段狀態
- 內容：ClickHouse、Tempo、Loki、Grafana、PostgreSQL 的 last success/lag/failure 原因回報
- 驗收：遇到 timeout / restart / partial 不回傳「空結果」誤判
- 測試：backend source health + browser e2e 錯誤面板測試

### Wave B（下一階段）：協作與告警收斂

7. **P1.1-02-01：案件協作欄位最小模型**

- 狀態：`completed`（2026-08-22）

- 目標：新增 Owner/Tags/Priority/Related Correlation 的只增量欄位，不改變既有權限主流程
- 內容：append-only notes、audit entry、SLA 狀態（自動計算）
- 驗收：lock_version / If-Match、Viewer/Investigator/Admin 操作一致
- 測試：更新 case lifecycle 契約 + backend + frontend mutation 測試
- 完成證據：`REQ-EH-014`、OpenAPI `addInvestigationNote`、migration `00005_case_collaboration.sql`、
  backend Karate 77/77、frontend Karate 11/11、frontend tests 40/40；PostgreSQL 與 API 重啟後以 API
  查回相同 owner、priority、tags、related correlation ID、note 與 `lock_version`。

8. **P1.1-02-02：Investigation 與 Timeline 互通**

- 狀態：`completed`（2026-08-23）

- 目標：從 timeline event 快速帶入到既有案件（不複製資料，保留只讀鏈）
- 內容：event detail 提供「加入案件」；後端以原始 bounded window 精確驗證 ClickHouse event，
  PostgreSQL 只保存 EVENT reference、SHA-256 與來源 correlation 關聯，不保存 payload
- 契約：`REQ-EH-015`、OpenAPI `attachInvestigationEvent`、
  `requirements/contracts/timeline-evidence-attachment-contract.md`
- 驗收：相同 event 重送為 idempotent no-op；新 evidence、related correlation 與 lock version 原子更新；
  Viewer／closed case 不可寫入；案件 Evidence 與 Audit 可追溯
- 測試：backend/frontend action path、完整 e2e 回歸與 PostgreSQL/API restart persistence
- 完成證據：完整 backend Karate 78/78、frontend Chrome Karate 12/12、frontend tests 42/42、
  Go test/vet、contract validation、typecheck/generated-client check/build 皆通過；PostgreSQL 與 API
  重啟後可查回相同 lock version、related correlation、EVENT reference、checksum 與 audit；實際瀏覽器
  驗證 modal 無水平 overflow，且同 correlation 標記與 priority 完整可見。

9. **P1.1-04-01：Pattern 成效度量 API + UI 欄位（只讀）**

- 狀態：`completed`（2026-08-24）

- 目標：展示每個 pattern 在固定資料窗的 hit / 最近命中 / 轉為案件數
- 內容：pattern metadata + 最近命中摘要 + 該 pattern 於 investigation links
- 驗收：無寫入，規則仍由 YAML Registry 驅動
- 測試：新增 metric schema + frontend metrics card 測試
- 完成證據：新增 `getPatternEffectiveness`，由 PostgreSQL `pattern_findings` 在 server-owned 30 天窗口
  彙總 hit、最近命中與 distinct Investigation 數；來源失敗回 503，UI 顯示不可用而非 0。
  OpenAPI／generated client／traceability 已同步；Go tests、Frontend 51/51、typecheck、lint、build、
  Backend Karate 94/94 與 Frontend Karate 17/17 通過。清理器另補 Scenario Lab 觸發 Grafana 自動建案的
  correlation-based cleanup，本輪時窗內案件殘留為 0。

9a. **P1.1-04-02：Pattern finding 回饋與 Git-based 治理補完**

- 狀態：`completed`（2026-08-24）
- 目標：補齊 confirmed／false-positive／needs-review 回饋、Scenario 覆蓋率、checksum 與變更來源
- 產品決策：本階段不加入 reviewer／approval workflow；Investigator／Admin 可直接判定，Viewer 唯讀
- 驗收：回饋不修改 runtime Pattern；YAML schema、generator drift、fixture regression 與 UI metadata 可追溯
- 完成證據：Registry 由原始 YAML 產生 source path、SHA-256 與 match／non-match fixture counts；
  finding 維持 append-only，feedback 保存於獨立 PostgreSQL table 並使用自己的 `If-Match`／lock version，
  更新與 `CLASSIFY_PATTERN_FINDING` audit 同 transaction。案件抽屜支援 CONFIRMED／FALSE_POSITIVE／
  NEEDS_REVIEW，Pattern Library 顯示治理 metadata。Go test/vet、Frontend 52/52、typecheck、lint、build、
  contract drift、Backend Karate 95/95、Frontend Karate 17/17 通過；API restart 後仍可查回 CONFIRMED v1
  與 audit，gate cleanup 後案件及 feedback 均為 0。契約見 `pattern-governance-contract.md`。

10. **P1.1-06-01：告警建案資格邊界（實務版本）**

- 狀態：`completed`（2026-08-24；不合格通知保存 receipt 與 disposition，不建立案件 Evidence）

- 目標：把現況文件化為可落地檢查清單：firing + severity + label + correlation 必要條件
- 內容：實作 `event_hunter=investigate` 入口條件 gate；缺少條件一律 ignored 並保存 receipt／disposition，
  只有已建立或連結真實案件時才保存案件 Evidence
- 驗收：duplicate / resolved / severity 邊界明確且有測試
- 測試：`e2e/backend/grafana-alert-webhook.feature` + evidence 檢查

10a. **P1.1-06-02：真實 Grafana Business Alert delivery**

- 狀態：`completed`（2026-08-24）
- 目標：讓具真實單一 correlation 的 terminal DLQ alert 自然經 Grafana 建立或連結 Investigation
- 內容：file-provisioned multi-dimensional rule、HMAC Contact Point、label-matched Notification Policy
- 驗收：firing 建案、Alerting deep link、success 後 resolved Evidence、案件不自動結案、restart 後 assets 仍在
- 測試：`scripts/verify-grafana-provisioning.sh` + `e2e/backend/grafana-auto-case.feature`

10b. **P1.1-02-03：案件列表進階排序與複合篩選**

- 狀態：`completed`（2026-08-24）
- 目標：補齊 P1.1-02 原始範圍中尚未交付的複合排序／篩選，不在前端對單頁資料假排序
- 驗收：排序與篩選由 bounded backend query 執行，URL 可分享，分頁前後結果穩定且具 E2E
- 完成證據：OpenAPI 與 generated client 已加入 `created_at`／`updated_at`、`asc`／`desc`；PostgreSQL
  read adapter 執行六種可組合篩選與 deterministic keyset sort，v2 cursor 綁定排序欄位及方向；
  `/investigations` query string 可分享並在 drawer navigation 保留。Go test/vet、Frontend 53/53、
  typecheck/lint/build、contract drift 均通過，Backend Karate 17 features／97 scenarios、Frontend Karate
  18/18 通過；gate cleanup 後案件、feedback、Scenario runs 均為 0。

### Wave C（立即封版阻擋）：可信度與穩定性修復

11. **EH-P1.1-007：Live ingestion liveness 與 restart recovery**

- 狀態：`completed`（P0；2026-08-24）
- 目標：修復 Kafka 有資料但 ingestion consumer 無 active member，API 仍回 ready 的假健康狀態
- 內容：consumer 自動恢復、Kafka group/lag liveness、dependency-aware readiness、temporary interruption
  與 restart 測試
- 驗收：新建訂單自然進入 Timeline；S8 由 actual observer 結束為 PASS；完整 backend pipeline E2E 通過
- 當時完成證據（其 ingestion runtime 後由 EH-POC-003 取代）：API `/health/ready` 同時驗證 PostgreSQL、ClickHouse、兩條 Redpanda Connect pipeline 與
  Stable/active ingestion group；disruptive test 在 Redpanda 中斷時得到 503，原 Connect container 不重建並
  自動 rejoin，S8 隨後 PASS；完整 Go test/vet、contract/Compose validation 與 Backend Karate 78/78 通過。

12. **EH-P1.1-008：Investigation read consistency 與 transactional audit**

- 狀態：`completed`（P1；2026-08-24）
- 目標：任何必要子查詢或 audit 失敗都不得被吞掉
- 內容：detail/summary partial/failure contract；case mutation、note、evidence、analysis 與 audit 的 transaction
  boundary；repository fault tests
- 驗收：子查詢失敗不回傳假完整資料；audit 失敗不留下未稽核 mutation
- 完成證據：detail/summary 改由 application snapshot 組合，findings、evidence、notes、audit 任一必要讀取失敗即
  明確失敗；PostgreSQL context-carried Unit of Work 讓建案、更新、結案、note、evidence、Pattern analysis 與
  audit 共用 transaction。application fault tests 驗證 rollback，真實 PostgreSQL integration test 以 audit role
  constraint 故障證明 case insert 一併回滾；完整 Go test/vet、contract validation 與 Backend Karate 78/78 通過。

13. **EH-P1.1-009：Payload authorization 與 recursive masking**

- 狀態：`completed`（P1；2026-08-24）
- 目標：讓 OpenAPI 的 payload 權限承諾與實作一致，並遮罩巢狀敏感欄位
- 驗收：Viewer 無法取得 payload；object/array 內的 configured secret/PII 也會遮罩；新增 role E2E
- 完成證據：Timeline、Event Search、Investigation Summary 在 ClickHouse query 前拒絕 VIEWER／INVESTIGATOR
  的 `include_payload=true`，只有具 `payload:read_sensitive` 的 ADMIN 可取得仍經遮罩的 payload；recursive
  masker 支援 object、array、camelCase/snake_case，涵蓋 customer ID、amount、credentials、token、email 與
  phone。OpenAPI、data-classification、generated client 與 UI control 已同步；防禦性 Karate fixture 直接寫入
  nested prohibited fields，驗證遮罩後同步刪除 probe。完整 Go test/vet、contract validation、Frontend 43 tests、
  typecheck/build/format 與 Backend Karate 83/83 通過。

14. **EH-P1.1-010：Investigation production navigation/state**

- 狀態：`completed`（P1；2026-08-24）
- 目標：完成 `/investigations/{id}` 真實 route 與右側 drawer 同步，移除 production 畫面的固定 demo 狀態
- 驗收：direct URL、refresh、back/forward、drawer close、close confirmation 與 API error/partial state 皆有 browser E2E
- 完成證據：案件選取以 URL 作為唯一 drawer 狀態來源，支援 direct URL、reload、browser back/forward、
  close 返回列表與明確 404；新案件的 root cause／resolution 預設為空白，結案需經必填確認 dialog。
  Summary／Evidence 不再注入固定 demo 日期，只有 URL 明確提供 `from`／`to` 時才傳入查詢窗。
  Frontend unit/component 48/48、typecheck、production build 通過；`@eh-p1-1-010` Karate 4/4 與完整
  Frontend Karate 16/16 通過，並以實際 localhost 瀏覽器確認右側 drawer、URL 與 back/forward 呈現。

15. **EH-P1.1-011：Historical Pattern analysis window**

- 狀態：`completed`（P2；結果可信度；2026-08-24）
- 目標：Pattern 使用案件/事件紀錄時間作為可重現錨點，不因測試或案件變舊而靜默 no-match
- 驗收：API 揭露 effective window；同一證據延後重跑仍得出一致結果；boundary 有測試
- 完成證據：分析服務先以案件 correlation 查詢仍在 ClickHouse retention 內的最早、最晚 canonical event，
  固定使用 `[earliest event, earliest event + 7 days)` 作為 server-owned effective window，並將
  `observed_at` 截止於 window end；API 與 UI 明確揭露 `EVALUATED`／`NO_EVENTS`、effective window 與
  source event count。無事件不再偽裝成 no-match，事件跨度超出上限則明確回傳
  `422 ANALYSIS_WINDOW_EXCEEDS_LIMIT`。完整政策記錄於 `historical-pattern-analysis-contract.md`；Go test/vet、
  OpenAPI generated client/check、Frontend 49/49、Backend Karate 92/92 與 Frontend Karate 17/17 通過。

16. **EH-P1.1-012：Release gate 與跨層品質清理**

- 狀態：`completed`（P1/P2；2026-08-24）
- 目標：收斂 mobile/accessibility、generated-client drift、graceful shutdown、content type 與 E2E data isolation
- 驗收：390px 無 document overflow；dialog keyboard 行為可用；`scenario-api:check` 與完整 Phase 1.1
  contract/backend/frontend/Karate/restart gate 全部通過；重跑測試不累積案件污染 Overview
- 完成證據：主要頁面於 390px 無 document-level overflow；drawer/modal 具 dialog semantics、初始焦點、
  focus trap、Escape 關閉與 focus restore，並有 reduced-motion/status announcement；OpenAPI 與 Event Lab
  generated clients、format check 均無 drift。API 支援 dependency-aware readiness、SIGTERM graceful shutdown
  與一致 JSON content type；完整本機 gate 通過 Backend 92/92、Frontend 17/17、live OTel、故障恢復、
  10 萬事件效能及 restart persistence。`cleanup-e2e-data.sh` 清除 gate 時窗內的隔離資料，查核結果為
  `[E2E]` 案件 0、該 gate 的 Scenario runs 0。

### Wave D（封版後續）：產品化能力擴充

17. **P1.1-07-01：Failure Scenario 補齊**

- 狀態：`completed`（2026-08-25；實際交付已擴充至 S1～S14）
- 目標：將更多 real failure 走過三服務 live path
- 內容：新增 2~4 個可回歸的 scenario 覆蓋（含重試、重複、轉儲、consumer 中斷）
- 驗收：每個情境驗證 trace/日志/attempt/ClickHouse/event pipeline 一致
- 測試：Scenario Lab actual observer + 事件時間窗斷言
- 完成證據：S1、S12～S14 走真實三服務 Outbox path，S2～S11 走隔離 Lab topic；涵蓋缺少出貨、duplicate、
  out-of-order、processing DLQ、schema violation、delay、付款失敗取消、完整配送、派車失敗重試及
  退貨退款。Backend E2E 逐一驗證 actual event/attempt/failure、trace ID、Tempo 與 Loki，完整基準 92/92。

18. **P1.1-08-01：運維 Runbook 初版**

- 狀態：`completed`（2026-08-24）
- 目標：整理啟停、restart、資料留存、secret rotation、backup/restore、故障回退步驟
- 內容：把既有腳本對應到 runbook 步驟（含預期輸出）
- 驗收：文件可由新人按步驟重跑一輪 restart persistence + 性能驗證
- 測試：文件審核 + 冒煙演練
- 完成證據：`operations-runbook.md` 定義操作分級、readiness 判讀、常見故障恢復、受控 restart、
  cold backup／restore、retention、secret rotation 與 release handoff；`verify-operations-runbook.sh`
  提供 static／唯讀 live／明確確認的 restart 三種檢查，`backup-local-volumes.sh` 只在 stack 全停時封存
  allowlist volumes 並產生 checksum，避免不一致 snapshot 或意外覆寫。

**P1.1-08-03：Single-host internal staging／pilot hardening profile**

- 狀態：`completed`（2026-08-28）
- 目標：把本機預設開發拓樸收斂成可重現、fail-closed 的內部單機試運行基線，但不冒稱 production ready
- 內容：`compose.hardening.yaml` 關閉所有非 edge host ports，只發布 Event Hunter／Grafana TLS edge；
  `.env.hardening.example`、`verify-hardening-profile.py` 強制九個長且互異的 secret、HTTPS URL、憑證路徑與
  private-key 權限；Grafana ClickHouse reader password 獨立參數化，Loki 啟用七天 retention
- 驗收：merged Compose 只存在兩個 TLS edge host ports，兩份 Nginx 設定通過 syntax check，hardening
  verifier 使用隔離測試憑證通過；internal pilot 的 synthetic-only PII 邊界、RPO 24h、RTO 4h、無 HA 與
  production 未解項均記錄於 `requirements/operations/single-host-hardening-baseline.md`

18a. **P1.1-08-02：正式部署 Hardening 與演練**

- 狀態：`pending`（需先決定部署環境、TLS／Secret、retention、PII、SSO 與 RTO／RPO）
- 目標：完成 P1.1-08 原始 Production Hardening，而不把本機 Compose smoke 誤稱為 production ready
- 內容：staging install／upgrade、TLS、secret rotation、實際 backup／restore、migration rollback、
  retention／deletion、容量與 DR 演練；OIDC／SSO／完整 RBAC 依核准決策另行納入
- 驗收：在目標 staging 留下可追蹤報告；本機 helper 或文件審閱不能取代實際演練

18b. **EH-POC-002：ClickHouse-first 功能完整度與操作安全**

- 狀態：`completed`（2026-08-27；後續由 EH-POC-003 正式採用，domain／attempt committed default 均為 clickhouse-mv）
- 目標：補齊安全 failure 可見性、technical DLQ、candidate 故障恢復與 raw landing 最小操作邊界
- 完成內容：Ingestion Issues API／UI、technical DLQ projector、真實 tombstone E2E、ClickHouse outage／
  connector+projector restart／backlog drain、7 天 TTL、default deny 與 24 小時 dry-run-first bounded purge
- 安全邊界：不在一般 read model 保存 raw payload、exception message 或 stack trace；未來 privileged raw reader
  必須另設角色、限時核准、來源限制、Secret Manager 與 query audit
- 驗收：POC Karate、functional recovery、raw purge、Backend 108/108、Frontend browser 25/25、Frontend
  unit/component 77/77、Go test/vet、contracts、typecheck、lint、format、generated client drift 與 build 通過
- 明確延後：load/soak、capacity/throughput benchmark 與高併發 duplicate/redelivery 壓力矩陣；只在永久
  production cutover 決策時恢復為 release blocker

18c. **EH-POC-003：ClickHouse-first 正式採用與 processing-attempt 替代**

- 狀態：`completed`（2026-08-27）
- 目標：讓 domain events 與 processing attempts 共用官方 ClickHouse Sink → raw → Materialized View 路線
- 完成內容：attempt raw／promoted／safe failure tables、canonical readers、fresh-start default、readiness、
  fixture mapping、restart recovery，並移除兩個 Redpanda Connect ETL services／設定／ports
- 保留元件：Redpanda Kafka-compatible broker 與 Debezium Kafka Connect；兩者不屬於被移除的 ETL worker
- 驗收：Backend 108/108、Scenario Lab 16/16、Grafana auto-case 1/1、POC 3/3、candidate-only domain +
  attempt outage/backlog recovery、contract／Compose／fixture loader 全數通過
- 明確延後：壓測、soak、capacity 與正式 raw-data governance，不阻擋本機功能完整度

### Wave E（可並行）安全與品質基礎掃描（非阻擋）

19. **P1.1-QA：靜態檢查與弱掃**

- 狀態：`completed`（2026-08-24）
- 目標：建立可重跑的 release 品質與已知漏洞掃描
- 內容：加入 govulncheck / staticcheck / lint 記錄（可先保留在 PR 補充紀錄）
- 驗收：有明確風險接受決策，並綁定 sign-off note
- 完成證據：首次 govulncheck 發現 host Go 1.26.1 標準函式庫有 19 個可達漏洞，因此將 repo toolchain、
  Docker builder 與 toolchain policy 固定至官方 1.26 系列最新 patch 1.26.7；部署後 API binary build info
  確認為 `go1.26.7`，重掃為 `No vulnerabilities found`。Staticcheck v0.8.1 清除 4 個 dead functions 與
  5 個 error-style findings；Frontend 加入 ESLint 10 flat config，修正 3 個 React/TypeScript findings，
  pnpm production/full audit 均為 0 known vulnerabilities。Compose policy 無 error，僅接受本機 Tempo
  為 volume compatibility 明確使用 `0:0`；`test-security-quality.sh` 產生可重跑 JSON report。

## 13. 建議執行節奏

- **已完成基準**：Phase 1.1 本機 release baseline、UX remediation、Wave C、P1.1-06-02、P1.1-07-01、
  P1.1-08-01、P1.1-08-03、EH-POC-002 與 Wave E 已驗收；single-host internal pilot 完成不代表
  internet-facing production ready
- **最近完成**：P1.1-04-01 Pattern 成效度量、P1.1-04-02 Pattern 回饋／Git-based 治理
- **既有核准交付範圍**：本機產品工作已完成；P1.1-02-03 已完成
- **UX remediation**：`P1.1-UX-01`～`P1.1-UX-13` 已完成；DAG 對應 `EH-P1.1-013`～
  `EH-P1.1-021`，涵蓋動態時間、案件窗口／狀態、responsive shell、Journey／Pattern 判讀、案件搜尋、
  Scenario Run History、Overview 與 Guide 一致性
- **待部署決策**：P1.1-08-02 目標環境 production Hardening 與演練；須先提供 staging、正式憑證／
  secret store、SSO、HA、資料治理及 RTO／RPO 核准，不能以 P1.1-08-03 單機基線代替
- **持續**：每次 release 重新執行 security-quality scan、runbook smoke 與完整 Exit Gate

每完成一項：

- 先更新 `implementation-plan.yaml` 對應 `status`
- 再補對應契約與測試
- 最後更新 sign-off 狀態頁（`phase-1-delivery-plan.md` / `phase-1-1-development-plan.md`）
