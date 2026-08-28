---
document_id: EH-DOC-HIST-001
status: completed
owner: product
last_reviewed: 2026-08-28
source_of_truth: false
canonical_topic: phase-1-delivery-history
supersedes: []
---

# Event Hunter Phase 1 交付與原型對照計畫

更新日期：2026-08-24（Phase 1.1 sign-off 後狀態校正）

## 1. 目的

本文件是 Phase 1 收尾的操作清單，讓後續 Agent 能區分：

- Phase 1 必須完成的 MVP 能力。
- 為了展示完整度而建議完成、但不阻擋 MVP 的項目。
- 只能在 Phase 2／3 開發的隔離重建與 Replay 能力。
- 明確不屬於 Event Hunter 的功能。

範圍仍以 `requirements/product/project-scope.yaml` 為準；需求映射以 `requirements/governance/traceability.yaml` 為準；工程依賴與完成狀態以 `requirements/delivery/implementation-plan.yaml` 為準；原型功能索引位於 `requirements/product/prototypes/prototype-phase-matrix.yaml`。本文件負責把它們轉成 Phase 1 可執行的收尾順序。

## 2. Phase 1 完成定義

Phase 1 只有在以下條件全部成立時才完成：

1. `REQ-EH-001`～`REQ-EH-009` 的 API、資料、UI 或外部 UI、以及 acceptance feature 均可追蹤。
2. Business Timeline 支援有界時間範圍，以及基本／進階 allowlist 條件，不接受任意 SQL。
3. Investigation 能從建立、查閱、分析、產生 Evidence 到結案完整操作，並保留 optimistic locking。
4. Pattern 結果是固定、唯讀、可重現的規則；Pattern Library 可由 UI 唯讀查看。
5. Event Hunter 可用受信任且正確編碼的連結開啟 Grafana Explore／Dashboard／Alerting、Tempo trace 與 Loki logs。
6. 符合資格的 Grafana 告警 payload 可經簽章 Webhook intake 建立或連結案件；Quality Worker 與
   Grafana assets 可重建。Phase 1 驗收的是受保護的 intake contract，不包含 provisioned Contact Point、
   Notification Policy 或真實 Grafana firing 自動送達 Event Hunter 的完整路徑。
7. Outbox → Debezium → Redpanda → ClickHouse Kafka Connect Sink → raw landing／Materialized Views → Timeline 的展示流程可由測試重現。
8. Viewer／Investigator／Admin 的 UI 可見操作與後端授權一致。
9. Backend、Frontend、Karate、契約驗證及效能門檻全部通過，重啟後設定與必要資料仍可使用。
10. 預設 UI 不顯示 Phase 2／3 Replay 控制，且 `TEMPORAL_ENABLED=false` 時核心功能正常。
11. Scenario Lab 清楚區分 S1、S12～S14 live services 與 S2～S11 isolated injection，所有結果由實際 Kafka／ClickHouse／OTel 回查，且不取代三個 demo services。

Temporal adapter `EH-MVP-010` 是選用項，不阻擋 Phase 1 完成。

## 3. 目前基準

### 已有可執行成果

- Go API、Demo Session、RBAC、Timeline、Investigation、Pattern、Summary 與 Evidence API。
- PostgreSQL／ClickHouse migrations、fixture loader 與三個 Demo domain services。
- 34 筆可重播 Domain Event fixture，涵蓋正常、付款失敗、配送完成、派車重試與退貨退款，並以 JSON Schema、Timeline Karate 與 Tempo／Loki E2E 驗證。
- 三個 Demo services 的 OpenTelemetry SDK、HTTP／Kafka instrumentation、W3C outbox context propagation，及真實 Order → Payment → Shipping telemetry vertical-slice／restart test；live SDK、synthetic fixture 與未啟用的 optional `otelc` profile 已明確分流。
- 三個 Demo services 依 [Live Event Observability Contract](../../contracts/live-event-observability-contract.md) 記錄具名的事件發布前、outbox commit 後、失敗與 consumer completed logs；Timeline 跳到 Loki 後可直接辨識每個 event type 與執行階段。
- Debezium、Redpanda broker、ClickHouse Kafka Connect Sink、domain／processing-attempt admission pipeline 與 Compose 設定。
- Quality Worker、Grafana datasource、Dashboard 與 Alert rules provisioning。
- Grafana signed webhook intake、replay protection、去重、建案、Evidence receipt 與 Alerting detail deep link；
  現有測試直接送出簽章 webhook fixture，並不代表 Grafana 已 provision Contact Point／Notification Policy。
