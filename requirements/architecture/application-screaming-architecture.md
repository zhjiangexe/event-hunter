---
document_id: EH-DOC-ARCH-002
status: active
owner: backend
last_reviewed: 2026-08-29
source_of_truth: true
canonical_topic: backend-application-screaming-architecture
supersedes: []
---

# Backend Application Screaming Architecture

更新日期：2026-08-29

## 目的

`backend/internal/contexts/*/application` 的正式依賴以業務能力命名，讓目錄結構先回答「Event Hunter 能做什麼」，再回答「使用哪一種技術」。HTTP、PostgreSQL、ClickHouse、Grafana 與測試 double 都透過 use-case 所宣告的窄 port 接入。

```text
Investigation Context
├── domain/
│   ├── InvestigationCase aggregate root
│   ├── cases rich InvestigationCase aggregate
│   └── legacy patterns / journeys compatibility domain
├── ports/
│   └── persistence-owned audit / evidence / finding contracts
└── application/
    ├── case_lifecycle/       list · create · get · update · close · collaborate · audit · evidence
    ├── evidence_attachment/  bounded event verification · case evidence attachment · idempotency
    ├── get_investigation_summary/ cross-store summary orchestration · partial source state · retention boundary
    ├── generate_evidence_manifest/ allowlist · locator validation · checksum · partial warnings
    ├── forensics/            bounded canonical event read model queries
    ├── ingestion_issues/     contract / admission / technical DLQ safe summaries
    ├── event_search/         legacy Pattern / Grafana fingerprint qualification
    ├── overview/             operational snapshot · smart-search identifier resolution
    ├── business_journey/     deprecated Journey compatibility read
    ├── journey_profiles/     deprecated profile compatibility catalog
    ├── saved_search/         personal bounded query lifecycle and read-only presets
    ├── pattern_analysis/     deprecated Pattern compatibility evaluation
    ├── pattern_effectiveness/ legacy finding outcome rollup
    ├── pattern_feedback/     legacy finding classification · note · audit
    └── alert_intake/         signed Grafana business-alert disposition and evidence
```

```text
Scenario Lab Context
├── domain/                    S1～S14 catalog · Run state · Actual · deterministic checks
├── ports/                     run repository · publisher · observer · order starter · links · telemetry · clock
├── application/               start · get · list · asynchronous observation
└── adapters/
    ├── inbound/httpapi/       Scenario Lab HTTP contract
    └── outbound/              PostgreSQL · ClickHouse · Kafka · Order API · links · OTel · synthetic fixtures

Quality Context
├── domain/                    bounded quality window · 31-day invariant · eligible-window rule
├── ports/                     quality aggregator
├── application/               aggregate · backfill · recurring schedule
└── adapters/outbound/clickhouse/ complete aggregation SQL and HTTP execution

Ingestion Context
├── domain/                    safe technical-DLQ summary and payload checksum
├── ports/                     source · failure repository · reporter
├── application/               persist-before-commit projection and in-place retry
└── adapters/                  Kafka source · ClickHouse writer · health · logging
```

```text
Event Check Context
├── domain/
│   ├── Identifier Resolver / Scope Resolver
│   ├── immutable Check Model Registry
│   ├── deterministic Flow / Expectation / Global Check evaluators
│   └── CheckSnapshot / FindingFeedback aggregates
├── ports/
│   └── canonical event reads / Snapshot persistence / audit / Case links
└── application/
    ├── evaluate_event_check/       resolve · select Model · evaluate · hash
    ├── list_check_models/          immutable Registry list · exact version read
    ├── save_check_snapshot/        re-evaluate · compare hashes · idempotent save
    ├── get_check_snapshot/         immutable Snapshot read
    ├── classify_check_finding/     optimistic Finding feedback · audit
    └── attach_check_snapshot/      optimistic Case link · audit
```

```text
HTTP / Grafana inbound adapters
        │
        ├── Investigation application capabilities
        │       └── case · evidence · ingestion issue · saved query · alert · legacy adapters
        └── Event Check application capabilities
                └── evaluate · model registry · snapshot · feedback · case handoff
                        │
                        ▼
       Investigation domain / Event Check domain / immutable registries
                        │
                        ▼
       PostgreSQL repositories · ClickHouse read model · Grafana receipt repository
```

## 邊界規則

- `cases/lifecycle.go` 是 `InvestigationCase` aggregate 的 application use case；狀態轉移、owner／priority／tags／related correlation 正規化、SLA 推導與 append-only note invariant 留在 domain aggregate。Application service 只編排 repository、actor 與 audit。
- `cases/attach_event.go` 以 bounded ClickHouse lookup 驗證 event 身分，呼叫 aggregate 建立只讀 reference，並透過窄 repository port 原子保存 evidence、related correlation 與 lock version；不複製 payload。
- `search/forensics.go` 只負責 bounded read-model access，不知道 HTTP、ClickHouse SQL 或 Grafana。
- `search/event_search.go` 負責跨 Pattern Registry、案件資料與 Forensics read model 的查詢條件組合；不接受任意 SQL。
- `operations/overview.go` 組合控制面與事件面 health／count read models，並以明確識別碼優先順序解析 Smart Search；不自行推測不存在的關聯。
- `compatibility/business_journey.go` 是 deprecated compatibility use case，只依 versioned Journey Profile 將 canonical events 組織成 milestone，不建立或修復
  production projection；`contracts/journeys/*.yaml` 經 generator 產生 domain registry，application service
  不在 runtime 解析設定檔。
