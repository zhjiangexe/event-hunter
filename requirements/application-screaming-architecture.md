# Investigation Application Screaming Architecture

更新日期：2026-08-26

## 目的

`backend/internal/contexts/investigation/application` 的正式依賴以業務能力命名，讓目錄結構先回答「Event Hunter 能做什麼」，再回答「使用哪一種技術」。HTTP、PostgreSQL、ClickHouse、Grafana 與測試 double 都透過 use-case 所宣告的窄 port 接入。

```text
Investigation Context
├── domain/
│   ├── InvestigationCase aggregate root
│   ├── patterns deterministic domain evaluator / registry
│   └── journeys versioned Journey Profile registry
├── ports/
│   └── persistence-owned audit / evidence / finding contracts
└── application/
    ├── case_lifecycle/       list · create · get · update · close · collaborate · audit · evidence
    ├── evidence_attachment/  bounded event verification · case evidence attachment · idempotency
    ├── forensics/            bounded timeline/event read model queries
    ├── event_search/         Pattern / Grafana fingerprint / severity qualification
    ├── overview/             operational snapshot · smart-search identifier resolution
    ├── business_journey/     canonical events → logistics milestones / anomalies
    ├── journey_profiles/     immutable runtime profile catalog / provenance
    ├── saved_search/         personal bounded query lifecycle and read-only presets
    ├── pattern_analysis/     deterministic pattern evaluation and finding persistence
    ├── pattern_effectiveness/ finding outcome rollup · quality signals
    ├── pattern_feedback/     valid / false-positive classification · note · audit
    └── alert_intake/         signed Grafana business-alert disposition and evidence
```

```text
HTTP / Grafana inbound adapters
        │
        ├── case_lifecycle.Service
        ├── evidence_attachment.Service
        ├── forensics.ForensicsService
        ├── event_search.EventSearchService
        ├── overview.Service
        ├── business_journey.Service
        ├── journey_profiles.Service
        ├── saved_search.Service
        ├── pattern_analysis.PatternService
        ├── pattern_effectiveness.Service
        ├── pattern_feedback.Service
        └── alert_intake.GrafanaAlertService
                │
                ▼
       Investigation domain aggregate / pattern registry / journey profile registry
                │
                ▼
 PostgreSQL repositories · ClickHouse read model · Grafana receipt repository
```

## 邊界規則

- `case_lifecycle` 是 `InvestigationCase` aggregate 的 application use case；狀態轉移、owner／priority／tags／related correlation 正規化、SLA 推導與 append-only note invariant 留在 domain aggregate。Application service 只編排 repository、actor 與 audit。
- `evidence_attachment` 以 bounded ClickHouse lookup 驗證 event 身分，呼叫 aggregate 建立只讀 reference，並透過窄 repository port 原子保存 evidence、related correlation 與 lock version；不複製 payload。
- `forensics` 只負責 bounded read-model access，不知道 HTTP、ClickHouse SQL 或 Grafana。
- `event_search` 負責跨 Pattern Registry、案件資料與 Forensics read model 的查詢條件組合；不接受任意 SQL。
- `overview` 組合控制面與事件面 health／count read models，並以明確識別碼優先順序解析 Smart Search；不自行推測不存在的關聯。
- `business_journey` 只依 versioned Journey Profile 將 canonical events 組織成 milestone，不建立或修復
  production projection；`contracts/journeys/*.yaml` 經 generator 產生 domain registry，application service
  不在 runtime 解析設定檔。
- `journey_profiles` 提供目前 API build 已編譯 profile 的唯讀 catalog；不解析 workspace YAML、不修改
  registry，也不假裝具備版本選擇或發布能力。
- `saved_search` 透過 rich domain aggregate 驗證 target、typed query 與七天時間窗；application service 只編排 owner-scoped repository 與版本化 preset。
- `pattern_analysis` 負責 deterministic registry evaluation、server-owned historical event window、
  trigger-relative pattern window、finding／evidence persistence 與 audit；時間視窗與 no-data 語意遵循
  `historical-pattern-analysis-contract.md`。
- `pattern_effectiveness` 只彙總已保存 finding 與回饋結果，不重新執行 Pattern 或修改案件。
- `pattern_feedback` 透過 finding identity 寫入 valid／false-positive 分類、note 與 audit，並保留重複提交語意。
- `alert_intake` 負責 Grafana receipt 去重、eligible disposition、建案／連案與證據寫入；raw-body HMAC 驗證留在 Grafana inbound adapter。
- Application package 不建立 database connection；所有外部存取均透過 constructor-injected port。
- `application/` 根目錄不放 service 或 facade；正式 composition root 與 platform adapters 必須依賴上述 capability package。架構回歸測試會阻止重新引用 flat application package。
- `demo/`、`platform/`、`scenario_lab/` 不是 Investigation application service 的替代品：它們分別代表示範 domain、跨 context 基礎設施與獨立 Scenario Lab context。

## Composition root 對照

`backend/cmd/api/main.go` 直接組裝 capability services：

| 能力 | Application package | 主要 outbound port |
|---|---|---|
| 案件生命週期 | `case_lifecycle` | `domain.CaseRepository`、`ports.InvestigationDetailsRepository` |
| Timeline Event 加入案件 | `evidence_attachment` | `domain.CaseEvidenceRepository`、`EventLookup`、`AuditWriter` |
| Timeline / Forensics | `forensics` | `ForensicsReadModel` |
| 進階事件搜尋 | `event_search` | `forensics.ReadModel`、`EventSearchQualifierRepository` |
| Overview／Smart Search | `overview` | control-plane snapshot、Forensics identifier resolver |
| Business Journey | `business_journey` | `forensics.ReadModel` |
| Journey Profile Registry | `journey_profiles` | immutable domain registry（無外部 store） |
| 個人 Saved Search | `saved_search` | `domain.SavedSearchRepository` |
| Pattern 分析 | `pattern_analysis` | Case repository、details repository、Forensics service |
| Pattern 成效 | `pattern_effectiveness` | finding／feedback read model |
| Pattern 回饋 | `pattern_feedback` | finding feedback repository、Audit writer |
| Grafana 告警 | `alert_intake` | `GrafanaAlertRepository` |

這樣新增 use case 時，應新增或擴充相應的業務目錄，不再把所有方法塞進一個通用 `Service`。