- React `/login`、`/timeline`、`/investigations`，案件 Cursor 分頁與右側詳細抽屜。
- React `/scenario-lab`、S1～S14 catalog、PostgreSQL run state，以及 Expected／Actual／checks／deep links；S1、S12～S14 實跑三服務，S2～S11 使用隔離 `event-lab.events`。
- Timeline 已支援四種基本識別碼、可編輯的 bounded from/to，以及 Pattern Registry、Grafana alert fingerprint、最低案件 severity 與其餘 allowlist 進階條件。
- Backend Karate 16 features／92 scenarios、Frontend Karate 17/17、Frontend Vitest 49/49 的正式通過基準。
- `scripts/test-phase-1-exit.sh` 已串接契約、Go／Frontend、Karate、真實 OpenTelemetry service chain、ingestion、Quality、Grafana 與 CI performance profile；加入 Scenario Lab 後的完整 disruptive chain 已於 2026-08-21 通過並產生新版 exit report。
- CI performance dataset 已驗證 100,000 events／20,000 correlation IDs／10,000 attempts／1,440 quality windows；最近一次 200 次四操作 query mix 為 0 error，Timeline p95 42.09 ms、Summary p95 41.42 ms。
- Quality E2E 使用正式 `payment.events` topic 上保留的 synthetic partition 99，避免固定窗口與產品展示事件互相污染；Loki 啟用 WAL 與 graceful shutdown flush，確保剛接收的 live canonical logs 也能通過 restart persistence。

### Phase 1 完成後核定

- Timeline 進階契約已定案並接上 API／UI：`pattern_id` 解析 ACTIVE Pattern 的 required／expected／exclusion 事件類型，`alert_id` 明確採 Grafana receipt fingerprint，最低 `severity` 依 `investigation_cases.severity` 解析 correlation；PostgreSQL qualifier 與 ClickHouse 時間／事件查詢由 application service 組合。
- Timeline、案件 Timeline 與 Evidence 的 Grafana Explore／Dashboard／Alerting、Tempo、Loki 及 Pattern outbound links 已完成；Grafana datasource、Dashboard 與 Alert rule UID 也已由 Backend Karate 對實際 provisioned API 驗證。
- Frontend API client 已改由 `openapi-typescript` 產生型別並由 `openapi-fetch` 執行，`pnpm api:check` 會阻擋 generated schema 漂移。
- Quality scheduler／backfill、worker failure mode、restart persistence 與 Grafana provisioning 已完成可執行驗證。
- Ingestion source acknowledgement、schema-violation DLQ／checksum metadata、transport dedup 與 processing-attempt identity 已完成可執行驗證。
- API request／PostgreSQL／ClickHouse timeout、每遠端位址 rate limit、ClickHouse 唯讀 query budget 與 429／504 契約已完成；Investigation 與 Grafana webhook 的 PostgreSQL SQL、Timeline／Investigation 的 ClickHouse SQL 均已移出 HTTP Handler，由 application service 與 repository／read-model adapter 承接。Local exit gate 與 CI workflow 已建立；Phase 1 final sign-off 本輪改以本機 `test-phase-1-exit.sh` 證據完成（不以 hosted CI 作為本輪 gate）。
- Investigation application service 已依 screaming architecture 按業務能力拆分為 `case_lifecycle`、`forensics`、`event_search`、`pattern_analysis` 與 `alert_intake`；舊平面 package 與通用 facade 已移除，正式 API composition root 與 infrastructure adapter 直接依賴各能力邊界。詳細邊界見 `requirements/architecture/application-screaming-architecture.md`。
- `implementation-plan.yaml` 已依 acceptance 與完整 Exit Gate 核定；本機 Phase 1／1.1 功能交付均為
  `completed`。尚未完成的是需要目標環境與治理決策的 `P1.1-08-02` production hardening，以及選用的
  `EH-MVP-010` Temporal adapter；兩者都不回溯否定本機 release baseline。