- `compatibility/journey_profiles.go` 提供 deprecated API 所需的唯讀 catalog；不解析 workspace YAML、不修改
  registry，也不假裝具備版本選擇或發布能力。
- `savedsearch/service.go` 透過 rich domain aggregate 驗證 target、typed query 與七天時間窗；application service 只編排 owner-scoped repository 與版本化 preset。
- `compatibility/pattern_analysis.go` 保留 legacy Case analysis 的 deterministic registry evaluation、server-owned historical event window、
  trigger-relative pattern window、finding／evidence persistence 與 audit；時間視窗與 no-data 語意遵循
  `historical-pattern-analysis-contract.md`。
- `compatibility/pattern_effectiveness.go` 只彙總已保存 finding 與回饋結果，不重新執行 Pattern 或修改案件。
- `compatibility/pattern_feedback.go` 透過 finding identity 寫入 valid／false-positive 分類、note 與 audit，並保留重複提交語意。
- `alerts/grafana.go` 負責 Grafana receipt 去重、eligible disposition、建案／連案與證據寫入；raw-body HMAC 驗證留在 Grafana inbound adapter。
- `search/ingestion_issues.go` 只組合 contract validation、ClickHouse admission quarantine 與 Kafka Connect technical
  DLQ 的 allowlisted metadata；不取得 raw landing payload，也不把 ingestion failure 當成業務流程偏離。
- Application package 不建立 database connection；所有外部存取均透過 constructor-injected port。
- Go package 以穩定 business capability 為粒度，不以 method 或單一 use case 建一層資料夾。Event Check 規模適合單一 `application` package，以具名檔案與 Handler／Request 區分 use case；Investigation 則固定分成 `alerts`、`cases`、`compatibility`、`operations`、`savedsearch`、`search` 六個 capability packages。
- `demo/`、`platform/`、`contexts/scenario_lab` 不是 Investigation application service 的替代品：它們分別代表示範 domain、跨 context 基礎設施與獨立 Scenario Lab bounded context。
- Scenario Lab 的 fixture envelope factory 位於 outbound adapter；domain／application 不依賴 demo event、Kafka、SQL、HTTP 或 OTel。
- Quality 與 Ingestion worker 的 scheduling／projection 語意位於 application，SQL／Kafka record identity／health／logging 位於 adapters；`cmd` 只處理 CLI、wiring、signal 與 shutdown。
- `internal/architecture/dependencies_test.go` 以 package import 與 source guardrail 阻止 domain／application 向外依賴、flat context source 與 `cmd` SQL 回流。
- Event Check application 不接受前端提交 deterministic result；`save_snapshot.go` 必須依原 request、
  `as_of` 與 pinned Model 重算並核對 hashes。一般 evaluation 不寫 PostgreSQL。
- Event Check Snapshot 只保存 event metadata、payload checksum 與 relation provenance，不複製 ClickHouse raw payload。

## Composition root 對照

`backend/cmd/api/main.go` 直接組裝 capability services：

| 能力 | Application package | 主要 outbound port |
|---|---|---|
| 案件生命週期／Evidence／Summary／Manifest | `investigation/application/cases` | Case repositories、Forensics read model、Unit of Work、Audit writer |
| Canonical event read／進階搜尋／Ingestion Issues | `investigation/application/search` | ClickHouse read models、Event Search qualifier repository |
| Overview／Smart Search | `investigation/application/operations` | control-plane snapshot、Forensics identifier resolver |
| Legacy Journey／Profile／Pattern | `investigation/application/compatibility` | immutable registries、Case repositories、Forensics service |
| 個人 Saved Search | `investigation/application/savedsearch` | `domain.SavedSearchRepository` |
| Grafana 告警 | `investigation/application/alerts` | `GrafanaAlertRepository` |
| Event Check 評估／Model／Snapshot／Feedback／Case handoff | `eventcheck/application` | canonical event read port、Snapshot repository、Unit of Work、Audit writer |
| Scenario Lab 執行 | `scenario_lab/application` | Run repository、Publisher、Observer、Order starter、Link／Telemetry ports |
| Quality aggregation | `quality/application` | Quality Aggregator |
| Technical DLQ projection | `ingestion/application` | DLQ Source、Failure Repository、Reporter |

新增 use case 時，先加入現有 capability package 的具名檔案；只有形成新的穩定業務能力且具備獨立 vocabulary／dependency boundary 時，才新增 package。不得按 HTTP method 或單一 application method 建資料夾。