## 4. Phase 1 收尾工作包

| 順序 | 工作包 | 目前狀態 | Phase 1 剩餘交付 |
|---|---|---|---|
| P1-01 | 規格與任務 DAG | 已完成 | Scope、traceability、implementation plan、原型矩陣與 sign-off 已同步核定。 |
| P1-02 | Timeline Search | 已完成 | 四種基本 key、from/to、完整進階 allowlist、masked payload、空結果與 truncated 狀態已完成；Pattern／Alert fingerprint／最低案件 severity 契約已由 OpenAPI、application boundary、component test 與 Karate E2E 驗證。 |
| P1-03 | Timeline Event Detail | 已完成 | Event metadata、processing summary、masked payload、trusted Grafana／Tempo／Loki links 與 backend／frontend E2E 均已完成；固定 fixture 以獨立 synthetic resource identity 載入 OTLP traces／logs，真實服務則以 OTel SDK、`otelhttp`、`otelslog` 與 franz-go `kotel` 產生 telemetry，並經 outbox／Kafka W3C context 延續同一條 trace。Live E2E 會比對 ClickHouse／Tempo／Loki trace ID、canonical log attributes、具名 `PREPARING`／`COMMITTED` lifecycle pairs 與 restart persistence；`COMMITTED` 僅代表 outbox transaction 成功，Kafka delivery 另由 consumer coordinates 證明。`otelc` 僅列為未啟用的後續 profile。 |
| P1-04 | Investigation Console | 已完成 | Drawer 內 Summary／Timeline／Patterns／Evidence／Audit、來源狀態、partial warning、操作錯誤回饋與 E2E 均已完成。 |
| P1-05 | Pattern Library | 已完成 | `contracts/patterns/*.yaml` 是唯一規則來源，generator 產生唯讀 Go registry，契約驗證會阻擋 drift；分析器依 `occurred_at` 執行 trigger-relative PT5M window、maturity 與 exclusion 判斷，未知／停用 Pattern 回 422。`/patterns?pattern_id=...` 可定位並醒目顯示規則，POST 以 405 拒絕。 |
| P1-06 | Evidence References | 已完成 | 分析流程保存 Event／Trace／Finding reference 與 checksum；manifest、source status、partial warning、allowlisted source actions、component／Karate E2E 均已完成。 |
| P1-07 | Grafana signed alert intake 與 outbound links | 已完成 | 直接送入的合格 firing／resolved webhook receipt 會保存為案件 Evidence；duplicate 不重複寫入；Alerting detail 僅由驗證過的相對路徑搭配 trusted origin 開啟。4 個 datasource、Event Quality Dashboard 與實際 Alert rule UID 均由 Grafana API smoke test 驗證，Backend／Frontend Karate 均通過。此工作包不宣稱 Grafana 已 provision Contact Point／Notification Policy；真實 firing 自動送達與建案列入 Phase 1.1 P1.1-06。 |
| P1-08 | Pipeline／Quality 收尾 | 已完成 | Compose scheduler、明確窗口 backfill、failure-only window、無 partial row failure mode、restart persistence，以及 4 datasource／1 dashboard／6 healthy alert rules 均已有腳本驗證。`EH-MVP-014` 的直接驗收已完成，嚴格 task status 仍受上游 DAG 狀態牽制。 |
| P1-09 | 工程品質 | 已完成 | Generated client／registry drift gate、API protection、ingestion acknowledgement／DLQ／dedup、application boundaries、Backend Karate 92/92、Frontend Vitest 49/49、browser Karate 17/17 與 CI performance profile 均已驗收；hosted runner 依決策不是本輪 gate。 |
| P1-10 | Vertical Slice Exit Gate | 已完成 | `scripts/test-phase-1-exit.sh` 提供一鍵啟動、完整 E2E、真實 OpenTelemetry service chain、效能 profile 與 reports；含 acknowledgement、recovery 與 restart persistence 的 disruptive chain 已通過並完成本機 sign-off。 |
| P1-11 | Scenario Lab | 已完成 | S1～S14 catalog、run persistence、actual observer、Hybrid execution、隔離 topic、UI 與 OpenAPI client 已完成；S1、S12～S14 live services 與 S2～S11 injection 實跑通過。Scenario envelope、Kafka header、Tempo 與 Loki 使用同一 trace ID，Grafana links 採正式 Explore panes。 |

建議依 P1-02 → P1-03 → P1-04 → P1-05 → P1-06／P1-07 → P1-08／P1-09 → P1-10 執行。P1-01 在每個工作包完成時同步更新。

## 5. 原型 HTML 與開發階段對照

### Phase 1 必須補完

| 原型功能 | 原型位置／行為 | 正式實作要求 |
|---|---|---|
| Demo role login | Viewer／Investigator／Admin | 已有；保留後端再次授權。 |
| Business Timeline 基本查詢 | Correlation／Aggregate／Trace／Event ID + from/to | 補齊 key selector、有界時間與 URL state。 |
| Timeline 進階條件 | event type/version、producer、causation、Kafka coordinates、Pattern ID、Alert ID、severity 等 | 已完成；Pattern 來自固定 Registry，Alert ID 是 Grafana fingerprint，severity 是案件最低門檻，所有條件均經 application boundary 轉成 allowlist 查詢，不接受任意 SQL。 |
| Timeline events | Event time、producer、sequence | 已完成 trace、Kafka coordinates、processing summary、masked payload 與 detail interaction。 |
| 建立調查案件 | 從查詢結果建立案件 | 已有基礎；非 correlation 查詢必須先選定或推導 correlation ID。 |
| 案件列表與詳細 | 列表進入案件摘要與 tabs | 已完成列表／Drawer、Summary、Timeline、Patterns、Evidence、Audit。 |
| Pattern 分析 | 固定 Pattern、唯讀分析 | 已完成 trigger-relative PT5M、maturity、late-event 與 exclusion 語意，以及 idempotent finding／Evidence 寫入、未知規則錯誤與 finding detail。分析來源目前限制為最近 7 天的有界 ClickHouse 查詢。 |
| Pattern Library | 唯讀規則列表 | 已完成 `/patterns` 並連接 `listPatterns`；只呈現由正式 YAML 產生的 Go registry，不搬入原型 mock 規則；`pattern_id` deep link 可定位並醒目顯示。 |
| Evidence Bundle | Event／Trace／Finding references | 已完成完整 manifest、partial warnings、checksum 與受控 source open actions。 |
| 開啟 Grafana | Timeline、案件、Evidence、告警摘要 | 建立 trusted link builder；支援 Explore、Dashboard、Alerting、Tempo 與 Loki。 |
| 結案 | Root cause、resolution、lock version | 已有基礎；補 conflict 與 validation UX。 |

### Phase 1 建議完成，但不阻擋核心需求

| 原型功能 | 決策 |
|---|---|
| 總覽頁 | 已於 Phase 1.1 以 `REQ-EH-010`、server-side aggregates 與 source health 完成 `/dashboard`；不使用 mock 數字。 |
| UI inventory modal | 只屬原型設計盤點工具，不是產品功能，不移植到正式 UI。 |
| Temporal case workflow 狀態 | `EH-MVP-010` 選用且預設關閉。若 Phase 1 不啟用，正式 UI 應隱藏 Workflow tab，而不是顯示假 RUNNING 狀態。 |

### Phase 2：Projection Rebuild

以下項目只有在 Phase 2 契約與權限完成後才能出現在 UI：

- 建立隔離 Read Model。
- Projection Rebuild 執行與進度。
- 現有 projection 與 rebuilt projection 比較。
- Domain Invariant 驗證與 rebuild report。
- Temporal Activity retry 與人工核准。

### Phase 3：Sandbox Behavioral Replay

以下項目留到 Phase 3：

- 指定服務版本與 Sandbox 環境。
- Replay approval／execution／cancel。
- 攔截外部副作用。
- 比較 output events、database diff、external call diff 與 Domain Invariants。
- Replay comparison report 與結案。

### 永不視為 Phase 1／2／3 一般功能

- Production Redrive。
- Event Hunter 自建 Grafana／Logs／Metrics／Traces Console。
- Event Catalog／Topic Registry CRUD。
- 通用 Alert routing、On-call、Escalation。
- LLM 自動判案、多租戶完整 RBAC、任意 Connector。

Production Redrive 若未來要做，必須成立獨立產品與權限模型，不可沿用 Sandbox Replay 按鈕。

## 6. Deep Link 最小規格

Phase 1 的外部連結至少需要：

- Grafana origin、Dashboard UID 與 datasource UID 從環境設定讀取。
- Explore 變數使用 `URLSearchParams` 或等價安全編碼，不串接任意使用者 URL。
- Trace link 只接受已驗證格式的 `trace_id`；Log link 以 correlation／trace／event ID 組成受控查詢。
- Dashboard／Alerting links 只允許預先設定的 Grafana origin。
- `target="_blank"` 同時使用 `rel="noopener noreferrer"`。
- Viewer 可開啟遮罩後證據；敏感 payload 仍依後端權限決定，不能靠前端 link 解除限制。
- Component tests 驗證 URL 編碼與 untrusted-origin rejection；Frontend E2E 驗證按鈕可見、目的 URL 類型與 Pattern deep-link 定位；Backend E2E 透過 Grafana API 驗證設定中的 datasource、Dashboard 與 Alert rule UID 確實存在。

## 7. Phase 1 最終驗收命令

最終應由 repo 內固定腳本提供一鍵入口；目前至少執行：

```bash
# 完整 release gate：會啟動 stack、暫停 ClickHouse 驗證 acknowledgement，並做 restart persistence。
bash scripts/test-phase-1-exit.sh

# 已有 stack 的非中斷開發驗證。
bash scripts/test-phase-1-exit.sh --no-start --skip-disruptive
```

上述入口會產生 `build/reports/phase-1-exit-summary.json`、
`build/reports/performance-fixture-summary.json`、`build/reports/performance-summary.json`，並保留
Backend／Frontend Karate HTML reports。分項除錯時才需要個別執行：

```bash
python3 scripts/validate-contracts.py
GOCACHE=/tmp/event-hunter-go-cache go -C backend test ./...
pnpm --dir frontend run api:check
pnpm --dir frontend run typecheck
pnpm --dir frontend exec vitest run
pnpm --dir frontend run build
bash scripts/test-backend-e2e.sh
bash scripts/test-frontend-e2e.sh
bash scripts/test-ingestion-pipeline.sh
bash scripts/test-ingestion-acknowledgement.sh --yes
```

API 保護設定由 `contracts/platform/failure-policy.yaml` 定義，Compose／`.env.example` 保存可重啟的環境變數；Go 單元測試驗證 5 秒 request deadline、2 秒 PostgreSQL `statement_timeout`、3 秒 ClickHouse deadline／唯讀 query budget，以及固定窗口 429／`Retry-After`。Backend Karate 同時驗證部署後 rate-limit headers。

另外必須保存：

- Backend／Frontend Karate HTML report。
- performance profile 結果。
- Compose restart／volume persistence 驗證結果。
- Grafana datasource／Dashboard／Alert provisioning 驗證結果。
- Phase 1 任務狀態與未完成項目為零的 sign-off。

P1-08 已提供固定入口：

```bash
bash scripts/test-quality-e2e.sh
bash scripts/verify-quality-runtime.sh
bash scripts/verify-grafana-provisioning.sh
bash scripts/verify-restart-persistence.sh
```

## 8. 後續 Agent 工作規則

1. 開始工作前先讀 `requirements/product/project-scope.yaml`、本文件與對應 `EH-MVP-*`。
2. 原型是 UX 意圖，不是 API 或狀態真相；原型的 mock data 與 Temporal 假狀態不可直接搬進正式 UI。
3. 新增欄位前先確認 OpenAPI／Query allowlist；不存在的查詢能力先改契約與測試。
4. 完成一個工作包時，同步更新 implementation plan、traceability、component test 與 Karate feature。
5. 只有全部 outputs 與 acceptance 通過，才能將 task 標記為 `completed`。
6. 不得為了原型 parity 提前加入 Replay 或 Production Redrive。
